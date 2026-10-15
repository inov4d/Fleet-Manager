package transport

import (
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTOptions configures the Paho client.
type MQTTOptions struct {
	URL            string // tcp://host:1883 or ssl://host:8883
	ClientID       string
	Username       string
	Password       string
	TLS            *tls.Config
	WillTopic      string
	WillPayload    []byte
	OnConnect      func()
	OnLost         func(error)
	ConnectTimeout time.Duration // default 10s
}

// MQTT is a Transport over an MQTT broker.
type MQTT struct {
	c    mqtt.Client
	mu   sync.Mutex
	subs map[string][]Handler
}

// NewMQTT connects (with automatic reconnection) to a broker.
func NewMQTT(o MQTTOptions) (*MQTT, error) {
	t := &MQTT{subs: map[string][]Handler{}}
	opts := mqtt.NewClientOptions().AddBroker(o.URL).SetClientID(o.ClientID).
		SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(2 * time.Second).
		SetKeepAlive(10 * time.Second).SetCleanSession(true).SetOrderMatters(false).
		SetConnectTimeout(5 * time.Second)
	if o.Username != "" {
		opts.SetUsername(o.Username).SetPassword(o.Password)
	}
	if o.TLS != nil {
		opts.SetTLSConfig(o.TLS)
	}
	if o.WillTopic != "" {
		opts.SetBinaryWill(o.WillTopic, o.WillPayload, 1, true)
	}
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		// re-subscribe after reconnection
		t.mu.Lock()
		filters := make([]string, 0, len(t.subs))
		for f := range t.subs {
			filters = append(filters, f)
		}
		t.mu.Unlock()
		for _, f := range filters {
			c.Subscribe(f, 1, t.dispatch)
		}
		if o.OnConnect != nil {
			o.OnConnect()
		}
	})
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		if o.OnLost != nil {
			o.OnLost(err)
		}
	})
	t.c = mqtt.NewClient(opts)
	tok := t.c.Connect()
	timeout := o.ConnectTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if !tok.WaitTimeout(timeout) {
		t.c.Disconnect(0)
		return nil, fmt.Errorf("mqtt connect timeout to %s", o.URL)
	}
	if err := tok.Error(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *MQTT) dispatch(_ mqtt.Client, m mqtt.Message) {
	t.mu.Lock()
	var hs []Handler
	for f, list := range t.subs {
		if Match(f, m.Topic()) {
			hs = append(hs, list...)
		}
	}
	t.mu.Unlock()
	for _, h := range hs {
		h(m.Topic(), m.Payload())
	}
}

func (t *MQTT) Publish(topic string, payload []byte, retain bool) error {
	tok := t.c.Publish(topic, 1, retain, payload)
	if !tok.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("publish timeout on %s", topic)
	}
	return tok.Error()
}

func (t *MQTT) Subscribe(filter string, h Handler) (func(), error) {
	t.mu.Lock()
	first := len(t.subs[filter]) == 0
	t.subs[filter] = append(t.subs[filter], h)
	t.mu.Unlock()
	if first {
		tok := t.c.Subscribe(filter, 1, t.dispatch)
		if !tok.WaitTimeout(5 * time.Second) {
			return nil, fmt.Errorf("subscribe timeout on %s", filter)
		}
		if err := tok.Error(); err != nil {
			return nil, err
		}
	}
	return func() {
		t.mu.Lock()
		list := t.subs[filter]
		for i := range list {
			if fmt.Sprintf("%p", list[i]) == fmt.Sprintf("%p", h) {
				list = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(list) == 0 {
			delete(t.subs, filter)
			t.c.Unsubscribe(filter)
		} else {
			t.subs[filter] = list
		}
		t.mu.Unlock()
	}, nil
}

func (t *MQTT) Close() error {
	t.c.Disconnect(250)
	return nil
}

// Connected reports the client connection state.
func (t *MQTT) Connected() bool { return t.c.IsConnectionOpen() }
