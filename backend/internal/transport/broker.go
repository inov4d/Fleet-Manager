package transport

import (
	"fmt"
	"log/slog"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

// BrokerOptions configures the embedded MQTT broker.
type BrokerOptions struct {
	Addr     string // ":1883"
	WSAddr   string // optional websocket listener, e.g. ":9001"
	Users    map[string]string
	AllowAll bool
	Logger   *slog.Logger
}

// Broker wraps the embedded Mochi MQTT server.
type Broker struct {
	srv *mochi.Server
}

// StartBroker starts an embedded broker. When no users are configured and
// AllowAll is false, only the configured users may connect; with AllowAll the
// broker accepts any client (development / air-gapped single machine).
func StartBroker(o BrokerOptions) (*Broker, error) {
	opts := &mochi.Options{InlineClient: true}
	if o.Logger != nil {
		opts.Logger = o.Logger
	}
	srv := mochi.New(opts)
	if o.AllowAll || len(o.Users) == 0 {
		if err := srv.AddHook(new(auth.AllowHook), nil); err != nil {
			return nil, err
		}
	} else {
		ledger := &auth.Ledger{}
		for u, p := range o.Users {
			ledger.Auth = append(ledger.Auth, auth.AuthRule{Username: auth.RString(u), Password: auth.RString(p), Allow: true})
		}
		ledger.ACL = append(ledger.ACL, auth.ACLRule{Filters: map[auth.RString]auth.Access{"#": auth.ReadWrite}})
		if err := srv.AddHook(new(auth.Hook), &auth.Options{Ledger: ledger}); err != nil {
			return nil, err
		}
	}
	if o.Addr == "" {
		o.Addr = ":1883"
	}
	tcp := listeners.NewTCP(listeners.Config{ID: "tcp", Address: o.Addr})
	if err := srv.AddListener(tcp); err != nil {
		return nil, fmt.Errorf("mqtt listener %s: %w", o.Addr, err)
	}
	if o.WSAddr != "" {
		ws := listeners.NewWebsocket(listeners.Config{ID: "ws", Address: o.WSAddr})
		if err := srv.AddListener(ws); err != nil {
			return nil, fmt.Errorf("mqtt ws listener %s: %w", o.WSAddr, err)
		}
	}
	if err := srv.Serve(); err != nil {
		return nil, err
	}
	return &Broker{srv: srv}, nil
}

// Close stops the broker.
func (b *Broker) Close() error { return b.srv.Close() }
