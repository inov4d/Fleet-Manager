package mapmgr

import (
	"math"

	"github.com/inov4d/fleet-manager/internal/core"
)

// TravelHeading decides the vehicle heading used to traverse an edge. An
// omnidirectional AGV may drive forward, backward or sideways: for
// MotionLateral the body keeps its heading and translates; for MotionForward it
// aligns with the edge (forward or reverse); for MotionAny the orientation
// requiring the smallest rotation is picked among the four body alignments.
func TravelHeading(current, edgeHeading float64, motion core.Motion) float64 {
	norm := func(a float64) float64 { return math.Atan2(math.Sin(a), math.Cos(a)) }
	switch motion {
	case core.MotionLateral:
		return current
	case core.MotionForward:
		cands := []float64{edgeHeading, edgeHeading + math.Pi}
		return closest(current, cands, norm)
	default:
		cands := []float64{edgeHeading, edgeHeading + math.Pi, edgeHeading + math.Pi/2, edgeHeading - math.Pi/2}
		return closest(current, cands, norm)
	}
}

func closest(current float64, cands []float64, norm func(float64) float64) float64 {
	best := cands[0]
	bd := math.Inf(1)
	for _, c := range cands {
		d := math.Abs(norm(c - current))
		if d < bd-1e-9 {
			bd, best = d, c
		}
	}
	return norm(best)
}
