package fleet

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
	"github.com/inov4d/fleet-manager/internal/transport"
)

func newCore(t *testing.T) *Core {
	t.Helper()
	st, _ := store.NewMemory("")
	cfg := DefaultConfig()
	cfg.TickHz = 20
	cfg.ReplanAfter = 3 * time.Second
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	c, err := New(cfg, Deps{Store: st, Transport: transport.NewMemory(), Logger: log})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestEndToEndTransportMission(t *testing.T) {
	c := newCore(t)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	st, err := c.StartSimulation(SimulationStart{Vehicles: 3, TimeScale: 20, ModelMix: map[string]float64{"AGV20T": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || len(st.VehicleIDs) != 3 {
		t.Fatalf("sim status %+v", st)
	}
	waitFor(t, 10*time.Second, "vehicles online and available", func() bool {
		n := 0
		for _, v := range c.Vehicles("") {
			if v.State.Availability == core.AvailAvailable && v.State.PositionInitialized {
				n++
			}
		}
		return n == 3
	})
	m, err := c.CreateMission(core.Mission{Origin: "PICK1", Destination: "DROP3", Load: core.MissionLoad{Present: true, MassKg: 12000, LengthM: 2.4, WidthM: 1.2}, Priority: 5})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "assignment", func() bool {
		mm, _ := c.Missions().Get(m.ID)
		return mm.Status == core.MissionExecuting && mm.VehicleID != ""
	})
	mm, _ := c.Missions().Get(m.ID)
	if mm.Assignment == nil || mm.Assignment.ChosenVehicleID != mm.VehicleID || len(mm.Assignment.Candidates) != 3 {
		t.Fatalf("assignment explanation missing: %+v", mm.Assignment)
	}
	vid := mm.VehicleID
	waitFor(t, 90*time.Second, "mission completion", func() bool {
		mm, _ := c.Missions().Get(m.ID)
		return mm.Status.Terminal()
	})
	mm, _ = c.Missions().Get(m.ID)
	if mm.Status != core.MissionCompleted {
		t.Fatalf("mission ended as %s: %s %+v", mm.Status, mm.ErrorMessage, mm.History)
	}
	for _, s := range mm.Steps {
		if s.Status != core.StepDone {
			t.Fatalf("step not done: %+v", s)
		}
	}
	waitFor(t, 5*time.Second, "vehicle available again with counters", func() bool {
		v, _ := c.Vehicle(vid)
		return v.State.Availability == core.AvailAvailable && v.State.MissionsCompleted == 1 && v.State.OdometerM > 50 && !v.State.LoadPresent
	})
	// only standing reservations remain for that vehicle
	for _, r := range c.Traffic().Reservations("") {
		if r.VehicleID == vid && r.State == core.ResvGranted {
			t.Fatalf("granted reservation left after completion: %+v", r)
		}
	}
	k := c.KPIs("", time.Time{}, time.Time{})
	if k.MissionsCompleted != 1 || k.DistanceM <= 0 {
		t.Fatalf("kpis %+v", k)
	}
	snap := c.Snapshot("")
	if len(snap.Vehicles) != 3 || snap.KPIs.MissionsCompleted != 1 {
		t.Fatal("snapshot")
	}
}

func TestCommsLossSuspendsAndProtects(t *testing.T) {
	c := newCore(t)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	if _, err := c.StartSimulation(SimulationStart{Vehicles: 1, TimeScale: 10, ModelMix: map[string]float64{"AGV40T": 1}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, "vehicle available", func() bool {
		vs := c.Vehicles("")
		return len(vs) == 1 && vs[0].State.Availability == core.AvailAvailable
	})
	vid := c.Vehicles("")[0].Vehicle.ID
	m, err := c.CreateMission(core.Mission{Origin: "PICK2", Destination: "DROP4", Load: core.MissionLoad{Present: true, MassKg: 30000}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "driving", func() bool {
		v, _ := c.Vehicle(vid)
		return v.State.Driving
	})
	if err := c.InjectFault(vid, "commsLoss"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 15*time.Second, "comms lost handling", func() bool {
		mm, _ := c.Missions().Get(m.ID)
		v, _ := c.Vehicle(vid)
		return v.State.Connection == core.ConnBroken && mm.Status == core.MissionSuspended
	})
	found := false
	for _, a := range c.Alarms().Active("") {
		if a.Code == "COMMS_LOST" && a.VehicleID == vid {
			found = true
		}
	}
	if !found {
		t.Fatal("COMMS_LOST alarm missing")
	}
	// reservations of the lost vehicle are uncertain, not released
	uncertain := 0
	for _, r := range c.Traffic().Reservations("") {
		if r.VehicleID == vid {
			if r.State != core.ResvUncertain {
				t.Fatalf("reservation %v should be uncertain", r)
			}
			uncertain++
		}
	}
	if uncertain == 0 {
		t.Fatal("expected uncertain reservations to be kept")
	}
	// reassignment without physical verification is refused when the load might be on board
	mm, _ := c.Missions().Get(m.ID)
	if mm.VehicleID != vid {
		t.Fatal("mission must stay assigned while the vehicle is lost")
	}
	// comms back: the mission resumes and completes
	if err := c.InjectFault(vid, "restore"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 120*time.Second, "completion after restore", func() bool {
		mm, _ := c.Missions().Get(m.ID)
		return mm.Status.Terminal()
	})
	mm, _ = c.Missions().Get(m.ID)
	if mm.Status != core.MissionCompleted {
		t.Fatalf("mission ended as %s: %+v", mm.Status, mm.History)
	}
	if len(c.Alarms().Active("")) != 0 {
		t.Fatalf("alarms should be cleared: %+v", c.Alarms().Active(""))
	}
}
