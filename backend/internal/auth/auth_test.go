package auth

import (
	"testing"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

func TestLoginAndTokens(t *testing.T) {
	st, _ := store.NewMemory("")
	clk := core.NewManualClock(time.Now())
	s, err := New(st, "secret", time.Hour, clk, "pw1234")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Login("admin", "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	tok, u, _, err := s.Login("ADMIN", "pw1234")
	if err != nil || u.Role != core.RoleAdmin {
		t.Fatal(err)
	}
	v, err := s.Verify(tok)
	if err != nil || v.ID != u.ID {
		t.Fatal("verify failed")
	}
	clk.Advance(2 * time.Hour)
	if _, err := s.Verify(tok); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err := s.CreateUser("admin", "", "xxxx", core.RoleViewer, nil); err == nil {
		t.Fatal("duplicate username")
	}
	k, err := s.CreateAPIKey("erp", core.RoleIntegration, core.SourceERP, "")
	if err != nil || k.Key == "" {
		t.Fatal(err)
	}
	got, err := s.VerifyAPIKey(k.Key)
	if err != nil || got.ID != k.ID || got.Source != core.SourceERP {
		t.Fatal("api key verify")
	}
	if _, err := s.VerifyAPIKey("nope"); err == nil {
		t.Fatal("bad key accepted")
	}
	if !Can(core.RoleOperator, PermMissionsWrite) || Can(core.RoleViewer, PermMissionsWrite) || Can(core.RoleSupervisor, PermUsers) {
		t.Fatal("rbac matrix")
	}
}
