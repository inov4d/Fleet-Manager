package vda5050

import (
	"fmt"
	"strings"

	"github.com/inov4d/fleet-manager/internal/core"
)

// Topic builds "<interface>/v2/<manufacturer>/<serial>/<topic>".
func Topic(iface, manufacturer, serial, topic string) string {
	if iface == "" {
		iface = DefaultInterface
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", iface, MajorVersion, sanitize(manufacturer), sanitize(serial), topic)
}

// Wildcard returns the subscription filter for a topic of every vehicle.
func Wildcard(iface, topic string) string {
	if iface == "" {
		iface = DefaultInterface
	}
	return fmt.Sprintf("%s/%s/+/+/%s", iface, MajorVersion, topic)
}

// ParseTopic extracts manufacturer, serial and topic from a topic string.
func ParseTopic(t string) (iface, manufacturer, serial, topic string, ok bool) {
	parts := strings.Split(t, "/")
	if len(parts) != 5 || parts[1] != MajorVersion {
		return "", "", "", "", false
	}
	return parts[0], parts[2], parts[3], parts[4], true
}

// sanitize removes characters that are illegal in MQTT topic levels.
func sanitize(s string) string {
	r := strings.NewReplacer("/", "_", "+", "_", "#", "_", " ", "_")
	return r.Replace(s)
}

// Validate checks a message header for protocol version compatibility.
func (h Header) Validate() error {
	if h.Version == "" {
		return core.Validation("missing version")
	}
	if !strings.HasPrefix(h.Version, "2.") {
		return core.Validation("unsupported VDA 5050 version %q (expected 2.x)", h.Version)
	}
	if h.SerialNumber == "" {
		return core.Validation("missing serialNumber")
	}
	if h.Manufacturer == "" {
		return core.Validation("missing manufacturer")
	}
	return nil
}

// ValidateState checks required state fields and enumerations.
func ValidateState(s *State) error {
	if err := s.Header.Validate(); err != nil {
		return err
	}
	switch s.OperatingMode {
	case OpAutomatic, OpSemiAutomatic, OpManual, OpService, OpTeachIn:
	default:
		return core.Validation("invalid operatingMode %q", s.OperatingMode)
	}
	switch s.SafetyState.EStop {
	case EStopAutoAck, EStopManual, EStopRemote, EStopNone:
	default:
		return core.Validation("invalid safetyState.eStop %q", s.SafetyState.EStop)
	}
	if s.BatteryState.BatteryCharge < 0 || s.BatteryState.BatteryCharge > 100 {
		return core.Validation("batteryCharge out of range")
	}
	for _, e := range s.Errors {
		if e.ErrorLevel != ErrorWarning && e.ErrorLevel != ErrorFatal {
			return core.Validation("invalid errorLevel %q", e.ErrorLevel)
		}
	}
	for _, a := range s.ActionStates {
		switch a.ActionStatus {
		case StatusWaiting, StatusInitializing, StatusRunning, StatusPaused, StatusFinished, StatusFailed:
		default:
			return core.Validation("invalid actionStatus %q", a.ActionStatus)
		}
	}
	if s.NodeStates == nil {
		s.NodeStates = []NodeState{}
	}
	if s.EdgeStates == nil {
		s.EdgeStates = []EdgeState{}
	}
	return nil
}

// ValidateOrder checks the structural rules of an order: alternating node /
// edge sequence ids, released base before horizon, edge endpoints matching.
func ValidateOrder(o *Order) error {
	if err := o.Header.Validate(); err != nil {
		return err
	}
	if o.OrderID == "" {
		return core.Validation("missing orderId")
	}
	if len(o.Nodes) == 0 {
		return core.Validation("order has no nodes")
	}
	if len(o.Edges) != len(o.Nodes)-1 {
		return core.Validation("order must have len(nodes)-1 edges")
	}
	seenUnreleased := false
	for i, n := range o.Nodes {
		if n.SequenceID != 2*i+o.Nodes[0].SequenceID {
			return core.Validation("node %s has sequenceId %d, expected %d", n.NodeID, n.SequenceID, 2*i+o.Nodes[0].SequenceID)
		}
		if !n.Released {
			seenUnreleased = true
		} else if seenUnreleased {
			return core.Validation("released node %s after horizon start", n.NodeID)
		}
		if n.Actions == nil {
			o.Nodes[i].Actions = []Action{}
		}
	}
	for i, e := range o.Edges {
		if e.SequenceID != o.Nodes[i].SequenceID+1 {
			return core.Validation("edge %s has sequenceId %d", e.EdgeID, e.SequenceID)
		}
		if e.StartNodeID != o.Nodes[i].NodeID || e.EndNodeID != o.Nodes[i+1].NodeID {
			return core.Validation("edge %s endpoints do not match nodes", e.EdgeID)
		}
		if e.Released && !(o.Nodes[i].Released && o.Nodes[i+1].Released) {
			return core.Validation("edge %s released but its nodes are not", e.EdgeID)
		}
		if e.Actions == nil {
			o.Edges[i].Actions = []Action{}
		}
	}
	return nil
}

// Sequencer tracks headerIds per topic to detect duplicates and out-of-order
// messages (section 6.2 of the specification).
type Sequencer struct {
	last map[string]int
}

func NewSequencer() *Sequencer { return &Sequencer{last: map[string]int{}} }

// Accept returns false when the headerId was already seen or is older than the
// last accepted one for the given key (serial+topic). A reset (headerId lower
// than last by more than 1000) is accepted to allow vehicle restarts.
func (s *Sequencer) Accept(key string, headerID int) bool {
	last, ok := s.last[key]
	if ok {
		if headerID == last {
			return false
		}
		if headerID < last && last-headerID < 1000 {
			return false
		}
	}
	s.last[key] = headerID
	return true
}

// Next returns the next headerId for outgoing messages of a key.
func (s *Sequencer) Next(key string) int {
	s.last[key]++
	return s.last[key]
}
