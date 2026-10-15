/* French labels for the contract enumerations. */
import type {
  AlarmSeverity,
  AlarmSource,
  Availability,
  EdgeMotion,
  EventKind,
  FaultType,
  MaintenanceMetric,
  MaintenanceState,
  MissionSource,
  MissionStatus,
  MissionType,
  NodeType,
  OperatingMode,
  ReservationState,
  StepAction,
  VehicleConnection,
  ZoneType,
} from "@/lib/types";

export const AVAILABILITY_LABELS: Record<Availability, string> = {
  available: "Disponible",
  busy: "Occupé",
  charging: "En charge",
  maintenance: "Maintenance",
  error: "Erreur",
  offline: "Hors ligne",
  disabled: "Désactivé",
  blocked: "Bloqué",
};

/** Hex colors for the live map (shared with badges). */
export const AVAILABILITY_COLORS: Record<Availability, number> = {
  available: 0x22c55e,
  busy: 0x3b82f6,
  charging: 0xeab308,
  maintenance: 0xa78bfa,
  error: 0xef4444,
  offline: 0x6b7280,
  disabled: 0x4b5563,
  blocked: 0xf97316,
};

export const CONNECTION_LABELS: Record<VehicleConnection, string> = {
  online: "En ligne",
  offline: "Hors ligne",
  connectionBroken: "Connexion perdue",
  unknown: "Inconnu",
};

export const OPERATING_MODE_LABELS: Record<OperatingMode, string> = {
  automatic: "Automatique",
  semiautomatic: "Semi-automatique",
  manual: "Manuel",
  service: "Service",
  teachin: "Apprentissage",
};

export const MISSION_STATUS_LABELS: Record<MissionStatus, string> = {
  created: "Créée",
  pending: "En attente",
  planned: "Planifiée",
  assigned: "Affectée",
  executing: "En cours",
  suspended: "Suspendue",
  completed: "Terminée",
  failed: "Échouée",
  cancelled: "Annulée",
};

export const MISSION_TYPE_LABELS: Record<MissionType, string> = {
  transport: "Transport",
  multiStop: "Multi-étapes",
  machineSupply: "Approvisionnement machine",
  productionEvacuation: "Évacuation production",
  transfer: "Transfert",
  returnToStation: "Retour station",
  charge: "Charge",
  manual: "Manuelle",
  external: "Externe",
};

export const MISSION_SOURCE_LABELS: Record<MissionSource, string> = {
  operator: "Opérateur",
  plc: "Automate",
  erp: "ERP",
  wms: "WMS",
  mes: "MES",
  system: "Système",
  api: "API",
};

export const STEP_ACTION_LABELS: Record<StepAction, string> = {
  move: "Déplacement",
  pick: "Prise",
  drop: "Dépose",
  wait: "Attente",
  charge: "Charge",
};

export const NODE_TYPE_LABELS: Record<NodeType, string> = {
  waypoint: "Point de passage",
  station: "Station",
  pickup: "Prise",
  dropoff: "Dépose",
  charging: "Charge",
  parking: "Parking",
  waiting: "Attente",
};

export const NODE_TYPE_COLORS: Record<NodeType, number> = {
  waypoint: 0x94a3b8,
  station: 0x3b82f6,
  pickup: 0x22c55e,
  dropoff: 0xf59e0b,
  charging: 0xeab308,
  parking: 0xa78bfa,
  waiting: 0x38bdf8,
};

export const EDGE_MOTION_LABELS: Record<EdgeMotion, string> = {
  forward: "Marche avant",
  lateral: "Latéral",
  any: "Libre",
};

export const ZONE_TYPE_LABELS: Record<ZoneType, string> = {
  exclusive: "Exclusive",
  singleLane: "Voie unique",
  bidirectional: "Bidirectionnelle",
  crossing: "Croisement",
  waiting: "Attente",
  forbidden: "Interdite",
  speedLimit: "Vitesse limitée",
  charging: "Charge",
};

export const ZONE_TYPE_COLORS: Record<ZoneType, number> = {
  exclusive: 0xf97316,
  singleLane: 0xeab308,
  bidirectional: 0x38bdf8,
  crossing: 0xa78bfa,
  waiting: 0x22c55e,
  forbidden: 0xef4444,
  speedLimit: 0xf59e0b,
  charging: 0x84cc16,
};

export const RESERVATION_STATE_LABELS: Record<ReservationState, string> = {
  requested: "Demandée",
  granted: "Accordée",
  active: "Active",
  released: "Libérée",
  uncertain: "Incertaine",
};

export const ALARM_SEVERITY_LABELS: Record<AlarmSeverity, string> = {
  info: "Info",
  warning: "Avertissement",
  error: "Erreur",
  critical: "Critique",
};

export const ALARM_SOURCE_LABELS: Record<AlarmSource, string> = {
  vehicle: "Véhicule",
  system: "Système",
  traffic: "Trafic",
  battery: "Batterie",
  comms: "Communication",
  maintenance: "Maintenance",
  mission: "Mission",
};

export const MAINTENANCE_METRIC_LABELS: Record<MaintenanceMetric, string> = {
  odometerM: "Odomètre (m)",
  operatingHours: "Heures de fonctionnement",
  missions: "Missions",
  calendarDays: "Jours calendaires",
};

export const MAINTENANCE_STATE_LABELS: Record<MaintenanceState, string> = {
  ok: "OK",
  dueSoon: "Bientôt",
  overdue: "En retard",
};

export const FAULT_LABELS: Record<FaultType, string> = {
  commsLoss: "Perte de communication",
  blocked: "Blocage",
  error: "Erreur",
  estop: "Arrêt d’urgence",
  restore: "Rétablir",
};

export const EVENT_KIND_LABELS: Record<EventKind, string> = {
  "mission.created": "Mission créée",
  "mission.updated": "Mission mise à jour",
  "mission.completed": "Mission terminée",
  "mission.failed": "Mission échouée",
  "alarm.raised": "Alarme levée",
  "alarm.cleared": "Alarme effacée",
  "vehicle.connection": "Connexion véhicule",
  "deadlock.detected": "Interblocage détecté",
  "deadlock.resolved": "Interblocage résolu",
  "map.activated": "Carte activée",
};

export function label<K extends string>(map: Record<K, string>, key: K | string | null | undefined): string {
  if (!key) return "—";
  return (map as Record<string, string>)[key] ?? key;
}
