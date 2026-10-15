package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
)

func exercise(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	if err := s.SaveSite(ctx, &core.Site{ID: "s1", Name: "Usine", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveVehicle(ctx, &core.Vehicle{ID: "v1", SiteID: "s1", Name: "AGV-1", ModelID: "AGV20T", Enabled: true, Tags: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	vs, _ := s.ListVehicles(ctx, "s1")
	if len(vs) != 1 || vs[0].Name != "AGV-1" {
		t.Fatalf("vehicles %v", vs)
	}
	if _, err := s.GetVehicle(ctx, "nope"); err == nil {
		t.Fatal("expected not found")
	}
	m := &core.Mission{ID: "m1", SiteID: "s1", Status: core.MissionPending, CreatedAt: time.Now(), Steps: []core.MissionStep{{Seq: 0, NodeID: "a", Action: core.StepMove}}}
	if err := s.SaveMission(ctx, m); err != nil {
		t.Fatal(err)
	}
	m.Status = core.MissionCompleted
	_ = s.SaveMission(ctx, m)
	ms, _ := s.ListMissions(ctx, MissionFilter{SiteID: "s1", ActiveOnly: true})
	if len(ms) != 0 {
		t.Fatal("completed mission should not be active")
	}
	ms, _ = s.ListMissions(ctx, MissionFilter{Status: "completed"})
	if len(ms) != 1 || len(ms[0].Steps) != 1 {
		t.Fatalf("missions %v", ms)
	}
	u := &core.User{ID: "u1", Username: "Admin", Role: core.RoleAdmin, PasswordHash: "h", Active: true}
	_ = s.SaveUser(ctx, u)
	got, err := s.GetUserByName(ctx, "admin")
	if err != nil || got.PasswordHash != "h" {
		t.Fatalf("user lookup %v %v", got, err)
	}
	_ = s.AppendAudit(ctx, &core.AuditEntry{ID: "a1", At: time.Now(), UserID: "u1", Username: "admin", Action: "mission.create"})
	al, _ := s.ListAudit(ctx, AuditFilter{Action: "mission"})
	if len(al) != 1 {
		t.Fatal("audit")
	}
	now := time.Now()
	_ = s.AppendTelemetry(ctx, []core.TelemetrySample{{VehicleID: "v1", TS: now, X: 1, BatteryCharge: 50}, {VehicleID: "v1", TS: now.Add(time.Second), X: 2, BatteryCharge: 49}})
	ts, _ := s.QueryTelemetry(ctx, "v1", now.Add(-time.Minute), now.Add(time.Minute), 10)
	if len(ts) != 2 {
		t.Fatalf("telemetry %v", ts)
	}
	_ = s.AppendMetric(ctx, "s1", "utilization", now, 42)
	pts, _ := s.QueryMetric(ctx, "s1", "utilization", now.Add(-time.Minute), now.Add(time.Minute))
	if len(pts) != 1 || pts[0].Value != 42 {
		t.Fatal("metrics")
	}
	_ = s.SetSetting(ctx, "dispatcher", core.DefaultDispatcherSettings())
	var ds core.DispatcherSettings
	ok, err := s.GetSetting(ctx, "dispatcher", &ds)
	if err != nil || !ok || ds.MinBatteryPct != 20 {
		t.Fatal("settings")
	}
	_ = s.SaveMap(ctx, &core.Map{ID: "map1", SiteID: "s1", Name: "m", Nodes: []core.Node{{ID: "a"}}})
	mp, err := s.GetMap(ctx, "map1")
	if err != nil || len(mp.Nodes) != 1 {
		t.Fatal("map")
	}
	_ = s.SaveAlarm(ctx, &core.Alarm{ID: "al1", SiteID: "s1", Active: true, RaisedAt: now, Severity: core.SevWarning})
	as, _ := s.ListAlarms(ctx, "s1", true, 10)
	if len(as) != 1 {
		t.Fatal("alarms")
	}
}

func TestMemoryStoreWithSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fm.json")
	s, err := NewMemory(path)
	if err != nil {
		t.Fatal(err)
	}
	exercise(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("snapshot not written")
	}
	s2, err := NewMemory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	v, err := s2.GetVehicle(context.Background(), "v1")
	if err != nil || v.Name != "AGV-1" {
		t.Fatal("snapshot reload failed")
	}
	u, _ := s2.GetUserByName(context.Background(), "admin")
	if u == nil || u.PasswordHash != "h" {
		t.Fatal("password hash must survive reload")
	}
}

func TestPostgresStore(t *testing.T) {
	dsn := os.Getenv("FM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("FM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, tbl := range []string{"sites", "vehicles", "maps", "missions", "alarms", "users", "audit", "telemetry", "metrics", "settings"} {
		if _, err := s.pool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatal(err)
		}
	}
	exercise(t, s)
}
