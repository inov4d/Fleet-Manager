package alarm

import (
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

func TestAlarmLifecycle(t *testing.T) {
	st, _ := store.NewMemory("")
	m := New(st, core.NewManualClock(time.Now()))
	raised, cleared := 0, 0
	m.SetHandlers(func(core.Alarm) { raised++ }, func(core.Alarm) { cleared++ })
	a, isNew := m.Raise(core.Alarm{Severity: core.SevError, Code: "COMMS_LOST", Source: core.AlarmSrcComms, VehicleID: "v1", Message: "lost"})
	if !isNew || !a.Active || raised != 1 {
		t.Fatal("raise")
	}
	if _, again := m.Raise(core.Alarm{Severity: core.SevError, Code: "COMMS_LOST", Source: core.AlarmSrcComms, VehicleID: "v1", Message: "lost"}); again {
		t.Fatal("duplicate must not re-raise")
	}
	if err := m.Acknowledge(a.ID, "op"); err != nil {
		t.Fatal(err)
	}
	if !m.ClearByKey(core.AlarmSrcComms, "COMMS_LOST", "v1", "") || cleared != 1 {
		t.Fatal("clear by key")
	}
	if len(m.Active("")) != 0 {
		t.Fatal("should be inactive")
	}
	h := m.History("", false, 10)
	if len(h) != 1 || h[0].AcknowledgedBy != "op" || h[0].ClearedAt == nil {
		t.Fatalf("history %+v", h)
	}
	// reload from store keeps active alarms
	m.Raise(core.Alarm{Severity: core.SevWarning, Code: "BAT_LOW", Source: core.AlarmSrcBattery, VehicleID: "v2"})
	m2 := New(st, core.NewManualClock(time.Now()))
	if len(m2.Active("")) != 1 {
		t.Fatal("active alarms must reload")
	}
}
