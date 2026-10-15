package fleet

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/sim"
)

// SimulationScenario describes automatic mission generation.
type SimulationScenario struct {
	Name              string  `json:"name"`
	MissionRatePerMin float64 `json:"missionRatePerMin"`
}

// SimulationFault is an active injected fault.
type SimulationFault struct {
	VehicleID string `json:"vehicleId"`
	Type      string `json:"type"`
}

// SimulationStatus is the API view.
type SimulationStatus struct {
	Running    bool                `json:"running"`
	TimeScale  float64             `json:"timeScale"`
	SimTime    time.Time           `json:"simTime"`
	MapID      string              `json:"mapId"`
	SiteID     string              `json:"siteId"`
	VehicleIDs []string            `json:"vehicleIds"`
	Scenario   *SimulationScenario `json:"scenario"`
	Faults     []SimulationFault   `json:"faults"`
}

// SimulationStart is the start request.
type SimulationStart struct {
	SiteID    string              `json:"siteId"`
	MapID     string              `json:"mapId"`
	Vehicles  int                 `json:"vehicles"`
	ModelMix  map[string]float64  `json:"modelMix"`
	TimeScale float64             `json:"timeScale"`
	Scenario  *SimulationScenario `json:"scenario"`
}

type simulationState struct {
	siteID      string
	mapID       string
	scenario    *SimulationScenario
	lastMission time.Time
	rng         *rand.Rand
	counter     int
}

// SimulationStatus returns the current status.
func (c *Core) SimulationStatus() SimulationStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.simStatus()
}

func (c *Core) simStatus() SimulationStatus {
	st := SimulationStatus{Running: c.simEngine.Running(), TimeScale: c.clock.Scale(), SimTime: c.clock.Now(), MapID: c.simState.mapID, SiteID: c.simState.siteID,
		VehicleIDs: c.simEngine.VehicleIDs(), Scenario: c.simState.scenario, Faults: []SimulationFault{}}
	for _, id := range st.VehicleIDs {
		if a := c.simEngine.Agent(id); a != nil {
			for _, f := range a.Faults() {
				st.Faults = append(st.Faults, SimulationFault{VehicleID: id, Type: string(f)})
			}
		}
	}
	return st
}

// StartSimulation spawns simulated vehicles on the site's active map (creating
// the demo map when the site has none).
func (c *Core) StartSimulation(req SimulationStart) (SimulationStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if req.SiteID == "" {
		req.SiteID = core.DefaultSiteID
	}
	if c.sites[req.SiteID] == nil {
		return SimulationStatus{}, core.NotFound("site", req.SiteID)
	}
	if req.Vehicles <= 0 {
		req.Vehicles = 5
	}
	if req.Vehicles > 40 {
		req.Vehicles = 40
	}
	if req.TimeScale <= 0 {
		req.TimeScale = 1
	}
	if req.MapID != "" {
		m, err := c.st.GetMap(context.Background(), req.MapID)
		if err != nil {
			return SimulationStatus{}, err
		}
		if err := c.activateMap(m); err != nil {
			return SimulationStatus{}, err
		}
	}
	if c.graphs[req.SiteID] == nil {
		demo := mapmgr.DemoMap(req.SiteID)
		if req.Vehicles > 12 {
			demo = mapmgr.DemoMapSized(req.SiteID, 6, 8)
		}
		demo.CreatedAt, demo.UpdatedAt = c.clock.Now(), c.clock.Now()
		if err := c.st.SaveMap(context.Background(), demo); err != nil {
			return SimulationStatus{}, err
		}
		if err := c.activateMap(demo); err != nil {
			return SimulationStatus{}, err
		}
	}
	ge := c.graphs[req.SiteID]
	c.simState = simulationState{siteID: req.SiteID, mapID: ge.m.ID, scenario: req.Scenario, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	if req.Scenario != nil && req.Scenario.Name == "none" {
		c.simState.scenario = nil
	}
	c.clock.SetScale(req.TimeScale)
	c.simEngine.Start()
	mix := req.ModelMix
	if len(mix) == 0 {
		mix = map[string]float64{"AGV20T": 0.6, "AGV40T": 0.4}
	}
	for i := 0; i < req.Vehicles; i++ {
		modelID := pickModel(mix, c.simState.rng, c.models)
		if _, err := c.spawnSimVehicle(req.SiteID, modelID, ""); err != nil {
			c.log.Warn("spawn failed", "err", err)
			break
		}
	}
	c.emit(core.Event{Kind: "simulation.started", SiteID: req.SiteID, Message: fmt.Sprintf("simulation démarrée avec %d AGV", req.Vehicles)})
	return c.simStatus(), nil
}

func pickModel(mix map[string]float64, rng *rand.Rand, models map[string]*core.VehicleModel) string {
	total := 0.0
	keys := make([]string, 0, len(mix))
	for k, w := range mix {
		if models[k] != nil && w > 0 {
			keys = append(keys, k)
			total += w
		}
	}
	sort.Strings(keys)
	if total <= 0 {
		return "AGV20T"
	}
	x := rng.Float64() * total
	for _, k := range keys {
		x -= mix[k]
		if x <= 0 {
			return k
		}
	}
	return keys[len(keys)-1]
}

// SpawnSimulatedVehicle adds one simulated vehicle.
func (c *Core) SpawnSimulatedVehicle(modelID, nodeID string) (*core.VehicleWithState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.simEngine.Running() {
		return nil, core.Conflict("simulation is not running")
	}
	if modelID == "" {
		modelID = "AGV20T"
	}
	return c.spawnSimVehicle(c.simState.siteID, modelID, nodeID)
}

func (c *Core) spawnSimVehicle(siteID, modelID, nodeID string) (*core.VehicleWithState, error) {
	ge := c.graphs[siteID]
	model := c.models[modelID]
	if ge == nil || model == nil {
		return nil, core.Validation("no map or unknown model")
	}
	// choose a free node: parking first, then charging, waiting, waypoints
	occupied := map[string]bool{}
	for _, r := range c.traffic.Reservations(siteID) {
		if r.Resource.Type == core.ResNode {
			occupied[r.Resource.ID] = true
		}
	}
	for id, v := range c.vehicles {
		if v.SiteID == siteID {
			if n := c.currentNode(v, c.rt[id], ge.g); n != "" {
				occupied[n] = true
			}
		}
	}
	for _, a := range c.simEngine.VehicleIDs() {
		if ag := c.simEngine.Agent(a); ag != nil {
			occupied[ag.Config().LastNodeID] = true
		}
	}
	start := nodeID
	if start == "" {
		for _, t := range []core.NodeType{core.NodeParking, core.NodeCharging, core.NodeWaiting, core.NodeWaypoint} {
			for _, n := range ge.m.Nodes {
				if n.Type == t && !occupied[n.ID] {
					start = n.ID
					break
				}
			}
			if start != "" {
				break
			}
		}
	}
	n := ge.g.Nodes[start]
	if n == nil {
		return nil, core.Conflict("no free node to spawn a vehicle")
	}
	c.simState.counter++
	serial := fmt.Sprintf("SIM-%03d", c.simState.counter)
	for {
		dup := false
		for _, v := range c.vehicles {
			if v.SerialNumber == serial {
				dup = true
			}
		}
		if !dup {
			break
		}
		c.simState.counter++
		serial = fmt.Sprintf("SIM-%03d", c.simState.counter)
	}
	v := core.Vehicle{ID: "sim-" + serial, SiteID: siteID, Name: "AGV " + serial, ModelID: modelID, Manufacturer: "inov4d", SerialNumber: serial, Protocol: core.ProtocolSimulated,
		Enabled: true, SafetyMarginM: 0.3, LocalizationUncertaintyM: 0.1, Tags: []string{"simulation"}, CreatedAt: c.clock.Now(), UpdatedAt: c.clock.Now()}
	if err := c.st.SaveVehicle(context.Background(), &v); err != nil {
		return nil, err
	}
	c.vehicles[v.ID] = &v
	c.rt[v.ID] = c.newRuntime(v.ID)
	c.gw.Register(v.ID, v.Manufacturer, v.SerialNumber)
	battery := 60 + c.simState.rng.Float64()*40
	cfg := sim.AgentConfig{VehicleID: v.ID, Manufacturer: v.Manufacturer, Serial: serial, Interface: c.cfg.VDAInterface, MapID: ge.m.ID, Model: *model,
		Pose: core.Pose{X: n.X, Y: n.Y, Theta: 0}, BatteryPct: battery, LastNodeID: n.ID}
	if _, err := c.simEngine.Spawn(cfg); err != nil {
		return nil, err
	}
	return &core.VehicleWithState{Vehicle: v, State: c.rt[v.ID].state}, nil
}

// StopSimulation removes simulated vehicles and stops the engine.
func (c *Core) StopSimulation() SimulationStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := c.simEngine.VehicleIDs()
	c.simEngine.Stop()
	for _, id := range ids {
		if ex := c.execs[id]; ex != nil {
			_, _ = c.missions.Cancel(ex.missionID, "simulation stopped")
			delete(c.execs, id)
		}
		c.traffic.Remove(id)
		c.gw.Unregister(id)
		c.analytics.Forget(id)
		c.alarms.ClearVehicleSource(core.AlarmSrcVehicle, id, nil)
		c.alarms.ClearVehicleSource(core.AlarmSrcComms, id, nil)
		c.alarms.ClearVehicleSource(core.AlarmSrcBattery, id, nil)
		c.alarms.ClearVehicleSource(core.AlarmSrcTraffic, id, nil)
		c.alarms.ClearVehicleSource(core.AlarmSrcSystem, id, nil)
		delete(c.vehicles, id)
		delete(c.rt, id)
		_ = c.st.DeleteVehicle(context.Background(), id)
	}
	// cancel remaining system missions of the scenario
	for _, m := range c.missions.List(c.simState.siteID, "", "", 0, true) {
		if m.Source == core.SourceSystem || (m.CreatedBy == "simulation") {
			_, _ = c.missions.Cancel(m.ID, "simulation stopped")
		}
	}
	c.clock.SetScale(1)
	c.simState.scenario = nil
	c.emit(core.Event{Kind: "simulation.stopped", SiteID: c.simState.siteID, Message: "simulation arrêtée"})
	return c.simStatus()
}

// SetTimeScale changes the simulation speed.
func (c *Core) SetTimeScale(scale float64) SimulationStatus {
	if scale <= 0 {
		scale = 1
	}
	if scale > 20 {
		scale = 20
	}
	c.clock.SetScale(scale)
	return c.SimulationStatus()
}

// SetScenario changes mission generation.
func (c *Core) SetScenario(s SimulationScenario) SimulationStatus {
	c.mu.Lock()
	if s.Name == "none" || s.Name == "" {
		c.simState.scenario = nil
	} else {
		if s.MissionRatePerMin <= 0 {
			s.MissionRatePerMin = 1
		}
		cp := s
		c.simState.scenario = &cp
	}
	c.mu.Unlock()
	return c.SimulationStatus()
}

// InjectFault applies a fault to a simulated vehicle.
func (c *Core) InjectFault(vehicleID, fault string) error {
	a := c.simEngine.Agent(vehicleID)
	if a == nil {
		return core.NotFound("simulated vehicle", vehicleID)
	}
	switch sim.FaultType(fault) {
	case sim.FaultCommsLoss, sim.FaultBlocked, sim.FaultError, sim.FaultEStop, sim.FaultRestore:
		a.SetFault(sim.FaultType(fault))
		return nil
	}
	return core.Validation("unknown fault %q", fault)
}

// simulationTick generates scenario missions.
func (c *Core) simulationTick(now time.Time) {
	s := c.simState.scenario
	if s == nil || !c.simEngine.Running() || s.Name != "randomTransport" {
		return
	}
	ge := c.graphs[c.simState.siteID]
	if ge == nil {
		return
	}
	interval := time.Duration(60 / s.MissionRatePerMin * float64(time.Second))
	if now.Sub(c.simState.lastMission) < interval {
		return
	}
	// keep the queue bounded
	if len(c.missions.Pending(c.simState.siteID)) > len(c.simEngine.VehicleIDs())*2 {
		return
	}
	c.simState.lastMission = now
	var picks, drops []string
	for _, n := range ge.m.Nodes {
		switch n.Type {
		case core.NodePickup:
			picks = append(picks, n.ID)
		case core.NodeDropoff:
			drops = append(drops, n.ID)
		}
	}
	if len(picks) == 0 || len(drops) == 0 {
		return
	}
	rng := c.simState.rng
	mass := 2000 + rng.Float64()*18000
	if rng.Float64() < 0.3 {
		mass = 20000 + rng.Float64()*20000
	}
	from, to := picks[rng.Intn(len(picks))], drops[rng.Intn(len(drops))]
	if rng.Float64() < 0.4 { // reverse flows too
		from, to = to, from
	}
	_, _ = c.missions.Create(core.Mission{SiteID: c.simState.siteID, Type: core.MissionTransport, Origin: from, Destination: to, Priority: 1 + rng.Intn(8),
		Load: core.MissionLoad{Present: true, MassKg: float64(int(mass)), LengthM: 2.4, WidthM: 1.2, HeightM: 1.0, Description: "charge simulée"}, Source: core.SourceSystem, CreatedBy: "simulation"},
		func(n string) bool { return ge.g.Nodes[n] != nil })
}
