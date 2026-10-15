// Package transport abstracts the publish/subscribe layer used by the vehicle
// gateway and the simulator: an in-memory bus for tests and single-process
// deployments, and MQTT (Paho client) for real vehicles. An embedded MQTT
// broker (Mochi) is provided so the product runs without external services.
package transport

import (
	"strings"
	"sync"
)

// Handler receives messages.
type Handler func(topic string, payload []byte)

// Transport is a minimal MQTT-like pub/sub abstraction.
type Transport interface {
	Publish(topic string, payload []byte, retain bool) error
	Subscribe(filter string, h Handler) (unsubscribe func(), err error)
	Close() error
}

// Match implements MQTT topic filter matching (+ and #).
func Match(filter, topic string) bool {
	f := strings.Split(filter, "/")
	t := strings.Split(topic, "/")
	for i := 0; i < len(f); i++ {
		if f[i] == "#" {
			return true
		}
		if i >= len(t) {
			return false
		}
		if f[i] != "+" && f[i] != t[i] {
			return false
		}
	}
	return len(f) == len(t)
}

// Memory is an in-process transport. Handlers run synchronously on the
// publisher goroutine unless Async is set.
type Memory struct {
	mu       sync.RWMutex
	subs     map[int]*memSub
	next     int
	retained map[string][]byte
	Async    bool
}

type memSub struct {
	filter string
	h      Handler
}

func NewMemory() *Memory { return &Memory{subs: map[int]*memSub{}, retained: map[string][]byte{}} }

func (m *Memory) Publish(topic string, payload []byte, retain bool) error {
	m.mu.Lock()
	if retain {
		if len(payload) == 0 {
			delete(m.retained, topic)
		} else {
			m.retained[topic] = append([]byte(nil), payload...)
		}
	}
	var hs []Handler
	for _, s := range m.subs {
		if Match(s.filter, topic) {
			hs = append(hs, s.h)
		}
	}
	m.mu.Unlock()
	for _, h := range hs {
		if m.Async {
			go h(topic, payload)
		} else {
			h(topic, payload)
		}
	}
	return nil
}

func (m *Memory) Subscribe(filter string, h Handler) (func(), error) {
	m.mu.Lock()
	id := m.next
	m.next++
	m.subs[id] = &memSub{filter: filter, h: h}
	var replay []struct {
		t string
		p []byte
	}
	for t, p := range m.retained {
		if Match(filter, t) {
			replay = append(replay, struct {
				t string
				p []byte
			}{t, p})
		}
	}
	m.mu.Unlock()
	for _, r := range replay {
		h(r.t, r.p)
	}
	return func() {
		m.mu.Lock()
		delete(m.subs, id)
		m.mu.Unlock()
	}, nil
}

func (m *Memory) Close() error { return nil }
