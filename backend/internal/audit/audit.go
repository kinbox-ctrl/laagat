// Package audit is the deterministic analysis and validation engine. Given a
// BOQ, an independent takeoff and reference rates, it produces an assessed
// estimate and a list of evidence-backed findings. It never calls AI and never
// changes its inputs.
package audit

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"buildcost/internal/calc"
)

// Thresholds (percent) for raising variance findings.
const (
	QtyReviewPct  = 5.0
	QtyMajorPct   = 15.0
	RateReviewPct = 15.0
	RateMajorPct  = 30.0
	StaleRateDays = 90
)

const (
	SevInfo   = "info"
	SevReview = "review"
	SevMajor  = "major"
)

type BOQLine struct {
	ID          int64
	SlNo        string
	Description string
	Unit        string
	Qty         float64
	Rate        float64
	Amount      float64
	WorkItem    string
}

type Input struct {
	Service       string  // audit | estimate
	DeclaredTotal float64 // total stated on the contractor's quotation (0 if unknown)
	Lines         []BOQLine
	Takeoff       map[string]float64 // net measured quantity per work item, canonical unit
	Rates         map[string]calc.ResourceRate
	AsOf          time.Time
}

type AssessedLine struct {
	BOQItemID      int64    `json:"boq_item_id"`
	SlNo           string   `json:"sl_no"`
	Description    string   `json:"description"`
	WorkItem       string   `json:"work_item"`
	Category       string   `json:"category"`
	Unit           string   `json:"unit"`
	QuotedQty      float64  `json:"quoted_qty"`
	QuotedRate     float64  `json:"quoted_rate"`
	QuotedAmount   float64  `json:"quoted_amount"`
	CanonUnit      string   `json:"canon_unit"`
	MeasuredQty    *float64 `json:"measured_qty"`   // this line's share, in the line's own unit
	ReferenceRate  *float64 `json:"reference_rate"` // per the line's own unit
	AssessedQty    float64  `json:"assessed_qty"`
	AssessedRate   float64  `json:"assessed_rate"`
	AssessedAmount float64  `json:"assessed_amount"`
	QtyBasis       string   `json:"qty_basis"`  // measured | quoted
	RateBasis      string   `json:"rate_basis"` // reference | quoted
}

type Finding struct {
	Fingerprint string         `json:"fingerprint"`
	Code        string         `json:"code"`
	Severity    string         `json:"severity"`
	Title       string         `json:"title"`
	Explanation string         `json:"explanation"`
	Action      string         `json:"required_action"`
	Evidence    map[string]any `json:"evidence"`
	Impact      float64        `json:"amount_impact"`
	BOQItemID   *int64         `json:"boq_item_id"`
}

type CategoryTotal struct {
	Category string  `json:"category"`
	Quoted   float64 `json:"quoted"`
	Assessed float64 `json:"assessed"`
}

type Summary struct {
	Service         string              `json:"service"`
	RuleVersion     string              `json:"rule_version"`
	DeclaredTotal   float64             `json:"declared_total"`
	QuotedTotal     float64             `json:"quoted_total"` // sum of BOQ line amounts
	AssessedTotal   float64             `json:"assessed_total"`
	Difference      float64             `json:"difference"` // quoted - assessed
	MeasuredValue   float64             `json:"measured_value"`
	MeasuredPct     float64             `json:"measured_pct"` // share of quoted value with independent quantities
	RateCheckedPct  float64             `json:"rate_checked_pct"`
	ByCategory      []CategoryTotal     `json:"by_category"`
	Materials       []calc.MaterialLine `json:"materials"`
	MaterialsBasis  string              `json:"materials_basis"`
	FindingCounts   map[string]int      `json:"finding_counts"`
	UnverifiedRates bool                `json:"unverified_rates"`
}

type Result struct {
	Lines    []AssessedLine `json:"lines"`
	Findings []Finding      `json:"findings"`
	Summary  Summary        `json:"summary"`
}

func pct(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return calc.Round((a-b)/b*100, 1)
}

func severityFor(absPct, review, major float64) string {
	switch {
	case absPct >= major:
		return SevMajor
	case absPct >= review:
		return SevReview
	}
	return ""
}

func inr(v float64) string { return "Rs " + FormatINR(v) }

// FormatINR formats an amount with Indian digit grouping (12,34,567).
func FormatINR(v float64) string {
	neg := v < 0
	n := int64(math.Round(math.Abs(v)))
	s := fmt.Sprintf("%d", n)
	if len(s) > 3 {
		head, tail := s[:len(s)-3], s[len(s)-3:]
		var parts []string
		for len(head) > 2 {
			parts = append([]string{head[len(head)-2:]}, parts...)
			head = head[:len(head)-2]
		}
		parts = append([]string{head}, parts...)
		s = strings.Join(parts, ",") + "," + tail
	}
	if neg {
		return "-" + s
	}
	return s
}

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, t := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == ':')
	}) {
		if len(t) > 1 {
			out[t] = true
		}
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for t := range a {
		if b[t] {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}

// Run executes every check and builds the assessed estimate.
func Run(in Input) Result {
	res := Result{}
	add := func(f Finding) { res.Findings = append(res.Findings, f) }
	isAudit := in.Service != "estimate"

	// Group quoted quantity per work item (canonical units) so that a takeoff
	// total can be compared against several BOQ lines of the same work.
	type group struct {
		quotedCanon float64
		lines       []int
		convertible bool
	}
	groups := map[string]*group{}
	canonQty := make([]float64, len(in.Lines))
	unitFactor := make([]float64, len(in.Lines)) // line unit -> canonical
	measurable := make([]bool, len(in.Lines))

	for i, l := range in.Lines {
		id := l.ID
		w := calc.Item(l.WorkItem)
		lineUnit, factor, unitOK := calc.NormalizeUnit(l.Unit)

		// Arithmetic of the line itself.
		if l.Qty != 0 && l.Rate != 0 && l.Amount != 0 {
			calcAmt := calc.Round(l.Qty*l.Rate, 2)
			if math.Abs(calcAmt-l.Amount) > math.Max(1, 0.005*math.Abs(l.Amount)) {
				add(Finding{
					Fingerprint: fmt.Sprintf("ARITH-LINE:%d", id), Code: "ARITH-LINE", Severity: SevMajor, BOQItemID: &id,
					Title:       fmt.Sprintf("Item %s: amount does not equal quantity x rate", l.SlNo),
					Explanation: fmt.Sprintf("%g %s x %s = %s, but the quotation states %s.", l.Qty, l.Unit, inr(l.Rate), inr(calcAmt), inr(l.Amount)),
					Action:      "Ask the contractor to correct the arithmetic and confirm which figure applies.",
					Evidence:    map[string]any{"quantity": l.Qty, "rate": l.Rate, "stated_amount": l.Amount, "computed_amount": calcAmt},
					Impact:      calc.Round(l.Amount-calcAmt, 2),
				})
			}
		}

		if !isAudit && l.Rate == 0 {
			add(Finding{
				Fingerprint: fmt.Sprintf("RATE-MISSING:%d", id), Code: "RATE-MISSING", Severity: SevMajor, BOQItemID: &id,
				Title:       fmt.Sprintf("Item %s has no rate", l.SlNo),
				Explanation: fmt.Sprintf("\"%s\" is in the estimate with a quantity but no rate, so it contributes nothing to the total.", l.Description),
				Action:      "Enter a sourced rate for this item, or add reference prices so a build-up can be computed.",
				Evidence:    map[string]any{"quantity": l.Qty, "unit": l.Unit},
			})
		}

		if w == nil {
			add(Finding{
				Fingerprint: fmt.Sprintf("UNMAPPED:%d", id), Code: "UNMAPPED", Severity: SevReview, BOQItemID: &id,
				Title:       fmt.Sprintf("Item %s is not mapped to a supported work item", l.SlNo),
				Explanation: fmt.Sprintf("\"%s\" could not be matched to the supported residential work items, so its quantity and rate were not independently checked and it is carried at the quoted amount.", l.Description),
				Action:      "Ask the contractor to describe exactly what this item covers and how the amount was arrived at.",
				Evidence:    map[string]any{"description": l.Description, "amount": l.Amount},
			})
			continue
		}

		lumpsum := unitOK && lineUnit == calc.UnitLS
		if lumpsum {
			sev := SevReview
			if w.Unit == calc.UnitLS {
				sev = SevInfo
			}
			if isAudit {
				add(Finding{
					Fingerprint: fmt.Sprintf("LUMPSUM:%d", id), Code: "LUMPSUM", Severity: sev, BOQItemID: &id,
					Title:       fmt.Sprintf("Item %s (%s) is quoted as a lump sum", l.SlNo, w.Name),
					Explanation: fmt.Sprintf("%s is included as a lump sum with no measurable quantity, so the scope it covers cannot be verified from the quotation.", inr(l.Amount)),
					Action:      "Request an itemised breakdown (quantities, makes and specifications) before signing.",
					Evidence:    map[string]any{"amount": l.Amount, "expected_unit": w.Unit},
				})
			}
			continue
		}
		if w.Unit == calc.UnitLS {
			continue // enumerated services: no canonical quantity to compare
		}
		if !unitOK || lineUnit != w.Unit {
			add(Finding{
				Fingerprint: fmt.Sprintf("UNIT:%d", id), Code: "UNIT-MISMATCH", Severity: SevReview, BOQItemID: &id,
				Title:       fmt.Sprintf("Item %s: unit \"%s\" does not match the measurement unit for %s", l.SlNo, l.Unit, w.Name),
				Explanation: fmt.Sprintf("This work is normally measured in %s (%s). The quoted unit \"%s\" cannot be converted, so the quantity and rate could not be compared.", w.Unit, w.Rule, l.Unit),
				Action:      "Ask the contractor to restate the item in the standard unit of measurement.",
				Evidence:    map[string]any{"quoted_unit": l.Unit, "expected_unit": w.Unit},
			})
			continue
		}
		measurable[i] = true
		unitFactor[i] = factor
		canonQty[i] = l.Qty * factor
		g := groups[l.WorkItem]
		if g == nil {
			g = &group{}
			groups[l.WorkItem] = g
		}
		g.quotedCanon += canonQty[i]
		g.lines = append(g.lines, i)
	}

	// Duplicate / overlapping items.
	if isAudit {
		tok := make([]map[string]bool, len(in.Lines))
		for i, l := range in.Lines {
			tok[i] = tokens(l.Description)
		}
		for i := 0; i < len(in.Lines); i++ {
			for j := i + 1; j < len(in.Lines); j++ {
				a, b := in.Lines[i], in.Lines[j]
				if a.WorkItem != b.WorkItem {
					continue
				}
				sim := jaccard(tok[i], tok[j])
				if sim < 0.75 {
					continue
				}
				id := b.ID
				add(Finding{
					Fingerprint: fmt.Sprintf("DUP:%d:%d", a.ID, b.ID), Code: "DUPLICATE", Severity: SevMajor, BOQItemID: &id,
					Title:       fmt.Sprintf("Items %s and %s appear to charge for the same work", a.SlNo, b.SlNo),
					Explanation: fmt.Sprintf("\"%s\" (%s) and \"%s\" (%s) have near-identical descriptions (%.0f%% similar).", a.Description, inr(a.Amount), b.Description, inr(b.Amount), sim*100),
					Action:      "Ask the contractor to confirm these are distinct scopes; if not, one of them should be removed.",
					Evidence:    map[string]any{"item_a": a.SlNo, "item_b": b.SlNo, "similarity": calc.Round(sim, 2), "amount_a": a.Amount, "amount_b": b.Amount},
					Impact:      math.Min(a.Amount, b.Amount),
				})
			}
		}
	}

	// Quantity variance per work item against the independent takeoff.
	scale := map[string]float64{} // measured / quoted per work item
	for code, g := range groups {
		measured, ok := in.Takeoff[code]
		if !ok || g.quotedCanon == 0 {
			continue
		}
		scale[code] = measured / g.quotedCanon
		if !isAudit {
			continue
		}
		v := pct(g.quotedCanon, measured)
		sev := severityFor(math.Abs(v), QtyReviewPct, QtyMajorPct)
		if sev == "" {
			continue
		}
		w := calc.Item(code)
		var impact float64
		var sl []string
		for _, i := range g.lines {
			l := in.Lines[i]
			sl = append(sl, l.SlNo)
			impact += (canonQty[i] - canonQty[i]*scale[code]) / unitFactor[i] * l.Rate
		}
		dir := "higher"
		if v < 0 {
			dir = "lower"
		}
		add(Finding{
			Fingerprint: "QTY:" + code, Code: "QTY-VAR", Severity: sev,
			Title:       fmt.Sprintf("%s: quoted quantity is %.1f%% %s than the independent takeoff", w.Name, math.Abs(v), dir),
			Explanation: fmt.Sprintf("The quotation (item %s) carries %.2f %s; the measurement sheet totals %.2f %s. At the quoted rates the difference is worth %s.", strings.Join(sl, ", "), g.quotedCanon, w.Unit, measured, w.Unit, inr(impact)),
			Action:      "Reconcile the quantity with the contractor against the drawings and the measurement sheet.",
			Evidence:    map[string]any{"work_item": code, "quoted_qty": calc.Round(g.quotedCanon, 3), "measured_qty": calc.Round(measured, 3), "unit": w.Unit, "variance_pct": v, "rule": w.Rule},
			Impact:      calc.Round(impact, 2),
		})
	}

	// Assessed lines and rate variance.
	buildUps := map[string]calc.RateBuildUp{}
	stale := map[string]string{}
	var quotedTotal, assessedTotal, measuredValue, rateCheckedValue float64
	catTotals := map[string]*CategoryTotal{}
	qtyForMaterials := map[string]float64{}

	for i, l := range in.Lines {
		w := calc.Item(l.WorkItem)
		al := AssessedLine{
			BOQItemID: l.ID, SlNo: l.SlNo, Description: l.Description, WorkItem: l.WorkItem, Unit: l.Unit,
			QuotedQty: l.Qty, QuotedRate: l.Rate, QuotedAmount: l.Amount,
			AssessedQty: l.Qty, AssessedRate: l.Rate, QtyBasis: "quoted", RateBasis: "quoted", Category: "Unclassified",
		}
		if al.QuotedAmount == 0 {
			al.QuotedAmount = calc.Round(l.Qty*l.Rate, 2)
		}
		if w != nil {
			al.Category, al.CanonUnit = w.Category, w.Unit
		}
		if measurable[i] {
			if s, ok := scale[l.WorkItem]; ok {
				m := calc.Round(l.Qty*s, 3)
				al.MeasuredQty, al.AssessedQty, al.QtyBasis = &m, m, "measured"
			}
			bu, seen := buildUps[l.WorkItem]
			if !seen {
				bu = calc.BuildRate(l.WorkItem, in.Rates)
				buildUps[l.WorkItem] = bu
				for _, bl := range bu.Lines {
					if bl.Missing || bl.Date == "" {
						continue
					}
					if d, err := time.Parse("2006-01-02", bl.Date); err == nil && in.AsOf.Sub(d).Hours() > StaleRateDays*24 {
						stale[bl.Resource] = bl.Date
					}
				}
			}
			if bu.Supported && bu.Complete {
				ref := calc.Round(bu.Rate*unitFactor[i], 2) // per the line's own unit
				al.ReferenceRate = &ref
				if !bu.AllVerified {
					res.Summary.UnverifiedRates = true
				}
				if isAudit && l.Rate > 0 {
					v := pct(l.Rate, ref)
					sev := severityFor(math.Abs(v), RateReviewPct, RateMajorPct)
					if sev != "" {
						id := l.ID
						impact := calc.Round((l.Rate-ref)*al.AssessedQty, 2)
						note, action := "", "Ask the contractor for the rate build-up (materials, labour, overheads) and supplier quotations."
						if !bu.AllVerified {
							sev = SevInfo
							note = " The reference uses prices that are not yet verified for this location, so this is indicative only and has not been applied to the assessed estimate."
							action = "Treat as indicative. Dated local supplier quotations for the resources in this build-up are needed to confirm it."
						}
						dir := "above"
						if v < 0 {
							dir = "below"
						}
						add(Finding{
							Fingerprint: fmt.Sprintf("RATE:%d", id), Code: "RATE-VAR", Severity: sev, BOQItemID: &id,
							Title:       fmt.Sprintf("Item %s: quoted rate is %.1f%% %s the reference build-up", l.SlNo, math.Abs(v), dir),
							Explanation: fmt.Sprintf("%s is quoted at %s per %s; the reference build-up (materials %s + labour %s + %.0f%% overhead and profit, per %s) gives %s per %s.%s", w.Name, inr(l.Rate), l.Unit, inr(bu.Material), inr(bu.Labour), bu.OverheadPct, w.Unit, inr(ref), l.Unit, note),
							Action:      action,
							Evidence:    map[string]any{"work_item": l.WorkItem, "quoted_rate": l.Rate, "reference_rate": ref, "variance_pct": v, "build_up": bu},
							Impact:      impact,
						})
					}
				}
				if bu.AllVerified && isAudit {
					al.AssessedRate, al.RateBasis = ref, "reference"
				}
			}
			qtyForMaterials[l.WorkItem] += al.AssessedQty * unitFactor[i]
		}
		al.AssessedAmount = calc.Round(al.AssessedQty*al.AssessedRate, 2)
		if al.QtyBasis == "quoted" && al.RateBasis == "quoted" {
			al.AssessedAmount = al.QuotedAmount // nothing independently assessed: carry the quoted figure
		}
		quotedTotal += al.QuotedAmount
		assessedTotal += al.AssessedAmount
		if al.QtyBasis == "measured" {
			measuredValue += al.QuotedAmount
		}
		if al.ReferenceRate != nil {
			rateCheckedValue += al.QuotedAmount
		}
		ct := catTotals[al.Category]
		if ct == nil {
			ct = &CategoryTotal{Category: al.Category}
			catTotals[al.Category] = ct
		}
		ct.Quoted += al.QuotedAmount
		ct.Assessed += al.AssessedAmount
		res.Lines = append(res.Lines, al)
	}

	// Quotation total arithmetic.
	if isAudit && in.DeclaredTotal > 0 && math.Abs(in.DeclaredTotal-quotedTotal) > math.Max(1, 0.001*in.DeclaredTotal) {
		add(Finding{
			Fingerprint: "ARITH-TOTAL", Code: "ARITH-TOTAL", Severity: SevMajor,
			Title:       "Quoted total does not equal the sum of the BOQ items",
			Explanation: fmt.Sprintf("The quotation states a total of %s, but the listed items add up to %s (difference %s).", inr(in.DeclaredTotal), inr(quotedTotal), inr(in.DeclaredTotal-quotedTotal)),
			Action:      "Ask the contractor what the difference covers (taxes, contingencies, unlisted items) or to correct the total.",
			Evidence:    map[string]any{"declared_total": in.DeclaredTotal, "sum_of_items": calc.Round(quotedTotal, 2)},
			Impact:      calc.Round(in.DeclaredTotal-quotedTotal, 2),
		})
	}

	// Scope that a complete house quotation is expected to address.
	present := map[string]bool{}
	for _, l := range in.Lines {
		present[l.WorkItem] = true
	}
	for _, w := range calc.Catalog {
		if !w.Required || present[w.Code] {
			continue
		}
		title := fmt.Sprintf("%s is not separately identified", w.Name)
		expl := fmt.Sprintf("No item in the BOQ was identified as %s. It may be excluded, or included inside another item without being stated.", strings.ToLower(w.Name))
		action := "Confirm in writing whether this work is included, and if so under which item and at what specification."
		if !isAudit {
			title = fmt.Sprintf("%s is not in the estimate", w.Name)
			expl = fmt.Sprintf("The estimate has no item for %s, which a complete house normally requires.", strings.ToLower(w.Name))
			action = "Measure and add this work, or record it as an exclusion so the client knows the estimate does not cover it."
		}
		add(Finding{
			Fingerprint: "MISSING:" + w.Code, Code: "MISSING-SCOPE", Severity: SevReview,
			Title: title, Explanation: expl, Action: action,
			Evidence: map[string]any{"work_item": w.Code, "category": w.Category},
		})
	}

	if len(stale) > 0 {
		var codes []string
		for c := range stale {
			codes = append(codes, c)
		}
		sort.Strings(codes)
		add(Finding{
			Fingerprint: "RATE-STALE", Code: "RATE-STALE", Severity: SevInfo,
			Title:       fmt.Sprintf("%d reference price(s) are more than %d days old", len(codes), StaleRateDays),
			Explanation: "Reference prices used in the rate build-ups are dated: " + strings.Join(codes, ", ") + ". Material prices move; older references weaken rate comparisons.",
			Action:      "Refresh these prices with current dated quotations before relying on the rate comparisons.",
			Evidence:    map[string]any{"resources": stale},
		})
	}

	sort.SliceStable(res.Findings, func(i, j int) bool {
		return sevRank(res.Findings[i].Severity) > sevRank(res.Findings[j].Severity)
	})

	s := &res.Summary
	s.Service, s.RuleVersion = in.Service, calc.RuleVersion
	s.DeclaredTotal = in.DeclaredTotal
	s.QuotedTotal, s.AssessedTotal = calc.Round(quotedTotal, 2), calc.Round(assessedTotal, 2)
	s.Difference = calc.Round(quotedTotal-assessedTotal, 2)
	s.MeasuredValue = calc.Round(measuredValue, 2)
	if quotedTotal > 0 {
		s.MeasuredPct = calc.Round(measuredValue/quotedTotal*100, 1)
		s.RateCheckedPct = calc.Round(rateCheckedValue/quotedTotal*100, 1)
	}
	for _, c := range append(calc.Categories(), "Unclassified") {
		if ct, ok := catTotals[c]; ok {
			ct.Quoted, ct.Assessed = calc.Round(ct.Quoted, 2), calc.Round(ct.Assessed, 2)
			s.ByCategory = append(s.ByCategory, *ct)
		}
	}
	s.Materials = calc.MaterialRequirement(qtyForMaterials)
	s.MaterialsBasis = "Computed from assessed quantities (measured where a takeoff exists, otherwise as quoted) using catalog consumption coefficients."
	s.FindingCounts = map[string]int{}
	for _, f := range res.Findings {
		s.FindingCounts[f.Severity]++
	}
	return res
}

func sevRank(s string) int {
	switch s {
	case SevMajor:
		return 3
	case SevReview:
		return 2
	}
	return 1
}
