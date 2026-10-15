// Package vda5050 implements the VDA 5050 version 2.0.0 message model
// (order, instantActions, state, visualization, connection, factsheet) and
// the topic layout "<interfaceName>/v<major>/<manufacturer>/<serialNumber>/<topic>".
//
// Field names and enumerations follow the official JSON schemas of VDA 5050
// 2.0.0. Optional fields are pointers or omitempty so messages validate
// against the schemas when serialised.
package vda5050

import "time"

const (
	Version          = "2.0.0"
	MajorVersion     = "v2"
	DefaultInterface = "uagv"

	TopicOrder          = "order"
	TopicInstantActions = "instantActions"
	TopicState          = "state"
	TopicVisualization  = "visualization"
	TopicConnection     = "connection"
	TopicFactsheet      = "factsheet"
)

// Header is common to all messages.
type Header struct {
	HeaderID     int       `json:"headerId"`
	Timestamp    time.Time `json:"timestamp"`
	Version      string    `json:"version"`
	Manufacturer string    `json:"manufacturer"`
	SerialNumber string    `json:"serialNumber"`
}

// ---------------------------------------------------------------------------
// Order

type Order struct {
	Header
	OrderID       string `json:"orderId"`
	OrderUpdateID int    `json:"orderUpdateId"`
	ZoneSetID     string `json:"zoneSetId,omitempty"`
	Nodes         []Node `json:"nodes"`
	Edges         []Edge `json:"edges"`
}

type NodePosition struct {
	X                     float64  `json:"x"`
	Y                     float64  `json:"y"`
	Theta                 *float64 `json:"theta,omitempty"`
	AllowedDeviationXY    *float64 `json:"allowedDeviationXY,omitempty"`
	AllowedDeviationTheta *float64 `json:"allowedDeviationTheta,omitempty"`
	MapID                 string   `json:"mapId"`
	MapDescription        string   `json:"mapDescription,omitempty"`
}

type Node struct {
	NodeID          string        `json:"nodeId"`
	SequenceID      int           `json:"sequenceId"`
	NodeDescription string        `json:"nodeDescription,omitempty"`
	Released        bool          `json:"released"`
	NodePosition    *NodePosition `json:"nodePosition,omitempty"`
	Actions         []Action      `json:"actions"`
}

type Edge struct {
	EdgeID           string   `json:"edgeId"`
	SequenceID       int      `json:"sequenceId"`
	EdgeDescription  string   `json:"edgeDescription,omitempty"`
	Released         bool     `json:"released"`
	StartNodeID      string   `json:"startNodeId"`
	EndNodeID        string   `json:"endNodeId"`
	MaxSpeed         *float64 `json:"maxSpeed,omitempty"`
	MaxHeight        *float64 `json:"maxHeight,omitempty"`
	MinHeight        *float64 `json:"minHeight,omitempty"`
	Orientation      *float64 `json:"orientation,omitempty"`
	OrientationType  string   `json:"orientationType,omitempty"` // GLOBAL | TANGENTIAL
	Direction        string   `json:"direction,omitempty"`
	RotationAllowed  *bool    `json:"rotationAllowed,omitempty"`
	MaxRotationSpeed *float64 `json:"maxRotationSpeed,omitempty"`
	Length           *float64 `json:"length,omitempty"`
	Actions          []Action `json:"actions"`
}

type BlockingType string

const (
	BlockingNone BlockingType = "NONE"
	BlockingSoft BlockingType = "SOFT"
	BlockingHard BlockingType = "HARD"
)

type ActionParameter struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type Action struct {
	ActionType        string            `json:"actionType"`
	ActionID          string            `json:"actionId"`
	ActionDescription string            `json:"actionDescription,omitempty"`
	BlockingType      BlockingType      `json:"blockingType"`
	ActionParameters  []ActionParameter `json:"actionParameters,omitempty"`
}

// Standard action types (VDA 5050 2.0.0, section 6.8 / 6.9).
const (
	ActionStartPause       = "startPause"
	ActionStopPause        = "stopPause"
	ActionStartCharging    = "startCharging"
	ActionStopCharging     = "stopCharging"
	ActionInitPosition     = "initPosition"
	ActionStateRequest     = "stateRequest"
	ActionLogReport        = "logReport"
	ActionPick             = "pick"
	ActionDrop             = "drop"
	ActionDetectObject     = "detectObject"
	ActionFinePositioning  = "finePositioning"
	ActionWaitForTrigger   = "waitForTrigger"
	ActionCancelOrder      = "cancelOrder"
	ActionFactsheetRequest = "factsheetRequest"
)

// ---------------------------------------------------------------------------
// Instant actions

type InstantActions struct {
	Header
	Actions []Action `json:"actions"`
}

// ---------------------------------------------------------------------------
// State

type OperatingMode string

const (
	OpAutomatic     OperatingMode = "AUTOMATIC"
	OpSemiAutomatic OperatingMode = "SEMIAUTOMATIC"
	OpManual        OperatingMode = "MANUAL"
	OpService       OperatingMode = "SERVICE"
	OpTeachIn       OperatingMode = "TEACHIN"
)

type NodeState struct {
	NodeID          string        `json:"nodeId"`
	SequenceID      int           `json:"sequenceId"`
	NodeDescription string        `json:"nodeDescription,omitempty"`
	NodePosition    *NodePosition `json:"nodePosition,omitempty"`
	Released        bool          `json:"released"`
}

type EdgeState struct {
	EdgeID          string `json:"edgeId"`
	SequenceID      int    `json:"sequenceId"`
	EdgeDescription string `json:"edgeDescription,omitempty"`
	Released        bool   `json:"released"`
}

type AGVPosition struct {
	X                   float64  `json:"x"`
	Y                   float64  `json:"y"`
	Theta               float64  `json:"theta"`
	MapID               string   `json:"mapId"`
	MapDescription      string   `json:"mapDescription,omitempty"`
	PositionInitialized bool     `json:"positionInitialized"`
	LocalizationScore   *float64 `json:"localizationScore,omitempty"`
	DeviationRange      *float64 `json:"deviationRange,omitempty"`
}

type Velocity struct {
	VX    *float64 `json:"vx,omitempty"`
	VY    *float64 `json:"vy,omitempty"`
	Omega *float64 `json:"omega,omitempty"`
}

type LoadDimensions struct {
	Length float64  `json:"length"`
	Width  float64  `json:"width"`
	Height *float64 `json:"height,omitempty"`
}

type Load struct {
	LoadID         string          `json:"loadId,omitempty"`
	LoadType       string          `json:"loadType,omitempty"`
	LoadPosition   string          `json:"loadPosition,omitempty"`
	LoadDimensions *LoadDimensions `json:"loadDimensions,omitempty"`
	Weight         *float64        `json:"weight,omitempty"`
}

type ActionStatus string

const (
	StatusWaiting      ActionStatus = "WAITING"
	StatusInitializing ActionStatus = "INITIALIZING"
	StatusRunning      ActionStatus = "RUNNING"
	StatusPaused       ActionStatus = "PAUSED"
	StatusFinished     ActionStatus = "FINISHED"
	StatusFailed       ActionStatus = "FAILED"
)

type ActionState struct {
	ActionID          string       `json:"actionId"`
	ActionType        string       `json:"actionType,omitempty"`
	ActionDescription string       `json:"actionDescription,omitempty"`
	ActionStatus      ActionStatus `json:"actionStatus"`
	ResultDescription string       `json:"resultDescription,omitempty"`
}

type BatteryState struct {
	BatteryCharge  float64  `json:"batteryCharge"`
	BatteryVoltage *float64 `json:"batteryVoltage,omitempty"`
	BatteryHealth  *float64 `json:"batteryHealth,omitempty"`
	Charging       bool     `json:"charging"`
	Reach          *float64 `json:"reach,omitempty"`
}

type ErrorReference struct {
	ReferenceKey   string `json:"referenceKey"`
	ReferenceValue string `json:"referenceValue"`
}

type ErrorLevel string

const (
	ErrorWarning ErrorLevel = "WARNING"
	ErrorFatal   ErrorLevel = "FATAL"
)

type Error struct {
	ErrorType        string           `json:"errorType"`
	ErrorReferences  []ErrorReference `json:"errorReferences,omitempty"`
	ErrorDescription string           `json:"errorDescription,omitempty"`
	ErrorHint        string           `json:"errorHint,omitempty"`
	ErrorLevel       ErrorLevel       `json:"errorLevel"`
}

type Info struct {
	InfoType        string `json:"infoType"`
	InfoDescription string `json:"infoDescription,omitempty"`
	InfoLevel       string `json:"infoLevel"` // INFO | DEBUG
}

type EStop string

const (
	EStopAutoAck EStop = "AUTOACK"
	EStopManual  EStop = "MANUAL"
	EStopRemote  EStop = "REMOTE"
	EStopNone    EStop = "NONE"
)

type SafetyState struct {
	EStop          EStop `json:"eStop"`
	FieldViolation bool  `json:"fieldViolation"`
}

type State struct {
	Header
	OrderID               string        `json:"orderId"`
	OrderUpdateID         int           `json:"orderUpdateId"`
	ZoneSetID             string        `json:"zoneSetId,omitempty"`
	LastNodeID            string        `json:"lastNodeId"`
	LastNodeSequenceID    int           `json:"lastNodeSequenceId"`
	Driving               bool          `json:"driving"`
	Paused                *bool         `json:"paused,omitempty"`
	NewBaseRequest        *bool         `json:"newBaseRequest,omitempty"`
	DistanceSinceLastNode *float64      `json:"distanceSinceLastNode,omitempty"`
	OperatingMode         OperatingMode `json:"operatingMode"`
	NodeStates            []NodeState   `json:"nodeStates"`
	EdgeStates            []EdgeState   `json:"edgeStates"`
	AGVPosition           *AGVPosition  `json:"agvPosition,omitempty"`
	Velocity              *Velocity     `json:"velocity,omitempty"`
	Loads                 []Load        `json:"loads,omitempty"`
	ActionStates          []ActionState `json:"actionStates"`
	BatteryState          BatteryState  `json:"batteryState"`
	Errors                []Error       `json:"errors"`
	Information           []Info        `json:"information,omitempty"`
	SafetyState           SafetyState   `json:"safetyState"`
}

// ---------------------------------------------------------------------------
// Visualization / connection / factsheet

type Visualization struct {
	Header
	AGVPosition *AGVPosition `json:"agvPosition,omitempty"`
	Velocity    *Velocity    `json:"velocity,omitempty"`
}

type ConnectionState string

const (
	ConnOnline  ConnectionState = "ONLINE"
	ConnOffline ConnectionState = "OFFLINE"
	ConnBroken  ConnectionState = "CONNECTIONBROKEN"
)

type Connection struct {
	Header
	ConnectionState ConnectionState `json:"connectionState"`
}

type Factsheet struct {
	Header
	TypeSpecification  TypeSpecification  `json:"typeSpecification"`
	PhysicalParameters PhysicalParameters `json:"physicalParameters"`
	ProtocolLimits     map[string]any     `json:"protocolLimits"`
	ProtocolFeatures   map[string]any     `json:"protocolFeatures"`
	AGVGeometry        map[string]any     `json:"agvGeometry"`
	LoadSpecification  map[string]any     `json:"loadSpecification"`
}

type TypeSpecification struct {
	SeriesName        string   `json:"seriesName"`
	SeriesDescription string   `json:"seriesDescription,omitempty"`
	AGVKinematic      string   `json:"agvKinematic"` // DIFF | OMNI | THREEWHEEL
	AGVClass          string   `json:"agvClass"`     // FORKLIFT | CONVEYOR | TUGGER | CARRIER
	MaxLoadMass       float64  `json:"maxLoadMass"`
	LocalizationTypes []string `json:"localizationTypes"`
	NavigationTypes   []string `json:"navigationTypes"`
}

type PhysicalParameters struct {
	SpeedMin        float64 `json:"speedMin"`
	SpeedMax        float64 `json:"speedMax"`
	AccelerationMax float64 `json:"accelerationMax"`
	DecelerationMax float64 `json:"decelerationMax"`
	HeightMin       float64 `json:"heightMin"`
	HeightMax       float64 `json:"heightMax"`
	Width           float64 `json:"width"`
	Length          float64 `json:"length"`
}
