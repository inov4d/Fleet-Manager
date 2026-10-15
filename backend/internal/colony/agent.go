package colony

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/planner"
	"github.com/inov4d/fleet-manager/internal/transport"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

// AgentConfig configures the on-board agent.
type AgentConfig struct {
	SiteID       string
	VehicleID    string
	Manufacturer string
	Serial       string
	Interface    string // VDA 5050 interface name (default uagv)
	// Response thresholds (the "ant" rules)
	LowBatteryPct      float64 // request a recharge when idle below this
	CriticalBatteryPct float64 // request a recharge even when a task is open
	ReserveBatteryPct  float64 // never take a task that would end below this
	ClaimScale         float64 // seconds of hesitation per second of cost unit (default 0.02)
	MinClaimDelay      time.Duration
	MaxClaimDelay      time.Duration
	ParkAfterIdle      time.Duration
	// NegotiateAfter is how long an agent waits behind a neighbour before
	// proposing a negotiation; NegotiateCooldown spaces negotiations per peer;
	// MaxChainHops bounds the propagation along the wait chain.
	NegotiateAfter    time.Duration
	NegotiateCooldown time.Duration
	MaxChainHops      int
}

// DefaultAgentConfig returns the reference thresholds.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{Interface: vda5050.DefaultInterface, LowBatteryPct: 30, CriticalBatteryPct: 15, ReserveBatteryPct: 10,
		ClaimScale: 0.02, MinClaimDelay: 100 * time.Millisecond, MaxClaimDelay: 3 * time.Second, ParkAfterIdle: 3 * time.Minute,
		NegotiateAfter: 4 * time.Second, NegotiateCooldown: 20 * time.Second, MaxChainHops: 8}
}

// Publisher is the minimal transport the agent needs (satisfied by transport.Transport).
type Publisher interface {
	Publish(topic string, payload []byte, retain bool) error
	Subscribe(filter string, h transport.Handler) (func(), error)
}

// Negotiation is a logged right-of-way exchange (for explainability).
type Negotiation struct {
	Peer     string    `json:"peer"` // the designated yielder (or the beneficiary when I yield)
	Resource string    `json:"resource"`
	MyCost   float64   `json:"myCost"`
	PeerCost float64   `json:"peerCost"` // the yielder's cost
	IYield   bool      `json:"iYield"`
	Chain    []string  `json:"chain"`
	At       time.Time `json:"at"`
}

// Decision is a logged evaluation of a task (for explainability).
type Decision struct {
	TaskID  string  `json:"taskId"`
	Cost    float64 `json:"cost"`
	Urgency float64 `json:"urgency"`
	Delay   float64 `json:"delayS"`
	Reason  string  `json:"reason"`
	OK      bool    `json:"ok"`
}

// Agent is the autonomous on-board decision maker.
type Agent struct {
	cfg AgentConfig
	tr  Publisher
	log *slog.Logger

	mu          sync.Mutex
	info        *VehicleInfo
	graph       *mapmgr.Graph
	mapVersion  int
	pose        core.Pose
	battery     float64
	lastNodeID  string
	loadPresent bool
	driving     bool
	hasState    bool
	tasks       []Task
	assignment  Assignment
	pending     *Claim
	claimTimer  *time.Timer
	plannedTask string // task the claim timer is armed for
	claimedTask string
	ticker      *time.Ticker
	stopCh      chan struct{}
	myTask      *Task // the task currently claimed / executed (for the yield cost)
	// negotiation
	conflict     *Conflict
	conflictSeen time.Time
	openProposal *Proposal
	lastTalk     map[string]time.Time
	negotiations []Negotiation
	yielded      int
	passed       int
	lastCharge   time.Time
	lastPark     time.Time
	idleSince    time.Time
	decisions    []Decision
	unsubs       []func()
	claims       int
	wins         int
}

// NewAgent creates an agent.
func NewAgent(cfg AgentConfig, tr Publisher, log *slog.Logger) *Agent {
	d := DefaultAgentConfig()
	if cfg.Interface == "" {
		cfg.Interface = d.Interface
	}
	if cfg.LowBatteryPct <= 0 {
		cfg.LowBatteryPct = d.LowBatteryPct
	}
	if cfg.CriticalBatteryPct <= 0 {
		cfg.CriticalBatteryPct = d.CriticalBatteryPct
	}
	if cfg.ReserveBatteryPct <= 0 {
		cfg.ReserveBatteryPct = d.ReserveBatteryPct
	}
	if cfg.ClaimScale <= 0 {
		cfg.ClaimScale = d.ClaimScale
	}
	if cfg.MinClaimDelay <= 0 {
		cfg.MinClaimDelay = d.MinClaimDelay
	}
	if cfg.MaxClaimDelay <= 0 {
		cfg.MaxClaimDelay = d.MaxClaimDelay
	}
	if cfg.ParkAfterIdle <= 0 {
		cfg.ParkAfterIdle = d.ParkAfterIdle
	}
	if cfg.NegotiateAfter <= 0 {
		cfg.NegotiateAfter = d.NegotiateAfter
	}
	if cfg.NegotiateCooldown <= 0 {
		cfg.NegotiateCooldown = d.NegotiateCooldown
	}
	if cfg.MaxChainHops <= 0 {
		cfg.MaxChainHops = d.MaxChainHops
	}
	if log == nil {
		log = slog.Default()
	}
	return &Agent{cfg: cfg, tr: tr, log: log.With("agent", cfg.VehicleID), idleSince: time.Now(), lastTalk: map[string]time.Time{}}
}

// Start subscribes to the board, the map, the registry and the vehicle's own state.
func (a *Agent) Start() error {
	c := a.cfg
	subs := map[string]transport.Handler{
		TopicTasks(c.SiteID):                                                     a.onTasks,
		TopicGrants(c.SiteID, c.VehicleID):                                       a.onGrant,
		TopicAssignments(c.SiteID, c.VehicleID):                                  a.onAssignment,
		TopicMap(c.SiteID):                                                       a.onMap,
		TopicVehicle(c.SiteID, c.VehicleID):                                      a.onVehicleInfo,
		TopicConflicts(c.SiteID, c.VehicleID):                                    a.onConflict,
		TopicInbox(c.SiteID, c.VehicleID):                                        a.onInbox,
		vda5050.Topic(c.Interface, c.Manufacturer, c.Serial, vda5050.TopicState): a.onState,
	}
	for topic, h := range subs {
		u, err := a.tr.Subscribe(topic, h)
		if err != nil {
			a.Stop()
			return err
		}
		a.unsubs = append(a.unsubs, u)
	}
	// periodic re-evaluation (urgency grows with time, states change)
	a.stopCh = make(chan struct{})
	a.ticker = time.NewTicker(2 * time.Second)
	go func() {
		for {
			select {
			case <-a.stopCh:
				return
			case <-a.ticker.C:
				a.evaluate()
				a.negotiate()
			}
		}
	}()
	return nil
}

// Stop unsubscribes.
func (a *Agent) Stop() {
	a.mu.Lock()
	if a.claimTimer != nil {
		a.claimTimer.Stop()
	}
	if a.stopCh != nil {
		close(a.stopCh)
		a.ticker.Stop()
		a.stopCh = nil
	}
	a.mu.Unlock()
	for _, u := range a.unsubs {
		u()
	}
	a.unsubs = nil
}

func (a *Agent) onVehicleInfo(_ string, p []byte) {
	var vi VehicleInfo
	if json.Unmarshal(p, &vi) != nil {
		return
	}
	a.mu.Lock()
	a.info = &vi
	a.mu.Unlock()
	a.evaluate()
}

func (a *Agent) onMap(_ string, p []byte) {
	var m core.Map
	if json.Unmarshal(p, &m) != nil {
		return
	}
	if err := mapmgr.Validate(&m); err != nil {
		return
	}
	a.mu.Lock()
	a.graph = mapmgr.NewGraph(&m)
	a.mapVersion = m.Version
	a.mu.Unlock()
	a.evaluate()
}

func (a *Agent) onState(_ string, p []byte) {
	var s vda5050.State
	if json.Unmarshal(p, &s) != nil {
		return
	}
	a.mu.Lock()
	if s.AGVPosition != nil {
		a.pose = core.Pose{X: s.AGVPosition.X, Y: s.AGVPosition.Y, Theta: s.AGVPosition.Theta}
	}
	a.battery = s.BatteryState.BatteryCharge
	a.lastNodeID = s.LastNodeID
	a.loadPresent = len(s.Loads) > 0
	a.driving = s.Driving
	first := !a.hasState
	a.hasState = true
	a.mu.Unlock()
	a.selfCare()
	if first {
		a.evaluate()
	}
}

func (a *Agent) onAssignment(_ string, p []byte) {
	var as Assignment
	if json.Unmarshal(p, &as) != nil {
		return
	}
	a.mu.Lock()
	prev := a.assignment
	a.assignment = as
	if as.Status == "idle" && prev.Status != "idle" {
		a.idleSince = time.Now()
		a.claimedTask = ""
		a.myTask = nil
	}
	a.mu.Unlock()
	a.evaluate()
}

func (a *Agent) onGrant(_ string, p []byte) {
	var g Grant
	if json.Unmarshal(p, &g) != nil {
		return
	}
	a.mu.Lock()
	if a.pending == nil || a.pending.ClaimID != g.ClaimID {
		a.mu.Unlock()
		return
	}
	a.pending = nil
	if g.Granted {
		a.wins++
		a.claimedTask = g.TaskID
		for i := range a.tasks {
			if a.tasks[i].ID == g.TaskID {
				t := a.tasks[i]
				a.myTask = &t
			}
		}
		a.log.Info("task won", "task", g.TaskID, "competing", g.Competing)
	} else {
		a.log.Debug("claim refused", "task", g.TaskID, "reason", g.Reason)
	}
	a.mu.Unlock()
	if !g.Granted {
		a.evaluate()
	}
}

func (a *Agent) onTasks(_ string, p []byte) {
	var b Board
	if json.Unmarshal(p, &b) != nil {
		return
	}
	a.mu.Lock()
	a.tasks = b.Tasks
	a.mu.Unlock()
	a.evaluate()
}

func (a *Agent) busy() bool {
	return a.assignment.Status == "assigned" || a.assignment.Status == "executing" || a.claimedTask != "" || a.pending != nil
}

// evaluate applies the response-threshold rule: every open task gets a cost
// from the agent's own planner; the agent hesitates proportionally to its cost
// over the task's urgency, then claims. Cheaper (closer, fitter) agents claim
// first, so the colony converges without any central ranking.
func (a *Agent) evaluate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busy() || !a.hasState || a.info == nil || a.graph == nil || !a.info.Enabled || a.loadPresent {
		a.disarm()
		return
	}
	cur := a.currentNode()
	if cur == "" {
		a.disarm()
		return
	}
	a.decisions = a.decisions[:0]
	best := -1
	bestDelay := time.Duration(math.MaxInt64)
	var bestCost float64
	for i, t := range a.tasks {
		d := a.assess(t, cur)
		a.decisions = append(a.decisions, d)
		if !d.OK {
			continue
		}
		delay := time.Duration(d.Delay * float64(time.Second))
		if delay < bestDelay {
			bestDelay, best, bestCost = delay, i, d.Cost
		}
	}
	if best < 0 {
		a.disarm()
		return
	}
	task := a.tasks[best]
	if a.claimTimer != nil && a.plannedTask == task.ID {
		return // the hesitation already runs for this task
	}
	a.disarm()
	a.plannedTask = task.ID
	a.claimTimer = time.AfterFunc(bestDelay, func() { a.claim(task.ID, bestCost) })
}

func (a *Agent) disarm() {
	if a.claimTimer != nil {
		a.claimTimer.Stop()
		a.claimTimer = nil
	}
	a.plannedTask = ""
}

func (a *Agent) assess(t Task, cur string) Decision {
	d := Decision{TaskID: t.ID, Urgency: t.Urgency}
	m := a.info.Model
	if t.RequiredModelID != "" && t.RequiredModelID != a.info.ModelID {
		d.Reason = "model mismatch"
		return d
	}
	if t.MassKg > m.MaxPayloadKg {
		d.Reason = "payload exceeds capacity"
		return d
	}
	if t.LoadPresent && (t.LoadLengthM > m.LengthM+2 || t.LoadWidthM > m.WidthM+2) {
		d.Reason = "load too large"
		return d
	}
	nodes := append([]string{cur}, t.Nodes...)
	route, err := planner.PlanThrough(planner.AStar{}, planner.Request{Graph: a.graph, Envelope: core.Footprint{LengthM: m.LengthM, WidthM: m.WidthM}, MassKg: t.MassKg, MaxSpeedMs: m.MaxSpeedMs, Heading: a.pose.Theta, HasHeading: true}, dedupe(nodes))
	if err != nil {
		d.Reason = "no route"
		return d
	}
	need := a.cfg.ReserveBatteryPct
	if m.BatteryCapacityKwh > 0 {
		need += route.LengthM * (m.EnergyKwhPerM + m.EnergyKwhPerTonM*t.MassKg/1000) / m.BatteryCapacityKwh * 100
	}
	if a.battery < need || a.battery < a.cfg.LowBatteryPct {
		d.Reason = fmt.Sprintf("battery %.0f%% insufficient (need %.0f%%)", a.battery, need)
		return d
	}
	// cost in seconds: travel + handling, with a mild battery penalty
	d.Cost = route.EstimatedTimeS + 16 + (100-a.battery)*0.5
	urg := t.Urgency
	if urg < 1 {
		urg = 1
	}
	delay := a.cfg.ClaimScale * d.Cost / urg * 10
	d.Delay = math.Max(a.cfg.MinClaimDelay.Seconds(), math.Min(a.cfg.MaxClaimDelay.Seconds(), delay))
	d.OK = true
	d.Reason = "candidate"
	return d
}

func dedupe(nodes []string) []string {
	out := nodes[:0:0]
	for _, n := range nodes {
		if len(out) == 0 || out[len(out)-1] != n {
			out = append(out, n)
		}
	}
	return out
}

func (a *Agent) currentNode() string {
	if a.graph == nil {
		return ""
	}
	p := core.Point{X: a.pose.X, Y: a.pose.Y}
	if n := a.graph.Nodes[a.lastNodeID]; n != nil && n.Point().Dist(p) <= 3 {
		return n.ID
	}
	if n, d := a.graph.NearestNode(p); n != nil && d <= 1.5 {
		return n.ID
	}
	return ""
}

func (a *Agent) claim(taskID string, cost float64) {
	a.mu.Lock()
	a.claimTimer, a.plannedTask = nil, ""
	if a.busy() {
		a.mu.Unlock()
		return
	}
	still := false
	for _, t := range a.tasks {
		if t.ID == taskID {
			still = true
		}
	}
	if !still {
		a.mu.Unlock()
		a.evaluate()
		return
	}
	c := Claim{ClaimID: uuid.NewString(), TaskID: taskID, VehicleID: a.cfg.VehicleID, Cost: cost, At: time.Now().UTC()}
	a.pending = &c
	a.claims++
	// safety: forget a claim that never gets an answer
	time.AfterFunc(5*time.Second, func() {
		a.mu.Lock()
		if a.pending != nil && a.pending.ClaimID == c.ClaimID {
			a.pending = nil
			a.mu.Unlock()
			a.evaluate()
			return
		}
		a.mu.Unlock()
	})
	a.mu.Unlock()
	payload, _ := json.Marshal(c)
	_ = a.tr.Publish(TopicClaims(a.cfg.SiteID), payload, false)
	a.log.Debug("claim", "task", taskID, "cost", cost)
}

// selfCare applies the individual rules: recharge when tired, park when idle.
func (a *Agent) selfCare() {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	idle := a.assignment.Status == "idle" && a.claimedTask == "" && a.pending == nil
	switch {
	case a.battery > 0 && a.battery <= a.cfg.CriticalBatteryPct && now.Sub(a.lastCharge) > time.Minute:
		a.lastCharge = now
		a.request("charge", "battery critical")
	case idle && a.battery > 0 && a.battery <= a.cfg.LowBatteryPct && now.Sub(a.lastCharge) > time.Minute:
		a.lastCharge = now
		a.request("charge", "battery low while idle")
	case idle && !a.driving && now.Sub(a.idleSince) > a.cfg.ParkAfterIdle && now.Sub(a.lastPark) > a.cfg.ParkAfterIdle && len(a.tasks) == 0:
		a.lastPark = now
		a.request("park", "idle")
	}
}

func (a *Agent) request(kind, reason string) {
	payload, _ := json.Marshal(Request{VehicleID: a.cfg.VehicleID, Type: kind, Reason: reason})
	_ = a.tr.Publish(TopicRequests(a.cfg.SiteID), payload, false)
}

// GiveBack releases the current task (e.g. the controller refuses it).
func (a *Agent) GiveBack(reason string) {
	a.mu.Lock()
	task := a.claimedTask
	if task == "" {
		task = a.assignment.TaskID
	}
	a.claimedTask = ""
	a.mu.Unlock()
	if task == "" {
		return
	}
	payload, _ := json.Marshal(Release{VehicleID: a.cfg.VehicleID, TaskID: task, Reason: reason})
	_ = a.tr.Publish(TopicReleases(a.cfg.SiteID), payload, false)
}

// Snapshot describes the agent for diagnostics.
type Snapshot struct {
	VehicleID    string        `json:"vehicleId"`
	Battery      float64       `json:"battery"`
	Node         string        `json:"node"`
	Assignment   Assignment    `json:"assignment"`
	Claims       int           `json:"claims"`
	Wins         int           `json:"wins"`
	Decisions    []Decision    `json:"decisions"`
	Yielded      int           `json:"yielded"`
	Passed       int           `json:"passed"`
	Negotiations []Negotiation `json:"negotiations"`
}

// Snapshot returns the current view of the agent.
func (a *Agent) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	ds := append([]Decision(nil), a.decisions...)
	sort.Slice(ds, func(i, j int) bool { return ds[i].Delay < ds[j].Delay })
	ng := append([]Negotiation(nil), a.negotiations...)
	if ng == nil {
		ng = []Negotiation{}
	}
	return Snapshot{VehicleID: a.cfg.VehicleID, Battery: a.battery, Node: a.currentNode(), Assignment: a.assignment, Claims: a.claims, Wins: a.wins, Decisions: ds,
		Yielded: a.yielded, Passed: a.passed, Negotiations: ng}
}

// ---------------------------------------------------------------------------
// Local negotiation of right of way

func (a *Agent) onConflict(_ string, p []byte) {
	var cf Conflict
	if json.Unmarshal(p, &cf) != nil {
		return
	}
	a.mu.Lock()
	if !cf.Active || len(cf.HeldBy) == 0 {
		a.conflict = nil
	} else if a.conflict == nil || a.conflict.Resource.Key() != cf.Resource.Key() {
		a.conflict = &cf
		a.conflictSeen = time.Now()
	}
	a.mu.Unlock()
	a.negotiate()
}

// yieldCost is the agent's own price for giving way: an idle vehicle yields for
// free; a working one weighs its task's urgency and its load; waiting a long
// time makes an agent less willing to yield again.
func (a *Agent) yieldCost() float64 {
	cost := 0.0
	if a.assignment.Status == "executing" || a.assignment.Status == "assigned" || a.claimedTask != "" {
		cost = 10
		if a.myTask != nil {
			cost += a.myTask.Urgency
		}
		if a.loadPresent {
			cost += 20
		}
	}
	if a.conflict != nil {
		waited := time.Since(a.conflictSeen).Seconds()
		cost += math.Min(waited, 60) / 3
	}
	return cost
}

// negotiate opens a proposal towards the neighbour that blocks us. The
// proposal then travels along the wait chain (see onProposal).
func (a *Agent) negotiate() {
	a.mu.Lock()
	cf := a.conflict
	if cf == nil || a.openProposal != nil || time.Since(a.conflictSeen) < a.cfg.NegotiateAfter {
		a.mu.Unlock()
		return
	}
	peer := cf.HeldBy[0]
	if last, ok := a.lastTalk[peer]; ok && time.Since(last) < a.cfg.NegotiateCooldown {
		a.mu.Unlock()
		return
	}
	a.lastTalk[peer] = time.Now()
	pr := Proposal{Nonce: uuid.NewString(), From: a.cfg.VehicleID, To: peer, Hops: 1, At: time.Now().UTC(),
		Chain: []ChainMember{{VehicleID: a.cfg.VehicleID, YieldCost: a.yieldCost(), WaitsFor: cf.Resource}}}
	a.openProposal = &pr
	nonce := pr.Nonce
	time.AfterFunc(5*time.Second, func() {
		a.mu.Lock()
		if a.openProposal != nil && a.openProposal.Nonce == nonce {
			a.openProposal = nil // no resolution: a link of the chain is not an agent (or unreachable)
		}
		a.mu.Unlock()
	})
	a.mu.Unlock()
	a.send(peer, pr)
	a.log.Debug("negotiation proposed", "to", peer, "resource", cf.Resource.Key(), "myCost", pr.Chain[0].YieldCost)
}

func (a *Agent) send(to string, msg any) {
	payload, _ := json.Marshal(msg)
	_ = a.tr.Publish(TopicInbox(a.cfg.SiteID, to), payload, false)
}

func (a *Agent) onInbox(_ string, p []byte) {
	var probe struct {
		Yielder *string `json:"yielder"`
		Chain   []any   `json:"chain"`
	}
	if json.Unmarshal(p, &probe) != nil {
		return
	}
	if probe.Yielder != nil {
		var r Resolution
		if json.Unmarshal(p, &r) == nil {
			a.onResolution(r)
		}
		return
	}
	var pr Proposal
	if json.Unmarshal(p, &pr) == nil && pr.From != "" {
		a.onProposal(pr)
	}
}

// onProposal either forwards the proposal to the agent blocking us (we are a
// link of the chain) or, as the terminal link, decides and notifies everyone.
func (a *Agent) onProposal(pr Proposal) {
	a.mu.Lock()
	for _, m := range pr.Chain {
		if m.VehicleID == a.cfg.VehicleID {
			a.mu.Unlock()
			return // already a member: stale duplicate
		}
	}
	me := ChainMember{VehicleID: a.cfg.VehicleID, YieldCost: a.yieldCost()}
	if a.conflict != nil {
		me.WaitsFor = a.conflict.Resource
	}
	chain := append(append([]ChainMember(nil), pr.Chain...), me)
	a.lastTalk[pr.From] = time.Now()
	next := ""
	if a.conflict != nil && len(a.conflict.HeldBy) > 0 {
		next = a.conflict.HeldBy[0]
	}
	inChain := false
	for _, m := range chain {
		if m.VehicleID == next {
			inChain = true
		}
	}
	cycle := next != "" && inChain
	terminal := next == "" || cycle || pr.Hops >= a.cfg.MaxChainHops
	a.mu.Unlock()
	if !terminal {
		fwd := Proposal{Nonce: pr.Nonce, From: a.cfg.VehicleID, To: next, Chain: chain, Hops: pr.Hops + 1, At: pr.At}
		a.send(next, fwd)
		a.log.Debug("negotiation forwarded along the wait chain", "to", next, "hops", fwd.Hops)
		return
	}
	yielder, beneficiary := DecideChain(chain, cycle)
	res := Resolution{Nonce: pr.Nonce, From: a.cfg.VehicleID, Chain: chain, Yielder: yielder, Beneficiary: beneficiary, Cycle: cycle}
	for _, m := range chain {
		if m.VehicleID == a.cfg.VehicleID {
			a.onResolution(res)
		} else {
			a.send(m.VehicleID, res)
		}
	}
}

// onResolution applies the chain decision locally.
func (a *Agent) onResolution(r Resolution) {
	a.mu.Lock()
	if a.openProposal != nil && a.openProposal.Nonce == r.Nonce {
		a.openProposal = nil
	}
	var mine, yc float64
	var resource core.ResourceRef
	member := false
	for i, m := range r.Chain {
		if m.VehicleID == a.cfg.VehicleID {
			mine, member = m.YieldCost, true
			if i > 0 {
				resource = r.Chain[i-1].WaitsFor // what my predecessor waits for: a resource I hold
			}
		}
		if m.VehicleID == r.Yielder {
			yc = m.YieldCost
		}
	}
	if !member {
		a.mu.Unlock()
		return
	}
	ids := make([]string, 0, len(r.Chain))
	for _, m := range r.Chain {
		ids = append(ids, m.VehicleID)
	}
	iYield := r.Yielder == a.cfg.VehicleID && r.Beneficiary != ""
	peer := r.Yielder
	if iYield {
		peer = r.Beneficiary
		// the resource the beneficiary waits for
		for _, m := range r.Chain {
			if m.VehicleID == r.Beneficiary {
				resource = m.WaitsFor
			}
		}
	}
	a.negotiations = append(a.negotiations, Negotiation{Peer: peer, Resource: resource.Key(), MyCost: mine, PeerCost: yc, IYield: iYield, Chain: ids, At: time.Now().UTC()})
	if len(a.negotiations) > 20 {
		a.negotiations = a.negotiations[len(a.negotiations)-20:]
	}
	if !iYield {
		a.passed++
	}
	a.mu.Unlock()
	if iYield {
		a.publishYield(r.Nonce, r.Beneficiary, resource, mine, yc, ids)
	}
}

func (a *Agent) publishYield(nonce, beneficiary string, res core.ResourceRef, my, peer float64, chain []string) {
	a.mu.Lock()
	a.yielded++
	a.conflict = nil
	a.mu.Unlock()
	y := Yield{Nonce: nonce, VehicleID: a.cfg.VehicleID, Beneficiary: beneficiary, Resource: res, MyCost: my, PeerCost: peer, Chain: chain,
		Reason: fmt.Sprintf("negotiated along a chain of %d: my yield cost %.1f is the lowest", len(chain), my), At: time.Now().UTC()}
	payload, _ := json.Marshal(y)
	_ = a.tr.Publish(TopicYields(a.cfg.SiteID), payload, false)
	a.log.Info("giving way", "to", beneficiary, "resource", res.Key(), "myCost", my, "peerCost", peer)
}
