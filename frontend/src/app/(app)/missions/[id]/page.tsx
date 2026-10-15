"use client";
import Link from "next/link";
import { useParams } from "next/navigation";
import { api, errorMessage } from "@/lib/api";
import { fmtDateTime, fmtDuration, fmtNum } from "@/lib/format";
import { MISSION_SOURCE_LABELS, MISSION_TYPE_LABELS, STEP_ACTION_LABELS, label } from "@/lib/labels";
import { useApi, useInterval } from "@/hooks/use-api";
import { useNodeName, useVehicleName } from "@/hooks/use-node-names";
import { useFleetStore } from "@/stores/fleet";
import { PageHeader, Card, CardHeader, CardBody, DescriptionList, Spinner, ErrorBox, ProgressBar, EmptyState } from "@/components/ui/card";
import { Badge, MissionStatusBadge } from "@/components/ui/badge";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { MissionActions } from "@/components/missions/mission-actions";
import { TERMINAL_MISSION_STATUSES } from "@/lib/types";

export default function MissionDetailPage() {
  const params = useParams<{ id: string }>();
  const id = decodeURIComponent(params.id);
  const live = useFleetStore((s) => s.missions.find((m) => m.id === id));
  const fetched = useApi(() => api.missions.get(id), [id]);
  const mission = live ?? fetched.data;
  const nodeName = useNodeName();
  const vehicleName = useVehicleName();
  useInterval(() => void fetched.refetch(), mission && TERMINAL_MISSION_STATUSES.includes(mission.status) ? null : 5000);

  if (!mission) {
    if (fetched.error) return <ErrorBox message={errorMessage(fetched.error)} />;
    return <Spinner />;
  }
  const dur =
    mission.startedAt && mission.completedAt ? (new Date(mission.completedAt).getTime() - new Date(mission.startedAt).getTime()) / 1000 : null;

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title={mission.name || mission.id}
        subtitle={
          <span className="flex items-center gap-2 flex-wrap">
            <span>{mission.id}</span>
            <MissionStatusBadge value={mission.status} />
            <Badge tone="neutral">{label(MISSION_TYPE_LABELS, mission.type)}</Badge>
            <Badge tone="muted">Priorité {mission.priority}</Badge>
          </span>
        }
        actions={<MissionActions mission={mission} onChanged={() => void fetched.refetch()} />}
      />
      {mission.errorMessage && <ErrorBox message={mission.errorMessage} />}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-3">
        <Card>
          <CardHeader title="Détails" />
          <CardBody className="flex flex-col gap-3">
            <ProgressBar value={(mission.progress ?? 0) * 100} label="Progression" />
            <DescriptionList
              items={[
                ["Origine", nodeName(mission.origin)],
                ["Destination", nodeName(mission.destination)],
                ["Véhicule", mission.vehicleId ? <Link className="text-accent hover:underline" href={`/vehicles/${encodeURIComponent(mission.vehicleId)}`}>{vehicleName(mission.vehicleId)}</Link> : "—"],
                ["Modèle requis", mission.requiredModelId || "—"],
                ["Source", label(MISSION_SOURCE_LABELS, mission.source)],
                ["Réf. externe", mission.externalRef || "—"],
                ["Échéance", fmtDateTime(mission.deadline)],
                ["Créée", fmtDateTime(mission.createdAt)],
                ["Affectée", fmtDateTime(mission.assignedAt)],
                ["Démarrée", fmtDateTime(mission.startedAt)],
                ["Terminée", fmtDateTime(mission.completedAt)],
                ["Durée", fmtDuration(dur)],
                ["Charge", mission.load?.present ? `${fmtNum(mission.load.massKg, 0)} kg · ${mission.load.lengthM}×${mission.load.widthM}×${mission.load.heightM} m` : "aucune"],
                ["Dépendances", mission.dependencies?.length ? mission.dependencies.map((d) => <Link key={d} className="text-accent hover:underline mr-1" href={`/missions/${encodeURIComponent(d)}`}>{d}</Link>) : "—"],
                ["Zones évitées", mission.constraints?.avoidZoneIds?.length ? mission.constraints.avoidZoneIds.join(", ") : "—"],
                ["Vitesse max", mission.constraints?.maxSpeedMs ? `${mission.constraints.maxSpeedMs} m/s` : "—"],
              ]}
            />
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Étapes" />
          <Table>
            <THead><tr><Th>#</Th><Th>Nœud</Th><Th>Action</Th><Th>Statut</Th><Th>Durée</Th></tr></THead>
            <TBody>
              {(mission.steps ?? []).map((s) => (
                <Tr key={s.seq} className={s.status === "active" ? "bg-accent-soft/40" : undefined}>
                  <Td className="tabular">{s.seq}</Td>
                  <Td>{nodeName(s.nodeId)}</Td>
                  <Td>{label(STEP_ACTION_LABELS, s.action)}</Td>
                  <Td><Badge tone={s.status === "done" ? "success" : s.status === "active" ? "accent" : s.status === "skipped" ? "muted" : "neutral"}>{s.status}</Badge></Td>
                  <Td className="tabular">{s.durationS ? fmtDuration(s.durationS) : "—"}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
          <CardHeader title="Itinéraire" subtitle={`${mission.routeNodeIds?.length ?? 0} nœuds`} className="border-t" />
          <CardBody>
            {mission.routeNodeIds?.length ? (
              <ol className="flex flex-wrap gap-1 text-[11px]">
                {mission.routeNodeIds.map((n, i) => (
                  <li key={`${n}-${i}`} className="inline-flex items-center gap-1">
                    <span className="rounded bg-surface-2 border border-border px-1.5 py-0.5">{nodeName(n)}</span>
                    {i < mission.routeNodeIds.length - 1 && <span className="text-fg-subtle">→</span>}
                  </li>
                ))}
              </ol>
            ) : (
              <p className="text-[11px] text-fg-subtle">Pas encore d’itinéraire calculé.</p>
            )}
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Chronologie" />
          <CardBody>
            {mission.history?.length ? (
              <ol className="relative border-l border-border ml-2 flex flex-col gap-2">
                {mission.history.map((h, i) => (
                  <li key={i} className="pl-3">
                    <span className="absolute -left-[5px] mt-1.5 size-2 rounded-full bg-accent" aria-hidden />
                    <div className="text-[11px] text-fg-muted tabular">{fmtDateTime(h.at)}</div>
                    <div className="text-xs"><MissionStatusBadge value={h.status} /> <span className="ml-1">{h.message}</span></div>
                  </li>
                ))}
              </ol>
            ) : (
              <EmptyState>Aucun événement.</EmptyState>
            )}
          </CardBody>
        </Card>
      </div>

      <Card>
        <CardHeader
          title="Explication de l’affectation"
          subtitle={
            mission.assignment
              ? `Véhicule choisi : ${vehicleName(mission.assignment.chosenVehicleId)} · poids : ${Object.entries(mission.assignment.weights ?? {}).map(([k, v]) => `${k}=${v}`).join(", ")}`
              : "Aucune affectation calculée"
          }
        />
        {mission.assignment ? (
          <Table>
            <THead>
              <tr>
                <Th>Véhicule</Th>
                <Th>Éligible</Th>
                <Th>Raison</Th>
                <Th className="text-right">Score</Th>
                <Th className="text-right">Distance</Th>
                <Th className="text-right">Temps</Th>
                <Th className="text-right">Batterie</Th>
                <Th className="text-right">Congestion</Th>
                <Th className="text-right">File</Th>
                <Th className="text-right">Priorité</Th>
              </tr>
            </THead>
            <TBody>
              {[...mission.assignment.candidates].sort((a, b) => Number(b.eligible) - Number(a.eligible) || b.score - a.score).map((c) => (
                <Tr key={c.vehicleId} className={c.vehicleId === mission.assignment?.chosenVehicleId ? "bg-success-soft/40" : undefined}>
                  <Td className="font-medium">{vehicleName(c.vehicleId)}</Td>
                  <Td>{c.eligible ? <Badge tone="success">oui</Badge> : <Badge tone="muted">non</Badge>}</Td>
                  <Td className="text-fg-muted">{c.reason || "—"}</Td>
                  <Td className="text-right tabular">{fmtNum(c.score, 2)}</Td>
                  <Td className="text-right tabular">{fmtNum(c.factors?.distanceM, 1)} m</Td>
                  <Td className="text-right tabular">{fmtDuration(c.factors?.travelTimeS)}</Td>
                  <Td className="text-right tabular">{fmtNum(c.factors?.battery, 2)}</Td>
                  <Td className="text-right tabular">{fmtNum(c.factors?.congestion, 2)}</Td>
                  <Td className="text-right tabular">{fmtNum(c.factors?.queue, 2)}</Td>
                  <Td className="text-right tabular">{fmtNum(c.factors?.priority, 2)}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        ) : null}
      </Card>
    </div>
  );
}
