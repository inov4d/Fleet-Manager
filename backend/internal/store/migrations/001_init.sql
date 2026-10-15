-- Fleet Manager schema. Entities are stored as JSONB documents with indexed
-- key columns; history tables are append-only.
CREATE TABLE IF NOT EXISTS sites (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE IF NOT EXISTS vehicle_models (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE IF NOT EXISTS vehicles (id TEXT PRIMARY KEY, site_id TEXT NOT NULL, data JSONB NOT NULL);
CREATE INDEX IF NOT EXISTS vehicles_site ON vehicles(site_id);
CREATE TABLE IF NOT EXISTS maps (id TEXT PRIMARY KEY, site_id TEXT NOT NULL, data JSONB NOT NULL);
CREATE INDEX IF NOT EXISTS maps_site ON maps(site_id);
CREATE TABLE IF NOT EXISTS missions (
  id TEXT PRIMARY KEY, site_id TEXT NOT NULL, status TEXT NOT NULL, vehicle_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL, data JSONB NOT NULL);
CREATE INDEX IF NOT EXISTS missions_site_created ON missions(site_id, created_at DESC);
CREATE INDEX IF NOT EXISTS missions_status ON missions(status);
CREATE TABLE IF NOT EXISTS alarms (id TEXT PRIMARY KEY, site_id TEXT NOT NULL, active BOOLEAN NOT NULL, raised_at TIMESTAMPTZ NOT NULL, data JSONB NOT NULL);
CREATE INDEX IF NOT EXISTS alarms_site_active ON alarms(site_id, active, raised_at DESC);
CREATE TABLE IF NOT EXISTS maintenance_plans (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE IF NOT EXISTS maintenance_records (id TEXT PRIMARY KEY, vehicle_id TEXT NOT NULL, done_at TIMESTAMPTZ NOT NULL, data JSONB NOT NULL);
CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, data JSONB NOT NULL);
CREATE TABLE IF NOT EXISTS api_keys (id TEXT PRIMARY KEY, key_hash TEXT NOT NULL, data JSONB NOT NULL);
CREATE TABLE IF NOT EXISTS webhooks (id TEXT PRIMARY KEY, data JSONB NOT NULL);
CREATE TABLE IF NOT EXISTS audit (id TEXT PRIMARY KEY, at TIMESTAMPTZ NOT NULL, user_id TEXT NOT NULL, username TEXT NOT NULL, action TEXT NOT NULL, data JSONB NOT NULL);
CREATE INDEX IF NOT EXISTS audit_at ON audit(at DESC);
CREATE TABLE IF NOT EXISTS telemetry (
  vehicle_id TEXT NOT NULL, ts TIMESTAMPTZ NOT NULL, x DOUBLE PRECISION, y DOUBLE PRECISION, theta DOUBLE PRECISION,
  battery DOUBLE PRECISION, speed DOUBLE PRECISION, mission_id TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS telemetry_vehicle_ts ON telemetry(vehicle_id, ts DESC);
CREATE TABLE IF NOT EXISTS metrics (site_id TEXT NOT NULL, metric TEXT NOT NULL, ts TIMESTAMPTZ NOT NULL, value DOUBLE PRECISION NOT NULL);
CREATE INDEX IF NOT EXISTS metrics_key_ts ON metrics(site_id, metric, ts DESC);
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, data JSONB NOT NULL);
-- TimescaleDB (optional): when the extension is installed, telemetry and metrics become hypertables.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
    PERFORM create_hypertable('telemetry', 'ts', if_not_exists => TRUE);
    PERFORM create_hypertable('metrics', 'ts', if_not_exists => TRUE);
  END IF;
END $$;
