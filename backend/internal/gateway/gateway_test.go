package gateway

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/transport"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

func stateJSON(headerID int, serial string, x float64) []byte {
	s := vda5050.State{Header: vda5050.Header{HeaderID: headerID, Timestamp: time.Now(), Version: vda5050.Version, Manufacturer: "inov4d", SerialNumber: serial},
		OperatingMode: vda5050.OpAutomatic, NodeStates: []vda5050.NodeState{}, EdgeStates: []vda5050.EdgeState{}, ActionStates: []vda5050.ActionState{},
		BatteryState: vda5050.BatteryState{BatteryCharge: 50}, Errors: []vda5050.Error{}, SafetyState: vda5050.SafetyState{EStop: vda5050.EStopNone},
		AGVPosition: &vda5050.AGVPosition{X: x, MapID: "m", PositionInitialized: true}}
	b, _ := json.Marshal(s)
	return b
}

func TestGatewayStateFlowAndTimeouts(t *testing.T) {
	bus := transport.NewMemory()
	clk := core.NewManualClock(time.Now())
	gw := New(bus, "uagv", clk, nil, 5*time.Second)
	var states []string
	var conns []core.Connection
	if err := gw.Start(func(v string, s *vda5050.State) { states = append(states, v) }, func(v string, c core.Connection) { conns = append(conns, c) }); err != nil {
		t.Fatal(err)
	}
	gw.Register("agv-1", "inov4d", "S1")
	topic := vda5050.Topic("uagv", "inov4d", "S1", vda5050.TopicState)
	_ = bus.Publish(topic, stateJSON(1, "S1", 1), false)
	_ = bus.Publish(topic, stateJSON(1, "S1", 1), false) // duplicate
	_ = bus.Publish(topic, []byte("{not json"), false)   // invalid
	_ = bus.Publish(vda5050.Topic("uagv", "inov4d", "UNKNOWN", vda5050.TopicState), stateJSON(1, "UNKNOWN", 1), false)
	if len(states) != 1 {
		t.Fatalf("expected 1 state delivery, got %d", len(states))
	}
	st := gw.Stats()
	if st.Duplicates != 1 || st.Invalid != 1 {
		t.Fatalf("stats %+v", st)
	}
	if d := gw.Discovered(); len(d) != 1 || d[0] != "inov4d/UNKNOWN" {
		t.Fatalf("discovered %v", d)
	}
	if len(conns) != 1 || conns[0] != core.ConnOnline {
		t.Fatalf("conns %v", conns)
	}
	clk.Advance(6 * time.Second)
	lost := gw.CheckTimeouts()
	if len(lost) != 1 || lost[0] != "agv-1" {
		t.Fatalf("lost %v", lost)
	}
	if c, _ := gw.Connection("agv-1"); c != core.ConnBroken {
		t.Fatal("expected connectionBroken")
	}
	// connection topic restores
	cm := vda5050.Connection{Header: vda5050.Header{HeaderID: 1, Version: vda5050.Version, Manufacturer: "inov4d", SerialNumber: "S1"}, ConnectionState: vda5050.ConnOnline}
	b, _ := json.Marshal(cm)
	_ = bus.Publish(vda5050.Topic("uagv", "inov4d", "S1", vda5050.TopicConnection), b, true)
	if c, _ := gw.Connection("agv-1"); c != core.ConnOnline {
		t.Fatal("expected online")
	}
}

func TestOrderBuildAndSend(t *testing.T) {
	bus := transport.NewMemory()
	gw := New(bus, "uagv", core.RealClock{}, nil, time.Second)
	_ = gw.Start(nil, nil)
	gw.Register("agv-1", "inov4d", "S1")
	var received []vda5050.Order
	_, _ = bus.Subscribe("uagv/v2/inov4d/S1/order", func(topic string, p []byte) {
		var o vda5050.Order
		if err := json.Unmarshal(p, &o); err != nil {
			t.Fatal(err)
		}
		received = append(received, o)
	})
	g := mapmgr.NewGraph(mapmgr.DemoMap("s"))
	route := []string{"PARK4", "N30", "N31", "N32", "PICK3"}
	act := vda5050.Action{ActionType: vda5050.ActionPick, ActionID: "a1", BlockingType: vda5050.BlockingHard}
	o := BuildOrder(g, "map-demo", "ord-1", 0, route, 0, 2, []RouteAction{{Index: 4, Action: act}}, 1.0, 0)
	if err := gw.SendOrder("agv-1", o); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 {
		t.Fatal("order not delivered")
	}
	ro := received[0]
	if ro.HeaderID != 1 || ro.Version != vda5050.Version || len(ro.Nodes) != 5 || len(ro.Edges) != 4 {
		t.Fatalf("bad order %+v", ro.Header)
	}
	if !ro.Nodes[2].Released || ro.Nodes[3].Released || !ro.Edges[1].Released || ro.Edges[2].Released {
		t.Fatal("base/horizon split wrong")
	}
	if len(ro.Nodes[4].Actions) != 1 || ro.Nodes[4].Actions[0].ActionType != "pick" {
		t.Fatal("action not attached")
	}
	// order update stitched at the last released node
	u := BuildOrder(g, "map-demo", "ord-1", 1, route, 2, 4, nil, 1.0, 0)
	if u.Nodes[0].NodeID != "N31" || u.Nodes[0].SequenceID != 4 {
		t.Fatalf("update must start at last base node with same sequenceId: %+v", u.Nodes[0])
	}
	if err := gw.SendOrder("agv-1", u); err != nil {
		t.Fatal(err)
	}
	if received[1].HeaderID != 2 {
		t.Fatal("header sequence")
	}
	if err := gw.Pause("agv-1"); err != nil {
		t.Fatal(err)
	}
}
