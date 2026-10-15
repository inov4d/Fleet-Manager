package api

import (
	"net/http"
	"time"

	"github.com/inov4d/fleet-manager/internal/auth"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/fleet"
	"github.com/inov4d/fleet-manager/internal/store"
)

func (s *Server) routes() {
	m := s.mux
	const v1 = "/api/v1"
	m.HandleFunc("GET /health", s.health)
	m.HandleFunc("GET "+v1+"/health", s.health)
	m.HandleFunc("GET /ws", s.hub.serve(s))

	// auth
	m.HandleFunc("POST "+v1+"/auth/login", s.login)
	m.HandleFunc("GET "+v1+"/auth/me", s.guard("", s.me))
	m.HandleFunc("POST "+v1+"/auth/logout", s.guard("", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	// sites
	m.HandleFunc("GET "+v1+"/sites", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.core.Sites()) }))
	m.HandleFunc("POST "+v1+"/sites", s.guard(auth.PermUsers, s.saveSite))
	m.HandleFunc("GET "+v1+"/sites/{id}", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		st, err := s.core.GetSite(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, st)
	}))
	m.HandleFunc("PUT "+v1+"/sites/{id}", s.guard(auth.PermUsers, s.saveSite))
	m.HandleFunc("DELETE "+v1+"/sites/{id}", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		if err := s.core.DeleteSite(r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "site.delete", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	// models
	m.HandleFunc("GET "+v1+"/vehicle-models", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.core.Models()) }))
	m.HandleFunc("PUT "+v1+"/vehicle-models/{id}", s.guard(auth.PermVehiclesWrite, func(w http.ResponseWriter, r *http.Request) {
		var vm core.VehicleModel
		if err := decode(r, &vm); err != nil {
			writeErr(w, err)
			return
		}
		vm.ID = r.PathValue("id")
		out, err := s.core.SaveModel(vm)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "model.save", vm.ID, vm)
		writeJSON(w, 200, out)
	}))

	// vehicles
	m.HandleFunc("GET "+v1+"/vehicles", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.core.Vehicles(q(r, "siteId"))) }))
	m.HandleFunc("POST "+v1+"/vehicles", s.guard(auth.PermVehiclesWrite, s.createVehicle))
	m.HandleFunc("GET "+v1+"/vehicles/{id}", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		v, err := s.core.Vehicle(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, v)
	}))
	m.HandleFunc("PUT "+v1+"/vehicles/{id}", s.guard(auth.PermVehiclesWrite, s.updateVehicle))
	m.HandleFunc("DELETE "+v1+"/vehicles/{id}", s.guard(auth.PermVehiclesWrite, func(w http.ResponseWriter, r *http.Request) {
		if err := s.core.DeleteVehicle(r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "vehicle.delete", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("POST "+v1+"/vehicles/{id}/commands", s.guard(auth.PermVehiclesCommand, s.vehicleCommand))
	m.HandleFunc("GET "+v1+"/vehicles/{id}/history", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		samples, err := s.core.VehicleHistory(r.PathValue("id"), qTime(r, "from"), qTime(r, "to"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if samples == nil {
			samples = []core.TelemetrySample{}
		}
		writeJSON(w, 200, samples)
	}))

	// maps
	m.HandleFunc("GET "+v1+"/maps", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.core.Maps(q(r, "siteId"))) }))
	m.HandleFunc("POST "+v1+"/maps", s.guard(auth.PermMapsWrite, s.saveMap))
	m.HandleFunc("POST "+v1+"/maps/import", s.guard(auth.PermMapsWrite, s.saveMap))
	m.HandleFunc("GET "+v1+"/maps/active", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		site := q(r, "siteId")
		if site == "" {
			site = core.DefaultSiteID
		}
		mp, err := s.core.ActiveMap(site)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, mp)
	}))
	m.HandleFunc("GET "+v1+"/maps/{id}", s.guard(auth.PermRead, s.getMap))
	m.HandleFunc("GET "+v1+"/maps/{id}/export", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+r.PathValue("id")+".json\"")
		s.getMap(w, r)
	}))
	m.HandleFunc("PUT "+v1+"/maps/{id}", s.guard(auth.PermMapsWrite, s.saveMap))
	m.HandleFunc("DELETE "+v1+"/maps/{id}", s.guard(auth.PermMapsWrite, func(w http.ResponseWriter, r *http.Request) {
		if err := s.core.DeleteMap(r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "map.delete", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("POST "+v1+"/maps/{id}/activate", s.guard(auth.PermMapsWrite, func(w http.ResponseWriter, r *http.Request) {
		mp, err := s.core.ActivateMap(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "map.activate", mp.ID, nil)
		writeJSON(w, 200, mp)
	}))
	m.HandleFunc("POST "+v1+"/maps/{id}/zones/{zoneId}/enabled", s.guard(auth.PermTrafficWrite, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Enabled bool `json:"enabled"`
		}
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		mp, err := s.core.SetZoneEnabled(r.PathValue("id"), r.PathValue("zoneId"), b.Enabled)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "map.zone.enabled", r.PathValue("zoneId"), b)
		writeJSON(w, 200, mp)
	}))
	m.HandleFunc("POST "+v1+"/maps/{id}/edges/{edgeId}/enabled", s.guard(auth.PermTrafficWrite, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Enabled bool `json:"enabled"`
		}
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		mp, err := s.core.SetEdgeEnabled(r.PathValue("id"), r.PathValue("edgeId"), b.Enabled)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "map.edge.enabled", r.PathValue("edgeId"), b)
		writeJSON(w, 200, mp)
	}))
	m.HandleFunc("POST "+v1+"/maps/{id}/route", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			From, To, VehicleID string
		}
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		rt, err := s.core.TestRoute(r.PathValue("id"), b.From, b.To, b.VehicleID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, rt)
	}))

	// missions
	m.HandleFunc("GET "+v1+"/missions", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.core.Missions().List(q(r, "siteId"), core.MissionStatus(q(r, "status")), q(r, "vehicleId"), qInt(r, "limit", 200), q(r, "active") == "true"))
	}))
	m.HandleFunc("POST "+v1+"/missions", s.guard(auth.PermMissionsWrite, s.createMission))
	m.HandleFunc("GET "+v1+"/missions/{id}", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		ms, err := s.core.Missions().Get(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, ms)
	}))
	m.HandleFunc("POST "+v1+"/missions/{id}/cancel", s.guard(auth.PermMissionsWrite, func(w http.ResponseWriter, r *http.Request) {
		ms, err := s.core.CancelMission(r.PathValue("id"), principalFrom(r).Username)
		s.missionResult(w, r, "mission.cancel", ms, err)
	}))
	m.HandleFunc("POST "+v1+"/missions/{id}/suspend", s.guard(auth.PermMissionsWrite, func(w http.ResponseWriter, r *http.Request) {
		ms, err := s.core.SuspendMission(r.PathValue("id"), principalFrom(r).Username)
		s.missionResult(w, r, "mission.suspend", ms, err)
	}))
	m.HandleFunc("POST "+v1+"/missions/{id}/resume", s.guard(auth.PermMissionsWrite, func(w http.ResponseWriter, r *http.Request) {
		ms, err := s.core.ResumeMission(r.PathValue("id"))
		s.missionResult(w, r, "mission.resume", ms, err)
	}))
	m.HandleFunc("POST "+v1+"/missions/{id}/reassign", s.guard(auth.PermMissionsWrite, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			VehicleID        string `json:"vehicleId"`
			ConfirmLoadState bool   `json:"confirmLoadState"`
		}
		_ = decode(r, &b)
		ms, err := s.core.ReassignMission(r.PathValue("id"), b.VehicleID, b.ConfirmLoadState, principalFrom(r).Username)
		s.missionResult(w, r, "mission.reassign", ms, err)
	}))
	m.HandleFunc("POST "+v1+"/missions/{id}/priority", s.guard(auth.PermMissionsWrite, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Priority int `json:"priority"`
		}
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		ms, err := s.core.Missions().SetPriority(r.PathValue("id"), b.Priority)
		s.missionResult(w, r, "mission.priority", ms, err)
	}))
	m.HandleFunc("DELETE "+v1+"/missions/{id}", s.guard(auth.PermMissionsWrite, func(w http.ResponseWriter, r *http.Request) {
		if err := s.core.Missions().Delete(r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "mission.delete", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}))

	// settings
	m.HandleFunc("GET "+v1+"/settings/dispatcher", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.core.DispatcherSettings()) }))
	m.HandleFunc("PUT "+v1+"/settings/dispatcher", s.guard(auth.PermSettings, func(w http.ResponseWriter, r *http.Request) {
		var b core.DispatcherSettings
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.core.SetDispatcherSettings(b); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "settings.dispatcher", "", b)
		writeJSON(w, 200, s.core.DispatcherSettings())
	}))
	m.HandleFunc("GET "+v1+"/settings/traffic", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.core.TrafficSettings()) }))
	m.HandleFunc("PUT "+v1+"/settings/traffic", s.guard(auth.PermSettings, func(w http.ResponseWriter, r *http.Request) {
		var b core.TrafficSettings
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.core.SetTrafficSettings(b); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "settings.traffic", "", b)
		writeJSON(w, 200, s.core.TrafficSettings())
	}))
	m.HandleFunc("GET "+v1+"/settings/battery", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.core.BatterySettings()) }))
	m.HandleFunc("PUT "+v1+"/settings/battery", s.guard(auth.PermSettings, func(w http.ResponseWriter, r *http.Request) {
		var b core.BatterySettings
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.core.SetBatterySettings(b); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "settings.battery", "", b)
		writeJSON(w, 200, s.core.BatterySettings())
	}))

	// traffic
	m.HandleFunc("GET "+v1+"/traffic/reservations", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		out := s.core.Traffic().Reservations(q(r, "siteId"))
		if out == nil {
			out = []core.Reservation{}
		}
		writeJSON(w, 200, out)
	}))
	m.HandleFunc("GET "+v1+"/traffic/deadlocks", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		out := s.core.Traffic().Deadlocks(q(r, "siteId"))
		if out == nil {
			out = []core.Deadlock{}
		}
		writeJSON(w, 200, out)
	}))
	m.HandleFunc("GET "+v1+"/traffic/blocked", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		out := s.core.BlockedVehicles()
		if out == nil {
			out = []core.BlockedVehicle{}
		}
		writeJSON(w, 200, out)
	}))
	m.HandleFunc("POST "+v1+"/traffic/reservations/{id}/release", s.guard(auth.PermTrafficWrite, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ConfirmPhysicalState bool `json:"confirmPhysicalState"`
		}
		_ = decode(r, &b)
		if !b.ConfirmPhysicalState {
			writeErr(w, core.Validation("confirmPhysicalState must be true: the physical presence of the vehicle has to be verified"))
			return
		}
		if err := s.core.Traffic().ManualRelease(r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "traffic.release", r.PathValue("id"), b)
		writeJSON(w, 200, map[string]bool{"released": true})
	}))

	// charging
	m.HandleFunc("GET "+v1+"/charging/stations", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		site := q(r, "siteId")
		if site == "" {
			site = core.DefaultSiteID
		}
		out := s.core.ChargingStations(site)
		if out == nil {
			out = []fleet.ChargingStation{}
		}
		writeJSON(w, 200, out)
	}))

	// alarms
	m.HandleFunc("GET "+v1+"/alarms", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		out := s.core.Alarms().History(q(r, "siteId"), q(r, "active") == "true", qInt(r, "limit", 500))
		if out == nil {
			out = []core.Alarm{}
		}
		writeJSON(w, 200, out)
	}))
	m.HandleFunc("POST "+v1+"/alarms/{id}/ack", s.guard(auth.PermAlarmsAck, func(w http.ResponseWriter, r *http.Request) {
		if err := s.core.Alarms().Acknowledge(r.PathValue("id"), principalFrom(r).Username); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "alarm.ack", r.PathValue("id"), nil)
		a, _ := s.core.Alarms().Get(r.PathValue("id"))
		writeJSON(w, 200, a)
	}))
	m.HandleFunc("POST "+v1+"/alarms/{id}/clear", s.guard(auth.PermAlarmsAck, func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.core.Alarms().Get(r.PathValue("id"))
		if !ok {
			writeErr(w, core.NotFound("alarm", r.PathValue("id")))
			return
		}
		if a.Source == core.AlarmSrcVehicle || a.Source == core.AlarmSrcComms {
			writeErr(w, core.Conflict("alarms reported by the vehicle clear themselves when the condition disappears"))
			return
		}
		if err := s.core.Alarms().Clear(a.ID); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "alarm.clear", a.ID, nil)
		a2, _ := s.core.Alarms().Get(a.ID)
		writeJSON(w, 200, a2)
	}))

	// maintenance
	m.HandleFunc("GET "+v1+"/maintenance/plans", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		out := s.core.Maintenance().Plans()
		if out == nil {
			out = []core.MaintenancePlan{}
		}
		writeJSON(w, 200, out)
	}))
	m.HandleFunc("POST "+v1+"/maintenance/plans", s.guard(auth.PermMaintenance, s.savePlan))
	m.HandleFunc("PUT "+v1+"/maintenance/plans/{id}", s.guard(auth.PermMaintenance, s.savePlan))
	m.HandleFunc("DELETE "+v1+"/maintenance/plans/{id}", s.guard(auth.PermMaintenance, func(w http.ResponseWriter, r *http.Request) {
		if err := s.core.Maintenance().DeletePlan(r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "maintenance.plan.delete", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("GET "+v1+"/maintenance/status", s.guard(auth.PermRead, s.maintenanceStatus))
	m.HandleFunc("GET "+v1+"/maintenance/records", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		out := s.core.Maintenance().Records(q(r, "vehicleId"))
		if out == nil {
			out = []core.MaintenanceRecord{}
		}
		writeJSON(w, 200, out)
	}))
	m.HandleFunc("POST "+v1+"/maintenance/records", s.guard(auth.PermMaintenance, s.recordMaintenance))

	// analytics
	m.HandleFunc("GET "+v1+"/analytics/kpis", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.core.KPIs(q(r, "siteId"), qTime(r, "from"), qTime(r, "to")))
	}))
	m.HandleFunc("GET "+v1+"/analytics/timeseries", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		site := q(r, "siteId")
		if site == "" {
			site = core.DefaultSiteID
		}
		pts, err := s.core.Timeseries(site, q(r, "metric"), qTime(r, "from"), qTime(r, "to"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if pts == nil {
			pts = []core.TimeseriesPoint{}
		}
		writeJSON(w, 200, pts)
	}))

	// simulation
	m.HandleFunc("GET "+v1+"/simulation", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.core.SimulationStatus()) }))
	m.HandleFunc("POST "+v1+"/simulation/start", s.guard(auth.PermSimulation, func(w http.ResponseWriter, r *http.Request) {
		var b fleet.SimulationStart
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		st, err := s.core.StartSimulation(b)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "simulation.start", "", b)
		writeJSON(w, 200, st)
	}))
	m.HandleFunc("POST "+v1+"/simulation/stop", s.guard(auth.PermSimulation, func(w http.ResponseWriter, r *http.Request) {
		s.recordAudit(r, "simulation.stop", "", nil)
		writeJSON(w, 200, s.core.StopSimulation())
	}))
	m.HandleFunc("POST "+v1+"/simulation/timescale", s.guard(auth.PermSimulation, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			TimeScale float64 `json:"timeScale"`
		}
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, s.core.SetTimeScale(b.TimeScale))
	}))
	m.HandleFunc("POST "+v1+"/simulation/vehicles", s.guard(auth.PermSimulation, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ModelID string `json:"modelId"`
			NodeID  string `json:"nodeId"`
		}
		_ = decode(r, &b)
		v, err := s.core.SpawnSimulatedVehicle(b.ModelID, b.NodeID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 201, v)
	}))
	m.HandleFunc("POST "+v1+"/simulation/faults", s.guard(auth.PermSimulation, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			VehicleID string `json:"vehicleId"`
			Type      string `json:"type"`
		}
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.core.InjectFault(b.VehicleID, b.Type); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "simulation.fault", b.VehicleID, b)
		writeJSON(w, 200, s.core.SimulationStatus())
	}))
	m.HandleFunc("POST "+v1+"/simulation/scenario", s.guard(auth.PermSimulation, func(w http.ResponseWriter, r *http.Request) {
		var b fleet.SimulationScenario
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, s.core.SetScenario(b))
	}))

	// colony (ant-colony task allocation)
	m.HandleFunc("GET "+v1+"/colony/board", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		site := q(r, "siteId")
		if site == "" {
			site = core.DefaultSiteID
		}
		writeJSON(w, 200, s.core.Colony(site))
	}))

	// users / keys / audit / system
	m.HandleFunc("GET "+v1+"/users", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		us, _ := s.core.Store().ListUsers(r.Context())
		if us == nil {
			us = []core.User{}
		}
		writeJSON(w, 200, us)
	}))
	m.HandleFunc("POST "+v1+"/users", s.guard(auth.PermUsers, s.createUser))
	m.HandleFunc("PUT "+v1+"/users/{id}", s.guard(auth.PermUsers, s.updateUser))
	m.HandleFunc("DELETE "+v1+"/users/{id}", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == principalFrom(r).ID {
			writeErr(w, core.Conflict("you cannot delete your own account"))
			return
		}
		if err := s.core.Store().DeleteUser(r.Context(), r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "user.delete", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("POST "+v1+"/users/{id}/password", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Password string `json:"password"`
		}
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.auth.SetPassword(r.PathValue("id"), b.Password); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "user.password", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("GET "+v1+"/api-keys", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		ks, _ := s.core.Store().ListAPIKeys(r.Context())
		if ks == nil {
			ks = []core.APIKey{}
		}
		writeJSON(w, 200, ks)
	}))
	m.HandleFunc("POST "+v1+"/api-keys", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Name   string             `json:"name"`
			Role   core.Role          `json:"role"`
			Source core.MissionSource `json:"source"`
			SiteID string             `json:"siteId"`
		}
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		k, err := s.auth.CreateAPIKey(b.Name, b.Role, b.Source, b.SiteID)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "apikey.create", k.ID, map[string]string{"name": k.Name, "role": string(k.Role)})
		writeJSON(w, 201, k)
	}))
	m.HandleFunc("DELETE "+v1+"/api-keys/{id}", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		if err := s.core.Store().DeleteAPIKey(r.Context(), r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "apikey.delete", r.PathValue("id"), nil)
		w.WriteHeader(http.StatusNoContent)
	}))
	m.HandleFunc("GET "+v1+"/audit", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		out := s.audit.List(store.AuditFilter{UserID: q(r, "user"), Action: q(r, "action"), Limit: qInt(r, "limit", 200)})
		if out == nil {
			out = []core.AuditEntry{}
		}
		writeJSON(w, 200, out)
	}))
	m.HandleFunc("GET "+v1+"/system/info", s.guard(auth.PermRead, func(w http.ResponseWriter, r *http.Request) {
		online := 0
		for _, v := range s.core.Vehicles("") {
			if v.State.Connection == core.ConnOnline {
				online++
			}
		}
		writeJSON(w, 200, map[string]any{"version": s.info.Version, "uptimeS": time.Since(s.core.StartedAt()).Seconds(), "store": s.info.Store,
			"mqtt": map[string]any{"embedded": s.info.MQTTEmbed, "url": s.info.MQTTURL}, "vehiclesOnline": online, "time": s.core.Clock().Now(),
			"gateway": s.core.Gateway().Stats(), "discoveredVehicles": s.core.Gateway().Discovered(), "timeScale": s.core.Clock().Scale()})
	}))

	// integration
	m.HandleFunc("POST "+v1+"/integration/missions", s.guard(auth.PermIntegration, s.createMission))
	m.HandleFunc("GET "+v1+"/integration/missions/{ref}", s.guard(auth.PermIntegration, func(w http.ResponseWriter, r *http.Request) {
		ms, ok := s.core.Missions().ByExternalRef(r.PathValue("ref"))
		if !ok {
			writeErr(w, core.NotFound("mission with externalRef", r.PathValue("ref")))
			return
		}
		writeJSON(w, 200, ms)
	}))
	m.HandleFunc("GET "+v1+"/integration/webhooks", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.core.Webhooks()) }))
	m.HandleFunc("POST "+v1+"/integration/webhooks", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		var b core.Webhook
		if err := decode(r, &b); err != nil {
			writeErr(w, err)
			return
		}
		wh, err := s.core.SaveWebhook(b)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.recordAudit(r, "webhook.save", wh.ID, wh.URL)
		writeJSON(w, 201, wh)
	}))
	m.HandleFunc("DELETE "+v1+"/integration/webhooks/{id}", s.guard(auth.PermUsers, func(w http.ResponseWriter, r *http.Request) {
		if err := s.core.DeleteWebhook(r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok", "version": s.info.Version})
}
