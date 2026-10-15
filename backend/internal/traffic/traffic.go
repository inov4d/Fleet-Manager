// Package traffic coordinates up to dozens of vehicles on a shared map through
// spatial resource reservations (nodes, edges, zones) combined with geometric
// swept-envelope checks of omnidirectional manoeuvres. It also detects blocked
// vehicles and deadlocks and proposes resolutions.
//
// Safety note: reservations are a traffic coordination mechanism, not a safety
// function. Collision avoidance remains the responsibility of the on-board
// safety controller; this package only decides which vehicle may be sent where.
package traffic

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/geometry"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
)

// VehicleCtx is what the manager needs to know about a vehicle at each call.
type VehicleCtx struct {
	ID          string
	SiteID      string
	Pose        core.Pose
	Envelope    core.Footprint // effective envelope incl. load and margins
	Priority    int
	CurrentNode string // node the vehicle stands on / last reached
	Connected   bool
	// RouteIndex is the index of CurrentNode in the route passed to Request
	// (routes may visit a node twice). Negative when the vehicle has no route.
	RouteIndex int
}

// Denial explains why the next segment could not be granted.
type Denial struct {
	Resource core.ResourceRef `json:"resource"`
	HeldBy   []string         `json:"heldBy"`
	Reason   string           `json:"reason"`
}

// GrantResult is the outcome of a reservation request.
type GrantResult struct {
	// GrantedThrough is the index in the route of the last node the vehicle
	// may travel to (equal to the current index = may not leave its node).
	GrantedThrough int
	Denied         *Denial
}

// Resolution is a deadlock resolution instruction for the fleet core.
type Resolution struct {
	DeadlockID string
	VehicleID  string
	Action     string // "replan" | "backoff"
	AvoidEdges []string
	AvoidNodes []string
}

type segment struct {
	index    int // route index of the destination node
	from, to string
	edgeID   string
	heading  float64
	envelope geometry.Polygon
	resIDs   []string
}

type hold struct {
	ctx            VehicleCtx
	segments       []*segment
	standingIDs    []string // reservations for the node/zones under the body
	waitingFor     *core.ResourceRef
	waitingOn      []string
	waitingSince   time.Time
	lastProgressAt time.Time
	lastPose       core.Pose
	uncertain      bool
	uncertainSince time.Time
	hasPose        bool
	bodyNow        geometry.Polygon
}

// Trace, when set, receives a line for every grant / release (debugging).
var Trace func(string)

// Strict enables internal consistency assertions (tests).
var Strict bool

func trace(format string, a ...any) {
	if Trace != nil {
		Trace(fmt.Sprintf(format, a...))
	}
}

// Manager is the traffic manager.
type Manager struct {
	mu        sync.Mutex
	clock     core.Clock
	settings  core.TrafficSettings
	graphs    map[string]*mapmgr.Graph // per site
	resv      map[string]*core.Reservation
	byRes     map[string]map[string]bool // resource key -> reservation ids
	holds     map[string]*hold
	deadlocks []core.Deadlock
	resolved  int
	laneDir   map[string]map[string]float64 // single lane zone -> vehicle -> heading
	recent    map[string]*recentDeadlock    // cycle key -> last occurrence
}

type recentDeadlock struct {
	at       time.Time
	victims  map[string]bool
	deferred time.Time // no re-detection before this time (failed resolution)
	id       string
}

// New creates a manager.
func New(settings core.TrafficSettings, clock core.Clock) *Manager {
	return &Manager{clock: clock, settings: settings, graphs: map[string]*mapmgr.Graph{}, resv: map[string]*core.Reservation{},
		byRes: map[string]map[string]bool{}, holds: map[string]*hold{}, laneDir: map[string]map[string]float64{}, recent: map[string]*recentDeadlock{}}
}

// Settings returns the current settings.
func (m *Manager) Settings() core.TrafficSettings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings
}

// SetSettings replaces the settings.
func (m *Manager) SetSettings(s core.TrafficSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.LookaheadSegments <= 0 {
		s.LookaheadSegments = 3
	}
	if s.Swept.Samples < 2 {
		s.Swept.Samples = 6
	}
	m.settings = s
}

// SetGraph installs the active map graph of a site. Existing reservations on
// resources that no longer exist are released.
func (m *Manager) SetGraph(siteID string, g *mapmgr.Graph) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.graphs[siteID] = g
	for _, r := range m.resv {
		if r.SiteID != siteID || r.State == core.ResvReleased {
			continue
		}
		exists := true
		switch r.Resource.Type {
		case core.ResNode:
			_, exists = g.Nodes[r.Resource.ID]
		case core.ResEdge:
			_, exists = g.Edges[r.Resource.ID]
		case core.ResZone:
			_, exists = g.Zones[r.Resource.ID]
		}
		if !exists {
			m.release(r)
		}
	}
	for _, h := range m.holds {
		if h.ctx.SiteID == siteID {
			for _, s := range h.segments {
				for _, id := range s.resIDs {
					if r := m.resv[id]; r != nil {
						m.release(r)
					}
				}
			}
			h.segments = nil
		}
	}
}

func (m *Manager) graph(siteID string) *mapmgr.Graph { return m.graphs[siteID] }

// Observe updates the vehicle position and releases the resources it has left.
// Resources are only released based on the vehicle's reported, trusted
// position; nothing is released on a timer.
func (m *Manager) Observe(ctx VehicleCtx) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	h := m.holds[ctx.ID]
	if h == nil {
		h = &hold{lastProgressAt: now}
		m.holds[ctx.ID] = h
	}
	prevNode := h.ctx.CurrentNode
	h.ctx = ctx
	// last known body: kept while disconnected so others keep their distance
	h.bodyNow = geometry.Rect(ctx.Pose, ctx.Envelope.LengthM, ctx.Envelope.WidthM)
	if !ctx.Connected {
		if !h.uncertain {
			h.uncertain = true
			h.uncertainSince = now
			for _, id := range m.vehicleResIDs(h) {
				if r := m.resv[id]; r != nil && r.State != core.ResvReleased {
					r.State = core.ResvUncertain
				}
			}
		}
		return
	}
	if h.uncertain {
		h.uncertain = false
		for _, id := range m.vehicleResIDs(h) {
			if r := m.resv[id]; r != nil && r.State == core.ResvUncertain {
				r.State = core.ResvGranted
			}
		}
	}
	if !h.hasPose || math.Hypot(ctx.Pose.X-h.lastPose.X, ctx.Pose.Y-h.lastPose.Y) > 0.05 || prevNode != ctx.CurrentNode {
		h.lastProgressAt = now
		h.lastPose = ctx.Pose
		h.hasPose = true
	}
	g := m.graph(ctx.SiteID)
	if g == nil {
		return
	}
	// drop segments the vehicle has completed (destination node reached). Their
	// reservations become "standing" and are released by recomputeHoldings
	// unless the body still physically occupies the resource.
	reached := -1
	for i, s := range h.segments {
		if ctx.RouteIndex >= 0 {
			if s.index <= ctx.RouteIndex {
				reached = i
			}
		} else if s.to == ctx.CurrentNode {
			reached = i
		}
	}
	if reached >= 0 {
		for _, s := range h.segments[:reached+1] {
			h.standingIDs = append(h.standingIDs, s.resIDs...)
		}
		h.segments = append([]*segment(nil), h.segments[reached+1:]...)
	}
	m.recomputeHoldings(h, g)
}

// recomputeHoldings keeps exactly: the current node, zones under the body,
// and resources of remaining granted segments. Everything else is released.
func (m *Manager) recomputeHoldings(h *hold, g *mapmgr.Graph) {
	needed := map[string]bool{}
	if h.ctx.CurrentNode != "" {
		needed[core.ResourceRef{Type: core.ResNode, ID: h.ctx.CurrentNode}.Key()] = true
	}
	if len(h.bodyNow) > 0 {
		for _, zid := range g.ZonesForPolygon(h.bodyNow) {
			if z := g.Zones[zid]; z != nil && mapmgr.Reservable(z.Type) {
				needed[core.ResourceRef{Type: core.ResZone, ID: zid}.Key()] = true
			}
		}
	}
	segRes := map[string]bool{}
	for _, s := range h.segments {
		for _, id := range s.resIDs {
			segRes[id] = true
		}
	}
	// release standing reservations no longer needed
	newStanding := h.standingIDs[:0]
	for _, id := range h.standingIDs {
		r := m.resv[id]
		if r == nil || r.State == core.ResvReleased {
			continue
		}
		if needed[r.Resource.Key()] || segRes[id] {
			newStanding = append(newStanding, id)
			delete(needed, r.Resource.Key())
			continue
		}
		m.release(r)
	}
	h.standingIDs = newStanding
	// resources of segments that cover a needed key count as held
	for _, s := range h.segments {
		for _, id := range s.resIDs {
			if r := m.resv[id]; r != nil {
				delete(needed, r.Resource.Key())
			}
		}
	}
	// create standing reservations for newly occupied resources (the vehicle
	// is physically there, so these are granted unconditionally and marked active)
	for key := range needed {
		ref := parseKey(key)
		if ref.Type == core.ResNode {
			trace("STAND %s node %s (holders %v)", h.ctx.ID, ref.ID, m.holdersOf(ref, h.ctx.ID))
		}
		r := m.newReservation(h.ctx, ref, 0)
		r.State = core.ResvActive
		h.standingIDs = append(h.standingIDs, r.ID)
	}
	// mark reservations of the current node active
	for _, id := range h.standingIDs {
		if r := m.resv[id]; r != nil && r.State == core.ResvGranted {
			r.State = core.ResvActive
		}
	}
	m.updateLaneDirs(h, g)
}

func (m *Manager) updateLaneDirs(h *hold, g *mapmgr.Graph) {
	for zid, vs := range m.laneDir {
		if _, ok := vs[h.ctx.ID]; ok {
			stillHeld := false
			for _, id := range m.vehicleResIDs(h) {
				if r := m.resv[id]; r != nil && r.State != core.ResvReleased && r.Resource.Type == core.ResZone && r.Resource.ID == zid {
					stillHeld = true
				}
			}
			if !stillHeld {
				delete(vs, h.ctx.ID)
			}
		}
	}
}

func parseKey(k string) core.ResourceRef {
	for i := 0; i < len(k); i++ {
		if k[i] == ':' {
			return core.ResourceRef{Type: core.ResourceType(k[:i]), ID: k[i+1:]}
		}
	}
	return core.ResourceRef{ID: k}
}

func (m *Manager) vehicleResIDs(h *hold) []string {
	var ids []string
	ids = append(ids, h.standingIDs...)
	for _, s := range h.segments {
		ids = append(ids, s.resIDs...)
	}
	return ids
}

func (m *Manager) newReservation(ctx VehicleCtx, ref core.ResourceRef, seq int) *core.Reservation {
	now := m.clock.Now()
	r := &core.Reservation{ID: uuid.NewString(), VehicleID: ctx.ID, SiteID: ctx.SiteID, Resource: ref, State: core.ResvGranted, Sequence: seq, RequestedAt: now, GrantedAt: &now}
	m.resv[r.ID] = r
	if m.byRes[ref.Key()] == nil {
		m.byRes[ref.Key()] = map[string]bool{}
	}
	m.byRes[ref.Key()][r.ID] = true
	return r
}

func (m *Manager) release(r *core.Reservation) {
	if r.State == core.ResvReleased {
		return
	}
	if r.Resource.Type == core.ResNode {
		trace("RELEASE %s node %s (%s)", r.VehicleID, r.Resource.ID, r.State)
	}
	now := m.clock.Now()
	r.State = core.ResvReleased
	r.ReleasedAt = &now
	delete(m.byRes[r.Resource.Key()], r.ID)
}

// holdersOf returns the distinct other vehicles holding a resource.
func (m *Manager) holdersOf(ref core.ResourceRef, except string) []string {
	seen := map[string]bool{}
	var out []string
	for id := range m.byRes[ref.Key()] {
		r := m.resv[id]
		if r == nil || r.State == core.ResvReleased || r.VehicleID == except {
			continue
		}
		if !seen[r.VehicleID] {
			seen[r.VehicleID] = true
			out = append(out, r.VehicleID)
		}
	}
	sort.Strings(out)
	return out
}

func (m *Manager) capacity(g *mapmgr.Graph, ref core.ResourceRef) int {
	switch ref.Type {
	case core.ResNode:
		if n := g.Nodes[ref.ID]; n != nil {
			return n.Capacity
		}
	case core.ResZone:
		if z := g.Zones[ref.ID]; z != nil {
			return z.Capacity
		}
	}
	return 1
}

// Request asks for the next segments of the full route; route[ctx.RouteIndex]
// must be the current node (RouteIndex 0 for a route starting at the vehicle).
// Already granted segments are kept; new ones are granted in order until a
// resource conflict, up to the lookahead window.
func (m *Manager) Request(ctx VehicleCtx, route []string) GrantResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.holds[ctx.ID]
	if h == nil {
		h = &hold{ctx: ctx, lastProgressAt: m.clock.Now()}
		m.holds[ctx.ID] = h
	}
	g := m.graph(ctx.SiteID)
	cur := ctx.RouteIndex
	if cur < 0 {
		cur = 0
	}
	if g == nil || cur >= len(route) || route[cur] != ctx.CurrentNode {
		return GrantResult{GrantedThrough: cur, Denied: &Denial{Reason: "route index does not match the current node"}}
	}
	if h.uncertain || !ctx.Connected {
		return GrantResult{Denied: &Denial{Reason: "vehicle state uncertain"}}
	}
	// drop granted segments that no longer match the route (replanning) or
	// that lie behind the vehicle
	valid := h.segments[:0]
	for _, s := range h.segments {
		if s.index > cur && s.index < len(route) && route[s.index] == s.to && route[s.index-1] == s.from {
			valid = append(valid, s)
		} else {
			for _, id := range s.resIDs {
				if r := m.resv[id]; r != nil {
					m.release(r)
				}
			}
		}
	}
	h.segments = valid
	granted := cur
	heading := ctx.Pose.Theta
	for _, s := range h.segments {
		granted = s.index
		heading = s.heading
	}
	// standing reservations cover the current node; if a granted segment is
	// still open but the vehicle has not left yet, fine.
	lookahead := m.settings.LookaheadSegments
	if lookahead <= 0 {
		lookahead = 3
	}
	for next := granted + 1; next < len(route) && next <= cur+lookahead; next++ {
		from, to := route[next-1], route[next]
		e := g.EdgeBetween(from, to)
		if e == nil || !e.Enabled {
			h.setWaiting(core.ResourceRef{Type: core.ResEdge, ID: from + "-" + to}, nil, m.clock.Now())
			return GrantResult{GrantedThrough: granted, Denied: &Denial{Resource: core.ResourceRef{Type: core.ResEdge, ID: from + "-" + to}, Reason: "edge missing or disabled"}}
		}
		seg := m.buildSegment(g, ctx, next, from, to, e, heading)
		if d := m.checkSegment(g, ctx, h, seg); d != nil {
			h.setWaiting(d.Resource, d.HeldBy, m.clock.Now())
			return GrantResult{GrantedThrough: granted, Denied: d}
		}
		// grant
		trace("GRANT %s seg %d %s->%s (holders of %s: %v)", ctx.ID, next, from, to, to, m.holdersOf(core.ResourceRef{Type: core.ResNode, ID: to}, ctx.ID))
		if Strict {
			if hs := m.holdersOf(core.ResourceRef{Type: core.ResNode, ID: to}, ctx.ID); len(hs) >= m.capacity(g, core.ResourceRef{Type: core.ResNode, ID: to}) {
				panic(fmt.Sprintf("traffic: granting %s to %s while held by %v", to, ctx.ID, hs))
			}
		}
		for _, ref := range m.segmentResources(g, seg) {
			r := m.newReservation(ctx, ref, next)
			seg.resIDs = append(seg.resIDs, r.ID)
			if ref.Type == core.ResZone {
				if z := g.Zones[ref.ID]; z != nil && z.Type == core.ZoneSingleLane {
					if m.laneDir[ref.ID] == nil {
						m.laneDir[ref.ID] = map[string]float64{}
					}
					m.laneDir[ref.ID][ctx.ID] = g.Heading(from, to)
				}
			}
		}
		h.segments = append(h.segments, seg)
		granted = next
		heading = seg.heading
	}
	h.waitingFor, h.waitingOn = nil, nil
	return GrantResult{GrantedThrough: granted}
}

func (h *hold) setWaiting(ref core.ResourceRef, heldBy []string, now time.Time) {
	if h.waitingFor == nil || h.waitingFor.Key() != ref.Key() {
		h.waitingSince = now
	}
	h.waitingFor = &ref
	h.waitingOn = heldBy
}

func (m *Manager) buildSegment(g *mapmgr.Graph, ctx VehicleCtx, idx int, from, to string, e *core.Edge, prevHeading float64) *segment {
	edgeHeading := g.Heading(from, to)
	travel := mapmgr.TravelHeading(prevHeading, edgeHeading, e.Motion, g.Length(e))
	a := core.Pose{X: g.Nodes[from].X, Y: g.Nodes[from].Y, Theta: prevHeading}
	b := core.Pose{X: g.Nodes[to].X, Y: g.Nodes[to].Y, Theta: travel}
	samples := m.settings.Swept.Samples
	if samples < 2 {
		samples = 6
	}
	env := geometry.SweptEnvelope(a, b, ctx.Envelope.LengthM, ctx.Envelope.WidthM, samples)
	return &segment{index: idx, from: from, to: to, edgeID: e.ID, heading: travel, envelope: env}
}

func (m *Manager) segmentResources(g *mapmgr.Graph, s *segment) []core.ResourceRef {
	refs := []core.ResourceRef{{Type: core.ResEdge, ID: s.edgeID}, {Type: core.ResNode, ID: s.to}}
	seen := map[string]bool{}
	for _, zid := range g.ZonesForPolygon(s.envelope) {
		if z := g.Zones[zid]; z != nil && mapmgr.Reservable(z.Type) && !seen[zid] {
			seen[zid] = true
			refs = append(refs, core.ResourceRef{Type: core.ResZone, ID: zid})
		}
	}
	return refs
}

// checkSegment verifies capacities, single-lane flow direction and geometric
// conflicts with other vehicles' envelopes and bodies.
func (m *Manager) checkSegment(g *mapmgr.Graph, ctx VehicleCtx, h *hold, s *segment) *Denial {
	for _, ref := range m.segmentResources(g, s) {
		if ref.Type == core.ResZone {
			if z := g.Zones[ref.ID]; z != nil && !z.Enabled {
				return &Denial{Resource: ref, Reason: "zone temporarily unavailable"}
			}
		}
		holders := m.holdersOf(ref, ctx.ID)
		if len(holders) >= m.capacity(g, ref) {
			return &Denial{Resource: ref, HeldBy: holders, Reason: "resource at capacity"}
		}
		if ref.Type == core.ResZone {
			if z := g.Zones[ref.ID]; z != nil && z.Type == core.ZoneSingleLane {
				dir := g.Heading(s.from, s.to)
				for vid, d := range m.laneDir[ref.ID] {
					if vid == ctx.ID {
						continue
					}
					if math.Cos(dir-d) < 0 {
						return &Denial{Resource: ref, HeldBy: holders, Reason: "single lane occupied in opposite direction"}
					}
				}
			}
		}
	}
	// geometric conflicts: other vehicles' granted envelopes and bodies
	for vid, oh := range m.holds {
		if vid == ctx.ID {
			continue
		}
		body := oh.bodyNow
		if oh.uncertain && len(body) > 0 {
			body = geometry.Offset(body, 1.0) // unknown state: keep extra distance
		}
		if len(body) > 0 && geometry.Intersects(s.envelope, body) {
			return &Denial{Resource: core.ResourceRef{Type: core.ResNode, ID: oh.ctx.CurrentNode}, HeldBy: []string{vid}, Reason: "envelope conflicts with vehicle body"}
		}
		for _, os := range oh.segments {
			if geometry.Intersects(s.envelope, os.envelope) {
				return &Denial{Resource: core.ResourceRef{Type: core.ResEdge, ID: os.edgeID}, HeldBy: []string{vid}, Reason: "swept envelope conflicts with granted manoeuvre"}
			}
		}
	}
	return nil
}

// ClearRoute releases all granted segments of a vehicle (mission end,
// cancellation, replanning). Standing reservations are kept.
func (m *Manager) ClearRoute(vehicleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.holds[vehicleID]
	if h == nil {
		return
	}
	if h.uncertain {
		return // never release anything for a vehicle in unknown state
	}
	for _, s := range h.segments {
		for _, id := range s.resIDs {
			if r := m.resv[id]; r != nil {
				m.release(r)
			}
		}
	}
	h.segments = nil
	h.waitingFor, h.waitingOn = nil, nil
	if g := m.graph(h.ctx.SiteID); g != nil {
		m.recomputeHoldings(h, g)
	}
}

// Remove forgets a vehicle entirely (vehicle deleted from the registry).
func (m *Manager) Remove(vehicleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.holds[vehicleID]
	if h == nil {
		return
	}
	for _, id := range m.vehicleResIDs(h) {
		if r := m.resv[id]; r != nil {
			m.release(r)
		}
	}
	delete(m.holds, vehicleID)
	for _, vs := range m.laneDir {
		delete(vs, vehicleID)
	}
}

// ManualRelease releases an uncertain reservation after a human confirmed the
// physical state. Returns an error for any other state.
func (m *Manager) ManualRelease(reservationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.resv[reservationID]
	if r == nil {
		return core.NotFound("reservation", reservationID)
	}
	if r.State != core.ResvUncertain {
		return core.Conflict("only uncertain reservations can be released manually (state %s)", r.State)
	}
	m.release(r)
	return nil
}

// Reservations lists non-released reservations (all sites when siteID is empty).
func (m *Manager) Reservations(siteID string) []core.Reservation {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []core.Reservation
	for _, r := range m.resv {
		if r.State == core.ResvReleased || (siteID != "" && r.SiteID != siteID) {
			continue
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].VehicleID != out[j].VehicleID {
			return out[i].VehicleID < out[j].VehicleID
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

// GrantedThrough returns the last granted route index for a vehicle (0 if none).
func (m *Manager) GrantedThrough(vehicleID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.holds[vehicleID]
	if h == nil || len(h.segments) == 0 {
		return 0
	}
	return h.segments[len(h.segments)-1].index
}

// Congestion returns a planner cost (metre equivalents) for an edge based on
// reservations held by other vehicles on the edge and its end nodes.
func (m *Manager) Congestion(siteID string) func(edgeID string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.graph(siteID)
	cost := map[string]float64{}
	for _, r := range m.resv {
		if r.State == core.ResvReleased || r.SiteID != siteID {
			continue
		}
		// metre-equivalents of expected delay: a standing vehicle blocks for a
		// while, a planned passage clears quickly, an unknown vehicle may never move
		w := 15.0
		switch r.State {
		case core.ResvActive:
			w = 50
		case core.ResvUncertain:
			w = 300
		}
		if r.Resource.Type == core.ResZone {
			w /= 3
		}
		cost[r.Resource.Key()] += w
	}
	return func(edgeID string) float64 {
		if g == nil {
			return 0
		}
		e := g.Edges[edgeID]
		if e == nil {
			return 0
		}
		c := cost[core.ResourceRef{Type: core.ResEdge, ID: edgeID}.Key()]
		c += cost[core.ResourceRef{Type: core.ResNode, ID: e.From}.Key()] / 2
		c += cost[core.ResourceRef{Type: core.ResNode, ID: e.To}.Key()]
		return c
	}
}

// OccupiedNodes returns the nodes on which other vehicles currently stand
// (active or uncertain reservations), excluding the given vehicle.
func (m *Manager) OccupiedNodes(siteID, except string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, r := range m.resv {
		if r.SiteID != siteID || r.VehicleID == except || r.Resource.Type != core.ResNode {
			continue
		}
		if (r.State == core.ResvActive || r.State == core.ResvUncertain) && !seen[r.Resource.ID] {
			seen[r.Resource.ID] = true
			out = append(out, r.Resource.ID)
		}
	}
	sort.Strings(out)
	return out
}

// Blocked returns vehicles that have not progressed for longer than the
// configured timeout while waiting for a resource or holding a granted segment.
func (m *Manager) Blocked() []core.BlockedVehicle {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	timeout := time.Duration(m.settings.BlockedTimeoutS * float64(time.Second))
	var out []core.BlockedVehicle
	for vid, h := range m.holds {
		if h.uncertain {
			continue
		}
		if h.waitingFor != nil && now.Sub(h.waitingSince) > timeout {
			out = append(out, core.BlockedVehicle{VehicleID: vid, Since: h.waitingSince, WaitingFor: []string{h.waitingFor.Key()}, HeldBy: joined(h.waitingOn), Reason: "waiting for resource"})
			continue
		}
		if len(h.segments) > 0 && h.hasPose && now.Sub(h.lastProgressAt) > timeout {
			out = append(out, core.BlockedVehicle{VehicleID: vid, Since: h.lastProgressAt, Reason: "no progress on granted segment"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VehicleID < out[j].VehicleID })
	return out
}

func joined(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}

// DetectDeadlocks finds cycles in the wait-for graph and returns a resolution
// for each new cycle: the lowest priority vehicle in the cycle is told to
// replan avoiding the contested resources, or to back off when no alternative
// exists (the fleet core decides which based on planner feedback).
func (m *Manager) DetectDeadlocks() []Resolution {
	m.mu.Lock()
	defer m.mu.Unlock()
	// only vehicles that are really stuck take part: nothing granted ahead,
	// not moving, and waiting for a few seconds (transient refusals while a
	// vehicle still drives on its granted base are not deadlocks)
	now := m.clock.Now()
	waits := map[string][]string{}
	for vid, h := range m.holds {
		if h.waitingFor == nil || len(h.segments) > 0 || h.uncertain {
			continue
		}
		if now.Sub(h.waitingSince) < 3*time.Second || (h.hasPose && now.Sub(h.lastProgressAt) < 2*time.Second) {
			continue
		}
		waits[vid] = h.waitingOn
	}
	var res []Resolution
	visited := map[string]bool{}
	for start := range waits {
		if visited[start] {
			continue
		}
		cycle := findCycle(start, waits, map[string]bool{}, nil)
		if cycle == nil {
			continue
		}
		for _, v := range cycle {
			visited[v] = true
		}
		if m.activeDeadlock(cycle) {
			continue
		}
		// choose the victim: lowest priority, tie-break on shortest wait. When
		// the same cycle reappears shortly after a resolution, rotate to a
		// vehicle that has not been the victim yet so both sides try to yield.
		sorted := append([]string(nil), cycle...)
		sort.Slice(sorted, func(i, j int) bool {
			hi, hj := m.holds[sorted[i]], m.holds[sorted[j]]
			if hi.ctx.Priority != hj.ctx.Priority {
				return hi.ctx.Priority < hj.ctx.Priority
			}
			return hi.waitingSince.After(hj.waitingSince)
		})
		ckey := strings.Join(sorted, "|")
		now := m.clock.Now()
		rec := m.recent[ckey]
		if rec == nil || now.Sub(rec.at) > 2*time.Minute {
			rec = &recentDeadlock{victims: map[string]bool{}}
			m.recent[ckey] = rec
		}
		if now.Before(rec.deferred) {
			continue // a resolution failed recently: give the fleet time before retrying
		}
		rec.at = now
		victim := sorted[0]
		for _, v := range sorted {
			if !rec.victims[v] {
				victim = v
				break
			}
		}
		if rec.victims[victim] { // everyone tried: start over
			rec.victims = map[string]bool{}
		}
		rec.victims[victim] = true
		var resources, avoidEdges, avoidNodes []string
		for _, v := range cycle {
			if h := m.holds[v]; h.waitingFor != nil {
				resources = append(resources, h.waitingFor.Key())
				switch h.waitingFor.Type {
				case core.ResEdge:
					avoidEdges = append(avoidEdges, h.waitingFor.ID)
				case core.ResNode:
					avoidNodes = append(avoidNodes, h.waitingFor.ID)
				}
			}
		}
		// also avoid the nodes occupied by the other vehicles of the cycle
		for _, v := range cycle {
			if v != victim {
				avoidNodes = append(avoidNodes, m.holds[v].ctx.CurrentNode)
			}
		}
		d := core.Deadlock{ID: uuid.NewString(), SiteID: m.holds[victim].ctx.SiteID, VehicleIDs: sorted, ResourceIDs: resources, DetectedAt: m.clock.Now(),
			Resolution: fmt.Sprintf("replan %s avoiding %v", victim, resources)}
		rec.id = d.ID
		m.deadlocks = append(m.deadlocks, d)
		res = append(res, Resolution{DeadlockID: d.ID, VehicleID: victim, Action: "replan", AvoidEdges: avoidEdges, AvoidNodes: avoidNodes})
	}
	return res
}

func (m *Manager) activeDeadlock(cycle []string) bool {
	set := map[string]bool{}
	for _, v := range cycle {
		set[v] = true
	}
	for i := len(m.deadlocks) - 1; i >= 0; i-- {
		d := m.deadlocks[i]
		if d.ResolvedAt != nil {
			continue
		}
		if len(d.VehicleIDs) != len(cycle) {
			continue
		}
		same := true
		for _, v := range d.VehicleIDs {
			if !set[v] {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

// ResolveDeadlock marks a deadlock as resolved. When the resolution failed
// (how starts with "no alternative"), the same cycle is not reported again
// for 20 seconds so the fleet can evolve before the next attempt.
func (m *Manager) ResolveDeadlock(id, how string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.deadlocks {
		if m.deadlocks[i].ID == id && m.deadlocks[i].ResolvedAt == nil {
			now := m.clock.Now()
			m.deadlocks[i].ResolvedAt = &now
			if how != "" {
				m.deadlocks[i].Resolution = how
			}
			m.resolved++
			if strings.HasPrefix(how, "no alternative") {
				for _, rec := range m.recent {
					if rec.id == id {
						rec.deferred = now.Add(20 * time.Second)
					}
				}
			}
		}
	}
}

// Deadlocks returns the deadlock history (newest first).
func (m *Manager) Deadlocks(siteID string) []core.Deadlock {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []core.Deadlock
	for i := len(m.deadlocks) - 1; i >= 0; i-- {
		if siteID == "" || m.deadlocks[i].SiteID == siteID {
			out = append(out, m.deadlocks[i])
		}
	}
	return out
}

// ResolvedCount returns the number of resolved deadlocks.
func (m *Manager) ResolvedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resolved
}

func findCycle(start string, waits map[string][]string, onPath map[string]bool, path []string) []string {
	onPath[start] = true
	path = append(path, start)
	for _, next := range waits[start] {
		if onPath[next] {
			// extract cycle from next to end of path
			for i, v := range path {
				if v == next {
					c := make([]string, len(path)-i)
					copy(c, path[i:])
					return c
				}
			}
		}
		if _, waiting := waits[next]; waiting {
			if c := findCycle(next, waits, onPath, path); c != nil {
				return c
			}
		}
	}
	onPath[start] = false
	return nil
}

// Waiting returns what a vehicle is waiting for (nil when not waiting).
func (m *Manager) Waiting(vehicleID string) *Denial {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.holds[vehicleID]
	if h == nil || h.waitingFor == nil {
		return nil
	}
	return &Denial{Resource: *h.waitingFor, HeldBy: h.waitingOn}
}

// Envelopes returns the granted swept envelopes of a vehicle (for display/tests).
func (m *Manager) Envelopes(vehicleID string) []geometry.Polygon {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.holds[vehicleID]
	if h == nil {
		return nil
	}
	var out []geometry.Polygon
	for _, s := range h.segments {
		out = append(out, s.envelope)
	}
	return out
}
