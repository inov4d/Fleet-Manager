// Package auth provides users, password hashing, JWT sessions, API keys and
// role based authorization.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

// Permission names used by the API layer.
const (
	PermRead            = "read"
	PermMissionsWrite   = "missions.write"
	PermVehiclesCommand = "vehicles.command"
	PermVehiclesWrite   = "vehicles.write"
	PermMapsWrite       = "maps.write"
	PermAlarmsAck       = "alarms.ack"
	PermTrafficWrite    = "traffic.write"
	PermMaintenance     = "maintenance.write"
	PermSimulation      = "simulation"
	PermSettings        = "settings"
	PermUsers           = "users"
	PermIntegration     = "integration"
)

var rolePerms = map[core.Role]map[string]bool{
	core.RoleViewer:      set(PermRead),
	core.RoleOperator:    set(PermRead, PermMissionsWrite, PermVehiclesCommand, PermAlarmsAck),
	core.RoleSupervisor:  set(PermRead, PermMissionsWrite, PermVehiclesCommand, PermAlarmsAck, PermVehiclesWrite, PermMapsWrite, PermTrafficWrite, PermMaintenance, PermSimulation, PermSettings),
	core.RoleAdmin:       set(PermRead, PermMissionsWrite, PermVehiclesCommand, PermAlarmsAck, PermVehiclesWrite, PermMapsWrite, PermTrafficWrite, PermMaintenance, PermSimulation, PermSettings, PermUsers, PermIntegration),
	core.RoleIntegration: set(PermRead, PermMissionsWrite, PermIntegration),
}

func set(ps ...string) map[string]bool {
	m := map[string]bool{}
	for _, p := range ps {
		m[p] = true
	}
	return m
}

// Can reports whether a role has a permission.
func Can(role core.Role, perm string) bool { return rolePerms[role][perm] }

// ValidRole checks a role name.
func ValidRole(r core.Role) bool { _, ok := rolePerms[r]; return ok }

// Service handles authentication.
type Service struct {
	st     store.Store
	secret []byte
	ttl    time.Duration
	clock  core.Clock
}

// New creates the service and bootstraps the admin account when no user exists.
func New(st store.Store, secret string, ttl time.Duration, clock core.Clock, adminPassword string) (*Service, error) {
	if secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		secret = hex.EncodeToString(b)
	}
	s := &Service{st: st, secret: []byte(secret), ttl: ttl, clock: clock}
	users, err := st.ListUsers(context.Background())
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		if adminPassword == "" {
			adminPassword = "admin"
		}
		if _, err := s.CreateUser("admin", "Administrateur", adminPassword, core.RoleAdmin, nil); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// CreateUser adds a user.
func (s *Service) CreateUser(username, display, password string, role core.Role, siteIDs []string) (*core.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, core.Validation("username is required")
	}
	if len(password) < 4 {
		return nil, core.Validation("password too short")
	}
	if !ValidRole(role) {
		return nil, core.Validation("invalid role %q", role)
	}
	if _, err := s.st.GetUserByName(context.Background(), username); err == nil {
		return nil, core.Conflict("username %q already exists", username)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	if siteIDs == nil {
		siteIDs = []string{}
	}
	u := &core.User{ID: uuid.NewString(), Username: username, DisplayName: display, Role: role, SiteIDs: siteIDs, Active: true, CreatedAt: s.clock.Now(), PasswordHash: string(h)}
	if u.DisplayName == "" {
		u.DisplayName = username
	}
	return u, s.st.SaveUser(context.Background(), u)
}

// SetPassword changes a user's password.
func (s *Service) SetPassword(userID, password string) error {
	if len(password) < 4 {
		return core.Validation("password too short")
	}
	u, err := s.st.GetUser(context.Background(), userID)
	if err != nil {
		return err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(h)
	return s.st.SaveUser(context.Background(), u)
}

// Claims are the JWT claims.
type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Login verifies credentials and issues a token.
func (s *Service) Login(username, password string) (string, *core.User, time.Time, error) {
	u, err := s.st.GetUserByName(context.Background(), username)
	if err != nil || !u.Active {
		return "", nil, time.Time{}, core.Unauthorized("invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", nil, time.Time{}, core.Unauthorized("invalid credentials")
	}
	exp := s.clock.Now().Add(s.ttl)
	claims := Claims{Role: string(u.Role), RegisteredClaims: jwt.RegisteredClaims{Subject: u.ID, ExpiresAt: jwt.NewNumericDate(exp), IssuedAt: jwt.NewNumericDate(s.clock.Now())}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", nil, time.Time{}, err
	}
	return tok, u, exp, nil
}

// Verify parses a token and loads the user.
func (s *Service) Verify(token string) (*core.User, error) {
	var claims Claims
	t, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	}, jwt.WithTimeFunc(func() time.Time { return s.clock.Now() }))
	if err != nil || !t.Valid {
		return nil, core.Unauthorized("invalid token")
	}
	u, err := s.st.GetUser(context.Background(), claims.Subject)
	if err != nil || !u.Active {
		return nil, core.Unauthorized("unknown user")
	}
	return u, nil
}

// CreateAPIKey issues a key; the clear value is only returned once.
func (s *Service) CreateAPIKey(name string, role core.Role, source core.MissionSource, siteID string) (*core.APIKey, error) {
	if name == "" {
		return nil, core.Validation("name is required")
	}
	if role == "" {
		role = core.RoleIntegration
	}
	if !ValidRole(role) {
		return nil, core.Validation("invalid role")
	}
	if source == "" {
		source = core.SourceAPI
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	key := "fmk_" + hex.EncodeToString(b)
	k := &core.APIKey{ID: uuid.NewString(), Name: name, Role: role, Source: source, SiteID: siteID, Key: key, KeyHash: HashKey(key), CreatedAt: s.clock.Now()}
	if err := s.st.SaveAPIKey(context.Background(), k); err != nil {
		return nil, err
	}
	return k, nil
}

// HashKey hashes an API key for storage/lookup.
func HashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// VerifyAPIKey resolves a key to its record.
func (s *Service) VerifyAPIKey(key string) (*core.APIKey, error) {
	if key == "" {
		return nil, core.Unauthorized("missing api key")
	}
	h := HashKey(key)
	keys, err := s.st.ListAPIKeys(context.Background())
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].KeyHash == h {
			return &keys[i], nil
		}
	}
	return nil, core.Unauthorized("invalid api key")
}
