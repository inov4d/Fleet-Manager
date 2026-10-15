package colony

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/transport"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

// fakeArbiter is a minimal board back-end.
type fakeArbiter struct {
	mu        sync.Mutex
	tasks     []Task
	granted   map[string]string // task -> vehicle
	requests  []Request
	m         *core.Map
	infos     []VehicleInfo
	busy      map[string]bool
	conflicts []Conflict
	yields    []Yield
}

func (f *fakeArbiter) OpenTasks(string) []Task {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Task
	for _, t := range f.tasks {
		if _, taken := f.granted[t.ID]; !taken {
			out = append(out, t)
		}
	}
	return out
}
func (f *fakeArbiter) Eligible(_, vid, tid string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, taken := f.granted[tid]; taken {
		return "taken", false
	}
	if f.busy[vid] {
		return "busy", false
	}
	return "", true
}
func (f *fakeArbiter) Grant(_, tid, vid string, _ []Claim) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.granted[tid] = vid
	f.busy[vid] = true
	return nil
}
func (f *fakeArbiter) RequestSystemTask(_, vid, kind, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, Request{VehicleID: vid, Type: kind, Reason: reason})
	return nil
}
func (f *fakeArbiter) Release(_, _, _, _ string) error { return nil }
func (f *fakeArbiter) Assignments(string) []Assignment {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Assignment
	for _, vi := range f.infos {
		st := "idle"
		tid := ""
		for t, v := range f.granted {
			if v == vi.ID {
				st, tid = "executing", t
			}
		}
		out = append(out, Assignment{VehicleID: vi.ID, TaskID: tid, Status: st, TS: time.Now()})
	}
	return out
}
func (f *fakeArbiter) ActiveMap(string) (*core.Map, bool) { return f.m, true }
func (f *fakeArbiter) Conflicts(string) []Conflict {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Conflict(nil), f.conflicts...)
}
func (f *fakeArbiter) Yield(_ string, y Yield) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.yields = append(f.yields, y)
	return nil
}
func (f *fakeArbiter) VehicleInfos(string) []VehicleInfo { return f.infos }

func stateMsg(serial, node string, x, y, battery float64) []byte {
	s := vda5050.State{Header: vda5050.Header{HeaderID: 1, Version: vda5050.Version, Manufacturer: "m", SerialNumber: serial}, OperatingMode: vda5050.OpAutomatic,
		NodeStates: []vda5050.NodeState{}, EdgeStates: []vda5050.EdgeState{}, ActionStates: []vda5050.ActionState{}, Errors: []vda5050.Error{}, LastNodeID: node,
		BatteryState: vda5050.BatteryState{BatteryCharge: battery}, SafetyState: vda5050.SafetyState{EStop: vda5050.EStopNone},
		AGVPosition: &vda5050.AGVPosition{X: x, Y: y, MapID: "map-demo", PositionInitialized: true}}
	b, _ := json.Marshal(s)
	return b
}

func TestColonyNearestAgentWinsAndBoardArbitrates(t *testing.T) {
	bus := transport.NewMemory()
	m := mapmgr.DemoMap("s")
	model := core.BuiltinModels()[0]
	arb := &fakeArbiter{granted: map[string]string{}, busy: map[string]bool{}, m: m,
		tasks: []Task{{ID: "t1", Origin: "PICK1", Destination: "DROP1", Nodes: []string{"PICK1", "DROP1"}, Priority: 5, Urgency: 5, MassKg: 1000, LoadPresent: true, CreatedAt: time.Now()}},
		infos: []VehicleInfo{{ID: "v1", SiteID: "s", ModelID: "AGV20T", Model: model, Enabled: true}, {ID: "v2", SiteID: "s", ModelID: "AGV20T", Model: model, Enabled: true}}}
	board := NewBoardServer(bus, arb, nil, 200*time.Millisecond)
	if err := board.Start([]string{"s"}, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	defer board.Stop()
	mk := func(id, serial string) *Agent {
		a := NewAgent(AgentConfig{SiteID: "s", VehicleID: id, Manufacturer: "m", Serial: serial, MinClaimDelay: 50 * time.Millisecond, MaxClaimDelay: 400 * time.Millisecond}, bus, nil)
		if err := a.Start(); err != nil {
			t.Fatal(err)
		}
		return a
	}
	far := mk("v1", "s1")
	near := mk("v2", "s2")
	defer far.Stop()
	defer near.Stop()
	time.Sleep(300 * time.Millisecond) // board publishes map / infos / tasks
	g := mapmgr.NewGraph(m)
	n1, n2 := g.Nodes["CHG4"], g.Nodes["N30"]
	_ = bus.Publish(vda5050.Topic("uagv", "m", "s1", vda5050.TopicState), stateMsg("s1", "CHG4", n1.X, n1.Y, 90), false)
	_ = bus.Publish(vda5050.Topic("uagv", "m", "s2", vda5050.TopicState), stateMsg("s2", "N30", n2.X, n2.Y, 90), false)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		arb.mu.Lock()
		w := arb.granted["t1"]
		arb.mu.Unlock()
		if w != "" {
			if w != "v2" {
				t.Fatalf("the nearest agent should win, got %s", w)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	arb.mu.Lock()
	if arb.granted["t1"] == "" {
		arb.mu.Unlock()
		t.Fatal("task was never claimed")
	}
	arb.mu.Unlock()
	time.Sleep(400 * time.Millisecond)
	st := board.Stats()
	if st.Granted != 1 {
		t.Fatalf("stats %+v", st)
	}
	if s := near.Snapshot(); s.Wins != 1 {
		t.Fatalf("near agent snapshot %+v", s)
	}
	// a second task: the winner is busy, the other agent takes it
	arb.mu.Lock()
	arb.tasks = append(arb.tasks, Task{ID: "t2", Origin: "PICK2", Destination: "DROP2", Nodes: []string{"PICK2", "DROP2"}, Priority: 3, Urgency: 3, CreatedAt: time.Now()})
	arb.mu.Unlock()
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		arb.mu.Lock()
		w := arb.granted["t2"]
		arb.mu.Unlock()
		if w == "v1" {
			return
		}
		if w != "" {
			t.Fatalf("busy agent must not win: %s", w)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("second task never claimed by the free agent")
}

func TestAgentRequestsChargeWhenLow(t *testing.T) {
	bus := transport.NewMemory()
	arb := &fakeArbiter{granted: map[string]string{}, busy: map[string]bool{}, m: mapmgr.DemoMap("s"), infos: []VehicleInfo{{ID: "v1", SiteID: "s", ModelID: "AGV20T", Model: core.BuiltinModels()[0], Enabled: true}}}
	board := NewBoardServer(bus, arb, nil, 100*time.Millisecond)
	_ = board.Start([]string{"s"}, 100*time.Millisecond)
	defer board.Stop()
	a := NewAgent(AgentConfig{SiteID: "s", VehicleID: "v1", Manufacturer: "m", Serial: "s1", LowBatteryPct: 30}, bus, nil)
	_ = a.Start()
	defer a.Stop()
	time.Sleep(250 * time.Millisecond)
	_ = bus.Publish(vda5050.Topic("uagv", "m", "s1", vda5050.TopicState), stateMsg("s1", "PARK1", -10, 10, 20), false)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		arb.mu.Lock()
		n := len(arb.requests)
		arb.mu.Unlock()
		if n > 0 {
			if arb.requests[0].Type != "charge" {
				t.Fatalf("expected charge request, got %+v", arb.requests[0])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no charge request")
}

// TestLocalNegotiationIdleNeighbourYields: v1 executes a task and waits behind
// v2, which is idle. v1 proposes; v2 answers with a lower yield cost and gives
// way; the board receives the agreed Yield.
func TestLocalNegotiationIdleNeighbourYields(t *testing.T) {
	bus := transport.NewMemory()
	model := core.BuiltinModels()[0]
	arb := &fakeArbiter{granted: map[string]string{"t1": "v1"}, busy: map[string]bool{"v1": true}, m: mapmgr.DemoMap("s"),
		infos:     []VehicleInfo{{ID: "v1", SiteID: "s", ModelID: "AGV20T", Model: model, Enabled: true}, {ID: "v2", SiteID: "s", ModelID: "AGV20T", Model: model, Enabled: true}},
		conflicts: []Conflict{{VehicleID: "v1", Resource: core.ResourceRef{Type: core.ResNode, ID: "N01"}, HeldBy: []string{"v2"}, Since: time.Now().Add(-10 * time.Second), Active: true}, {VehicleID: "v2"}}}
	board := NewBoardServer(bus, arb, nil, 100*time.Millisecond)
	_ = board.Start([]string{"s"}, 100*time.Millisecond)
	defer board.Stop()
	dbg := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mk := func(id, serial string) *Agent {
		a := NewAgent(AgentConfig{SiteID: "s", VehicleID: id, Manufacturer: "m", Serial: serial, NegotiateAfter: 300 * time.Millisecond}, bus, dbg)
		if err := a.Start(); err != nil {
			t.Fatal(err)
		}
		return a
	}
	v1, v2 := mk("v1", "s1"), mk("v2", "s2")
	defer v1.Stop()
	defer v2.Stop()
	_ = bus.Publish(vda5050.Topic("uagv", "m", "s1", vda5050.TopicState), stateMsg("s1", "N00", 10, 10, 90), false)
	_ = bus.Publish(vda5050.Topic("uagv", "m", "s2", vda5050.TopicState), stateMsg("s2", "N01", 40, 10, 90), false)
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		arb.mu.Lock()
		n := len(arb.yields)
		arb.mu.Unlock()
		if n > 0 {
			y := arb.yields[0]
			if y.VehicleID != "v2" || y.Beneficiary != "v1" || y.Resource.ID != "N01" {
				t.Fatalf("unexpected yield %+v", y)
			}
			s1, s2 := v1.Snapshot(), v2.Snapshot()
			if s2.Yielded != 1 || s1.Passed != 1 || len(s1.Negotiations) != 1 || s1.Negotiations[0].IYield {
				t.Fatalf("snapshots v1=%+v v2=%+v", s1, s2)
			}
			if st := board.Stats(); st.Negotiated != 1 {
				t.Fatalf("board stats %+v", st)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no negotiated yield")
}

func TestDecideYieldIsSymmetric(t *testing.T) {
	if !DecideYield("a", 1, "b", 5) || DecideYield("b", 5, "a", 1) {
		t.Fatal("cheaper side must yield")
	}
	if !DecideYield("a", 3, "b", 3) || DecideYield("b", 3, "a", 3) {
		t.Fatal("ties must be broken consistently")
	}
}
