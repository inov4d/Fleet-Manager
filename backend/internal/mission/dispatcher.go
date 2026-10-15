package mission

import (
	"fmt"
	"math"
	"sort"

	"github.com/inov4d/fleet-manager/internal/core"
)

// CandidateCtx is what the dispatcher knows about a vehicle.
type CandidateCtx struct {
	VehicleID    string
	SiteID       string
	ModelID      string
	Available    bool   // idle, online, automatic, no fatal error
	Unavailable  string // reason when not available
	BatteryPct   float64
	CapacityKg   float64
	BodyLengthM  float64
	BodyWidthM   float64
	MaxOverhangM float64 // allowed load overhang on each side
	QueueLen     int     // missions already queued/assigned
	CurrentNode  string
	// Route estimates the cost of serving a mission from the vehicle's position
	// (approach to origin + mission path). ok=false means unreachable.
	Route func(m core.Mission) (distanceM, timeS, congestion float64, route []string, ok bool)
	// EnergyPct estimates the battery percentage needed for a distance.
	EnergyPct func(distanceM float64, massKg float64) float64
}

// Decision is the outcome for one mission.
type Decision struct {
	MissionID   string
	VehicleID   string
	Route       []string
	Explanation core.AssignmentExplanation
}

// Dispatch assigns pending missions (already ordered by priority) to vehicles.
// Each vehicle gets at most one mission per round. The scoring is a weighted
// sum of normalised factors (lower is better) and every candidate's reasoning
// is kept for the operator.
func Dispatch(pending []core.Mission, cands []CandidateCtx, s core.DispatcherSettings) []Decision {
	taken := map[string]bool{}
	var out []Decision
	w := s.Weights
	wd, wt, wb, wc, wq := w["distance"], w["travelTime"], w["battery"], w["congestion"], w["queue"]
	for _, m := range pending {
		expl := core.AssignmentExplanation{Weights: map[string]float64{"distance": wd, "travelTime": wt, "battery": wb, "congestion": wc, "queue": wq}}
		best := -1
		bestScore := math.Inf(1)
		var bestRoute []string
		for _, c := range cands {
			cand := core.Candidate{VehicleID: c.VehicleID}
			reason, route, ok := eligible(m, c, s, taken)
			if !ok {
				cand.Reason = reason
				expl.Candidates = append(expl.Candidates, cand)
				continue
			}
			d, t, cg, _, _ := c.Route(m) // already computed in eligible; cheap enough to recompute
			f := core.CandidateFactors{DistanceM: d, TravelTimeS: t, Battery: c.BatteryPct, Congestion: cg, Queue: float64(c.QueueLen), Priority: float64(m.Priority)}
			score := wd*d/100 + wt*t/60 + wb*(100-c.BatteryPct)/100 + wc*cg/100 + wq*float64(c.QueueLen)
			cand.Eligible, cand.Score, cand.Factors = true, round(score), f
			cand.Reason = fmt.Sprintf("score %.2f (dist %.0f m, %.0f s, bat %.0f%%, cong %.0f)", score, d, t, c.BatteryPct, cg)
			expl.Candidates = append(expl.Candidates, cand)
			if score < bestScore {
				bestScore, best, bestRoute = score, len(expl.Candidates)-1, route
			}
		}
		sort.SliceStable(expl.Candidates, func(i, j int) bool {
			if expl.Candidates[i].Eligible != expl.Candidates[j].Eligible {
				return expl.Candidates[i].Eligible
			}
			return expl.Candidates[i].Score < expl.Candidates[j].Score
		})
		if best < 0 {
			continue
		}
		chosen := expl.Candidates[0].VehicleID
		expl.ChosenVehicleID = chosen
		taken[chosen] = true
		out = append(out, Decision{MissionID: m.ID, VehicleID: chosen, Route: bestRoute, Explanation: expl})
	}
	return out
}

func round(v float64) float64 { return math.Round(v*100) / 100 }

func eligible(m core.Mission, c CandidateCtx, s core.DispatcherSettings, taken map[string]bool) (string, []string, bool) {
	if taken[c.VehicleID] {
		return "already assigned in this round", nil, false
	}
	if c.SiteID != m.SiteID {
		return "different site", nil, false
	}
	if !c.Available {
		return c.Unavailable, nil, false
	}
	if m.RequiredModelID != "" && m.RequiredModelID != c.ModelID {
		return "model " + c.ModelID + " does not match required " + m.RequiredModelID, nil, false
	}
	if m.Load.MassKg > c.CapacityKg {
		return fmt.Sprintf("payload %.0f kg exceeds capacity %.0f kg", m.Load.MassKg, c.CapacityKg), nil, false
	}
	if m.Load.Present && c.BodyLengthM > 0 && (m.Load.LengthM > c.BodyLengthM+2*c.MaxOverhangM || m.Load.WidthM > c.BodyWidthM+2*c.MaxOverhangM) {
		return "load dimensions incompatible with vehicle geometry", nil, false
	}
	if c.Route == nil {
		return "no route estimator", nil, false
	}
	d, _, _, route, ok := c.Route(m)
	if !ok {
		return "no feasible route", nil, false
	}
	need := s.ReserveBatteryPct
	if c.EnergyPct != nil {
		need += c.EnergyPct(d, m.Load.MassKg)
	}
	if c.BatteryPct < s.MinBatteryPct || c.BatteryPct < need {
		return fmt.Sprintf("battery %.0f%% below required %.0f%%", c.BatteryPct, math.Max(s.MinBatteryPct, need)), nil, false
	}
	return "", route, true
}
