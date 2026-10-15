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

// Memory is an in-process transport. Each subscription has its own ordered
// delivery queue served by one goroutine, so handlers never run on the
// publisher's goroutine (as with a real broker). NewMemorySync delivers
// synchronously instead, which is convenient in unit tests.
type Memory struct {
	mu       sync.RWMutex
	subs     map[int]*memSub
	next     int
	retained map[string][]byte
	sync     bool
}

type memSub struct {
	filter string
	h      Handler
	queue  chan memMsg
	done   chan struct{}
}

type memMsg struct {
	topic   string
	payload []byte
}

func NewMemory() *Memory { return &Memory{subs: map[int]*memSub{}, retained: map[string][]byte{}} }

// NewMemorySync creates a transport that calls handlers synchronously.
func NewMemorySync() *Memory {
	m := NewMemory()
	m.sync = true
	return m
}

func (m *Memory) Publish(topic string, payload []byte, retain bool) error {
	m.mu.Lock()
	if retain {
		if len(payload) == 0 {
			delete(m.retained, topic)
		} else {
			m.retained[topic] = append([]byte(nil), payload...)
		}
	}
	var targets []*memSub
	for _, s := range m.subs {
		if Match(s.filter, topic) {
			targets = append(targets, s)
		}
	}
	m.mu.Unlock()
	for _, s := range targets {
		s.deliver(topic, payload, m.sync)
	}
	return nil
}

func (s *memSub) deliver(topic string, payload []byte, sync bool) {
	if sync {
		s.h(topic, payload)
		return
	}
	select {
	case s.queue <- memMsg{topic, payload}:
	case <-s.done:
	}
}

func (m *Memory) Subscribe(filter string, h Handler) (func(), error) {
	m.mu.Lock()
	id := m.next
	m.next++
	s := &memSub{filter: filter, h: h, queue: make(chan memMsg, 4096), done: make(chan struct{})}
	m.subs[id] = s
	var replay []memMsg
	for t, p := range m.retained {
		if Match(filter, t) {
			replay = append(replay, memMsg{t, p})
		}
	}
	m.mu.Unlock()
	if !m.sync {
		go func() {
			for {
				select {
				case msg := <-s.queue:
					s.h(msg.topic, msg.payload)
				case <-s.done:
					return
				}
			}
		}()
	}
	for _, r := range replay {
		s.deliver(r.topic, r.payload, m.sync)
	}
	return func() {
		m.mu.Lock()
		if cur, ok := m.subs[id]; ok && cur == s {
			delete(m.subs, id)
			close(s.done)
		}
		m.mu.Unlock()
	}, nil
}

func (m *Memory) Close() error { return nil }
