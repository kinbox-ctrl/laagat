package calc

import (
	"math"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestUnits(t *testing.T) {
	if q, ok := ConvertQty(100, "Cft", UnitM3); !ok || !near(q, 2.83168, 1e-4) {
		t.Fatalf("cft->m3 = %v %v", q, ok)
	}
	if q, ok := ConvertQty(2, "MT", UnitKg); !ok || q != 2000 {
		t.Fatalf("MT->kg = %v %v", q, ok)
	}
	if _, ok := ConvertQty(10, "sqft", UnitM3); ok {
		t.Fatal("sqft must not convert to m3")
	}
	// Rs 50 per sqft is Rs 538.2 per m2.
	if r, ok := ConvertRate(50, "Sq.Ft", UnitM2); !ok || !near(r, 538.2, 0.1) {
		t.Fatalf("rate sqft->m2 = %v %v", r, ok)
	}
	if _, _, ok := NormalizeUnit("bananas"); ok {
		t.Fatal("unknown unit accepted")
	}
}

func TestSteelWeight(t *testing.T) {
	// 12 mm bar weighs 0.889 kg/m.
	if w := SteelUnitWeight(12); !near(w, 0.889, 0.001) {
		t.Fatalf("12mm = %v", w)
	}
	r, err := ComputeMeasurement(MeasurementInput{Method: MethodSteel, Nos: 20, Length: 6.5, DiaMM: 12})
	if err != nil || !near(r.Quantity, 115.556, 0.01) {
		t.Fatalf("steel line = %v %v", r, err)
	}
}

func TestLBD(t *testing.T) {
	r, err := ComputeMeasurement(MeasurementInput{Method: MethodLBD, Nos: 2, Length: 10, Breadth: 0.9, Depth: 1.2})
	if err != nil || !near(r.Quantity, 21.6, 1e-9) {
		t.Fatalf("lbd = %v %v", r, err)
	}
	r, _ = ComputeMeasurement(MeasurementInput{Method: MethodLBD, Nos: 1, Length: 1.2, Depth: 2.1, IsDeduction: true})
	if !near(r.Quantity, -2.52, 1e-9) {
		t.Fatalf("deduction = %v", r)
	}
	if _, err := ComputeMeasurement(MeasurementInput{Method: MethodLBD, Nos: 1}); err == nil {
		t.Fatal("empty line accepted")
	}
	if _, err := ComputeMeasurement(MeasurementInput{Method: MethodLBD, Length: -1}); err == nil {
		t.Fatal("negative dimension accepted")
	}
}

func TestOpeningRulesIS1200(t *testing.T) {
	// Plaster: 0.6 x 0.6 ventilator (0.36 m2) is not deducted.
	if r := OpeningDeduction("PL-INT", 2, 0.6, 0.6, 0.23); r.Quantity != 0 {
		t.Fatalf("small opening deducted: %v", r)
	}
	// Plaster: 1.0 x 2.1 door (2.1 m2): half the opening per face.
	if r := OpeningDeduction("PL-INT", 4, 1.0, 2.1, 0.23); !near(r.Quantity, -4.2, 1e-9) {
		t.Fatalf("mid opening: %v", r)
	}
	// Plaster: 2.4 x 2.1 (5.04 m2): full deduction less half the reveal.
	r := OpeningDeduction("PL-EXT", 1, 2.4, 2.1, 0.23)
	want := -(5.04 - (2*2.1+2*2.4)*0.23/2)
	if !near(r.Quantity, want, 0.001) {
		t.Fatalf("large opening: %v want %v", r, want)
	}
	// Masonry volume: door 1.0 x 2.1 in a 230 wall.
	if r := OpeningDeduction("BW-230", 3, 1.0, 2.1, 0.23); !near(r.Quantity, -1.449, 0.001) {
		t.Fatalf("masonry opening: %v", r)
	}
	if r := OpeningDeduction("BW-230", 5, 0.3, 0.3, 0.23); r.Quantity != 0 {
		t.Fatalf("0.09 m2 masonry opening deducted: %v", r)
	}
}

func TestMixCoefficients(t *testing.T) {
	get := func(item, res string) float64 {
		for _, c := range Item(item).Coefficients {
			if c.Resource == res {
				return c.Qty
			}
		}
		t.Fatalf("no %s in %s", res, item)
		return 0
	}
	// M20 nominal mix: about 8 bags cement, 0.42 m3 sand, 0.84 m3 aggregate per m3.
	if c := get("RCC-M20", "CEMENT"); !near(c, 8.07, 0.05) {
		t.Fatalf("M20 cement = %v", c)
	}
	if s := get("RCC-M20", "SAND"); !near(s, 0.42, 0.005) {
		t.Fatalf("M20 sand = %v", s)
	}
	// PCC 1:4:8: about 3.4 bags per m3.
	if c := get("PCC-148", "CEMENT"); !near(c, 3.41, 0.05) {
		t.Fatalf("PCC cement = %v", c)
	}
}

func TestBuildRate(t *testing.T) {
	rates := map[string]ResourceRate{
		"CEMENT": {Resource: "CEMENT", Rate: 400, Verified: true},
		"SAND":   {Resource: "SAND", Rate: 2000, Verified: true},
		"AGG20":  {Resource: "AGG20", Rate: 1500, Verified: true},
		"MASON":  {Resource: "MASON", Rate: 900, Verified: true},
		"HELPER": {Resource: "HELPER", Rate: 600, Verified: false},
	}
	b := BuildRate("RCC-M20", rates)
	if !b.Supported || !b.Complete || b.AllVerified {
		t.Fatalf("flags: %+v", b)
	}
	sum := 0.0
	for _, l := range b.Lines {
		sum += l.Amount
	}
	if !near(b.Material+b.Labour, sum, 0.02) || !near(b.Rate, sum*1.15, 0.02) {
		t.Fatalf("build-up does not add up: %+v", b)
	}
	delete(rates, "SAND")
	if b := BuildRate("RCC-M20", rates); b.Complete {
		t.Fatal("build-up complete despite missing sand price")
	}
	if b := BuildRate("WP", rates); b.Supported {
		t.Fatal("waterproofing has no build-up")
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"Earthwork excavation for foundation in ordinary soil":   "EW-EXC",
		"Providing and laying PCC 1:4:8 below footings":          "PCC-148",
		"RCC M20 in footings, columns, beams and slabs":          "RCC-M20",
		"Supplying and fixing TMT steel reinforcement Fe500":     "STEEL",
		"Brick masonry in CM 1:6 in superstructure":              "BW-230",
		"115 mm half brick partition wall in CM 1:4":             "BW-115",
		"12mm thick internal plastering in CM 1:4":               "PL-INT",
		"External plaster 20 mm thick in two coats":              "PL-EXT",
		"Vitrified tile flooring 600x600":                        "FLR-VT",
		"Waterproofing treatment to terrace and toilets":         "WP",
		"Interior painting with putty, primer and emulsion":      "PAINT-INT",
		"Exterior painting with weather coat":                    "PAINT-EXT",
		"Electrical wiring and fittings":                         "ELEC",
		"Plumbing and sanitary works":                            "PLUMB",
		"Backfilling in foundation trenches with excavated soil": "EW-FILL",
		"Anti-termite treatment":                                 "ATT",
		"Shuttering and centering for RCC work":                  "FORM",
		"Modular kitchen cabinets":                               "",
	}
	for desc, want := range cases {
		if got := Classify(desc); got != want {
			t.Errorf("Classify(%q) = %q, want %q", desc, got, want)
		}
	}
}

func TestMaterialRequirement(t *testing.T) {
	m := MaterialRequirement(map[string]float64{"RCC-M20": 10, "PCC-148": 4})
	var cement float64
	for _, l := range m {
		if l.Resource == "CEMENT" {
			cement = l.Qty
		}
	}
	// 10 x 8.07 + 4 x 3.41 = about 94 bags.
	if !near(cement, 94.4, 0.6) {
		t.Fatalf("cement bags = %v", cement)
	}
}
