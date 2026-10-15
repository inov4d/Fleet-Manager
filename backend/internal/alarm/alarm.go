// Package alarm manages the alarm lifecycle: raise (deduplicated by key),
// acknowledge, clear, history, and the derived counters used by analytics.
package alarm

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

// Manager keeps active alarms in memory and persists every change.
type Manager struct {
	mu          sync.Mutex
	st          store.Store
	clock       core.Clock
	active      map[string]*core.Alarm // key -> alarm
	byID        map[string]*core.Alarm
	onRaise     func(core.Alarm)
	onClear     func(core.Alarm)
	raisedTotal int
}

// New creates a manager and loads active alarms from the store.
func New(st store.Store, clock core.Clock) *Manager {
	m := &Manager{st: st, clock: clock, active: map[string]*core.Alarm{}, byID: map[string]*core.Alarm{}}
	if st != nil {
		if list, err := st.ListAlarms(context.Background(), "", true, 0); err == nil {
			for i := range list {
				a := list[i]
				m.active[keyOf(a)] = &a
				m.byID[a.ID] = &a
			}
		}
	}
	return m
}

// SetHandlers registers event callbacks.
func (m *Manager) SetHandlers(onRaise, onClear func(core.Alarm)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRaise, m.onClear = onRaise, onClear
}

func keyOf(a core.Alarm) string {
	return string(a.Source) + "|" + a.Code + "|" + a.VehicleID + "|" + a.MissionID
}

// Raise creates an alarm unless an identical active one exists; it returns the
// alarm and whether it was newly raised.
func (m *Manager) Raise(a core.Alarm) (core.Alarm, bool) {
	m.mu.Lock()
	k := keyOf(a)
	if ex := m.active[k]; ex != nil {
		if ex.Message != a.Message || ex.Severity != a.Severity {
			ex.Message, ex.Severity = a.Message, a.Severity
			m.persist(ex)
		}
		c := *ex
		m.mu.Unlock()
		return c, false
	}
	a.ID = uuid.NewString()
	a.Active = true
	a.RaisedAt = m.clock.Now()
	if a.SiteID == "" {
		a.SiteID = core.DefaultSiteID
	}
	m.active[k] = &a
	m.byID[a.ID] = &a
	m.raisedTotal++
	m.persist(&a)
	h := m.onRaise
	c := a
	m.mu.Unlock()
	if h != nil {
		h(c)
	}
	return c, true
}

// ClearByKey clears the active alarm matching source/code/vehicle/mission.
func (m *Manager) ClearByKey(source core.AlarmSource, code, vehicleID, missionID string) bool {
	k := keyOf(core.Alarm{Source: source, Code: code, VehicleID: vehicleID, MissionID: missionID})
	m.mu.Lock()
	a := m.active[k]
	if a == nil {
		m.mu.Unlock()
		return false
	}
	m.clear(a)
	h := m.onClear
	c := *a
	m.mu.Unlock()
	if h != nil {
		h(c)
	}
	return true
}

// ClearVehicleSource clears all active alarms of a source for a vehicle, except those whose code is in keep.
func (m *Manager) ClearVehicleSource(source core.AlarmSource, vehicleID string, keep map[string]bool) {
	m.mu.Lock()
	var cleared []core.Alarm
	for _, a := range m.active {
		if a.Source == source && a.VehicleID == vehicleID && !keep[a.Code] {
			m.clear(a)
			cleared = append(cleared, *a)
		}
	}
	h := m.onClear
	m.mu.Unlock()
	if h != nil {
		for _, a := range cleared {
			h(a)
		}
	}
}

func (m *Manager) clear(a *core.Alarm) {
	now := m.clock.Now()
	a.Active = false
	a.ClearedAt = &now
	delete(m.active, keyOf(*a))
	m.persist(a)
}

// Clear clears an alarm by id (operator action).
func (m *Manager) Clear(id string) error {
	m.mu.Lock()
	a := m.byID[id]
	if a == nil {
		m.mu.Unlock()
		return core.NotFound("alarm", id)
	}
	if !a.Active {
		m.mu.Unlock()
		return nil
	}
	m.clear(a)
	h := m.onClear
	c := *a
	m.mu.Unlock()
	if h != nil {
		h(c)
	}
	return nil
}

// Acknowledge marks an alarm as seen by a user.
func (m *Manager) Acknowledge(id, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.byID[id]
	if a == nil {
		return core.NotFound("alarm", id)
	}
	now := m.clock.Now()
	a.AcknowledgedAt = &now
	a.AcknowledgedBy = user
	m.persist(a)
	return nil
}

func (m *Manager) persist(a *core.Alarm) {
	if m.st != nil {
		_ = m.st.SaveAlarm(context.Background(), a)
	}
}

// Active lists active alarms (newest first).
func (m *Manager) Active(siteID string) []core.Alarm {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]core.Alarm, 0, len(m.active))
	for _, a := range m.active {
		if siteID == "" || a.SiteID == siteID {
			out = append(out, *a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RaisedAt.After(out[j].RaisedAt) })
	return out
}

// ActiveForVehicle counts active alarms for a vehicle.
func (m *Manager) ActiveForVehicle(vehicleID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, a := range m.active {
		if a.VehicleID == vehicleID {
			n++
		}
	}
	return n
}

// History lists alarms from the store.
func (m *Manager) History(siteID string, activeOnly bool, limit int) []core.Alarm {
	if m.st == nil || activeOnly {
		return m.Active(siteID)
	}
	list, err := m.st.ListAlarms(context.Background(), siteID, false, limit)
	if err != nil {
		return nil
	}
	return list
}

// Get returns an alarm by id.
func (m *Manager) Get(id string) (*core.Alarm, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a := m.byID[id]; a != nil {
		c := *a
		return &c, true
	}
	return nil, false
}

// RaisedSince counts alarms raised after t (for analytics, from memory).
func (m *Manager) RaisedTotal() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.raisedTotal
}

var _ = time.Second
