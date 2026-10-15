/* Typed fetch client for the Fleet Manager API (docs/api.md). */
import { useAuthStore } from "@/stores/auth";
import type {
  Alarm,
  ApiErrorBody,
  ApiKey,
  AuditEntry,
  BatterySettings,
  BlockedVehicle,
  ChargingStation,
  Deadlock,
  DispatcherSettings,
  ErrorCode,
  FleetMap,
  FleetMapSummary,
  Kpis,
  LoginResponse,
  MaintenancePlan,
  MaintenanceRecord,
  MaintenanceStatus,
  Mission,
  MissionCreate,
  MissionListQuery,
  Reservation,
  Role,
  RouteRequest,
  RouteResult,
  SimulationStartRequest,
  SimulationStatus,
  Site,
  SystemInfo,
  TelemetrySample,
  TimeseriesMetric,
  TimeseriesPoint,
  TrafficSettings,
  User,
  UserCreate,
  Vehicle,
  VehicleCommandRequest,
  VehicleModel,
  VehicleWithState,
  FaultType,
  ScenarioName,
} from "@/lib/types";

export class ApiError extends Error {
  readonly status: number;
  readonly code: ErrorCode | "network" | "unknown";

  constructor(status: number, code: ApiError["code"], message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

/** Resolve the API base URL: `NEXT_PUBLIC_API_URL` + `/api/v1`, or relative `/api/v1` when empty. */
export function apiBaseUrl(): string {
  const raw = process.env.NEXT_PUBLIC_API_URL;
  const root = raw === undefined ? "http://localhost:8080" : raw;
  return `${root.replace(/\/+$/, "")}/api/v1`;
}

export function wsUrl(): string {
  const raw = process.env.NEXT_PUBLIC_WS_URL;
  if (raw === undefined) return "ws://localhost:8080/ws";
  if (raw !== "") return raw;
  if (typeof window === "undefined") return "ws://localhost:8080/ws";
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/ws`;
}

type Query = Record<string, string | number | boolean | undefined | null>;

function buildQuery(q?: Query): string {
  if (!q) return "";
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) {
    if (v === undefined || v === null || v === "") continue;
    params.set(k, String(v));
  }
  const s = params.toString();
  return s ? `?${s}` : "";
}

let onUnauthorized: (() => void) | null = null;
/** Register the handler called on HTTP 401 (redirect to /login). */
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

interface RequestOptions {
  query?: Query;
  body?: unknown;
  rawBody?: BodyInit;
  headers?: Record<string, string>;
  /** Skip the 401 handler (login endpoint). */
  noAuthRedirect?: boolean;
}

async function request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  const token = useAuthStore.getState().token;
  const headers: Record<string, string> = { Accept: "application/json", ...opts.headers };
  if (token) headers.Authorization = `Bearer ${token}`;
  let body: BodyInit | undefined;
  if (opts.rawBody !== undefined) {
    body = opts.rawBody;
  } else if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }

  let res: Response;
  try {
    res = await fetch(`${apiBaseUrl()}${path}${buildQuery(opts.query)}`, { method, headers, body });
  } catch (e) {
    throw new ApiError(0, "network", e instanceof Error ? e.message : "Erreur réseau");
  }

  if (res.status === 401 && !opts.noAuthRedirect) {
    onUnauthorized?.();
  }

  if (!res.ok) {
    let errBody: Partial<ApiErrorBody> = {};
    try {
      errBody = (await res.json()) as Partial<ApiErrorBody>;
    } catch {
      /* non-JSON error */
    }
    throw new ApiError(res.status, errBody.code ?? "unknown", errBody.error ?? `${res.status} ${res.statusText}`);
  }

  if (res.status === 204) return undefined as T;
  const text = await res.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

const get = <T>(path: string, query?: Query) => request<T>("GET", path, { query });
const post = <T>(path: string, body?: unknown, query?: Query) => request<T>("POST", path, { body, query });
const put = <T>(path: string, body?: unknown) => request<T>("PUT", path, { body });
const del = <T = void>(path: string) => request<T>("DELETE", path);

export const api = {
  auth: {
    login: (username: string, password: string) =>
      request<LoginResponse>("POST", "/auth/login", { body: { username, password }, noAuthRedirect: true }),
    me: () => get<User>("/auth/me"),
    logout: () => post<void>("/auth/logout"),
  },

  sites: {
    list: () => get<Site[]>("/sites"),
    get: (id: string) => get<Site>(`/sites/${enc(id)}`),
    create: (site: Partial<Site>) => post<Site>("/sites", site),
    update: (id: string, site: Partial<Site>) => put<Site>(`/sites/${enc(id)}`, site),
    remove: (id: string) => del(`/sites/${enc(id)}`),
  },

  vehicleModels: {
    list: () => get<VehicleModel[]>("/vehicle-models"),
    update: (id: string, model: VehicleModel) => put<VehicleModel>(`/vehicle-models/${enc(id)}`, model),
  },

  vehicles: {
    list: (siteId?: string) => get<VehicleWithState[]>("/vehicles", { siteId }),
    get: (id: string) => get<VehicleWithState>(`/vehicles/${enc(id)}`),
    create: (vehicle: Partial<Vehicle>) => post<Vehicle>("/vehicles", vehicle),
    update: (id: string, vehicle: Partial<Vehicle>) => put<Vehicle>(`/vehicles/${enc(id)}`, vehicle),
    remove: (id: string) => del(`/vehicles/${enc(id)}`),
    command: (id: string, cmd: VehicleCommandRequest) =>
      post<{ accepted: boolean }>(`/vehicles/${enc(id)}/commands`, cmd),
    history: (id: string, from?: string, to?: string) =>
      get<TelemetrySample[]>(`/vehicles/${enc(id)}/history`, { from, to }),
  },

  maps: {
    list: (siteId?: string) => get<FleetMapSummary[]>("/maps", { siteId }),
    get: (id: string) => get<FleetMap>(`/maps/${enc(id)}`),
    active: (siteId?: string) => get<FleetMap>("/maps/active", { siteId }),
    create: (map: Partial<FleetMap>) => post<FleetMap>("/maps", map),
    update: (id: string, map: FleetMap) => put<FleetMap>(`/maps/${enc(id)}`, map),
    remove: (id: string) => del(`/maps/${enc(id)}`),
    activate: (id: string) => post<FleetMap | void>(`/maps/${enc(id)}/activate`),
    setZoneEnabled: (mapId: string, zoneId: string, enabled: boolean) =>
      post<void>(`/maps/${enc(mapId)}/zones/${enc(zoneId)}/enabled`, { enabled }),
    setEdgeEnabled: (mapId: string, edgeId: string, enabled: boolean) =>
      post<void>(`/maps/${enc(mapId)}/edges/${enc(edgeId)}/enabled`, { enabled }),
    route: (mapId: string, req: RouteRequest) => post<RouteResult>(`/maps/${enc(mapId)}/route`, req),
    exportMap: (id: string) => get<FleetMap>(`/maps/${enc(id)}/export`),
    importMap: (json: string) =>
      request<FleetMap>("POST", "/maps/import", { rawBody: json, headers: { "Content-Type": "application/json" } }),
  },

  missions: {
    list: (q: MissionListQuery = {}) => get<Mission[]>("/missions", { ...q }),
    get: (id: string) => get<Mission>(`/missions/${enc(id)}`),
    create: (m: MissionCreate) => post<Mission>("/missions", m),
    cancel: (id: string) => post<Mission | void>(`/missions/${enc(id)}/cancel`),
    suspend: (id: string) => post<Mission | void>(`/missions/${enc(id)}/suspend`),
    resume: (id: string) => post<Mission | void>(`/missions/${enc(id)}/resume`),
    reassign: (id: string, body: { vehicleId?: string; confirmLoadState?: boolean }) =>
      post<Mission | void>(`/missions/${enc(id)}/reassign`, body),
    setPriority: (id: string, priority: number) => post<Mission | void>(`/missions/${enc(id)}/priority`, { priority }),
    remove: (id: string) => del(`/missions/${enc(id)}`),
  },

  settings: {
    getDispatcher: () => get<DispatcherSettings>("/settings/dispatcher"),
    putDispatcher: (s: DispatcherSettings) => put<DispatcherSettings>("/settings/dispatcher", s),
    getTraffic: () => get<TrafficSettings>("/settings/traffic"),
    putTraffic: (s: TrafficSettings) => put<TrafficSettings>("/settings/traffic", s),
    getBattery: () => get<BatterySettings>("/settings/battery"),
    putBattery: (s: BatterySettings) => put<BatterySettings>("/settings/battery", s),
  },

  traffic: {
    reservations: (siteId?: string) => get<Reservation[]>("/traffic/reservations", { siteId }),
    deadlocks: (siteId?: string) => get<Deadlock[]>("/traffic/deadlocks", { siteId }),
    blocked: () => get<Array<BlockedVehicle | string>>("/traffic/blocked"),
    release: (reservationId: string) =>
      post<void>(`/traffic/reservations/${enc(reservationId)}/release`, { confirmPhysicalState: true }),
  },

  charging: {
    stations: (siteId?: string) => get<ChargingStation[]>("/charging/stations", { siteId }),
  },

  alarms: {
    list: (q: { siteId?: string; active?: boolean; limit?: number } = {}) => get<Alarm[]>("/alarms", { ...q }),
    ack: (id: string) => post<Alarm | void>(`/alarms/${enc(id)}/ack`),
    clear: (id: string) => post<Alarm | void>(`/alarms/${enc(id)}/clear`),
  },

  maintenance: {
    plans: () => get<MaintenancePlan[]>("/maintenance/plans"),
    createPlan: (p: Partial<MaintenancePlan>) => post<MaintenancePlan>("/maintenance/plans", p),
    updatePlan: (id: string, p: Partial<MaintenancePlan>) => put<MaintenancePlan>(`/maintenance/plans/${enc(id)}`, p),
    removePlan: (id: string) => del(`/maintenance/plans/${enc(id)}`),
    status: (siteId?: string) => get<MaintenanceStatus[]>("/maintenance/status", { siteId }),
    record: (r: { vehicleId: string; planId: string; notes: string }) =>
      post<MaintenanceRecord>("/maintenance/records", r),
    records: (vehicleId?: string) => get<MaintenanceRecord[]>("/maintenance/records", { vehicleId }),
  },

  analytics: {
    kpis: (siteId?: string, from?: string, to?: string) => get<Kpis>("/analytics/kpis", { siteId, from, to }),
    timeseries: (q: { siteId?: string; metric: TimeseriesMetric; from?: string; to?: string; bucket?: string }) =>
      get<TimeseriesPoint[]>("/analytics/timeseries", { ...q }),
  },

  simulation: {
    status: () => get<SimulationStatus>("/simulation"),
    start: (req: SimulationStartRequest) => post<SimulationStatus | void>("/simulation/start", req),
    stop: () => post<void>("/simulation/stop"),
    timescale: (timeScale: number) => post<void>("/simulation/timescale", { timeScale }),
    spawn: (modelId: string, nodeId?: string) => post<void>("/simulation/vehicles", { modelId, nodeId }),
    fault: (vehicleId: string, type: FaultType) => post<void>("/simulation/faults", { vehicleId, type }),
    scenario: (name: ScenarioName, missionRatePerMin: number) =>
      post<void>("/simulation/scenario", { name, missionRatePerMin }),
  },

  users: {
    list: () => get<User[]>("/users"),
    create: (u: UserCreate) => post<User>("/users", u),
    update: (id: string, u: Partial<UserCreate>) => put<User>(`/users/${enc(id)}`, u),
    remove: (id: string) => del(`/users/${enc(id)}`),
    setPassword: (id: string, password: string) => post<void>(`/users/${enc(id)}/password`, { password }),
  },

  apiKeys: {
    list: () => get<ApiKey[]>("/api-keys"),
    create: (body: { name: string; role: Role }) => post<ApiKey>("/api-keys", body),
    remove: (id: string) => del(`/api-keys/${enc(id)}`),
  },

  audit: {
    list: (q: { limit?: number; user?: string; action?: string } = {}) => get<AuditEntry[]>("/audit", { ...q }),
  },

  system: {
    info: () => get<SystemInfo>("/system/info"),
    health: () => get<{ status: string }>("/health"),
  },
};

function enc(s: string): string {
  return encodeURIComponent(s);
}

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return "Erreur inconnue";
}
