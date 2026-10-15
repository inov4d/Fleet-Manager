package fleet

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/geometry"
	"github.com/inov4d/fleet-manager/internal/traffic"
)

// TestFleetStressBodiesNeverOverlap runs a dense random-transport scenario and
// checks every 100 ms that no two vehicle bodies overlap and that no two
// stopped vehicles report the same node.
func TestFleetStressBodiesNeverOverlap(t *testing.T) {
	runScenario(t, SimulationStart{Vehicles: 10, TimeScale: 20, Scenario: &SimulationScenario{Name: "randomTransport", MissionRatePerMin: 6}}, 75*time.Second, 3)
}

// TestFleetSingleVehicleBaseline calibrates the free-flow mission duration.
func TestFleetSingleVehicleBaseline(t *testing.T) {
	runScenario(t, SimulationStart{Vehicles: 1, TimeScale: 20, Scenario: &SimulationScenario{Name: "randomTransport", MissionRatePerMin: 6}}, 75*time.Second, 2)
}

// TestFleetModerateDensity measures throughput on the large demo layout.
func TestFleetModerateDensity(t *testing.T) {
	runScenario(t, SimulationStart{Vehicles: 14, TimeScale: 20, Scenario: &SimulationScenario{Name: "randomTransport", MissionRatePerMin: 8}}, 75*time.Second, 3)
}

func runScenario(t *testing.T, start SimulationStart, wall time.Duration, minCompleted int) {
	t.Helper()
	if testing.Short() {
		t.Skip("stress test")
	}
	c := newCore(t)
	var traceMu sync.Mutex
	var lines []string
	traffic.Trace = func(l string) {
		traceMu.Lock()
		lines = append(lines, time.Now().Format("15:04:05.000")+" "+l)
		if len(lines) > 20000 {
			lines = lines[len(lines)-20000:]
		}
		traceMu.Unlock()
	}
	traffic.Strict = true
	defer func() { traffic.Trace = nil; traffic.Strict = false }()
	dump := func(node string) string {
		traceMu.Lock()
		defer traceMu.Unlock()
		var out []string
		for _, l := range lines {
			if strings.Contains(l, " "+node+" ") {
				out = append(out, l)
			}
		}
		if len(out) > 25 {
			out = out[len(out)-25:]
		}
		return strings.Join(out, "\n")
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	if _, err := c.StartSimulation(start); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(wall)
	checks := 0
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		checks++
		vs := c.Vehicles("")
		for i := 0; i < len(vs); i++ {
			for j := i + 1; j < len(vs); j++ {
				a, b := vs[i], vs[j]
				if !a.State.PositionInitialized || !b.State.PositionInitialized {
					continue
				}
				ma, mb := c.models[a.Vehicle.ModelID], c.models[b.Vehicle.ModelID]
				ra := geometry.Rect(a.State.Pose, ma.LengthM, ma.WidthM)
				rb := geometry.Rect(b.State.Pose, mb.LengthM, mb.WidthM)
				if geometry.Intersects(ra, rb) {
					t.Fatalf("bodies overlap after %d checks:\n%s\n%s\nTRACE %s:\n%s\nTRACE %s:\n%s", checks, describe(c, a), describe(c, b), a.Vehicle.ID, dump(a.Vehicle.ID), b.Vehicle.ID, dump(b.Vehicle.ID))
				}
				if !a.State.Driving && !b.State.Driving && a.State.LastNodeID != "" && a.State.LastNodeID == b.State.LastNodeID {
					t.Fatalf("two stopped vehicles on %s:\n%s\n%s", a.State.LastNodeID, describe(c, a), describe(c, b))
				}
			}
		}
	}
	k := c.KPIs("", time.Time{}, time.Time{})
	// moving ratio from telemetry: share of in-mission samples with speed > 0
	inMission, moving := 0, 0
	for _, v := range c.Vehicles("") {
		samples, _ := c.Store().QueryTelemetry(context.Background(), v.Vehicle.ID, time.Time{}, time.Time{}, 0)
		for _, s := range samples {
			if s.MissionID != "" {
				inMission++
				if s.Speed > 0.05 {
					moving++
				}
			}
		}
	}
	ratio := 0.0
	if inMission > 0 {
		ratio = float64(moving) / float64(inMission) * 100
	}
	t.Logf("%d vehicles: %d checks, completed=%d failed=%d avgDuration=%.0fs avgWait=%.0fs deadlocksResolved=%d moving=%.0f%% alarms=%d",
		start.Vehicles, checks, k.MissionsCompleted, k.MissionsFailed, k.AvgMissionDurationS, k.AvgWaitTimeS, k.DeadlocksResolved, ratio, k.ActiveAlarms)
	if k.MissionsCompleted < minCompleted {
		t.Fatalf("fleet is not moving enough: %+v", k)
	}
}

func describe(c *Core, v core.VehicleWithState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s pose=(%.1f,%.1f,%.2f) lastNode=%s seq=%d order=%s driving=%v avail=%s mission=%s",
		v.Vehicle.Name, v.State.Pose.X, v.State.Pose.Y, v.State.Pose.Theta, v.State.LastNodeID, v.State.LastNodeSequenceID, v.State.OrderID, v.State.Driving, v.State.Availability, v.State.MissionID)
	if m, err := c.Missions().Get(v.State.MissionID); err == nil {
		fmt.Fprintf(&b, "\n   route=%v", m.RouteNodeIDs)
		for _, h := range m.History {
			fmt.Fprintf(&b, "\n   %s %s: %s", h.At.Format("15:04:05"), h.Status, h.Message)
		}
	}
	c.mu.RLock()
	if ex := c.execs[v.Vehicle.ID]; ex != nil {
		fmt.Fprintf(&b, "\n   exec routeIdx=%d sentThrough=%d orderID=%s route=%v", ex.routeIdx, ex.sentThrough, ex.orderID, ex.route)
	}
	c.mu.RUnlock()
	return b.String()
}
