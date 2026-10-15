package fleet

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/gateway"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/mission"
	"github.com/inov4d/fleet-manager/internal/planner"
	"github.com/inov4d/fleet-manager/internal/traffic"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

// execution is the runtime of one assigned mission on one vehicle.
type execution struct {
	missionID        string
	vehicleID        string
	siteID           string
	priority         int
	route            []string
	routeIdx         int   // index of the last node reached
	stepAt           []int // route index at which each step executes
	orderID          string
	updateID         int
	sentThrough      int // last released route index sent (-1 none)
	waitingSince     time.Time
	replans          int
	lastStateSeq     int
	pausedByOperator bool
	pickedUp         bool
	dropped          bool
	chargeDone       bool
	startedAt        time.Time
	lastProgressAt   time.Time
	forcedAvoidNodes []string
	forcedAvoidEdges []string
	replanRequested  bool
	deadlockID       string
	isCharge         bool
	isReturn         bool
	lastOrderAt      time.Time
	// yielding: after backing off, do not re-enter the contested area until
	// the other vehicle has moved on (or the yield expires)
	yieldTo       string
	yieldFromNode string
	yieldIdx      int
	yieldUntil    time.Time
	yieldPeers    []string // other vehicles of the deadlock cycle
}

// holdersMoving reports whether every holder of a denied resource is driving.
func holdersMoving(c *Core, holders []string) bool {
	if len(holders) == 0 {
		return false
	}
	for _, h := range holders {
		r := c.rt[h]
		if r == nil || !r.state.Driving {
			return false
		}
	}
	return true
}

func appendUnique(list []string, items ...string) []string {
	for _, it := range items {
		found := false
		for _, l := range list {
			if l == it {
				found = true
				break
			}
		}
		if !found {
			list = append(list, it)
		}
	}
	return list
}

// loadOnBoard reports whether the mission load may be on the vehicle.
func (e *execution) loadOnBoard(st core.VehicleState) bool {
	if st.LoadPresent {
		return true
	}
	return e.pickedUp && !e.dropped
}

func actionID(missionID string, seq int) string { return missionID + ":" + fmt.Sprint(seq) }

// routeActions converts mission steps into VDA 5050 node actions.
func routeActions(m *core.Mission, ex *execution) []gateway.RouteAction {
	var out []gateway.RouteAction
	for i, s := range m.Steps {
		if s.Action == core.StepMove || ex.stepAt[i] < 0 {
			continue
		}
		a := vda5050.Action{ActionID: actionID(m.ID, s.Seq), BlockingType: vda5050.BlockingHard, ActionParameters: []vda5050.ActionParameter{}}
		switch s.Action {
		case core.StepPick:
			a.ActionType = vda5050.ActionPick
			a.ActionParameters = append(a.ActionParameters, vda5050.ActionParameter{Key: "weight", Value: m.Load.MassKg}, vda5050.ActionParameter{Key: "loadId", Value: m.ID})
		case core.StepDrop:
			a.ActionType = vda5050.ActionDrop
		case core.StepWait:
			a.ActionType = vda5050.ActionWaitForTrigger
			d := s.DurationS
			if d <= 0 {
				d = 5
			}
			a.ActionParameters = append(a.ActionParameters, vda5050.ActionParameter{Key: "duration", Value: d})
		case core.StepCharge:
			a.ActionType = vda5050.ActionStartCharging
		}
		out = append(out, gateway.RouteAction{Index: ex.stepAt[i], Action: a})
	}
	return out
}

// planMission computes the full route for a mission from a start node and the
// route index of each step.
func (c *Core) planMission(g *mapmgr.Graph, m *core.Mission, start string, env core.Footprint, mass, maxSpeed float64, avoidNodes, avoidEdges []string) (*planner.Route, []int, error) {
	nodes := []string{start}
	for _, s := range m.Steps {
		if s.Status == core.StepDone {
			continue
		}
		if nodes[len(nodes)-1] != s.NodeID {
			nodes = append(nodes, s.NodeID)
		}
	}
	req := planner.Request{Graph: g, Envelope: env, MassKg: mass, MaxSpeedMs: maxSpeed, AvoidZones: m.Constraints.AvoidZoneIDs, AvoidNodes: avoidNodes, AvoidEdges: avoidEdges,
		Congestion: c.traffic.Congestion(m.SiteID), DirectedCost: c.intentCost(m.SiteID, m.VehicleID)}
	var route *planner.Route
	var err error
	if len(nodes) == 1 {
		route = &planner.Route{NodeIDs: []string{start}, EdgeIDs: []string{}, Costs: map[string]float64{}}
	} else {
		route, err = planner.PlanThrough(c.planner, req, nodes)
		if err != nil {
			return nil, nil, err
		}
	}
	// step positions: walk the route; each pending step binds to the first
	// occurrence of its node after the previous step's position
	stepAt := make([]int, len(m.Steps))
	pos := 0
	for i, s := range m.Steps {
		stepAt[i] = -1
		if s.Status == core.StepDone {
			continue
		}
		for j := pos; j < len(route.NodeIDs); j++ {
			if route.NodeIDs[j] == s.NodeID {
				stepAt[i] = j
				pos = j
				break
			}
		}
		if stepAt[i] < 0 {
			return nil, nil, core.Conflict("step %d node %s not on route", i, s.NodeID)
		}
	}
	return route, stepAt, nil
}

// intentCost returns a directed edge cost derived from the declared
// intentions of the other vehicles (their next route segments): driving
// against another vehicle's intent on a bidirectional edge is the classic
// head-on standoff, so it is strongly discouraged; following in the same
// direction costs a little.
func (c *Core) intentCost(siteID, except string) func(edgeID, from, to string) float64 {
	g := c.graphs[siteID]
	if g == nil {
		return nil
	}
	type intent struct{ fromTo, toFrom int } // traversals in each direction of the stored edge
	intents := map[string]*intent{}
	for vid, ex := range c.execs {
		if vid == except || ex.siteID != siteID {
			continue
		}
		for i := ex.routeIdx; i+1 < len(ex.route) && i < ex.routeIdx+6; i++ {
			e := g.g.EdgeBetween(ex.route[i], ex.route[i+1])
			if e == nil {
				continue
			}
			it := intents[e.ID]
			if it == nil {
				it = &intent{}
				intents[e.ID] = it
			}
			if e.From == ex.route[i] {
				it.fromTo++
			} else {
				it.toFrom++
			}
		}
	}
	return func(edgeID, from, to string) float64 {
		it := intents[edgeID]
		if it == nil {
			return 0
		}
		e := g.g.Edges[edgeID]
		if e == nil {
			return 0
		}
		same, opposite := it.fromTo, it.toFrom
		if e.From != from { // traversing the edge backwards
			same, opposite = it.toFrom, it.fromTo
		}
		return float64(same)*10 + float64(opposite)*80
	}
}

// candidates builds dispatcher contexts for the vehicles of a site.
func (c *Core) candidates(siteID string, pending []core.Mission) []mission.CandidateCtx {
	var out []mission.CandidateCtx
	g := c.graphs[siteID]
	for id, v := range c.vehicles {
		if v.SiteID != siteID {
			continue
		}
		r := c.rt[id]
		model := c.models[v.ModelID]
		cc := mission.CandidateCtx{VehicleID: id, SiteID: siteID, ModelID: v.ModelID, BatteryPct: r.state.BatteryCharge, MaxOverhangM: 1.0}
		if model != nil {
			cc.CapacityKg, cc.BodyLengthM, cc.BodyWidthM = model.MaxPayloadKg, model.LengthM, model.WidthM
		}
		if v.Footprint != nil {
			cc.BodyLengthM, cc.BodyWidthM = v.Footprint.LengthM, v.Footprint.WidthM
		}
		node := ""
		if g != nil {
			node = c.currentNode(v, r, g.g)
		}
		cc.CurrentNode = node
		switch {
		case g == nil:
			cc.Unavailable = "no active map"
		case r.state.Availability != core.AvailAvailable:
			cc.Unavailable = string(r.state.Availability)
		case r.state.OperatingMode != core.ModeAutomatic:
			cc.Unavailable = "operating mode " + string(r.state.OperatingMode)
		case !r.state.PositionInitialized:
			cc.Unavailable = "position not initialised"
		case node == "":
			cc.Unavailable = "position not on a map node"
		case r.state.LoadPresent:
			cc.Unavailable = "load on board"
		default:
			cc.Available = true
		}
		if cc.Available {
			vid, env, maxSpeed := id, r.state.LoadEnvelope, 0.0
			if model != nil {
				maxSpeed = model.MaxSpeedMs
			}
			cc.Route = func(m core.Mission) (float64, float64, float64, []string, bool) {
				mm := m
				mass := m.Load.MassKg
				route, _, err := c.planMission(g.g, &mm, node, env, mass, maxSpeed, nil, nil)
				if err != nil {
					return 0, 0, 0, nil, false
				}
				_ = vid
				return route.LengthM, route.EstimatedTimeS, route.Costs["congestion"], route.NodeIDs, true
			}
			cc.EnergyPct = func(d, mass float64) float64 {
				if model == nil || model.BatteryCapacityKwh <= 0 {
					return 0
				}
				return d * (model.EnergyKwhPerM + model.EnergyKwhPerTonM*mass/1000) / model.BatteryCapacityKwh * 100
			}
		}
		out = append(out, cc)
	}
	return out
}

// dispatchMissions assigns pending missions per site.
func (c *Core) dispatchMissions() {
	for sid := range c.sites {
		pending := c.missions.Pending(sid)
		if len(pending) == 0 || c.graphs[sid] == nil {
			continue
		}
		cands := c.candidates(sid, pending)
		// pinned missions (charging, parking, manual moves, operator reassignment) first
		var free []core.Mission
		taken := map[string]bool{}
		for _, m := range pending {
			vid, pinned := c.pins[m.ID]
			if !pinned {
				free = append(free, m)
				continue
			}
			var only []mission.CandidateCtx
			for _, cc := range cands {
				if cc.VehicleID == vid {
					only = append(only, cc)
				}
			}
			if len(only) == 0 {
				if c.vehicles[vid] == nil {
					delete(c.pins, m.ID) // vehicle gone: open the mission to the fleet
				}
				continue
			}
			for _, d := range mission.Dispatch([]core.Mission{m}, only, c.dispatch) {
				if c.startExecution(d) {
					taken[d.VehicleID] = true
					delete(c.pins, m.ID)
				}
			}
		}
		var rest []mission.CandidateCtx
		for _, cc := range cands {
			if !taken[cc.VehicleID] && !c.pinnedVehicle(cc.VehicleID) {
				rest = append(rest, cc)
			}
		}
		for _, d := range mission.Dispatch(free, rest, c.dispatch) {
			c.startExecution(d)
		}
	}
}

// pinnedVehicle reports whether a vehicle is reserved by a pending pinned mission.
func (c *Core) pinnedVehicle(vehicleID string) bool {
	for mid, vid := range c.pins {
		if vid == vehicleID {
			if m, err := c.missions.Get(mid); err == nil && m.Status == core.MissionPending {
				return true
			}
			delete(c.pins, mid)
		}
	}
	return false
}

func (c *Core) startExecution(d mission.Decision) bool {
	v := c.vehicles[d.VehicleID]
	if v == nil {
		return false
	}
	r := c.rt[d.VehicleID]
	g := c.graphs[v.SiteID]
	if g == nil || c.execs[d.VehicleID] != nil {
		return false
	}
	m, err := c.missions.Get(d.MissionID)
	if err != nil {
		return false
	}
	start := c.currentNode(v, r, g.g)
	model := c.models[v.ModelID]
	maxSpeed := 0.0
	if model != nil {
		maxSpeed = model.MaxSpeedMs
	}
	expl := d.Explanation
	route, stepAt, err := c.planMission(g.g, m, start, r.state.LoadEnvelope, m.Load.MassKg, maxSpeed, nil, nil)
	if err != nil {
		c.log.Warn("route planning failed at assignment", "mission", m.ID, "err", err)
		return false
	}
	if _, err := c.missions.Assign(m.ID, v.ID, &expl, route.NodeIDs); err != nil {
		return false
	}
	ex := &execution{missionID: m.ID, vehicleID: v.ID, siteID: v.SiteID, priority: m.Priority, route: route.NodeIDs, stepAt: stepAt, sentThrough: -1,
		startedAt: c.clock.Now(), lastProgressAt: c.clock.Now(), isCharge: m.Type == core.MissionCharge, isReturn: m.Type == core.MissionReturnToStation}
	c.execs[v.ID] = ex
	_, _ = c.missions.Start(m.ID)
	c.log.Info("mission assigned", "mission", m.ID, "vehicle", v.Name, "route", len(route.NodeIDs))
	return true
}

// stepExecution advances one execution.
func (c *Core) stepExecution(vid string, now time.Time) {
	ex := c.execs[vid]
	v := c.vehicles[vid]
	r := c.rt[vid]
	g := c.graphs[v.SiteID]
	if g == nil {
		return
	}
	m, err := c.missions.Get(ex.missionID)
	if err != nil || m.Status.Terminal() {
		c.endExecution(vid, true)
		return
	}
	// vehicle gone offline
	if r.state.Connection != core.ConnOnline {
		if m.Status == core.MissionExecuting {
			_, _ = c.missions.Suspend(m.ID, "communication lost with "+v.Name)
		}
		if now.Sub(r.offlineSince) > c.cfg.ReassignAfter {
			c.withdrawVehicle(vid, "vehicle offline for too long")
		}
		return
	}
	if m.Status == core.MissionSuspended && !ex.pausedByOperator && !m.LoadOnBoardUnverified {
		_, _ = c.missions.Resume(m.ID)
		m.Status = core.MissionExecuting
		// the vehicle kept (or lost) its order during the outage: the re-base
		// logic below detects a lost order from the reported orderId
	}
	if m.Status != core.MissionExecuting {
		return
	}
	if r.state.Availability == core.AvailError || r.maintenance || !v.Enabled {
		return // alarms already raised; operator decides
	}
	// progress from the vehicle's reported node (sequenceId = 2 * route index)
	if r.state.OrderID == ex.orderID && ex.orderID != "" {
		idx := r.state.LastNodeSequenceID / 2
		if idx > ex.routeIdx && idx < len(ex.route) && ex.route[idx] == r.state.LastNodeID {
			ex.routeIdx = idx
			ex.lastProgressAt = now
		}
	}
	// step completion
	allDone := true
	for i := range m.Steps {
		s := &m.Steps[i]
		if s.Status == core.StepDone {
			continue
		}
		at := ex.stepAt[i]
		if at < 0 || ex.routeIdx < at {
			allDone = false
			break
		}
		switch s.Action {
		case core.StepMove:
			c.completeStep(m, i, ex)
		case core.StepCharge:
			if !ex.chargeDone {
				if st := actionStatus(r.state, actionID(m.ID, s.Seq)); st == "FINISHED" || r.state.Charging {
					if r.state.BatteryCharge >= c.battery.TargetPct {
						_ = c.gw.StopCharging(vid)
						ex.chargeDone = true
						c.completeStep(m, i, ex)
					} else {
						allDone = false
					}
				} else if st == "FAILED" {
					c.failExecution(vid, m, "charging action failed")
					return
				} else {
					allDone = false
				}
			}
		default:
			st := actionStatus(r.state, actionID(m.ID, s.Seq))
			switch st {
			case "FINISHED":
				if s.Action == core.StepPick {
					ex.pickedUp = true
				}
				if s.Action == core.StepDrop {
					ex.dropped = true
				}
				c.completeStep(m, i, ex)
			case "FAILED":
				c.failExecution(vid, m, "action "+string(s.Action)+" failed on "+v.Name)
				return
			default:
				allDone = false
			}
		}
		if m.Steps[i].Status != core.StepDone {
			allDone = false
			break
		}
	}
	progress := 0.0
	if len(ex.route) > 1 {
		progress = float64(ex.routeIdx) / float64(len(ex.route)-1)
	}
	if math.Abs(progress-m.Progress) > 0.01 {
		_, _ = c.missions.Update(m.ID, func(mm *core.Mission) (string, error) { mm.Progress = progress; return "", nil })
	}
	if allDone && ex.routeIdx >= len(ex.route)-1 {
		c.completeExecution(vid, m)
		return
	}
	// traffic reservations for the remaining route
	cur := c.currentNode(v, r, g.g)
	routeNode := g.g.Nodes[ex.route[ex.routeIdx]]
	offRoute := false
	if cur != "" && !r.state.Driving && routeNode != nil && routeNode.Point().Dist(core.Point{X: r.state.Pose.X, Y: r.state.Pose.Y}) > 3.0 {
		// vehicle clearly stands somewhere else (manual move / relocalisation): replan from here
		offRoute = true
	}
	if !offRoute && cur != "" {
		cur = ex.route[ex.routeIdx]
	}
	atBaseEnd := !r.state.Driving && ex.routeIdx >= ex.sentThrough && cur != "" && (ex.orderID == "" || r.state.OrderID == ex.orderID)
	if offRoute && (ex.orderID == "" || r.state.OrderID == ex.orderID) {
		c.log.Warn("vehicle off its route, replanning", "vehicle", v.Name, "expected", ex.route[ex.routeIdx], "at", cur)
		c.traffic.ClearRoute(vid)
		if !c.replanExecution(vid, m, cur) {
			return
		}
		atBaseEnd = false
	}
	if ex.replanRequested && atBaseEnd {
		switch {
		case c.replanExecution(vid, m, cur):
			ex.replanRequested = false
		case ex.deadlockID != "" && c.backoff(vid, m, cur):
			ex.replanRequested = false
		case ex.deadlockID != "":
			c.traffic.ResolveDeadlock(ex.deadlockID, "no alternative route; "+v.Name+" waits")
			ex.deadlockID = ""
			ex.replanRequested = false
			ex.forcedAvoidNodes, ex.forcedAvoidEdges = nil, nil
		default:
			ex.replanRequested = false
		}
	}
	// an order the vehicle did not take over (rejected or lost): re-base a new
	// order on the node where the vehicle actually stands
	if ex.orderID != "" && r.state.OrderID != ex.orderID && now.Sub(ex.lastOrderAt) > 5*time.Second && !r.state.Driving && cur != "" {
		idx := -1
		for i := ex.routeIdx; i < len(ex.route); i++ {
			if ex.route[i] == cur {
				idx = i
				break
			}
		}
		c.log.Warn("vehicle did not take the order, re-basing", "vehicle", v.Name, "reported", r.state.OrderID, "expected", ex.orderID, "node", cur)
		if idx >= 0 {
			ex.routeIdx = idx
			c.traffic.ClearRoute(vid)
			ex.orderID, ex.sentThrough = "", -1
		} else if !c.replanExecution(vid, m, cur) {
			ex.lastOrderAt = now
		}
	}
	ctx := traffic.VehicleCtx{ID: vid, SiteID: v.SiteID, Pose: r.state.Pose, Envelope: r.state.LoadEnvelope, Priority: m.Priority, CurrentNode: ex.route[ex.routeIdx], Connected: true, RouteIndex: ex.routeIdx}
	res := c.traffic.Request(ctx, ex.route)
	granted := res.GrantedThrough
	if traffic.Strict {
		if gt := c.traffic.GrantedThrough(vid); gt != res.GrantedThrough && res.GrantedThrough > ex.routeIdx {
			panic(fmt.Sprintf("traffic result %d but GrantedThrough() %d for %s (denied=%+v) sentThrough=%d orderID=%q", res.GrantedThrough, gt, v.Name, res.Denied, ex.sentThrough, ex.orderID))
		}
	}
	// a blocking action on an intermediate node: do not release beyond it until done
	for i, s := range m.Steps {
		if s.Status != core.StepDone && s.Action != core.StepMove && ex.stepAt[i] > ex.routeIdx && ex.stepAt[i] < granted {
			granted = ex.stepAt[i]
		}
	}
	// yielding after a back-off: hold at the back-off node until the peer moved
	if ex.yieldTo != "" {
		peer := c.rt[ex.yieldTo]
		moved := peer == nil || peer.state.LastNodeID != ex.yieldFromNode || c.execs[ex.yieldTo] == nil
		if moved || now.After(ex.yieldUntil) {
			ex.yieldTo = ""
		} else if granted > ex.yieldIdx && ex.yieldIdx >= ex.routeIdx {
			granted = ex.yieldIdx
		}
	}
	if res.Denied != nil && res.GrantedThrough <= ex.routeIdx {
		if ex.waitingSince.IsZero() {
			ex.waitingSince = now
		}
		// replan quickly when the holder stands still (it may wait on us or be
		// parked), but give a moving holder time to clear the resource
		patience := c.cfg.ReplanAfter
		if holdersMoving(c, res.Denied.HeldBy) {
			patience = 4 * c.cfg.ReplanAfter
		}
		if now.Sub(ex.waitingSince) > patience && atBaseEnd {
			// accumulate what blocked us so successive replans route further away
			switch res.Denied.Resource.Type {
			case core.ResNode:
				ex.forcedAvoidNodes = appendUnique(ex.forcedAvoidNodes, res.Denied.Resource.ID)
			case core.ResEdge:
				ex.forcedAvoidEdges = appendUnique(ex.forcedAvoidEdges, res.Denied.Resource.ID)
			}
			if c.replanExecution(vid, m, cur) {
				ex.waitingSince = time.Time{}
				granted = ex.routeIdx // the new route has no grants yet: next tick requests them
			} else {
				ex.waitingSince = now // no alternative yet: keep waiting, keep the avoid set
			}
		}
	} else {
		ex.waitingSince = time.Time{}
		if res.GrantedThrough > ex.routeIdx {
			ex.forcedAvoidNodes, ex.forcedAvoidEdges = nil, nil
		}
	}
	if granted > ex.sentThrough || ex.orderID == "" {
		c.sendOrder(vid, m, ex, g.g, granted)
	}
}

func (c *Core) completeStep(m *core.Mission, i int, ex *execution) {
	now := c.clock.Now()
	m.Steps[i].Status = core.StepDone
	m.Steps[i].CompletedAt = &now
	_, _ = c.missions.Update(m.ID, func(mm *core.Mission) (string, error) {
		if i < len(mm.Steps) {
			mm.Steps[i].Status = core.StepDone
			mm.Steps[i].CompletedAt = &now
			if mm.Steps[i].StartedAt == nil {
				mm.Steps[i].StartedAt = &now
			}
		}
		return "mission.updated", nil
	})
}

func actionStatus(st core.VehicleState, id string) string {
	for _, a := range st.ActionStates {
		if a.ActionID == id {
			return a.ActionStatus
		}
	}
	return ""
}

func (c *Core) sendOrder(vid string, m *core.Mission, ex *execution, g *mapmgr.Graph, releasedThrough int) {
	r := c.rt[vid]
	v := c.vehicles[vid]
	model := c.models[v.ModelID]
	maxSpeed := 0.0
	if model != nil {
		maxSpeed = model.MaxSpeedMs
	}
	if m.Constraints.MaxSpeedMs != nil && *m.Constraints.MaxSpeedMs > 0 && (*m.Constraints.MaxSpeedMs < maxSpeed || maxSpeed == 0) {
		maxSpeed = *m.Constraints.MaxSpeedMs
	}
	mapID := ""
	if ge := c.graphs[v.SiteID]; ge != nil {
		mapID = ge.m.ID
	}
	startIdx := 0
	if ex.orderID == "" {
		ex.orderID = "ord-" + uuid.NewString()[:8]
		ex.updateID = 0
		startIdx = ex.routeIdx
	} else {
		ex.updateID++
		startIdx = ex.sentThrough
		if startIdx < ex.routeIdx {
			startIdx = ex.routeIdx
		}
	}
	ex.lastOrderAt = c.clock.Now()
	if releasedThrough < startIdx {
		releasedThrough = startIdx
	}
	if traffic.Strict && releasedThrough > ex.routeIdx {
		if gt := c.traffic.GrantedThrough(vid); releasedThrough > gt {
			panic(fmt.Sprintf("core: releasing %s through %d (%s) but traffic granted through %d; route=%v routeIdx=%d", v.Name, releasedThrough, ex.route[releasedThrough], gt, ex.route, ex.routeIdx))
		}
	}
	o := gateway.BuildOrder(g, mapID, ex.orderID, ex.updateID, ex.route, startIdx, releasedThrough, routeActions(m, ex), maxSpeed, r.state.Pose.Theta)
	if err := c.gw.SendOrder(vid, o); err != nil {
		c.log.Warn("order send failed", "vehicle", vid, "err", err)
		return
	}
	ex.sentThrough = releasedThrough
}

// replanExecution recomputes the remaining route from the current node.
func (c *Core) replanExecution(vid string, m *core.Mission, cur string) bool {
	ex := c.execs[vid]
	v := c.vehicles[vid]
	r := c.rt[vid]
	g := c.graphs[v.SiteID]
	if g == nil || cur == "" {
		return false
	}
	model := c.models[v.ModelID]
	maxSpeed := 0.0
	if model != nil {
		maxSpeed = model.MaxSpeedMs
	}
	// first try to route around every vehicle currently standing still, then
	// fall back to the forced avoid set only (congestion costs still apply)
	occupied := c.traffic.OccupiedNodes(v.SiteID, vid)
	avoidN := appendUnique(append([]string{}, ex.forcedAvoidNodes...), occupied...)
	route, stepAt, err := c.planMission(g.g, m, cur, r.state.LoadEnvelope, m.Load.MassKg, maxSpeed, avoidN, ex.forcedAvoidEdges)
	if err != nil {
		route, stepAt, err = c.planMission(g.g, m, cur, r.state.LoadEnvelope, m.Load.MassKg, maxSpeed, ex.forcedAvoidNodes, ex.forcedAvoidEdges)
	}
	if err != nil {
		return false
	}
	same := len(route.NodeIDs) == len(ex.route)-ex.routeIdx
	if same {
		for i, n := range route.NodeIDs {
			if ex.route[ex.routeIdx+i] != n {
				same = false
				break
			}
		}
	}
	if same {
		return false
	}
	// a detour only pays off when it is not much longer than the remaining
	// route (otherwise waiting for the resource to clear is cheaper)
	if ex.deadlockID == "" {
		remaining := 0.0
		for i := ex.routeIdx; i+1 < len(ex.route); i++ {
			if e := g.g.EdgeBetween(ex.route[i], ex.route[i+1]); e != nil {
				remaining += g.g.Length(e)
			}
		}
		if route.LengthM > 1.5*remaining+30 {
			return false
		}
	}
	c.traffic.ClearRoute(vid)
	ex.route = route.NodeIDs
	ex.routeIdx = 0
	ex.stepAt = stepAt
	ex.replans++
	// a new order (VDA 5050: a different orderId starting at the current node)
	ex.orderID = ""
	ex.sentThrough = -1
	_, _ = c.missions.Update(m.ID, func(mm *core.Mission) (string, error) {
		mm.RouteNodeIDs = route.NodeIDs
		mm.History = append(mm.History, core.MissionEvent{At: c.clock.Now(), Status: mm.Status, Message: fmt.Sprintf("replanned from %s (%d nodes)", cur, len(route.NodeIDs))})
		return "mission.updated", nil
	})
	if ex.deadlockID != "" {
		c.traffic.ResolveDeadlock(ex.deadlockID, "vehicle "+v.Name+" replanned")
		c.emit(core.Event{Kind: "deadlock.resolved", SiteID: v.SiteID, VehicleID: vid, Message: v.Name + " a été replanifié"})
		ex.deadlockID = ""
	}
	return true
}

func (c *Core) completeExecution(vid string, m *core.Mission) {
	v := c.vehicles[vid]
	r := c.rt[vid]
	if _, err := c.missions.Complete(m.ID); err == nil {
		r.counters.MissionsCompleted++
		done, _ := c.missions.Get(m.ID)
		if done != nil {
			c.analytics.RecordMission(*done, 0)
		}
		c.log.Info("mission completed", "mission", m.ID, "vehicle", v.Name)
	}
	c.endExecution(vid, false)
}

func (c *Core) failExecution(vid string, m *core.Mission, reason string) {
	v := c.vehicles[vid]
	_, _ = c.missions.Fail(m.ID, reason)
	if done, _ := c.missions.Get(m.ID); done != nil {
		c.analytics.RecordMission(*done, 0)
	}
	c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevError, Code: "MISSION_FAILED", Source: core.AlarmSrcMission, VehicleID: vid, MissionID: m.ID, Message: reason})
	c.endExecution(vid, true)
}

func (c *Core) handleDeadlock(res traffic.Resolution) {
	ex := c.execs[res.VehicleID]
	v := c.vehicles[res.VehicleID]
	if v == nil {
		return
	}
	c.emit(core.Event{Kind: "deadlock.detected", SiteID: v.SiteID, VehicleID: res.VehicleID, Message: "interblocage détecté, " + v.Name + " va être replanifié"})
	c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevWarning, Code: "DEADLOCK", Source: core.AlarmSrcTraffic, VehicleID: res.VehicleID, Message: "Interblocage: " + v.Name + " replanifié"})
	if ex == nil {
		c.traffic.ResolveDeadlock(res.DeadlockID, "vehicle idle, reservations cleared")
		c.traffic.ClearRoute(res.VehicleID)
		return
	}
	ex.forcedAvoidNodes = append(ex.forcedAvoidNodes, res.AvoidNodes...)
	ex.forcedAvoidEdges = append(ex.forcedAvoidEdges, res.AvoidEdges...)
	ex.replanRequested = true
	ex.deadlockID = res.DeadlockID
	ex.yieldPeers = nil
	for _, d := range c.traffic.Deadlocks(v.SiteID) {
		if d.ID == res.DeadlockID {
			for _, o := range d.VehicleIDs {
				if o != res.VehicleID {
					ex.yieldPeers = append(ex.yieldPeers, o)
				}
			}
		}
	}
	// the replan itself happens in stepExecution once the vehicle stands still
	// at the end of its released base (a new order must start where the
	// vehicle is, never between two nodes)
}

// backoff inserts a detour to a free waiting node before continuing.
func (c *Core) backoff(vid string, m *core.Mission, cur string) bool {
	v := c.vehicles[vid]
	r := c.rt[vid]
	ex := c.execs[vid]
	g := c.graphs[v.SiteID]
	if g == nil || cur == "" {
		return false
	}
	occupied := map[string]bool{}
	for _, res := range c.traffic.Reservations(v.SiteID) {
		if res.Resource.Type == core.ResNode && res.VehicleID != vid {
			occupied[res.Resource.ID] = true
		}
	}
	best := ""
	bestLen := math.Inf(1)
	var bestRoute []string
	avoidN := append([]string{}, ex.forcedAvoidNodes...)
	for n := range occupied {
		avoidN = append(avoidN, n)
	}
	// stay off the next nodes of the vehicles we are yielding to
	for _, oid := range ex.yieldPeers {
		if oex := c.execs[oid]; oex != nil {
			for i := oex.routeIdx; i < len(oex.route) && i < oex.routeIdx+4; i++ {
				avoidN = append(avoidN, oex.route[i])
				occupied[oex.route[i]] = true
			}
		}
	}
	skip := map[string]bool{}
	for _, an := range avoidN {
		skip[an] = true
	}
	for _, n := range g.m.Nodes {
		if (n.Type != core.NodeWaiting && n.Type != core.NodeParking && n.Type != core.NodeWaypoint) || occupied[n.ID] || skip[n.ID] || n.ID == cur {
			continue
		}
		rt, err := c.planner.Plan(planner.Request{Graph: g.g, From: cur, To: n.ID, Envelope: r.state.LoadEnvelope, AvoidNodes: avoidN, AvoidEdges: ex.forcedAvoidEdges, Congestion: c.traffic.Congestion(v.SiteID)})
		if err != nil || len(rt.NodeIDs) > 4 {
			continue
		}
		if rt.LengthM < bestLen {
			bestLen, best, bestRoute = rt.LengthM, n.ID, rt.NodeIDs
		}
	}
	if best == "" {
		return false
	}
	// detour then the original remaining route from the detour node
	rest, stepAt, err := c.planMission(g.g, m, best, r.state.LoadEnvelope, m.Load.MassKg, 0, nil, nil)
	if err != nil {
		return false
	}
	full := append(append([]string{}, bestRoute...), rest.NodeIDs[1:]...)
	for i := range stepAt {
		if stepAt[i] >= 0 {
			stepAt[i] += len(bestRoute) - 1
		}
	}
	c.traffic.ClearRoute(vid)
	ex.route, ex.routeIdx, ex.stepAt = full, 0, stepAt
	ex.orderID, ex.sentThrough = "", -1
	ex.replans++
	// yield: hold at the back-off node until the first peer has moved away
	ex.yieldTo, ex.yieldFromNode, ex.yieldIdx, ex.yieldUntil = "", "", len(bestRoute)-1, c.clock.Now().Add(60*time.Second)
	for _, oid := range ex.yieldPeers {
		if orx := c.rt[oid]; orx != nil {
			ex.yieldTo, ex.yieldFromNode = oid, orx.state.LastNodeID
			break
		}
	}
	_, _ = c.missions.Update(m.ID, func(mm *core.Mission) (string, error) {
		mm.RouteNodeIDs = full
		mm.History = append(mm.History, core.MissionEvent{At: c.clock.Now(), Status: mm.Status, Message: "backing off to " + best + " to resolve a deadlock"})
		return "mission.updated", nil
	})
	if ex.deadlockID != "" {
		c.traffic.ResolveDeadlock(ex.deadlockID, "vehicle "+v.Name+" backs off to "+best)
		c.emit(core.Event{Kind: "deadlock.resolved", SiteID: v.SiteID, VehicleID: vid, Message: v.Name + " recule vers " + best})
		ex.deadlockID = ""
	}
	return true
}

func (c *Core) handleBlocked(now time.Time) {
	blocked := map[string]core.BlockedVehicle{}
	for _, b := range c.traffic.Blocked() {
		blocked[b.VehicleID] = b
	}
	for id, v := range c.vehicles {
		r := c.rt[id]
		b, isBlocked := blocked[id]
		if isBlocked && c.execs[id] != nil {
			if !r.blocked {
				r.blocked = true
				r.blockedSince = now
				c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevWarning, Code: "VEHICLE_BLOCKED", Source: core.AlarmSrcTraffic, VehicleID: id, Message: v.Name + " bloqué: " + b.Reason + " " + b.HeldBy})
			}
		} else if r.blocked {
			r.blocked = false
			c.alarms.ClearByKey(core.AlarmSrcTraffic, "VEHICLE_BLOCKED", id, "")
		}
	}
}

// BlockedVehicles exposes the traffic manager's blocked list.
func (c *Core) BlockedVehicles() []core.BlockedVehicle { return c.traffic.Blocked() }

// CreateMission validates against the active map and queues a mission.
func (c *Core) CreateMission(m core.Mission) (*core.Mission, error) {
	c.mu.RLock()
	if m.SiteID == "" {
		m.SiteID = core.DefaultSiteID
	}
	g := c.graphs[m.SiteID]
	c.mu.RUnlock()
	if g == nil {
		return nil, core.Conflict("site %s has no active map", m.SiteID)
	}
	if m.RequiredModelID != "" && c.models[m.RequiredModelID] == nil {
		return nil, core.Validation("unknown model %q", m.RequiredModelID)
	}
	return c.missions.Create(m, func(n string) bool { return g.g.Nodes[n] != nil })
}

// CancelMission cancels a mission wherever it is.
func (c *Core) CancelMission(id, by string) (*core.Mission, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, err := c.missions.Get(id)
	if err != nil {
		return nil, err
	}
	if m.VehicleID != "" && c.execs[m.VehicleID] != nil && c.execs[m.VehicleID].missionID == id {
		if err := c.cancelExecution(m.VehicleID, "cancelled by "+by); err != nil {
			return nil, err
		}
		return c.missions.Get(id)
	}
	return c.missions.Cancel(id, by)
}

// SuspendMission pauses a mission (and its vehicle).
func (c *Core) SuspendMission(id, by string) (*core.Mission, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, err := c.missions.Suspend(id, "suspended by "+by)
	if err != nil {
		return nil, err
	}
	if ex := c.execs[m.VehicleID]; ex != nil && ex.missionID == id {
		ex.pausedByOperator = true
		_ = c.gw.Pause(m.VehicleID)
	}
	return m, nil
}

// ResumeMission resumes a suspended mission.
func (c *Core) ResumeMission(id string) (*core.Mission, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, err := c.missions.Get(id)
	if err != nil {
		return nil, err
	}
	if m.LoadOnBoardUnverified {
		return nil, core.Conflict("load state must be verified: use reassign with confirmLoadState")
	}
	m, err = c.missions.Resume(id)
	if err != nil {
		return nil, err
	}
	if ex := c.execs[m.VehicleID]; ex != nil && ex.missionID == id {
		ex.pausedByOperator = false
		_ = c.gw.Resume(m.VehicleID)
	}
	return m, nil
}

// ReassignMission returns a mission to the queue (optionally pinned to a vehicle).
func (c *Core) ReassignMission(id, vehicleID string, confirmLoad bool, by string) (*core.Mission, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, err := c.missions.Get(id)
	if err != nil {
		return nil, err
	}
	if vehicleID != "" && c.vehicles[vehicleID] == nil {
		return nil, core.NotFound("vehicle", vehicleID)
	}
	if ex := c.execs[m.VehicleID]; ex != nil && ex.missionID == id {
		if ex.loadOnBoard(c.rt[m.VehicleID].state) && !confirmLoad {
			return nil, core.Conflict("load may be on board of %s: physical state must be verified (confirmLoadState)", m.VehicleID)
		}
		c.endExecution(m.VehicleID, true)
	}
	m, err = c.missions.Unassign(id, "reassigned by "+by, confirmLoad)
	if err != nil {
		return nil, err
	}
	if vehicleID != "" {
		c.pinned(id, vehicleID)
	}
	return m, err
}

// pinned forces the next dispatch of a mission to one vehicle.
func (c *Core) pinned(missionID, vehicleID string) {
	if c.pins == nil {
		c.pins = map[string]string{}
	}
	c.pins[missionID] = vehicleID
}
