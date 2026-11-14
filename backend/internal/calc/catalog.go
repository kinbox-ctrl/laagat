package calc

import (
	"sort"
	"strings"
)

// RuleVersion identifies the set of measurement rules and consumption
// coefficients in this file. Bump it whenever a rule or coefficient changes;
// every analysis run and report records the version it was produced with.
//
// STATUS: DRAFT. These constants are standard textbook values and have not yet
// been signed off by a qualified quantity surveyor. Reports state this.
const RuleVersion = "2026.11-draft1"

// OverheadProfitPct is the contractor overhead and profit applied on top of
// material and labour cost in a rate build-up.
const OverheadProfitPct = 15.0

// Resource is a priced input (material or labour) used in rate build-ups.
type Resource struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Kind string `json:"kind"` // material | labour
	Unit string `json:"unit"`
}

// Coefficient is the quantity of a resource consumed per unit of a work item.
type Coefficient struct {
	Resource string  `json:"resource"`
	Qty      float64 `json:"qty"`
	Basis    string  `json:"basis"`
}

// WorkItem is a canonical, supported item of residential construction work.
type WorkItem struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Unit     string `json:"unit"`
	// Required marks scope that a complete house quotation is expected to
	// address. Its absence produces a "missing scope" finding.
	Required bool   `json:"required"`
	Rule     string `json:"rule"` // measurement rule reference
	// Keywords: the item matches a description when every word of any one
	// group is present. Exclude words veto a match.
	Keywords [][]string `json:"-"`
	Exclude  []string   `json:"-"`
	// Coefficients is empty for items with no supported rate build-up.
	Coefficients []Coefficient `json:"coefficients"`
}

var Resources = []Resource{
	{"CEMENT", "Cement OPC/PPC (50 kg bag)", "material", UnitBag},
	{"SAND", "River / M-sand (fine aggregate)", "material", UnitM3},
	{"AGG20", "Coarse aggregate 20 mm", "material", UnitM3},
	{"AGG40", "Coarse aggregate 40 mm", "material", UnitM3},
	{"BRICK", "Clay brick, modular 190x90x90", "material", UnitNos},
	{"STEEL", "TMT reinforcement bar Fe500", "material", UnitKg},
	{"BWIRE", "Binding wire", "material", UnitKg},
	{"TILE-VT", "Vitrified floor tile 600x600", "material", UnitM2},
	{"MASON", "Mason (skilled)", "labour", UnitDay},
	{"HELPER", "Helper / mazdoor (unskilled)", "labour", UnitDay},
	{"BARBENDER", "Bar bender (skilled)", "labour", UnitDay},
}

const cementBagM3 = 0.0347 // one 50 kg bag at 1440 kg/m3

// mix returns cement (bags), sand (m3) and coarse aggregate (m3) for a dry
// volume of nominal-mix material with ratio c:s:a.
func mix(dryVolume, c, s, a float64) (bags, sand, agg float64) {
	total := c + s + a
	return Round(dryVolume*c/total/cementBagM3, 3), Round(dryVolume*s/total, 4), Round(dryVolume*a/total, 4)
}

func buildCatalog() []WorkItem {
	pccC, pccS, pccA := mix(1.54, 1, 4, 8)
	rccC, rccS, rccA := mix(1.54, 1, 1.5, 3)
	bwC, bwS, _ := mix(0.23*1.33, 1, 6, 0)
	hbC, hbS, _ := mix(0.115*0.23*1.33, 1, 4, 0)
	piC, piS, _ := mix(0.012*1.30*1.25, 1, 4, 0)
	peC, peS, _ := mix(0.020*1.30*1.25, 1, 4, 0)
	tlC, tlS, _ := mix(0.020*1.25, 1, 4, 0)

	return []WorkItem{
		{Code: "EW-EXC", Name: "Earthwork excavation in foundation", Category: "Earthwork", Unit: UnitM3, Required: true,
			Rule:     "IS 1200 Part 1 - measured net as length x breadth x depth of trench/pit",
			Keywords: [][]string{{"excavation"}, {"earthwork"}, {"earth", "work"}, {"digging"}},
			Exclude:  []string{"filling", "backfill", "refilling"},
			Coefficients: []Coefficient{
				{"HELPER", 0.45, "Manual excavation in ordinary soil up to 1.5 m, incl. lift and lead up to 50 m"},
			}},
		{Code: "EW-FILL", Name: "Backfilling / plinth filling", Category: "Earthwork", Unit: UnitM3,
			Rule:     "IS 1200 Part 1 - consolidated volume of fill",
			Keywords: [][]string{{"backfill"}, {"filling"}, {"refilling"}, {"back", "fill"}},
			Coefficients: []Coefficient{
				{"HELPER", 0.30, "Filling in 200 mm layers with watering and ramming, excavated earth"},
			}},
		{Code: "ATT", Name: "Anti-termite treatment", Category: "Earthwork", Unit: UnitM2, Required: true,
			Rule:     "Measured as plan area treated",
			Keywords: [][]string{{"termite"}, {"anti", "termite"}}},
		{Code: "PCC-148", Name: "Plain cement concrete 1:4:8", Category: "Concrete", Unit: UnitM3, Required: true,
			Rule:     "IS 1200 Part 2 - net volume as laid",
			Keywords: [][]string{{"pcc"}, {"p.c.c"}, {"plain", "cement", "concrete"}, {"lean", "concrete"}, {"1:4:8"}},
			Exclude:  []string{"flooring", "tile"},
			Coefficients: []Coefficient{
				{"CEMENT", pccC, "Nominal mix 1:4:8, dry volume factor 1.54"},
				{"SAND", pccS, "Nominal mix 1:4:8, dry volume factor 1.54"},
				{"AGG40", pccA, "Nominal mix 1:4:8, dry volume factor 1.54"},
				{"MASON", 0.10, "Mixing, laying and consolidating"},
				{"HELPER", 1.60, "Mixing, laying and consolidating"},
			}},
		{Code: "RCC-M20", Name: "Reinforced cement concrete M20 (excl. steel and formwork)", Category: "RCC", Unit: UnitM3, Required: true,
			Rule:     "IS 1200 Part 2 - net volume; no deduction for reinforcement",
			Keywords: [][]string{{"rcc"}, {"r.c.c"}, {"reinforced", "concrete"}, {"m20"}, {"m25"}, {"1:1.5:3"}},
			Exclude:  []string{"steel", "tmt", "reinforcement bar", "shuttering", "formwork", "centering"},
			Coefficients: []Coefficient{
				{"CEMENT", rccC, "Nominal mix 1:1.5:3, dry volume factor 1.54"},
				{"SAND", rccS, "Nominal mix 1:1.5:3, dry volume factor 1.54"},
				{"AGG20", rccA, "Nominal mix 1:1.5:3, dry volume factor 1.54"},
				{"MASON", 0.25, "Mixing, placing, vibrating and curing"},
				{"HELPER", 2.50, "Mixing, placing, vibrating and curing"},
			}},
		{Code: "STEEL", Name: "TMT reinforcement steel Fe500, cut, bent and tied", Category: "RCC", Unit: UnitKg, Required: true,
			Rule:     "IS 1200 Part 8 - weight from bar bending schedule, unit weight d^2/162 kg/m",
			Keywords: [][]string{{"tmt"}, {"reinforcement"}, {"steel", "bar"}, {"fe500"}, {"fe", "500"}, {"steel", "work"}, {"steel"}},
			Exclude:  []string{"door", "window", "grill", "railing", "gate", "structural steel"},
			Coefficients: []Coefficient{
				{"STEEL", 1.03, "Net weight plus 3% cutting wastage"},
				{"BWIRE", 0.008, "8 kg binding wire per tonne"},
				{"BARBENDER", 0.010, "10 bar bender days per tonne"},
				{"HELPER", 0.010, "10 helper days per tonne"},
			}},
		{Code: "FORM", Name: "Formwork / shuttering", Category: "RCC", Unit: UnitM2,
			Rule:     "IS 1200 Part 5 - contact area of concrete",
			Keywords: [][]string{{"formwork"}, {"shuttering"}, {"centering"}}},
		{Code: "DPC", Name: "Damp proof course", Category: "Masonry", Unit: UnitM2, Required: true,
			Rule:     "Measured as plan area of walls at plinth level",
			Keywords: [][]string{{"dpc"}, {"d.p.c"}, {"damp", "proof"}}},
		{Code: "BW-230", Name: "Brick masonry 230 mm in CM 1:6", Category: "Masonry", Unit: UnitM3, Required: true,
			Rule:     "IS 1200 Part 3 - volume; no deduction for openings up to 0.1 m2",
			Keywords: [][]string{{"brickwork"}, {"brick", "work"}, {"brick", "masonry"}, {"masonry"}, {"9", "inch", "wall"}},
			Exclude:  []string{"half brick", "115", "4.5", "partition", "plaster"},
			Coefficients: []Coefficient{
				{"BRICK", 500, "Modular brick with 10 mm joints"},
				{"CEMENT", bwC, "Mortar 1:6, 0.23 m3 wet mortar per m3, dry factor 1.33"},
				{"SAND", bwS, "Mortar 1:6, 0.23 m3 wet mortar per m3, dry factor 1.33"},
				{"MASON", 0.90, "Laying, raking joints and curing"},
				{"HELPER", 1.40, "Laying, raking joints and curing"},
			}},
		{Code: "BW-115", Name: "Half-brick masonry 115 mm in CM 1:4", Category: "Masonry", Unit: UnitM2,
			Rule:     "IS 1200 Part 3 - walls one brick thick or less measured in area",
			Keywords: [][]string{{"half", "brick"}, {"115", "mm", "brick"}, {"4.5", "inch"}, {"partition", "wall"}},
			Coefficients: []Coefficient{
				{"BRICK", 58, "Modular brick with 10 mm joints, per m2"},
				{"CEMENT", hbC, "Mortar 1:4"},
				{"SAND", hbS, "Mortar 1:4"},
				{"MASON", 0.12, "Laying and curing"},
				{"HELPER", 0.18, "Laying and curing"},
			}},
		{Code: "PL-INT", Name: "Internal plaster 12 mm in CM 1:4", Category: "Plastering", Unit: UnitM2, Required: true,
			Rule:     "IS 1200 Part 12 - openings <=0.5 m2 not deducted; 0.5-3 m2 deducted on one face; >3 m2 deducted both faces, jambs added",
			Keywords: [][]string{{"internal", "plaster"}, {"inside", "plaster"}, {"12", "mm", "plaster"}, {"ceiling", "plaster"}, {"plaster"}},
			Exclude:  []string{"external", "outside", "outer", "20 mm", "20mm", "pop", "putty", "gypsum"},
			Coefficients: []Coefficient{
				{"CEMENT", piC, "12 mm, 30% extra for joints and undulation, dry factor 1.25, mix 1:4"},
				{"SAND", piS, "12 mm, 30% extra for joints and undulation, dry factor 1.25, mix 1:4"},
				{"MASON", 0.08, "Plastering and curing"},
				{"HELPER", 0.10, "Plastering and curing"},
			}},
		{Code: "PL-EXT", Name: "External plaster 20 mm in CM 1:4", Category: "Plastering", Unit: UnitM2, Required: true,
			Rule:     "IS 1200 Part 12 - opening rule as for internal plaster",
			Keywords: [][]string{{"external", "plaster"}, {"outside", "plaster"}, {"outer", "plaster"}, {"20", "mm", "plaster"}},
			Coefficients: []Coefficient{
				{"CEMENT", peC, "20 mm in two coats, 30% extra, dry factor 1.25, mix 1:4"},
				{"SAND", peS, "20 mm in two coats, 30% extra, dry factor 1.25, mix 1:4"},
				{"MASON", 0.11, "Plastering, scaffolding use and curing"},
				{"HELPER", 0.14, "Plastering, scaffolding use and curing"},
			}},
		{Code: "FLR-VT", Name: "Vitrified tile flooring on 20 mm mortar bed", Category: "Flooring", Unit: UnitM2, Required: true,
			Rule:     "IS 1200 Part 11 - net floor area laid",
			Keywords: [][]string{{"vitrified"}, {"tile", "flooring"}, {"floor", "tile"}, {"flooring"}, {"tiles"}},
			Exclude:  []string{"wall tile", "dado", "skirting"},
			Coefficients: []Coefficient{
				{"TILE-VT", 1.05, "Net area plus 5% cutting wastage"},
				{"CEMENT", tlC + 0.066, "20 mm bed 1:4 plus 3.3 kg/m2 cement slurry"},
				{"SAND", tlS, "20 mm bed 1:4"},
				{"MASON", 0.10, "Laying, jointing and cleaning"},
				{"HELPER", 0.12, "Laying, jointing and cleaning"},
			}},
		{Code: "WP", Name: "Waterproofing (roof, toilets, wet areas)", Category: "Finishes", Unit: UnitM2, Required: true,
			Rule:     "Measured as treated area including upturns",
			Keywords: [][]string{{"waterproofing"}, {"water", "proofing"}, {"water", "proof"}}},
		{Code: "PAINT-INT", Name: "Interior painting (putty, primer, emulsion)", Category: "Finishes", Unit: UnitM2, Required: true,
			Rule:     "IS 1200 Part 13 - area painted",
			Keywords: [][]string{{"interior", "paint"}, {"internal", "paint"}, {"emulsion"}, {"putty"}, {"painting"}, {"paint"}},
			Exclude:  []string{"exterior", "external", "weather", "apex"}},
		{Code: "PAINT-EXT", Name: "Exterior painting", Category: "Finishes", Unit: UnitM2, Required: true,
			Rule:     "IS 1200 Part 13 - area painted",
			Keywords: [][]string{{"exterior", "paint"}, {"external", "paint"}, {"weather", "coat"}, {"weatherproof", "paint"}, {"apex"}}},
		{Code: "DOOR", Name: "Doors with frames and hardware", Category: "Joinery", Unit: UnitNos, Required: true,
			Rule:     "Enumerated by type and size",
			Keywords: [][]string{{"door"}, {"doors"}}},
		{Code: "WIN", Name: "Windows and ventilators", Category: "Joinery", Unit: UnitM2, Required: true,
			Rule:     "Measured as overall frame area",
			Keywords: [][]string{{"window"}, {"windows"}, {"ventilator"}}},
		{Code: "ELEC", Name: "Electrical works", Category: "Services", Unit: UnitLS, Required: true,
			Rule:     "Enumerated per point / per fitting; lump sums require a breakdown",
			Keywords: [][]string{{"electrical"}, {"electric"}, {"wiring"}, {"electrification"}}},
		{Code: "PLUMB", Name: "Plumbing and sanitary works", Category: "Services", Unit: UnitLS, Required: true,
			Rule:     "Enumerated per fixture and per metre of pipe; lump sums require a breakdown",
			Keywords: [][]string{{"plumbing"}, {"sanitary"}, {"water", "supply"}, {"drainage"}}},
	}
}

// Catalog is the supported set of residential work items.
var Catalog = buildCatalog()

var catalogIndex = func() map[string]*WorkItem {
	m := map[string]*WorkItem{}
	for i := range Catalog {
		m[Catalog[i].Code] = &Catalog[i]
	}
	return m
}()

// Item returns the catalog work item with the given code, or nil.
func Item(code string) *WorkItem { return catalogIndex[code] }

// Categories returns catalog categories in catalog order.
func Categories() []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range Catalog {
		if !seen[w.Category] {
			seen[w.Category] = true
			out = append(out, w.Category)
		}
	}
	return out
}

func normalizeText(s string) string {
	s = strings.ToLower(s)
	repl := strings.NewReplacer(",", " ", "(", " ", ")", " ", "/", " ", "-", " ", "&", " ", ";", " ", "\n", " ", "\t", " ")
	return " " + strings.Join(strings.Fields(repl.Replace(s)), " ") + " "
}

func containsWord(text, word string) bool {
	// Substring match on word starts: "plaster" matches "plastering",
	// "tile" matches "tiles", but "pcc" does not match "upcc".
	return strings.Contains(text, " "+word)
}

// Classify maps a free-text BOQ description to a catalog work item code using
// deterministic keyword rules. It returns "" when no rule matches; such items
// must be mapped by an analyst (optionally from an AI suggestion).
func Classify(description string) string {
	text := normalizeText(description)
	type hit struct {
		code  string
		score int
	}
	var hits []hit
	for _, w := range Catalog {
		vetoed := false
		for _, ex := range w.Exclude {
			if strings.Contains(text, ex) {
				vetoed = true
				break
			}
		}
		if vetoed {
			continue
		}
		best := 0
		for _, group := range w.Keywords {
			all := true
			chars := 0
			for _, kw := range group {
				if !containsWord(text, kw) {
					all = false
					break
				}
				chars += len(kw)
			}
			// Longer, more specific keyword groups win.
			if all && len(group)*100+chars > best {
				best = len(group)*100 + chars
			}
		}
		if best > 0 {
			hits = append(hits, hit{w.Code, best})
		}
	}
	if len(hits) == 0 {
		return ""
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	return hits[0].code
}
