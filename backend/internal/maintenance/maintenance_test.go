package maintenance

import (
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

func TestEvaluateAndRecord(t *testing.T) {
	st, _ := store.NewMemory("")
	clk := core.NewManualClock(time.Now())
	m := New(st, clk)
	model := "AGV20T"
	p := &core.MaintenancePlan{Name: "Graissage bogies", ModelID: &model, Metric: core.MetricOdometer, IntervalValue: 1000, DueSoonFraction: 0.9}
	if err := m.SavePlan(p); err != nil {
		t.Fatal(err)
	}
	vehicles := []core.Vehicle{{ID: "v1", ModelID: "AGV20T"}, {ID: "v2", ModelID: "AGV40T"}}
	counters := func(id string) Counters { return Counters{OdometerM: 950} }
	st1 := m.Evaluate(vehicles, counters)
	if len(st1) != 1 || st1[0].VehicleID != "v1" || st1[0].State != "dueSoon" {
		t.Fatalf("%+v", st1)
	}
	counters = func(id string) Counters { return Counters{OdometerM: 1200} }
	if s := m.Evaluate(vehicles, counters); s[0].State != "overdue" {
		t.Fatalf("%+v", s)
	}
	if _, err := m.Record(p.ID, "v1", "tech", "fait", 1200); err != nil {
		t.Fatal(err)
	}
	s := m.Evaluate(vehicles, counters)
	if s[0].State != "ok" || s[0].NextDueValue != 2200 {
		t.Fatalf("after record %+v", s)
	}
}
