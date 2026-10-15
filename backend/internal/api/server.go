// Package api exposes the REST API and the WebSocket stream described in
// docs/api.md.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/inov4d/fleet-manager/internal/audit"
	"github.com/inov4d/fleet-manager/internal/auth"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/fleet"
)

// Server serves the API.
type Server struct {
	core    *fleet.Core
	auth    *auth.Service
	audit   *audit.Logger
	log     *slog.Logger
	origins []string
	mux     *http.ServeMux
	hub     *hub
	info    SystemInfo
}

// SystemInfo is static deployment information.
type SystemInfo struct {
	Version   string `json:"version"`
	Store     string `json:"store"`
	MQTTEmbed bool   `json:"mqttEmbedded"`
	MQTTURL   string `json:"mqttUrl"`
}

// New builds the server.
func New(c *fleet.Core, a *auth.Service, al *audit.Logger, log *slog.Logger, origins []string, info SystemInfo) *Server {
	s := &Server{core: c, auth: a, audit: al, log: log, origins: origins, mux: http.NewServeMux(), info: info}
	s.hub = newHub(c, log)
	s.routes()
	return s
}

// Handler returns the root handler with middleware.
func (s *Server) Handler() http.Handler {
	return s.recoverMW(s.corsMW(s.logMW(s.mux)))
}

// Close stops the websocket hub.
func (s *Server) Close() { s.hub.close() }

// principal is the authenticated caller.
type principal struct {
	ID       string
	Username string
	Role     core.Role
	Source   core.MissionSource
	SiteID   string
	APIKey   bool
}

type ctxKey int

const principalKey ctxKey = 1

func principalFrom(r *http.Request) *principal {
	p, _ := r.Context().Value(principalKey).(*principal)
	return p
}

// --- helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, err error) {
	var ce *core.Error
	if errors.As(err, &ce) {
		status := http.StatusInternalServerError
		switch ce.Code {
		case core.CodeNotFound:
			status = http.StatusNotFound
		case core.CodeValidation:
			status = http.StatusBadRequest
		case core.CodeConflict:
			status = http.StatusConflict
		case core.CodeUnauthorized:
			status = http.StatusUnauthorized
		case core.CodeForbidden:
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": ce.Msg, "code": string(ce.Code)})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error(), "code": string(core.CodeInternal)})
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 16<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return core.Validation("invalid JSON body: %v", err)
	}
	return nil
}

func q(r *http.Request, key string) string { return strings.TrimSpace(r.URL.Query().Get(key)) }

func qInt(r *http.Request, key string, def int) int {
	if v, err := strconv.Atoi(q(r, key)); err == nil {
		return v
	}
	return def
}

func qTime(r *http.Request, key string) time.Time {
	if v := q(r, key); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// --- middleware

func (s *Server) recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic in handler", "path", r.URL.Path, "err", rec)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error", "code": "internal"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/ws" && r.URL.Path != "/health" {
			s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
		}
	})
}

func (s *Server) corsMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := ""
			for _, o := range s.origins {
				if o == "*" || strings.EqualFold(o, origin) {
					allowed = origin
					break
				}
			}
			if allowed != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowed)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authenticate resolves the caller from the bearer token, the token query
// parameter (websocket) or an API key.
func (s *Server) authenticate(r *http.Request) (*principal, error) {
	if key := r.Header.Get("X-API-Key"); key != "" {
		k, err := s.auth.VerifyAPIKey(key)
		if err != nil {
			return nil, err
		}
		return &principal{ID: "apikey:" + k.ID, Username: "apikey:" + k.Name, Role: k.Role, Source: k.Source, SiteID: k.SiteID, APIKey: true}, nil
	}
	tok := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		tok = strings.TrimSpace(h[7:])
	} else if t := q(r, "token"); t != "" {
		tok = t
	}
	if tok == "" {
		return nil, core.Unauthorized("authentication required")
	}
	u, err := s.auth.Verify(tok)
	if err != nil {
		return nil, err
	}
	return &principal{ID: u.ID, Username: u.Username, Role: u.Role}, nil
}

// guard wraps a handler with authentication and a permission check.
func (s *Server) guard(perm string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.authenticate(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if perm != "" && !auth.Can(p.Role, perm) {
			writeErr(w, core.Forbidden("role %s lacks permission %s", p.Role, perm))
			return
		}
		ctx := context.WithValue(r.Context(), principalKey, p)
		h(w, r.WithContext(ctx))
	}
}

func (s *Server) recordAudit(r *http.Request, action, target string, details any) {
	p := principalFrom(r)
	if p == nil {
		return
	}
	s.audit.Record(p.ID, p.Username, action, target, details, clientIP(r))
}
