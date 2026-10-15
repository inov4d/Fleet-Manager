// Package audit records who did what (every mutating API call and important
// system decisions).
package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

// Logger writes audit entries.
type Logger struct {
	st    store.Store
	clock core.Clock
	log   *slog.Logger
}

func New(st store.Store, clock core.Clock, log *slog.Logger) *Logger {
	if log == nil {
		log = slog.Default()
	}
	return &Logger{st: st, clock: clock, log: log}
}

// Record appends an entry. details may be any JSON-serialisable value.
func (l *Logger) Record(userID, username, action, target string, details any, ip string) {
	d := ""
	if details != nil {
		if s, ok := details.(string); ok {
			d = s
		} else if b, err := json.Marshal(details); err == nil {
			d = string(b)
		}
	}
	if len(d) > 2000 {
		d = d[:2000]
	}
	e := &core.AuditEntry{ID: uuid.NewString(), At: l.clock.Now(), UserID: userID, Username: username, Action: action, Target: target, Details: d, IP: ip}
	if err := l.st.AppendAudit(context.Background(), e); err != nil {
		l.log.Warn("audit append failed", "err", err)
	}
	l.log.Info("audit", "user", username, "action", action, "target", target)
}

// List returns entries.
func (l *Logger) List(f store.AuditFilter) []core.AuditEntry {
	out, _ := l.st.ListAudit(context.Background(), f)
	return out
}
