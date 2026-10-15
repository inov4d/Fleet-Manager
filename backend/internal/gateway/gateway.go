// Package gateway is the vehicle communication gateway: it maps registered
// vehicles to their VDA 5050 topics, validates and sequences incoming state /
// connection / visualization messages, detects communication loss and sends
// orders and instant actions. It is protocol-version aware and manufacturer
// independent; proprietary adapters can be added by implementing Adapter.
package gateway

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/transport"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

// StateHandler receives validated vehicle states.
type StateHandler func(vehicleID string, s *vda5050.State)

// ConnectionHandler receives connection state changes.
type ConnectionHandler func(vehicleID string, c core.Connection)

// Adapter is the per-protocol interface. The VDA 5050 implementation is the
// Gateway itself; proprietary protocols implement the same contract.
type Adapter interface {
	SendOrder(vehicleID string, o *vda5050.Order) error
	SendInstantActions(vehicleID string, actions ...vda5050.Action) error
}

type conn struct {
	vehicleID    string
	manufacturer string
	serial       string
	connState    core.Connection
	lastSeen     time.Time
	lastState    *vda5050.State
	lastOrder    *vda5050.Order
	seqOrder     int
	seqInstant   int
}

// Gateway implements the VDA 5050 side.
type Gateway struct {
	tr      transport.Transport
	iface   string
	clock   core.Clock
	log     *slog.Logger
	timeout time.Duration

	mu         sync.Mutex
	conns      map[string]*conn  // by vehicle id
	byKey      map[string]string // manufacturer/serial -> vehicle id
	discovered map[string]time.Time
	seq        *vda5050.Sequencer
	onState    StateHandler
	onConn     ConnectionHandler
	unsubs     []func()
	stats      Stats
}

// Stats are gateway counters exposed for diagnostics.
type Stats struct {
	Received   int `json:"received"`
	Duplicates int `json:"duplicates"`
	Invalid    int `json:"invalid"`
	Sent       int `json:"sent"`
}

// New creates a gateway on a transport. commsTimeout is the heartbeat window
// after which a silent vehicle is considered connectionBroken.
func New(tr transport.Transport, iface string, clock core.Clock, log *slog.Logger, commsTimeout time.Duration) *Gateway {
	if iface == "" {
		iface = vda5050.DefaultInterface
	}
	if log == nil {
		log = slog.Default()
	}
	return &Gateway{tr: tr, iface: iface, clock: clock, log: log, timeout: commsTimeout,
		conns: map[string]*conn{}, byKey: map[string]string{}, discovered: map[string]time.Time{}, seq: vda5050.NewSequencer()}
}

// Start subscribes to the vehicle topics.
func (g *Gateway) Start(onState StateHandler, onConn ConnectionHandler) error {
	g.mu.Lock()
	g.onState, g.onConn = onState, onConn
	g.mu.Unlock()
	for _, topic := range []string{vda5050.TopicState, vda5050.TopicConnection, vda5050.TopicVisualization} {
		u, err := g.tr.Subscribe(vda5050.Wildcard(g.iface, topic), g.handle)
		if err != nil {
			return err
		}
		g.unsubs = append(g.unsubs, u)
	}
	return nil
}

// Stop unsubscribes.
func (g *Gateway) Stop() {
	for _, u := range g.unsubs {
		u()
	}
	g.unsubs = nil
}

func key(manufacturer, serial string) string { return manufacturer + "/" + serial }

// Register maps a vehicle to its manufacturer / serial number.
func (g *Gateway) Register(vehicleID, manufacturer, serial string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if old := g.conns[vehicleID]; old != nil {
		delete(g.byKey, key(old.manufacturer, old.serial))
	}
	g.conns[vehicleID] = &conn{vehicleID: vehicleID, manufacturer: manufacturer, serial: serial, connState: core.ConnUnknown}
	g.byKey[key(manufacturer, serial)] = vehicleID
	delete(g.discovered, key(manufacturer, serial))
}

// Unregister forgets a vehicle.
func (g *Gateway) Unregister(vehicleID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c := g.conns[vehicleID]; c != nil {
		delete(g.byKey, key(c.manufacturer, c.serial))
		delete(g.conns, vehicleID)
	}
}

// Discovered lists manufacturer/serial pairs seen on the bus but not registered.
func (g *Gateway) Discovered() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.discovered))
	for k := range g.discovered {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Stats returns counters.
func (g *Gateway) Stats() Stats {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stats
}

func (g *Gateway) handle(topic string, payload []byte) {
	_, manufacturer, serial, kind, ok := vda5050.ParseTopic(topic)
	if !ok {
		return
	}
	g.mu.Lock()
	g.stats.Received++
	vid, known := g.byKey[key(manufacturer, serial)]
	if !known {
		g.discovered[key(manufacturer, serial)] = g.clock.Now()
		g.mu.Unlock()
		return
	}
	c := g.conns[vid]
	onState, onConn := g.onState, g.onConn
	g.mu.Unlock()

	switch kind {
	case vda5050.TopicState:
		var s vda5050.State
		if err := json.Unmarshal(payload, &s); err != nil {
			g.invalid(vid, "state", err)
			return
		}
		if err := vda5050.ValidateState(&s); err != nil {
			g.invalid(vid, "state", err)
			return
		}
		g.mu.Lock()
		if !g.seq.Accept(vid+"/state", s.HeaderID) {
			g.stats.Duplicates++
			g.mu.Unlock()
			return
		}
		c.lastSeen = g.clock.Now()
		c.lastState = &s
		changed := c.connState != core.ConnOnline
		c.connState = core.ConnOnline
		g.mu.Unlock()
		if changed && onConn != nil {
			onConn(vid, core.ConnOnline)
		}
		if onState != nil {
			onState(vid, &s)
		}
	case vda5050.TopicConnection:
		var m vda5050.Connection
		if err := json.Unmarshal(payload, &m); err != nil {
			g.invalid(vid, "connection", err)
			return
		}
		var st core.Connection
		switch m.ConnectionState {
		case vda5050.ConnOnline:
			st = core.ConnOnline
		case vda5050.ConnOffline:
			st = core.ConnOffline
		case vda5050.ConnBroken:
			st = core.ConnBroken
		default:
			g.invalid(vid, "connection", fmt.Errorf("bad connectionState %q", m.ConnectionState))
			return
		}
		g.mu.Lock()
		if st == core.ConnOnline {
			c.lastSeen = g.clock.Now()
		}
		changed := c.connState != st
		c.connState = st
		g.mu.Unlock()
		if changed && onConn != nil {
			onConn(vid, st)
		}
	case vda5050.TopicVisualization:
		g.mu.Lock()
		c.lastSeen = g.clock.Now()
		g.mu.Unlock()
	}
}

func (g *Gateway) invalid(vid, kind string, err error) {
	g.mu.Lock()
	g.stats.Invalid++
	g.mu.Unlock()
	g.log.Warn("invalid vda5050 message", "vehicle", vid, "topic", kind, "err", err)
}

// CheckTimeouts marks vehicles silent for longer than the heartbeat window as
// connectionBroken and returns their ids.
func (g *Gateway) CheckTimeouts() []string {
	g.mu.Lock()
	now := g.clock.Now()
	var lost []string
	onConn := g.onConn
	for vid, c := range g.conns {
		if c.connState == core.ConnOnline && now.Sub(c.lastSeen) > g.timeout {
			c.connState = core.ConnBroken
			lost = append(lost, vid)
		}
	}
	g.mu.Unlock()
	sort.Strings(lost)
	if onConn != nil {
		for _, vid := range lost {
			onConn(vid, core.ConnBroken)
		}
	}
	return lost
}

// Connection returns the connection state and last-seen time of a vehicle.
func (g *Gateway) Connection(vehicleID string) (core.Connection, time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c := g.conns[vehicleID]
	if c == nil {
		return core.ConnUnknown, time.Time{}
	}
	return c.connState, c.lastSeen
}

// LastState returns the last validated state of a vehicle.
func (g *Gateway) LastState(vehicleID string) *vda5050.State {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c := g.conns[vehicleID]; c != nil {
		return c.lastState
	}
	return nil
}

// LastOrder returns the last order sent to a vehicle.
func (g *Gateway) LastOrder(vehicleID string) *vda5050.Order {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c := g.conns[vehicleID]; c != nil {
		return c.lastOrder
	}
	return nil
}

func (g *Gateway) header(c *conn, seq int) vda5050.Header {
	return vda5050.Header{HeaderID: seq, Timestamp: g.clock.Now(), Version: vda5050.Version, Manufacturer: c.manufacturer, SerialNumber: c.serial}
}

// SendOrder validates and publishes an order (or order update).
func (g *Gateway) SendOrder(vehicleID string, o *vda5050.Order) error {
	g.mu.Lock()
	c := g.conns[vehicleID]
	if c == nil {
		g.mu.Unlock()
		return core.NotFound("vehicle", vehicleID)
	}
	c.seqOrder++
	o.Header = g.header(c, c.seqOrder)
	if err := vda5050.ValidateOrder(o); err != nil {
		c.seqOrder--
		g.mu.Unlock()
		return err
	}
	c.lastOrder = o
	g.stats.Sent++
	topic := vda5050.Topic(g.iface, c.manufacturer, c.serial, vda5050.TopicOrder)
	g.mu.Unlock()
	b, err := json.Marshal(o)
	if err != nil {
		return err
	}
	g.log.Debug("order sent", "vehicle", vehicleID, "orderId", o.OrderID, "update", o.OrderUpdateID, "nodes", len(o.Nodes))
	return g.tr.Publish(topic, b, false)
}

// SendInstantActions publishes instant actions.
func (g *Gateway) SendInstantActions(vehicleID string, actions ...vda5050.Action) error {
	g.mu.Lock()
	c := g.conns[vehicleID]
	if c == nil {
		g.mu.Unlock()
		return core.NotFound("vehicle", vehicleID)
	}
	c.seqInstant++
	msg := vda5050.InstantActions{Header: g.header(c, c.seqInstant), Actions: actions}
	g.stats.Sent++
	topic := vda5050.Topic(g.iface, c.manufacturer, c.serial, vda5050.TopicInstantActions)
	g.mu.Unlock()
	for i := range msg.Actions {
		if msg.Actions[i].BlockingType == "" {
			msg.Actions[i].BlockingType = vda5050.BlockingHard
		}
		if msg.Actions[i].ActionParameters == nil {
			msg.Actions[i].ActionParameters = []vda5050.ActionParameter{}
		}
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return g.tr.Publish(topic, b, false)
}

// Convenience instant actions ---------------------------------------------

func (g *Gateway) Pause(vehicleID string) error {
	return g.SendInstantActions(vehicleID, vda5050.Action{ActionType: vda5050.ActionStartPause, ActionID: "pause-" + newID(), BlockingType: vda5050.BlockingHard})
}

func (g *Gateway) Resume(vehicleID string) error {
	return g.SendInstantActions(vehicleID, vda5050.Action{ActionType: vda5050.ActionStopPause, ActionID: "resume-" + newID(), BlockingType: vda5050.BlockingHard})
}

func (g *Gateway) CancelOrder(vehicleID string) error {
	return g.SendInstantActions(vehicleID, vda5050.Action{ActionType: vda5050.ActionCancelOrder, ActionID: "cancel-" + newID(), BlockingType: vda5050.BlockingHard})
}

func (g *Gateway) StartCharging(vehicleID string) error {
	return g.SendInstantActions(vehicleID, vda5050.Action{ActionType: vda5050.ActionStartCharging, ActionID: "charge-" + newID(), BlockingType: vda5050.BlockingHard})
}

func (g *Gateway) StopCharging(vehicleID string) error {
	return g.SendInstantActions(vehicleID, vda5050.Action{ActionType: vda5050.ActionStopCharging, ActionID: "stopcharge-" + newID(), BlockingType: vda5050.BlockingHard})
}

func (g *Gateway) RequestState(vehicleID string) error {
	return g.SendInstantActions(vehicleID, vda5050.Action{ActionType: vda5050.ActionStateRequest, ActionID: "state-" + newID(), BlockingType: vda5050.BlockingNone})
}

var idCounter int64
var idMu sync.Mutex

func newID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return fmt.Sprintf("%d-%d", time.Now().UnixMilli(), idCounter)
}
