"use client";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { Plus, RefreshCw } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { can } from "@/lib/auth";
import { MISSION_STATUS_LABELS, MISSION_TYPE_LABELS } from "@/lib/labels";
import type { Mission, MissionStatus, MissionType } from "@/lib/types";
import { useApi, useInterval } from "@/hooks/use-api";
import { useAuthStore } from "@/stores/auth";
import { useFleetStore } from "@/stores/fleet";
import { useSiteStore } from "@/stores/site";
import { PageHeader, Card, Spinner, ErrorBox } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Chips } from "@/components/ui/tabs";
import { MissionsTable } from "@/components/missions/missions-table";

const ALL_STATUSES = Object.keys(MISSION_STATUS_LABELS) as MissionStatus[];

export default function MissionsPage() {
  const user = useAuthStore((s) => s.user);
  const siteId = useSiteStore((s) => s.siteId);
  const liveMissions = useFleetStore((s) => s.missions);
  const [statuses, setStatuses] = useState<MissionStatus[]>([]);
  const [type, setType] = useState("");
  const [vehicleId, setVehicleId] = useState("");
  const [q, setQ] = useState("");
  const [limit, setLimit] = useState(200);
  const vehicles = useFleetStore((s) => s.vehicles);

  const query = useApi(() => api.missions.list({ siteId, limit, vehicleId: vehicleId || undefined }), [siteId, limit, vehicleId]);
  useInterval(() => void query.refetch(), 10000);

  // Merge live (non-terminal) missions from the snapshot with the REST list.
  const merged = useMemo(() => {
    const byId = new Map<string, Mission>();
    for (const m of query.data ?? []) byId.set(m.id, m);
    for (const m of liveMissions) if (!vehicleId || m.vehicleId === vehicleId) byId.set(m.id, m);
    return Array.from(byId.values()).sort((a, b) => (b.createdAt ?? "").localeCompare(a.createdAt ?? ""));
  }, [query.data, liveMissions, vehicleId]);

  const counts = useMemo(() => {
    const c: Partial<Record<MissionStatus, number>> = {};
    for (const m of merged) c[m.status] = (c[m.status] ?? 0) + 1;
    return c;
  }, [merged]);

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return merged.filter((m) => {
      if (statuses.length && !statuses.includes(m.status)) return false;
      if (type && m.type !== type) return false;
      if (needle && !`${m.name} ${m.id} ${m.externalRef}`.toLowerCase().includes(needle)) return false;
      return true;
    });
  }, [merged, statuses, type, q]);

  useEffect(() => {
    setStatuses([]);
  }, [siteId]);

  return (
    <div>
      <PageHeader
        title="Missions"
        subtitle={`${merged.length} missions chargées`}
        actions={
          <>
            <Button size="sm" variant="ghost" onClick={() => void query.refetch()} loading={query.loading}>
              <RefreshCw className="size-3.5" /> Actualiser
            </Button>
            {can(user, "missions.create") && (
              <Link href="/missions/new">
                <Button size="sm" variant="primary">
                  <Plus className="size-3.5" /> Nouvelle mission
                </Button>
              </Link>
            )}
          </>
        }
      />
      <Card>
        <div className="flex flex-wrap items-center gap-2 p-2 border-b border-border">
          <Input aria-label="Rechercher" placeholder="Rechercher…" value={q} onChange={(e) => setQ(e.target.value)} className="w-48" />
          <Select
            aria-label="Type"
            value={type}
            onChange={(e) => setType(e.target.value)}
            placeholder="Tous types"
            options={(Object.keys(MISSION_TYPE_LABELS) as MissionType[]).map((k) => ({ value: k, label: MISSION_TYPE_LABELS[k] }))}
            className="w-48"
          />
          <Select
            aria-label="Véhicule"
            value={vehicleId}
            onChange={(e) => setVehicleId(e.target.value)}
            placeholder="Tous véhicules"
            options={vehicles.map((v) => ({ value: v.vehicle.id, label: v.vehicle.name }))}
            className="w-44"
          />
          <Select
            aria-label="Limite"
            value={String(limit)}
            onChange={(e) => setLimit(Number(e.target.value))}
            options={[50, 200, 500, 1000].map((n) => ({ value: String(n), label: `${n} dernières` }))}
            className="w-36"
          />
          <Chips
            items={ALL_STATUSES.map((s) => ({ key: s, label: MISSION_STATUS_LABELS[s], count: counts[s] ?? 0 }))}
            value={statuses}
            onChange={setStatuses}
          />
        </div>
        {query.error && !query.data ? <ErrorBox message={errorMessage(query.error)} /> : null}
        {query.loading && !query.data ? <Spinner /> : <MissionsTable missions={filtered} onChanged={() => void query.refetch()} />}
      </Card>
    </div>
  );
}
