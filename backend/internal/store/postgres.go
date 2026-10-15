package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/inov4d/fleet-manager/internal/core"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Postgres implements Store on PostgreSQL (TimescaleDB compatible).
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres connects and applies migrations.
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	p := &Postgres{pool: pool}
	if err := p.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return p, nil
}

func (p *Postgres) migrate(ctx context.Context) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if _, err := p.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	for _, n := range names {
		var exists bool
		if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, n).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sqlBytes, err := migrations.ReadFile("migrations/" + n)
		if err != nil {
			return err
		}
		if _, err := p.pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("migration %s: %w", n, err)
		}
		if _, err := p.pool.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, n); err != nil {
			return err
		}
	}
	return nil
}

func (p *Postgres) Kind() string { return "postgres" }
func (p *Postgres) Close() error { p.pool.Close(); return nil }

func notFoundIf(err error, what, id string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return core.NotFound(what, id)
	}
	return err
}

func listDocs[T any](ctx context.Context, p *Postgres, q string, args ...any) ([]T, error) {
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]T, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func getDoc[T any](ctx context.Context, p *Postgres, what, id, q string, args ...any) (*T, error) {
	var raw []byte
	if err := p.pool.QueryRow(ctx, q, args...).Scan(&raw); err != nil {
		return nil, notFoundIf(err, what, id)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// --- sites
func (p *Postgres) ListSites(ctx context.Context) ([]core.Site, error) {
	return listDocs[core.Site](ctx, p, `SELECT data FROM sites ORDER BY data->>'name'`)
}
func (p *Postgres) GetSite(ctx context.Context, id string) (*core.Site, error) {
	return getDoc[core.Site](ctx, p, "site", id, `SELECT data FROM sites WHERE id=$1`, id)
}
func (p *Postgres) SaveSite(ctx context.Context, s *core.Site) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO sites(id,data) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET data=EXCLUDED.data`, s.ID, marshal(s))
	return err
}
func (p *Postgres) DeleteSite(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM sites WHERE id=$1`, id)
	return err
}

// --- models
func (p *Postgres) ListModels(ctx context.Context) ([]core.VehicleModel, error) {
	return listDocs[core.VehicleModel](ctx, p, `SELECT data FROM vehicle_models ORDER BY id`)
}
func (p *Postgres) SaveModel(ctx context.Context, m *core.VehicleModel) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO vehicle_models(id,data) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET data=EXCLUDED.data`, m.ID, marshal(m))
	return err
}

// --- vehicles
func (p *Postgres) ListVehicles(ctx context.Context, siteID string) ([]core.Vehicle, error) {
	if siteID == "" {
		return listDocs[core.Vehicle](ctx, p, `SELECT data FROM vehicles ORDER BY data->>'name'`)
	}
	return listDocs[core.Vehicle](ctx, p, `SELECT data FROM vehicles WHERE site_id=$1 ORDER BY data->>'name'`, siteID)
}
func (p *Postgres) GetVehicle(ctx context.Context, id string) (*core.Vehicle, error) {
	return getDoc[core.Vehicle](ctx, p, "vehicle", id, `SELECT data FROM vehicles WHERE id=$1`, id)
}
func (p *Postgres) SaveVehicle(ctx context.Context, v *core.Vehicle) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO vehicles(id,site_id,data) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET site_id=EXCLUDED.site_id, data=EXCLUDED.data`, v.ID, v.SiteID, marshal(v))
	return err
}
func (p *Postgres) DeleteVehicle(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM vehicles WHERE id=$1`, id)
	return err
}

// --- maps
func (p *Postgres) ListMaps(ctx context.Context, siteID string) ([]core.Map, error) {
	if siteID == "" {
		return listDocs[core.Map](ctx, p, `SELECT data FROM maps ORDER BY data->>'name'`)
	}
	return listDocs[core.Map](ctx, p, `SELECT data FROM maps WHERE site_id=$1 ORDER BY data->>'name'`, siteID)
}
func (p *Postgres) GetMap(ctx context.Context, id string) (*core.Map, error) {
	return getDoc[core.Map](ctx, p, "map", id, `SELECT data FROM maps WHERE id=$1`, id)
}
func (p *Postgres) SaveMap(ctx context.Context, m *core.Map) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO maps(id,site_id,data) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET site_id=EXCLUDED.site_id, data=EXCLUDED.data`, m.ID, m.SiteID, marshal(m))
	return err
}
func (p *Postgres) DeleteMap(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM maps WHERE id=$1`, id)
	return err
}

// --- missions
func (p *Postgres) ListMissions(ctx context.Context, f MissionFilter) ([]core.Mission, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.SiteID != "" {
		add("site_id=$%d", f.SiteID)
	}
	if f.Status != "" {
		add("status=$%d", f.Status)
	}
	if f.VehicleID != "" {
		add("vehicle_id=$%d", f.VehicleID)
	}
	if f.ActiveOnly {
		where = append(where, "status NOT IN ('completed','failed','cancelled')")
	}
	q := `SELECT data FROM missions`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	return listDocs[core.Mission](ctx, p, q, args...)
}
func (p *Postgres) GetMission(ctx context.Context, id string) (*core.Mission, error) {
	return getDoc[core.Mission](ctx, p, "mission", id, `SELECT data FROM missions WHERE id=$1`, id)
}
func (p *Postgres) SaveMission(ctx context.Context, m *core.Mission) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO missions(id,site_id,status,vehicle_id,created_at,data) VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT(id) DO UPDATE SET site_id=EXCLUDED.site_id, status=EXCLUDED.status, vehicle_id=EXCLUDED.vehicle_id, data=EXCLUDED.data`,
		m.ID, m.SiteID, string(m.Status), m.VehicleID, m.CreatedAt, marshal(m))
	return err
}
func (p *Postgres) DeleteMission(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM missions WHERE id=$1`, id)
	return err
}

// --- alarms
func (p *Postgres) ListAlarms(ctx context.Context, siteID string, activeOnly bool, limit int) ([]core.Alarm, error) {
	q := `SELECT data FROM alarms WHERE ($1='' OR site_id=$1) AND (NOT $2 OR active) ORDER BY raised_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	return listDocs[core.Alarm](ctx, p, q, siteID, activeOnly)
}
func (p *Postgres) SaveAlarm(ctx context.Context, a *core.Alarm) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO alarms(id,site_id,active,raised_at,data) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT(id) DO UPDATE SET active=EXCLUDED.active, data=EXCLUDED.data`, a.ID, a.SiteID, a.Active, a.RaisedAt, marshal(a))
	return err
}

// --- maintenance
func (p *Postgres) ListMaintenancePlans(ctx context.Context) ([]core.MaintenancePlan, error) {
	return listDocs[core.MaintenancePlan](ctx, p, `SELECT data FROM maintenance_plans ORDER BY data->>'name'`)
}
func (p *Postgres) SaveMaintenancePlan(ctx context.Context, m *core.MaintenancePlan) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO maintenance_plans(id,data) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET data=EXCLUDED.data`, m.ID, marshal(m))
	return err
}
func (p *Postgres) DeleteMaintenancePlan(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM maintenance_plans WHERE id=$1`, id)
	return err
}
func (p *Postgres) ListMaintenanceRecords(ctx context.Context, vehicleID string) ([]core.MaintenanceRecord, error) {
	return listDocs[core.MaintenanceRecord](ctx, p, `SELECT data FROM maintenance_records WHERE ($1='' OR vehicle_id=$1) ORDER BY done_at DESC`, vehicleID)
}
func (p *Postgres) SaveMaintenanceRecord(ctx context.Context, r *core.MaintenanceRecord) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO maintenance_records(id,vehicle_id,done_at,data) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET data=EXCLUDED.data`, r.ID, r.VehicleID, r.DoneAt, marshal(r))
	return err
}

// --- users
type userRow struct {
	core.User
}

func (p *Postgres) ListUsers(ctx context.Context) ([]core.User, error) {
	rows, err := p.pool.Query(ctx, `SELECT data, password_hash FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]core.User, 0)
	for rows.Next() {
		var raw []byte
		var hash string
		if err := rows.Scan(&raw, &hash); err != nil {
			return nil, err
		}
		var u core.User
		if err := json.Unmarshal(raw, &u); err != nil {
			return nil, err
		}
		u.PasswordHash = hash
		out = append(out, u)
	}
	return out, rows.Err()
}
func (p *Postgres) getUser(ctx context.Context, q string, arg string) (*core.User, error) {
	var raw []byte
	var hash string
	if err := p.pool.QueryRow(ctx, q, arg).Scan(&raw, &hash); err != nil {
		return nil, notFoundIf(err, "user", arg)
	}
	var u core.User
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil, err
	}
	u.PasswordHash = hash
	return &u, nil
}
func (p *Postgres) GetUser(ctx context.Context, id string) (*core.User, error) {
	return p.getUser(ctx, `SELECT data, password_hash FROM users WHERE id=$1`, id)
}
func (p *Postgres) GetUserByName(ctx context.Context, username string) (*core.User, error) {
	return p.getUser(ctx, `SELECT data, password_hash FROM users WHERE lower(username)=lower($1)`, username)
}
func (p *Postgres) SaveUser(ctx context.Context, u *core.User) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO users(id,username,password_hash,data) VALUES($1,$2,$3,$4)
		ON CONFLICT(id) DO UPDATE SET username=EXCLUDED.username, password_hash=EXCLUDED.password_hash, data=EXCLUDED.data`, u.ID, u.Username, u.PasswordHash, marshal(u))
	return err
}
func (p *Postgres) DeleteUser(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	return err
}

func (p *Postgres) ListAPIKeys(ctx context.Context) ([]core.APIKey, error) {
	rows, err := p.pool.Query(ctx, `SELECT data, key_hash FROM api_keys ORDER BY data->>'name'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]core.APIKey, 0)
	for rows.Next() {
		var raw []byte
		var hash string
		if err := rows.Scan(&raw, &hash); err != nil {
			return nil, err
		}
		var k core.APIKey
		if err := json.Unmarshal(raw, &k); err != nil {
			return nil, err
		}
		k.KeyHash = hash
		out = append(out, k)
	}
	return out, rows.Err()
}
func (p *Postgres) SaveAPIKey(ctx context.Context, k *core.APIKey) error {
	c := *k
	c.Key = ""
	_, err := p.pool.Exec(ctx, `INSERT INTO api_keys(id,key_hash,data) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET key_hash=EXCLUDED.key_hash, data=EXCLUDED.data`, k.ID, k.KeyHash, marshal(c))
	return err
}
func (p *Postgres) DeleteAPIKey(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM api_keys WHERE id=$1`, id)
	return err
}
func (p *Postgres) ListWebhooks(ctx context.Context) ([]core.Webhook, error) {
	return listDocs[core.Webhook](ctx, p, `SELECT data FROM webhooks ORDER BY id`)
}
func (p *Postgres) SaveWebhook(ctx context.Context, w *core.Webhook) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO webhooks(id,data) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET data=EXCLUDED.data`, w.ID, marshal(w))
	return err
}
func (p *Postgres) DeleteWebhook(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM webhooks WHERE id=$1`, id)
	return err
}

// --- audit
func (p *Postgres) AppendAudit(ctx context.Context, e *core.AuditEntry) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO audit(id,at,user_id,username,action,data) VALUES($1,$2,$3,$4,$5,$6)`, e.ID, e.At, e.UserID, e.Username, e.Action, marshal(e))
	return err
}
func (p *Postgres) ListAudit(ctx context.Context, f AuditFilter) ([]core.AuditEntry, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	return listDocs[core.AuditEntry](ctx, p, `SELECT data FROM audit WHERE ($1='' OR user_id=$1 OR username=$1) AND ($2='' OR action LIKE $2||'%') ORDER BY at DESC LIMIT $3`, f.UserID, f.Action, limit)
}

// --- telemetry
func (p *Postgres) AppendTelemetry(ctx context.Context, samples []core.TelemetrySample) error {
	if len(samples) == 0 {
		return nil
	}
	rows := make([][]any, 0, len(samples))
	for _, s := range samples {
		rows = append(rows, []any{s.VehicleID, s.TS, s.X, s.Y, s.Theta, s.BatteryCharge, s.Speed, s.MissionID})
	}
	_, err := p.pool.CopyFrom(ctx, pgx.Identifier{"telemetry"}, []string{"vehicle_id", "ts", "x", "y", "theta", "battery", "speed", "mission_id"}, pgx.CopyFromRows(rows))
	return err
}
func (p *Postgres) QueryTelemetry(ctx context.Context, vehicleID string, from, to time.Time, limit int) ([]core.TelemetrySample, error) {
	if to.IsZero() {
		to = time.Now().Add(time.Hour)
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}
	rows, err := p.pool.Query(ctx, `SELECT ts,x,y,theta,battery,speed,mission_id FROM telemetry WHERE vehicle_id=$1 AND ts BETWEEN $2 AND $3 ORDER BY ts`, vehicleID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]core.TelemetrySample, 0)
	for rows.Next() {
		var s core.TelemetrySample
		if err := rows.Scan(&s.TS, &s.X, &s.Y, &s.Theta, &s.BatteryCharge, &s.Speed, &s.MissionID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if limit > 0 && len(out) > limit {
		step := float64(len(out)) / float64(limit)
		ds := make([]core.TelemetrySample, 0, limit)
		for i := 0; i < limit; i++ {
			ds = append(ds, out[int(float64(i)*step)])
		}
		out = ds
	}
	return out, rows.Err()
}
func (p *Postgres) AppendMetric(ctx context.Context, siteID, metric string, ts time.Time, value float64) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO metrics(site_id,metric,ts,value) VALUES($1,$2,$3,$4)`, siteID, metric, ts, value)
	return err
}
func (p *Postgres) QueryMetric(ctx context.Context, siteID, metric string, from, to time.Time) ([]core.TimeseriesPoint, error) {
	if to.IsZero() {
		to = time.Now().Add(time.Hour)
	}
	if from.IsZero() {
		from = to.Add(-24 * time.Hour)
	}
	rows, err := p.pool.Query(ctx, `SELECT ts,value FROM metrics WHERE site_id=$1 AND metric=$2 AND ts BETWEEN $3 AND $4 ORDER BY ts`, siteID, metric, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]core.TimeseriesPoint, 0)
	for rows.Next() {
		var pt core.TimeseriesPoint
		if err := rows.Scan(&pt.TS, &pt.Value); err != nil {
			return nil, err
		}
		out = append(out, pt)
	}
	return out, rows.Err()
}
func (p *Postgres) GetSetting(ctx context.Context, key string, out any) (bool, error) {
	var raw []byte
	if err := p.pool.QueryRow(ctx, `SELECT data FROM settings WHERE key=$1`, key).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, json.Unmarshal(raw, out)
}
func (p *Postgres) SetSetting(ctx context.Context, key string, v any) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO settings(key,data) VALUES($1,$2) ON CONFLICT(key) DO UPDATE SET data=EXCLUDED.data`, key, marshal(v))
	return err
}
