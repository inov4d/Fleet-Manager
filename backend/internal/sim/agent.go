// Package sim simulates AGVs that speak VDA 5050 over the transport layer, so
// the fleet manager exercises exactly the same communication path as with
// real vehicles. The kinematic model is omnidirectional (four steerable
// bogies): the body rotates to the ordered orientation, then translates in
// any direction with forward / lateral speed limits and acceleration limits.
package sim

import (
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/transport"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

// AgentConfig describes a simulated vehicle.
type AgentConfig struct {
	VehicleID    string
	Manufacturer string
	Serial       string
	Interface    string
	MapID        string
	Model        core.VehicleModel
	Pose         core.Pose
	BatteryPct   float64
	LastNodeID   string
	StatePeriod  time.Duration // sim time, default 1s
	VisPeriod    time.Duration // sim time, default 200ms
	ChargerKw    float64       // default 50
}

// FaultType is an injectable fault.
type FaultType string

const (
	FaultCommsLoss FaultType = "commsLoss"
	FaultBlocked   FaultType = "blocked"
	FaultError     FaultType = "error"
	FaultEStop     FaultType = "estop"
	FaultRestore   FaultType = "restore"
)

type runningAction struct {
	action  vda5050.Action
	endsAt  time.Time
	nodeIdx int
}

// Agent is one simulated AGV.
type Agent struct {
	cfg   AgentConfig
	tr    transport.Transport
	clock core.Clock

	mu            sync.Mutex
	pose          core.Pose
	speed         float64 // current translation speed
	vel           core.Velocity
	battery       float64
	charging      bool
	paused        bool
	driving       bool
	order         *vda5050.Order
	nodeIdx       int // index in order.Nodes of the next node to reach
	lastNodeID    string
	lastNodeSeq   int
	actionStates  []vda5050.ActionState
	running       *runningAction
	errors        []vda5050.Error
	eStop         vda5050.EStop
	loads         []vda5050.Load
	commsLoss     bool
	blocked       bool
	hdrState      int
	hdrVis        int
	hdrConn       int
	odometer      float64
	lastStateAt   time.Time
	lastVisAt     time.Time
	dirty         bool
	unsubs        []func()
	started       bool
	simTime       time.Time
	nodeArrivedAt time.Time
}

// NewAgent creates a simulated vehicle.
func NewAgent(cfg AgentConfig, tr transport.Transport, clock core.Clock) *Agent {
	if cfg.StatePeriod <= 0 {
		cfg.StatePeriod = time.Second
	}
	if cfg.VisPeriod <= 0 {
		cfg.VisPeriod = 200 * time.Millisecond
	}
	if cfg.Interface == "" {
		cfg.Interface = vda5050.DefaultInterface
	}
	if cfg.ChargerKw <= 0 {
		cfg.ChargerKw = 50
	}
	if cfg.BatteryPct <= 0 {
		cfg.BatteryPct = 100
	}
	return &Agent{cfg: cfg, tr: tr, clock: clock, pose: cfg.Pose, battery: cfg.BatteryPct, lastNodeID: cfg.LastNodeID,
		eStop: vda5050.EStopNone, errors: []vda5050.Error{}, actionStates: []vda5050.ActionState{}, simTime: clock.Now()}
}

func (a *Agent) topic(t string) string {
	return vda5050.Topic(a.cfg.Interface, a.cfg.Manufacturer, a.cfg.Serial, t)
}

// Start subscribes to orders / instant actions and announces ONLINE.
func (a *Agent) Start() error {
	u1, err := a.tr.Subscribe(a.topic(vda5050.TopicOrder), a.onOrder)
	if err != nil {
		return err
	}
	u2, err := a.tr.Subscribe(a.topic(vda5050.TopicInstantActions), a.onInstantActions)
	if err != nil {
		u1()
		return err
	}
	a.mu.Lock()
	a.unsubs = []func(){u1, u2}
	a.started = true
	a.mu.Unlock()
	a.publishConnection(vda5050.ConnOnline)
	a.publishFactsheet()
	a.publishState()
	return nil
}

// Stop announces OFFLINE and unsubscribes.
func (a *Agent) Stop() {
	a.publishConnection(vda5050.ConnOffline)
	a.mu.Lock()
	for _, u := range a.unsubs {
		u()
	}
	a.unsubs = nil
	a.started = false
	a.mu.Unlock()
}

func (a *Agent) header(n *int) vda5050.Header {
	*n++
	return vda5050.Header{HeaderID: *n, Timestamp: a.clock.Now(), Version: vda5050.Version, Manufacturer: a.cfg.Manufacturer, SerialNumber: a.cfg.Serial}
}

func (a *Agent) publishConnection(s vda5050.ConnectionState) {
	a.mu.Lock()
	if a.commsLoss {
		a.mu.Unlock()
		return
	}
	msg := vda5050.Connection{Header: a.header(&a.hdrConn), ConnectionState: s}
	a.mu.Unlock()
	b, _ := json.Marshal(msg)
	_ = a.tr.Publish(a.topic(vda5050.TopicConnection), b, true)
}

func (a *Agent) publishFactsheet() {
	a.mu.Lock()
	m := a.cfg.Model
	fs := vda5050.Factsheet{Header: a.header(&a.hdrConn),
		TypeSpecification: vda5050.TypeSpecification{SeriesName: m.Name, AGVKinematic: "OMNI", AGVClass: "CARRIER", MaxLoadMass: m.MaxPayloadKg,
			LocalizationTypes: []string{"NATURAL"}, NavigationTypes: []string{"AUTONOMOUS"}},
		PhysicalParameters: vda5050.PhysicalParameters{SpeedMax: m.MaxSpeedMs, AccelerationMax: m.AccelMs2, DecelerationMax: m.AccelMs2, HeightMin: m.HeightM, HeightMax: m.HeightM, Width: m.WidthM, Length: m.LengthM},
		ProtocolLimits:     map[string]any{}, ProtocolFeatures: map[string]any{"optionalParameters": []any{}, "agvActions": []any{}},
		AGVGeometry: map[string]any{"wheelDefinitions": []any{}}, LoadSpecification: map[string]any{"loadPositions": []any{}}}
	a.mu.Unlock()
	b, _ := json.Marshal(fs)
	_ = a.tr.Publish(a.topic(vda5050.TopicFactsheet), b, true)
}

func (a *Agent) buildState() vda5050.State {
	paused := a.paused
	nbr := a.order != nil && a.nodeIdx < len(a.order.Nodes) && !a.order.Nodes[a.nodeIdx].Released
	dist := 0.0
	s := vda5050.State{Header: a.header(&a.hdrState), OperatingMode: vda5050.OpAutomatic, Driving: a.driving, Paused: &paused, NewBaseRequest: &nbr,
		DistanceSinceLastNode: &dist, LastNodeID: a.lastNodeID, LastNodeSequenceID: a.lastNodeSeq,
		NodeStates: []vda5050.NodeState{}, EdgeStates: []vda5050.EdgeState{}, ActionStates: append([]vda5050.ActionState{}, a.actionStates...),
		BatteryState: vda5050.BatteryState{BatteryCharge: math.Round(a.battery*10) / 10, Charging: a.charging, BatteryVoltage: f(48 + a.battery/100*6), BatteryHealth: f(97)},
		Errors:       append([]vda5050.Error{}, a.errors...), SafetyState: vda5050.SafetyState{EStop: a.eStop, FieldViolation: false},
		AGVPosition: &vda5050.AGVPosition{X: a.pose.X, Y: a.pose.Y, Theta: a.pose.Theta, MapID: a.cfg.MapID, PositionInitialized: true, LocalizationScore: f(0.95)},
		Velocity:    &vda5050.Velocity{VX: f(a.vel.VX), VY: f(a.vel.VY), Omega: f(a.vel.Omega)}, Loads: append([]vda5050.Load{}, a.loads...)}
	if a.order != nil {
		s.OrderID = a.order.OrderID
		s.OrderUpdateID = a.order.OrderUpdateID
		for i := a.nodeIdx; i < len(a.order.Nodes); i++ {
			n := a.order.Nodes[i]
			s.NodeStates = append(s.NodeStates, vda5050.NodeState{NodeID: n.NodeID, SequenceID: n.SequenceID, Released: n.Released, NodePosition: n.NodePosition})
			if i-1 >= 0 && i-1 < len(a.order.Edges) {
				e := a.order.Edges[i-1]
				s.EdgeStates = append(s.EdgeStates, vda5050.EdgeState{EdgeID: e.EdgeID, SequenceID: e.SequenceID, Released: e.Released})
			}
		}
	}
	return s
}

func (a *Agent) publishState() {
	a.mu.Lock()
	if a.commsLoss || !a.started {
		a.mu.Unlock()
		return
	}
	s := a.buildState()
	a.lastStateAt = a.simTime
	a.dirty = false
	a.mu.Unlock()
	b, _ := json.Marshal(s)
	_ = a.tr.Publish(a.topic(vda5050.TopicState), b, false)
}

func (a *Agent) publishVisualization() {
	a.mu.Lock()
	if a.commsLoss || !a.started {
		a.mu.Unlock()
		return
	}
	v := vda5050.Visualization{Header: a.header(&a.hdrVis), AGVPosition: &vda5050.AGVPosition{X: a.pose.X, Y: a.pose.Y, Theta: a.pose.Theta, MapID: a.cfg.MapID, PositionInitialized: true},
		Velocity: &vda5050.Velocity{VX: f(a.vel.VX), VY: f(a.vel.VY), Omega: f(a.vel.Omega)}}
	a.lastVisAt = a.simTime
	a.mu.Unlock()
	b, _ := json.Marshal(v)
	_ = a.tr.Publish(a.topic(vda5050.TopicVisualization), b, false)
}

func (a *Agent) addError(t, desc string, level vda5050.ErrorLevel) {
	for _, e := range a.errors {
		if e.ErrorType == t {
			return
		}
	}
	a.errors = append(a.errors, vda5050.Error{ErrorType: t, ErrorDescription: desc, ErrorLevel: level})
	a.dirty = true
}

func (a *Agent) clearError(t string) {
	out := a.errors[:0]
	for _, e := range a.errors {
		if e.ErrorType != t {
			out = append(out, e)
		}
	}
	a.errors = out
}

// onOrder handles orders and order updates following VDA 5050 section 6.6.
func (a *Agent) onOrder(_ string, payload []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.commsLoss {
		return
	}
	var o vda5050.Order
	if err := json.Unmarshal(payload, &o); err != nil {
		a.addError("validationError", "order json: "+err.Error(), vda5050.ErrorWarning)
		return
	}
	if err := vda5050.ValidateOrder(&o); err != nil {
		a.addError("validationError", err.Error(), vda5050.ErrorWarning)
		return
	}
	a.clearError("validationError")
	a.clearError("orderUpdateError")
	if a.order != nil && a.order.OrderID == o.OrderID {
		if o.OrderUpdateID <= a.order.OrderUpdateID {
			return // old or duplicate update
		}
		// stitch: first node of the update must be the last node of the current base
		idx := -1
		for i, n := range a.order.Nodes {
			if n.SequenceID == o.Nodes[0].SequenceID && n.NodeID == o.Nodes[0].NodeID {
				idx = i
			}
		}
		if idx < 0 {
			a.addError("orderUpdateError", "update does not stitch to current order", vda5050.ErrorWarning)
			return
		}
		merged := &vda5050.Order{Header: o.Header, OrderID: o.OrderID, OrderUpdateID: o.OrderUpdateID}
		merged.Nodes = append(append([]vda5050.Node{}, a.order.Nodes[:idx]...), o.Nodes...)
		merged.Edges = append(append([]vda5050.Edge{}, a.order.Edges[:idx]...), o.Edges...)
		a.order = merged
		if a.nodeIdx > len(a.order.Nodes) {
			a.nodeIdx = len(a.order.Nodes)
		}
		a.dirty = true
		return
	}
	// new order: accepted when idle, or when its first node is where we stand
	if a.order != nil && a.nodeIdx < len(a.order.Nodes) && a.order.Nodes[a.nodeIdx].Released && a.driving {
		// still executing a released base: the new order must start at the last node reached
		if o.Nodes[0].NodeID != a.lastNodeID {
			a.addError("orderUpdateError", "new order while executing base", vda5050.ErrorWarning)
			return
		}
	}
	a.order = &o
	a.actionStates = a.actionStates[:0]
	a.running = nil
	a.lastNodeSeq = o.Nodes[0].SequenceID
	a.lastNodeID = o.Nodes[0].NodeID
	a.nodeIdx = 1
	a.speed = 0
	for _, n := range o.Nodes {
		for _, act := range n.Actions {
			a.actionStates = append(a.actionStates, vda5050.ActionState{ActionID: act.ActionID, ActionType: act.ActionType, ActionStatus: vda5050.StatusWaiting})
		}
	}
	if len(o.Nodes) == 1 {
		// a single-node order: execute its actions in place
		a.nodeIdx = 1
		a.startNodeActions(0)
	} else {
		// actions on the first node (where we stand) run before moving
		a.startNodeActions(0)
	}
	a.dirty = true
}

func (a *Agent) onInstantActions(_ string, payload []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.commsLoss {
		return
	}
	var ia vda5050.InstantActions
	if err := json.Unmarshal(payload, &ia); err != nil {
		return
	}
	for _, act := range ia.Actions {
		st := vda5050.ActionState{ActionID: act.ActionID, ActionType: act.ActionType, ActionStatus: vda5050.StatusFinished}
		switch act.ActionType {
		case vda5050.ActionStartPause:
			a.paused = true
		case vda5050.ActionStopPause:
			a.paused = false
		case vda5050.ActionCancelOrder:
			a.cancelOrder()
		case vda5050.ActionStartCharging:
			a.charging = true
		case vda5050.ActionStopCharging:
			a.charging = false
		case vda5050.ActionInitPosition:
			for _, p := range act.ActionParameters {
				switch p.Key {
				case "x":
					a.pose.X = toF(p.Value)
				case "y":
					a.pose.Y = toF(p.Value)
				case "theta":
					a.pose.Theta = toF(p.Value)
				case "lastNodeId":
					a.lastNodeID, _ = p.Value.(string)
				}
			}
		case vda5050.ActionStateRequest:
			a.dirty = true
		case vda5050.ActionFactsheetRequest:
			go a.publishFactsheet()
		default:
			st.ActionStatus = vda5050.StatusFailed
			st.ResultDescription = "unsupported instant action"
		}
		a.actionStates = append(a.actionStates, st)
	}
	a.dirty = true
}

func (a *Agent) cancelOrder() {
	for i := range a.actionStates {
		if a.actionStates[i].ActionStatus == vda5050.StatusWaiting || a.actionStates[i].ActionStatus == vda5050.StatusRunning {
			a.actionStates[i].ActionStatus = vda5050.StatusFailed
			a.actionStates[i].ResultDescription = "cancelled"
		}
	}
	a.order = nil
	a.running = nil
	a.nodeIdx = 0
	a.driving = false
	a.speed = 0
	a.vel = core.Velocity{}
}

func (a *Agent) startNodeActions(nodeIdx int) {
	if a.order == nil || nodeIdx >= len(a.order.Nodes) {
		return
	}
	for _, act := range a.order.Nodes[nodeIdx].Actions {
		if a.actionStatus(act.ActionID) == vda5050.StatusWaiting {
			dur := actionDuration(act)
			a.setActionStatus(act.ActionID, vda5050.StatusRunning, "")
			a.running = &runningAction{action: act, endsAt: a.simTime.Add(dur), nodeIdx: nodeIdx}
			if act.ActionType == vda5050.ActionStartCharging {
				a.charging = true
			}
			return // one blocking action at a time
		}
	}
}

func actionDuration(act vda5050.Action) time.Duration {
	switch act.ActionType {
	case vda5050.ActionPick, vda5050.ActionDrop:
		return 8 * time.Second
	case vda5050.ActionWaitForTrigger, "wait":
		for _, p := range act.ActionParameters {
			if p.Key == "duration" {
				return time.Duration(toF(p.Value) * float64(time.Second))
			}
		}
		return 5 * time.Second
	case vda5050.ActionStartCharging:
		return 2 * time.Second
	default:
		return time.Second
	}
}

func (a *Agent) actionStatus(id string) vda5050.ActionStatus {
	for _, s := range a.actionStates {
		if s.ActionID == id {
			return s.ActionStatus
		}
	}
	return ""
}

func (a *Agent) setActionStatus(id string, st vda5050.ActionStatus, result string) {
	for i := range a.actionStates {
		if a.actionStates[i].ActionID == id {
			a.actionStates[i].ActionStatus = st
			a.actionStates[i].ResultDescription = result
			a.dirty = true
		}
	}
}

func (a *Agent) finishAction(r *runningAction) {
	switch r.action.ActionType {
	case vda5050.ActionPick:
		w := 0.0
		id, typ := "", "pallet"
		for _, p := range r.action.ActionParameters {
			switch p.Key {
			case "weight":
				w = toF(p.Value)
			case "loadId":
				id, _ = p.Value.(string)
			case "loadType":
				typ, _ = p.Value.(string)
			}
		}
		if id == "" {
			id = "load-" + r.action.ActionID
		}
		a.loads = append(a.loads, vda5050.Load{LoadID: id, LoadType: typ, Weight: f(w)})
	case vda5050.ActionDrop:
		a.loads = nil
	}
	a.setActionStatus(r.action.ActionID, vda5050.StatusFinished, "")
	a.running = nil
}

// Step advances the simulation by dt seconds of simulated time.
func (a *Agent) Step(dt float64) {
	a.mu.Lock()
	a.simTime = a.simTime.Add(time.Duration(dt * float64(time.Second)))
	m := a.cfg.Model
	// battery
	if a.charging {
		a.battery += a.cfg.ChargerKw / m.BatteryCapacityKwh * 100 * dt / 3600
		if a.battery >= 100 {
			a.battery = 100
		}
	} else {
		a.battery -= 0.02 / 60 * dt // idle consumption
	}
	// blocking actions
	if a.running != nil {
		if !a.simTime.Before(a.running.endsAt) {
			a.finishAction(a.running)
			a.startNodeActions(a.nodeIdx - 1)
		}
	}
	canMove := a.order != nil && a.running == nil && !a.paused && !a.blocked && a.eStop == vda5050.EStopNone && !a.hasFatal() && !a.charging
	moved := false
	if canMove && a.nodeIdx < len(a.order.Nodes) && a.order.Nodes[a.nodeIdx].Released {
		moved = a.moveToward(dt)
	}
	if !moved {
		a.speed = 0
		a.vel = core.Velocity{}
		if a.driving {
			a.driving = false
			a.dirty = true
		}
	}
	if a.battery < 0 {
		a.battery = 0
		a.addError("batteryEmpty", "battery depleted", vda5050.ErrorFatal)
	}
	needState := a.dirty || a.simTime.Sub(a.lastStateAt) >= a.cfg.StatePeriod
	needVis := a.simTime.Sub(a.lastVisAt) >= a.cfg.VisPeriod
	a.mu.Unlock()
	if needState {
		a.publishState()
	}
	if needVis {
		a.publishVisualization()
	}
}

func (a *Agent) hasFatal() bool {
	for _, e := range a.errors {
		if e.ErrorLevel == vda5050.ErrorFatal {
			return true
		}
	}
	return false
}

// moveToward performs one kinematic step toward the next released node.
func (a *Agent) moveToward(dt float64) bool {
	m := a.cfg.Model
	n := a.order.Nodes[a.nodeIdx]
	if n.NodePosition == nil {
		return false
	}
	target := core.Point{X: n.NodePosition.X, Y: n.NodePosition.Y}
	var edge *vda5050.Edge
	if a.nodeIdx-1 < len(a.order.Edges) {
		edge = &a.order.Edges[a.nodeIdx-1]
	}
	maxSpeed := m.MaxSpeedMs
	if edge != nil && edge.MaxSpeed != nil && *edge.MaxSpeed > 0 && *edge.MaxSpeed < maxSpeed {
		maxSpeed = *edge.MaxSpeed
	}
	a.driving = true
	// 1. rotate to the ordered orientation
	if edge != nil && edge.Orientation != nil && (edge.RotationAllowed == nil || *edge.RotationAllowed) {
		d := normAngle(*edge.Orientation - a.pose.Theta)
		if math.Abs(d) > 0.01 {
			step := m.MaxRotationRadS * dt
			if math.Abs(d) <= step {
				a.pose.Theta = *edge.Orientation
			} else {
				a.pose.Theta = normAngle(a.pose.Theta + math.Copysign(step, d))
			}
			a.vel = core.Velocity{Omega: math.Copysign(m.MaxRotationRadS, d)}
			a.speed = 0
			return true
		}
	}
	// 2. translate
	dx, dy := target.X-a.pose.X, target.Y-a.pose.Y
	dist := math.Hypot(dx, dy)
	if dist < 0.03 {
		a.arrive(n, target)
		return true
	}
	dirX, dirY := dx/dist, dy/dist
	// body-frame direction decides forward vs lateral speed limit
	bx := dirX*math.Cos(a.pose.Theta) + dirY*math.Sin(a.pose.Theta)
	by := -dirX*math.Sin(a.pose.Theta) + dirY*math.Cos(a.pose.Theta)
	lim := maxSpeed
	if math.Abs(by) > 0.7 && m.MaxLateralSpeedMs > 0 && m.MaxLateralSpeedMs < lim {
		lim = m.MaxLateralSpeedMs
	}
	v := a.speed + m.AccelMs2*dt
	if v > lim {
		v = lim
	}
	stop := math.Sqrt(2 * m.AccelMs2 * dist)
	if v > stop {
		v = stop
	}
	if v < 0.05 {
		v = 0.05
	}
	stepLen := v * dt
	if stepLen >= dist {
		stepLen = dist
	}
	a.pose.X += dirX * stepLen
	a.pose.Y += dirY * stepLen
	a.speed = v
	a.vel = core.Velocity{VX: v * bx, VY: v * by}
	a.odometer += stepLen
	loadT := 0.0
	for _, l := range a.loads {
		if l.Weight != nil {
			loadT += *l.Weight / 1000
		}
	}
	a.battery -= (m.EnergyKwhPerM + m.EnergyKwhPerTonM*loadT) * stepLen / m.BatteryCapacityKwh * 100
	if stepLen >= dist {
		a.arrive(n, target)
	}
	return true
}

func (a *Agent) arrive(n vda5050.Node, target core.Point) {
	a.pose.X, a.pose.Y = target.X, target.Y
	if n.NodePosition != nil && n.NodePosition.Theta != nil {
		a.pose.Theta = *n.NodePosition.Theta
	}
	a.lastNodeID = n.NodeID
	a.lastNodeSeq = n.SequenceID
	a.nodeIdx++
	a.speed = 0
	a.nodeArrivedAt = a.simTime
	a.startNodeActions(a.nodeIdx - 1)
	a.dirty = true
}

// SetFault injects or clears a fault.
func (a *Agent) SetFault(t FaultType) {
	switch t {
	case FaultCommsLoss:
		a.mu.Lock()
		a.commsLoss = true
		a.mu.Unlock()
	case FaultBlocked:
		a.mu.Lock()
		a.blocked = true
		a.mu.Unlock()
	case FaultError:
		a.mu.Lock()
		a.addError("simulatedFault", "simulated drive fault", vda5050.ErrorFatal)
		a.mu.Unlock()
	case FaultEStop:
		a.mu.Lock()
		a.eStop = vda5050.EStopManual
		a.dirty = true
		a.mu.Unlock()
	case FaultRestore:
		a.mu.Lock()
		wasLoss := a.commsLoss
		a.commsLoss, a.blocked = false, false
		a.clearError("simulatedFault")
		a.eStop = vda5050.EStopNone
		a.dirty = true
		a.mu.Unlock()
		if wasLoss {
			a.publishConnection(vda5050.ConnOnline)
		}
	}
	if t != FaultCommsLoss {
		a.publishState()
	}
}

// Faults returns the active faults.
func (a *Agent) Faults() []FaultType {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []FaultType
	if a.commsLoss {
		out = append(out, FaultCommsLoss)
	}
	if a.blocked {
		out = append(out, FaultBlocked)
	}
	for _, e := range a.errors {
		if e.ErrorType == "simulatedFault" {
			out = append(out, FaultError)
		}
	}
	if a.eStop != vda5050.EStopNone {
		out = append(out, FaultEStop)
	}
	return out
}

// Snapshot returns a copy of the agent's observable state (tests / diagnostics).
type Snapshot struct {
	Pose       core.Pose
	Battery    float64
	LastNodeID string
	Driving    bool
	Charging   bool
	Loads      int
	Odometer   float64
	OrderID    string
	NodeIdx    int
}

func (a *Agent) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := Snapshot{Pose: a.pose, Battery: a.battery, LastNodeID: a.lastNodeID, Driving: a.driving, Charging: a.charging, Loads: len(a.loads), Odometer: a.odometer, NodeIdx: a.nodeIdx}
	if a.order != nil {
		s.OrderID = a.order.OrderID
	}
	return s
}

// Config returns the agent configuration.
func (a *Agent) Config() AgentConfig { return a.cfg }

func normAngle(x float64) float64 { return math.Atan2(math.Sin(x), math.Cos(x)) }

func f(v float64) *float64 { return &v }

func toF(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case json.Number:
		fl, _ := x.Float64()
		return fl
	}
	return 0
}
