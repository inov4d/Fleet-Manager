/* TypeScript types for every object of docs/api.md (API contract v1). */

export type Role = "admin" | "supervisor" | "operator" | "viewer" | "integration";

export interface User {
  id: string;
  username: string;
  displayName: string;
  role: Role;
  siteIds: string[];
  createdAt: string;
  active: boolean;
}

export interface LoginResponse {
  token: string;
  user: User;
  expiresAt: string;
}

export type ErrorCode = "not_found" | "validation" | "conflict" | "unauthorized" | "forbidden" | "internal";

export interface ApiErrorBody {
  error: string;
  code: ErrorCode;
}

export interface Site {
  id: string;
  name: string;
  description: string;
  timezone: string;
  createdAt: string;
}

export interface VehicleModel {
  id: string;
  name: string;
  lengthM: number;
  widthM: number;
  heightM: number;
  maxPayloadKg: number;
  maxSpeedMs: number;
  maxLateralSpeedMs: number;
  maxRotationRadS: number;
  accelMs2: number;
  batteryCapacityKwh: number;
  bogieCount: number;
  motorsPerBogie: number;
  wheelDiameterMm: number;
  gearRatio: number;
  encoderResolutionDeg: number;
  energyKwhPerM: number;
  energyKwhPerTonM: number;
}

export type VehicleProtocol = "vda5050" | "simulated";

export interface Footprint {
  lengthM: number;
  widthM: number;
}

export interface Vehicle {
  id: string;
  siteId: string;
  name: string;
  modelId: string;
  manufacturer: string;
  serialNumber: string;
  protocol: VehicleProtocol;
  enabled: boolean;
  safetyMarginM: number;
  localizationUncertaintyM: number;
  footprint: Footprint | null;
  tags: string[];
  createdAt: string;
  updatedAt: string;
}

export type VehicleConnection = "online" | "offline" | "connectionBroken" | "unknown";
export type OperatingMode = "automatic" | "semiautomatic" | "manual" | "service" | "teachin";
export type Availability =
  | "available"
  | "busy"
  | "charging"
  | "maintenance"
  | "error"
  | "offline"
  | "disabled"
  | "blocked";
export type EStopState = "none" | "manual" | "autoAck" | "remote";

export interface Pose {
  x: number;
  y: number;
  theta: number;
}

export interface Velocity {
  vx: number;
  vy: number;
  omega: number;
}

export interface VehicleLoad {
  loadId: string;
  loadType: string;
  weightKg: number;
}

export interface VehicleError {
  errorType: string;
  errorLevel: "warning" | "fatal";
  errorDescription: string;
  errorHint: string;
}

export interface SafetyState {
  eStop: EStopState;
  fieldViolation: boolean;
}

export interface VehicleState {
  vehicleId: string;
  connection: VehicleConnection;
  lastSeen: string | null;
  pose: Pose;
  localizationScore: number;
  positionInitialized: boolean;
  velocity: Velocity;
  driving: boolean;
  paused: boolean;
  operatingMode: OperatingMode;
  batteryCharge: number;
  batteryVoltage: number;
  charging: boolean;
  batteryHealth: number;
  missionId: string;
  orderId: string;
  lastNodeId: string;
  lastNodeSequenceId: number;
  loadPresent: boolean;
  loadMassKg: number;
  loads: VehicleLoad[];
  errors: VehicleError[];
  safetyState: SafetyState;
  availability: Availability;
  odometerM: number;
  operatingHours: number;
  missionsCompleted: number;
  loadEnvelope: Footprint;
  updatedAt: string;
}

export interface VehicleWithState {
  vehicle: Vehicle;
  state: VehicleState;
}

export type VehicleCommand =
  | "pause"
  | "resume"
  | "cancelMission"
  | "enable"
  | "disable"
  | "sendToCharge"
  | "sendToNode"
  | "clearErrors"
  | "maintenance"
  | "endMaintenance";

export interface VehicleCommandRequest {
  command: VehicleCommand;
  nodeId?: string;
}

export interface TelemetrySample {
  ts: string;
  x: number;
  y: number;
  theta: number;
  batteryCharge: number;
  speed: number;
  missionId: string;
}

/* Maps */

export type NodeType = "waypoint" | "station" | "pickup" | "dropoff" | "charging" | "parking" | "waiting";

export interface MapNode {
  id: string;
  name: string;
  x: number;
  y: number;
  type: NodeType;
  theta: number | null;
  chargerPowerKw: number | null;
  capacity: number;
}

export type EdgeMotion = "forward" | "lateral" | "any";

export interface MapEdge {
  id: string;
  from: string;
  to: string;
  bidirectional: boolean;
  maxSpeedMs: number;
  motion: EdgeMotion;
  costFactor: number;
  widthM: number;
  enabled: boolean;
}

export type ZoneType =
  | "exclusive"
  | "singleLane"
  | "bidirectional"
  | "crossing"
  | "waiting"
  | "forbidden"
  | "speedLimit"
  | "charging";

export interface Point {
  x: number;
  y: number;
}

export interface MapZone {
  id: string;
  name: string;
  type: ZoneType;
  polygon: Point[];
  capacity: number;
  enabled: boolean;
  maxSpeedMs: number | null;
}

export interface FleetMap {
  id: string;
  siteId: string;
  name: string;
  version: number;
  active: boolean;
  widthM: number;
  heightM: number;
  originX: number;
  originY: number;
  nodes: MapNode[];
  edges: MapEdge[];
  zones: MapZone[];
  createdAt: string;
  updatedAt: string;
}

/** `GET /maps` returns summaries without nodes/edges/zones. */
export type FleetMapSummary = Omit<FleetMap, "nodes" | "edges" | "zones"> &
  Partial<Pick<FleetMap, "nodes" | "edges" | "zones">>;

export interface RouteRequest {
  from: string;
  to: string;
  vehicleId?: string;
}

export interface RouteResult {
  nodeIds: string[];
  edgeIds: string[];
  lengthM: number;
  costs: Record<string, number>;
}

/* Missions */

export type MissionType =
  | "transport"
  | "multiStop"
  | "machineSupply"
  | "productionEvacuation"
  | "transfer"
  | "returnToStation"
  | "charge"
  | "manual"
  | "external";

export type MissionStatus =
  | "created"
  | "pending"
  | "planned"
  | "assigned"
  | "executing"
  | "suspended"
  | "completed"
  | "failed"
  | "cancelled";

export const TERMINAL_MISSION_STATUSES: MissionStatus[] = ["completed", "failed", "cancelled"];

export type StepAction = "move" | "pick" | "drop" | "wait" | "charge";
export type StepStatus = "pending" | "active" | "done" | "skipped";

export interface MissionStep {
  seq: number;
  nodeId: string;
  action: StepAction;
  durationS: number;
  status: StepStatus;
  startedAt: string | null;
  completedAt: string | null;
}

export interface MissionLoad {
  present: boolean;
  massKg: number;
  lengthM: number;
  widthM: number;
  heightM: number;
  description: string;
}

export type MissionSource = "operator" | "plc" | "erp" | "wms" | "mes" | "system" | "api";

export interface MissionConstraints {
  avoidZoneIds: string[];
  maxSpeedMs: number | null;
}

export interface MissionHistoryEntry {
  at: string;
  status: MissionStatus | string;
  message: string;
}

export interface AssignmentFactors {
  distanceM: number;
  travelTimeS: number;
  battery: number;
  congestion: number;
  queue: number;
  priority: number;
}

export interface AssignmentCandidate {
  vehicleId: string;
  eligible: boolean;
  reason: string;
  score: number;
  factors: AssignmentFactors;
}

export interface AssignmentExplanation {
  chosenVehicleId: string;
  weights: Record<string, number>;
  candidates: AssignmentCandidate[];
}

export interface Mission {
  id: string;
  siteId: string;
  name: string;
  type: MissionType;
  priority: number;
  status: MissionStatus;
  origin: string;
  destination: string;
  steps: MissionStep[];
  load: MissionLoad;
  vehicleId: string;
  requiredModelId: string;
  source: MissionSource;
  dependencies: string[];
  constraints: MissionConstraints;
  deadline: string | null;
  createdAt: string;
  assignedAt: string | null;
  startedAt: string | null;
  completedAt: string | null;
  routeNodeIds: string[];
  progress: number;
  assignment: AssignmentExplanation | null;
  history: MissionHistoryEntry[];
  errorMessage: string;
  externalRef: string;
}

/** Body sent to POST /missions (server fills status, timestamps, assignment...). */
export interface MissionCreate {
  siteId: string;
  name: string;
  type: MissionType;
  priority: number;
  origin: string;
  destination: string;
  steps: Array<Pick<MissionStep, "seq" | "nodeId" | "action" | "durationS">>;
  load: MissionLoad;
  requiredModelId: string;
  source: MissionSource;
  dependencies: string[];
  constraints: MissionConstraints;
  deadline: string | null;
  externalRef?: string;
}

export interface MissionListQuery {
  siteId?: string;
  status?: MissionStatus | string;
  vehicleId?: string;
  limit?: number;
}

export type AllocationMode = "central" | "colony";

export interface DispatcherSettings {
  mode: AllocationMode;
  weights: {
    distance: number;
    travelTime: number;
    battery: number;
    congestion: number;
    queue: number;
  };
  minBatteryPct: number;
  reserveBatteryPct: number;
  lookaheadSegments: number;
}

/* Traffic */

export type ReservationState = "requested" | "granted" | "active" | "released" | "uncertain";
export type ResourceType = "node" | "edge" | "zone";

export interface Reservation {
  id: string;
  vehicleId: string;
  resource: { type: ResourceType; id: string };
  state: ReservationState;
  sequence: number;
  requestedAt: string | null;
  grantedAt: string | null;
  releasedAt: string | null;
  expiresAt: string | null;
}

export interface Deadlock {
  id: string;
  vehicleIds: string[];
  resourceIds: string[];
  detectedAt: string;
  resolution: string;
  resolvedAt: string | null;
}

export interface TrafficSettings {
  lookaheadSegments: number;
  blockedTimeoutS: number;
  commsTimeoutS: number;
  swept: { samples: number };
}

/** Shape of `/traffic/blocked` is not detailed in the contract; we assume a list of vehicle ids or objects. */
export interface BlockedVehicle {
  vehicleId: string;
  since?: string;
  reason?: string;
  resourceId?: string;
}

/* Battery / charging */

export interface ChargingStation {
  nodeId: string;
  name: string;
  powerKw: number;
  occupiedBy: string | null;
  reservedBy: string | null;
}

export interface BatterySettings {
  lowPct: number;
  criticalPct: number;
  targetPct: number;
  opportunityPct: number;
  idleBeforeChargeS: number;
}

/* Alarms */

export type AlarmSeverity = "info" | "warning" | "error" | "critical";
export type AlarmSource = "vehicle" | "system" | "traffic" | "battery" | "comms" | "maintenance" | "mission";

export interface Alarm {
  id: string;
  siteId: string;
  severity: AlarmSeverity;
  code: string;
  source: AlarmSource;
  vehicleId: string;
  missionId: string;
  message: string;
  active: boolean;
  raisedAt: string;
  acknowledgedAt: string | null;
  acknowledgedBy: string;
  clearedAt: string | null;
}

/* Maintenance */

export type MaintenanceMetric = "odometerM" | "operatingHours" | "missions" | "calendarDays";
export type MaintenanceState = "ok" | "dueSoon" | "overdue";

export interface MaintenancePlan {
  id: string;
  vehicleId: string | null;
  modelId: string | null;
  name: string;
  metric: MaintenanceMetric;
  intervalValue: number;
  lastDoneValue: number;
  lastDoneAt: string | null;
  dueSoonFraction: number;
}

export interface MaintenanceStatus {
  planId: string;
  vehicleId: string;
  currentValue: number;
  nextDueValue: number;
  remaining: number;
  state: MaintenanceState;
}

export interface MaintenanceRecord {
  id: string;
  vehicleId: string;
  planId: string;
  doneAt: string;
  doneBy: string;
  notes: string;
}

/* Analytics */

export interface VehicleKpi {
  vehicleId: string;
  utilizationPct: number;
  missions: number;
  distanceM: number;
  batteryChargePct: number;
  alarms: number;
}

export interface Kpis {
  missionsCompleted: number;
  missionsFailed: number;
  missionsPerHour: number;
  avgMissionDurationS: number;
  avgWaitTimeS: number;
  fleetUtilizationPct: number;
  distanceM: number;
  activeAlarms: number;
  deadlocksResolved: number;
  energyKwh: number;
  vehicles?: VehicleKpi[];
}

export type TimeseriesMetric = "missionsPerHour" | "utilization" | "activeVehicles" | "alarms";

export interface TimeseriesPoint {
  ts: string;
  value: number;
}

/* Simulation */

export type FaultType = "commsLoss" | "blocked" | "error" | "estop" | "restore";
export type ScenarioName = "randomTransport" | "none";

export interface SimulationScenario {
  name: string;
  missionRatePerMin: number;
}

export interface SimulationStatus {
  running: boolean;
  timeScale: number;
  simTime: string;
  mapId: string;
  vehicleIds: string[];
  scenario: SimulationScenario | null;
  faults: Array<{ vehicleId: string; type: FaultType }>;
}

export interface SimulationStartRequest {
  siteId: string;
  mapId?: string;
  vehicles: number;
  modelMix: Record<string, number>;
  timeScale: number;
  scenario: { name: ScenarioName; missionRatePerMin: number };
}

/* Users, API keys, audit, system */

export interface UserCreate {
  username: string;
  displayName: string;
  role: Role;
  siteIds: string[];
  password?: string;
  active?: boolean;
}

export interface ApiKey {
  id: string;
  name: string;
  role: Role;
  key?: string;
  createdAt: string;
}

export interface AuditEntry {
  id: string;
  at: string;
  userId: string;
  username: string;
  action: string;
  target: string;
  details: string;
  ip: string;
}

export interface SystemInfo {
  version: string;
  uptimeS: number;
  store: "memory" | "postgres";
  mqtt: { embedded: boolean; url: string };
  vehiclesOnline: number;
  time: string;
}

/* WebSocket */

export interface Snapshot {
  vehicles: VehicleWithState[];
  missions: Mission[];
  reservations: Reservation[];
  alarms: Alarm[];
  kpis: Kpis;
  simulation: SimulationStatus;
}

export type EventKind =
  | "mission.created"
  | "mission.updated"
  | "mission.completed"
  | "mission.failed"
  | "alarm.raised"
  | "alarm.cleared"
  | "vehicle.connection"
  | "deadlock.detected"
  | "deadlock.resolved"
  | "map.activated";

export interface FleetEvent {
  kind: EventKind;
  siteId: string;
  vehicleId?: string;
  missionId?: string;
  alarmId?: string;
  message: string;
}

export type WsMessage =
  | { type: "snapshot"; ts: string; data: Snapshot }
  | { type: "event"; ts: string; event: FleetEvent };

export interface StoredEvent extends FleetEvent {
  id: number;
  ts: string;
}

/* Colony (ant-colony task allocation) */

export interface ColonyTask {
  id: string;
  type: string;
  origin: string;
  destination: string;
  nodes: string[];
  priority: number;
  urgency: number;
  massKg: number;
  loadPresent: boolean;
  requiredModelId: string;
  createdAt: string;
}

export interface ColonyDecision {
  taskId: string;
  cost: number;
  urgency: number;
  delayS: number;
  reason: string;
  ok: boolean;
}

export interface ColonyNegotiation {
  peer: string;
  resource: string;
  myCost: number;
  peerCost: number;
  iYield: boolean;
  at: string;
}

export interface ColonyAgent {
  vehicleId: string;
  battery: number;
  node: string;
  assignment: { vehicleId: string; taskId: string; status: "idle" | "assigned" | "executing" | ""; ts: string };
  claims: number;
  wins: number;
  decisions: ColonyDecision[];
  yielded: number;
  passed: number;
  negotiations: ColonyNegotiation[];
}

export interface ColonyYield {
  nonce: string;
  vehicleId: string;
  beneficiary: string;
  resource: { type: string; id: string };
  myCost: number;
  peerCost: number;
  reason: string;
  at: string;
}

export interface ColonyGrant {
  claimId: string;
  taskId: string;
  vehicleId: string;
  granted: boolean;
  reason: string;
  competing: number;
}

export interface ColonyStatus {
  mode: AllocationMode;
  tasks: ColonyTask[];
  agents: ColonyAgent[];
  stats: { granted: number; rejected: number; negotiated: number; recent: ColonyGrant[]; yields: ColonyYield[] };
}
