package fleet

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

// Vehicles lists vehicles with their state.
func (c *Core) Vehicles(siteID string) []core.VehicleWithState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]core.VehicleWithState, 0, len(c.vehicles))
	for id, v := range c.vehicles {
		if siteID != "" && v.SiteID != siteID {
			continue
		}
		out = append(out, core.VehicleWithState{Vehicle: *v, State: c.rt[id].state})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Vehicle.Name < out[j].Vehicle.Name })
	return out
}

// Vehicle returns one vehicle with state.
func (c *Core) Vehicle(id string) (*core.VehicleWithState, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v := c.vehicles[id]
	if v == nil {
		return nil, core.NotFound("vehicle", id)
	}
	return &core.VehicleWithState{Vehicle: *v, State: c.rt[id].state}, nil
}

func (c *Core) validateVehicle(v *core.Vehicle) error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" {
		return core.Validation("vehicle name is required")
	}
	if v.SiteID == "" {
		v.SiteID = core.DefaultSiteID
	}
	if c.sites[v.SiteID] == nil {
		return core.Validation("unknown site %q", v.SiteID)
	}
	if c.models[v.ModelID] == nil {
		return core.Validation("unknown model %q", v.ModelID)
	}
	if v.Protocol == "" {
		v.Protocol = core.ProtocolVDA5050
	}
	if v.Protocol != core.ProtocolVDA5050 && v.Protocol != core.ProtocolSimulated {
		return core.Validation("unsupported protocol %q", v.Protocol)
	}
	if v.Manufacturer == "" {
		v.Manufacturer = "inov4d"
	}
	if v.SerialNumber == "" {
		return core.Validation("serial number is required")
	}
	for id, other := range c.vehicles {
		if id != v.ID && other.Manufacturer == v.Manufacturer && other.SerialNumber == v.SerialNumber {
			return core.Conflict("serial number %q already used by %s", v.SerialNumber, other.Name)
		}
	}
	if v.SafetyMarginM < 0 || v.LocalizationUncertaintyM < 0 {
		return core.Validation("margins cannot be negative")
	}
	if v.Footprint != nil && (v.Footprint.LengthM <= 0 || v.Footprint.WidthM <= 0) {
		v.Footprint = nil
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	return nil
}

// CreateVehicle registers a vehicle.
func (c *Core) CreateVehicle(v core.Vehicle) (*core.VehicleWithState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v.ID == "" {
		v.ID = "agv-" + uuid.NewString()[:8]
	}
	if c.vehicles[v.ID] != nil {
		return nil, core.Conflict("vehicle %q already exists", v.ID)
	}
	if err := c.validateVehicle(&v); err != nil {
		return nil, err
	}
	now := c.clock.Now()
	v.CreatedAt, v.UpdatedAt = now, now
	if err := c.st.SaveVehicle(context.Background(), &v); err != nil {
		return nil, err
	}
	c.vehicles[v.ID] = &v
	c.rt[v.ID] = c.newRuntime(v.ID)
	c.gw.Register(v.ID, v.Manufacturer, v.SerialNumber)
	return &core.VehicleWithState{Vehicle: v, State: c.rt[v.ID].state}, nil
}

// UpdateVehicle changes configuration.
func (c *Core) UpdateVehicle(id string, upd core.Vehicle) (*core.VehicleWithState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ex := c.vehicles[id]
	if ex == nil {
		return nil, core.NotFound("vehicle", id)
	}
	upd.ID = id
	upd.CreatedAt = ex.CreatedAt
	if upd.Protocol == "" {
		upd.Protocol = ex.Protocol
	}
	if err := c.validateVehicle(&upd); err != nil {
		return nil, err
	}
	upd.UpdatedAt = c.clock.Now()
	if err := c.st.SaveVehicle(context.Background(), &upd); err != nil {
		return nil, err
	}
	if upd.Manufacturer != ex.Manufacturer || upd.SerialNumber != ex.SerialNumber {
		c.gw.Register(id, upd.Manufacturer, upd.SerialNumber)
	}
	c.vehicles[id] = &upd
	if !upd.Enabled {
		c.withdrawVehicle(id, "vehicle disabled")
	}
	return &core.VehicleWithState{Vehicle: upd, State: c.rt[id].state}, nil
}

// DeleteVehicle removes a vehicle without active mission.
func (c *Core) DeleteVehicle(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vehicles[id] == nil {
		return core.NotFound("vehicle", id)
	}
	if c.execs[id] != nil {
		return core.Conflict("vehicle has an active mission")
	}
	if err := c.st.DeleteVehicle(context.Background(), id); err != nil {
		return err
	}
	delete(c.vehicles, id)
	delete(c.rt, id)
	c.gw.Unregister(id)
	c.traffic.Remove(id)
	c.analytics.Forget(id)
	if c.simEngine.Agent(id) != nil {
		c.simEngine.Remove(id)
	}
	return nil
}

// VehicleHistory returns telemetry samples.
func (c *Core) VehicleHistory(id string, from, to time.Time) ([]core.TelemetrySample, error) {
	if _, err := c.Vehicle(id); err != nil {
		return nil, err
	}
	return c.st.QueryTelemetry(context.Background(), id, from, to, 2000)
}

// Command executes an operator command on a vehicle.
func (c *Core) Command(id, command, nodeID, by string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.vehicles[id]
	if v == nil {
		return core.NotFound("vehicle", id)
	}
	r := c.rt[id]
	ex := c.execs[id]
	switch command {
	case "pause":
		if ex != nil {
			ex.pausedByOperator = true
			if _, err := c.missions.Suspend(ex.missionID, "paused by "+by); err != nil {
				return err
			}
		}
		return c.gw.Pause(id)
	case "resume":
		if ex != nil {
			ex.pausedByOperator = false
			if _, err := c.missions.Resume(ex.missionID); err != nil {
				return err
			}
		}
		return c.gw.Resume(id)
	case "cancelMission":
		if ex == nil {
			return core.Conflict("vehicle has no mission")
		}
		return c.cancelExecution(id, "cancelled by "+by)
	case "enable", "disable":
		v.Enabled = command == "enable"
		v.UpdatedAt = c.clock.Now()
		if !v.Enabled {
			c.withdrawVehicle(id, "vehicle disabled by "+by)
		}
		return c.st.SaveVehicle(context.Background(), v)
	case "maintenance", "endMaintenance":
		r.maintenance = command == "maintenance"
		if r.maintenance {
			c.withdrawVehicle(id, "maintenance requested by "+by)
		}
		return c.st.SetSetting(context.Background(), "maintenance:"+id, r.maintenance)
	case "clearErrors":
		return c.gw.RequestState(id)
	case "sendToCharge":
		if ex != nil {
			return core.Conflict("vehicle is executing a mission")
		}
		_, err := c.createChargeMission(v, r, 7, by)
		return err
	case "sendToNode":
		if nodeID == "" {
			return core.Validation("nodeId is required")
		}
		if ex != nil {
			return core.Conflict("vehicle is executing a mission")
		}
		g := c.graphs[v.SiteID]
		if g == nil || g.g.Nodes[nodeID] == nil {
			return core.Validation("unknown node %q", nodeID)
		}
		m, err := c.missions.Create(core.Mission{SiteID: v.SiteID, Type: core.MissionManual, Destination: nodeID, Priority: 6, Source: core.SourceOperator,
			Name: "Déplacement manuel " + v.Name + " → " + nodeID, CreatedBy: by}, nil)
		if err != nil {
			return err
		}
		c.pinned(m.ID, v.ID)
		return nil
	}
	return core.Validation("unknown command %q", command)
}

// withdrawVehicle takes a vehicle out of service: its mission is held or
// returned to the queue depending on whether a load may be on board.
func (c *Core) withdrawVehicle(id, reason string) {
	ex := c.execs[id]
	if ex == nil {
		return
	}
	r := c.rt[id]
	if ex.loadOnBoard(r.state) {
		_, _ = c.missions.HoldForVerification(ex.missionID, reason+" — charge possiblement à bord, vérification requise")
		v := c.vehicles[id]
		c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevWarning, Code: "LOAD_UNVERIFIED", Source: core.AlarmSrcMission, VehicleID: id, MissionID: ex.missionID,
			Message: v.Name + ": mission suspendue, état physique de la charge à vérifier avant réaffectation"})
		c.endExecution(id, false)
		return
	}
	_, _ = c.missions.Unassign(ex.missionID, reason+" — mission remise en file", false)
	c.endExecution(id, false)
}

// cancelExecution cancels the mission of a vehicle and tells the vehicle.
func (c *Core) cancelExecution(id, reason string) error {
	ex := c.execs[id]
	if ex == nil {
		return nil
	}
	if _, err := c.missions.Cancel(ex.missionID, reason); err != nil {
		return err
	}
	c.endExecution(id, true)
	return nil
}

// endExecution drops the execution, clears reservations and optionally sends cancelOrder.
func (c *Core) endExecution(id string, cancelOrder bool) {
	delete(c.execs, id)
	c.traffic.ClearRoute(id)
	if cancelOrder {
		_ = c.gw.CancelOrder(id)
	}
	if r := c.rt[id]; r != nil {
		r.idleSince = c.clock.Now()
	}
}

var _ = vda5050.Version
