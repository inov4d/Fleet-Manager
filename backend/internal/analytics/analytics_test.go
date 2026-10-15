package analytics

import (
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

func TestKPIs(t *testing.T) {
	st, _ := store.NewMemory("")
	clk := core.NewManualClock(time.Now())
	e := New(st, clk)
	for i := 0; i < 10; i++ {
		e.Observe("v1", i < 5, 10, 0.01)
		clk.Advance(time.Second)
	}
	now := clk.Now()
	start := now.Add(-2 * time.Minute)
	assigned := now.Add(-110 * time.Second)
	m := core.Mission{ID: "m", SiteID: "s", VehicleID: "v1", Status: core.MissionCompleted, CreatedAt: start, AssignedAt: &assigned, StartedAt: &assigned, CompletedAt: &now}
	e.RecordMission(m, 100)
	e.RecordMission(core.Mission{ID: "f", SiteID: "s", VehicleID: "v1", Status: core.MissionFailed}, 0)
	k := e.KPIs(Input{SiteID: "s", Vehicles: []core.VehicleWithState{{Vehicle: core.Vehicle{ID: "v1", SiteID: "s"}, State: core.VehicleState{Connection: core.ConnOnline, BatteryCharge: 70}}}})
	if k.MissionsCompleted != 1 || k.MissionsFailed != 1 || k.AvgWaitTimeS != 10 || k.AvgMissionDurationS != 110 {
		t.Fatalf("%+v", k)
	}
	if k.FleetUtilizationPct < 40 || k.FleetUtilizationPct > 60 || k.DistanceM != 100 || k.VehiclesOnline != 1 {
		t.Fatalf("%+v", k)
	}
	e.Sample("s", Input{}, time.Minute)
	pts, _ := st.QueryMetric(nil, "s", "utilization", time.Time{}, time.Time{})
	if len(pts) != 1 {
		t.Fatal("sample not stored")
	}
}
