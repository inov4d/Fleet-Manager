package api

import (
	"net/http"
	"time"

	"github.com/inov4d/fleet-manager/internal/auth"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/maintenance"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	tok, u, exp, err := s.auth.Login(b.Username, b.Password)
	if err != nil {
		s.audit.Record("", b.Username, "auth.login.failed", "", nil, clientIP(r))
		writeErr(w, err)
		return
	}
	s.audit.Record(u.ID, u.Username, "auth.login", "", nil, clientIP(r))
	writeJSON(w, 200, map[string]any{"token": tok, "user": u, "expiresAt": exp.Format(time.RFC3339)})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r)
	if p.APIKey {
		writeJSON(w, 200, core.User{ID: p.ID, Username: p.Username, DisplayName: p.Username, Role: p.Role, SiteIDs: []string{}, Active: true})
		return
	}
	u, err := s.core.Store().GetUser(r.Context(), p.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, u)
}

func (s *Server) saveSite(w http.ResponseWriter, r *http.Request) {
	var b core.Site
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	if id := r.PathValue("id"); id != "" {
		b.ID = id
	}
	st, err := s.core.SaveSite(b)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, "site.save", st.ID, st)
	writeJSON(w, 200, st)
}

func (s *Server) createVehicle(w http.ResponseWriter, r *http.Request) {
	var b core.Vehicle
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	v, err := s.core.CreateVehicle(b)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, "vehicle.create", v.Vehicle.ID, v.Vehicle)
	writeJSON(w, 201, v)
}

func (s *Server) updateVehicle(w http.ResponseWriter, r *http.Request) {
	var b core.Vehicle
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	v, err := s.core.UpdateVehicle(r.PathValue("id"), b)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, "vehicle.update", v.Vehicle.ID, v.Vehicle)
	writeJSON(w, 200, v)
}

func (s *Server) vehicleCommand(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Command string `json:"command"`
		NodeID  string `json:"nodeId"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	p := principalFrom(r)
	// supervisor-level commands
	switch b.Command {
	case "enable", "disable", "maintenance", "endMaintenance", "sendToCharge", "clearErrors", "sendToNode":
		if !auth.Can(p.Role, auth.PermVehiclesWrite) {
			writeErr(w, core.Forbidden("command %s requires the supervisor role", b.Command))
			return
		}
	}
	if err := s.core.Command(r.PathValue("id"), b.Command, b.NodeID, p.Username); err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, "vehicle.command."+b.Command, r.PathValue("id"), b)
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

func (s *Server) getMap(w http.ResponseWriter, r *http.Request) {
	mp, err := s.core.GetMap(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, mp)
}

func (s *Server) saveMap(w http.ResponseWriter, r *http.Request) {
	var b core.Map
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	if id := r.PathValue("id"); id != "" {
		b.ID = id
	} else if r.Method == http.MethodPost {
		// POST creates: a client-provided id is honoured only when unused
		if b.ID != "" {
			if _, err := s.core.GetMap(b.ID); err == nil {
				b.ID = ""
			}
		}
	}
	mp, err := s.core.SaveMap(b)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, "map.save", mp.ID, map[string]any{"name": mp.Name, "version": mp.Version, "nodes": len(mp.Nodes), "edges": len(mp.Edges), "zones": len(mp.Zones)})
	status := 200
	if r.Method == http.MethodPost {
		status = 201
	}
	writeJSON(w, status, mp)
}

func (s *Server) createMission(w http.ResponseWriter, r *http.Request) {
	var b core.Mission
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	p := principalFrom(r)
	b.CreatedBy = p.Username
	if p.APIKey {
		b.Source = p.Source
		if p.SiteID != "" {
			b.SiteID = p.SiteID
		}
		if b.ExternalRef != "" {
			if ex, ok := s.core.Missions().ByExternalRef(b.ExternalRef); ok && !ex.Status.Terminal() {
				writeJSON(w, 200, ex) // idempotent creation
				return
			}
		}
	} else if b.Source == "" {
		b.Source = core.SourceOperator
	}
	b.ID = ""
	b.VehicleID = ""
	ms, err := s.core.CreateMission(b)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, "mission.create", ms.ID, map[string]any{"type": ms.Type, "origin": ms.Origin, "destination": ms.Destination, "priority": ms.Priority})
	writeJSON(w, 201, ms)
}

func (s *Server) missionResult(w http.ResponseWriter, r *http.Request, action string, ms *core.Mission, err error) {
	if err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, action, ms.ID, nil)
	writeJSON(w, 200, ms)
}

func (s *Server) savePlan(w http.ResponseWriter, r *http.Request) {
	var b core.MaintenancePlan
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	if id := r.PathValue("id"); id != "" {
		b.ID = id
	}
	if err := s.core.Maintenance().SavePlan(&b); err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, "maintenance.plan.save", b.ID, b)
	writeJSON(w, 200, b)
}

func (s *Server) maintenanceStatus(w http.ResponseWriter, r *http.Request) {
	vs := s.core.Vehicles(q(r, "siteId"))
	vehicles := make([]core.Vehicle, 0, len(vs))
	states := map[string]core.VehicleState{}
	for _, v := range vs {
		vehicles = append(vehicles, v.Vehicle)
		states[v.Vehicle.ID] = v.State
	}
	out := s.core.Maintenance().Evaluate(vehicles, func(id string) maintenance.Counters {
		st := states[id]
		return maintenance.Counters{OdometerM: st.OdometerM, OperatingHours: st.OperatingHours, Missions: st.MissionsCompleted}
	})
	if out == nil {
		out = []core.MaintenanceStatus{}
	}
	writeJSON(w, 200, out)
}

func (s *Server) recordMaintenance(w http.ResponseWriter, r *http.Request) {
	var b struct {
		VehicleID string `json:"vehicleId"`
		PlanID    string `json:"planId"`
		Notes     string `json:"notes"`
	}
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	v, err := s.core.Vehicle(b.VehicleID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var plan *core.MaintenancePlan
	for _, p := range s.core.Maintenance().Plans() {
		if p.ID == b.PlanID {
			pp := p
			plan = &pp
		}
	}
	if plan == nil {
		writeErr(w, core.NotFound("maintenance plan", b.PlanID))
		return
	}
	value := maintenance.CounterValue(plan.Metric, maintenance.Counters{OdometerM: v.State.OdometerM, OperatingHours: v.State.OperatingHours, Missions: v.State.MissionsCompleted})
	rec, err := s.core.Maintenance().Record(plan.ID, b.VehicleID, principalFrom(r).Username, b.Notes, value)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, "maintenance.record", rec.ID, b)
	writeJSON(w, 201, rec)
}

type userBody struct {
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Role        core.Role `json:"role"`
	SiteIDs     []string  `json:"siteIds"`
	Password    string    `json:"password"`
	Active      *bool     `json:"active"`
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var b userBody
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	u, err := s.auth.CreateUser(b.Username, b.DisplayName, b.Password, b.Role, b.SiteIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.recordAudit(r, "user.create", u.ID, map[string]any{"username": u.Username, "role": u.Role})
	writeJSON(w, 201, u)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	var b userBody
	if err := decode(r, &b); err != nil {
		writeErr(w, err)
		return
	}
	u, err := s.core.Store().GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if b.Role != "" {
		if !auth.ValidRole(b.Role) {
			writeErr(w, core.Validation("invalid role"))
			return
		}
		if u.ID == principalFrom(r).ID && b.Role != core.RoleAdmin {
			writeErr(w, core.Conflict("you cannot remove your own admin role"))
			return
		}
		u.Role = b.Role
	}
	if b.DisplayName != "" {
		u.DisplayName = b.DisplayName
	}
	if b.Username != "" && b.Username != u.Username {
		if _, err := s.core.Store().GetUserByName(r.Context(), b.Username); err == nil {
			writeErr(w, core.Conflict("username already used"))
			return
		}
		u.Username = b.Username
	}
	if b.SiteIDs != nil {
		u.SiteIDs = b.SiteIDs
	}
	if b.Active != nil {
		u.Active = *b.Active
	}
	if err := s.core.Store().SaveUser(r.Context(), u); err != nil {
		writeErr(w, err)
		return
	}
	if b.Password != "" {
		if err := s.auth.SetPassword(u.ID, b.Password); err != nil {
			writeErr(w, err)
			return
		}
	}
	s.recordAudit(r, "user.update", u.ID, map[string]any{"username": u.Username, "role": u.Role, "active": u.Active})
	writeJSON(w, 200, u)
}
