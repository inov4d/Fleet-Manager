package planner

import (
	"testing"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
)

func lineMap() *core.Map {
	m := &core.Map{Name: "line", Nodes: []core.Node{{ID: "a", X: 0}, {ID: "b", X: 10}, {ID: "c", X: 20}, {ID: "d", X: 10, Y: 10}},
		Edges: []core.Edge{
			{ID: "ab", From: "a", To: "b", Bidirectional: true, Enabled: true},
			{ID: "bc", From: "b", To: "c", Bidirectional: true, Enabled: true},
			{ID: "ad", From: "a", To: "d", Bidirectional: true, Enabled: true},
			{ID: "dc", From: "d", To: "c", Bidirectional: true, Enabled: true},
		}}
	_ = mapmgr.Validate(m)
	return m
}

func TestShortestPath(t *testing.T) {
	g := mapmgr.NewGraph(lineMap())
	r, err := AStar{}.Plan(Request{Graph: g, From: "a", To: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.NodeIDs) != 3 || r.NodeIDs[1] != "b" {
		t.Fatalf("expected a-b-c, got %v", r.NodeIDs)
	}
	if r.LengthM != 20 {
		t.Fatalf("length %v", r.LengthM)
	}
}

func TestDisabledEdgeAndCongestion(t *testing.T) {
	m := lineMap()
	m.Edges[0].Enabled = false
	g := mapmgr.NewGraph(m)
	r, err := AStar{}.Plan(Request{Graph: g, From: "a", To: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if r.NodeIDs[1] != "d" {
		t.Fatalf("expected detour via d, got %v", r.NodeIDs)
	}
	// congestion on the straight path pushes the route onto the detour
	g2 := mapmgr.NewGraph(lineMap())
	r2, _ := AStar{}.Plan(Request{Graph: g2, From: "a", To: "c", Congestion: func(e string) float64 {
		if e == "bc" {
			return 100
		}
		return 0
	}})
	if r2.NodeIDs[1] != "d" {
		t.Fatalf("congestion ignored: %v", r2.NodeIDs)
	}
}

func TestOneWayAndNoRoute(t *testing.T) {
	m := &core.Map{Name: "ow", Nodes: []core.Node{{ID: "a"}, {ID: "b", X: 5}}, Edges: []core.Edge{{ID: "ab", From: "a", To: "b", Enabled: true}}}
	_ = mapmgr.Validate(m)
	g := mapmgr.NewGraph(m)
	if _, err := (AStar{}).Plan(Request{Graph: g, From: "b", To: "a"}); err == nil {
		t.Fatal("expected no route against a one-way edge")
	}
}

func TestForbiddenZoneAndWidth(t *testing.T) {
	m := lineMap()
	m.Zones = []core.Zone{{ID: "f", Type: core.ZoneForbidden, Enabled: true, Polygon: []core.Point{{X: 14, Y: -1}, {X: 16, Y: -1}, {X: 16, Y: 1}, {X: 14, Y: 1}}}}
	_ = mapmgr.Validate(m)
	g := mapmgr.NewGraph(m)
	r, err := AStar{}.Plan(Request{Graph: g, From: "a", To: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if r.NodeIDs[1] != "d" {
		t.Fatalf("forbidden zone ignored: %v", r.NodeIDs)
	}
	// a 40T vehicle (3 m wide, 8 m long) does not fit in a 2.5 m corridor
	m2 := lineMap()
	m2.Edges[1].WidthM = 2.5
	g2 := mapmgr.NewGraph(m2)
	r2, err := AStar{}.Plan(Request{Graph: g2, From: "a", To: "c", Envelope: core.Footprint{LengthM: 8, WidthM: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if r2.NodeIDs[1] != "d" {
		t.Fatalf("width constraint ignored: %v", r2.NodeIDs)
	}
}

func TestDemoMapRoutes(t *testing.T) {
	g := mapmgr.NewGraph(mapmgr.DemoMap("s"))
	r, err := AStar{}.Plan(Request{Graph: g, From: "PICK1", To: "DROP5", MassKg: 20000})
	if err != nil {
		t.Fatal(err)
	}
	if r.LengthM <= 0 || r.EstimatedTimeS <= 0 {
		t.Fatalf("bad metrics %+v", r)
	}
	// multi stop
	mr, err := PlanThrough(AStar{}, Request{Graph: g}, []string{"PARK1", "PICK2", "DROP4", "CHG1"})
	if err != nil {
		t.Fatal(err)
	}
	if mr.NodeIDs[0] != "PARK1" || mr.NodeIDs[len(mr.NodeIDs)-1] != "CHG1" {
		t.Fatalf("bad multi-stop route %v", mr.NodeIDs)
	}
}
