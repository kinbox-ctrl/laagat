package calc

import (
	"fmt"
	"math"
)

// Measurement methods.
const (
	MethodLBD     = "lbd"     // nos x length x breadth x depth (blank dimensions are ignored)
	MethodSteel   = "steel"   // nos x cutting length (m) x d^2/162 (kg/m)
	MethodOpening = "opening" // door/window opening deducted per IS 1200 rules
	MethodDirect  = "direct"  // quantity entered directly (enumerated items)
)

// MeasurementInput is one line of a measurement sheet.
type MeasurementInput struct {
	Method      string
	WorkItem    string
	Nos         float64
	Length      float64 // m
	Breadth     float64 // m
	Depth       float64 // m (height or thickness)
	DiaMM       float64 // steel only
	IsDeduction bool
}

// MeasurementResult is the computed quantity with a human-readable trail.
type MeasurementResult struct {
	Quantity float64 // signed: deductions are negative
	Formula  string
}

// SteelUnitWeight returns the weight of a reinforcement bar in kg per metre.
func SteelUnitWeight(diaMM float64) float64 { return diaMM * diaMM / 162.0 }

// ComputeMeasurement evaluates one measurement line.
func ComputeMeasurement(in MeasurementInput) (MeasurementResult, error) {
	nos := in.Nos
	if nos == 0 {
		nos = 1
	}
	if nos < 0 || in.Length < 0 || in.Breadth < 0 || in.Depth < 0 || in.DiaMM < 0 {
		return MeasurementResult{}, fmt.Errorf("dimensions must not be negative; use the deduction flag instead")
	}
	var qty float64
	var formula string
	switch in.Method {
	case MethodLBD, "":
		if in.Length == 0 && in.Breadth == 0 && in.Depth == 0 {
			return MeasurementResult{}, fmt.Errorf("at least one dimension is required")
		}
		qty = nos
		formula = fmtNum(nos)
		for _, d := range []float64{in.Length, in.Breadth, in.Depth} {
			if d != 0 {
				qty *= d
				formula += " x " + fmtNum(d)
			}
		}
	case MethodSteel:
		if in.DiaMM == 0 || in.Length == 0 {
			return MeasurementResult{}, fmt.Errorf("steel lines need bar diameter and cutting length")
		}
		w := SteelUnitWeight(in.DiaMM)
		qty = nos * in.Length * w
		formula = fmt.Sprintf("%s x %s m x (%s^2/162 = %.3f kg/m)", fmtNum(nos), fmtNum(in.Length), fmtNum(in.DiaMM), w)
	case MethodOpening:
		if in.Length == 0 || in.Depth == 0 {
			return MeasurementResult{}, fmt.Errorf("opening lines need width (length) and height (depth)")
		}
		r := OpeningDeduction(in.WorkItem, nos, in.Length, in.Depth, in.Breadth)
		return r, nil
	case MethodDirect:
		if in.Length == 0 {
			return MeasurementResult{}, fmt.Errorf("direct lines carry the quantity in the length field")
		}
		qty = nos * in.Length
		formula = fmt.Sprintf("%s x %s", fmtNum(nos), fmtNum(in.Length))
	default:
		return MeasurementResult{}, fmt.Errorf("unknown measurement method %q", in.Method)
	}
	if in.IsDeduction {
		qty = -qty
		formula = "DEDUCT " + formula
	}
	return MeasurementResult{Quantity: Round(qty, 3), Formula: formula}, nil
}

// OpeningDeduction applies the IS 1200 opening rules for one type of opening
// (nos openings of width x height in a wall of the given thickness, metres).
//
// Plaster (IS 1200 Part 12), per face measured:
//   - opening <= 0.5 m2: no deduction, no addition for jambs
//   - 0.5 m2 < opening <= 3 m2: deduct one face only; jambs not measured
//   - opening > 3 m2: deduct the opening and add jambs, soffit and sill
//
// A plaster line here represents ONE face of the wall. For the 0.5-3 m2 band
// the rule deducts one face in total, so a one-face line carries half of the
// opening area as its share.
//
// Masonry (IS 1200 Part 3): openings up to 0.1 m2 are not deducted; larger
// openings are deducted in full (area x wall thickness for volume items).
func OpeningDeduction(workItem string, nos, width, height, wallThickness float64) MeasurementResult {
	area := width * height
	switch workItem {
	case "PL-INT", "PL-EXT":
		switch {
		case area <= 0.5:
			return MeasurementResult{0, fmt.Sprintf("Opening %s x %s = %.2f m2 <= 0.5 m2: no deduction (IS 1200 Pt 12)", fmtNum(width), fmtNum(height), area)}
		case area <= 3:
			q := nos * area * 0.5
			return MeasurementResult{-Round(q, 3), fmt.Sprintf("DEDUCT %s x %s x %s x 0.5 (opening %.2f m2 in 0.5-3 m2 band: one face deducted across both faces, IS 1200 Pt 12)", fmtNum(nos), fmtNum(width), fmtNum(height), area)}
		default:
			jamb := (2*height + 2*width) * wallThickness / 2 // this face's half of the reveal
			q := nos * (area - jamb)
			return MeasurementResult{-Round(q, 3), fmt.Sprintf("DEDUCT %s x (%s x %s - jambs/soffit/sill %.2f m2) (opening %.2f m2 > 3 m2, IS 1200 Pt 12)", fmtNum(nos), fmtNum(width), fmtNum(height), jamb, area)}
		}
	case "BW-230":
		if area <= 0.1 {
			return MeasurementResult{0, fmt.Sprintf("Opening %.2f m2 <= 0.1 m2: no deduction (IS 1200 Pt 3)", area)}
		}
		t := wallThickness
		if t == 0 {
			t = 0.23
		}
		q := nos * area * t
		return MeasurementResult{-Round(q, 3), fmt.Sprintf("DEDUCT %s x %s x %s x %s thick (IS 1200 Pt 3)", fmtNum(nos), fmtNum(width), fmtNum(height), fmtNum(t))}
	default: // area items such as BW-115
		if area <= 0.1 {
			return MeasurementResult{0, fmt.Sprintf("Opening %.2f m2 <= 0.1 m2: no deduction", area)}
		}
		q := nos * area
		return MeasurementResult{-Round(q, 3), fmt.Sprintf("DEDUCT %s x %s x %s", fmtNum(nos), fmtNum(width), fmtNum(height))}
	}
}

// CentreLineLength returns the total centre-line length of the walls of a
// rectangular block of rooms measured to wall centre lines, less half a wall
// width for every T-junction (centre-line method).
func CentreLineLength(sumOfWallCentreLengths float64, tJunctions int, wallWidth float64) float64 {
	return Round(sumOfWallCentreLengths-float64(tJunctions)*wallWidth/2, 3)
}

func fmtNum(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", Round(v, 3))
}
