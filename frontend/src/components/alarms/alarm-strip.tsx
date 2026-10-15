"use client";
import Link from "next/link";
import { Bell } from "lucide-react";
import type { Alarm } from "@/lib/types";
import { fmtTime } from "@/lib/format";
import { SeverityBadge } from "@/components/ui/badge";
import { useVehicleName } from "@/hooks/use-node-names";

export function AlarmStrip({ alarms }: { alarms: Alarm[] }) {
  const vehicleName = useVehicleName();
  const sorted = [...alarms].sort((a, b) => rank(b.severity) - rank(a.severity));
  if (sorted.length === 0) {
    return (
      <div className="flex items-center gap-2 rounded-md border border-border bg-surface px-3 py-2 text-xs text-fg-muted">
        <Bell className="size-3.5" aria-hidden /> Aucune alarme active
      </div>
    );
  }
  return (
    <div className="flex gap-2 overflow-x-auto pb-1" role="list" aria-label="Alarmes actives">
      {sorted.map((a) => (
        <Link
          key={a.id}
          href="/alarms"
          role="listitem"
          className="shrink-0 w-64 rounded-md border border-border bg-surface px-3 py-2 hover:bg-surface-2"
        >
          <div className="flex items-center justify-between gap-2 mb-1">
            <SeverityBadge value={a.severity} />
            <span className="text-[10px] text-fg-subtle tabular">{fmtTime(a.raisedAt)}</span>
          </div>
          <div className="text-xs truncate" title={a.message}>
            {a.message}
          </div>
          <div className="text-[10px] text-fg-muted truncate">
            {a.code} {a.vehicleId ? `· ${vehicleName(a.vehicleId)}` : ""}
          </div>
        </Link>
      ))}
    </div>
  );
}

function rank(s: Alarm["severity"]): number {
  return s === "critical" ? 4 : s === "error" ? 3 : s === "warning" ? 2 : 1;
}
