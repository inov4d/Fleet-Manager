// Package fleet is the Fleet Core: it wires the registry, gateway, planner,
// traffic manager, mission manager, battery / charging logic, alarms,
// maintenance, analytics and simulation into one deterministic control loop.
package fleet

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/alarm"
	"github.com/inov4d/fleet-manager/internal/analytics"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/gateway"
	"github.com/inov4d/fleet-manager/internal/geometry"
	"github.com/inov4d/fleet-manager/internal/maintenance"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/mission"
	"github.com/inov4d/fleet-manager/internal/planner"
	"github.com/inov4d/fleet-manager/internal/sim"
	"github.com/inov4d/fleet-manager/internal/store"
	"github.com/inov4d/fleet-manager/internal/traffic"
	"github.com/inov4d/fleet-manager/internal/transport"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

// Config tunes the core.
type Config struct {
	TickHz          int
	VDAInterface    string
	CommsTimeout    time.Duration
	ReplanAfter     time.Duration // waiting time before replanning around a contested resource
	ReassignAfter   time.Duration // offline time before a mission without load is returned to the queue
	IdleBeforePark  time.Duration
	TelemetryPeriod time.Duration
	Version         string
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{TickHz: 10, VDAInterface: vda5050.DefaultInterface, CommsTimeout: 5 * time.Second, ReplanAfter: 8 * time.Second,
		ReassignAfter: 2 * time.Minute, IdleBeforePark: 3 * time.Minute, TelemetryPeriod: 2 * time.Second, Version: "dev"}
}

type graphEntry struct {
	m *core.Map
	g *mapmgr.Graph
}

type counters struct {
	OdometerM         float64 `json:"odometerM"`
	OperatingHours    float64 `json:"operatingHours"`
	MissionsCompleted int     `json:"missionsCompleted"`
}

type vehicleRuntime struct {
	state           core.VehicleState
	lastPose        core.Pose
	hasPose         bool
	lastStateAt     time.Time
	lastTelemetry   time.Time
	idleSince       time.Time
	offlineSince    time.Time
	maintenance     bool
	counters        counters
	lastBattery     float64
	energyAcc       float64
	nodeMismatch    bool
	pendingDistance float64
	lastTickAt      time.Time
	blocked         bool
	blockedSince    time.Time
}

// Core is the fleet orchestrator.
type Core struct {
	cfg       Config
	log       *slog.Logger
	clock     *core.ScaledClock
	st        store.Store
	tr        transport.Transport
	gw        *gateway.Gateway
	traffic   *traffic.Manager
	planner   planner.Planner
	missions  *mission.Manager
	alarms    *alarm.Manager
	maint     *maintenance.Manager
	analytics *analytics.Engine
	simEngine *sim.Engine

	mu       sync.RWMutex
	sites    map[string]*core.Site
	models   map[string]*core.VehicleModel
	vehicles map[string]*core.Vehicle
	rt       map[string]*vehicleRuntime
	graphs   map[string]*graphEntry
	execs    map[string]*execution
	dispatch core.DispatcherSettings
	battery  core.BatterySettings
	simState simulationState
	whMu     sync.RWMutex
	webhooks []core.Webhook
	pins     map[string]string // mission id -> vehicle id forced assignment

	subsMu       sync.Mutex
	subs         map[int]func(core.Event)
	nextSub      int
	stop         chan struct{}
	wg           sync.WaitGroup
	startedAt    time.Time
	lastMaint    time.Time
	lastSample   time.Time
	lastCounters time.Time
	running      bool
}

// Deps are the collaborators injected into the core.
type Deps struct {
	Store     store.Store
	Transport transport.Transport
	Clock     *core.ScaledClock
	Logger    *slog.Logger
}

// New builds a core from its dependencies and loads configuration from the store.
func New(cfg Config, d Deps) (*Core, error) {
	if d.Clock == nil {
		d.Clock = core.NewScaledClock(1)
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if cfg.TickHz <= 0 {
		cfg.TickHz = 10
	}
	c := &Core{cfg: cfg, log: d.Logger, clock: d.Clock, st: d.Store, tr: d.Transport,
		sites: map[string]*core.Site{}, models: map[string]*core.VehicleModel{}, vehicles: map[string]*core.Vehicle{}, rt: map[string]*vehicleRuntime{},
		graphs: map[string]*graphEntry{}, execs: map[string]*execution{}, subs: map[int]func(core.Event){}, startedAt: d.Clock.Now()}
	c.gw = gateway.New(d.Transport, cfg.VDAInterface, d.Clock, d.Logger, cfg.CommsTimeout)
	c.planner = planner.AStar{}
	c.missions = mission.New(d.Store, d.Clock)
	c.alarms = alarm.New(d.Store, d.Clock)
	c.maint = maintenance.New(d.Store, d.Clock)
	c.analytics = analytics.New(d.Store, d.Clock)
	c.simEngine = sim.NewEngine(d.Transport, d.Clock)
	ts := core.DefaultTrafficSettings()
	if _, err := d.Store.GetSetting(context.Background(), "traffic", &ts); err != nil {
		return nil, err
	}
	c.traffic = traffic.New(ts, d.Clock)
	c.traffic.SetSettings(ts)
	c.dispatch = core.DefaultDispatcherSettings()
	_, _ = d.Store.GetSetting(context.Background(), "dispatcher", &c.dispatch)
	c.battery = core.DefaultBatterySettings()
	_, _ = d.Store.GetSetting(context.Background(), "battery", &c.battery)
	if err := c.load(); err != nil {
		return nil, err
	}
	c.alarms.SetHandlers(func(a core.Alarm) {
		c.emit(core.Event{Kind: "alarm.raised", SiteID: a.SiteID, VehicleID: a.VehicleID, MissionID: a.MissionID, AlarmID: a.ID, Message: a.Message})
	}, func(a core.Alarm) {
		c.emit(core.Event{Kind: "alarm.cleared", SiteID: a.SiteID, VehicleID: a.VehicleID, AlarmID: a.ID, Message: a.Message})
	})
	c.missions.SetHandler(func(m core.Mission, kind string) {
		c.emit(core.Event{Kind: kind, SiteID: m.SiteID, VehicleID: m.VehicleID, MissionID: m.ID, Message: m.Name + ": " + string(m.Status)})
	})
	return c, nil
}

func (c *Core) load() error {
	ctx := context.Background()
	sites, err := c.st.ListSites(ctx)
	if err != nil {
		return err
	}
	if len(sites) == 0 {
		s := core.Site{ID: core.DefaultSiteID, Name: "Site principal", Timezone: "Europe/Paris", CreatedAt: c.clock.Now()}
		if err := c.st.SaveSite(ctx, &s); err != nil {
			return err
		}
		sites = []core.Site{s}
	}
	for i := range sites {
		c.sites[sites[i].ID] = &sites[i]
	}
	models, err := c.st.ListModels(ctx)
	if err != nil {
		return err
	}
	for _, m := range core.BuiltinModels() {
		found := false
		for _, ex := range models {
			if ex.ID == m.ID {
				found = true
			}
		}
		if !found {
			if err := c.st.SaveModel(ctx, &m); err != nil {
				return err
			}
			models = append(models, m)
		}
	}
	for i := range models {
		c.models[models[i].ID] = &models[i]
	}
	vehicles, err := c.st.ListVehicles(ctx, "")
	if err != nil {
		return err
	}
	for i := range vehicles {
		v := vehicles[i]
		if v.Protocol == core.ProtocolSimulated {
			// simulated vehicles do not survive a restart
			_ = c.st.DeleteVehicle(ctx, v.ID)
			continue
		}
		c.vehicles[v.ID] = &v
		c.rt[v.ID] = c.newRuntime(v.ID)
		c.gw.Register(v.ID, v.Manufacturer, v.SerialNumber)
	}
	maps, err := c.st.ListMaps(ctx, "")
	if err != nil {
		return err
	}
	for i := range maps {
		if maps[i].Active {
			m := maps[i]
			if err := mapmgr.Validate(&m); err == nil {
				c.installGraph(&m)
			}
		}
	}
	c.whMu.Lock()
	c.webhooks, _ = c.st.ListWebhooks(ctx)
	c.whMu.Unlock()
	return nil
}

func (c *Core) newRuntime(id string) *vehicleRuntime {
	r := &vehicleRuntime{state: core.VehicleState{VehicleID: id, Connection: core.ConnUnknown, Availability: core.AvailOffline, OperatingMode: core.ModeAutomatic,
		Loads: []core.Load{}, Errors: []core.VehicleError{}, ActionStates: []core.ActionState{}, SafetyState: core.SafetyState{EStop: "none"}}, idleSince: c.clock.Now()}
	_, _ = c.st.GetSetting(context.Background(), "counters:"+id, &r.counters)
	_, _ = c.st.GetSetting(context.Background(), "maintenance:"+id, &r.maintenance)
	return r
}

func (c *Core) installGraph(m *core.Map) {
	g := mapmgr.NewGraph(m)
	c.graphs[m.SiteID] = &graphEntry{m: m, g: g}
	c.traffic.SetGraph(m.SiteID, g)
}

// Start launches the gateway and the control loop.
func (c *Core) Start() error {
	if err := c.gw.Start(c.onVehicleState, c.onVehicleConnection); err != nil {
		return err
	}
	c.mu.Lock()
	c.running = true
	c.stop = make(chan struct{})
	stop := c.stop
	c.mu.Unlock()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		t := time.NewTicker(time.Second / time.Duration(c.cfg.TickHz))
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				c.Tick()
			}
		}
	}()
	c.log.Info("fleet core started", "tickHz", c.cfg.TickHz, "vehicles", len(c.vehicles))
	return nil
}

// Stop halts the loop, the simulation and persists counters.
func (c *Core) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stop)
	c.mu.Unlock()
	c.wg.Wait()
	c.simEngine.Stop()
	c.gw.Stop()
	c.persistCounters(true)
}

// Subscribe registers an event listener; the returned func unsubscribes.
func (c *Core) Subscribe(fn func(core.Event)) func() {
	c.subsMu.Lock()
	id := c.nextSub
	c.nextSub++
	c.subs[id] = fn
	c.subsMu.Unlock()
	return func() {
		c.subsMu.Lock()
		delete(c.subs, id)
		c.subsMu.Unlock()
	}
}

func (c *Core) emit(e core.Event) {
	e.At = c.clock.Now()
	c.subsMu.Lock()
	fns := make([]func(core.Event), 0, len(c.subs))
	for _, f := range c.subs {
		fns = append(fns, f)
	}
	c.subsMu.Unlock()
	for _, f := range fns {
		f(e)
	}
	c.deliverWebhooks(e)
}

// Accessors ----------------------------------------------------------------

func (c *Core) Clock() *core.ScaledClock          { return c.clock }
func (c *Core) Store() store.Store                { return c.st }
func (c *Core) Alarms() *alarm.Manager            { return c.alarms }
func (c *Core) Missions() *mission.Manager        { return c.missions }
func (c *Core) Traffic() *traffic.Manager         { return c.traffic }
func (c *Core) Maintenance() *maintenance.Manager { return c.maint }
func (c *Core) Analytics() *analytics.Engine      { return c.analytics }
func (c *Core) Gateway() *gateway.Gateway         { return c.gw }
func (c *Core) Version() string                   { return c.cfg.Version }
func (c *Core) StartedAt() time.Time              { return c.startedAt }

// Tick runs one control cycle. It is safe to call manually (tests).
func (c *Core) Tick() {
	now := c.clock.Now()
	// 1. communication timeouts
	for _, vid := range c.gw.CheckTimeouts() {
		c.log.Warn("vehicle communication lost", "vehicle", vid)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// 2. per-vehicle state refresh and traffic observation
	for id, v := range c.vehicles {
		r := c.rt[id]
		c.refreshDerived(v, r, now)
		if g := c.graphs[v.SiteID]; g != nil && r.state.PositionInitialized {
			node := c.currentNode(v, r, g.g)
			prio, ri := 0, -1
			if ex := c.execs[id]; ex != nil && ex.routeIdx < len(ex.route) {
				prio = ex.priority
				// the route position reported by the vehicle (sequenceId) is
				// authoritative while it follows its order; geometry only takes
				// over when the vehicle clearly stands somewhere else
				rn := g.g.Nodes[ex.route[ex.routeIdx]]
				if rn != nil && (r.state.Driving || rn.Point().Dist(core.Point{X: r.state.Pose.X, Y: r.state.Pose.Y}) <= 3.0) {
					node, ri = rn.ID, ex.routeIdx
				}
			}
			c.traffic.Observe(traffic.VehicleCtx{ID: id, SiteID: v.SiteID, Pose: r.state.Pose, Envelope: r.state.LoadEnvelope, Priority: prio, CurrentNode: node, Connected: r.state.Connection == core.ConnOnline, RouteIndex: ri})
		}
		c.vehicleAlarms(v, r)
		c.analytics.Observe(id, c.execs[id] != nil, r.distanceDelta(), r.takeEnergy())
	}
	// 3. executions
	for id := range c.execs {
		c.stepExecution(id, now)
	}
	// 4. deadlocks
	for _, res := range c.traffic.DetectDeadlocks() {
		c.handleDeadlock(res)
	}
	// 5. blocked vehicles
	c.handleBlocked(now)
	// 6. dispatch, charging, parking
	c.dispatchMissions()
	c.batteryManagement(now)
	// 7. telemetry / analytics / maintenance housekeeping
	c.housekeeping(now)
	c.simulationTick(now)
}

// refreshDerived computes connection, envelope, availability, counters.
func (c *Core) refreshDerived(v *core.Vehicle, r *vehicleRuntime, now time.Time) {
	conn, lastSeen := c.gw.Connection(v.ID)
	if r.state.Connection == core.ConnOnline && conn != core.ConnOnline {
		r.offlineSince = now
	}
	r.state.Connection = conn
	if !lastSeen.IsZero() {
		ls := lastSeen
		r.state.LastSeen = &ls
	}
	model := c.models[v.ModelID]
	bodyL, bodyW := 0.0, 0.0
	if model != nil {
		bodyL, bodyW = model.LengthM, model.WidthM
	}
	if v.Footprint != nil && v.Footprint.LengthM > 0 {
		bodyL, bodyW = v.Footprint.LengthM, v.Footprint.WidthM
	}
	loadL, loadW := 0.0, 0.0
	if ex := c.execs[v.ID]; ex != nil && ex.loadOnBoard(r.state) {
		if m, err := c.missions.Get(ex.missionID); err == nil && m.Load.Present {
			loadL, loadW = m.Load.LengthM, m.Load.WidthM
		}
	}
	r.state.LoadEnvelope = geometry.EffectiveFootprint(bodyL, bodyW, loadL, loadW, v.SafetyMarginM, v.LocalizationUncertaintyM)
	// odometer / hours from observed motion
	if r.hasPose && r.state.Driving {
		r.counters.OperatingHours += c.dtSeconds(r) / 3600
	}
	r.state.MissionID = ""
	if ex := c.execs[v.ID]; ex != nil {
		r.state.MissionID = ex.missionID
	}
	r.state.OdometerM = r.counters.OdometerM
	r.state.OperatingHours = r.counters.OperatingHours
	r.state.MissionsCompleted = r.counters.MissionsCompleted
	// availability
	switch {
	case !v.Enabled:
		r.state.Availability = core.AvailDisabled
	case conn != core.ConnOnline:
		r.state.Availability = core.AvailOffline
	case r.maintenance:
		r.state.Availability = core.AvailMaintenance
	case r.state.SafetyState.EStop != "none" || hasFatal(r.state.Errors):
		r.state.Availability = core.AvailError
	case r.blocked:
		r.state.Availability = core.AvailBlocked
	case r.state.Charging:
		r.state.Availability = core.AvailCharging
	case c.execs[v.ID] != nil:
		r.state.Availability = core.AvailBusy
	default:
		r.state.Availability = core.AvailAvailable
	}
	if r.state.Availability != core.AvailAvailable {
		r.idleSince = now
	}
	r.state.UpdatedAt = now
}

func (r *vehicleRuntime) distanceDelta() float64 {
	d := r.pendingDistance
	r.pendingDistance = 0
	return d
}

func (r *vehicleRuntime) takeEnergy() float64 {
	e := r.energyAcc
	r.energyAcc = 0
	return e
}

func (c *Core) dtSeconds(r *vehicleRuntime) float64 {
	if r.lastTickAt.IsZero() {
		r.lastTickAt = c.clock.Now()
		return 0
	}
	now := c.clock.Now()
	dt := now.Sub(r.lastTickAt).Seconds()
	r.lastTickAt = now
	if dt < 0 || dt > 60 {
		return 0
	}
	return dt
}

func hasFatal(errs []core.VehicleError) bool {
	for _, e := range errs {
		if e.ErrorLevel == "fatal" {
			return true
		}
	}
	return false
}

// currentNode resolves the node a vehicle stands on: the reported lastNodeId
// when the position is consistent with it, otherwise the nearest node.
func (c *Core) currentNode(v *core.Vehicle, r *vehicleRuntime, g *mapmgr.Graph) string {
	p := core.Point{X: r.state.Pose.X, Y: r.state.Pose.Y}
	if n := g.Nodes[r.state.LastNodeID]; n != nil && n.Point().Dist(p) <= 3.0 {
		r.nodeMismatch = false
		return n.ID
	}
	n, d := g.NearestNode(p)
	if n != nil && d <= 1.5 {
		return n.ID
	}
	if n != nil && r.state.Driving {
		// between nodes while driving: keep the last node reported by the vehicle
		if ln := g.Nodes[r.state.LastNodeID]; ln != nil {
			return ln.ID
		}
	}
	r.nodeMismatch = true
	return ""
}

// onVehicleState ingests a validated VDA 5050 state (gateway goroutine).
func (c *Core) onVehicleState(vehicleID string, s *vda5050.State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.vehicles[vehicleID]
	r := c.rt[vehicleID]
	if v == nil || r == nil {
		return
	}
	st := &r.state
	now := c.clock.Now()
	if s.AGVPosition != nil {
		np := core.Pose{X: s.AGVPosition.X, Y: s.AGVPosition.Y, Theta: s.AGVPosition.Theta}
		if r.hasPose {
			d := math.Hypot(np.X-r.lastPose.X, np.Y-r.lastPose.Y)
			if d < 50 { // ignore teleports (re-localisation)
				r.counters.OdometerM += d
				r.pendingDistance += d
			}
		}
		r.lastPose, r.hasPose = np, true
		st.Pose = np
		st.PositionInitialized = s.AGVPosition.PositionInitialized
		if s.AGVPosition.LocalizationScore != nil {
			st.LocalizationScore = *s.AGVPosition.LocalizationScore
		} else {
			st.LocalizationScore = 1
		}
	}
	if s.Velocity != nil {
		st.Velocity = core.Velocity{VX: deref(s.Velocity.VX), VY: deref(s.Velocity.VY), Omega: deref(s.Velocity.Omega)}
	}
	st.Driving = s.Driving
	st.Paused = s.Paused != nil && *s.Paused
	st.OperatingMode = core.OperatingMode(lower(string(s.OperatingMode)))
	if r.lastBattery > 0 && s.BatteryState.BatteryCharge < r.lastBattery {
		if m := c.models[v.ModelID]; m != nil {
			r.energyAcc += (r.lastBattery - s.BatteryState.BatteryCharge) / 100 * m.BatteryCapacityKwh
		}
	}
	r.lastBattery = s.BatteryState.BatteryCharge
	st.BatteryCharge = s.BatteryState.BatteryCharge
	st.BatteryVoltage = deref(s.BatteryState.BatteryVoltage)
	st.BatteryHealth = deref(s.BatteryState.BatteryHealth)
	st.Charging = s.BatteryState.Charging
	st.OrderID = s.OrderID
	st.OrderUpdateID = s.OrderUpdateID
	st.LastNodeID = s.LastNodeID
	st.LastNodeSequenceID = s.LastNodeSequenceID
	st.NewBaseRequest = s.NewBaseRequest != nil && *s.NewBaseRequest
	st.DistanceSinceLastNode = deref(s.DistanceSinceLastNode)
	st.Loads = st.Loads[:0]
	st.LoadMassKg = 0
	for _, l := range s.Loads {
		st.Loads = append(st.Loads, core.Load{LoadID: l.LoadID, LoadType: l.LoadType, WeightKg: deref(l.Weight)})
		st.LoadMassKg += deref(l.Weight)
	}
	st.LoadPresent = len(s.Loads) > 0
	st.Errors = st.Errors[:0]
	for _, e := range s.Errors {
		st.Errors = append(st.Errors, core.VehicleError{ErrorType: e.ErrorType, ErrorLevel: lower(string(e.ErrorLevel)), ErrorDescription: e.ErrorDescription, ErrorHint: e.ErrorHint})
	}
	st.SafetyState = core.SafetyState{EStop: estop(s.SafetyState.EStop), FieldViolation: s.SafetyState.FieldViolation}
	st.ActionStates = st.ActionStates[:0]
	for _, a := range s.ActionStates {
		st.ActionStates = append(st.ActionStates, core.ActionState{ActionID: a.ActionID, ActionType: a.ActionType, ActionStatus: string(a.ActionStatus)})
	}
	r.lastStateAt = now
	if ex := c.execs[vehicleID]; ex != nil {
		ex.lastStateSeq = s.HeaderID
	}
}

func (c *Core) onVehicleConnection(vehicleID string, conn core.Connection) {
	c.mu.Lock()
	v := c.vehicles[vehicleID]
	c.mu.Unlock()
	if v == nil {
		return
	}
	c.emit(core.Event{Kind: "vehicle.connection", SiteID: v.SiteID, VehicleID: vehicleID, Message: v.Name + ": " + string(conn)})
	if conn == core.ConnOnline {
		c.alarms.ClearByKey(core.AlarmSrcComms, "COMMS_LOST", vehicleID, "")
	} else {
		c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevError, Code: "COMMS_LOST", Source: core.AlarmSrcComms, VehicleID: vehicleID, Message: v.Name + ": communication perdue (" + string(conn) + ")"})
	}
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func estop(e vda5050.EStop) string {
	switch e {
	case vda5050.EStopAutoAck:
		return "autoAck"
	case vda5050.EStopManual:
		return "manual"
	case vda5050.EStopRemote:
		return "remote"
	}
	return "none"
}

// vehicleAlarms raises / clears alarms derived from the vehicle state.
func (c *Core) vehicleAlarms(v *core.Vehicle, r *vehicleRuntime) {
	keep := map[string]bool{}
	for _, e := range r.state.Errors {
		sev := core.SevWarning
		if e.ErrorLevel == "fatal" {
			sev = core.SevError
		}
		code := "VEH_" + e.ErrorType
		keep[code] = true
		c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: sev, Code: code, Source: core.AlarmSrcVehicle, VehicleID: v.ID, Message: v.Name + ": " + e.ErrorType + " " + e.ErrorDescription})
	}
	if r.state.SafetyState.EStop != "none" {
		keep["ESTOP"] = true
		c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevCritical, Code: "ESTOP", Source: core.AlarmSrcVehicle, VehicleID: v.ID, Message: v.Name + ": arrêt d'urgence (" + r.state.SafetyState.EStop + ")"})
	}
	if r.state.SafetyState.FieldViolation {
		keep["FIELD_VIOLATION"] = true
		c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevWarning, Code: "FIELD_VIOLATION", Source: core.AlarmSrcVehicle, VehicleID: v.ID, Message: v.Name + ": champ de sécurité violé"})
	}
	if r.state.Connection == core.ConnOnline {
		c.alarms.ClearVehicleSource(core.AlarmSrcVehicle, v.ID, keep)
	}
	if r.state.Connection == core.ConnOnline {
		switch {
		case r.state.BatteryCharge <= c.battery.CriticalPct:
			c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevCritical, Code: "BATTERY_CRITICAL", Source: core.AlarmSrcBattery, VehicleID: v.ID, Message: v.Name + ": batterie critique"})
			c.alarms.ClearByKey(core.AlarmSrcBattery, "BATTERY_LOW", v.ID, "")
		case r.state.BatteryCharge <= c.battery.LowPct:
			c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevWarning, Code: "BATTERY_LOW", Source: core.AlarmSrcBattery, VehicleID: v.ID, Message: v.Name + ": batterie faible"})
			c.alarms.ClearByKey(core.AlarmSrcBattery, "BATTERY_CRITICAL", v.ID, "")
		default:
			c.alarms.ClearByKey(core.AlarmSrcBattery, "BATTERY_LOW", v.ID, "")
			c.alarms.ClearByKey(core.AlarmSrcBattery, "BATTERY_CRITICAL", v.ID, "")
		}
	}
	if r.nodeMismatch && r.state.PositionInitialized && r.state.Connection == core.ConnOnline && !r.state.Driving {
		c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevWarning, Code: "OFF_GRAPH", Source: core.AlarmSrcSystem, VehicleID: v.ID, Message: v.Name + ": position hors du réseau de circulation"})
	} else {
		c.alarms.ClearByKey(core.AlarmSrcSystem, "OFF_GRAPH", v.ID, "")
	}
}

func (c *Core) housekeeping(now time.Time) {
	// telemetry
	var batch []core.TelemetrySample
	for id, r := range c.rt {
		if r.state.Connection != core.ConnOnline || now.Sub(r.lastTelemetry) < c.cfg.TelemetryPeriod {
			continue
		}
		r.lastTelemetry = now
		mid := ""
		if ex := c.execs[id]; ex != nil {
			mid = ex.missionID
		}
		batch = append(batch, core.TelemetrySample{VehicleID: id, TS: now, X: r.state.Pose.X, Y: r.state.Pose.Y, Theta: r.state.Pose.Theta, BatteryCharge: r.state.BatteryCharge,
			Speed: math.Hypot(r.state.Velocity.VX, r.state.Velocity.VY), MissionID: mid})
	}
	if len(batch) > 0 {
		go func() { _ = c.st.AppendTelemetry(context.Background(), batch) }()
	}
	if now.Sub(c.lastSample) >= time.Minute {
		c.lastSample = now
		for sid := range c.sites {
			in := c.analyticsInput(sid)
			go c.analytics.Sample(sid, in, 0)
		}
		c.missions.Prune(24 * time.Hour)
	}
	if now.Sub(c.lastMaint) >= time.Minute {
		c.lastMaint = now
		c.evaluateMaintenance()
	}
	if now.Sub(c.lastCounters) >= 30*time.Second {
		c.lastCounters = now
		c.persistCounters(false)
	}
}

func (c *Core) persistCounters(locked bool) {
	if !locked {
		// called under c.mu
	}
	type kv struct {
		k string
		v counters
	}
	var list []kv
	if locked {
		c.mu.RLock()
	}
	for id, r := range c.rt {
		list = append(list, kv{"counters:" + id, r.counters})
	}
	if locked {
		c.mu.RUnlock()
	}
	go func() {
		for _, e := range list {
			_ = c.st.SetSetting(context.Background(), e.k, e.v)
		}
	}()
}

func (c *Core) evaluateMaintenance() {
	vs := make([]core.Vehicle, 0, len(c.vehicles))
	for _, v := range c.vehicles {
		vs = append(vs, *v)
	}
	ctr := func(id string) maintenance.Counters {
		if r := c.rt[id]; r != nil {
			return maintenance.Counters{OdometerM: r.counters.OdometerM, OperatingHours: r.counters.OperatingHours, Missions: r.counters.MissionsCompleted}
		}
		return maintenance.Counters{}
	}
	overdue := map[string]bool{}
	for _, s := range c.maint.Evaluate(vs, ctr) {
		if s.State == "overdue" {
			overdue[s.VehicleID] = true
			if v := c.vehicles[s.VehicleID]; v != nil {
				c.alarms.Raise(core.Alarm{SiteID: v.SiteID, Severity: core.SevWarning, Code: "MAINT_OVERDUE", Source: core.AlarmSrcMaintenance, VehicleID: v.ID, Message: v.Name + ": maintenance en retard (" + s.PlanName + ")"})
			}
		}
	}
	for id := range c.vehicles {
		if !overdue[id] {
			c.alarms.ClearByKey(core.AlarmSrcMaintenance, "MAINT_OVERDUE", id, "")
		}
	}
}

type analyticsInput = analytics.Input

func (c *Core) analyticsInput(siteID string) analytics.Input {
	var vs []core.VehicleWithState
	for id, v := range c.vehicles {
		if v.SiteID == siteID {
			vs = append(vs, core.VehicleWithState{Vehicle: *v, State: c.rt[id].state})
		}
	}
	return analytics.Input{SiteID: siteID, Vehicles: vs, ActiveAlarms: len(c.alarms.Active(siteID)), Deadlocks: c.traffic.ResolvedCount(), AlarmsByVehicle: c.alarms.ActiveForVehicle}
}

// Sites / models ------------------------------------------------------------

func (c *Core) Sites() []core.Site {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]core.Site, 0, len(c.sites))
	for _, s := range c.sites {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c *Core) GetSite(id string) (*core.Site, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if s := c.sites[id]; s != nil {
		cp := *s
		return &cp, nil
	}
	return nil, core.NotFound("site", id)
}

func (c *Core) SaveSite(s core.Site) (*core.Site, error) {
	if s.Name == "" {
		return nil, core.Validation("site name is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if s.ID == "" {
		s.ID = "site-" + uuid.NewString()[:8]
		s.CreatedAt = c.clock.Now()
	} else if ex := c.sites[s.ID]; ex != nil {
		s.CreatedAt = ex.CreatedAt
	} else {
		s.CreatedAt = c.clock.Now()
	}
	if s.Timezone == "" {
		s.Timezone = "Europe/Paris"
	}
	if err := c.st.SaveSite(context.Background(), &s); err != nil {
		return nil, err
	}
	c.sites[s.ID] = &s
	return &s, nil
}

func (c *Core) DeleteSite(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id == core.DefaultSiteID {
		return core.Conflict("the default site cannot be deleted")
	}
	for _, v := range c.vehicles {
		if v.SiteID == id {
			return core.Conflict("site still has vehicles")
		}
	}
	delete(c.sites, id)
	return c.st.DeleteSite(context.Background(), id)
}

func (c *Core) Models() []core.VehicleModel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]core.VehicleModel, 0, len(c.models))
	for _, m := range c.models {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *Core) SaveModel(m core.VehicleModel) (*core.VehicleModel, error) {
	if m.ID == "" || m.LengthM <= 0 || m.WidthM <= 0 || m.MaxPayloadKg <= 0 || m.MaxSpeedMs <= 0 || m.BatteryCapacityKwh <= 0 {
		return nil, core.Validation("model needs id, positive dimensions, payload, speed and battery capacity")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.st.SaveModel(context.Background(), &m); err != nil {
		return nil, err
	}
	c.models[m.ID] = &m
	return &m, nil
}

// Settings -------------------------------------------------------------------

func (c *Core) DispatcherSettings() core.DispatcherSettings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dispatch
}

func (c *Core) SetDispatcherSettings(s core.DispatcherSettings) error {
	if s.Weights == nil {
		s.Weights = core.DefaultDispatcherSettings().Weights
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dispatch = s
	return c.st.SetSetting(context.Background(), "dispatcher", s)
}

func (c *Core) BatterySettings() core.BatterySettings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.battery
}

func (c *Core) SetBatterySettings(s core.BatterySettings) error {
	if s.CriticalPct >= s.LowPct || s.LowPct >= s.TargetPct || s.TargetPct > 100 {
		return core.Validation("thresholds must satisfy critical < low < target <= 100")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.battery = s
	return c.st.SetSetting(context.Background(), "battery", s)
}

func (c *Core) TrafficSettings() core.TrafficSettings { return c.traffic.Settings() }

func (c *Core) SetTrafficSettings(s core.TrafficSettings) error {
	c.traffic.SetSettings(s)
	return c.st.SetSetting(context.Background(), "traffic", c.traffic.Settings())
}

// Webhooks -------------------------------------------------------------------

func (c *Core) Webhooks() []core.Webhook {
	c.whMu.RLock()
	defer c.whMu.RUnlock()
	return append([]core.Webhook{}, c.webhooks...)
}

func (c *Core) SaveWebhook(w core.Webhook) (*core.Webhook, error) {
	if w.URL == "" {
		return nil, core.Validation("url is required")
	}
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	c.whMu.Lock()
	defer c.whMu.Unlock()
	if err := c.st.SaveWebhook(context.Background(), &w); err != nil {
		return nil, err
	}
	c.webhooks, _ = c.st.ListWebhooks(context.Background())
	return &w, nil
}

func (c *Core) DeleteWebhook(id string) error {
	c.whMu.Lock()
	defer c.whMu.Unlock()
	if err := c.st.DeleteWebhook(context.Background(), id); err != nil {
		return err
	}
	c.webhooks, _ = c.st.ListWebhooks(context.Background())
	return nil
}
