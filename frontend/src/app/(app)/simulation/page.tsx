"use client";
import { useEffect, useMemo, useState } from "react";
import { Play, Plus, Square, Zap } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { can } from "@/lib/auth";
import type { FaultType, ScenarioName } from "@/lib/types";
import { FAULT_LABELS } from "@/lib/labels";
import { fmtDateTime } from "@/lib/format";
import { useApi, useInterval } from "@/hooks/use-api";
import { useVehicleName } from "@/hooks/use-node-names";
import { useAuthStore } from "@/stores/auth";
import { useFleetStore } from "@/stores/fleet";
import { useSiteStore } from "@/stores/site";
import { toast } from "@/stores/ui";
import { PageHeader, Card, CardHeader, CardBody, DescriptionList, ErrorBox } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Field, Input, Select, Slider } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";

export default function SimulationPage() {
  const siteId = useSiteStore((s) => s.siteId);
  const user = useAuthStore((s) => s.user);
  const canManage = can(user, "simulation.manage");
  const live = useFleetStore((s) => s.simulation);
  const vehicles = useFleetStore((s) => s.vehicles);
  const vehicleName = useVehicleName();
  const fetched = useApi(() => api.simulation.status(), []);
  useInterval(() => void fetched.refetch(), 10000);
  const status = live ?? fetched.data ?? null;
  const maps = useApi(() => api.maps.list(siteId), [siteId]);
  const models = useApi(() => api.vehicleModels.list(), []);
  const activeMap = useFleetStore((s) => s.activeMap);

  const [mapId, setMapId] = useState("");
  const [count, setCount] = useState(5);
  const [mix, setMix] = useState<Record<string, number>>({ AGV20T: 50, AGV40T: 50 });
  const [timeScale, setTimeScale] = useState(1);
  const [scenario, setScenario] = useState<ScenarioName>("randomTransport");
  const [rate, setRate] = useState(2);
  const [spawnModel, setSpawnModel] = useState("AGV20T");
  const [spawnNode, setSpawnNode] = useState("");
  const [faultVehicle, setFaultVehicle] = useState("");
  const [faultType, setFaultType] = useState<FaultType>("blocked");
  const [busy, setBusy] = useState<string | null>(null);
  const [liveScale, setLiveScale] = useState<number | null>(null);

  useEffect(() => {
    if (models.data) {
      setMix((m) => {
        const next: Record<string, number> = {};
        for (const md of models.data!) next[md.id] = m[md.id] ?? 0;
        if (Object.values(next).every((v) => v === 0)) for (const md of models.data!) next[md.id] = Math.round(100 / models.data!.length);
        return next;
      });
    }
  }, [models.data]);

  const mixTotal = useMemo(() => Object.values(mix).reduce((a, b) => a + b, 0), [mix]);

  const run = async (key: string, fn: () => Promise<unknown>, ok: string) => {
    setBusy(key);
    try {
      await fn();
      toast.success(ok);
      void fetched.refetch();
    } catch (e) {
      toast.error("Échec", errorMessage(e));
    } finally {
      setBusy(null);
    }
  };

  const start = () =>
    run(
      "start",
      () =>
        api.simulation.start({
          siteId,
          mapId: mapId || undefined,
          vehicles: count,
          modelMix: Object.fromEntries(Object.entries(mix).map(([k, v]) => [k, mixTotal > 0 ? v / mixTotal : 0])),
          timeScale,
          scenario: { name: scenario, missionRatePerMin: scenario === "none" ? 0 : rate },
        }),
      "Simulation démarrée",
    );

  const simVehicles = vehicles.filter((v) => v.vehicle.protocol === "simulated" || status?.vehicleIds?.includes(v.vehicle.id));

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title="Simulation"
        subtitle="Véhicules simulés via VDA 5050 sur le broker MQTT embarqué"
        actions={
          canManage &&
          status?.running && (
            <Button size="sm" variant="danger" onClick={() => run("stop", () => api.simulation.stop(), "Simulation arrêtée")} loading={busy === "stop"}>
              <Square className="size-3.5" /> Arrêter
            </Button>
          )
        }
      />
      {fetched.error && !status && <ErrorBox message={errorMessage(fetched.error)} />}
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-3">
        <Card>
          <CardHeader title="État" actions={status?.running ? <Badge tone="success">en cours</Badge> : <Badge tone="muted">arrêtée</Badge>} />
          <CardBody className="flex flex-col gap-3">
            <DescriptionList
              items={[
                ["Échelle de temps", status ? `×${status.timeScale}` : "—"],
                ["Temps simulé", fmtDateTime(status?.simTime)],
                ["Carte", status?.mapId || "—"],
                ["Véhicules", String(status?.vehicleIds?.length ?? 0)],
                ["Scénario", status?.scenario ? `${status.scenario.name} · ${status.scenario.missionRatePerMin} missions/min` : "aucun"],
                ["Pannes actives", String(status?.faults?.length ?? 0)],
              ]}
            />
            {canManage && status?.running && (
              <>
                <Field label={`Échelle de temps : ×${liveScale ?? status.timeScale}`} htmlFor="s-scale">
                  <Slider id="s-scale" min={0.1} max={20} step={0.1} value={liveScale ?? status.timeScale} onChange={(e) => setLiveScale(Number(e.target.value))} />
                </Field>
                <div className="flex gap-1">
                  {[0.5, 1, 2, 5, 10].map((v) => (
                    <Button key={v} size="xs" onClick={() => setLiveScale(v)}>×{v}</Button>
                  ))}
                  <Button size="xs" variant="primary" className="ml-auto" disabled={liveScale === null} loading={busy === "scale"} onClick={() => run("scale", () => api.simulation.timescale(liveScale ?? 1).then(() => setLiveScale(null)), "Échelle de temps modifiée")}>
                    Appliquer
                  </Button>
                </div>
                <Field label="Scénario" htmlFor="s-scn2">
                  <div className="flex gap-1">
                    <Select id="s-scn2" value={scenario} onChange={(e) => setScenario(e.target.value as ScenarioName)} options={[{ value: "randomTransport", label: "Transports aléatoires" }, { value: "none", label: "Aucun" }]} />
                    <Input type="number" min={0} step={0.5} value={rate} onChange={(e) => setRate(Number(e.target.value))} aria-label="Missions par minute" className="w-20" />
                    <Button size="md" loading={busy === "scenario"} onClick={() => run("scenario", () => api.simulation.scenario(scenario, rate), "Scénario modifié")}>OK</Button>
                  </div>
                </Field>
              </>
            )}
          </CardBody>
        </Card>

        {canManage && !status?.running && (
          <Card className="xl:col-span-2">
            <CardHeader title="Démarrer une simulation" />
            <CardBody className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Field label="Site" htmlFor="s-site"><Input id="s-site" value={siteId} readOnly /></Field>
              <Field label="Carte" htmlFor="s-map" hint="Vide = carte active du site.">
                <Select id="s-map" value={mapId} onChange={(e) => setMapId(e.target.value)} placeholder="Carte active" options={(maps.data ?? []).map((m) => ({ value: m.id, label: `${m.name}${m.active ? " (active)" : ""}` }))} />
              </Field>
              <Field label={`Nombre de véhicules : ${count}`} htmlFor="s-count"><Slider id="s-count" min={1} max={40} step={1} value={count} onChange={(e) => setCount(Number(e.target.value))} /></Field>
              <Field label={`Échelle de temps : ×${timeScale}`} htmlFor="s-ts"><Slider id="s-ts" min={0.1} max={20} step={0.1} value={timeScale} onChange={(e) => setTimeScale(Number(e.target.value))} /></Field>
              <fieldset className="md:col-span-2 rounded-md border border-border p-3">
                <legend className="text-xs text-fg-muted px-1">Répartition des modèles ({mixTotal} %)</legend>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
                  {(models.data ?? []).map((m) => (
                    <Field key={m.id} label={`${m.name} : ${mix[m.id] ?? 0} %`} htmlFor={`mix-${m.id}`}>
                      <Slider id={`mix-${m.id}`} min={0} max={100} step={5} value={mix[m.id] ?? 0} onChange={(e) => setMix((x) => ({ ...x, [m.id]: Number(e.target.value) }))} />
                    </Field>
                  ))}
                </div>
                {mixTotal !== 100 && <p className="text-[11px] text-warning mt-1">La répartition est normalisée à 100 % à l’envoi.</p>}
              </fieldset>
              <Field label="Scénario" htmlFor="s-scn">
                <Select id="s-scn" value={scenario} onChange={(e) => setScenario(e.target.value as ScenarioName)} options={[{ value: "randomTransport", label: "Transports aléatoires" }, { value: "none", label: "Aucun" }]} />
              </Field>
              <Field label="Missions par minute" htmlFor="s-rate"><Input id="s-rate" type="number" min={0} step={0.5} value={rate} onChange={(e) => setRate(Number(e.target.value))} disabled={scenario === "none"} /></Field>
              <div className="md:col-span-2 flex justify-end">
                <Button variant="primary" onClick={start} loading={busy === "start"} disabled={mixTotal === 0}>
                  <Play className="size-3.5" /> Démarrer
                </Button>
              </div>
            </CardBody>
          </Card>
        )}

        {canManage && status?.running && (
          <>
            <Card>
              <CardHeader title="Ajouter un véhicule" />
              <CardBody className="flex flex-col gap-3">
                <Field label="Modèle" htmlFor="sp-model"><Select id="sp-model" value={spawnModel} onChange={(e) => setSpawnModel(e.target.value)} options={(models.data ?? []).map((m) => ({ value: m.id, label: m.name }))} /></Field>
                <Field label="Nœud de départ" htmlFor="sp-node" hint="Vide = choisi par la simulation.">
                  <Select id="sp-node" value={spawnNode} onChange={(e) => setSpawnNode(e.target.value)} placeholder="Automatique" options={(activeMap?.nodes ?? []).map((n) => ({ value: n.id, label: n.name || n.id }))} />
                </Field>
                <Button onClick={() => run("spawn", () => api.simulation.spawn(spawnModel, spawnNode || undefined), "Véhicule ajouté")} loading={busy === "spawn"}>
                  <Plus className="size-3.5" /> Ajouter
                </Button>
              </CardBody>
            </Card>
            <Card>
              <CardHeader title="Injecter une panne" />
              <CardBody className="flex flex-col gap-3">
                <Field label="Véhicule" htmlFor="f-veh"><Select id="f-veh" value={faultVehicle} onChange={(e) => setFaultVehicle(e.target.value)} placeholder="Choisir…" options={simVehicles.map((v) => ({ value: v.vehicle.id, label: v.vehicle.name }))} /></Field>
                <Field label="Type" htmlFor="f-type"><Select id="f-type" value={faultType} onChange={(e) => setFaultType(e.target.value as FaultType)} options={(Object.keys(FAULT_LABELS) as FaultType[]).map((k) => ({ value: k, label: FAULT_LABELS[k] }))} /></Field>
                <Button variant={faultType === "restore" ? "primary" : "danger"} disabled={!faultVehicle} loading={busy === "fault"} onClick={() => run("fault", () => api.simulation.fault(faultVehicle, faultType), faultType === "restore" ? "Véhicule rétabli" : "Panne injectée")}>
                  <Zap className="size-3.5" /> {faultType === "restore" ? "Rétablir" : "Injecter"}
                </Button>
              </CardBody>
            </Card>
          </>
        )}
      </div>

      <Card>
        <CardHeader title="Véhicules simulés" subtitle={`${simVehicles.length} véhicules`} />
        <Table>
          <THead>
            <tr><Th>Véhicule</Th><Th>Modèle</Th><Th>Disponibilité</Th><Th>Pannes</Th>{canManage && status?.running && <Th className="text-right">Actions rapides</Th>}</tr>
          </THead>
          <TBody>
            {simVehicles.map((v) => {
              const faults = (status?.faults ?? []).filter((f) => f.vehicleId === v.vehicle.id);
              return (
                <Tr key={v.vehicle.id}>
                  <Td className="font-medium">{vehicleName(v.vehicle.id)}</Td>
                  <Td>{v.vehicle.modelId}</Td>
                  <Td>{v.state.availability}</Td>
                  <Td>{faults.length ? faults.map((f) => <Badge key={f.type} tone="danger" className="mr-1">{FAULT_LABELS[f.type] ?? f.type}</Badge>) : <span className="text-fg-subtle">—</span>}</Td>
                  {canManage && status?.running && (
                    <Td className="text-right whitespace-nowrap">
                      {(["commsLoss", "blocked", "error", "estop"] as FaultType[]).map((t) => (
                        <Button key={t} size="xs" variant="ghost" className="ml-0.5" onClick={() => run(`f-${v.vehicle.id}-${t}`, () => api.simulation.fault(v.vehicle.id, t), "Panne injectée")} loading={busy === `f-${v.vehicle.id}-${t}`}>
                          {FAULT_LABELS[t]}
                        </Button>
                      ))}
                      <Button size="xs" variant="outline" className="ml-0.5" onClick={() => run(`f-${v.vehicle.id}-restore`, () => api.simulation.fault(v.vehicle.id, "restore"), "Véhicule rétabli")} loading={busy === `f-${v.vehicle.id}-restore`}>
                        Rétablir
                      </Button>
                    </Td>
                  )}
                </Tr>
              );
            })}
            {simVehicles.length === 0 && (
              <Tr><Td colSpan={5} className="text-center text-fg-muted py-4">Aucun véhicule simulé.</Td></Tr>
            )}
          </TBody>
        </Table>
      </Card>
    </div>
  );
}
