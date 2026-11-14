package audit

import (
	"math"
	"testing"
	"time"

	"buildcost/internal/calc"
)

func verifiedRates() map[string]calc.ResourceRate {
	r := map[string]calc.ResourceRate{}
	for code, v := range map[string]float64{
		"CEMENT": 400, "SAND": 2000, "AGG20": 1500, "AGG40": 1400, "BRICK": 8, "STEEL": 60,
		"BWIRE": 80, "MASON": 900, "HELPER": 600, "BARBENDER": 900, "TILE-VT": 600,
	} {
		r[code] = calc.ResourceRate{Resource: code, Rate: v, Verified: true, EffectiveDate: "2026-11-01", Source: "test"}
	}
	return r
}

func find(res Result, code string) []Finding {
	var out []Finding
	for _, f := range res.Findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func TestAuditFindings(t *testing.T) {
	rccRef := calc.BuildRate("RCC-M20", verifiedRates()).Rate
	in := Input{
		Service: "audit", DeclaredTotal: 2000000, AsOf: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
		Rates: verifiedRates(),
		Lines: []BOQLine{
			{ID: 1, SlNo: "1", Description: "RCC M20 in footings and columns", Unit: "cum", Qty: 50, Rate: rccRef * 1.4, Amount: calc.Round(50*rccRef*1.4, 2), WorkItem: "RCC-M20"},
			{ID: 2, SlNo: "2", Description: "Brick masonry in CM 1:6", Unit: "cum", Qty: 100, Rate: 7000, Amount: 710000, WorkItem: "BW-230"},
			{ID: 3, SlNo: "3", Description: "Internal plaster 12mm CM 1:4 walls", Unit: "sqft", Qty: 5000, Rate: 40, Amount: 200000, WorkItem: "PL-INT"},
			{ID: 4, SlNo: "4", Description: "Internal plaster 12mm CM 1:4 walls", Unit: "sqft", Qty: 5000, Rate: 40, Amount: 200000, WorkItem: "PL-INT"},
			{ID: 5, SlNo: "5", Description: "Electrical works", Unit: "LS", Qty: 1, Rate: 300000, Amount: 300000, WorkItem: "ELEC"},
			{ID: 6, SlNo: "6", Description: "Modular kitchen", Unit: "LS", Qty: 1, Rate: 150000, Amount: 150000},
			{ID: 7, SlNo: "7", Description: "Vitrified tile flooring", Unit: "cum", Qty: 10, Rate: 100, Amount: 1000, WorkItem: "FLR-VT"},
		},
		Takeoff: map[string]float64{"RCC-M20": 40, "BW-230": 99},
	}
	res := Run(in)

	if f := find(res, "ARITH-LINE"); len(f) != 1 || *f[0].BOQItemID != 2 || f[0].Impact != 10000 {
		t.Errorf("ARITH-LINE: %+v", f)
	}
	if f := find(res, "DUPLICATE"); len(f) != 1 || f[0].Impact != 200000 {
		t.Errorf("DUPLICATE: %+v", f)
	}
	if f := find(res, "UNMAPPED"); len(f) != 1 || *f[0].BOQItemID != 6 {
		t.Errorf("UNMAPPED: %+v", f)
	}
	if f := find(res, "UNIT-MISMATCH"); len(f) != 1 || *f[0].BOQItemID != 7 {
		t.Errorf("UNIT-MISMATCH: %+v", f)
	}
	if f := find(res, "LUMPSUM"); len(f) != 1 || f[0].Severity != SevInfo {
		t.Errorf("LUMPSUM: %+v", f)
	}
	// RCC quoted 50 vs measured 40 = +25% -> major. Brickwork 100 vs 99 = 1% -> none.
	q := find(res, "QTY-VAR")
	if len(q) != 1 || q[0].Severity != SevMajor || q[0].Evidence["variance_pct"] != 25.0 {
		t.Errorf("QTY-VAR: %+v", q)
	}
	if want := 10 * rccRef * 1.4; math.Abs(q[0].Impact-want) > 1 {
		t.Errorf("QTY-VAR impact %v want %v", q[0].Impact, want)
	}
	// RCC rate is 40% above reference -> major.
	var rcc *Finding
	for _, f := range find(res, "RATE-VAR") {
		if *f.BOQItemID == 1 {
			rcc = &f
		}
	}
	if rcc == nil || rcc.Severity != SevMajor {
		t.Errorf("RATE-VAR for RCC: %+v", rcc)
	}
	if f := find(res, "ARITH-TOTAL"); len(f) != 1 {
		t.Errorf("ARITH-TOTAL: %+v", f)
	}
	missing := map[string]bool{}
	for _, f := range find(res, "MISSING-SCOPE") {
		missing[f.Evidence["work_item"].(string)] = true
	}
	if !missing["WP"] || !missing["STEEL"] || missing["RCC-M20"] || missing["ELEC"] {
		t.Errorf("MISSING-SCOPE: %v", missing)
	}

	// Assessed RCC line = measured qty x verified reference rate.
	l := res.Lines[0]
	if l.QtyBasis != "measured" || l.RateBasis != "reference" || math.Abs(l.AssessedAmount-40*rccRef) > 0.5 {
		t.Errorf("assessed RCC line: %+v", l)
	}
	// Unmapped and lump-sum lines carry the quoted amount unchanged.
	if res.Lines[5].AssessedAmount != 150000 || res.Lines[4].AssessedAmount != 300000 {
		t.Errorf("carried lines: %+v %+v", res.Lines[4], res.Lines[5])
	}
	var sumQ, sumA float64
	for _, l := range res.Lines {
		sumQ += l.QuotedAmount
		sumA += l.AssessedAmount
	}
	if math.Abs(sumQ-res.Summary.QuotedTotal) > 0.01 || math.Abs(sumA-res.Summary.AssessedTotal) > 0.01 {
		t.Errorf("summary totals do not match lines")
	}
	var catQ float64
	for _, c := range res.Summary.ByCategory {
		catQ += c.Quoted
	}
	if math.Abs(catQ-res.Summary.QuotedTotal) > 0.01 {
		t.Errorf("category totals %v != quoted %v", catQ, res.Summary.QuotedTotal)
	}
}

func TestUnverifiedRatesNeverChangeTheAssessment(t *testing.T) {
	rates := verifiedRates()
	c := rates["CEMENT"]
	c.Verified = false
	rates["CEMENT"] = c
	ref := calc.BuildRate("RCC-M20", rates).Rate
	res := Run(Input{
		Service: "audit", Rates: rates, AsOf: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
		Lines: []BOQLine{{ID: 1, SlNo: "1", Description: "RCC M20", Unit: "cum", Qty: 10, Rate: ref * 2, Amount: 10 * ref * 2, WorkItem: "RCC-M20"}},
	})
	f := find(res, "RATE-VAR")
	if len(f) != 1 || f[0].Severity != SevInfo {
		t.Fatalf("expected indicative info finding, got %+v", f)
	}
	if res.Lines[0].RateBasis != "quoted" || res.Summary.Difference != 0 || !res.Summary.UnverifiedRates {
		t.Fatalf("unverified rate altered the assessment: %+v %+v", res.Lines[0], res.Summary)
	}
}

func TestStaleRates(t *testing.T) {
	rates := verifiedRates()
	c := rates["CEMENT"]
	c.EffectiveDate = "2026-01-01"
	rates["CEMENT"] = c
	res := Run(Input{
		Service: "audit", Rates: rates, AsOf: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
		Lines: []BOQLine{{ID: 1, SlNo: "1", Description: "RCC M20", Unit: "cum", Qty: 10, Rate: 9000, Amount: 90000, WorkItem: "RCC-M20"}},
	})
	if len(find(res, "RATE-STALE")) != 1 {
		t.Fatalf("no stale finding: %+v", res.Findings)
	}
}

func TestEstimateModeSkipsContractorComparisons(t *testing.T) {
	res := Run(Input{
		Service: "estimate", Rates: verifiedRates(), AsOf: time.Now(),
		Lines:   []BOQLine{{ID: 1, SlNo: "1", Description: "RCC M20", Unit: "m3", Qty: 40, Rate: 9000, Amount: 360000, WorkItem: "RCC-M20"}},
		Takeoff: map[string]float64{"RCC-M20": 40},
	})
	for _, f := range res.Findings {
		if f.Code != "MISSING-SCOPE" {
			t.Errorf("unexpected finding in estimate mode: %s", f.Code)
		}
	}
	if res.Summary.AssessedTotal != 360000 {
		t.Errorf("estimate total %v", res.Summary.AssessedTotal)
	}
}

func TestFormatINR(t *testing.T) {
	for v, want := range map[float64]string{4200000: "42,00,000", 999: "999", 1000: "1,000", 123456789: "12,34,56,789", -250000: "-2,50,000"} {
		if got := FormatINR(v); got != want {
			t.Errorf("FormatINR(%v) = %s want %s", v, got, want)
		}
	}
}
