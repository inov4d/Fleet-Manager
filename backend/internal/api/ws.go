package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/fleet"
)

// hub streams snapshots and events to websocket clients.
type hub struct {
	core    *fleet.Core
	log     *slog.Logger
	mu      sync.Mutex
	clients map[*client]bool
	unsub   func()
}

type client struct {
	conn   *websocket.Conn
	siteID string
	events chan core.Event
	mu     sync.Mutex
}

func newHub(c *fleet.Core, log *slog.Logger) *hub {
	h := &hub{core: c, log: log, clients: map[*client]bool{}}
	h.unsub = c.Subscribe(h.broadcast)
	return h
}

func (h *hub) close() {
	if h.unsub != nil {
		h.unsub()
	}
}

func (h *hub) broadcast(e core.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		c.mu.Lock()
		site := c.siteID
		c.mu.Unlock()
		if site != "" && e.SiteID != "" && e.SiteID != site {
			continue
		}
		select {
		case c.events <- e:
		default:
		}
	}
}

type wsMessage struct {
	Type  string          `json:"type"`
	TS    time.Time       `json:"ts"`
	Data  any             `json:"data,omitempty"`
	Event *core.Event     `json:"event,omitempty"`
	Raw   json.RawMessage `json:"-"`
}

func (h *hub) serve(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.authenticate(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			return
		}
		cl := &client{conn: conn, siteID: q(r, "siteId"), events: make(chan core.Event, 256)}
		h.mu.Lock()
		h.clients[cl] = true
		h.mu.Unlock()
		defer func() {
			h.mu.Lock()
			delete(h.clients, cl)
			h.mu.Unlock()
			conn.Close(websocket.StatusNormalClosure, "bye")
		}()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		// reader: subscribe messages
		go func() {
			defer cancel()
			for {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var m struct {
					Type   string `json:"type"`
					SiteID string `json:"siteId"`
				}
				if json.Unmarshal(data, &m) == nil && m.Type == "subscribe" {
					cl.mu.Lock()
					cl.siteID = m.SiteID
					cl.mu.Unlock()
				}
			}
		}()
		h.log.Debug("ws client connected", "user", p.Username)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		send := func(v any) error {
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			wctx, c2 := context.WithTimeout(ctx, 5*time.Second)
			defer c2()
			return conn.Write(wctx, websocket.MessageText, b)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-cl.events:
				ev := e
				if err := send(wsMessage{Type: "event", TS: time.Now().UTC(), Event: &ev}); err != nil {
					return
				}
			case <-ticker.C:
				cl.mu.Lock()
				site := cl.siteID
				cl.mu.Unlock()
				snap := h.core.Snapshot(site)
				if err := send(wsMessage{Type: "snapshot", TS: time.Now().UTC(), Data: snap}); err != nil {
					return
				}
			}
		}
	}
}
