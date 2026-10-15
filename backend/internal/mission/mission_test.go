package mission

import (
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

func newMgr() (*Manager, *core.ManualClock) {
	st, _ := store.NewMemory("")
	clk := core.NewManualClock(time.Now())
	return New(st, clk), clk
}

func TestLifecycleAndDependencies(t *testing.T) {
	m, _ := newMgr()
	a, err := m.Create(core.Mission{Origin: "p1", Destination: "d1", Load: core.MissionLoad{Present: true, MassKg: 1000}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != core.MissionPending || len(a.Steps) != 2 || a.Steps[0].Action != core.StepPick || a.Steps[1].Action != core.StepDrop {
		t.Fatalf("%+v", a)
	}
	b, err := m.Create(core.Mission{Destination: "d2", Dependencies: []string{a.ID}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != core.MissionCreated {
		t.Fatal("dependent mission must wait")
	}
	if _, err := m.Assign(a.ID, "v1", nil, []string{"x", "p1", "d1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Complete(a.ID); err != nil {
		t.Fatal(err)
	}
	bb, _ := m.Get(b.ID)
	if bb.Status != core.MissionPending {
		t.Fatal("dependency release failed")
	}
	if _, err := m.Cancel(a.ID, "op"); err == nil {
		t.Fatal("cannot cancel a completed mission")
	}
	if err := m.Delete(b.ID); err == nil {
		t.Fatal("cannot delete a pending mission")
	}
	if _, err := m.Create(core.Mission{Destination: "d", Dependencies: []string{"ghost"}}, nil); err == nil {
		t.Fatal("unknown dependency accepted")
	}
	if _, err := m.Create(core.Mission{Destination: "zz"}, func(n string) bool { return n != "zz" }); err == nil {
		t.Fatal("unknown node accepted")
	}
}

func TestControlledReassignment(t *testing.T) {
	m, _ := newMgr()
	a, _ := m.Create(core.Mission{Origin: "p1", Destination: "d1", Load: core.MissionLoad{Present: true, MassKg: 1000}}, nil)
	_, _ = m.Assign(a.ID, "v1", nil, nil)
	_, _ = m.Start(a.ID)
	if _, err := m.HoldForVerification(a.ID, "vehicle lost with load possibly on board"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Unassign(a.ID, "reassign", false); err == nil {
		t.Fatal("reassignment without verification must be refused")
	}
	ms, err := m.Unassign(a.ID, "reassign after check", true)
	if err != nil || ms.Status != core.MissionPending || ms.VehicleID != "" {
		t.Fatalf("%v %+v", err, ms)
	}
}

func TestRestartResetsExecutions(t *testing.T) {
	st, _ := store.NewMemory("")
	clk := core.NewManualClock(time.Now())
	m := New(st, clk)
	a, _ := m.Create(core.Mission{Destination: "d1"}, nil)
	_, _ = m.Assign(a.ID, "v1", nil, nil)
	_, _ = m.Start(a.ID)
	m2 := New(st, clk)
	got, _ := m2.Get(a.ID)
	if got.Status != core.MissionPending || got.VehicleID != "" {
		t.Fatalf("after restart %+v", got)
	}
}

func TestDispatcherScoringAndExplanation(t *testing.T) {
	s := core.DefaultDispatcherSettings()
	route := func(d float64) func(core.Mission) (float64, float64, float64, []string, bool) {
		return func(core.Mission) (float64, float64, float64, []string, bool) {
			return d, d / 1.0, 0, []string{"a", "b"}, true
		}
	}
	energy := func(d, mass float64) float64 { return d * 0.05 }
	cands := []CandidateCtx{
		{VehicleID: "near-lowbat", SiteID: "s", ModelID: "AGV20T", Available: true, BatteryPct: 25, CapacityKg: 20000, QueueLen: 0, Route: route(50), EnergyPct: energy},
		{VehicleID: "far-ok", SiteID: "s", ModelID: "AGV20T", Available: true, BatteryPct: 90, CapacityKg: 20000, QueueLen: 0, Route: route(300), EnergyPct: energy},
		{VehicleID: "small", SiteID: "s", ModelID: "AGV20T", Available: true, BatteryPct: 90, CapacityKg: 20000, QueueLen: 0, Route: route(10), EnergyPct: energy},
		{VehicleID: "busy", SiteID: "s", ModelID: "AGV40T", Available: false, Unavailable: "busy", BatteryPct: 90, CapacityKg: 40000, Route: route(10)},
		{VehicleID: "big", SiteID: "s", ModelID: "AGV40T", Available: true, BatteryPct: 95, CapacityKg: 40000, QueueLen: 0, Route: route(120), EnergyPct: energy},
	}
	heavy := core.Mission{ID: "m1", SiteID: "s", Priority: 5, Load: core.MissionLoad{Present: true, MassKg: 30000}}
	light := core.Mission{ID: "m2", SiteID: "s", Priority: 3, Load: core.MissionLoad{Present: true, MassKg: 1000}}
	dec := Dispatch([]core.Mission{heavy, light}, cands, s)
	if len(dec) != 2 {
		t.Fatalf("decisions %v", dec)
	}
	if dec[0].MissionID != "m1" || dec[0].VehicleID != "big" {
		t.Fatalf("heavy mission should go to the 40T: %+v", dec[0])
	}
	if dec[1].VehicleID != "small" {
		t.Fatalf("light mission should go to the nearest eligible: %+v", dec[1])
	}
	var reasons = map[string]string{}
	for _, c := range dec[1].Explanation.Candidates {
		reasons[c.VehicleID] = c.Reason
	}
	if reasons["near-lowbat"] == "" || reasons["busy"] != "busy" || reasons["big"] != "already assigned in this round" {
		t.Fatalf("explanations %v", reasons)
	}
	if dec[1].Explanation.Candidates[0].VehicleID != "small" || !dec[1].Explanation.Candidates[0].Eligible {
		t.Fatal("candidates should be sorted best first")
	}
}
