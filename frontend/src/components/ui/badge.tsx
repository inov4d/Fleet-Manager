import type { HTMLAttributes } from "react";
import { cn } from "@/lib/utils";
import type { AlarmSeverity, Availability, MaintenanceState, MissionStatus, ReservationState, VehicleConnection } from "@/lib/types";
import {
  ALARM_SEVERITY_LABELS,
  AVAILABILITY_LABELS,
  CONNECTION_LABELS,
  MAINTENANCE_STATE_LABELS,
  MISSION_STATUS_LABELS,
  RESERVATION_STATE_LABELS,
  label,
} from "@/lib/labels";

export type BadgeTone = "neutral" | "accent" | "success" | "warning" | "danger" | "info" | "muted";

const TONES: Record<BadgeTone, string> = {
  neutral: "bg-surface-3 text-fg border-border",
  muted: "bg-surface-2 text-fg-muted border-border",
  accent: "bg-accent-soft text-accent border-transparent",
  success: "bg-success-soft text-success border-transparent",
  warning: "bg-warning-soft text-warning border-transparent",
  danger: "bg-danger-soft text-danger border-transparent",
  info: "bg-info-soft text-info border-transparent",
};

export function Badge({ tone = "neutral", className, children, ...rest }: HTMLAttributes<HTMLSpanElement> & { tone?: BadgeTone }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium leading-none border whitespace-nowrap",
        TONES[tone],
        className,
      )}
      {...rest}
    >
      {children}
    </span>
  );
}

export const AVAILABILITY_TONES: Record<Availability, BadgeTone> = {
  available: "success",
  busy: "accent",
  charging: "warning",
  maintenance: "info",
  error: "danger",
  offline: "muted",
  disabled: "muted",
  blocked: "warning",
};

export function AvailabilityBadge({ value }: { value: Availability | string }) {
  return <Badge tone={AVAILABILITY_TONES[value as Availability] ?? "neutral"}>{label(AVAILABILITY_LABELS, value)}</Badge>;
}

const CONNECTION_TONES: Record<VehicleConnection, BadgeTone> = {
  online: "success",
  offline: "muted",
  connectionBroken: "danger",
  unknown: "neutral",
};

export function ConnectionBadge({ value }: { value: VehicleConnection | string }) {
  return <Badge tone={CONNECTION_TONES[value as VehicleConnection] ?? "neutral"}>{label(CONNECTION_LABELS, value)}</Badge>;
}

export const MISSION_STATUS_TONES: Record<MissionStatus, BadgeTone> = {
  created: "muted",
  pending: "neutral",
  planned: "info",
  assigned: "info",
  executing: "accent",
  suspended: "warning",
  completed: "success",
  failed: "danger",
  cancelled: "muted",
};

export function MissionStatusBadge({ value }: { value: MissionStatus | string }) {
  return <Badge tone={MISSION_STATUS_TONES[value as MissionStatus] ?? "neutral"}>{label(MISSION_STATUS_LABELS, value)}</Badge>;
}

const SEVERITY_TONES: Record<AlarmSeverity, BadgeTone> = {
  info: "info",
  warning: "warning",
  error: "danger",
  critical: "danger",
};

export function SeverityBadge({ value }: { value: AlarmSeverity | string }) {
  const tone = SEVERITY_TONES[value as AlarmSeverity] ?? "neutral";
  return (
    <Badge tone={tone} className={value === "critical" ? "ring-1 ring-danger" : undefined}>
      {label(ALARM_SEVERITY_LABELS, value)}
    </Badge>
  );
}

const RESERVATION_TONES: Record<ReservationState, BadgeTone> = {
  requested: "muted",
  granted: "accent",
  active: "success",
  released: "neutral",
  uncertain: "warning",
};

export function ReservationBadge({ value }: { value: ReservationState | string }) {
  return <Badge tone={RESERVATION_TONES[value as ReservationState] ?? "neutral"}>{label(RESERVATION_STATE_LABELS, value)}</Badge>;
}

const MAINT_TONES: Record<MaintenanceState, BadgeTone> = { ok: "success", dueSoon: "warning", overdue: "danger" };

export function MaintenanceBadge({ value }: { value: MaintenanceState | string }) {
  return <Badge tone={MAINT_TONES[value as MaintenanceState] ?? "neutral"}>{label(MAINTENANCE_STATE_LABELS, value)}</Badge>;
}
