// Command agvagent is the open-source on-board agent of the ant-colony mode.
// It runs on the AGV (next to the VDA 5050 controller), connects to the site
// broker, watches the shared task board and claims the most opportune task
// using only local knowledge: its own position, battery, load and a route
// estimate over the shared map. Any other implementation that speaks the
// board protocol (docs/colony.md) can replace it.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/inov4d/fleet-manager/internal/colony"
	"github.com/inov4d/fleet-manager/internal/transport"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envF(k string, d float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(k), 64); err == nil {
		return v
	}
	return d
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := colony.DefaultAgentConfig()
	cfg.SiteID = env("AGV_SITE", "site-default")
	cfg.VehicleID = env("AGV_VEHICLE_ID", "")
	cfg.Manufacturer = env("AGV_MANUFACTURER", "inov4d")
	cfg.Serial = env("AGV_SERIAL", "")
	cfg.Interface = env("AGV_VDA_INTERFACE", "uagv")
	cfg.LowBatteryPct = envF("AGV_LOW_BATTERY", cfg.LowBatteryPct)
	cfg.CriticalBatteryPct = envF("AGV_CRITICAL_BATTERY", cfg.CriticalBatteryPct)
	cfg.ClaimScale = envF("AGV_CLAIM_SCALE", cfg.ClaimScale)
	if cfg.VehicleID == "" || cfg.Serial == "" {
		fmt.Fprintln(os.Stderr, "AGV_VEHICLE_ID and AGV_SERIAL are required")
		os.Exit(2)
	}
	tr, err := transport.NewMQTT(transport.MQTTOptions{URL: env("AGV_MQTT_URL", "tcp://127.0.0.1:1883"), ClientID: "agvagent-" + cfg.VehicleID,
		Username: env("AGV_MQTT_USERNAME", ""), Password: env("AGV_MQTT_PASSWORD", ""),
		OnLost: func(err error) { log.Warn("mqtt lost", "err", err) }, OnConnect: func() { log.Info("mqtt connected") }})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mqtt:", err)
		os.Exit(1)
	}
	defer tr.Close()
	ag := colony.NewAgent(cfg, tr, log)
	if err := ag.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "agent:", err)
		os.Exit(1)
	}
	defer ag.Stop()
	log.Info("agvagent running", "vehicle", cfg.VehicleID, "site", cfg.SiteID)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-sig:
			return
		case <-t.C:
			s := ag.Snapshot()
			log.Info("status", "battery", s.Battery, "node", s.Node, "assignment", s.Assignment.Status, "claims", s.Claims, "wins", s.Wins)
		}
	}
}
