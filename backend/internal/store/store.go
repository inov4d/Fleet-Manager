// Package store persists configuration and history. Runtime state (positions,
// reservations) lives in memory; everything an operator configures or audits
// goes through Store. Two implementations exist: Memory (optionally snapshot
// to a JSON file, for single-box deployments) and Postgres.
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
)

// MissionFilter narrows mission listings.
type MissionFilter struct {
	SiteID     string
	Status     string
	VehicleID  string
	Limit      int
	ActiveOnly bool
}

// AuditFilter narrows audit listings.
type AuditFilter struct {
	UserID string
	Action string
	Limit  int
}

// Store is the persistence contract.
type Store interface {
	// sites
	ListSites(ctx context.Context) ([]core.Site, error)
	GetSite(ctx context.Context, id string) (*core.Site, error)
	SaveSite(ctx context.Context, s *core.Site) error
	DeleteSite(ctx context.Context, id string) error
	// vehicle models
	ListModels(ctx context.Context) ([]core.VehicleModel, error)
	SaveModel(ctx context.Context, m *core.VehicleModel) error
	// vehicles
	ListVehicles(ctx context.Context, siteID string) ([]core.Vehicle, error)
	GetVehicle(ctx context.Context, id string) (*core.Vehicle, error)
	SaveVehicle(ctx context.Context, v *core.Vehicle) error
	DeleteVehicle(ctx context.Context, id string) error
	// maps
	ListMaps(ctx context.Context, siteID string) ([]core.Map, error)
	GetMap(ctx context.Context, id string) (*core.Map, error)
	SaveMap(ctx context.Context, m *core.Map) error
	DeleteMap(ctx context.Context, id string) error
	// missions
	ListMissions(ctx context.Context, f MissionFilter) ([]core.Mission, error)
	GetMission(ctx context.Context, id string) (*core.Mission, error)
	SaveMission(ctx context.Context, m *core.Mission) error
	DeleteMission(ctx context.Context, id string) error
	// alarms
	ListAlarms(ctx context.Context, siteID string, activeOnly bool, limit int) ([]core.Alarm, error)
	SaveAlarm(ctx context.Context, a *core.Alarm) error
	// maintenance
	ListMaintenancePlans(ctx context.Context) ([]core.MaintenancePlan, error)
	SaveMaintenancePlan(ctx context.Context, p *core.MaintenancePlan) error
	DeleteMaintenancePlan(ctx context.Context, id string) error
	ListMaintenanceRecords(ctx context.Context, vehicleID string) ([]core.MaintenanceRecord, error)
	SaveMaintenanceRecord(ctx context.Context, r *core.MaintenanceRecord) error
	// users / keys / webhooks
	ListUsers(ctx context.Context) ([]core.User, error)
	GetUser(ctx context.Context, id string) (*core.User, error)
	GetUserByName(ctx context.Context, username string) (*core.User, error)
	SaveUser(ctx context.Context, u *core.User) error
	DeleteUser(ctx context.Context, id string) error
	ListAPIKeys(ctx context.Context) ([]core.APIKey, error)
	SaveAPIKey(ctx context.Context, k *core.APIKey) error
	DeleteAPIKey(ctx context.Context, id string) error
	ListWebhooks(ctx context.Context) ([]core.Webhook, error)
	SaveWebhook(ctx context.Context, w *core.Webhook) error
	DeleteWebhook(ctx context.Context, id string) error
	// audit
	AppendAudit(ctx context.Context, e *core.AuditEntry) error
	ListAudit(ctx context.Context, f AuditFilter) ([]core.AuditEntry, error)
	// telemetry / metrics
	AppendTelemetry(ctx context.Context, samples []core.TelemetrySample) error
	QueryTelemetry(ctx context.Context, vehicleID string, from, to time.Time, limit int) ([]core.TelemetrySample, error)
	AppendMetric(ctx context.Context, siteID, metric string, ts time.Time, value float64) error
	QueryMetric(ctx context.Context, siteID, metric string, from, to time.Time) ([]core.TimeseriesPoint, error)
	// settings (json documents by key)
	GetSetting(ctx context.Context, key string, out any) (bool, error)
	SetSetting(ctx context.Context, key string, v any) error

	Kind() string
	Close() error
}

func marshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
