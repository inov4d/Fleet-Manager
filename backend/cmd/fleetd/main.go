// Command fleetd runs the Fleet Manager: REST API, WebSocket stream, embedded
// MQTT broker (optional), VDA 5050 gateway, control loop and simulation.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/inov4d/fleet-manager/internal/api"
	"github.com/inov4d/fleet-manager/internal/audit"
	"github.com/inov4d/fleet-manager/internal/auth"
	"github.com/inov4d/fleet-manager/internal/config"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/fleet"
	"github.com/inov4d/fleet-manager/internal/integration"
	"github.com/inov4d/fleet-manager/internal/store"
	"github.com/inov4d/fleet-manager/internal/transport"
)

var version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fleetd:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load(version)
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)
	log.Info("fleet manager starting", "version", version, "store", cfg.StoreKind, "http", cfg.HTTPAddr)

	// store
	var st store.Store
	var err error
	switch cfg.StoreKind {
	case "postgres":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		st, err = store.NewPostgres(ctx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
	default:
		if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
			return err
		}
		st, err = store.NewMemory(filepath.Join(cfg.DataDir, "fleet.json"))
		if err != nil {
			return fmt.Errorf("memory store: %w", err)
		}
	}
	defer st.Close()

	// persistent JWT secret when none is configured
	secret := cfg.JWTSecret
	if secret == "" {
		var stored string
		if ok, _ := st.GetSetting(context.Background(), "jwtSecret", &stored); ok && stored != "" {
			secret = stored
		} else {
			secret = randomHex(32)
			_ = st.SetSetting(context.Background(), "jwtSecret", secret)
		}
	}

	// transport: embedded broker + client, external broker, or in-memory
	var tr transport.Transport
	var broker *transport.Broker
	if cfg.MQTTEmbedded {
		users := map[string]string{}
		if cfg.MQTTUsername != "" {
			users[cfg.MQTTUsername] = cfg.MQTTPassword
		}
		broker, err = transport.StartBroker(transport.BrokerOptions{Addr: cfg.MQTTAddr, WSAddr: cfg.MQTTWSAddr, Users: users, AllowAll: len(users) == 0, Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))})
		if err != nil {
			return fmt.Errorf("embedded mqtt broker: %w", err)
		}
		defer broker.Close()
		log.Info("embedded MQTT broker listening", "addr", cfg.MQTTAddr)
	}
	if strings.EqualFold(cfg.MQTTURL, "memory") {
		tr = transport.NewMemory()
	} else {
		m, err := transport.NewMQTT(transport.MQTTOptions{URL: cfg.MQTTURL, ClientID: "fleet-manager-" + randomHex(4), Username: cfg.MQTTUsername, Password: cfg.MQTTPassword,
			OnLost: func(err error) { log.Warn("mqtt connection lost", "err", err) }, OnConnect: func() { log.Info("mqtt connected", "url", cfg.MQTTURL) }})
		if err != nil {
			return fmt.Errorf("mqtt connect %s: %w", cfg.MQTTURL, err)
		}
		defer m.Close()
		tr = m
	}

	clock := core.NewScaledClock(1)
	fcfg := fleet.DefaultConfig()
	fcfg.TickHz = cfg.TickHz
	fcfg.VDAInterface = cfg.VDAInterface
	fcfg.CommsTimeout = cfg.CommsTimeout
	fcfg.Version = version
	fc, err := fleet.New(fcfg, fleet.Deps{Store: st, Transport: tr, Clock: clock, Logger: log})
	if err != nil {
		return fmt.Errorf("fleet core: %w", err)
	}
	authSvc, err := auth.New(st, secret, cfg.TokenTTL, clock, cfg.AdminPassword)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	auditLog := audit.New(st, clock, log)
	if err := fc.Start(); err != nil {
		return err
	}
	defer fc.Stop()

	bridge := integration.NewMQTTBridge(tr, fc, log)
	if err := bridge.Start(); err != nil {
		return err
	}
	defer bridge.Stop()

	if cfg.SimAutostart > 0 {
		if _, err := fc.StartSimulation(fleet.SimulationStart{Vehicles: cfg.SimAutostart, TimeScale: cfg.SimTimeScale, Scenario: &fleet.SimulationScenario{Name: "randomTransport", MissionRatePerMin: 2}}); err != nil {
			log.Warn("simulation autostart failed", "err", err)
		} else {
			log.Info("simulation started", "vehicles", cfg.SimAutostart, "timeScale", cfg.SimTimeScale)
		}
	}

	srv := api.New(fc, authSvc, auditLog, log, cfg.CORSOrigins, api.SystemInfo{Version: version, Store: st.Kind(), MQTTEmbed: cfg.MQTTEmbedded, MQTTURL: cfg.MQTTURL})
	defer srv.Close()
	httpSrv := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		log.Info("http api listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case s := <-sig:
		log.Info("shutting down", "signal", s.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(ctx)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
