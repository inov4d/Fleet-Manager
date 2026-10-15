package traffic

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/geometry"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/planner"
)

var env20 = core.Footprint{LengthM: 6.8, WidthM: 3.2}

func corridor() (*Manager, *mapmgr.Graph, *core.ManualClock) {
	m := &core.Map{Name: "c", Nodes: []core.Node{{ID: "a", X: 0}, {ID: "b", X: 20}, {ID: "c", X: 40}, {ID: "d", X: 60}, {ID: "s", X: 20, Y: 20}},
		Edges: []core.Edge{
			{ID: "ab", From: "a", To: "b", Bidirectional: true, Enabled: true},
			{ID: "bc", From: "b", To: "c", Bidirectional: true, Enabled: true},
			{ID: "cd", From: "c", To: "d", Bidirectional: true, Enabled: true},
			{ID: "bs", From: "b", To: "s", Bidirectional: true, Enabled: true},
		},
		Zones: []core.Zone{{ID: "cross", Type: core.ZoneCrossing, Enabled: true, Capacity: 1, Polygon: []core.Point{{X: 15, Y: -5}, {X: 25, Y: -5}, {X: 25, Y: 5}, {X: 15, Y: 5}}}}}
	_ = mapmgr.Validate(m)
	g := mapmgr.NewGraph(m)
	clk := core.NewManualClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	tm := New(core.DefaultTrafficSettings(), clk)
	tm.SetGraph("s", g)
	return tm, g, clk
}

func ctxAt(id string, g *mapmgr.Graph, node string, theta float64, prio int) VehicleCtx {
	n := g.Nodes[node]
	return VehicleCtx{ID: id, SiteID: "s", Pose: core.Pose{X: n.X, Y: n.Y, Theta: theta}, Envelope: env20, Priority: prio, CurrentNode: node, Connected: true, RouteIndex: -1}
}

// req issues a request for a route starting at the vehicle's current node.
func req(tm *Manager, v VehicleCtx, route []string) GrantResult {
	v.RouteIndex = 0
	return tm.Request(v, route)
}

func TestHeadOnBidirectionalEdge(t *testing.T) {
	tm, g, _ := corridor()
	v1 := ctxAt("v1", g, "a", 0, 5)
	v2 := ctxAt("v2", g, "d", math.Pi, 5)
	tm.Observe(v1)
	tm.Observe(v2)
	r1 := req(tm, v1, []string{"a", "b", "c", "d"})
	// v2 stands on d (and physically covers the end of edge cd): v1 gets a-b-c only
	if r1.GrantedThrough != 2 || r1.Denied == nil || r1.Denied.HeldBy[0] != "v2" {
		t.Fatalf("v1 should get a-b-c and be denied d (held by v2), got %d %+v", r1.GrantedThrough, r1.Denied)
	}
	r2 := req(tm, v2, []string{"d", "c", "b", "a"})
	if r2.GrantedThrough != 0 || r2.Denied == nil {
		t.Fatalf("v2 must be denied, got %+v", r2)
	}
	if len(r2.Denied.HeldBy) == 0 || r2.Denied.HeldBy[0] != "v1" {
		t.Fatalf("denial should name v1: %+v", r2.Denied)
	}
}

func TestCrossingZoneReleasedOnProgress(t *testing.T) {
	tm, g, _ := corridor()
	v1 := ctxAt("v1", g, "a", 0, 5)
	v2 := ctxAt("v2", g, "s", -math.Pi/2, 5)
	tm.Observe(v1)
	tm.Observe(v2)
	if r := req(tm, v1, []string{"a", "b", "c"}); r.GrantedThrough != 2 {
		t.Fatalf("v1 grant %+v", r)
	}
	// v2 wants to enter b (crossing zone) - held by v1
	if r := req(tm, v2, []string{"s", "b", "a"}); r.GrantedThrough != 0 {
		t.Fatalf("v2 should be denied: %+v", r)
	}
	// v1 drives to c: reaches b then c
	v1.CurrentNode, v1.Pose = "b", core.Pose{X: 20, Y: 0}
	tm.Observe(v1)
	v1.CurrentNode, v1.Pose = "c", core.Pose{X: 40, Y: 0}
	tm.Observe(v1)
	// now the crossing and node b must be free
	for _, r := range tm.Reservations("s") {
		if r.VehicleID == "v1" && (r.Resource.ID == "b" || r.Resource.ID == "cross" || r.Resource.ID == "ab") {
			t.Fatalf("v1 still holds %v after leaving", r.Resource)
		}
	}
	if r := req(tm, v2, []string{"s", "b", "a"}); r.GrantedThrough != 2 {
		t.Fatalf("v2 should now be granted: %+v", r)
	}
	// v1 keeps its current node c
	held := false
	for _, r := range tm.Reservations("s") {
		if r.VehicleID == "v1" && r.Resource.Type == core.ResNode && r.Resource.ID == "c" && r.State == core.ResvActive {
			held = true
		}
	}
	if !held {
		t.Fatal("v1 must keep an active reservation on its current node")
	}
}

func TestCommsLossKeepsReservationsUncertain(t *testing.T) {
	tm, g, clk := corridor()
	v1 := ctxAt("v1", g, "a", 0, 5)
	tm.Observe(v1)
	req(tm, v1, []string{"a", "b", "c"})
	v1.Connected = false
	tm.Observe(v1)
	clk.Advance(time.Hour)
	tm.Observe(v1)
	var uncertain, other int
	var anyID string
	for _, r := range tm.Reservations("s") {
		if r.State == core.ResvUncertain {
			uncertain++
			anyID = r.ID
		} else {
			other++
		}
	}
	if uncertain == 0 || other != 0 {
		t.Fatalf("expected all reservations uncertain: %d uncertain, %d other", uncertain, other)
	}
	// nothing is released on the timer, even ClearRoute refuses
	tm.ClearRoute("v1")
	if len(tm.Reservations("s")) != uncertain {
		t.Fatal("reservations of an uncertain vehicle must not be released")
	}
	// another vehicle must keep away from the unknown vehicle's resources
	v2 := ctxAt("v2", g, "d", math.Pi, 5)
	tm.Observe(v2)
	if r := req(tm, v2, []string{"d", "c", "b"}); r.GrantedThrough != 0 {
		t.Fatalf("v2 must not get resources held by an uncertain vehicle: %+v", r)
	}
	// a human confirms the physical state: manual release allowed
	if err := tm.ManualRelease(anyID); err != nil {
		t.Fatal(err)
	}
	// requests from the uncertain vehicle are refused
	if r := req(tm, v1, []string{"a", "b"}); r.Denied == nil {
		t.Fatal("uncertain vehicle must not be granted anything")
	}
	// comms back: reservations return to granted
	v1.Connected = true
	tm.Observe(v1)
	for _, r := range tm.Reservations("s") {
		if r.State == core.ResvUncertain {
			t.Fatal("reservations should be restored after reconnection")
		}
	}
}

func TestDeadlockDetection(t *testing.T) {
	tm, g, clk := corridor()
	// v1 at b wants c, v2 at c wants b: each holds the other's target node.
	v1 := ctxAt("v1", g, "b", 0, 5)
	v2 := ctxAt("v2", g, "c", math.Pi, 3)
	tm.Observe(v1)
	tm.Observe(v2)
	r1 := req(tm, v1, []string{"b", "c", "d"})
	r2 := req(tm, v2, []string{"c", "b", "a"})
	if r1.GrantedThrough != 0 || r2.GrantedThrough != 0 {
		t.Fatalf("both should be denied: %+v %+v", r1, r2)
	}
	res := tm.DetectDeadlocks()
	if len(res) != 1 {
		t.Fatalf("expected one deadlock resolution, got %v", res)
	}
	if res[0].VehicleID != "v2" {
		t.Fatalf("lowest priority vehicle (v2) should be the victim, got %s", res[0].VehicleID)
	}
	if len(tm.Deadlocks("s")) != 1 {
		t.Fatal("deadlock should be recorded")
	}
	// second detection of the same cycle does not duplicate
	if len(tm.DetectDeadlocks()) != 0 {
		t.Fatal("duplicate deadlock reported")
	}
	tm.ResolveDeadlock(res[0].DeadlockID, "replanned")
	if tm.ResolvedCount() != 1 {
		t.Fatal("resolved count")
	}
	clk.Advance(2 * time.Minute)
	b := tm.Blocked()
	if len(b) != 2 {
		t.Fatalf("both vehicles should be reported blocked: %v", b)
	}
}

func TestSingleLaneDirection(t *testing.T) {
	m := &core.Map{Name: "lane", Nodes: []core.Node{{ID: "a", X: 0}, {ID: "b", X: 30}, {ID: "c", X: 60}, {ID: "d", X: 90}},
		Edges: []core.Edge{
			{ID: "ab", From: "a", To: "b", Bidirectional: true, Enabled: true},
			{ID: "bc", From: "b", To: "c", Bidirectional: true, Enabled: true},
			{ID: "cd", From: "c", To: "d", Bidirectional: true, Enabled: true},
		},
		Zones: []core.Zone{{ID: "lane", Type: core.ZoneSingleLane, Enabled: true, Capacity: 3, Polygon: []core.Point{{X: 10, Y: -5}, {X: 80, Y: -5}, {X: 80, Y: 5}, {X: 10, Y: 5}}}}}
	_ = mapmgr.Validate(m)
	g := mapmgr.NewGraph(m)
	tm := New(core.DefaultTrafficSettings(), core.NewManualClock(time.Now()))
	tm.SetGraph("s", g)
	v1 := ctxAt("v1", g, "a", 0, 5)
	v3 := ctxAt("v3", g, "d", math.Pi, 5)
	tm.Observe(v1)
	tm.Observe(v3)
	if r := req(tm, v1, []string{"a", "b"}); r.GrantedThrough != 1 {
		t.Fatalf("v1 %+v", r)
	}
	// same direction, capacity 3: a follower may enter (but the node b is held by v1 so it
	// can only request as far as geometry/capacity allows). Opposite direction is refused.
	r := req(tm, v3, []string{"d", "c"})
	if r.GrantedThrough != 0 || r.Denied == nil || r.Denied.Reason != "single lane occupied in opposite direction" {
		t.Fatalf("opposite direction must be refused: %+v", r)
	}
}

func TestDisabledZoneRefused(t *testing.T) {
	tm, g, _ := corridor()
	g.Zones["cross"].Enabled = false
	v1 := ctxAt("v1", g, "a", 0, 5)
	tm.Observe(v1)
	r := req(tm, v1, []string{"a", "b"})
	if r.GrantedThrough != 0 || r.Denied == nil || r.Denied.Reason != "zone temporarily unavailable" {
		t.Fatalf("%+v", r)
	}
}

// TestFortyVehiclesInvariants drives 40 vehicles along random routes on the
// demo map and checks that no exclusive resource is ever double-booked and
// that granted manoeuvre envelopes of different vehicles never overlap.
func TestFortyVehiclesInvariants(t *testing.T) {
	g := mapmgr.NewGraph(mapmgr.DemoMapSized("s", 6, 8))
	clk := core.NewManualClock(time.Now())
	tm := New(core.DefaultTrafficSettings(), clk)
	tm.SetGraph("s", g)
	rng := rand.New(rand.NewSource(42))
	nodeIDs := make([]string, 0, len(g.Map.Nodes))
	for _, n := range g.Map.Nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}
	type veh struct {
		ctx     VehicleCtx
		route   []string
		pos     int
		waiting int
	}
	var vs []*veh
	used := map[string]bool{}
	for i := 0; i < 40; i++ {
		var start string
		for {
			start = nodeIDs[rng.Intn(len(nodeIDs))]
			if !used[start] {
				used[start] = true
				break
			}
		}
		env := env20
		if i%2 == 1 {
			env = core.Footprint{LengthM: 8.8, WidthM: 3.8}
		}
		n := g.Nodes[start]
		v := &veh{ctx: VehicleCtx{ID: "v" + itoa(i), SiteID: "s", Pose: core.Pose{X: n.X, Y: n.Y}, Envelope: env, Priority: 1 + rng.Intn(10), CurrentNode: start, Connected: true}}
		vs = append(vs, v)
		tm.Observe(v.ctx)
	}
	newRoute := func(v *veh, avoidNodes []string) {
		for tries := 0; tries < 20; tries++ {
			dst := nodeIDs[rng.Intn(len(nodeIDs))]
			if dst == v.ctx.CurrentNode {
				continue
			}
			r, err := (planner.AStar{}).Plan(planner.Request{Graph: g, From: v.ctx.CurrentNode, To: dst, Envelope: v.ctx.Envelope,
				AvoidNodes: avoidNodes, Congestion: tm.Congestion("s")})
			if err == nil {
				v.route, v.pos = r.NodeIDs, 0
				return
			}
		}
	}
	for _, v := range vs {
		newRoute(v, nil)
	}
	start := time.Now()
	moves := 0
	for tick := 0; tick < 400; tick++ {
		clk.Advance(time.Second)
		for _, v := range vs {
			if v.route == nil {
				newRoute(v, nil)
				continue
			}
			v.ctx.RouteIndex = v.pos
			res := tm.Request(v.ctx, v.route)
			if res.GrantedThrough == 0 && res.Denied != nil {
				// fleet-core policy: after waiting a while, replan around the
				// contested resource using congestion costs
				v.waiting++
				if v.waiting > 5 {
					v.waiting = 0
					var avoid []string
					if res.Denied.Resource.Type == core.ResNode {
						avoid = append(avoid, res.Denied.Resource.ID)
					}
					tm.ClearRoute(v.ctx.ID)
					v.route = nil
					newRoute(v, avoid)
				}
				continue
			}
			v.waiting = 0
			if res.GrantedThrough > v.pos {
				// advance one node, heading chosen like the vehicle would
				v.pos++
				n := g.Nodes[v.route[v.pos]]
				e := g.EdgeBetween(v.route[v.pos-1], v.route[v.pos])
				th := mapmgr.TravelHeading(v.ctx.Pose.Theta, g.Heading(v.route[v.pos-1], v.route[v.pos]), e.Motion)
				v.ctx.CurrentNode = v.route[v.pos]
				v.ctx.Pose = core.Pose{X: n.X, Y: n.Y, Theta: th}
				tm.Observe(v.ctx)
				moves++
				if v.pos == len(v.route)-1 {
					tm.ClearRoute(v.ctx.ID)
					v.route = nil
				}
			}
		}
		checkInvariants(t, tm, g)
		if r := tm.DetectDeadlocks(); len(r) > 0 {
			for _, x := range r {
				// resolve by dropping the victim's route; it replans next tick
				for _, v := range vs {
					if v.ctx.ID == x.VehicleID {
						tm.ClearRoute(v.ctx.ID)
						v.route = nil
						newRoute(v, x.AvoidNodes)
					}
				}
				tm.ResolveDeadlock(x.DeadlockID, "replanned")
			}
		}
	}
	elapsed := time.Since(start)
	if moves < 200 {
		t.Fatalf("fleet barely moved: %d moves", moves)
	}
	t.Logf("40 vehicles, 400 ticks, %d moves, %d deadlocks resolved, %v", moves, tm.ResolvedCount(), elapsed)
	if elapsed > 20*time.Second {
		t.Fatalf("too slow: %v", elapsed)
	}
}

func checkInvariants(t *testing.T, tm *Manager, g *mapmgr.Graph) {
	t.Helper()
	holders := map[string]map[string]bool{}
	planned := map[string]bool{} // resources with at least one granted (planned move) reservation
	for _, r := range tm.Reservations("s") {
		k := r.Resource.Key()
		if holders[k] == nil {
			holders[k] = map[string]bool{}
		}
		holders[k][r.VehicleID] = true
		if r.State == core.ResvGranted {
			planned[k] = true
		}
	}
	for k, vs := range holders {
		if !planned[k] {
			continue // only physically present vehicles (active): placement, not a grant
		}
		ref := parseKey(k)
		capacity := 1
		switch ref.Type {
		case core.ResNode:
			capacity = g.Nodes[ref.ID].Capacity
		case core.ResZone:
			capacity = g.Zones[ref.ID].Capacity
		}
		if len(vs) > capacity {
			t.Fatalf("resource %s over capacity: %v", k, vs)
		}
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	ids := make([]string, 0, len(tm.holds))
	for id := range tm.holds {
		ids = append(ids, id)
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a, b := tm.holds[ids[i]], tm.holds[ids[j]]
			for _, sa := range a.segments {
				for _, sb := range b.segments {
					if geometry.Intersects(sa.envelope, sb.envelope) {
						t.Fatalf("envelopes of %s and %s overlap", ids[i], ids[j])
					}
				}
				if len(b.bodyNow) > 0 && geometry.Intersects(sa.envelope, b.bodyNow) {
					t.Fatalf("envelope of %s overlaps body of %s", ids[i], ids[j])
				}
			}
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
