package fleet

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
)

// Snapshot is the WebSocket payload.
type Snapshot struct {
	Vehicles     []core.VehicleWithState `json:"vehicles"`
	Missions     []core.Mission          `json:"missions"`
	Reservations []core.Reservation      `json:"reservations"`
	Alarms       []core.Alarm            `json:"alarms"`
	KPIs         core.KPIs               `json:"kpis"`
	Simulation   SimulationStatus        `json:"simulation"`
	Blocked      []core.BlockedVehicle   `json:"blocked"`
}

// Snapshot builds the live state of a site (empty = all sites).
func (c *Core) Snapshot(siteID string) Snapshot {
	c.mu.RLock()
	vs := make([]core.VehicleWithState, 0, len(c.vehicles))
	for id, v := range c.vehicles {
		if siteID == "" || v.SiteID == siteID {
			vs = append(vs, core.VehicleWithState{Vehicle: *v, State: c.rt[id].state})
		}
	}
	simSt := c.simStatus()
	c.mu.RUnlock()
	in := c.analyticsInputUnlocked(siteID, vs)
	k := c.analytics.KPIs(in)
	k.Vehicles = nil
	return Snapshot{Vehicles: vs, Missions: c.missions.List(siteID, "", "", 0, true), Reservations: c.traffic.Reservations(siteID), Alarms: c.alarms.Active(siteID),
		KPIs: k, Simulation: simSt, Blocked: c.traffic.Blocked()}
}

func (c *Core) analyticsInputUnlocked(siteID string, vs []core.VehicleWithState) analyticsInput {
	return analyticsInput{SiteID: siteID, Vehicles: vs, ActiveAlarms: len(c.alarms.Active(siteID)), Deadlocks: c.traffic.ResolvedCount(), AlarmsByVehicle: c.alarms.ActiveForVehicle}
}

// KPIs computes analytics for a window.
func (c *Core) KPIs(siteID string, from, to time.Time) core.KPIs {
	vs := c.Vehicles(siteID)
	in := c.analyticsInputUnlocked(siteID, vs)
	in.From, in.To = from, to
	return c.analytics.KPIs(in)
}

// Timeseries returns stored metric points.
func (c *Core) Timeseries(siteID, metric string, from, to time.Time) ([]core.TimeseriesPoint, error) {
	return c.st.QueryMetric(context.Background(), siteID, metric, from, to)
}

// deliverWebhooks posts events to configured webhooks (asynchronously).
func (c *Core) deliverWebhooks(e core.Event) {
	c.whMu.RLock()
	hooks := append([]core.Webhook{}, c.webhooks...)
	c.whMu.RUnlock()
	if len(hooks) == 0 {
		return
	}
	body, _ := json.Marshal(e)
	for _, h := range hooks {
		match := len(h.Events) == 0
		for _, k := range h.Events {
			if k == e.Kind {
				match = true
			}
		}
		if !match {
			continue
		}
		go func(h core.Webhook) {
			req, err := http.NewRequest(http.MethodPost, h.URL, bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			if h.Secret != "" {
				mac := hmac.New(sha256.New, []byte(h.Secret))
				mac.Write(body)
				req.Header.Set("X-Fleet-Signature", hex.EncodeToString(mac.Sum(nil)))
			}
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				c.log.Warn("webhook delivery failed", "url", h.URL, "err", err)
				return
			}
			resp.Body.Close()
		}(h)
	}
}
