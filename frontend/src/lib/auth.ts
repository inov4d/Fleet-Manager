import type { Role, User } from "@/lib/types";

/** UI permissions mirroring the role summary of the API contract. */
export type Permission =
  | "missions.create"
  | "missions.cancel"
  | "missions.manage" // suspend/resume/reassign/priority
  | "alarms.ack"
  | "alarms.clear"
  | "vehicles.command" // pause / resume
  | "vehicles.command.all" // sendToCharge, enable/disable, maintenance...
  | "vehicles.manage" // CRUD
  | "maps.manage"
  | "simulation.manage"
  | "maintenance.manage"
  | "traffic.release"
  | "settings.dispatcher"
  | "settings.battery"
  | "settings.traffic"
  | "users.manage"
  | "sites.manage"
  | "apikeys.manage"
  | "audit.read";

const OPERATOR: Permission[] = ["missions.create", "missions.cancel", "missions.manage", "alarms.ack", "vehicles.command"];
const SUPERVISOR: Permission[] = [
  ...OPERATOR,
  "alarms.clear",
  "vehicles.command.all",
  "vehicles.manage",
  "maps.manage",
  "simulation.manage",
  "maintenance.manage",
  "traffic.release",
];
const ADMIN: Permission[] = [
  ...SUPERVISOR,
  "settings.dispatcher",
  "settings.battery",
  "settings.traffic",
  "users.manage",
  "sites.manage",
  "apikeys.manage",
  "audit.read",
];

const ROLE_PERMISSIONS: Record<Role, ReadonlySet<Permission>> = {
  viewer: new Set(),
  operator: new Set(OPERATOR),
  supervisor: new Set(SUPERVISOR),
  admin: new Set(ADMIN),
  integration: new Set<Permission>(["missions.create"]),
};

export function can(user: User | null | undefined, permission: Permission): boolean {
  if (!user) return false;
  return ROLE_PERMISSIONS[user.role]?.has(permission) ?? false;
}

export function canAny(user: User | null | undefined, permissions: Permission[]): boolean {
  return permissions.some((p) => can(user, p));
}

export const ROLE_LABELS: Record<Role, string> = {
  admin: "Administrateur",
  supervisor: "Superviseur",
  operator: "Opérateur",
  viewer: "Lecteur",
  integration: "Intégration",
};
