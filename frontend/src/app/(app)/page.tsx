"use client";
import Link from "next/link";
import { Activity, AlertTriangle, CheckCircle2, Gauge, Route, Truck, XCircle } from "lucide-react";
import { useFleetStore } from "@/stores/fleet";
import { useActiveMap } from "@/hooks/use-active-map";
import { useApi } from "@/hooks/use-api";
import { api } from "@/lib/api";
import { fmtDistance, fmtNum, fmtPct } from "@/lib/format";
import { KpiTile, Card, CardHeader } from "@/components/ui/card";
import { LiveMapDynamic } from "@/components/map/live-map-dynamic";
import { MissionsTable } from "@/components/missions/missions-table";
import { AlarmStrip } from "@/components/alarms/alarm-strip";
import { Button } from "@/components/ui/button";
import { AVAILABILITY_LABELS } from "@/lib/labels";
import { AvailabilityBadge } from "@/components/ui/badge";
import type { Availability } from "@/lib/types";

export default function DashboardPage() {
  const map = useActiveMap();
  const kpis = useFleetStore((s) => s.kpis);
  const vehicles = useFleetStore((s) => s.vehicles);
  const missions = useFleetStore((s) => s.missions);
  const alarms = useFleetStore((s) => s.alarms);
  const models = useApi(() => api.vehicleModels.list(), []);

  const online = vehicles.filter((v) => v.state.connection === "online").length;
  const byAvailability = vehicles.reduce<Partial<Record<Availability, number>>>((acc, v) => {
    acc[v.state.availability] = (acc[v.state.availability] ?? 0) + 1;
    return acc;
  }, {});
  const activeMissions = missions.filter((m) => m.status === "executing" || m.status === "assigned" || m.status === "planned" || m.status === "suspended");
  const waiting = missions.filter((m) => m.status === "pending" || m.status === "created");

  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-7 gap-2">
        <KpiTile label="Missions / h" value={fmtNum(kpis?.missionsPerHour, 1)} icon={<Route className="size-4" />} tone="accent" />
        <KpiTile label="Utilisation" value={fmtPct(kpis?.fleetUtilizationPct)} icon={<Gauge className="size-4" />} />
        <KpiTile label="AGV en ligne" value={`${online}`} unit={`/ ${vehicles.length}`} icon={<Truck className="size-4" />} tone={online < vehicles.length ? "warning" : "success"} />
        <KpiTile label="Alarmes actives" value={`${alarms.length}`} icon={<AlertTriangle className="size-4" />} tone={alarms.length > 0 ? "danger" : "neutral"} />
        <KpiTile label="Terminées" value={fmtNum(kpis?.missionsCompleted, 0)} icon={<CheckCircle2 className="size-4" />} tone="success" />
        <KpiTile label="Échouées" value={fmtNum(kpis?.missionsFailed, 0)} icon={<XCircle className="size-4" />} tone={(kpis?.missionsFailed ?? 0) > 0 ? "danger" : "neutral"} />
        <KpiTile label="Distance" value={fmtDistance(kpis?.distanceM)} icon={<Activity className="size-4" />} hint={kpis ? `${fmtNum(kpis.energyKwh, 1)} kWh` : undefined} />
      </div>

      <AlarmStrip alarms={alarms} />

      <div className="grid grid-cols-1 xl:grid-cols-3 gap-3">
        <Card className="xl:col-span-2 overflow-hidden flex flex-col min-h-[420px]">
          <CardHeader
            title="Carte en direct"
            subtitle={map ? `${map.name} · v${map.version}` : "Aucune carte active"}
            actions={
              <Link href="/map">
                <Button size="xs" variant="ghost">Plein écran</Button>
              </Link>
            }
          />
          <div className="flex-1 min-h-[380px]">
            <LiveMapDynamic map={map} models={models.data} readOnly />
          </div>
        </Card>
        <Card className="flex flex-col min-h-[420px]">
          <CardHeader title="Flotte" subtitle={`${vehicles.length} véhicules`} />
          <div className="p-3 flex flex-wrap gap-1.5">
            {(Object.keys(AVAILABILITY_LABELS) as Availability[]).map((k) =>
              byAvailability[k] ? (
                <span key={k} className="inline-flex items-center gap-1 text-xs">
                  <AvailabilityBadge value={k} /> <span className="tabular">{byAvailability[k]}</span>
                </span>
              ) : null,
            )}
          </div>
          <ul className="flex-1 overflow-y-auto divide-y divide-border text-xs" aria-label="Véhicules">
            {vehicles.map((v) => (
              <li key={v.vehicle.id} className="flex items-center justify-between gap-2 px-3 py-1.5">
                <Link href={`/vehicles/${encodeURIComponent(v.vehicle.id)}`} className="font-medium hover:underline truncate">
                  {v.vehicle.name}
                </Link>
                <span className="flex items-center gap-2 shrink-0">
                  <span className="tabular text-fg-muted">{Math.round(v.state.batteryCharge)} %</span>
                  <AvailabilityBadge value={v.state.availability} />
                </span>
              </li>
            ))}
            {vehicles.length === 0 && <li className="px-3 py-6 text-center text-fg-muted">Aucun véhicule</li>}
          </ul>
        </Card>
      </div>

      <Card>
        <CardHeader
          title="Missions actives"
          subtitle={`${activeMissions.length} en cours · ${waiting.length} en attente`}
          actions={
            <Link href="/missions">
              <Button size="xs" variant="ghost">Toutes les missions</Button>
            </Link>
          }
        />
        <MissionsTable missions={[...activeMissions, ...waiting]} compact />
      </Card>
    </div>
  );
}
