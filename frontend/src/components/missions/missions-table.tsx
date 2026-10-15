"use client";
import Link from "next/link";
import type { Mission } from "@/lib/types";
import { MISSION_TYPE_LABELS, label } from "@/lib/labels";
import { fmtDateTime } from "@/lib/format";
import { useNodeName, useVehicleName } from "@/hooks/use-node-names";
import { MissionStatusBadge } from "@/components/ui/badge";
import { ProgressBar, EmptyState } from "@/components/ui/card";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { MissionActions } from "@/components/missions/mission-actions";

export function MissionsTable({ missions, compact, onChanged }: { missions: Mission[]; compact?: boolean; onChanged?: () => void }) {
  const nodeName = useNodeName();
  const vehicleName = useVehicleName();
  if (missions.length === 0) return <EmptyState>Aucune mission</EmptyState>;
  return (
    <Table>
      <THead>
        <tr>
          <Th>Mission</Th>
          <Th>Statut</Th>
          <Th className="text-right">Prio.</Th>
          {!compact && <Th>Type</Th>}
          <Th>Véhicule</Th>
          <Th>Trajet</Th>
          <Th className="w-28">Progression</Th>
          {!compact && <Th>Créée</Th>}
          {!compact && <Th>Démarrée</Th>}
          {!compact && <Th>Terminée</Th>}
          <Th className="text-right">Actions</Th>
        </tr>
      </THead>
      <TBody>
        {missions.map((m) => (
          <Tr key={m.id}>
            <Td>
              <Link href={`/missions/${encodeURIComponent(m.id)}`} className="font-medium hover:underline">
                {m.name || m.id}
              </Link>
              {compact && <div className="text-[10px] text-fg-subtle">{label(MISSION_TYPE_LABELS, m.type)}</div>}
            </Td>
            <Td>
              <MissionStatusBadge value={m.status} />
            </Td>
            <Td className="text-right tabular">{m.priority}</Td>
            {!compact && <Td>{label(MISSION_TYPE_LABELS, m.type)}</Td>}
            <Td>
              {m.vehicleId ? (
                <Link href={`/vehicles/${encodeURIComponent(m.vehicleId)}`} className="hover:underline">
                  {vehicleName(m.vehicleId)}
                </Link>
              ) : (
                <span className="text-fg-subtle">—</span>
              )}
            </Td>
            <Td className="whitespace-nowrap">
              {nodeName(m.origin)} <span className="text-fg-subtle">→</span> {nodeName(m.destination)}
              {m.steps?.length > 2 && <span className="text-fg-subtle"> (+{m.steps.length - 2})</span>}
            </Td>
            <Td>
              <div className="flex items-center gap-2">
                <ProgressBar value={(m.progress ?? 0) * 100} tone={m.status === "failed" ? "danger" : m.status === "completed" ? "success" : "accent"} label="Progression" />
                <span className="tabular text-[10px] w-8 text-right">{Math.round((m.progress ?? 0) * 100)}%</span>
              </div>
            </Td>
            {!compact && <Td className="whitespace-nowrap text-fg-muted">{fmtDateTime(m.createdAt)}</Td>}
            {!compact && <Td className="whitespace-nowrap text-fg-muted">{fmtDateTime(m.startedAt)}</Td>}
            {!compact && <Td className="whitespace-nowrap text-fg-muted">{fmtDateTime(m.completedAt)}</Td>}
            <Td className="text-right">
              <MissionActions mission={m} compact onChanged={onChanged} />
            </Td>
          </Tr>
        ))}
      </TBody>
    </Table>
  );
}
