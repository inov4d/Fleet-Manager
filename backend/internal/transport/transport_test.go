package transport

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		f, t string
		ok   bool
	}{
		{"a/b/c", "a/b/c", true}, {"a/+/c", "a/x/c", true}, {"a/#", "a/b/c/d", true}, {"a/+", "a/b/c", false},
		{"uagv/v2/+/+/state", "uagv/v2/m/s/state", true}, {"uagv/v2/+/+/state", "uagv/v2/m/s/order", false},
	}
	for _, c := range cases {
		if Match(c.f, c.t) != c.ok {
			t.Errorf("%s vs %s", c.f, c.t)
		}
	}
}

func TestMemoryRetainedAndWildcard(t *testing.T) {
	m := NewMemorySync()
	_ = m.Publish("x/1/connection", []byte("ONLINE"), true)
	var got []string
	unsub, _ := m.Subscribe("x/+/connection", func(topic string, p []byte) { got = append(got, topic+"="+string(p)) })
	if len(got) != 1 || got[0] != "x/1/connection=ONLINE" {
		t.Fatalf("retained replay missing: %v", got)
	}
	_ = m.Publish("x/2/connection", []byte("OFFLINE"), false)
	if len(got) != 2 {
		t.Fatal("wildcard delivery")
	}
	unsub()
	_ = m.Publish("x/3/connection", []byte("x"), false)
	if len(got) != 2 {
		t.Fatal("unsubscribe")
	}
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestEmbeddedBrokerWithPahoClients(t *testing.T) {
	port := freePort(t)
	b, err := StartBroker(BrokerOptions{Addr: fmt.Sprintf("127.0.0.1:%d", port), Users: map[string]string{"fleet": "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	url := fmt.Sprintf("tcp://127.0.0.1:%d", port)
	if _, err := NewMQTT(MQTTOptions{URL: url, ClientID: "bad", Username: "fleet", Password: "wrong", ConnectTimeout: 2 * time.Second}); err == nil {
		t.Fatal("wrong password must be rejected")
	}
	a, err := NewMQTT(MQTTOptions{URL: url, ClientID: "a", Username: "fleet", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	c, err := NewMQTT(MQTTOptions{URL: url, ClientID: "c", Username: "fleet", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var mu sync.Mutex
	var got []string
	done := make(chan struct{}, 4)
	_, err = c.Subscribe("uagv/v2/+/+/state", func(topic string, p []byte) {
		mu.Lock()
		got = append(got, topic)
		mu.Unlock()
		done <- struct{}{}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Publish("uagv/v2/m/s1/state", []byte("{}"), false); err != nil {
		t.Fatal(err)
	}
	if err := a.Publish("uagv/v2/m/s2/state", []byte("{}"), false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for messages")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}
