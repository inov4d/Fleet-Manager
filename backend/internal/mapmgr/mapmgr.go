// Package mapmgr validates maps and derives the topological graph and the
// spatial resources (nodes, edges, zones) used by the planner and the traffic
// manager.
package mapmgr

import (
	"fmt"
	"math"
	"strings"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/geometry"
)

const (
	DefaultEdgeWidthM  = 3.5
	DefaultEdgeSpeedMs = 1.0
)

// Validate checks referential integrity and fills defaults in place.
func Validate(m *core.Map) error {
	if strings.TrimSpace(m.Name) == "" {
		return core.Validation("map name is required")
	}
	if m.SiteID == "" {
		m.SiteID = core.DefaultSiteID
	}
	nodes := map[string]bool{}
	for i := range m.Nodes {
		n := &m.Nodes[i]
		if n.ID == "" {
			return core.Validation("node %d has no id", i)
		}
		if nodes[n.ID] {
			return core.Validation("duplicate node id %q", n.ID)
		}
		nodes[n.ID] = true
		if n.Type == "" {
			n.Type = core.NodeWaypoint
		}
		switch n.Type {
		case core.NodeWaypoint, core.NodeStation, core.NodePickup, core.NodeDropoff, core.NodeCharging, core.NodeParking, core.NodeWaiting:
		default:
			return core.Validation("node %q has invalid type %q", n.ID, n.Type)
		}
		if n.Capacity <= 0 {
			n.Capacity = 1
		}
		if n.Name == "" {
			n.Name = n.ID
		}
	}
	edges := map[string]bool{}
	for i := range m.Edges {
		e := &m.Edges[i]
		if e.ID == "" {
			return core.Validation("edge %d has no id", i)
		}
		if edges[e.ID] {
			return core.Validation("duplicate edge id %q", e.ID)
		}
		edges[e.ID] = true
		if !nodes[e.From] || !nodes[e.To] {
			return core.Validation("edge %q references unknown node (%q -> %q)", e.ID, e.From, e.To)
		}
		if e.From == e.To {
			return core.Validation("edge %q is a self loop", e.ID)
		}
		if e.MaxSpeedMs <= 0 {
			e.MaxSpeedMs = DefaultEdgeSpeedMs
		}
		if e.Motion == "" {
			e.Motion = core.MotionAny
		}
		switch e.Motion {
		case core.MotionAny, core.MotionForward, core.MotionLateral:
		default:
			return core.Validation("edge %q has invalid motion %q", e.ID, e.Motion)
		}
		if e.CostFactor <= 0 {
			e.CostFactor = 1
		}
		if e.WidthM <= 0 {
			e.WidthM = DefaultEdgeWidthM
		}
	}
	zones := map[string]bool{}
	for i := range m.Zones {
		z := &m.Zones[i]
		if z.ID == "" {
			return core.Validation("zone %d has no id", i)
		}
		if zones[z.ID] {
			return core.Validation("duplicate zone id %q", z.ID)
		}
		zones[z.ID] = true
		if len(z.Polygon) < 3 {
			return core.Validation("zone %q needs at least 3 points", z.ID)
		}
		if z.Type == "" {
			z.Type = core.ZoneExclusive
		}
		switch z.Type {
		case core.ZoneExclusive, core.ZoneSingleLane, core.ZoneBidirectional, core.ZoneCrossing, core.ZoneWaiting, core.ZoneForbidden, core.ZoneSpeedLimit, core.ZoneCharging:
		default:
			return core.Validation("zone %q has invalid type %q", z.ID, z.Type)
		}
		if z.Capacity <= 0 {
			z.Capacity = 1
		}
		if z.Name == "" {
			z.Name = z.ID
		}
	}
	if m.WidthM <= 0 || m.HeightM <= 0 {
		mn, mx := extent(m)
		m.OriginX, m.OriginY = mn.X-5, mn.Y-5
		m.WidthM, m.HeightM = (mx.X-mn.X)+10, (mx.Y-mn.Y)+10
	}
	return nil
}

func extent(m *core.Map) (core.Point, core.Point) {
	mn := core.Point{X: math.Inf(1), Y: math.Inf(1)}
	mx := core.Point{X: math.Inf(-1), Y: math.Inf(-1)}
	for _, n := range m.Nodes {
		mn.X, mn.Y = math.Min(mn.X, n.X), math.Min(mn.Y, n.Y)
		mx.X, mx.Y = math.Max(mx.X, n.X), math.Max(mx.Y, n.Y)
	}
	for _, z := range m.Zones {
		for _, p := range z.Polygon {
			mn.X, mn.Y = math.Min(mn.X, p.X), math.Min(mn.Y, p.Y)
			mx.X, mx.Y = math.Max(mx.X, p.X), math.Max(mx.Y, p.Y)
		}
	}
	if math.IsInf(mn.X, 1) {
		return core.Point{}, core.Point{X: 100, Y: 60}
	}
	return mn, mx
}

// Graph is the navigable view of a map.
type Graph struct {
	Map       *core.Map
	Nodes     map[string]*core.Node
	Edges     map[string]*core.Edge
	Zones     map[string]*core.Zone
	out       map[string][]*core.Edge // outgoing edges per node (direction aware)
	edgeZones map[string][]string     // edge id -> zone ids intersected
	nodeZones map[string][]string
}

// NewGraph indexes a validated map.
func NewGraph(m *core.Map) *Graph {
	g := &Graph{Map: m, Nodes: map[string]*core.Node{}, Edges: map[string]*core.Edge{}, Zones: map[string]*core.Zone{},
		out: map[string][]*core.Edge{}, edgeZones: map[string][]string{}, nodeZones: map[string][]string{}}
	for i := range m.Nodes {
		n := &m.Nodes[i]
		g.Nodes[n.ID] = n
	}
	for i := range m.Zones {
		z := &m.Zones[i]
		g.Zones[z.ID] = z
	}
	for i := range m.Edges {
		e := &m.Edges[i]
		g.Edges[e.ID] = e
		g.out[e.From] = append(g.out[e.From], e)
		if e.Bidirectional {
			g.out[e.To] = append(g.out[e.To], e)
		}
		a, b := g.Nodes[e.From], g.Nodes[e.To]
		for _, z := range m.Zones {
			if geometry.SegmentIntersectsPolygon(a.Point(), b.Point(), geometry.Polygon(z.Polygon)) {
				g.edgeZones[e.ID] = append(g.edgeZones[e.ID], z.ID)
			}
		}
	}
	for _, n := range m.Nodes {
		for _, z := range m.Zones {
			if geometry.ContainsPoint(geometry.Polygon(z.Polygon), n.Point()) {
				g.nodeZones[n.ID] = append(g.nodeZones[n.ID], z.ID)
			}
		}
	}
	return g
}

// Outgoing returns traversable edges leaving node id with their far node.
func (g *Graph) Outgoing(nodeID string) []Traversal {
	var out []Traversal
	for _, e := range g.out[nodeID] {
		to := e.To
		if to == nodeID {
			to = e.From
		}
		out = append(out, Traversal{Edge: e, From: nodeID, To: to})
	}
	return out
}

// Traversal is an edge crossed in a given direction.
type Traversal struct {
	Edge *core.Edge
	From string
	To   string
}

// EdgeBetween returns the edge from a to b honouring direction, or nil.
func (g *Graph) EdgeBetween(a, b string) *core.Edge {
	for _, e := range g.out[a] {
		if (e.From == a && e.To == b) || (e.Bidirectional && e.To == a && e.From == b) {
			return e
		}
	}
	return nil
}

// Length of an edge in metres.
func (g *Graph) Length(e *core.Edge) float64 {
	return g.Nodes[e.From].Point().Dist(g.Nodes[e.To].Point())
}

// Heading of travel from node a to node b.
func (g *Graph) Heading(a, b string) float64 {
	pa, pb := g.Nodes[a].Point(), g.Nodes[b].Point()
	return math.Atan2(pb.Y-pa.Y, pb.X-pa.X)
}

// ZonesForEdge returns zone ids intersected by the edge segment.
func (g *Graph) ZonesForEdge(edgeID string) []string { return g.edgeZones[edgeID] }

// ZonesForNode returns zone ids containing the node.
func (g *Graph) ZonesForNode(nodeID string) []string { return g.nodeZones[nodeID] }

// ZonesForPolygon returns zone ids intersecting a polygon (e.g. a swept envelope).
func (g *Graph) ZonesForPolygon(p geometry.Polygon) []string {
	var out []string
	for _, z := range g.Map.Zones {
		if geometry.Intersects(p, geometry.Polygon(z.Polygon)) {
			out = append(out, z.ID)
		}
	}
	return out
}

// Reservable reports whether a zone type participates in traffic reservations.
func Reservable(t core.ZoneType) bool {
	switch t {
	case core.ZoneExclusive, core.ZoneSingleLane, core.ZoneBidirectional, core.ZoneCrossing, core.ZoneWaiting, core.ZoneCharging:
		return true
	}
	return false
}

// ChargingNodes lists nodes of type charging.
func (g *Graph) ChargingNodes() []*core.Node {
	var out []*core.Node
	for i := range g.Map.Nodes {
		if g.Map.Nodes[i].Type == core.NodeCharging {
			out = append(out, &g.Map.Nodes[i])
		}
	}
	return out
}

// NearestNode returns the closest node to a point (any type) and its distance.
func (g *Graph) NearestNode(p core.Point) (*core.Node, float64) {
	var best *core.Node
	bd := math.Inf(1)
	for i := range g.Map.Nodes {
		d := g.Map.Nodes[i].Point().Dist(p)
		if d < bd {
			bd, best = d, &g.Map.Nodes[i]
		}
	}
	return best, bd
}

// DemoMap builds a realistic factory layout: 3 horizontal aisles x 5 vertical
// aisles, pickup/dropoff stations, a charging area, a single-lane corridor and
// crossing zones at intersections.
func DemoMap(siteID string) *core.Map { return DemoMapSized(siteID, 4, 6) }

// DemoMapSized builds the demo layout with the given number of aisles.
func DemoMapSized(siteID string, nRows, nCols int) *core.Map {
	m := &core.Map{ID: "map-demo", SiteID: siteID, Name: "Atelier démo", Version: 1, Active: true}
	cols := make([]float64, nCols)
	for i := range cols {
		cols[i] = 10 + 30*float64(i)
	}
	rows := make([]float64, nRows)
	for i := range rows {
		rows[i] = 10 + 25*float64(i)
	}
	id := func(r, c int) string { return fmt.Sprintf("N%d%d", r, c) }
	for r, y := range rows {
		for c, x := range cols {
			m.Nodes = append(m.Nodes, core.Node{ID: id(r, c), Name: id(r, c), X: x, Y: y, Type: core.NodeWaypoint, Capacity: 1})
		}
	}
	bidir := func(eid, a, b string, speed float64) {
		m.Edges = append(m.Edges, core.Edge{ID: eid, From: a, To: b, Bidirectional: true, MaxSpeedMs: speed, Motion: core.MotionAny, CostFactor: 1, WidthM: 4, Enabled: true})
	}
	oneway := func(eid, a, b string, speed float64) {
		m.Edges = append(m.Edges, core.Edge{ID: eid, From: a, To: b, Bidirectional: false, MaxSpeedMs: speed, Motion: core.MotionAny, CostFactor: 1, WidthM: 4, Enabled: true})
	}
	// horizontal aisles: row 0 and 2 bidirectional, row 1 is a one-way loop (left to right)
	for r := range rows {
		for c := 0; c < len(cols)-1; c++ {
			eid := fmt.Sprintf("H%d%d", r, c)
			if r == 1 {
				oneway(eid, id(r, c), id(r, c+1), 1.0)
			} else {
				bidir(eid, id(r, c), id(r, c+1), 1.0)
			}
		}
	}
	// vertical aisles
	for c := range cols {
		for r := 0; r < len(rows)-1; r++ {
			bidir(fmt.Sprintf("V%d%d", r, c), id(r, c), id(r+1, c), 0.8)
		}
	}
	// stations hanging off the top row (pickups) and bottom row (dropoffs)
	for c, x := range cols {
		pid := fmt.Sprintf("PICK%d", c+1)
		did := fmt.Sprintf("DROP%d", c+1)
		th := math.Pi / 2
		m.Nodes = append(m.Nodes,
			core.Node{ID: pid, Name: fmt.Sprintf("Poste prélèvement %d", c+1), X: x, Y: rows[len(rows)-1] + 12, Type: core.NodePickup, Theta: &th, Capacity: 1},
			core.Node{ID: did, Name: fmt.Sprintf("Poste dépose %d", c+1), X: x, Y: rows[0] - 12, Type: core.NodeDropoff, Theta: &th, Capacity: 1},
		)
		bidir("EP"+pid, id(len(rows)-1, c), pid, 0.5)
		bidir("ED"+did, id(0, c), did, 0.5)
	}
	// charging bay on the right
	for i := range rows {
		cid := fmt.Sprintf("CHG%d", i+1)
		pw := 50.0
		m.Nodes = append(m.Nodes, core.Node{ID: cid, Name: fmt.Sprintf("Chargeur %d", i+1), X: cols[len(cols)-1] + 30, Y: rows[i], Type: core.NodeCharging, ChargerPowerKw: &pw, Capacity: 1})
		bidir("EC"+cid, id(i, len(cols)-1), cid, 0.5)
	}
	// parking spots on the left
	for i := range rows {
		pid := fmt.Sprintf("PARK%d", i+1)
		m.Nodes = append(m.Nodes, core.Node{ID: pid, Name: fmt.Sprintf("Parking %d", i+1), X: -10, Y: rows[i], Type: core.NodeParking, Capacity: 1})
		bidir("EPK"+pid, id(i, 0), pid, 0.5)
	}
	// zones: crossings at the 15 intersections, a single lane corridor on the
	// middle vertical aisle, a forbidden area, a speed-limited pedestrian area
	for r, y := range rows {
		for c, x := range cols {
			m.Zones = append(m.Zones, core.Zone{ID: fmt.Sprintf("X%d%d", r, c), Name: fmt.Sprintf("Croisement %s", id(r, c)), Type: core.ZoneCrossing,
				Polygon: []core.Point{{X: x - 4, Y: y - 4}, {X: x + 4, Y: y - 4}, {X: x + 4, Y: y + 4}, {X: x - 4, Y: y + 4}}, Capacity: 1, Enabled: true})
		}
	}
	m.Zones = append(m.Zones,
		core.Zone{ID: "Z-SINGLE", Name: "Couloir étroit", Type: core.ZoneSingleLane, Capacity: 1, Enabled: true,
			Polygon: []core.Point{{X: 67, Y: 15}, {X: 73, Y: 15}, {X: 73, Y: 55}, {X: 67, Y: 55}}},
		core.Zone{ID: "Z-FORBID", Name: "Zone interdite (stockage)", Type: core.ZoneForbidden, Capacity: 1, Enabled: true,
			Polygon: []core.Point{{X: 110, Y: 40}, {X: 125, Y: 40}, {X: 125, Y: 55}, {X: 110, Y: 55}}},
		core.Zone{ID: "Z-SLOW", Name: "Zone piétons (vitesse réduite)", Type: core.ZoneSpeedLimit, Capacity: 10, Enabled: true, MaxSpeedMs: ptr(0.3),
			Polygon: []core.Point{{X: 0, Y: 25}, {X: 50, Y: 25}, {X: 50, Y: 45}, {X: 0, Y: 45}}},
		core.Zone{ID: "Z-CHARGE", Name: "Aire de charge", Type: core.ZoneCharging, Capacity: nRows, Enabled: true,
			Polygon: []core.Point{{X: cols[len(cols)-1] + 20, Y: 0}, {X: cols[len(cols)-1] + 40, Y: 0}, {X: cols[len(cols)-1] + 40, Y: rows[len(rows)-1] + 10}, {X: cols[len(cols)-1] + 20, Y: rows[len(rows)-1] + 10}}},
	)
	_ = Validate(m)
	return m
}

func ptr(f float64) *float64 { return &f }
