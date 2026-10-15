package vda5050

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTopic(t *testing.T) {
	tp := Topic("", "inov4d", "AGV 001", TopicState)
	if tp != "uagv/v2/inov4d/AGV_001/state" {
		t.Fatal(tp)
	}
	i, m, s, topic, ok := ParseTopic(tp)
	if !ok || i != "uagv" || m != "inov4d" || s != "AGV_001" || topic != "state" {
		t.Fatal("parse failed")
	}
	if Wildcard("uagv", TopicState) != "uagv/v2/+/+/state" {
		t.Fatal("wildcard")
	}
}

func TestStateRoundTripAndValidation(t *testing.T) {
	js := `{"headerId":1,"timestamp":"2026-01-01T00:00:00Z","version":"2.0.0","manufacturer":"x","serialNumber":"s1",
	"orderId":"","orderUpdateId":0,"lastNodeId":"","lastNodeSequenceId":0,"driving":false,"operatingMode":"AUTOMATIC",
	"nodeStates":[],"edgeStates":[],"actionStates":[],"batteryState":{"batteryCharge":80,"charging":false},"errors":[],
	"safetyState":{"eStop":"NONE","fieldViolation":false},"agvPosition":{"x":1,"y":2,"theta":0.5,"mapId":"m","positionInitialized":true}}`
	var s State
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		t.Fatal(err)
	}
	if err := ValidateState(&s); err != nil {
		t.Fatal(err)
	}
	s.OperatingMode = "BOGUS"
	if err := ValidateState(&s); err == nil {
		t.Fatal("expected invalid operating mode")
	}
	s.OperatingMode = OpAutomatic
	s.Version = "1.1.0"
	if err := ValidateState(&s); err == nil {
		t.Fatal("expected version mismatch")
	}
}

func TestValidateOrder(t *testing.T) {
	o := &Order{Header: Header{HeaderID: 1, Timestamp: time.Now(), Version: Version, Manufacturer: "m", SerialNumber: "s"}, OrderID: "o1",
		Nodes: []Node{{NodeID: "a", SequenceID: 0, Released: true}, {NodeID: "b", SequenceID: 2, Released: true}, {NodeID: "c", SequenceID: 4, Released: false}},
		Edges: []Edge{{EdgeID: "ab", SequenceID: 1, Released: true, StartNodeID: "a", EndNodeID: "b"}, {EdgeID: "bc", SequenceID: 3, Released: false, StartNodeID: "b", EndNodeID: "c"}}}
	if err := ValidateOrder(o); err != nil {
		t.Fatal(err)
	}
	bad := *o
	bad.Edges = []Edge{{EdgeID: "ab", SequenceID: 1, Released: true, StartNodeID: "a", EndNodeID: "b"}, {EdgeID: "bc", SequenceID: 3, Released: true, StartNodeID: "b", EndNodeID: "c"}}
	if err := ValidateOrder(&bad); err == nil {
		t.Fatal("released edge into horizon must fail")
	}
	bad2 := *o
	bad2.Nodes = []Node{{NodeID: "a", SequenceID: 0, Released: false}, {NodeID: "b", SequenceID: 2, Released: true}, {NodeID: "c", SequenceID: 4, Released: true}}
	if err := ValidateOrder(&bad2); err == nil {
		t.Fatal("released after horizon must fail")
	}
}

func TestSequencer(t *testing.T) {
	s := NewSequencer()
	if !s.Accept("k", 1) || s.Accept("k", 1) || s.Accept("k", 0) || !s.Accept("k", 2) {
		t.Fatal("sequence rules")
	}
	if !s.Accept("k", 5000) || !s.Accept("k", 1) { // restart of the vehicle
		t.Fatal("restart should be accepted")
	}
}
