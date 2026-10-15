package sim

import (
	"sort"
	"sync"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/transport"
)

// Engine runs simulated agents at a configurable time scale.
type Engine struct {
	mu       sync.Mutex
	tr       transport.Transport
	clock    *core.ScaledClock
	agents   map[string]*Agent
	running  bool
	stop     chan struct{}
	wallTick time.Duration
	simTime  time.Time
}

// NewEngine creates an engine using the given scaled clock.
func NewEngine(tr transport.Transport, clock *core.ScaledClock) *Engine {
	return &Engine{tr: tr, clock: clock, agents: map[string]*Agent{}, wallTick: 50 * time.Millisecond}
}

// Spawn creates and starts an agent.
func (e *Engine) Spawn(cfg AgentConfig) (*Agent, error) {
	a := NewAgent(cfg, e.tr, e.clock)
	if err := a.Start(); err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.agents[cfg.VehicleID] = a
	e.mu.Unlock()
	return a, nil
}

// Remove stops and forgets an agent.
func (e *Engine) Remove(vehicleID string) {
	e.mu.Lock()
	a := e.agents[vehicleID]
	delete(e.agents, vehicleID)
	e.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// Agent returns an agent by vehicle id.
func (e *Engine) Agent(vehicleID string) *Agent {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.agents[vehicleID]
}

// VehicleIDs lists the simulated vehicles.
func (e *Engine) VehicleIDs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0, len(e.agents))
	for id := range e.agents {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Start launches the stepping loop.
func (e *Engine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.stop = make(chan struct{})
	stop := e.stop
	e.mu.Unlock()
	go func() {
		t := time.NewTicker(e.wallTick)
		defer t.Stop()
		last := time.Now()
		for {
			select {
			case <-stop:
				return
			case now := <-t.C:
				wall := now.Sub(last).Seconds()
				last = now
				e.StepAll(wall * e.clock.Scale())
			}
		}
	}()
}

// StepAll advances every agent by dt simulated seconds (sub-stepped for stability).
func (e *Engine) StepAll(dt float64) {
	e.mu.Lock()
	agents := make([]*Agent, 0, len(e.agents))
	for _, a := range e.agents {
		agents = append(agents, a)
	}
	e.mu.Unlock()
	for dt > 0 {
		step := dt
		if step > 0.2 {
			step = 0.2
		}
		for _, a := range agents {
			a.Step(step)
		}
		dt -= step
	}
}

// Stop halts the loop and all agents.
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	close(e.stop)
	agents := e.agents
	e.agents = map[string]*Agent{}
	e.mu.Unlock()
	for _, a := range agents {
		a.Stop()
	}
}

// Running reports whether the loop is active.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Clock returns the scaled clock.
func (e *Engine) Clock() *core.ScaledClock { return e.clock }
