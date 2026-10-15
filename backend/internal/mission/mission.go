// Package mission manages the mission lifecycle (creation, dependencies,
// state transitions, history) and the explainable automatic assignment
// engine (dispatcher).
package mission

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

// Manager holds missions in memory and persists changes.
type Manager struct {
	mu       sync.Mutex
	st       store.Store
	clock    core.Clock
	missions map[string]*core.Mission
	onChange func(core.Mission, string)
}

// New loads non-terminal and recent missions from the store.
func New(st store.Store, clock core.Clock) *Manager {
	m := &Manager{st: st, clock: clock, missions: map[string]*core.Mission{}}
	if st != nil {
		if list, err := st.ListMissions(context.Background(), store.MissionFilter{Limit: 2000}); err == nil {
			for i := range list {
				ms := list[i]
				// executions do not survive a restart: assigned/executing missions go back to pending
				if ms.Status == core.MissionAssigned || ms.Status == core.MissionExecuting || ms.Status == core.MissionPlanned {
					ms.Status = core.MissionPending
					ms.VehicleID, ms.RouteNodeIDs, ms.Assignment = "", nil, nil
					ms.History = append(ms.History, core.MissionEvent{At: clock.Now(), Status: core.MissionPending, Message: "reset after fleet manager restart"})
					for j := range ms.Steps {
						if ms.Steps[j].Status == core.StepActive {
							ms.Steps[j].Status = core.StepPending
						}
					}
				}
				m.missions[ms.ID] = &ms
			}
		}
	}
	return m
}

// SetHandler registers a change callback (mission copy, event kind).
func (m *Manager) SetHandler(h func(core.Mission, string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = h
}

func (m *Manager) emit(ms *core.Mission, kind string) {
	if m.st != nil {
		_ = m.st.SaveMission(context.Background(), ms)
	}
	if m.onChange != nil {
		m.onChange(*ms, kind)
	}
}

func (m *Manager) record(ms *core.Mission, status core.MissionStatus, msg string) {
	ms.Status = status
	ms.History = append(ms.History, core.MissionEvent{At: m.clock.Now(), Status: status, Message: msg})
}

// Validate checks and normalises a new mission.
func Validate(ms *core.Mission, nodeExists func(string) bool) error {
	if ms.SiteID == "" {
		ms.SiteID = core.DefaultSiteID
	}
	if ms.Type == "" {
		ms.Type = core.MissionTransport
	}
	switch ms.Type {
	case core.MissionTransport, core.MissionMultiStop, core.MissionMachineSupply, core.MissionProductionEvacuation, core.MissionTransfer,
		core.MissionReturnToStation, core.MissionCharge, core.MissionManual, core.MissionExternal:
	default:
		return core.Validation("invalid mission type %q", ms.Type)
	}
	if ms.Priority < 1 {
		ms.Priority = 5
	}
	if ms.Priority > 10 {
		ms.Priority = 10
	}
	if len(ms.Steps) == 0 {
		if ms.Destination == "" {
			return core.Validation("destination is required")
		}
		if ms.Origin != "" && ms.Origin != ms.Destination {
			a := core.StepMove
			if ms.Load.Present {
				a = core.StepPick
			}
			ms.Steps = append(ms.Steps, core.MissionStep{NodeID: ms.Origin, Action: a})
		}
		a := core.StepMove
		switch {
		case ms.Type == core.MissionCharge:
			a = core.StepCharge
		case ms.Load.Present:
			a = core.StepDrop
		}
		ms.Steps = append(ms.Steps, core.MissionStep{NodeID: ms.Destination, Action: a})
	}
	for i := range ms.Steps {
		s := &ms.Steps[i]
		s.Seq = i
		s.Status = core.StepPending
		if s.Action == "" {
			s.Action = core.StepMove
		}
		switch s.Action {
		case core.StepMove, core.StepPick, core.StepDrop, core.StepWait, core.StepCharge:
		default:
			return core.Validation("step %d has invalid action %q", i, s.Action)
		}
		if s.NodeID == "" {
			return core.Validation("step %d has no node", i)
		}
		if nodeExists != nil && !nodeExists(s.NodeID) {
			return core.Validation("step %d references unknown node %q", i, s.NodeID)
		}
	}
	if ms.Origin == "" {
		ms.Origin = ms.Steps[0].NodeID
	}
	if ms.Destination == "" {
		ms.Destination = ms.Steps[len(ms.Steps)-1].NodeID
	}
	if ms.Load.MassKg < 0 {
		return core.Validation("load mass cannot be negative")
	}
	if ms.Source == "" {
		ms.Source = core.SourceOperator
	}
	if ms.Dependencies == nil {
		ms.Dependencies = []string{}
	}
	if ms.Constraints.AvoidZoneIDs == nil {
		ms.Constraints.AvoidZoneIDs = []string{}
	}
	if ms.RouteNodeIDs == nil {
		ms.RouteNodeIDs = []string{}
	}
	if ms.History == nil {
		ms.History = []core.MissionEvent{}
	}
	return nil
}

// Create validates, stores and returns the mission.
func (m *Manager) Create(ms core.Mission, nodeExists func(string) bool) (*core.Mission, error) {
	if err := Validate(&ms, nodeExists); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ms.ID == "" {
		ms.ID = uuid.NewString()
	}
	if _, exists := m.missions[ms.ID]; exists {
		return nil, core.Conflict("mission %q already exists", ms.ID)
	}
	for _, d := range ms.Dependencies {
		if _, ok := m.missions[d]; !ok {
			return nil, core.Validation("unknown dependency %q", d)
		}
	}
	ms.CreatedAt = m.clock.Now()
	if ms.Name == "" {
		ms.Name = string(ms.Type) + " " + ms.Origin + " → " + ms.Destination
	}
	m.record(&ms, core.MissionCreated, "created by "+string(ms.Source))
	if m.depsSatisfied(&ms) {
		m.record(&ms, core.MissionPending, "waiting for a vehicle")
	}
	m.missions[ms.ID] = &ms
	m.emit(&ms, "mission.created")
	c := ms
	return &c, nil
}

func (m *Manager) depsSatisfied(ms *core.Mission) bool {
	for _, d := range ms.Dependencies {
		dep := m.missions[d]
		if dep == nil || dep.Status != core.MissionCompleted {
			return false
		}
	}
	return true
}

// Get returns a copy.
func (m *Manager) Get(id string) (*core.Mission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms := m.missions[id]
	if ms == nil {
		if m.st != nil {
			if stored, err := m.st.GetMission(context.Background(), id); err == nil {
				return stored, nil
			}
		}
		return nil, core.NotFound("mission", id)
	}
	c := *ms
	return &c, nil
}

// List returns missions (newest first), optionally filtered.
func (m *Manager) List(siteID string, status core.MissionStatus, vehicleID string, limit int, activeOnly bool) []core.Mission {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]core.Mission, 0, len(m.missions))
	for _, ms := range m.missions {
		if siteID != "" && ms.SiteID != siteID {
			continue
		}
		if status != "" && ms.Status != status {
			continue
		}
		if vehicleID != "" && ms.VehicleID != vehicleID {
			continue
		}
		if activeOnly && ms.Status.Terminal() {
			continue
		}
		out = append(out, *ms)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Pending returns pending missions for a site ordered by priority (desc) then age.
func (m *Manager) Pending(siteID string) []core.Mission {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []core.Mission
	for _, ms := range m.missions {
		if ms.Status == core.MissionPending && (siteID == "" || ms.SiteID == siteID) {
			out = append(out, *ms)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// Update applies fn to a mission under lock and persists/emits when fn returns a kind.
func (m *Manager) Update(id string, fn func(ms *core.Mission) (kind string, err error)) (*core.Mission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms := m.missions[id]
	if ms == nil {
		return nil, core.NotFound("mission", id)
	}
	kind, err := fn(ms)
	if err != nil {
		return nil, err
	}
	if kind != "" {
		m.emit(ms, kind)
	}
	c := *ms
	return &c, nil
}

// Assign marks a mission assigned to a vehicle with its explanation and route.
func (m *Manager) Assign(id, vehicleID string, expl *core.AssignmentExplanation, route []string) (*core.Mission, error) {
	return m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status != core.MissionPending {
			return "", core.Conflict("mission %s is %s", id, ms.Status)
		}
		now := m.clock.Now()
		ms.VehicleID = vehicleID
		ms.AssignedAt = &now
		ms.Assignment = expl
		ms.RouteNodeIDs = route
		ms.LoadOnBoardUnverified = false
		m.record(ms, core.MissionAssigned, "assigned to "+vehicleID)
		return "mission.updated", nil
	})
}

// Start marks execution start.
func (m *Manager) Start(id string) (*core.Mission, error) {
	return m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status != core.MissionAssigned && ms.Status != core.MissionSuspended && ms.Status != core.MissionPlanned {
			return "", core.Conflict("mission %s is %s", id, ms.Status)
		}
		if ms.StartedAt == nil {
			now := m.clock.Now()
			ms.StartedAt = &now
		}
		m.record(ms, core.MissionExecuting, "execution started")
		return "mission.updated", nil
	})
}

// Complete marks a mission completed.
func (m *Manager) Complete(id string) (*core.Mission, error) {
	ms, err := m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status.Terminal() {
			return "", core.Conflict("mission %s already %s", id, ms.Status)
		}
		now := m.clock.Now()
		ms.CompletedAt = &now
		ms.Progress = 1
		for i := range ms.Steps {
			if ms.Steps[i].Status != core.StepDone {
				ms.Steps[i].Status = core.StepDone
			}
		}
		m.record(ms, core.MissionCompleted, "completed")
		return "mission.completed", nil
	})
	if err == nil {
		m.releaseDependents(id)
	}
	return ms, err
}

// Fail marks a mission failed.
func (m *Manager) Fail(id, reason string) (*core.Mission, error) {
	return m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status.Terminal() {
			return "", core.Conflict("mission %s already %s", id, ms.Status)
		}
		now := m.clock.Now()
		ms.CompletedAt = &now
		ms.ErrorMessage = reason
		m.record(ms, core.MissionFailed, reason)
		return "mission.failed", nil
	})
}

// Cancel cancels a mission (any non-terminal state).
func (m *Manager) Cancel(id, by string) (*core.Mission, error) {
	return m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status.Terminal() {
			return "", core.Conflict("mission %s already %s", id, ms.Status)
		}
		now := m.clock.Now()
		ms.CompletedAt = &now
		m.record(ms, core.MissionCancelled, "cancelled by "+by)
		return "mission.updated", nil
	})
}

// Suspend pauses an executing or assigned mission.
func (m *Manager) Suspend(id, reason string) (*core.Mission, error) {
	return m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status != core.MissionExecuting && ms.Status != core.MissionAssigned {
			return "", core.Conflict("mission %s is %s", id, ms.Status)
		}
		m.record(ms, core.MissionSuspended, reason)
		return "mission.updated", nil
	})
}

// Resume returns a suspended mission to execution (or assigned if not started).
func (m *Manager) Resume(id string) (*core.Mission, error) {
	return m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status != core.MissionSuspended {
			return "", core.Conflict("mission %s is %s", id, ms.Status)
		}
		if ms.VehicleID == "" {
			m.record(ms, core.MissionPending, "resumed, waiting for a vehicle")
		} else if ms.StartedAt == nil {
			m.record(ms, core.MissionAssigned, "resumed")
		} else {
			m.record(ms, core.MissionExecuting, "resumed")
		}
		return "mission.updated", nil
	})
}

// Unassign returns a mission to the pending queue (controlled reassignment).
// When the load may be on board and the physical state is unverified, the
// caller must pass confirmLoad=true, otherwise the call is refused.
func (m *Manager) Unassign(id, reason string, confirmLoad bool) (*core.Mission, error) {
	return m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status.Terminal() || ms.VehicleID == "" {
			return "", core.Conflict("mission %s cannot be reassigned (status %s)", id, ms.Status)
		}
		if ms.LoadOnBoardUnverified && !confirmLoad {
			return "", core.Conflict("load may be on board of %s: physical state must be verified (confirmLoadState)", ms.VehicleID)
		}
		ms.VehicleID = ""
		ms.AssignedAt, ms.StartedAt = nil, nil
		ms.RouteNodeIDs = []string{}
		ms.Assignment = nil
		ms.Progress = 0
		ms.LoadOnBoardUnverified = false
		for i := range ms.Steps {
			ms.Steps[i].Status = core.StepPending
			ms.Steps[i].StartedAt, ms.Steps[i].CompletedAt = nil, nil
		}
		m.record(ms, core.MissionPending, reason)
		return "mission.updated", nil
	})
}

// HoldForVerification suspends a mission whose vehicle became unavailable
// while a load might be on board.
func (m *Manager) HoldForVerification(id, reason string) (*core.Mission, error) {
	return m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status.Terminal() {
			return "", nil
		}
		ms.LoadOnBoardUnverified = true
		m.record(ms, core.MissionSuspended, reason)
		return "mission.updated", nil
	})
}

// SetPriority changes priority of a non-terminal mission.
func (m *Manager) SetPriority(id string, p int) (*core.Mission, error) {
	return m.Update(id, func(ms *core.Mission) (string, error) {
		if ms.Status.Terminal() {
			return "", core.Conflict("mission is terminal")
		}
		if p < 1 || p > 10 {
			return "", core.Validation("priority must be 1..10")
		}
		ms.Priority = p
		return "mission.updated", nil
	})
}

// Delete removes a terminal mission.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms := m.missions[id]
	if ms != nil && !ms.Status.Terminal() {
		return core.Conflict("only terminal missions can be deleted")
	}
	delete(m.missions, id)
	if m.st != nil {
		return m.st.DeleteMission(context.Background(), id)
	}
	return nil
}

func (m *Manager) releaseDependents(completedID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ms := range m.missions {
		if ms.Status != core.MissionCreated {
			continue
		}
		depends := false
		for _, d := range ms.Dependencies {
			if d == completedID {
				depends = true
			}
		}
		if depends && m.depsSatisfied(ms) {
			m.record(ms, core.MissionPending, "dependencies satisfied")
			m.emit(ms, "mission.updated")
		}
	}
}

// ByExternalRef finds a mission by external reference.
func (m *Manager) ByExternalRef(ref string) (*core.Mission, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ms := range m.missions {
		if ms.ExternalRef == ref {
			c := *ms
			return &c, true
		}
	}
	return nil, false
}

// Prune drops terminal missions older than maxAge from memory (they stay in the store).
func (m *Manager) Prune(maxAge time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cut := m.clock.Now().Add(-maxAge)
	for id, ms := range m.missions {
		if ms.Status.Terminal() && ms.CompletedAt != nil && ms.CompletedAt.Before(cut) {
			delete(m.missions, id)
		}
	}
}
