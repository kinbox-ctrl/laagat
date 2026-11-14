package calc

import "sort"

// ResourceRate is a dated, sourced reference price for one resource.
type ResourceRate struct {
	Resource      string  `json:"resource"`
	Rate          float64 `json:"rate"`
	Location      string  `json:"location"`
	EffectiveDate string  `json:"effective_date"`
	Source        string  `json:"source"`
	Verified      bool    `json:"verified"`
}

// BuildUpLine is one resource row of a rate build-up.
type BuildUpLine struct {
	Resource string  `json:"resource"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Unit     string  `json:"unit"`
	Qty      float64 `json:"qty"`
	Rate     float64 `json:"rate"`
	Amount   float64 `json:"amount"`
	Basis    string  `json:"basis"`
	Source   string  `json:"source"`
	Date     string  `json:"date"`
	Location string  `json:"location"`
	Verified bool    `json:"verified"`
	Missing  bool    `json:"missing"`
}

// RateBuildUp is the reference rate for one unit of a work item.
type RateBuildUp struct {
	WorkItem    string        `json:"work_item"`
	Unit        string        `json:"unit"`
	Lines       []BuildUpLine `json:"lines"`
	Material    float64       `json:"material"`
	Labour      float64       `json:"labour"`
	OverheadPct float64       `json:"overhead_pct"`
	Overhead    float64       `json:"overhead"`
	Rate        float64       `json:"rate"`
	// Supported is false for items with no coefficients in the catalog.
	Supported bool `json:"supported"`
	// Complete is true when every resource had a reference price.
	Complete bool `json:"complete"`
	// AllVerified is true when every price used is a verified, sourced rate.
	AllVerified bool `json:"all_verified"`
}

var resourceIndex = func() map[string]Resource {
	m := map[string]Resource{}
	for _, r := range Resources {
		m[r.Code] = r
	}
	return m
}()

// BuildRate computes the reference rate for a work item from consumption
// coefficients and the supplied resource prices.
func BuildRate(itemCode string, rates map[string]ResourceRate) RateBuildUp {
	w := Item(itemCode)
	b := RateBuildUp{WorkItem: itemCode, OverheadPct: OverheadProfitPct}
	if w == nil {
		return b
	}
	b.Unit = w.Unit
	if len(w.Coefficients) == 0 {
		return b
	}
	b.Supported, b.Complete, b.AllVerified = true, true, true
	for _, c := range w.Coefficients {
		res := resourceIndex[c.Resource]
		line := BuildUpLine{Resource: c.Resource, Name: res.Name, Kind: res.Kind, Unit: res.Unit, Qty: c.Qty, Basis: c.Basis}
		r, ok := rates[c.Resource]
		if !ok {
			line.Missing = true
			b.Complete = false
			b.AllVerified = false
		} else {
			line.Rate, line.Source, line.Date, line.Location, line.Verified = r.Rate, r.Source, r.EffectiveDate, r.Location, r.Verified
			line.Amount = Round(c.Qty*r.Rate, 2)
			if !r.Verified {
				b.AllVerified = false
			}
			if res.Kind == "labour" {
				b.Labour += line.Amount
			} else {
				b.Material += line.Amount
			}
		}
		b.Lines = append(b.Lines, line)
	}
	b.Material, b.Labour = Round(b.Material, 2), Round(b.Labour, 2)
	b.Overhead = Round((b.Material+b.Labour)*b.OverheadPct/100, 2)
	b.Rate = Round(b.Material+b.Labour+b.Overhead, 2)
	return b
}

// MaterialLine is the total requirement of one resource for a project.
type MaterialLine struct {
	Resource string  `json:"resource"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Unit     string  `json:"unit"`
	Qty      float64 `json:"qty"`
}

// MaterialRequirement totals resource consumption for the given work item
// quantities (canonical units), using the catalog coefficients.
func MaterialRequirement(qtyByItem map[string]float64) []MaterialLine {
	totals := map[string]float64{}
	for code, qty := range qtyByItem {
		w := Item(code)
		if w == nil {
			continue
		}
		for _, c := range w.Coefficients {
			totals[c.Resource] += c.Qty * qty
		}
	}
	var out []MaterialLine
	for _, r := range Resources {
		if q, ok := totals[r.Code]; ok {
			out = append(out, MaterialLine{r.Code, r.Name, r.Kind, r.Unit, Round(q, 2)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Kind > out[j].Kind })
	return out
}
