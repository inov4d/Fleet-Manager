// Package config reads the runtime configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the process configuration.
type Config struct {
	HTTPAddr      string
	StoreKind     string // memory | postgres
	DatabaseURL   string
	DataDir       string
	MQTTEmbedded  bool
	MQTTAddr      string
	MQTTWSAddr    string
	MQTTURL       string
	MQTTUsername  string
	MQTTPassword  string
	VDAInterface  string
	JWTSecret     string
	TokenTTL      time.Duration
	AdminPassword string
	CORSOrigins   []string
	LogLevel      string
	TickHz        int
	CommsTimeout  time.Duration
	SimAutostart  int
	SimTimeScale  float64
	Version       string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

// Load reads the environment.
func Load(version string) Config {
	c := Config{
		HTTPAddr:      env("FM_HTTP_ADDR", ":8080"),
		StoreKind:     strings.ToLower(env("FM_STORE", "memory")),
		DatabaseURL:   env("FM_DATABASE_URL", ""),
		DataDir:       env("FM_DATA_DIR", "./data"),
		MQTTEmbedded:  envBool("FM_MQTT_EMBEDDED", true),
		MQTTAddr:      env("FM_MQTT_ADDR", ":1883"),
		MQTTWSAddr:    env("FM_MQTT_WS_ADDR", ""),
		MQTTURL:       env("FM_MQTT_URL", ""),
		MQTTUsername:  env("FM_MQTT_USERNAME", ""),
		MQTTPassword:  env("FM_MQTT_PASSWORD", ""),
		VDAInterface:  env("FM_VDA_INTERFACE", "uagv"),
		JWTSecret:     env("FM_JWT_SECRET", ""),
		TokenTTL:      envDur("FM_TOKEN_TTL", 12*time.Hour),
		AdminPassword: env("FM_ADMIN_PASSWORD", "admin"),
		LogLevel:      strings.ToLower(env("FM_LOG_LEVEL", "info")),
		TickHz:        envInt("FM_TICK_HZ", 10),
		CommsTimeout:  envDur("FM_COMMS_TIMEOUT", 5*time.Second),
		SimAutostart:  envInt("FM_SIM_AUTOSTART", 0),
		SimTimeScale:  envFloat("FM_SIM_TIMESCALE", 1),
		Version:       version,
	}
	if o := env("FM_CORS_ORIGINS", "*"); o != "" {
		for _, s := range strings.Split(o, ",") {
			if s = strings.TrimSpace(s); s != "" {
				c.CORSOrigins = append(c.CORSOrigins, s)
			}
		}
	}
	if c.MQTTURL == "" {
		addr := c.MQTTAddr
		if strings.HasPrefix(addr, ":") {
			addr = "127.0.0.1" + addr
		}
		c.MQTTURL = "tcp://" + addr
	}
	if c.StoreKind == "postgres" && c.DatabaseURL == "" {
		c.DatabaseURL = "postgres://fleet:fleet@localhost:5432/fleet?sslmode=disable"
	}
	return c
}
