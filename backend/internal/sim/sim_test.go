package sim

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/gateway"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/transport"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

func TestAgentFollowsOrderOverVDA5050(t *testing.T) {
	bus := transport.NewMemory()
	clk := core.NewScaledClock(1)
	g := mapmgr.NewGraph(mapmgr.DemoMap("s"))
	model := core.BuiltinModels()[0]
	start := g.Nodes["PARK4"]
	a := NewAgent(AgentConfig{VehicleID: "v1", Manufacturer: "inov4d", Serial: "S1", MapID: "map-demo", Model: model,
		Pose: core.Pose{X: start.X, Y: start.Y}, BatteryPct: 80, LastNodeID: "PARK4"}, bus, clk)
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	gw := gateway.New(bus, "uagv", clk, nil, 5*time.Second)
	var last *vda5050.State
	_ = gw.Start(func(v string, s *vda5050.State) { last = s }, nil)
	gw.Register("v1", "inov4d", "S1")

	route := []string{"PARK4", "N30", "N31", "PICK2"}
	pick := vda5050.Action{ActionType: vda5050.ActionPick, ActionID: "m1-pick", BlockingType: vda5050.BlockingHard, ActionParameters: []vda5050.ActionParameter{{Key: "weight", Value: 15000.0}}}
	// base released up to N31, PICK2 in the horizon
	o := gateway.BuildOrder(g, "map-demo", "ord-1", 0, route, 0, 2, []gateway.RouteAction{{Index: 3, Action: pick}}, 0, 0)
	if err := gw.SendOrder("v1", o); err != nil {
		t.Fatal(err)
	}
	// step until N31 reached (max 600 s sim)
	reached := false
	for i := 0; i < 6000; i++ {
		a.Step(0.1)
		if a.Snapshot().LastNodeID == "N31" && !a.Snapshot().Driving {
			reached = true
			break
		}
	}
	if !reached {
		t.Fatalf("did not reach N31: %+v", a.Snapshot())
	}
	if last == nil || last.LastNodeID != "N31" || last.NewBaseRequest == nil || !*last.NewBaseRequest {
		t.Fatalf("state should report N31 and newBaseRequest: %+v", last)
	}
	if len(last.NodeStates) != 1 || last.NodeStates[0].NodeID != "PICK2" || last.NodeStates[0].Released {
		t.Fatalf("horizon node state wrong: %+v", last.NodeStates)
	}
	// release the horizon with an order update stitched at N31
	u := gateway.BuildOrder(g, "map-demo", "ord-1", 1, route, 2, 3, []gateway.RouteAction{{Index: 3, Action: pick}}, 0, 0)
	if err := gw.SendOrder("v1", u); err != nil {
		t.Fatal(err)
	}
	done := false
	for i := 0; i < 6000; i++ {
		a.Step(0.1)
		s := a.Snapshot()
		if s.LastNodeID == "PICK2" && s.Loads == 1 && !s.Driving {
			done = true
			break
		}
	}
	if !done {
		t.Fatalf("pick not completed: %+v", a.Snapshot())
	}
	if last.ActionStates[0].ActionStatus != vda5050.StatusFinished {
		t.Fatalf("action state %+v", last.ActionStates)
	}
	snap := a.Snapshot()
	if snap.Battery >= 80 || snap.Odometer < 50 {
		t.Fatalf("battery/odometer not updated: %+v", snap)
	}
	// cancel order and pause / resume paths
	if err := gw.CancelOrder("v1"); err != nil {
		t.Fatal(err)
	}
	a.Step(0.1)
	if a.Snapshot().OrderID != "" {
		t.Fatal("order should be cancelled")
	}
	// comms loss: no state published
	a.SetFault(FaultCommsLoss)
	before := gw.Stats().Received
	for i := 0; i < 30; i++ {
		a.Step(0.1)
	}
	if gw.Stats().Received != before {
		t.Fatal("agent must be silent during comms loss")
	}
	a.SetFault(FaultRestore)
	a.Step(0.1)
	if gw.Stats().Received == before {
		t.Fatal("agent should publish again after restore")
	}
}

func TestOrderValidationErrorReported(t *testing.T) {
	bus := transport.NewMemory()
	clk := core.NewScaledClock(1)
	a := NewAgent(AgentConfig{VehicleID: "v1", Manufacturer: "m", Serial: "s", Model: core.BuiltinModels()[0]}, bus, clk)
	_ = a.Start()
	bad := vda5050.Order{Header: vda5050.Header{HeaderID: 1, Version: "2.0.0", Manufacturer: "m", SerialNumber: "s"}, OrderID: "x",
		Nodes: []vda5050.Node{{NodeID: "a", SequenceID: 0, Released: true}, {NodeID: "b", SequenceID: 3, Released: true}}, Edges: []vda5050.Edge{{EdgeID: "e", SequenceID: 1, Released: true, StartNodeID: "a", EndNodeID: "b"}}}
	b, _ := json.Marshal(bad)
	var states []vda5050.State
	_, _ = bus.Subscribe("uagv/v2/m/s/state", func(_ string, p []byte) {
		var s vda5050.State
		_ = json.Unmarshal(p, &s)
		states = append(states, s)
	})
	_ = bus.Publish("uagv/v2/m/s/order", b, false)
	a.Step(0.1)
	if len(states) == 0 || len(states[len(states)-1].Errors) == 0 || states[len(states)-1].Errors[0].ErrorType != "validationError" {
		t.Fatalf("validation error not reported: %+v", states)
	}
}

func TestEngineStepping(t *testing.T) {
	bus := transport.NewMemory()
	clk := core.NewScaledClock(5)
	e := NewEngine(bus, clk)
	_, err := e.Spawn(AgentConfig{VehicleID: "v1", Manufacturer: "m", Serial: "s1", Model: core.BuiltinModels()[1], BatteryPct: 50})
	if err != nil {
		t.Fatal(err)
	}
	e.Start()
	time.Sleep(300 * time.Millisecond)
	if !e.Running() || len(e.VehicleIDs()) != 1 {
		t.Fatal("engine not running")
	}
	e.Agent("v1").SetFault(FaultEStop)
	if f := e.Agent("v1").Faults(); len(f) != 1 || f[0] != FaultEStop {
		t.Fatalf("faults %v", f)
	}
	e.Stop()
	if e.Running() {
		t.Fatal("engine should stop")
	}
}
