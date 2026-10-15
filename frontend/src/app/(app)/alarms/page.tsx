"use client";
import { useEffect, useMemo, useState } from "react";
import { Check, Eraser, RefreshCw } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { can } from "@/lib/auth";
import type { Alarm, AlarmSeverity, AlarmSource } from "@/lib/types";
import { ALARM_SEVERITY_LABELS, ALARM_SOURCE_LABELS, label } from "@/lib/labels";
import { fmtDateTime } from "@/lib/format";
import { useApi, useInterval } from "@/hooks/use-api";
import { useVehicleName } from "@/hooks/use-node-names";
import { useAuthStore } from "@/stores/auth";
import { useEventsStore } from "@/stores/events";
import { useFleetStore } from "@/stores/fleet";
import { useSiteStore } from "@/stores/site";
import { toast } from "@/stores/ui";
import { PageHeader, Card, EmptyState, ErrorBox, Spinner } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Tabs, Chips } from "@/components/ui/tabs";
import { SeverityBadge, Badge } from "@/components/ui/badge";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import Link from "next/link";

type Tab = "active" | "history";

export default function AlarmsPage() {
  const siteId = useSiteStore((s) => s.siteId);
  const user = useAuthStore((s) => s.user);
  const liveAlarms = useFleetStore((s) => s.alarms);
  const events = useEventsStore((s) => s.events);
  const vehicleName = useVehicleName();
  const [tab, setTab] = useState<Tab>("active");
  const [severities, setSeverities] = useState<AlarmSeverity[]>([]);
  const [source, setSource] = useState("");
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState<string | null>(null);

  const history = useApi(() => api.alarms.list({ siteId, limit: 500 }), [siteId, tab === "history"], tab === "history");
  const activeRest = useApi(() => api.alarms.list({ siteId, active: true, limit: 500 }), [siteId]);
  useInterval(() => void (tab === "history" ? history.refetch() : activeRest.refetch()), 15000);

  // Refetch history when alarm events arrive.
  const lastAlarmEventId = useMemo(() => events.find((e) => e.kind === "alarm.raised" || e.kind === "alarm.cleared")?.id ?? 0, [events]);
  useEffect(() => {
    if (tab === "history") void history.refetch();
    else void activeRest.refetch();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lastAlarmEventId]);

  const active = useMemo(() => {
    const byId = new Map<string, Alarm>();
    for (const a of activeRest.data ?? []) if (a.active) byId.set(a.id, a);
    for (const a of liveAlarms) byId.set(a.id, a);
    return Array.from(byId.values());
  }, [activeRest.data, liveAlarms]);

  const historyData = history.data;
  const source_ = useMemo(() => (tab === "active" ? active : (historyData ?? [])), [tab, active, historyData]);
  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return source_
      .filter((a) => (severities.length ? severities.includes(a.severity) : true))
      .filter((a) => (source ? a.source === source : true))
      .filter((a) => (needle ? `${a.message} ${a.code} ${a.vehicleId} ${a.missionId}`.toLowerCase().includes(needle) : true))
      .sort((a, b) => (b.raisedAt ?? "").localeCompare(a.raisedAt ?? ""));
  }, [source_, severities, source, q]);

  const counts = useMemo(() => {
    const c: Partial<Record<AlarmSeverity, number>> = {};
    for (const a of source_) c[a.severity] = (c[a.severity] ?? 0) + 1;
    return c;
  }, [source_]);

  const act = async (a: Alarm, kind: "ack" | "clear") => {
    setBusy(`${kind}-${a.id}`);
    try {
      if (kind === "ack") await api.alarms.ack(a.id);
      else await api.alarms.clear(a.id);
      toast.success(kind === "ack" ? "Alarme acquittée" : "Alarme effacée", a.code);
      void activeRest.refetch();
      if (tab === "history") void history.refetch();
    } catch (e) {
      toast.error("Action refusée", errorMessage(e));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title="Alarmes"
        subtitle={`${active.length} actives`}
        actions={
          <Button size="sm" variant="ghost" onClick={() => void (tab === "history" ? history.refetch() : activeRest.refetch())}>
            <RefreshCw className="size-3.5" /> Actualiser
          </Button>
        }
      />
      <Card>
        <Tabs<Tab>
          tabs={[
            { key: "active", label: `Actives (${active.length})` },
            { key: "history", label: "Historique" },
          ]}
          value={tab}
          onChange={setTab}
          className="px-2"
        />
        <div className="flex flex-wrap items-center gap-2 p-2 border-b border-border">
          <Input aria-label="Rechercher" placeholder="Rechercher…" value={q} onChange={(e) => setQ(e.target.value)} className="w-48" />
          <Select
            aria-label="Source"
            value={source}
            onChange={(e) => setSource(e.target.value)}
            placeholder="Toutes sources"
            options={(Object.keys(ALARM_SOURCE_LABELS) as AlarmSource[]).map((k) => ({ value: k, label: ALARM_SOURCE_LABELS[k] }))}
            className="w-44"
          />
          <Chips
            items={(Object.keys(ALARM_SEVERITY_LABELS) as AlarmSeverity[]).map((k) => ({ key: k, label: ALARM_SEVERITY_LABELS[k], count: counts[k] ?? 0 }))}
            value={severities}
            onChange={setSeverities}
          />
        </div>
        {tab === "history" && history.error && <ErrorBox message={errorMessage(history.error)} />}
        {tab === "history" && history.loading && !history.data ? (
          <Spinner />
        ) : filtered.length === 0 ? (
          <EmptyState>Aucune alarme.</EmptyState>
        ) : (
          <Table>
            <THead>
              <tr>
                <Th>Sévérité</Th>
                <Th>Code</Th>
                <Th>Message</Th>
                <Th>Source</Th>
                <Th>Véhicule</Th>
                <Th>Mission</Th>
                <Th>Levée</Th>
                <Th>Acquittée</Th>
                {tab === "history" && <Th>Effacée</Th>}
                <Th className="text-right">Actions</Th>
              </tr>
            </THead>
            <TBody>
              {filtered.map((a) => (
                <Tr key={a.id} className={!a.active ? "opacity-70" : undefined}>
                  <Td><SeverityBadge value={a.severity} /></Td>
                  <Td className="font-mono text-[11px]">{a.code}</Td>
                  <Td className="max-w-md">{a.message}</Td>
                  <Td>{label(ALARM_SOURCE_LABELS, a.source)}</Td>
                  <Td>{a.vehicleId ? <Link href={`/vehicles/${encodeURIComponent(a.vehicleId)}`} className="hover:underline">{vehicleName(a.vehicleId)}</Link> : "—"}</Td>
                  <Td>{a.missionId ? <Link href={`/missions/${encodeURIComponent(a.missionId)}`} className="hover:underline">{a.missionId}</Link> : "—"}</Td>
                  <Td className="whitespace-nowrap text-fg-muted">{fmtDateTime(a.raisedAt)}</Td>
                  <Td className="whitespace-nowrap text-fg-muted">
                    {a.acknowledgedAt ? (
                      <span title={a.acknowledgedBy}>{fmtDateTime(a.acknowledgedAt)} {a.acknowledgedBy && <span className="text-fg-subtle">({a.acknowledgedBy})</span>}</span>
                    ) : a.active ? (
                      <Badge tone="warning">à acquitter</Badge>
                    ) : (
                      "—"
                    )}
                  </Td>
                  {tab === "history" && <Td className="whitespace-nowrap text-fg-muted">{fmtDateTime(a.clearedAt)}</Td>}
                  <Td className="text-right whitespace-nowrap">
                    {a.active && !a.acknowledgedAt && can(user, "alarms.ack") && (
                      <Button size="xs" onClick={() => act(a, "ack")} loading={busy === `ack-${a.id}`}>
                        <Check className="size-3" /> Acquitter
                      </Button>
                    )}
                    {a.active && a.source === "system" && can(user, "alarms.clear") && (
                      <Button size="xs" variant="ghost" className="ml-1" onClick={() => act(a, "clear")} loading={busy === `clear-${a.id}`}>
                        <Eraser className="size-3" /> Effacer
                      </Button>
                    )}
                  </Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </Card>
    </div>
  );
}
