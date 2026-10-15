// Package integration exposes the fleet to industrial systems: MQTT topics for
// mission requests and fleet events, and the OPC UA adapter contract.
// REST (API keys) and webhooks live in the API / core packages.
package integration

import (
	"encoding/json"
	"log/slog"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/fleet"
	"github.com/inov4d/fleet-manager/internal/transport"
)

// MQTTBridge publishes fleet events on fleet/<siteId>/events and accepts
// mission requests on fleet/<siteId>/missions/request.
type MQTTBridge struct {
	tr    transport.Transport
	core  *fleet.Core
	log   *slog.Logger
	unsub []func()
}

// NewMQTTBridge wires the bridge.
func NewMQTTBridge(tr transport.Transport, c *fleet.Core, log *slog.Logger) *MQTTBridge {
	return &MQTTBridge{tr: tr, core: c, log: log}
}

// Start subscribes and starts publishing.
func (b *MQTTBridge) Start() error {
	u, err := b.tr.Subscribe("fleet/+/missions/request", b.onMissionRequest)
	if err != nil {
		return err
	}
	b.unsub = append(b.unsub, u)
	b.unsub = append(b.unsub, b.core.Subscribe(func(e core.Event) {
		site := e.SiteID
		if site == "" {
			site = core.DefaultSiteID
		}
		payload, _ := json.Marshal(e)
		_ = b.tr.Publish("fleet/"+site+"/events", payload, false)
	}))
	return nil
}

// Stop unsubscribes.
func (b *MQTTBridge) Stop() {
	for _, u := range b.unsub {
		u()
	}
}

func (b *MQTTBridge) onMissionRequest(topic string, payload []byte) {
	var m core.Mission
	if err := json.Unmarshal(payload, &m); err != nil {
		b.log.Warn("invalid mission request on mqtt", "topic", topic, "err", err)
		return
	}
	// site from the topic
	site := core.DefaultSiteID
	parts := splitTopic(topic)
	if len(parts) == 4 {
		site = parts[1]
	}
	m.SiteID = site
	if m.Source == "" {
		m.Source = core.SourcePLC
	}
	m.CreatedBy = "mqtt"
	if m.ExternalRef != "" {
		if ex, ok := b.core.Missions().ByExternalRef(m.ExternalRef); ok && !ex.Status.Terminal() {
			return
		}
	}
	if _, err := b.core.CreateMission(m); err != nil {
		b.log.Warn("mqtt mission request rejected", "err", err)
		resp, _ := json.Marshal(map[string]string{"externalRef": m.ExternalRef, "error": err.Error()})
		_ = b.tr.Publish("fleet/"+site+"/missions/rejected", resp, false)
	}
}

func splitTopic(t string) []string {
	var out []string
	cur := ""
	for _, ch := range t {
		if ch == '/' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(ch)
	}
	return append(out, cur)
}
