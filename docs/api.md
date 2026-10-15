# Fleet Manager — API contract (v1)

Base URL: `http://<host>:8080/api/v1`. All payloads are JSON, field names are camelCase.
Timestamps are RFC 3339 UTC strings. Distances are metres, angles are radians (`theta`),
speeds m/s, masses kg, energy kWh, battery state of charge in percent (0–100).

## Authentication

| Method | Path | Body | Response |
|---|---|---|---|
| POST | `/auth/login` | `{"username","password"}` | `{"token","user":User,"expiresAt"}` |
| GET | `/auth/me` | — | `User` |
| POST | `/auth/logout` | — | `204` |

Send `Authorization: Bearer <token>` on every other call. External systems may instead send
`X-API-Key: <key>` (integration role). Roles: `admin`, `supervisor`, `operator`, `viewer`,
`integration`.

Role rights (summary): viewer = read everything; operator = + create/cancel missions, ack alarms,
vehicle pause/resume; supervisor = + vehicles CRUD, maps, simulation, maintenance; admin = + users,
sites, api keys, settings; integration = create/read missions and read fleet state only.

`User = {"id","username","displayName","role","siteIds":[],"createdAt","active":true}`

Default bootstrap account: `admin` / value of `FM_ADMIN_PASSWORD` (default `admin`, change it).

Errors: `{"error":"message","code":"not_found|validation|conflict|unauthorized|forbidden|internal"}`
with the matching HTTP status.

## Sites

`Site = {"id","name","description","timezone","createdAt"}`

GET/POST `/sites`, GET/PUT/DELETE `/sites/{id}`. Every vehicle, map and mission carries a `siteId`.
A default site `site-default` exists.

## Vehicle models

```
VehicleModel = {
  "id": "AGV20T", "name": "AGV 20T",
  "lengthM": 6.0, "widthM": 2.4, "heightM": 0.25,
  "maxPayloadKg": 20000,
  "maxSpeedMs": 1.0, "maxLateralSpeedMs": 0.5, "maxRotationRadS": 0.2, "accelMs2": 0.2,
  "batteryCapacityKwh": 40,
  "bogieCount": 4, "motorsPerBogie": 2, "wheelDiameterMm": 200, "gearRatio": 40,
  "encoderResolutionDeg": 0.5,
  "energyKwhPerM": 0.0005, "energyKwhPerTonM": 0.00002
}
```
GET `/vehicle-models`, PUT `/vehicle-models/{id}` (supervisor). `AGV20T` and `AGV40T` are built in.

## Vehicles

```
Vehicle = {
  "id","siteId","name","modelId","manufacturer","serialNumber",
  "protocol": "vda5050" | "simulated",
  "enabled": true,
  "safetyMarginM": 0.3, "localizationUncertaintyM": 0.1,
  "footprint": {"lengthM","widthM"} | null,       // optional override of model geometry
  "tags": ["string"], "createdAt","updatedAt"
}
VehicleState = {
  "vehicleId",
  "connection": "online" | "offline" | "connectionBroken" | "unknown",
  "lastSeen": ts | null,
  "pose": {"x","y","theta"}, "localizationScore": 0..1, "positionInitialized": bool,
  "velocity": {"vx","vy","omega"},
  "driving": bool, "paused": bool,
  "operatingMode": "automatic" | "semiautomatic" | "manual" | "service" | "teachin",
  "batteryCharge": 0..100, "batteryVoltage": number, "charging": bool, "batteryHealth": 0..100,
  "missionId": string, "orderId": string, "lastNodeId": string, "lastNodeSequenceId": int,
  "loadPresent": bool, "loadMassKg": number, "loads": [{"loadId","loadType","weightKg"}],
  "errors": [{"errorType","errorLevel":"warning"|"fatal","errorDescription","errorHint"}],
  "safetyState": {"eStop": "none"|"manual"|"autoAck"|"remote", "fieldViolation": bool},
  "availability": "available"|"busy"|"charging"|"maintenance"|"error"|"offline"|"disabled"|"blocked",
  "odometerM": number, "operatingHours": number, "missionsCompleted": int,
  "loadEnvelope": {"lengthM","widthM"},   // effective envelope incl. load + margins
  "updatedAt": ts
}
```

| Method | Path | Notes |
|---|---|---|
| GET | `/vehicles?siteId=` | `[{"vehicle":Vehicle,"state":VehicleState}]` |
| POST | `/vehicles` | body `Vehicle` (id optional) |
| GET | `/vehicles/{id}` | `{"vehicle","state"}` |
| PUT | `/vehicles/{id}` | update configuration |
| DELETE | `/vehicles/{id}` | only when no active mission |
| POST | `/vehicles/{id}/commands` | `{"command": "pause"|"resume"|"cancelMission"|"enable"|"disable"|"sendToCharge"|"sendToNode"|"clearErrors"|"maintenance"|"endMaintenance", "nodeId"?: string}` → `202 {"accepted":true}` |
| GET | `/vehicles/{id}/history?from=&to=` | telemetry samples `[{"ts","x","y","theta","batteryCharge","speed","missionId"}]` |

## Maps

```
Map = {
  "id","siteId","name","version": int, "active": bool,
  "widthM","heightM","originX","originY",
  "nodes": [Node], "edges": [Edge], "zones": [Zone],
  "createdAt","updatedAt"
}
Node = {"id","name","x","y","type": "waypoint"|"station"|"pickup"|"dropoff"|"charging"|"parking"|"waiting",
        "theta": number|null, "chargerPowerKw": number|null, "capacity": 1}
Edge = {"id","from","to","bidirectional": bool, "maxSpeedMs", "motion": "forward"|"lateral"|"any",
        "costFactor": 1.0, "widthM": 3.0, "enabled": true}
Zone = {"id","name","type": "exclusive"|"singleLane"|"bidirectional"|"crossing"|"waiting"|"forbidden"|"speedLimit"|"charging",
        "polygon": [{"x","y"}], "capacity": 1, "enabled": true, "maxSpeedMs": number|null}
```

| Method | Path | Notes |
|---|---|---|
| GET | `/maps?siteId=` | summaries (no nodes/edges) |
| POST | `/maps` | full Map |
| GET | `/maps/{id}` | full Map |
| PUT | `/maps/{id}` | replaces content, bumps `version` |
| DELETE | `/maps/{id}` | not when active |
| POST | `/maps/{id}/activate` | makes the map active for its site |
| GET | `/maps/active?siteId=` | active full map |
| POST | `/maps/{id}/zones/{zoneId}/enabled` | `{"enabled": bool}` temporarily (un)available zone |
| POST | `/maps/{id}/edges/{edgeId}/enabled` | `{"enabled": bool}` |
| POST | `/maps/{id}/route` | `{"from","to","vehicleId"?}` → `{"nodeIds":[],"edgeIds":[],"lengthM","costs":{...}}` |
| GET | `/maps/{id}/export` | map file (same JSON) |
| POST | `/maps/import` | map JSON file (multipart `file` or raw body) |

## Missions

```
Mission = {
  "id","siteId","name",
  "type": "transport"|"multiStop"|"machineSupply"|"productionEvacuation"|"transfer"|"returnToStation"|"charge"|"manual"|"external",
  "priority": 1..10 (10 = highest),
  "status": "created"|"pending"|"planned"|"assigned"|"executing"|"suspended"|"completed"|"failed"|"cancelled",
  "origin": nodeId, "destination": nodeId,
  "steps": [{"seq", "nodeId", "action": "move"|"pick"|"drop"|"wait"|"charge", "durationS": number,
             "status": "pending"|"active"|"done"|"skipped", "startedAt","completedAt"}],
  "load": {"present": bool, "massKg", "lengthM","widthM","heightM","description"},
  "vehicleId": string, "requiredModelId": string,
  "source": "operator"|"plc"|"erp"|"wms"|"mes"|"system"|"api",
  "dependencies": ["missionId"],
  "constraints": {"avoidZoneIds": [], "maxSpeedMs": number|null},
  "deadline": ts|null, "createdAt","assignedAt","startedAt","completedAt",
  "routeNodeIds": [], "progress": 0..1,
  "assignment": AssignmentExplanation | null,
  "history": [{"at","status","message"}],
  "errorMessage": string,
  "externalRef": string
}
AssignmentExplanation = {
  "chosenVehicleId", "weights": {...},
  "candidates": [{"vehicleId","eligible": bool, "reason": string, "score": number,
                  "factors": {"distanceM","travelTimeS","battery","congestion","queue","priority"}}]
}
```

| Method | Path | Notes |
|---|---|---|
| GET | `/missions?siteId=&status=&vehicleId=&limit=` | newest first |
| POST | `/missions` | create → status `pending` (or `created` when dependencies are open) |
| GET | `/missions/{id}` | |
| POST | `/missions/{id}/cancel` | |
| POST | `/missions/{id}/suspend` | |
| POST | `/missions/{id}/resume` | |
| POST | `/missions/{id}/reassign` | `{"vehicleId"?: string}` controlled reassignment; refused when a load is possibly on board and its physical state is unverified unless `{"confirmLoadState": true}` |
| POST | `/missions/{id}/priority` | `{"priority": int}` |
| DELETE | `/missions/{id}` | only terminal missions |

Dispatcher configuration: GET/PUT `/settings/dispatcher` →
`{"mode": "central"|"colony", "weights":{"distance","travelTime","battery","congestion","queue"},"minBatteryPct","reserveBatteryPct","lookaheadSegments"}`.
`mode: "colony"` switches to the ant-colony allocation: open missions are published on the shared board and the
autonomous on-board agents claim them (see `docs/colony.md`); the weights are then only used for the central
eligibility check.

## Colony board

GET `/colony/board?siteId=` →
```
{"mode": "central"|"colony",
 "tasks": [{"id","type","origin","destination","nodes":[],"priority","urgency","massKg","loadPresent","requiredModelId","createdAt"}],
 "agents": [{"vehicleId","battery","node","assignment":{"taskId","status":"idle"|"assigned"|"executing"},"claims","wins",
             "decisions":[{"taskId","cost","urgency","delayS","reason","ok"}],
             "yielded","passed","negotiations":[{"peer","resource","myCost","peerCost","iYield","at"}]}],   // in-process agents (simulated vehicles)
 "stats": {"granted","rejected","negotiated","recent":[{"claimId","taskId","vehicleId","granted","reason","competing"}],
           "yields":[{"nonce","vehicleId","beneficiary","resource":{"type","id"},"myCost","peerCost","reason","at"}]}}
```

## Traffic

```
Reservation = {"id","vehicleId","resource": {"type": "node"|"edge"|"zone", "id"}, "state": "requested"|"granted"|"active"|"released"|"uncertain",
               "sequence": int, "requestedAt","grantedAt","releasedAt","expiresAt"}
Deadlock = {"id","vehicleIds": [], "resourceIds": [], "detectedAt", "resolution": string, "resolvedAt"}
```

GET `/traffic/reservations?siteId=`, GET `/traffic/deadlocks?siteId=`, GET `/traffic/blocked`,
POST `/traffic/reservations/{id}/release` `{"confirmPhysicalState": true}` (supervisor, for uncertain reservations only),
GET/PUT `/settings/traffic` → `{"lookaheadSegments","blockedTimeoutS","commsTimeoutS","swept":{"samples"}}`.

## Battery / charging

GET `/charging/stations?siteId=` → `[{"nodeId","name","powerKw","occupiedBy": vehicleId|null, "reservedBy": vehicleId|null}]`
GET/PUT `/settings/battery` → `{"lowPct","criticalPct","targetPct","opportunityPct","idleBeforeChargeS"}`

## Alarms

```
Alarm = {"id","siteId","severity": "info"|"warning"|"error"|"critical", "code", "source": "vehicle"|"system"|"traffic"|"battery"|"comms"|"maintenance"|"mission",
         "vehicleId", "missionId", "message", "active": bool, "raisedAt","acknowledgedAt","acknowledgedBy","clearedAt"}
```
GET `/alarms?siteId=&active=true&limit=`, POST `/alarms/{id}/ack`, POST `/alarms/{id}/clear` (system alarms only).

## Maintenance

```
MaintenancePlan = {"id","vehicleId"|null (null = all), "modelId"|null, "name", "metric": "odometerM"|"operatingHours"|"missions"|"calendarDays", "intervalValue", "lastDoneValue", "lastDoneAt", "dueSoonFraction": 0.9}
MaintenanceStatus = {"planId","vehicleId","currentValue","nextDueValue","remaining","state": "ok"|"dueSoon"|"overdue"}
MaintenanceRecord = {"id","vehicleId","planId","doneAt","doneBy","notes"}
```
GET/POST `/maintenance/plans`, PUT/DELETE `/maintenance/plans/{id}`, GET `/maintenance/status?siteId=`,
POST `/maintenance/records` `{"vehicleId","planId","notes"}` resets the counter, GET `/maintenance/records?vehicleId=`.

## Analytics

GET `/analytics/kpis?siteId=&from=&to=` →
```
{"missionsCompleted","missionsFailed","missionsPerHour","avgMissionDurationS","avgWaitTimeS",
 "fleetUtilizationPct","distanceM","activeAlarms","deadlocksResolved","energyKwh",
 "vehicles":[{"vehicleId","utilizationPct","missions","distanceM","batteryChargePct","alarms"}]}
```
GET `/analytics/timeseries?siteId=&metric=missionsPerHour|utilization|activeVehicles|alarms&from=&to=&bucket=5m` → `[{"ts","value"}]`

## Simulation

```
SimulationStatus = {"running": bool, "timeScale": number, "simTime": ts, "mapId", "vehicleIds": [], "scenario": {"name","missionRatePerMin"}|null, "faults": [{"vehicleId","type"}]}
```
| Method | Path | Body |
|---|---|---|
| GET | `/simulation` | status |
| POST | `/simulation/start` | `{"siteId","mapId"?, "vehicles": 5, "modelMix": {"AGV20T": 0.5, "AGV40T": 0.5}, "timeScale": 1.0, "scenario": {"name": "randomTransport", "missionRatePerMin": 2}}` |
| POST | `/simulation/stop` | — |
| POST | `/simulation/timescale` | `{"timeScale"}` |
| POST | `/simulation/vehicles` | `{"modelId","nodeId"?}` spawns one more |
| POST | `/simulation/faults` | `{"vehicleId","type": "commsLoss"|"blocked"|"error"|"estop"|"restore"}` |
| POST | `/simulation/scenario` | `{"name": "randomTransport"|"none", "missionRatePerMin"}` |

Simulated vehicles speak VDA 5050 over the (embedded) MQTT broker — the same path as real AGVs.

## Users, API keys, audit, settings

GET/POST `/users`, PUT/DELETE `/users/{id}`, POST `/users/{id}/password` `{"password"}`.
GET/POST `/api-keys` → `{"id","name","role","key"(only on create),"createdAt"}`, DELETE `/api-keys/{id}`.
GET `/audit?limit=&user=&action=` → `[{"id","at","userId","username","action","target","details","ip"}]`.
GET `/system/info` → `{"version","uptimeS","store":"memory"|"postgres","mqtt":{"embedded":bool,"url"},"vehiclesOnline","time"}`.
GET `/health` (no auth) → `{"status":"ok"}`.

## Integration gateway

`X-API-Key` protected. POST `/integration/missions` same body as `/missions` with `source` forced to the key's
configured source, `externalRef` recommended. GET `/integration/missions/{externalRef}`.
Webhooks: GET/POST `/integration/webhooks` `{"id","url","events":["mission.completed","mission.failed","alarm.raised"],"secret"}`.
MQTT topics (broker): `fleet/<siteId>/events` (JSON events, published), `fleet/<siteId>/missions/request` (JSON Mission, consumed).
OPC UA: see `docs/integration.md` (adapter interface; server exposure is roadmap item).

## WebSocket

`GET /ws?token=<jwt>` (also accepts the Authorization header). Server pushes:

```
{"type": "snapshot", "ts", "data": {
   "vehicles": [{"vehicle":Vehicle,"state":VehicleState}],
   "missions": [Mission]            // non-terminal missions
   "reservations": [Reservation],
   "alarms": [Alarm],               // active
   "kpis": {...},                   // same shape as /analytics/kpis (without vehicles)
   "simulation": SimulationStatus
}}
{"type": "event", "ts", "event": {"kind": "mission.created|mission.updated|mission.completed|mission.failed|alarm.raised|alarm.cleared|vehicle.connection|deadlock.detected|deadlock.resolved|map.activated", "siteId", "vehicleId"?, "missionId"?, "alarmId"?, "message"}}
```
Snapshots are sent at ~4 Hz while a client is connected; the client may send `{"type":"subscribe","siteId":"..."}`
to filter by site. Everything the client needs to draw the live map is in the snapshot plus the active map
(fetched once via `/maps/active`, refetched on `map.activated`).
