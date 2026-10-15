package fleet

import (
	"math"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
)

// ChargingStation is the API view of a charging node.
type ChargingStation struct {
	NodeID     string  `json:"nodeId"`
	Name       string  `json:"name"`
	PowerKw    float64 `json:"powerKw"`
	OccupiedBy *string `json:"occupiedBy"`
	ReservedBy *string `json:"reservedBy"`
}

// ChargingStations lists charging nodes of a site with occupancy.
func (c *Core) ChargingStations(siteID string) []ChargingStation {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.chargingStations(siteID)
}

func (c *Core) chargingStations(siteID string) []ChargingStation {
	g := c.graphs[siteID]
	if g == nil {
		return nil
	}
	held := map[string]string{}
	for _, r := range c.traffic.Reservations(siteID) {
		if r.Resource.Type == core.ResNode {
			held[r.Resource.ID] = r.VehicleID
		}
	}
	target := map[string]string{}
	for vid, ex := range c.execs {
		if ex.isCharge {
			target[ex.route[len(ex.route)-1]] = vid
		}
	}
	var out []ChargingStation
	for _, n := range g.g.ChargingNodes() {
		s := ChargingStation{NodeID: n.ID, Name: n.Name}
		if n.ChargerPowerKw != nil {
			s.PowerKw = *n.ChargerPowerKw
		}
		if v, ok := held[n.ID]; ok {
			vv := v
			s.OccupiedBy = &vv
		}
		if v, ok := target[n.ID]; ok {
			vv := v
			s.ReservedBy = &vv
		}
		out = append(out, s)
	}
	return out
}

// batteryManagement creates charge missions for idle vehicles that need it,
// and return-to-station missions for vehicles idling on working nodes.
func (c *Core) batteryManagement(now time.Time) {
	for id, v := range c.vehicles {
		r := c.rt[id]
		if r.state.Availability != core.AvailAvailable || c.execs[id] != nil || c.hasQueuedMission(id) {
			continue
		}
		idle := now.Sub(r.idleSince)
		if c.dispatch.Mode == core.AllocationColony && r.state.BatteryCharge > c.battery.CriticalPct {
			continue // the agents decide for themselves; the centre keeps only the critical safety net
		}
		switch {
		case r.state.BatteryCharge <= c.battery.CriticalPct:
			_, _ = c.createChargeMission(v, r, 9, "battery manager (critical)")
		case r.state.BatteryCharge <= c.battery.LowPct:
			_, _ = c.createChargeMission(v, r, 7, "battery manager (low)")
		case r.state.BatteryCharge <= c.battery.OpportunityPct && idle > time.Duration(c.battery.IdleBeforeChargeS*float64(time.Second)):
			_, _ = c.createChargeMission(v, r, 2, "battery manager (opportunity)")
		case idle > c.cfg.IdleBeforePark:
			c.createParkMission(v, r)
		}
	}
}

func (c *Core) hasQueuedMission(vehicleID string) bool {
	for mid, p := range c.pins {
		if p.vehicleID == vehicleID {
			if m, err := c.missions.Get(mid); err == nil && !m.Status.Terminal() {
				return true
			}
		}
	}
	return false
}

// createChargeMission picks the nearest free charging station.
func (c *Core) createChargeMission(v *core.Vehicle, r *vehicleRuntime, priority int, by string) (*core.Mission, error) {
	g := c.graphs[v.SiteID]
	if g == nil {
		return nil, core.Conflict("no active map")
	}
	// already queued or charging at a station?
	for _, m := range c.missions.List(v.SiteID, "", v.ID, 0, true) {
		if m.Type == core.MissionCharge {
			return &m, nil
		}
	}
	p := core.Point{X: r.state.Pose.X, Y: r.state.Pose.Y}
	best := ""
	bestD := math.Inf(1)
	for _, s := range c.chargingStations(v.SiteID) {
		if s.OccupiedBy != nil && *s.OccupiedBy != v.ID {
			continue
		}
		if s.ReservedBy != nil && *s.ReservedBy != v.ID {
			continue
		}
		if d := g.g.Nodes[s.NodeID].Point().Dist(p); d < bestD {
			bestD, best = d, s.NodeID
		}
	}
	if best == "" {
		return nil, core.Conflict("no free charging station")
	}
	m, err := c.missions.Create(core.Mission{SiteID: v.SiteID, Type: core.MissionCharge, Destination: best, Priority: priority, Source: core.SourceSystem,
		Name: "Recharge " + v.Name, CreatedBy: by}, nil)
	if err != nil {
		return nil, err
	}
	c.pinned(m.ID, v.ID)
	return m, nil
}

// createParkMission sends an idle vehicle standing on a working node to a free parking node.
func (c *Core) createParkMission(v *core.Vehicle, r *vehicleRuntime) {
	g := c.graphs[v.SiteID]
	if g == nil {
		return
	}
	cur := c.currentNode(v, r, g.g)
	if n := g.g.Nodes[cur]; n == nil || n.Type == core.NodeParking || n.Type == core.NodeCharging || n.Type == core.NodeWaiting {
		return
	}
	held := map[string]bool{}
	for _, res := range c.traffic.Reservations(v.SiteID) {
		if res.Resource.Type == core.ResNode && res.VehicleID != v.ID {
			held[res.Resource.ID] = true
		}
	}
	for _, m := range c.missions.List(v.SiteID, "", "", 0, true) {
		if m.Type == core.MissionReturnToStation {
			held[m.Destination] = true
			if m.VehicleID == v.ID || c.pins[m.ID].vehicleID == v.ID {
				return
			}
		}
	}
	p := core.Point{X: r.state.Pose.X, Y: r.state.Pose.Y}
	best := ""
	bestD := math.Inf(1)
	for _, n := range g.m.Nodes {
		if n.Type != core.NodeParking || held[n.ID] {
			continue
		}
		if d := n.Point().Dist(p); d < bestD {
			bestD, best = d, n.ID
		}
	}
	if best == "" {
		return
	}
	m, err := c.missions.Create(core.Mission{SiteID: v.SiteID, Type: core.MissionReturnToStation, Destination: best, Priority: 1, Source: core.SourceSystem, Name: "Retour station " + v.Name}, nil)
	if err == nil {
		c.pinned(m.ID, v.ID)
	}
	r.idleSince = c.clock.Now()
}
