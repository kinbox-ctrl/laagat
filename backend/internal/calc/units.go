// Package calc is the deterministic construction calculation engine.
// It has no database, network or AI dependencies: every function is a pure
// calculation that can be unit-tested and traced back to its inputs.
package calc

import (
	"math"
	"strings"
)

// Canonical units used by the engine.
const (
	UnitM3  = "m3"
	UnitM2  = "m2"
	UnitM   = "m"
	UnitKg  = "kg"
	UnitNos = "nos"
	UnitLS  = "ls"
	UnitBag = "bag"
	UnitDay = "day"
)

type unitDef struct {
	canonical string
	factor    float64 // multiply a quantity in this unit by factor to get the canonical unit
}

var unitAliases = map[string]unitDef{
	// volume
	"m3": {UnitM3, 1}, "cum": {UnitM3, 1}, "cu.m": {UnitM3, 1}, "cu.m.": {UnitM3, 1}, "cumt": {UnitM3, 1},
	"cubic metre": {UnitM3, 1}, "cubic meter": {UnitM3, 1}, "m³": {UnitM3, 1}, "cmt": {UnitM3, 1},
	"cft": {UnitM3, 0.0283168}, "cu.ft": {UnitM3, 0.0283168}, "cuft": {UnitM3, 0.0283168}, "ft3": {UnitM3, 0.0283168},
	"brass": {UnitM3, 2.83168}, // 100 cft
	// area
	"m2": {UnitM2, 1}, "sqm": {UnitM2, 1}, "sq.m": {UnitM2, 1}, "sq.m.": {UnitM2, 1}, "sqmt": {UnitM2, 1},
	"square metre": {UnitM2, 1}, "square meter": {UnitM2, 1}, "m²": {UnitM2, 1}, "smt": {UnitM2, 1},
	"sqft": {UnitM2, 0.092903}, "sq.ft": {UnitM2, 0.092903}, "sq.ft.": {UnitM2, 0.092903}, "sft": {UnitM2, 0.092903}, "ft2": {UnitM2, 0.092903},
	// length
	"m": {UnitM, 1}, "rm": {UnitM, 1}, "rmt": {UnitM, 1}, "r.m": {UnitM, 1}, "mtr": {UnitM, 1}, "metre": {UnitM, 1}, "meter": {UnitM, 1},
	"rft": {UnitM, 0.3048}, "r.ft": {UnitM, 0.3048}, "ft": {UnitM, 0.3048}, "feet": {UnitM, 0.3048},
	// weight
	"kg": {UnitKg, 1}, "kgs": {UnitKg, 1},
	"mt": {UnitKg, 1000}, "tonne": {UnitKg, 1000}, "ton": {UnitKg, 1000}, "t": {UnitKg, 1000},
	"quintal": {UnitKg, 100}, "qtl": {UnitKg, 100},
	// count
	"nos": {UnitNos, 1}, "no": {UnitNos, 1}, "no.": {UnitNos, 1}, "nos.": {UnitNos, 1}, "each": {UnitNos, 1}, "ea": {UnitNos, 1},
	"number": {UnitNos, 1}, "point": {UnitNos, 1}, "points": {UnitNos, 1}, "pcs": {UnitNos, 1},
	// lump sum
	"ls": {UnitLS, 1}, "l.s": {UnitLS, 1}, "l.s.": {UnitLS, 1}, "lumpsum": {UnitLS, 1}, "lump sum": {UnitLS, 1}, "job": {UnitLS, 1}, "lot": {UnitLS, 1},
	// resources
	"bag": {UnitBag, 1}, "bags": {UnitBag, 1},
	"day": {UnitDay, 1}, "days": {UnitDay, 1}, "manday": {UnitDay, 1},
}

// NormalizeUnit maps a free-text unit ("Cu.M", "SFT", "MT") to a canonical
// unit and the factor that converts a quantity into that canonical unit.
func NormalizeUnit(raw string) (canonical string, factor float64, ok bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.Join(strings.Fields(key), " ")
	if d, found := unitAliases[key]; found {
		return d.canonical, d.factor, true
	}
	key = strings.ReplaceAll(key, " ", "")
	if d, found := unitAliases[key]; found {
		return d.canonical, d.factor, true
	}
	return "", 0, false
}

// ConvertQty converts qty from a free-text unit to the given canonical unit.
// ok is false when the units are unknown or dimensionally incompatible.
func ConvertQty(qty float64, fromUnit, toCanonical string) (float64, bool) {
	c, f, ok := NormalizeUnit(fromUnit)
	if !ok || c != toCanonical {
		return 0, false
	}
	return qty * f, true
}

// ConvertRate converts a rate quoted per fromUnit into a rate per canonical unit.
func ConvertRate(rate float64, fromUnit, toCanonical string) (float64, bool) {
	c, f, ok := NormalizeUnit(fromUnit)
	if !ok || c != toCanonical || f == 0 {
		return 0, false
	}
	return rate / f, true
}

// Round rounds v to the given number of decimal places.
func Round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
