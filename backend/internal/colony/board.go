package colony

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/transport"
)

// Arbiter is what the board needs from the Fleet Core.
type Arbiter interface {
	OpenTasks(siteID string) []Task
	// Eligible is the central safety net (site, model, payload, availability).
	Eligible(siteID, vehicleID, taskID string) (reason string, ok bool)
	// Grant hands the task to the vehicle, keeping the competing claims as the
	// assignment explanation.
	Grant(siteID, taskID, vehicleID string, claims []Claim) error
	RequestSystemTask(siteID, vehicleID, kind, reason string) error
	Release(siteID, vehicleID, taskID, reason string) error
	Assignments(siteID string) []Assignment
	ActiveMap(siteID string) (*core.Map, bool)
	VehicleInfos(siteID string) []VehicleInfo
}

// BoardServer publishes the board and arbitrates claims.
type BoardServer struct {
	tr     transport.Transport
	arb    Arbiter
	log    *slog.Logger
	window time.Duration

	mu       sync.Mutex
	sites    map[string]bool
	windows  map[string]*claimWindow // task id -> open claim window
	hashes   map[string]string       // topic -> last published hash
	unsubs   []func()
	stop     chan struct{}
	history  []Grant
	granted  int
	rejected int
}

type claimWindow struct {
	siteID string
	claims []Claim
	timer  *time.Timer
}

// Stats are the board counters.
type Stats struct {
	Granted  int     `json:"granted"`
	Rejected int     `json:"rejected"`
	Recent   []Grant `json:"recent"`
}

// NewBoardServer creates the server. window is the wall-clock time during
// which competing claims for the same task are collected before the lowest
// cost wins (ants do not need a central score: the board only breaks ties).
func NewBoardServer(tr transport.Transport, arb Arbiter, log *slog.Logger, window time.Duration) *BoardServer {
	if log == nil {
		log = slog.Default()
	}
	if window <= 0 {
		window = 300 * time.Millisecond
	}
	return &BoardServer{tr: tr, arb: arb, log: log, window: window, sites: map[string]bool{}, windows: map[string]*claimWindow{}, hashes: map[string]string{}}
}

// Start subscribes to the agent topics and starts publishing.
func (b *BoardServer) Start(sites []string, publishEvery time.Duration) error {
	for _, s := range sites {
		b.sites[s] = true
	}
	for topic, h := range map[string]transport.Handler{
		"fleet/+/board/claims":   b.onClaim,
		"fleet/+/board/requests": b.onRequest,
		"fleet/+/board/releases": b.onRelease,
	} {
		u, err := b.tr.Subscribe(topic, h)
		if err != nil {
			return err
		}
		b.unsubs = append(b.unsubs, u)
	}
	b.stop = make(chan struct{})
	if publishEvery <= 0 {
		publishEvery = 300 * time.Millisecond
	}
	go func() {
		t := time.NewTicker(publishEvery)
		defer t.Stop()
		for {
			select {
			case <-b.stop:
				return
			case <-t.C:
				b.PublishAll()
			}
		}
	}()
	return nil
}

// Stop halts publishing.
func (b *BoardServer) Stop() {
	if b.stop != nil {
		close(b.stop)
		b.stop = nil
	}
	for _, u := range b.unsubs {
		u()
	}
	b.unsubs = nil
}

// AddSite starts publishing for a site.
func (b *BoardServer) AddSite(site string) {
	b.mu.Lock()
	b.sites[site] = true
	b.mu.Unlock()
}

func siteOf(topic string) string {
	// fleet/<site>/...
	start, end := -1, -1
	for i, ch := range topic {
		if ch == '/' {
			if start < 0 {
				start = i + 1
			} else {
				end = i
				break
			}
		}
	}
	if start < 0 || end < 0 {
		return ""
	}
	return topic[start:end]
}

func (b *BoardServer) publishIfChanged(topic string, v any, retain bool) {
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	sum := sha256.Sum256(payload)
	h := hex.EncodeToString(sum[:8])
	b.mu.Lock()
	if b.hashes[topic] == h {
		b.mu.Unlock()
		return
	}
	b.hashes[topic] = h
	b.mu.Unlock()
	_ = b.tr.Publish(topic, payload, retain)
}

// PublishAll publishes tasks, assignments, maps and vehicle infos of every site.
func (b *BoardServer) PublishAll() {
	b.mu.Lock()
	sites := make([]string, 0, len(b.sites))
	for s := range b.sites {
		sites = append(sites, s)
	}
	b.mu.Unlock()
	sort.Strings(sites)
	for _, site := range sites {
		tasks := b.arb.OpenTasks(site)
		if tasks == nil {
			tasks = []Task{}
		}
		b.publishIfChanged(TopicTasks(site), Board{TS: time.Now().UTC(), SiteID: site, Tasks: tasks}, true)
		for _, a := range b.arb.Assignments(site) {
			b.publishIfChanged(TopicAssignments(site, a.VehicleID), a, true)
		}
		if m, ok := b.arb.ActiveMap(site); ok {
			b.publishIfChanged(TopicMap(site), m, true)
		}
		for _, vi := range b.arb.VehicleInfos(site) {
			b.publishIfChanged(TopicVehicle(site, vi.ID), vi, true)
		}
	}
}

func (b *BoardServer) onClaim(topic string, payload []byte) {
	var c Claim
	if err := json.Unmarshal(payload, &c); err != nil || c.TaskID == "" || c.VehicleID == "" {
		return
	}
	site := siteOf(topic)
	if reason, ok := b.arb.Eligible(site, c.VehicleID, c.TaskID); !ok {
		b.answer(site, Grant{ClaimID: c.ClaimID, TaskID: c.TaskID, VehicleID: c.VehicleID, Granted: false, Reason: reason})
		return
	}
	b.mu.Lock()
	w := b.windows[c.TaskID]
	if w == nil {
		w = &claimWindow{siteID: site}
		b.windows[c.TaskID] = w
		taskID := c.TaskID
		w.timer = time.AfterFunc(b.window, func() { b.closeWindow(taskID) })
	}
	w.claims = append(w.claims, c)
	b.mu.Unlock()
}

// closeWindow picks the lowest cost claim and grants it.
func (b *BoardServer) closeWindow(taskID string) {
	b.mu.Lock()
	w := b.windows[taskID]
	delete(b.windows, taskID)
	b.mu.Unlock()
	if w == nil || len(w.claims) == 0 {
		return
	}
	claims := append([]Claim(nil), w.claims...)
	sort.SliceStable(claims, func(i, j int) bool { return claims[i].Cost < claims[j].Cost })
	winner := -1
	for i, c := range claims {
		if reason, ok := b.arb.Eligible(w.siteID, c.VehicleID, taskID); !ok {
			b.answer(w.siteID, Grant{ClaimID: c.ClaimID, TaskID: taskID, VehicleID: c.VehicleID, Reason: reason, Competing: len(claims)})
			continue
		}
		if err := b.arb.Grant(w.siteID, taskID, c.VehicleID, claims); err != nil {
			b.answer(w.siteID, Grant{ClaimID: c.ClaimID, TaskID: taskID, VehicleID: c.VehicleID, Reason: err.Error(), Competing: len(claims)})
			continue
		}
		winner = i
		b.answer(w.siteID, Grant{ClaimID: c.ClaimID, TaskID: taskID, VehicleID: c.VehicleID, Granted: true, Reason: "lowest cost claim", Competing: len(claims)})
		break
	}
	for i, c := range claims {
		if winner >= 0 && i > winner {
			b.answer(w.siteID, Grant{ClaimID: c.ClaimID, TaskID: taskID, VehicleID: c.VehicleID, Reason: "outbid by " + claims[winner].VehicleID, Competing: len(claims)})
		}
	}
	b.PublishAll()
}

func (b *BoardServer) answer(site string, g Grant) {
	b.mu.Lock()
	if g.Granted {
		b.granted++
	} else {
		b.rejected++
	}
	b.history = append(b.history, g)
	if len(b.history) > 200 {
		b.history = b.history[len(b.history)-200:]
	}
	b.mu.Unlock()
	payload, _ := json.Marshal(g)
	_ = b.tr.Publish(TopicGrants(site, g.VehicleID), payload, false)
}

func (b *BoardServer) onRequest(topic string, payload []byte) {
	var r Request
	if err := json.Unmarshal(payload, &r); err != nil || r.VehicleID == "" {
		return
	}
	if err := b.arb.RequestSystemTask(siteOf(topic), r.VehicleID, r.Type, r.Reason); err != nil {
		b.log.Debug("colony request refused", "vehicle", r.VehicleID, "type", r.Type, "err", err)
	}
}

func (b *BoardServer) onRelease(topic string, payload []byte) {
	var r Release
	if err := json.Unmarshal(payload, &r); err != nil || r.VehicleID == "" {
		return
	}
	if err := b.arb.Release(siteOf(topic), r.VehicleID, r.TaskID, r.Reason); err != nil {
		b.log.Warn("colony release refused", "vehicle", r.VehicleID, "task", r.TaskID, "err", err)
	}
}

// Stats returns counters and recent grants.
func (b *BoardServer) Stats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	recent := append([]Grant(nil), b.history...)
	if len(recent) > 50 {
		recent = recent[len(recent)-50:]
	}
	return Stats{Granted: b.granted, Rejected: b.rejected, Recent: recent}
}

var _ = context.Background
