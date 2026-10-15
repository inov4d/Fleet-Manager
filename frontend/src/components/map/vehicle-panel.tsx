"use client";
import Link from "next/link";
import { X } from "lucide-react";
import type { MapNode, VehicleWithState } from "@/lib/types";
import { AVAILABILITY_LABELS, CONNECTION_LABELS, OPERATING_MODE_LABELS, label } from "@/lib/labels";
import { fmtCoord, fmtDeg, fmtNum, fmtRelative } from "@/lib/format";
import { useNow } from "@/hooks/use-now";
import { useNodeName } from "@/hooks/use-node-names";
import { AvailabilityBadge, ConnectionBadge } from "@/components/ui/badge";
import { BatteryBar, DescriptionList } from "@/components/ui/card";
import { IconButton } from "@/components/ui/button";
import { VehicleCommands } from "@/components/vehicles/vehicle-commands";
import { vehicleColorCss } from "@/lib/utils";

export function VehiclePanel({ item, nodes, onClose }: { item: VehicleWithState; nodes: MapNode[]; onClose: () => void }) {
  const now = useNow(1000);
  const nodeName = useNodeName();
  const { vehicle, state } = item;
  const speed = Math.hypot(state.velocity?.vx ?? 0, state.velocity?.vy ?? 0);
  return (
    <aside className="w-80 max-w-full h-full overflow-y-auto border-l border-border bg-surface flex flex-col" aria-label={`Véhicule ${vehicle.name}`}>
      <div className="flex items-center justify-between gap-2 px-3 py-2 border-b border-border">
        <div className="flex items-center gap-2 min-w-0">
          <span className="size-3 rounded-sm shrink-0" style={{ background: vehicleColorCss(vehicle.id) }} aria-hidden />
          <Link href={`/vehicles/${encodeURIComponent(vehicle.id)}`} className="font-semibold text-sm truncate hover:underline">
            {vehicle.name}
          </Link>
        </div>
        <IconButton label="Fermer le panneau" onClick={onClose} size="xs">
          <X className="size-4" />
        </IconButton>
      </div>
      <div className="p-3 flex flex-col gap-3">
        <div className="flex flex-wrap gap-1">
          <AvailabilityBadge value={state.availability} />
          <ConnectionBadge value={state.connection} />
          {state.paused && <span className="text-[11px] text-warning">En pause</span>}
        </div>
        <BatteryBar value={state.batteryCharge} charging={state.charging} />
        <DescriptionList
          items={[
            ["Modèle", vehicle.modelId],
            ["Mode", label(OPERATING_MODE_LABELS, state.operatingMode)],
            ["Position", `${fmtCoord(state.pose?.x)} ; ${fmtCoord(state.pose?.y)} · ${fmtDeg(state.pose?.theta)}`],
            ["Vitesse", `${fmtNum(speed, 2)} m/s`],
            ["Dernier nœud", nodeName(state.lastNodeId)],
            ["Mission", state.missionId ? <Link className="text-accent hover:underline" href={`/missions/${encodeURIComponent(state.missionId)}`}>{state.missionId}</Link> : "—"],
            ["Charge", state.loadPresent ? `${fmtNum(state.loadMassKg, 0)} kg` : "aucune"],
            ["Enveloppe", `${fmtNum(state.loadEnvelope?.lengthM, 2)} × ${fmtNum(state.loadEnvelope?.widthM, 2)} m`],
            ["Localisation", `${fmtNum((state.localizationScore ?? 0) * 100, 0)} %`],
            ["Arrêt d’urgence", state.safetyState?.eStop ?? "—"],
            ["Dernière vue", fmtRelative(state.lastSeen, now)],
          ]}
        />
        {state.errors.length > 0 && (
          <ul className="text-[11px] flex flex-col gap-1" aria-label="Erreurs">
            {state.errors.map((e, i) => (
              <li key={i} className={e.errorLevel === "fatal" ? "text-danger" : "text-warning"}>
                <strong>{e.errorType}</strong> — {e.errorDescription}
              </li>
            ))}
          </ul>
        )}
        <div>
          <h3 className="text-[11px] uppercase tracking-wide text-fg-muted mb-1">Commandes</h3>
          <VehicleCommands item={item} nodes={nodes} compact />
        </div>
        <p className="text-[10px] text-fg-subtle">{label(AVAILABILITY_LABELS, state.availability)} · {label(CONNECTION_LABELS, state.connection)}</p>
      </div>
    </aside>
  );
}
