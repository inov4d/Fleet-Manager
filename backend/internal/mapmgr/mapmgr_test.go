package mapmgr

import (
	"testing"

	"github.com/inov4d/fleet-manager/internal/core"
)

func TestValidateDefaultsAndErrors(t *testing.T) {
	m := &core.Map{Name: "t", Nodes: []core.Node{{ID: "a", X: 0, Y: 0}, {ID: "b", X: 10, Y: 0}}, Edges: []core.Edge{{ID: "e", From: "a", To: "b"}}}
	if err := Validate(m); err != nil {
		t.Fatal(err)
	}
	if m.Edges[0].WidthM != DefaultEdgeWidthM || m.Edges[0].CostFactor != 1 || m.Nodes[0].Capacity != 1 {
		t.Fatalf("defaults not applied: %+v", m.Edges[0])
	}
	bad := &core.Map{Name: "t", Nodes: []core.Node{{ID: "a"}}, Edges: []core.Edge{{ID: "e", From: "a", To: "zz"}}}
	if err := Validate(bad); err == nil {
		t.Fatal("expected error for dangling edge")
	}
}

func TestGraphDirection(t *testing.T) {
	m := &core.Map{Name: "t", Nodes: []core.Node{{ID: "a"}, {ID: "b", X: 10}}, Edges: []core.Edge{{ID: "e", From: "a", To: "b", Bidirectional: false}}}
	_ = Validate(m)
	g := NewGraph(m)
	if g.EdgeBetween("a", "b") == nil || g.EdgeBetween("b", "a") != nil {
		t.Fatal("direction not honoured")
	}
	if len(g.Outgoing("b")) != 0 {
		t.Fatal("one way edge must not be traversable backwards")
	}
}

func TestDemoMap(t *testing.T) {
	m := DemoMap("s")
	if err := Validate(m); err != nil {
		t.Fatal(err)
	}
	g := NewGraph(m)
	if len(g.ChargingNodes()) != 4 {
		t.Fatalf("charging nodes %d", len(g.ChargingNodes()))
	}
	// the middle vertical aisle crosses the single lane zone
	if zs := g.ZonesForEdge("V02"); len(zs) == 0 {
		t.Fatal("V02 should intersect the single lane zone")
	}
	found := false
	for _, z := range g.ZonesForEdge("V02") {
		if z == "Z-SINGLE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Z-SINGLE missing from %v", g.ZonesForEdge("V02"))
	}
	if zs := g.ZonesForNode("N11"); len(zs) < 1 {
		t.Fatal("N11 should be in a crossing zone")
	}
}
