package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
)

// Memory is an in-memory store with optional JSON snapshot persistence.
type Memory struct {
	mu       sync.RWMutex
	saveMu   sync.Mutex
	path     string
	dirty    bool
	stopSave chan struct{}
	loopDone chan struct{}

	Sites        map[string]*core.Site
	Models       map[string]*core.VehicleModel
	Vehicles     map[string]*core.Vehicle
	Maps         map[string]*core.Map
	Missions     map[string]*core.Mission
	Alarms       map[string]*core.Alarm
	Plans        map[string]*core.MaintenancePlan
	Records      map[string]*core.MaintenanceRecord
	Users        map[string]*core.User
	APIKeys      map[string]*core.APIKey
	Webhooks     map[string]*core.Webhook
	Audit        []core.AuditEntry
	Telemetry    map[string][]core.TelemetrySample
	Metrics      map[string][]core.TimeseriesPoint
	Settings     map[string]json.RawMessage
	TelemetryMax int
}

type snapshot struct {
	Sites    []core.Site                `json:"sites"`
	Models   []core.VehicleModel        `json:"models"`
	Vehicles []core.Vehicle             `json:"vehicles"`
	Maps     []core.Map                 `json:"maps"`
	Missions []core.Mission             `json:"missions"`
	Alarms   []core.Alarm               `json:"alarms"`
	Plans    []core.MaintenancePlan     `json:"plans"`
	Records  []core.MaintenanceRecord   `json:"records"`
	Users    []core.User                `json:"users"`
	UserHash map[string]string          `json:"userHash"`
	APIKeys  []core.APIKey              `json:"apiKeys"`
	KeyHash  map[string]string          `json:"keyHash"`
	Webhooks []core.Webhook             `json:"webhooks"`
	Audit    []core.AuditEntry          `json:"audit"`
	Settings map[string]json.RawMessage `json:"settings"`
}

// NewMemory creates a store; when path is not empty the content is loaded from
// and periodically saved to that JSON file.
func NewMemory(path string) (*Memory, error) {
	m := &Memory{path: path, Sites: map[string]*core.Site{}, Models: map[string]*core.VehicleModel{}, Vehicles: map[string]*core.Vehicle{},
		Maps: map[string]*core.Map{}, Missions: map[string]*core.Mission{}, Alarms: map[string]*core.Alarm{}, Plans: map[string]*core.MaintenancePlan{},
		Records: map[string]*core.MaintenanceRecord{}, Users: map[string]*core.User{}, APIKeys: map[string]*core.APIKey{}, Webhooks: map[string]*core.Webhook{},
		Telemetry: map[string][]core.TelemetrySample{}, Metrics: map[string][]core.TimeseriesPoint{}, Settings: map[string]json.RawMessage{}, TelemetryMax: 20000}
	if path != "" {
		if err := m.load(); err != nil {
			return nil, err
		}
		m.stopSave = make(chan struct{})
		m.loopDone = make(chan struct{})
		go m.saveLoop()
	}
	return m, nil
}

func (m *Memory) Kind() string { return "memory" }

func (m *Memory) load() error {
	b, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var s snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for i := range s.Sites {
		m.Sites[s.Sites[i].ID] = &s.Sites[i]
	}
	for i := range s.Models {
		m.Models[s.Models[i].ID] = &s.Models[i]
	}
	for i := range s.Vehicles {
		m.Vehicles[s.Vehicles[i].ID] = &s.Vehicles[i]
	}
	for i := range s.Maps {
		m.Maps[s.Maps[i].ID] = &s.Maps[i]
	}
	for i := range s.Missions {
		m.Missions[s.Missions[i].ID] = &s.Missions[i]
	}
	for i := range s.Alarms {
		m.Alarms[s.Alarms[i].ID] = &s.Alarms[i]
	}
	for i := range s.Plans {
		m.Plans[s.Plans[i].ID] = &s.Plans[i]
	}
	for i := range s.Records {
		m.Records[s.Records[i].ID] = &s.Records[i]
	}
	for i := range s.Users {
		u := s.Users[i]
		u.PasswordHash = s.UserHash[u.ID]
		m.Users[u.ID] = &u
	}
	for i := range s.APIKeys {
		k := s.APIKeys[i]
		k.KeyHash = s.KeyHash[k.ID]
		m.APIKeys[k.ID] = &k
	}
	for i := range s.Webhooks {
		m.Webhooks[s.Webhooks[i].ID] = &s.Webhooks[i]
	}
	m.Audit = s.Audit
	if s.Settings != nil {
		m.Settings = s.Settings
	}
	return nil
}

func (m *Memory) saveLoop() {
	defer close(m.loopDone)
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-m.stopSave:
			_ = m.Save()
			return
		case <-t.C:
			m.mu.RLock()
			d := m.dirty
			m.mu.RUnlock()
			if d {
				_ = m.Save()
			}
		}
	}
}

// Save writes the snapshot to disk (serialised: one writer at a time).
func (m *Memory) Save() error {
	if m.path == "" {
		return nil
	}
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	m.mu.Lock()
	s := snapshot{UserHash: map[string]string{}, KeyHash: map[string]string{}, Settings: m.Settings}
	for _, v := range m.Sites {
		s.Sites = append(s.Sites, *v)
	}
	for _, v := range m.Models {
		s.Models = append(s.Models, *v)
	}
	for _, v := range m.Vehicles {
		s.Vehicles = append(s.Vehicles, *v)
	}
	for _, v := range m.Maps {
		s.Maps = append(s.Maps, *v)
	}
	for _, v := range m.Missions {
		s.Missions = append(s.Missions, *v)
	}
	for _, v := range m.Alarms {
		s.Alarms = append(s.Alarms, *v)
	}
	for _, v := range m.Plans {
		s.Plans = append(s.Plans, *v)
	}
	for _, v := range m.Records {
		s.Records = append(s.Records, *v)
	}
	for _, v := range m.Users {
		s.Users = append(s.Users, *v)
		s.UserHash[v.ID] = v.PasswordHash
	}
	for _, v := range m.APIKeys {
		k := *v
		k.Key = ""
		s.APIKeys = append(s.APIKeys, k)
		s.KeyHash[v.ID] = v.KeyHash
	}
	for _, v := range m.Webhooks {
		s.Webhooks = append(s.Webhooks, *v)
	}
	if len(m.Audit) > 5000 {
		m.Audit = m.Audit[len(m.Audit)-5000:]
	}
	s.Audit = m.Audit
	m.dirty = false
	m.mu.Unlock()
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

func (m *Memory) Close() error {
	if m.stopSave != nil {
		close(m.stopSave)
		<-m.loopDone // the loop performs the final save
		m.stopSave = nil
	}
	return nil
}

func (m *Memory) touch() { m.dirty = true }

// --- sites

func (m *Memory) ListSites(_ context.Context) ([]core.Site, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.Site, 0, len(m.Sites))
	for _, s := range m.Sites {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) GetSite(_ context.Context, id string) (*core.Site, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s := m.Sites[id]; s != nil {
		c := *s
		return &c, nil
	}
	return nil, core.NotFound("site", id)
}

func (m *Memory) SaveSite(_ context.Context, s *core.Site) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *s
	m.Sites[s.ID] = &c
	m.touch()
	return nil
}

func (m *Memory) DeleteSite(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Sites, id)
	m.touch()
	return nil
}

// --- models

func (m *Memory) ListModels(_ context.Context) ([]core.VehicleModel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.VehicleModel, 0, len(m.Models))
	for _, v := range m.Models {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *Memory) SaveModel(_ context.Context, v *core.VehicleModel) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.Models[v.ID] = &c
	m.touch()
	return nil
}

// --- vehicles

func (m *Memory) ListVehicles(_ context.Context, siteID string) ([]core.Vehicle, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.Vehicle, 0, len(m.Vehicles))
	for _, v := range m.Vehicles {
		if siteID == "" || v.SiteID == siteID {
			out = append(out, *v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) GetVehicle(_ context.Context, id string) (*core.Vehicle, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v := m.Vehicles[id]; v != nil {
		c := *v
		return &c, nil
	}
	return nil, core.NotFound("vehicle", id)
}

func (m *Memory) SaveVehicle(_ context.Context, v *core.Vehicle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.Vehicles[v.ID] = &c
	m.touch()
	return nil
}

func (m *Memory) DeleteVehicle(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Vehicles, id)
	m.touch()
	return nil
}

// --- maps

func (m *Memory) ListMaps(_ context.Context, siteID string) ([]core.Map, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.Map, 0, len(m.Maps))
	for _, v := range m.Maps {
		if siteID == "" || v.SiteID == siteID {
			out = append(out, *v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) GetMap(_ context.Context, id string) (*core.Map, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v := m.Maps[id]; v != nil {
		return cloneMap(v), nil
	}
	return nil, core.NotFound("map", id)
}

func cloneMap(v *core.Map) *core.Map {
	b, _ := json.Marshal(v)
	var c core.Map
	_ = json.Unmarshal(b, &c)
	return &c
}

func (m *Memory) SaveMap(_ context.Context, v *core.Map) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Maps[v.ID] = cloneMap(v)
	m.touch()
	return nil
}

func (m *Memory) DeleteMap(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Maps, id)
	m.touch()
	return nil
}

// --- missions

func (m *Memory) ListMissions(_ context.Context, f MissionFilter) ([]core.Mission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.Mission, 0)
	for _, v := range m.Missions {
		if f.SiteID != "" && v.SiteID != f.SiteID {
			continue
		}
		if f.Status != "" && string(v.Status) != f.Status {
			continue
		}
		if f.VehicleID != "" && v.VehicleID != f.VehicleID {
			continue
		}
		if f.ActiveOnly && v.Status.Terminal() {
			continue
		}
		out = append(out, *cloneMission(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func cloneMission(v *core.Mission) *core.Mission {
	b, _ := json.Marshal(v)
	var c core.Mission
	_ = json.Unmarshal(b, &c)
	return &c
}

func (m *Memory) GetMission(_ context.Context, id string) (*core.Mission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v := m.Missions[id]; v != nil {
		return cloneMission(v), nil
	}
	return nil, core.NotFound("mission", id)
}

func (m *Memory) SaveMission(_ context.Context, v *core.Mission) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Missions[v.ID] = cloneMission(v)
	m.touch()
	return nil
}

func (m *Memory) DeleteMission(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Missions, id)
	m.touch()
	return nil
}

// --- alarms

func (m *Memory) ListAlarms(_ context.Context, siteID string, activeOnly bool, limit int) ([]core.Alarm, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.Alarm, 0)
	for _, v := range m.Alarms {
		if siteID != "" && v.SiteID != siteID {
			continue
		}
		if activeOnly && !v.Active {
			continue
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RaisedAt.After(out[j].RaisedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) SaveAlarm(_ context.Context, v *core.Alarm) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.Alarms[v.ID] = &c
	m.touch()
	return nil
}

// --- maintenance

func (m *Memory) ListMaintenancePlans(_ context.Context) ([]core.MaintenancePlan, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.MaintenancePlan, 0, len(m.Plans))
	for _, v := range m.Plans {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) SaveMaintenancePlan(_ context.Context, v *core.MaintenancePlan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.Plans[v.ID] = &c
	m.touch()
	return nil
}

func (m *Memory) DeleteMaintenancePlan(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Plans, id)
	m.touch()
	return nil
}

func (m *Memory) ListMaintenanceRecords(_ context.Context, vehicleID string) ([]core.MaintenanceRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.MaintenanceRecord, 0)
	for _, v := range m.Records {
		if vehicleID == "" || v.VehicleID == vehicleID {
			out = append(out, *v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DoneAt.After(out[j].DoneAt) })
	return out, nil
}

func (m *Memory) SaveMaintenanceRecord(_ context.Context, v *core.MaintenanceRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.Records[v.ID] = &c
	m.touch()
	return nil
}

// --- users

func (m *Memory) ListUsers(_ context.Context) ([]core.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.User, 0, len(m.Users))
	for _, v := range m.Users {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

func (m *Memory) GetUser(_ context.Context, id string) (*core.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v := m.Users[id]; v != nil {
		c := *v
		return &c, nil
	}
	return nil, core.NotFound("user", id)
}

func (m *Memory) GetUserByName(_ context.Context, username string) (*core.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, v := range m.Users {
		if strings.EqualFold(v.Username, username) {
			c := *v
			return &c, nil
		}
	}
	return nil, core.NotFound("user", username)
}

func (m *Memory) SaveUser(_ context.Context, v *core.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.Users[v.ID] = &c
	m.touch()
	return nil
}

func (m *Memory) DeleteUser(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Users, id)
	m.touch()
	return nil
}

func (m *Memory) ListAPIKeys(_ context.Context) ([]core.APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.APIKey, 0, len(m.APIKeys))
	for _, v := range m.APIKeys {
		c := *v
		c.Key = ""
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) SaveAPIKey(_ context.Context, v *core.APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	c.Key = ""
	m.APIKeys[v.ID] = &c
	m.touch()
	return nil
}

func (m *Memory) DeleteAPIKey(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.APIKeys, id)
	m.touch()
	return nil
}

func (m *Memory) ListWebhooks(_ context.Context) ([]core.Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.Webhook, 0, len(m.Webhooks))
	for _, v := range m.Webhooks {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *Memory) SaveWebhook(_ context.Context, v *core.Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *v
	m.Webhooks[v.ID] = &c
	m.touch()
	return nil
}

func (m *Memory) DeleteWebhook(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Webhooks, id)
	m.touch()
	return nil
}

// --- audit

func (m *Memory) AppendAudit(_ context.Context, e *core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Audit = append(m.Audit, *e)
	if len(m.Audit) > 10000 {
		m.Audit = m.Audit[len(m.Audit)-10000:]
	}
	m.touch()
	return nil
}

func (m *Memory) ListAudit(_ context.Context, f AuditFilter) ([]core.AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.AuditEntry, 0)
	for i := len(m.Audit) - 1; i >= 0; i-- {
		e := m.Audit[i]
		if f.UserID != "" && e.UserID != f.UserID && e.Username != f.UserID {
			continue
		}
		if f.Action != "" && !strings.HasPrefix(e.Action, f.Action) {
			continue
		}
		out = append(out, e)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out, nil
}

// --- telemetry

func (m *Memory) AppendTelemetry(_ context.Context, samples []core.TelemetrySample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range samples {
		l := append(m.Telemetry[s.VehicleID], s)
		if len(l) > m.TelemetryMax {
			l = l[len(l)-m.TelemetryMax:]
		}
		m.Telemetry[s.VehicleID] = l
	}
	return nil
}

func (m *Memory) QueryTelemetry(_ context.Context, vehicleID string, from, to time.Time, limit int) ([]core.TelemetrySample, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.TelemetrySample, 0)
	for _, s := range m.Telemetry[vehicleID] {
		if (!from.IsZero() && s.TS.Before(from)) || (!to.IsZero() && s.TS.After(to)) {
			continue
		}
		out = append(out, s)
	}
	if limit > 0 && len(out) > limit {
		// down-sample evenly
		step := float64(len(out)) / float64(limit)
		ds := make([]core.TelemetrySample, 0, limit)
		for i := 0; i < limit; i++ {
			ds = append(ds, out[int(float64(i)*step)])
		}
		out = ds
	}
	return out, nil
}

func (m *Memory) AppendMetric(_ context.Context, siteID, metric string, ts time.Time, value float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := siteID + "/" + metric
	l := append(m.Metrics[k], core.TimeseriesPoint{TS: ts, Value: value})
	if len(l) > 20000 {
		l = l[len(l)-20000:]
	}
	m.Metrics[k] = l
	return nil
}

func (m *Memory) QueryMetric(_ context.Context, siteID, metric string, from, to time.Time) ([]core.TimeseriesPoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.TimeseriesPoint, 0)
	for _, p := range m.Metrics[siteID+"/"+metric] {
		if (!from.IsZero() && p.TS.Before(from)) || (!to.IsZero() && p.TS.After(to)) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (m *Memory) GetSetting(_ context.Context, key string, out any) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.Settings[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(raw, out)
}

func (m *Memory) SetSetting(_ context.Context, key string, v any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Settings[key] = marshal(v)
	m.touch()
	return nil
}
