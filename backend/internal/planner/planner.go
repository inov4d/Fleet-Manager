// Package planner computes topological routes with weighted costs (distance,
// congestion, energy, manoeuvres), honouring forbidden zones, disabled edges,
// edge direction and vehicle geometry. It exposes an interface so cooperative
// or multi-agent planners (CA*, MAPF) can be plugged in later.
package planner

import (
	"container/heap"
	"math"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
)

// Request describes a planning problem.
type Request struct {
	Graph      *mapmgr.Graph
	From, To   string
	VehicleID  string
	Envelope   core.Footprint // effective envelope (body + load + margins)
	MassKg     float64        // vehicle + load mass used for the energy cost
	MaxSpeedMs float64        // vehicle capability (0 = unlimited)
	AvoidZones []string       // mission constraint
	AvoidEdges []string       // e.g. edges held by a blocked vehicle
	AvoidNodes []string       // e.g. nodes occupied by vehicles of a deadlock cycle
	Heading    float64        // current heading (for the first manoeuvre cost)
	HasHeading bool
	Weights    Weights
	// Congestion returns an extra cost in metre-equivalents for an edge (0 = free).
	Congestion func(edgeID string) float64
	// DirectedCost returns an extra cost for traversing an edge from node a to
	// node b (used for head-on avoidance against other vehicles' intents).
	DirectedCost func(edgeID, from, to string) float64
}

// Weights tune the cost components (metre equivalents).
type Weights struct {
	Distance        float64 // multiplier on physical length * edge cost factor
	Congestion      float64 // multiplier on the congestion callback
	EnergyPerTonM   float64 // metre-equivalents per tonne per metre
	ManoeuvrePerRad float64 // metre-equivalents per radian of heading change
	LateralPenalty  float64 // multiplier on lateral-only edges
	TimeWeight      float64 // metre-equivalents per second of travel time
}

func DefaultWeights() Weights {
	return Weights{Distance: 1, Congestion: 1, EnergyPerTonM: 0.02, ManoeuvrePerRad: 2.0, LateralPenalty: 1.3, TimeWeight: 0.5}
}

// Route is a planned path.
type Route struct {
	NodeIDs        []string           `json:"nodeIds"`
	EdgeIDs        []string           `json:"edgeIds"`
	LengthM        float64            `json:"lengthM"`
	EstimatedTimeS float64            `json:"estimatedTimeS"`
	Costs          map[string]float64 `json:"costs"`
}

// Planner is the pluggable planning interface.
type Planner interface {
	Plan(req Request) (*Route, error)
}

// AStar is the default single-agent planner.
type AStar struct{}

type key struct {
	node string
	prev string // previous edge id ("" at start)
}

type item struct {
	k     key
	f, g  float64
	index int
}

type pq []*item

func (p pq) Len() int           { return len(p) }
func (p pq) Less(i, j int) bool { return p[i].f < p[j].f }
func (p pq) Swap(i, j int)      { p[i], p[j] = p[j], p[i]; p[i].index = i; p[j].index = j }
func (p *pq) Push(x any)        { it := x.(*item); it.index = len(*p); *p = append(*p, it) }
func (p *pq) Pop() any          { old := *p; n := len(old); it := old[n-1]; *p = old[:n-1]; return it }

type parent struct {
	k    key
	edge string
}

// Plan runs A* over (node, incoming edge) states so manoeuvre costs are exact.
func (AStar) Plan(req Request) (*Route, error) {
	g := req.Graph
	if g == nil {
		return nil, core.Validation("no graph")
	}
	if _, ok := g.Nodes[req.From]; !ok {
		return nil, core.NotFound("node", req.From)
	}
	if _, ok := g.Nodes[req.To]; !ok {
		return nil, core.NotFound("node", req.To)
	}
	w := req.Weights
	if w == (Weights{}) {
		w = DefaultWeights()
	}
	avoidZ := map[string]bool{}
	for _, z := range req.AvoidZones {
		avoidZ[z] = true
	}
	avoidE := map[string]bool{}
	for _, e := range req.AvoidEdges {
		avoidE[e] = true
	}
	avoidN := map[string]bool{}
	for _, n := range req.AvoidNodes {
		if n != req.From && n != req.To {
			avoidN[n] = true
		}
	}
	goal := g.Nodes[req.To].Point()
	h := func(n string) float64 { return g.Nodes[n].Point().Dist(goal) * w.Distance }

	start := key{node: req.From}
	gScore := map[key]float64{start: 0}
	parents := map[key]parent{}
	open := &pq{}
	heap.Init(open)
	heap.Push(open, &item{k: start, f: h(req.From), g: 0})
	closed := map[key]bool{}
	var found *key

	for open.Len() > 0 {
		cur := heap.Pop(open).(*item)
		if closed[cur.k] {
			continue
		}
		closed[cur.k] = true
		if cur.k.node == req.To {
			k := cur.k
			found = &k
			break
		}
		for _, tr := range g.Outgoing(cur.k.node) {
			e := tr.Edge
			if !e.Enabled || avoidE[e.ID] || avoidN[tr.To] {
				continue
			}
			if !fits(req.Envelope, e) {
				continue
			}
			if blocked(g, e.ID, avoidZ) {
				continue
			}
			step := edgeCost(req, w, cur.k, tr)
			nk := key{node: tr.To, prev: e.ID}
			ng := cur.g + step
			if old, ok := gScore[nk]; ok && ng >= old {
				continue
			}
			gScore[nk] = ng
			parents[nk] = parent{k: cur.k, edge: e.ID}
			heap.Push(open, &item{k: nk, g: ng, f: ng + h(tr.To)})
		}
	}
	if found == nil {
		return nil, core.Conflict("no route from %s to %s", req.From, req.To)
	}
	// reconstruct
	var nodes, edges []string
	k := *found
	for {
		nodes = append([]string{k.node}, nodes...)
		p, ok := parents[k]
		if !ok {
			break
		}
		edges = append([]string{p.edge}, edges...)
		k = p.k
	}
	r := &Route{NodeIDs: nodes, EdgeIDs: edges, Costs: map[string]float64{}}
	r.Costs["total"] = gScore[*found]
	fillMetrics(req, r)
	return r, nil
}

// fits checks that the corridor width accommodates the vehicle for the motion
// allowed on the edge. An omnidirectional AGV may travel sideways, so the
// relevant dimension is the one perpendicular to travel.
func fits(env core.Footprint, e *core.Edge) bool {
	if env.LengthM == 0 && env.WidthM == 0 {
		return true
	}
	switch e.Motion {
	case core.MotionForward:
		return env.WidthM <= e.WidthM
	case core.MotionLateral:
		return env.LengthM <= e.WidthM
	default:
		return math.Min(env.WidthM, env.LengthM) <= e.WidthM
	}
}

func blocked(g *mapmgr.Graph, edgeID string, avoid map[string]bool) bool {
	for _, zid := range g.ZonesForEdge(edgeID) {
		z := g.Zones[zid]
		if avoid[zid] {
			return true
		}
		if z.Type == core.ZoneForbidden && z.Enabled {
			return true
		}
		if !z.Enabled && z.Type != core.ZoneForbidden && z.Type != core.ZoneSpeedLimit {
			// a disabled reservable zone is temporarily unavailable
			return true
		}
	}
	return false
}

func edgeCost(req Request, w Weights, from key, tr mapmgr.Traversal) float64 {
	g := req.Graph
	e := tr.Edge
	length := g.Length(e)
	cost := length * e.CostFactor * w.Distance
	// lateral-only edges are slower and less efficient
	if e.Motion == core.MotionLateral {
		cost *= w.LateralPenalty
	}
	// travel time
	speed := e.MaxSpeedMs
	if req.MaxSpeedMs > 0 && req.MaxSpeedMs < speed {
		speed = req.MaxSpeedMs
	}
	for _, zid := range g.ZonesForEdge(e.ID) {
		if z := g.Zones[zid]; z.MaxSpeedMs != nil && *z.MaxSpeedMs > 0 && *z.MaxSpeedMs < speed {
			speed = *z.MaxSpeedMs
		}
	}
	if speed > 0 {
		cost += length / speed * w.TimeWeight
	}
	// energy: heavier loads cost more
	cost += req.MassKg / 1000 * length * w.EnergyPerTonM
	// congestion
	if req.Congestion != nil {
		cost += req.Congestion(e.ID) * w.Congestion
	}
	if req.DirectedCost != nil {
		cost += req.DirectedCost(e.ID, tr.From, tr.To) * w.Congestion
	}
	// manoeuvre: heading change relative to the previous edge (or current heading)
	newHeading := g.Heading(tr.From, tr.To)
	var prevHeading float64
	have := false
	if from.prev != "" {
		pe := g.Edges[from.prev]
		a, b := pe.From, pe.To
		if b != tr.From { // traversed backwards on a bidirectional edge
			a, b = b, a
		}
		prevHeading = g.Heading(a, b)
		have = true
	} else if req.HasHeading {
		prevHeading, have = req.Heading, true
	}
	if have && e.Motion != core.MotionLateral {
		d := math.Abs(math.Atan2(math.Sin(newHeading-prevHeading), math.Cos(newHeading-prevHeading)))
		cost += d * w.ManoeuvrePerRad
	}
	return cost
}

func fillMetrics(req Request, r *Route) {
	g := req.Graph
	var length, timeS, energyKwh float64
	for i, eid := range r.EdgeIDs {
		e := g.Edges[eid]
		l := g.Length(e)
		length += l
		speed := e.MaxSpeedMs
		if req.MaxSpeedMs > 0 && req.MaxSpeedMs < speed {
			speed = req.MaxSpeedMs
		}
		for _, zid := range g.ZonesForEdge(eid) {
			if z := g.Zones[zid]; z.MaxSpeedMs != nil && *z.MaxSpeedMs > 0 && *z.MaxSpeedMs < speed {
				speed = *z.MaxSpeedMs
			}
		}
		timeS += l / speed
		if i > 0 {
			// pause for heading changes (approx. 2 s per 90 degrees)
			a := g.Heading(r.NodeIDs[i-1], r.NodeIDs[i])
			b := g.Heading(r.NodeIDs[i], r.NodeIDs[i+1])
			d := math.Abs(math.Atan2(math.Sin(b-a), math.Cos(b-a)))
			timeS += d / (math.Pi / 2) * 2
		}
		energyKwh += l * (0.0005 + req.MassKg/1000*0.00002)
	}
	r.LengthM = length
	r.EstimatedTimeS = timeS
	r.Costs["distanceM"] = length
	r.Costs["timeS"] = timeS
	r.Costs["energyKwh"] = energyKwh
	if req.Congestion != nil {
		c := 0.0
		for _, eid := range r.EdgeIDs {
			c += req.Congestion(eid)
		}
		r.Costs["congestion"] = c
	}
}

// PlanThrough chains routes through an ordered list of nodes.
func PlanThrough(p Planner, req Request, nodes []string) (*Route, error) {
	if len(nodes) < 2 {
		return nil, core.Validation("need at least two nodes")
	}
	total := &Route{Costs: map[string]float64{}}
	for i := 0; i+1 < len(nodes); i++ {
		sub := req
		sub.From, sub.To = nodes[i], nodes[i+1]
		if i > 0 {
			sub.HasHeading = false
		}
		r, err := p.Plan(sub)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			total.NodeIDs = append(total.NodeIDs, r.NodeIDs...)
		} else {
			total.NodeIDs = append(total.NodeIDs, r.NodeIDs[1:]...)
		}
		total.EdgeIDs = append(total.EdgeIDs, r.EdgeIDs...)
		total.LengthM += r.LengthM
		total.EstimatedTimeS += r.EstimatedTimeS
		for k, v := range r.Costs {
			total.Costs[k] += v
		}
	}
	return total, nil
}
