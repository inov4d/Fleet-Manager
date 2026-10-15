package fleet

import (
	"fmt"
	"sort"
	"time"

	"github.com/inov4d/fleet-manager/internal/colony"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/mission"
)

// arbiter adapts the core to the colony board.
type arbiter Core

func (a *arbiter) core() *Core { return (*Core)(a) }

// OpenTasks lists pending, unpinned missions with their urgency. In central
// mode the board is empty (the dispatcher assigns).
func (a *arbiter) OpenTasks(siteID string) []colony.Task {
	c := a.core()
	c.mu.RLock()
	mode := c.dispatch.Mode
	pinned := map[string]bool{}
	for mid := range c.pins {
		pinned[mid] = true
	}
	c.mu.RUnlock()
	if mode != core.AllocationColony {
		return nil
	}
	now := c.clock.Now()
	var out []colony.Task
	for _, m := range c.missions.Pending(siteID) {
		if pinned[m.ID] {
			continue
		}
		t := colony.Task{ID: m.ID, Type: string(m.Type), Origin: m.Origin, Destination: m.Destination, Priority: m.Priority, MassKg: m.Load.MassKg,
			LoadPresent: m.Load.Present, LoadLengthM: m.Load.LengthM, LoadWidthM: m.Load.WidthM, RequiredModelID: m.RequiredModelID, Deadline: m.Deadline, CreatedAt: m.CreatedAt}
		for _, s := range m.Steps {
			if s.Status != core.StepDone {
				t.Nodes = append(t.Nodes, s.NodeID)
			}
		}
		// urgency: priority plus age pressure (one point per minute waiting),
		// doubled past the deadline
		t.Urgency = float64(m.Priority) + now.Sub(m.CreatedAt).Minutes()
		if m.Deadline != nil && now.After(*m.Deadline) {
			t.Urgency *= 2
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Urgency > out[j].Urgency })
	return out
}

// Eligible runs the central checks for one vehicle / task pair.
func (a *arbiter) Eligible(siteID, vehicleID, taskID string) (string, bool) {
	c := a.core()
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, err := c.missions.Get(taskID)
	if err != nil {
		return "task not found", false
	}
	if m.Status != core.MissionPending {
		return "task already taken (" + string(m.Status) + ")", false
	}
	if p, ok := c.pins[taskID]; ok && p.vehicleID != vehicleID {
		return "task already granted to " + p.vehicleID, false
	}
	if c.vehicles[vehicleID] == nil {
		return "unknown vehicle", false
	}
	if c.execs[vehicleID] != nil || c.pinnedVehicle(vehicleID) {
		return "vehicle already has a task", false
	}
	for _, cc := range c.candidates(siteID, nil) {
		if cc.VehicleID != vehicleID {
			continue
		}
		dec := mission.Dispatch([]core.Mission{*m}, []mission.CandidateCtx{cc}, c.dispatch)
		if len(dec) == 1 {
			return "", true
		}
		if !cc.Available {
			return cc.Unavailable, false
		}
		return "not eligible (capacity, model, battery or route)", false
	}
	return "vehicle not on this site", false
}

// Grant pins the task to the vehicle with the claims as explanation.
func (a *arbiter) Grant(siteID, taskID, vehicleID string, claims []colony.Claim) error {
	c := a.core()
	c.mu.Lock()
	defer c.mu.Unlock()
	m, err := c.missions.Get(taskID)
	if err != nil {
		return err
	}
	if m.Status != core.MissionPending {
		return core.Conflict("task is %s", m.Status)
	}
	if p, ok := c.pins[taskID]; ok && p.vehicleID != vehicleID {
		return core.Conflict("task already granted")
	}
	expl := &core.AssignmentExplanation{ChosenVehicleID: vehicleID, Weights: map[string]float64{"mode": 1}}
	for _, cl := range claims {
		cand := core.Candidate{VehicleID: cl.VehicleID, Eligible: true, Score: cl.Cost, Reason: fmt.Sprintf("claimed with cost %.0f s", cl.Cost), Factors: core.CandidateFactors{TravelTimeS: cl.Cost}}
		if cl.VehicleID == vehicleID {
			cand.Reason = fmt.Sprintf("won the claim with cost %.0f s (%d competing)", cl.Cost, len(claims))
		}
		expl.Candidates = append(expl.Candidates, cand)
	}
	c.pinWith(taskID, vehicleID, expl)
	_, _ = c.missions.Update(taskID, func(mm *core.Mission) (string, error) {
		mm.History = append(mm.History, core.MissionEvent{At: c.clock.Now(), Status: mm.Status, Message: "claimed by " + vehicleID + " on the colony board"})
		return "mission.updated", nil
	})
	c.log.Info("colony claim granted", "task", taskID, "vehicle", vehicleID, "claims", len(claims))
	return nil
}

// RequestSystemTask creates a charge or park mission on behalf of an agent.
func (a *arbiter) RequestSystemTask(siteID, vehicleID, kind, reason string) error {
	c := a.core()
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.vehicles[vehicleID]
	if v == nil {
		return core.NotFound("vehicle", vehicleID)
	}
	r := c.rt[vehicleID]
	if c.execs[vehicleID] != nil || c.pinnedVehicle(vehicleID) {
		return core.Conflict("vehicle busy")
	}
	switch kind {
	case "charge":
		prio := 7
		if r.state.BatteryCharge <= c.battery.CriticalPct {
			prio = 9
		}
		_, err := c.createChargeMission(v, r, prio, "agent: "+reason)
		return err
	case "park":
		c.createParkMission(v, r)
		return nil
	}
	return core.Validation("unknown request %q", kind)
}

// Release gives a task back (load guard applies).
func (a *arbiter) Release(siteID, vehicleID, taskID, reason string) error {
	c := a.core()
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := c.pins[taskID]; ok && p.vehicleID == vehicleID {
		delete(c.pins, taskID)
		return nil
	}
	m, err := c.missions.Get(taskID)
	if err != nil {
		return err
	}
	if m.VehicleID != vehicleID {
		return core.Conflict("task not held by %s", vehicleID)
	}
	if ex := c.execs[vehicleID]; ex != nil && ex.missionID == taskID {
		if ex.loadOnBoard(c.rt[vehicleID].state) {
			return core.Conflict("load on board: a human must verify before the task changes hands")
		}
		c.endExecution(vehicleID, true)
	}
	_, err = c.missions.Unassign(taskID, "released by the agent: "+reason, false)
	return err
}

// Assignments reports what each vehicle of the site is doing.
func (a *arbiter) Assignments(siteID string) []colony.Assignment {
	c := a.core()
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := c.clock.Now()
	var out []colony.Assignment
	for id, v := range c.vehicles {
		if v.SiteID != siteID {
			continue
		}
		as := colony.Assignment{VehicleID: id, Status: "idle", TS: now}
		if ex := c.execs[id]; ex != nil {
			as.TaskID, as.Status = ex.missionID, "executing"
		} else {
			for mid, p := range c.pins {
				if p.vehicleID == id {
					as.TaskID, as.Status = mid, "assigned"
				}
			}
		}
		out = append(out, as)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VehicleID < out[j].VehicleID })
	return out
}

// ActiveMap returns the site's active map.
func (a *arbiter) ActiveMap(siteID string) (*core.Map, bool) {
	c := a.core()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if ge := c.graphs[siteID]; ge != nil {
		return ge.m, true
	}
	return nil, false
}

// VehicleInfos lists the registry entries of the site.
func (a *arbiter) VehicleInfos(siteID string) []colony.VehicleInfo {
	c := a.core()
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []colony.VehicleInfo
	for id, v := range c.vehicles {
		if v.SiteID != siteID {
			continue
		}
		vi := colony.VehicleInfo{ID: id, SiteID: siteID, ModelID: v.ModelID, Enabled: v.Enabled}
		if m := c.models[v.ModelID]; m != nil {
			vi.Model = *m
			if v.Footprint != nil {
				vi.Model.LengthM, vi.Model.WidthM = v.Footprint.LengthM, v.Footprint.WidthM
			}
		}
		out = append(out, vi)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ColonyStatus is the API view of the colony mode.
type ColonyStatus struct {
	Mode   core.AllocationMode `json:"mode"`
	Tasks  []colony.Task       `json:"tasks"`
	Agents []colony.Snapshot   `json:"agents"`
	Stats  colony.Stats        `json:"stats"`
}

// Colony returns the board state for a site.
func (c *Core) Colony(siteID string) ColonyStatus {
	st := ColonyStatus{Mode: c.DispatcherSettings().Mode, Tasks: (*arbiter)(c).OpenTasks(siteID), Stats: c.board.Stats(), Agents: []colony.Snapshot{}}
	if st.Tasks == nil {
		st.Tasks = []colony.Task{}
	}
	c.mu.RLock()
	ids := make([]string, 0, len(c.agents))
	for id := range c.agents {
		if v := c.vehicles[id]; v != nil && (siteID == "" || v.SiteID == siteID) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	agents := make([]*colony.Agent, 0, len(ids))
	for _, id := range ids {
		agents = append(agents, c.agents[id])
	}
	c.mu.RUnlock()
	for _, ag := range agents {
		st.Agents = append(st.Agents, ag.Snapshot())
	}
	return st
}

var _ = time.Second
