// Package core holds the domain model shared by every Fleet Manager module.
// JSON field names follow docs/api.md exactly.
package core

import (
	"math"
	"time"
)

// Pose is a planar pose in metres / radians (map frame, y up).
type Pose struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Theta float64 `json:"theta"`
}

// Velocity is a planar twist in the vehicle frame.
type Velocity struct {
	VX    float64 `json:"vx"`
	VY    float64 `json:"vy"`
	Omega float64 `json:"omega"`
}

// Point is a 2D point.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Dist returns the euclidean distance between two points.
func (p Point) Dist(q Point) float64 { return math.Hypot(p.X-q.X, p.Y-q.Y) }

// ---------------------------------------------------------------------------
// Sites

type Site struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Timezone    string    `json:"timezone"`
	CreatedAt   time.Time `json:"createdAt"`
}

const DefaultSiteID = "site-default"

// ---------------------------------------------------------------------------
// Vehicle models and vehicles

type VehicleModel struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	LengthM              float64 `json:"lengthM"`
	WidthM               float64 `json:"widthM"`
	HeightM              float64 `json:"heightM"`
	MaxPayloadKg         float64 `json:"maxPayloadKg"`
	MaxSpeedMs           float64 `json:"maxSpeedMs"`
	MaxLateralSpeedMs    float64 `json:"maxLateralSpeedMs"`
	MaxRotationRadS      float64 `json:"maxRotationRadS"`
	AccelMs2             float64 `json:"accelMs2"`
	BatteryCapacityKwh   float64 `json:"batteryCapacityKwh"`
	BogieCount           int     `json:"bogieCount"`
	MotorsPerBogie       int     `json:"motorsPerBogie"`
	WheelDiameterMm      float64 `json:"wheelDiameterMm"`
	GearRatio            float64 `json:"gearRatio"`
	EncoderResolutionDeg float64 `json:"encoderResolutionDeg"`
	EnergyKwhPerM        float64 `json:"energyKwhPerM"`
	EnergyKwhPerTonM     float64 `json:"energyKwhPerTonM"`
}

// BuiltinModels returns the two commercial models. Values are configurable
// through the API; these are engineering defaults, not certified figures.
func BuiltinModels() []VehicleModel {
	return []VehicleModel{
		{
			ID: "AGV20T", Name: "AGV 20T", LengthM: 6.0, WidthM: 2.4, HeightM: 0.25,
			MaxPayloadKg: 20000, MaxSpeedMs: 1.0, MaxLateralSpeedMs: 0.5, MaxRotationRadS: 0.2, AccelMs2: 0.2,
			BatteryCapacityKwh: 40, BogieCount: 4, MotorsPerBogie: 2, WheelDiameterMm: 200, GearRatio: 40,
			EncoderResolutionDeg: 0.5, EnergyKwhPerM: 0.0005, EnergyKwhPerTonM: 0.00002,
		},
		{
			ID: "AGV40T", Name: "AGV 40T", LengthM: 8.0, WidthM: 3.0, HeightM: 0.25,
			MaxPayloadKg: 40000, MaxSpeedMs: 0.8, MaxLateralSpeedMs: 0.4, MaxRotationRadS: 0.15, AccelMs2: 0.15,
			BatteryCapacityKwh: 80, BogieCount: 4, MotorsPerBogie: 2, WheelDiameterMm: 200, GearRatio: 60,
			EncoderResolutionDeg: 0.5, EnergyKwhPerM: 0.0008, EnergyKwhPerTonM: 0.00002,
		},
	}
}

type Protocol string

const (
	ProtocolVDA5050   Protocol = "vda5050"
	ProtocolSimulated Protocol = "simulated"
)

type Footprint struct {
	LengthM float64 `json:"lengthM"`
	WidthM  float64 `json:"widthM"`
}

type Vehicle struct {
	ID                       string     `json:"id"`
	SiteID                   string     `json:"siteId"`
	Name                     string     `json:"name"`
	ModelID                  string     `json:"modelId"`
	Manufacturer             string     `json:"manufacturer"`
	SerialNumber             string     `json:"serialNumber"`
	Protocol                 Protocol   `json:"protocol"`
	Enabled                  bool       `json:"enabled"`
	SafetyMarginM            float64    `json:"safetyMarginM"`
	LocalizationUncertaintyM float64    `json:"localizationUncertaintyM"`
	Footprint                *Footprint `json:"footprint"`
	Tags                     []string   `json:"tags"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

type Connection string

const (
	ConnOnline  Connection = "online"
	ConnOffline Connection = "offline"
	ConnBroken  Connection = "connectionBroken"
	ConnUnknown Connection = "unknown"
)

type OperatingMode string

const (
	ModeAutomatic     OperatingMode = "automatic"
	ModeSemiAutomatic OperatingMode = "semiautomatic"
	ModeManual        OperatingMode = "manual"
	ModeService       OperatingMode = "service"
	ModeTeachIn       OperatingMode = "teachin"
)

type Availability string

const (
	AvailAvailable   Availability = "available"
	AvailBusy        Availability = "busy"
	AvailCharging    Availability = "charging"
	AvailMaintenance Availability = "maintenance"
	AvailError       Availability = "error"
	AvailOffline     Availability = "offline"
	AvailDisabled    Availability = "disabled"
	AvailBlocked     Availability = "blocked"
)

type Load struct {
	LoadID   string  `json:"loadId"`
	LoadType string  `json:"loadType"`
	WeightKg float64 `json:"weightKg"`
}

type VehicleError struct {
	ErrorType        string `json:"errorType"`
	ErrorLevel       string `json:"errorLevel"` // warning | fatal
	ErrorDescription string `json:"errorDescription"`
	ErrorHint        string `json:"errorHint"`
}

type SafetyState struct {
	EStop          string `json:"eStop"` // none | manual | autoAck | remote
	FieldViolation bool   `json:"fieldViolation"`
}

type VehicleState struct {
	VehicleID             string         `json:"vehicleId"`
	Connection            Connection     `json:"connection"`
	LastSeen              *time.Time     `json:"lastSeen"`
	Pose                  Pose           `json:"pose"`
	LocalizationScore     float64        `json:"localizationScore"`
	PositionInitialized   bool           `json:"positionInitialized"`
	Velocity              Velocity       `json:"velocity"`
	Driving               bool           `json:"driving"`
	Paused                bool           `json:"paused"`
	OperatingMode         OperatingMode  `json:"operatingMode"`
	BatteryCharge         float64        `json:"batteryCharge"`
	BatteryVoltage        float64        `json:"batteryVoltage"`
	Charging              bool           `json:"charging"`
	BatteryHealth         float64        `json:"batteryHealth"`
	MissionID             string         `json:"missionId"`
	OrderID               string         `json:"orderId"`
	OrderUpdateID         int            `json:"orderUpdateId"`
	LastNodeID            string         `json:"lastNodeId"`
	LastNodeSequenceID    int            `json:"lastNodeSequenceId"`
	LoadPresent           bool           `json:"loadPresent"`
	LoadMassKg            float64        `json:"loadMassKg"`
	Loads                 []Load         `json:"loads"`
	Errors                []VehicleError `json:"errors"`
	SafetyState           SafetyState    `json:"safetyState"`
	Availability          Availability   `json:"availability"`
	OdometerM             float64        `json:"odometerM"`
	OperatingHours        float64        `json:"operatingHours"`
	MissionsCompleted     int            `json:"missionsCompleted"`
	LoadEnvelope          Footprint      `json:"loadEnvelope"`
	NewBaseRequest        bool           `json:"newBaseRequest"`
	DistanceSinceLastNode float64        `json:"distanceSinceLastNode"`
	ActionStates          []ActionState  `json:"actionStates"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

type ActionState struct {
	ActionID     string `json:"actionId"`
	ActionType   string `json:"actionType"`
	ActionStatus string `json:"actionStatus"` // WAITING INITIALIZING RUNNING PAUSED FINISHED FAILED
}

// VehicleWithState is the API representation of a vehicle.
type VehicleWithState struct {
	Vehicle Vehicle      `json:"vehicle"`
	State   VehicleState `json:"state"`
}

// TelemetrySample is a down-sampled vehicle history point.
type TelemetrySample struct {
	VehicleID     string    `json:"vehicleId,omitempty"`
	TS            time.Time `json:"ts"`
	X             float64   `json:"x"`
	Y             float64   `json:"y"`
	Theta         float64   `json:"theta"`
	BatteryCharge float64   `json:"batteryCharge"`
	Speed         float64   `json:"speed"`
	MissionID     string    `json:"missionId"`
}

// ---------------------------------------------------------------------------
// Maps

type NodeType string

const (
	NodeWaypoint NodeType = "waypoint"
	NodeStation  NodeType = "station"
	NodePickup   NodeType = "pickup"
	NodeDropoff  NodeType = "dropoff"
	NodeCharging NodeType = "charging"
	NodeParking  NodeType = "parking"
	NodeWaiting  NodeType = "waiting"
)

type Node struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	X              float64  `json:"x"`
	Y              float64  `json:"y"`
	Type           NodeType `json:"type"`
	Theta          *float64 `json:"theta"`
	ChargerPowerKw *float64 `json:"chargerPowerKw"`
	Capacity       int      `json:"capacity"`
}

func (n Node) Point() Point { return Point{n.X, n.Y} }

type Motion string

const (
	MotionForward Motion = "forward"
	MotionLateral Motion = "lateral"
	MotionAny     Motion = "any"
)

type Edge struct {
	ID            string  `json:"id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	Bidirectional bool    `json:"bidirectional"`
	MaxSpeedMs    float64 `json:"maxSpeedMs"`
	Motion        Motion  `json:"motion"`
	CostFactor    float64 `json:"costFactor"`
	WidthM        float64 `json:"widthM"`
	Enabled       bool    `json:"enabled"`
}

type ZoneType string

const (
	ZoneExclusive     ZoneType = "exclusive"
	ZoneSingleLane    ZoneType = "singleLane"
	ZoneBidirectional ZoneType = "bidirectional"
	ZoneCrossing      ZoneType = "crossing"
	ZoneWaiting       ZoneType = "waiting"
	ZoneForbidden     ZoneType = "forbidden"
	ZoneSpeedLimit    ZoneType = "speedLimit"
	ZoneCharging      ZoneType = "charging"
)

type Zone struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Type       ZoneType `json:"type"`
	Polygon    []Point  `json:"polygon"`
	Capacity   int      `json:"capacity"`
	Enabled    bool     `json:"enabled"`
	MaxSpeedMs *float64 `json:"maxSpeedMs"`
}

type Map struct {
	ID        string    `json:"id"`
	SiteID    string    `json:"siteId"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	Active    bool      `json:"active"`
	WidthM    float64   `json:"widthM"`
	HeightM   float64   `json:"heightM"`
	OriginX   float64   `json:"originX"`
	OriginY   float64   `json:"originY"`
	Nodes     []Node    `json:"nodes"`
	Edges     []Edge    `json:"edges"`
	Zones     []Zone    `json:"zones"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Summary returns the map without its content (list views).
func (m Map) Summary() Map {
	s := m
	s.Nodes, s.Edges, s.Zones = nil, nil, nil
	return s
}

// ---------------------------------------------------------------------------
// Missions

type MissionType string

const (
	MissionTransport            MissionType = "transport"
	MissionMultiStop            MissionType = "multiStop"
	MissionMachineSupply        MissionType = "machineSupply"
	MissionProductionEvacuation MissionType = "productionEvacuation"
	MissionTransfer             MissionType = "transfer"
	MissionReturnToStation      MissionType = "returnToStation"
	MissionCharge               MissionType = "charge"
	MissionManual               MissionType = "manual"
	MissionExternal             MissionType = "external"
)

type MissionStatus string

const (
	MissionCreated   MissionStatus = "created"
	MissionPending   MissionStatus = "pending"
	MissionPlanned   MissionStatus = "planned"
	MissionAssigned  MissionStatus = "assigned"
	MissionExecuting MissionStatus = "executing"
	MissionSuspended MissionStatus = "suspended"
	MissionCompleted MissionStatus = "completed"
	MissionFailed    MissionStatus = "failed"
	MissionCancelled MissionStatus = "cancelled"
)

// Terminal reports whether the status is final.
func (s MissionStatus) Terminal() bool {
	return s == MissionCompleted || s == MissionFailed || s == MissionCancelled
}

type StepAction string

const (
	StepMove   StepAction = "move"
	StepPick   StepAction = "pick"
	StepDrop   StepAction = "drop"
	StepWait   StepAction = "wait"
	StepCharge StepAction = "charge"
)

type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepActive  StepStatus = "active"
	StepDone    StepStatus = "done"
	StepSkipped StepStatus = "skipped"
)

type MissionStep struct {
	Seq         int        `json:"seq"`
	NodeID      string     `json:"nodeId"`
	Action      StepAction `json:"action"`
	DurationS   float64    `json:"durationS"`
	Status      StepStatus `json:"status"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
}

type MissionLoad struct {
	Present     bool    `json:"present"`
	MassKg      float64 `json:"massKg"`
	LengthM     float64 `json:"lengthM"`
	WidthM      float64 `json:"widthM"`
	HeightM     float64 `json:"heightM"`
	Description string  `json:"description"`
}

type MissionConstraints struct {
	AvoidZoneIDs []string `json:"avoidZoneIds"`
	MaxSpeedMs   *float64 `json:"maxSpeedMs"`
}

type MissionEvent struct {
	At      time.Time     `json:"at"`
	Status  MissionStatus `json:"status"`
	Message string        `json:"message"`
}

type CandidateFactors struct {
	DistanceM   float64 `json:"distanceM"`
	TravelTimeS float64 `json:"travelTimeS"`
	Battery     float64 `json:"battery"`
	Congestion  float64 `json:"congestion"`
	Queue       float64 `json:"queue"`
	Priority    float64 `json:"priority"`
}

type Candidate struct {
	VehicleID string           `json:"vehicleId"`
	Eligible  bool             `json:"eligible"`
	Reason    string           `json:"reason"`
	Score     float64          `json:"score"`
	Factors   CandidateFactors `json:"factors"`
}

type AssignmentExplanation struct {
	ChosenVehicleID string             `json:"chosenVehicleId"`
	Weights         map[string]float64 `json:"weights"`
	Candidates      []Candidate        `json:"candidates"`
}

type MissionSource string

const (
	SourceOperator MissionSource = "operator"
	SourcePLC      MissionSource = "plc"
	SourceERP      MissionSource = "erp"
	SourceWMS      MissionSource = "wms"
	SourceMES      MissionSource = "mes"
	SourceSystem   MissionSource = "system"
	SourceAPI      MissionSource = "api"
)

type Mission struct {
	ID              string                 `json:"id"`
	SiteID          string                 `json:"siteId"`
	Name            string                 `json:"name"`
	Type            MissionType            `json:"type"`
	Priority        int                    `json:"priority"`
	Status          MissionStatus          `json:"status"`
	Origin          string                 `json:"origin"`
	Destination     string                 `json:"destination"`
	Steps           []MissionStep          `json:"steps"`
	Load            MissionLoad            `json:"load"`
	VehicleID       string                 `json:"vehicleId"`
	RequiredModelID string                 `json:"requiredModelId"`
	Source          MissionSource          `json:"source"`
	Dependencies    []string               `json:"dependencies"`
	Constraints     MissionConstraints     `json:"constraints"`
	Deadline        *time.Time             `json:"deadline"`
	CreatedAt       time.Time              `json:"createdAt"`
	AssignedAt      *time.Time             `json:"assignedAt"`
	StartedAt       *time.Time             `json:"startedAt"`
	CompletedAt     *time.Time             `json:"completedAt"`
	RouteNodeIDs    []string               `json:"routeNodeIds"`
	Progress        float64                `json:"progress"`
	Assignment      *AssignmentExplanation `json:"assignment"`
	History         []MissionEvent         `json:"history"`
	ErrorMessage    string                 `json:"errorMessage"`
	ExternalRef     string                 `json:"externalRef"`
	CreatedBy       string                 `json:"createdBy"`
	// LoadOnBoardUnverified is set when a vehicle became unavailable while a
	// load was potentially on board: reassignment then requires a human check.
	LoadOnBoardUnverified bool `json:"loadOnBoardUnverified"`
}

// CurrentStep returns the index of the first step not done, or -1.
func (m *Mission) CurrentStep() int {
	for i, s := range m.Steps {
		if s.Status == StepPending || s.Status == StepActive {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Traffic

type ResourceType string

const (
	ResNode ResourceType = "node"
	ResEdge ResourceType = "edge"
	ResZone ResourceType = "zone"
)

type ResourceRef struct {
	Type ResourceType `json:"type"`
	ID   string       `json:"id"`
}

func (r ResourceRef) Key() string { return string(r.Type) + ":" + r.ID }

type ReservationState string

const (
	ResvRequested ReservationState = "requested"
	ResvGranted   ReservationState = "granted"
	ResvActive    ReservationState = "active"
	ResvReleased  ReservationState = "released"
	ResvUncertain ReservationState = "uncertain"
)

type Reservation struct {
	ID          string           `json:"id"`
	VehicleID   string           `json:"vehicleId"`
	SiteID      string           `json:"siteId"`
	Resource    ResourceRef      `json:"resource"`
	State       ReservationState `json:"state"`
	Sequence    int              `json:"sequence"`
	RequestedAt time.Time        `json:"requestedAt"`
	GrantedAt   *time.Time       `json:"grantedAt"`
	ReleasedAt  *time.Time       `json:"releasedAt"`
	ExpiresAt   *time.Time       `json:"expiresAt"`
}

type Deadlock struct {
	ID          string     `json:"id"`
	SiteID      string     `json:"siteId"`
	VehicleIDs  []string   `json:"vehicleIds"`
	ResourceIDs []string   `json:"resourceIds"`
	DetectedAt  time.Time  `json:"detectedAt"`
	Resolution  string     `json:"resolution"`
	ResolvedAt  *time.Time `json:"resolvedAt"`
}

type BlockedVehicle struct {
	VehicleID  string    `json:"vehicleId"`
	Since      time.Time `json:"since"`
	WaitingFor []string  `json:"waitingFor"`
	HeldBy     string    `json:"heldBy"`
	Reason     string    `json:"reason"`
}

// ---------------------------------------------------------------------------
// Alarms

type Severity string

const (
	SevInfo     Severity = "info"
	SevWarning  Severity = "warning"
	SevError    Severity = "error"
	SevCritical Severity = "critical"
)

type AlarmSource string

const (
	AlarmSrcVehicle     AlarmSource = "vehicle"
	AlarmSrcSystem      AlarmSource = "system"
	AlarmSrcTraffic     AlarmSource = "traffic"
	AlarmSrcBattery     AlarmSource = "battery"
	AlarmSrcComms       AlarmSource = "comms"
	AlarmSrcMaintenance AlarmSource = "maintenance"
	AlarmSrcMission     AlarmSource = "mission"
)

type Alarm struct {
	ID             string      `json:"id"`
	SiteID         string      `json:"siteId"`
	Severity       Severity    `json:"severity"`
	Code           string      `json:"code"`
	Source         AlarmSource `json:"source"`
	VehicleID      string      `json:"vehicleId"`
	MissionID      string      `json:"missionId"`
	Message        string      `json:"message"`
	Active         bool        `json:"active"`
	RaisedAt       time.Time   `json:"raisedAt"`
	AcknowledgedAt *time.Time  `json:"acknowledgedAt"`
	AcknowledgedBy string      `json:"acknowledgedBy"`
	ClearedAt      *time.Time  `json:"clearedAt"`
}

// ---------------------------------------------------------------------------
// Maintenance

type MaintenanceMetric string

const (
	MetricOdometer MaintenanceMetric = "odometerM"
	MetricHours    MaintenanceMetric = "operatingHours"
	MetricMissions MaintenanceMetric = "missions"
	MetricCalendar MaintenanceMetric = "calendarDays"
)

type MaintenancePlan struct {
	ID              string            `json:"id"`
	VehicleID       *string           `json:"vehicleId"`
	ModelID         *string           `json:"modelId"`
	Name            string            `json:"name"`
	Metric          MaintenanceMetric `json:"metric"`
	IntervalValue   float64           `json:"intervalValue"`
	LastDoneValue   float64           `json:"lastDoneValue"`
	LastDoneAt      *time.Time        `json:"lastDoneAt"`
	DueSoonFraction float64           `json:"dueSoonFraction"`
	CreatedAt       time.Time         `json:"createdAt"`
}

type MaintenanceStatus struct {
	PlanID       string            `json:"planId"`
	PlanName     string            `json:"planName"`
	VehicleID    string            `json:"vehicleId"`
	Metric       MaintenanceMetric `json:"metric"`
	CurrentValue float64           `json:"currentValue"`
	NextDueValue float64           `json:"nextDueValue"`
	Remaining    float64           `json:"remaining"`
	State        string            `json:"state"` // ok | dueSoon | overdue
}

type MaintenanceRecord struct {
	ID        string    `json:"id"`
	VehicleID string    `json:"vehicleId"`
	PlanID    string    `json:"planId"`
	DoneAt    time.Time `json:"doneAt"`
	DoneBy    string    `json:"doneBy"`
	Notes     string    `json:"notes"`
}

// ---------------------------------------------------------------------------
// Users / auth / audit

type Role string

const (
	RoleAdmin       Role = "admin"
	RoleSupervisor  Role = "supervisor"
	RoleOperator    Role = "operator"
	RoleViewer      Role = "viewer"
	RoleIntegration Role = "integration"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"displayName"`
	Role         Role      `json:"role"`
	SiteIDs      []string  `json:"siteIds"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"createdAt"`
	PasswordHash string    `json:"-"`
}

type APIKey struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Role      Role          `json:"role"`
	Source    MissionSource `json:"source"`
	SiteID    string        `json:"siteId"`
	Key       string        `json:"key,omitempty"`
	KeyHash   string        `json:"-"`
	CreatedAt time.Time     `json:"createdAt"`
}

type AuditEntry struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	UserID   string    `json:"userId"`
	Username string    `json:"username"`
	Action   string    `json:"action"`
	Target   string    `json:"target"`
	Details  string    `json:"details"`
	IP       string    `json:"ip"`
}

type Webhook struct {
	ID     string   `json:"id"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Secret string   `json:"secret"`
}

// ---------------------------------------------------------------------------
// Settings

// AllocationMode selects who decides task assignment.
type AllocationMode string

const (
	// AllocationCentral: the fleet manager's dispatcher scores and assigns.
	AllocationCentral AllocationMode = "central"
	// AllocationColony: autonomous on-board agents claim tasks from the shared
	// board (ant colony); the fleet manager only arbitrates and verifies.
	AllocationColony AllocationMode = "colony"
)

type DispatcherSettings struct {
	Mode              AllocationMode     `json:"mode"`
	Weights           map[string]float64 `json:"weights"`
	MinBatteryPct     float64            `json:"minBatteryPct"`
	ReserveBatteryPct float64            `json:"reserveBatteryPct"`
	LookaheadSegments int                `json:"lookaheadSegments"`
}

func DefaultDispatcherSettings() DispatcherSettings {
	return DispatcherSettings{
		Mode:              AllocationCentral,
		Weights:           map[string]float64{"distance": 1.0, "travelTime": 1.0, "battery": 0.5, "congestion": 0.7, "queue": 1.0},
		MinBatteryPct:     20,
		ReserveBatteryPct: 10,
		LookaheadSegments: 3,
	}
}

type TrafficSettings struct {
	LookaheadSegments int     `json:"lookaheadSegments"`
	BlockedTimeoutS   float64 `json:"blockedTimeoutS"`
	CommsTimeoutS     float64 `json:"commsTimeoutS"`
	Swept             struct {
		Samples int `json:"samples"`
	} `json:"swept"`
}

func DefaultTrafficSettings() TrafficSettings {
	t := TrafficSettings{LookaheadSegments: 3, BlockedTimeoutS: 60, CommsTimeoutS: 5}
	t.Swept.Samples = 6
	return t
}

type BatterySettings struct {
	LowPct            float64 `json:"lowPct"`
	CriticalPct       float64 `json:"criticalPct"`
	TargetPct         float64 `json:"targetPct"`
	OpportunityPct    float64 `json:"opportunityPct"`
	IdleBeforeChargeS float64 `json:"idleBeforeChargeS"`
}

func DefaultBatterySettings() BatterySettings {
	return BatterySettings{LowPct: 30, CriticalPct: 15, TargetPct: 90, OpportunityPct: 60, IdleBeforeChargeS: 60}
}

// ---------------------------------------------------------------------------
// Events (WebSocket / integration)

type Event struct {
	Kind      string    `json:"kind"`
	SiteID    string    `json:"siteId"`
	VehicleID string    `json:"vehicleId,omitempty"`
	MissionID string    `json:"missionId,omitempty"`
	AlarmID   string    `json:"alarmId,omitempty"`
	Message   string    `json:"message"`
	At        time.Time `json:"at"`
}

// KPIs are the fleet-level performance figures.
type KPIs struct {
	MissionsCompleted   int           `json:"missionsCompleted"`
	MissionsFailed      int           `json:"missionsFailed"`
	MissionsPerHour     float64       `json:"missionsPerHour"`
	AvgMissionDurationS float64       `json:"avgMissionDurationS"`
	AvgWaitTimeS        float64       `json:"avgWaitTimeS"`
	FleetUtilizationPct float64       `json:"fleetUtilizationPct"`
	DistanceM           float64       `json:"distanceM"`
	ActiveAlarms        int           `json:"activeAlarms"`
	DeadlocksResolved   int           `json:"deadlocksResolved"`
	EnergyKwh           float64       `json:"energyKwh"`
	VehiclesOnline      int           `json:"vehiclesOnline"`
	Vehicles            []VehicleKPIs `json:"vehicles,omitempty"`
}

type VehicleKPIs struct {
	VehicleID        string  `json:"vehicleId"`
	UtilizationPct   float64 `json:"utilizationPct"`
	Missions         int     `json:"missions"`
	DistanceM        float64 `json:"distanceM"`
	BatteryChargePct float64 `json:"batteryChargePct"`
	Alarms           int     `json:"alarms"`
}

type TimeseriesPoint struct {
	TS    time.Time `json:"ts"`
	Value float64   `json:"value"`
}
