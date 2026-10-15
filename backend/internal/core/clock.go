package core

import (
	"sync"
	"time"
)

// Clock abstracts time so the simulation can run faster than real time and
// tests are deterministic.
type Clock interface {
	Now() time.Time
}

// RealClock uses the wall clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

// ScaledClock advances sim time at a configurable multiple of wall time.
type ScaledClock struct {
	mu     sync.Mutex
	base   time.Time // sim time at anchor
	anchor time.Time // wall time at anchor
	scale  float64
}

func NewScaledClock(scale float64) *ScaledClock {
	now := time.Now().UTC()
	return &ScaledClock{base: now, anchor: now, scale: scale}
}

func (c *ScaledClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	elapsed := time.Since(c.anchor)
	return c.base.Add(time.Duration(float64(elapsed) * c.scale))
}

func (c *ScaledClock) SetScale(scale float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now().UTC()
	c.base = c.base.Add(time.Duration(float64(now.Sub(c.anchor)) * c.scale))
	c.anchor = now
	if scale <= 0 {
		scale = 1
	}
	c.scale = scale
}

func (c *ScaledClock) Scale() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scale
}

// ManualClock is for tests.
type ManualClock struct {
	mu sync.Mutex
	t  time.Time
}

func NewManualClock(t time.Time) *ManualClock { return &ManualClock{t: t} }
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
func (c *ManualClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}
