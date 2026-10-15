"use client";
import { useState } from "react";
import { api, errorMessage } from "@/lib/api";
import { fmtDistance, fmtDuration, fmtNum, fmtPct, toLocalInput } from "@/lib/format";
import { useApi } from "@/hooks/use-api";
import { useVehicleName } from "@/hooks/use-node-names";
import { useSiteStore } from "@/stores/site";
import { PageHeader, Card, CardHeader, KpiTile, EmptyState, ErrorBox } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { TimeseriesChart } from "@/components/charts/timeseries-chart";
import { chartColors } from "@/components/charts/chart-theme";
import { BatteryBar } from "@/components/ui/card";

const PRESETS = [
  { label: "1 h", ms: 3600_000 },
  { label: "8 h", ms: 8 * 3600_000 },
  { label: "24 h", ms: 24 * 3600_000 },
  { label: "7 j", ms: 7 * 24 * 3600_000 },
];

export default function AnalyticsPage() {
  const siteId = useSiteStore((s) => s.siteId);
  const vehicleName = useVehicleName();
  const [from, setFrom] = useState(() => toLocalInput(new Date(Date.now() - 24 * 3600_000)));
  const [to, setTo] = useState(() => toLocalInput(new Date()));
  const [bucket, setBucket] = useState("15m");
  const fromIso = new Date(from).toISOString();
  const toIso = new Date(to).toISOString();

  const kpis = useApi(() => api.analytics.kpis(siteId, fromIso, toIso), [siteId, fromIso, toIso]);
  const ts = (metric: "missionsPerHour" | "utilization" | "activeVehicles" | "alarms") =>
    api.analytics.timeseries({ siteId, metric, from: fromIso, to: toIso, bucket });
  const missionsPerHour = useApi(() => ts("missionsPerHour"), [siteId, fromIso, toIso, bucket]);
  const utilization = useApi(() => ts("utilization"), [siteId, fromIso, toIso, bucket]);
  const activeVehicles = useApi(() => ts("activeVehicles"), [siteId, fromIso, toIso, bucket]);
  const alarms = useApi(() => ts("alarms"), [siteId, fromIso, toIso, bucket]);
  const k = kpis.data;

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title="Analytique"
        actions={
          <div className="flex flex-wrap items-center gap-1.5">
            {PRESETS.map((p) => (
              <Button key={p.label} size="xs" onClick={() => { setFrom(toLocalInput(new Date(Date.now() - p.ms))); setTo(toLocalInput(new Date())); }}>
                {p.label}
              </Button>
            ))}
            <Input type="datetime-local" aria-label="Début" value={from} onChange={(e) => setFrom(e.target.value)} className="h-7 w-44" />
            <span className="text-fg-subtle">→</span>
            <Input type="datetime-local" aria-label="Fin" value={to} onChange={(e) => setTo(e.target.value)} className="h-7 w-44" />
            <Select aria-label="Granularité" value={bucket} onChange={(e) => setBucket(e.target.value)} options={["1m", "5m", "15m", "1h", "1d"].map((b) => ({ value: b, label: `Pas ${b}` }))} className="h-7 w-28" />
          </div>
        }
      />
      {kpis.error && <ErrorBox message={errorMessage(kpis.error)} />}
      <div className="grid grid-cols-2 md:grid-cols-5 gap-2">
        <KpiTile label="Missions terminées" value={fmtNum(k?.missionsCompleted, 0)} tone="success" />
        <KpiTile label="Missions échouées" value={fmtNum(k?.missionsFailed, 0)} tone={(k?.missionsFailed ?? 0) > 0 ? "danger" : "neutral"} />
        <KpiTile label="Missions / h" value={fmtNum(k?.missionsPerHour, 1)} tone="accent" />
        <KpiTile label="Utilisation flotte" value={fmtPct(k?.fleetUtilizationPct)} />
        <KpiTile label="Distance" value={fmtDistance(k?.distanceM)} />
        <KpiTile label="Durée moy. mission" value={fmtDuration(k?.avgMissionDurationS)} />
        <KpiTile label="Attente moyenne" value={fmtDuration(k?.avgWaitTimeS)} />
        <KpiTile label="Énergie" value={fmtNum(k?.energyKwh, 1)} unit="kWh" />
        <KpiTile label="Alarmes actives" value={fmtNum(k?.activeAlarms, 0)} tone={(k?.activeAlarms ?? 0) > 0 ? "warning" : "neutral"} />
        <KpiTile label="Interblocages résolus" value={fmtNum(k?.deadlocksResolved, 0)} />
      </div>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
        <TimeseriesChart title="Missions par heure" data={missionsPerHour.data} loading={missionsPerHour.loading} color={chartColors.c1} kind="bar" />
        <TimeseriesChart title="Utilisation" unit="%" data={utilization.data} loading={utilization.loading} color={chartColors.c2} domain={[0, 100]} />
        <TimeseriesChart title="Véhicules actifs" data={activeVehicles.data} loading={activeVehicles.loading} color={chartColors.c5} />
        <TimeseriesChart title="Alarmes" data={alarms.data} loading={alarms.loading} color={chartColors.c4} kind="bar" />
      </div>
      <Card>
        <CardHeader title="Par véhicule" subtitle="Sur la période sélectionnée" />
        {!k?.vehicles?.length ? (
          <EmptyState>Aucune donnée par véhicule.</EmptyState>
        ) : (
          <Table>
            <THead>
              <tr>
                <Th>Véhicule</Th>
                <Th className="text-right">Utilisation</Th>
                <Th className="text-right">Missions</Th>
                <Th className="text-right">Distance</Th>
                <Th>Batterie</Th>
                <Th className="text-right">Alarmes</Th>
              </tr>
            </THead>
            <TBody>
              {[...k.vehicles].sort((a, b) => b.utilizationPct - a.utilizationPct).map((v) => (
                <Tr key={v.vehicleId}>
                  <Td className="font-medium">{vehicleName(v.vehicleId)}</Td>
                  <Td className="text-right tabular">{fmtPct(v.utilizationPct, 1)}</Td>
                  <Td className="text-right tabular">{v.missions}</Td>
                  <Td className="text-right tabular">{fmtDistance(v.distanceM)}</Td>
                  <Td><BatteryBar value={v.batteryChargePct} /></Td>
                  <Td className="text-right tabular">{v.alarms}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </Card>
    </div>
  );
}
