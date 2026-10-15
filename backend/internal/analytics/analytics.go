// Package analytics aggregates fleet performance figures (KPIs) and records
// periodic metric samples for time series charts.
package analytics

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

type missionRecord struct {
	siteID    string
	vehicleID string
	at        time.Time
	durationS float64
	waitS     float64
	failed    bool
	distanceM float64
}

// Engine collects completion records and vehicle usage.
type Engine struct {
	mu         sync.Mutex
	st         store.Store
	clock      core.Clock
	records    []missionRecord
	busyTime   map[string]float64 // vehicle -> busy seconds
	totalTime  map[string]float64 // vehicle -> observed seconds
	distance   map[string]float64
	energy     map[string]float64
	lastTick   map[string]time.Time
	lastSample time.Time
	startedAt  time.Time
}

func New(st store.Store, clock core.Clock) *Engine {
	return &Engine{st: st, clock: clock, busyTime: map[string]float64{}, totalTime: map[string]float64{}, distance: map[string]float64{}, energy: map[string]float64{}, lastTick: map[string]time.Time{}, startedAt: clock.Now()}
}

// RecordMission stores the outcome of a terminal mission.
func (e *Engine) RecordMission(m core.Mission, distanceM float64) {
	if m.Status != core.MissionCompleted && m.Status != core.MissionFailed {
		return
	}
	r := missionRecord{siteID: m.SiteID, vehicleID: m.VehicleID, at: e.clock.Now(), failed: m.Status == core.MissionFailed, distanceM: distanceM}
	if m.StartedAt != nil && m.CompletedAt != nil {
		r.durationS = m.CompletedAt.Sub(*m.StartedAt).Seconds()
	}
	if m.AssignedAt != nil {
		r.waitS = m.AssignedAt.Sub(m.CreatedAt).Seconds()
	}
	e.mu.Lock()
	e.records = append(e.records, r)
	if len(e.records) > 50000 {
		e.records = e.records[len(e.records)-50000:]
	}
	e.mu.Unlock()
}

// Observe accumulates vehicle usage. Call once per tick per vehicle.
func (e *Engine) Observe(vehicleID string, busy bool, distanceDeltaM, energyDeltaKwh float64) {
	now := e.clock.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	if last, ok := e.lastTick[vehicleID]; ok {
		dt := now.Sub(last).Seconds()
		if dt > 0 && dt < 60 {
			e.totalTime[vehicleID] += dt
			if busy {
				e.busyTime[vehicleID] += dt
			}
		}
	}
	e.lastTick[vehicleID] = now
	if distanceDeltaM > 0 {
		e.distance[vehicleID] += distanceDeltaM
	}
	if energyDeltaKwh > 0 {
		e.energy[vehicleID] += energyDeltaKwh
	}
}

// Forget drops a vehicle's usage.
func (e *Engine) Forget(vehicleID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.busyTime, vehicleID)
	delete(e.totalTime, vehicleID)
	delete(e.distance, vehicleID)
	delete(e.energy, vehicleID)
	delete(e.lastTick, vehicleID)
}

// Input is the live context needed to compute KPIs.
type Input struct {
	SiteID          string
	From, To        time.Time
	Vehicles        []core.VehicleWithState
	ActiveAlarms    int
	Deadlocks       int
	AlarmsByVehicle func(id string) int
}

// KPIs computes the figures for a window (zero window = since start).
func (e *Engine) KPIs(in Input) core.KPIs {
	e.mu.Lock()
	defer e.mu.Unlock()
	from, to := in.From, in.To
	if to.IsZero() {
		to = e.clock.Now()
	}
	if from.IsZero() {
		from = e.startedAt
	}
	k := core.KPIs{ActiveAlarms: in.ActiveAlarms, DeadlocksResolved: in.Deadlocks}
	var durSum, waitSum float64
	var durN int
	perVehicle := map[string]*core.VehicleKPIs{}
	for _, r := range e.records {
		if in.SiteID != "" && r.siteID != in.SiteID {
			continue
		}
		if r.at.Before(from) || r.at.After(to) {
			continue
		}
		if r.failed {
			k.MissionsFailed++
		} else {
			k.MissionsCompleted++
			durSum += r.durationS
			waitSum += r.waitS
			durN++
		}
		if perVehicle[r.vehicleID] == nil {
			perVehicle[r.vehicleID] = &core.VehicleKPIs{VehicleID: r.vehicleID}
		}
		perVehicle[r.vehicleID].Missions++
	}
	hours := to.Sub(from).Hours()
	if hours > 0 {
		k.MissionsPerHour = float64(k.MissionsCompleted) / hours
	}
	if durN > 0 {
		k.AvgMissionDurationS = durSum / float64(durN)
		k.AvgWaitTimeS = waitSum / float64(durN)
	}
	var utilSum float64
	var utilN int
	for _, v := range in.Vehicles {
		id := v.Vehicle.ID
		if in.SiteID != "" && v.Vehicle.SiteID != in.SiteID {
			continue
		}
		if perVehicle[id] == nil {
			perVehicle[id] = &core.VehicleKPIs{VehicleID: id}
		}
		pv := perVehicle[id]
		if t := e.totalTime[id]; t > 0 {
			pv.UtilizationPct = e.busyTime[id] / t * 100
			utilSum += pv.UtilizationPct
			utilN++
		}
		pv.DistanceM = e.distance[id]
		pv.BatteryChargePct = v.State.BatteryCharge
		if in.AlarmsByVehicle != nil {
			pv.Alarms = in.AlarmsByVehicle(id)
		}
		k.DistanceM += e.distance[id]
		k.EnergyKwh += e.energy[id]
		if v.State.Connection == core.ConnOnline {
			k.VehiclesOnline++
		}
	}
	if utilN > 0 {
		k.FleetUtilizationPct = utilSum / float64(utilN)
	}
	for _, pv := range perVehicle {
		k.Vehicles = append(k.Vehicles, *pv)
	}
	sort.Slice(k.Vehicles, func(i, j int) bool { return k.Vehicles[i].VehicleID < k.Vehicles[j].VehicleID })
	return k
}

// Sample persists metric points every interval (missionsPerHour over the last
// hour, utilization, activeVehicles, alarms).
func (e *Engine) Sample(siteID string, in Input, interval time.Duration) {
	now := e.clock.Now()
	e.mu.Lock()
	if now.Sub(e.lastSample) < interval {
		e.mu.Unlock()
		return
	}
	e.lastSample = now
	e.mu.Unlock()
	in.SiteID = siteID
	in.From, in.To = now.Add(-time.Hour), now
	k := e.KPIs(in)
	active := 0
	for _, v := range in.Vehicles {
		if v.State.Availability == core.AvailBusy {
			active++
		}
	}
	ctx := context.Background()
	_ = e.st.AppendMetric(ctx, siteID, "missionsPerHour", now, k.MissionsPerHour)
	_ = e.st.AppendMetric(ctx, siteID, "utilization", now, k.FleetUtilizationPct)
	_ = e.st.AppendMetric(ctx, siteID, "activeVehicles", now, float64(active))
	_ = e.st.AppendMetric(ctx, siteID, "alarms", now, float64(in.ActiveAlarms))
}
