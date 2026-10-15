"use client";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { Trash2 } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { can } from "@/lib/auth";
import { fmtCoord, fmtDateTime, fmtDeg, fmtDistance, fmtNum, fmtRelative, toLocalInput } from "@/lib/format";
import { OPERATING_MODE_LABELS, label } from "@/lib/labels";
import { useApi } from "@/hooks/use-api";
import { useNow } from "@/hooks/use-now";
import { useNodeName } from "@/hooks/use-node-names";
import { useAuthStore } from "@/stores/auth";
import { useFleetStore } from "@/stores/fleet";
import { useSiteStore } from "@/stores/site";
import { toast } from "@/stores/ui";
import { PageHeader, Card, CardHeader, CardBody, BatteryBar, DescriptionList, Spinner, ErrorBox, EmptyState } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/dialog";
import { AvailabilityBadge, ConnectionBadge, Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { VehicleCommands } from "@/components/vehicles/vehicle-commands";
import { VehicleForm, valuesToVehicle, vehicleToValues } from "@/components/vehicles/vehicle-form";
import { VehicleHistoryChart } from "@/components/charts/vehicle-history-chart";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";

export default function VehicleDetailPage() {
  const params = useParams<{ id: string }>();
  const id = decodeURIComponent(params.id);
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const siteId = useSiteStore((s) => s.siteId);
  const now = useNow(1000);
  const nodeName = useNodeName();
  const live = useFleetStore((s) => s.vehicles.find((v) => v.vehicle.id === id));
  const fetched = useApi(() => api.vehicles.get(id), [id]);
  const models = useApi(() => api.vehicleModels.list(), []);
  const activeMap = useFleetStore((s) => s.activeMap);
  const item = live ?? fetched.data;
  const [from, setFrom] = useState(() => toLocalInput(new Date(Date.now() - 3600_000)));
  const [to, setTo] = useState(() => toLocalInput(new Date()));
  const history = useApi(
    () => api.vehicles.history(id, new Date(from).toISOString(), new Date(to).toISOString()),
    [id, from, to],
  );
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const formInitial = useMemo(() => vehicleToValues(item?.vehicle), [item?.vehicle]);

  if (!item) {
    if (fetched.error) return <ErrorBox message={errorMessage(fetched.error)} />;
    return <Spinner />;
  }
  const { vehicle, state } = item;
  const speed = Math.hypot(state.velocity?.vx ?? 0, state.velocity?.vy ?? 0);

  const doDelete = async () => {
    setDeleting(true);
    try {
      await api.vehicles.remove(vehicle.id);
      toast.success("Véhicule supprimé", vehicle.name);
      router.push("/vehicles");
    } catch (e) {
      toast.error("Suppression refusée", errorMessage(e));
    } finally {
      setDeleting(false);
      setConfirmDelete(false);
    }
  };

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title={vehicle.name}
        subtitle={
          <span className="flex items-center gap-2 flex-wrap">
            <span>{vehicle.id} · {vehicle.modelId} · {vehicle.protocol}</span>
            <AvailabilityBadge value={state.availability} />
            <ConnectionBadge value={state.connection} />
            {!vehicle.enabled && <Badge tone="muted">Désactivé</Badge>}
          </span>
        }
        actions={
          <>
            <Link href="/map"><Button size="sm" variant="ghost">Voir sur la carte</Button></Link>
            {can(user, "vehicles.manage") && (
              <Button size="sm" variant="danger" onClick={() => setConfirmDelete(true)} disabled={!!state.missionId}>
                <Trash2 className="size-3.5" /> Supprimer
              </Button>
            )}
          </>
        }
      />

      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-3">
        <Card>
          <CardHeader title="État" />
          <CardBody>
            <DescriptionList
              items={[
                ["Mode", label(OPERATING_MODE_LABELS, state.operatingMode)],
                ["En mouvement", state.driving ? "oui" : "non"],
                ["En pause", state.paused ? "oui" : "non"],
                ["Position", `${fmtCoord(state.pose?.x)} ; ${fmtCoord(state.pose?.y)}`],
                ["Cap", fmtDeg(state.pose?.theta)],
                ["Vitesse", `${fmtNum(speed, 2)} m/s · ω ${fmtNum(state.velocity?.omega, 2)}`],
                ["Localisation", `${fmtNum((state.localizationScore ?? 0) * 100, 0)} % ${state.positionInitialized ? "" : "(non initialisée)"}`],
                ["Dernier nœud", `${nodeName(state.lastNodeId)} (#${state.lastNodeSequenceId ?? 0})`],
                ["Mission", state.missionId ? <Link className="text-accent hover:underline" href={`/missions/${encodeURIComponent(state.missionId)}`}>{state.missionId}</Link> : "—"],
                ["Ordre VDA", state.orderId || "—"],
                ["Dernière vue", fmtRelative(state.lastSeen, now)],
              ]}
            />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Batterie" />
          <CardBody className="flex flex-col gap-2">
            <BatteryBar value={state.batteryCharge} charging={state.charging} />
            <DescriptionList
              items={[
                ["Tension", `${fmtNum(state.batteryVoltage, 1)} V`],
                ["Santé", `${fmtNum(state.batteryHealth, 0)} %`],
                ["En charge", state.charging ? "oui" : "non"],
              ]}
            />
            <h3 className="text-[11px] uppercase tracking-wide text-fg-muted mt-2">Compteurs</h3>
            <DescriptionList
              items={[
                ["Odomètre", fmtDistance(state.odometerM)],
                ["Heures", `${fmtNum(state.operatingHours, 1)} h`],
                ["Missions terminées", String(state.missionsCompleted ?? 0)],
              ]}
            />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Sécurité et erreurs" />
          <CardBody className="flex flex-col gap-2">
            <DescriptionList
              items={[
                ["Arrêt d’urgence", state.safetyState?.eStop === "none" ? <span className="text-success">aucun</span> : <span className="text-danger">{state.safetyState?.eStop}</span>],
                ["Violation de champ", state.safetyState?.fieldViolation ? <span className="text-danger">oui</span> : "non"],
                ["Marge sécurité", `${fmtNum(vehicle.safetyMarginM, 2)} m`],
                ["Enveloppe", `${fmtNum(state.loadEnvelope?.lengthM, 2)} × ${fmtNum(state.loadEnvelope?.widthM, 2)} m`],
              ]}
            />
            {state.errors.length === 0 ? (
              <p className="text-[11px] text-fg-subtle">Aucune erreur signalée.</p>
            ) : (
              <ul className="flex flex-col gap-1 text-[11px]">
                {state.errors.map((e, i) => (
                  <li key={i} className="rounded border border-border px-2 py-1">
                    <span className={e.errorLevel === "fatal" ? "text-danger font-medium" : "text-warning font-medium"}>{e.errorType}</span> — {e.errorDescription}
                    {e.errorHint && <div className="text-fg-muted">{e.errorHint}</div>}
                  </li>
                ))}
              </ul>
            )}
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Charges" />
          <CardBody>
            <DescriptionList items={[["Charge présente", state.loadPresent ? "oui" : "non"], ["Masse", `${fmtNum(state.loadMassKg, 0)} kg`]]} />
            {state.loads?.length > 0 && (
              <Table className="mt-2">
                <THead><tr><Th>ID</Th><Th>Type</Th><Th className="text-right">Poids</Th></tr></THead>
                <TBody>
                  {state.loads.map((l) => (
                    <Tr key={l.loadId}><Td>{l.loadId}</Td><Td>{l.loadType}</Td><Td className="text-right tabular">{fmtNum(l.weightKg, 0)} kg</Td></Tr>
                  ))}
                </TBody>
              </Table>
            )}
          </CardBody>
        </Card>
      </div>

      <Card>
        <CardHeader title="Commandes" />
        <CardBody>
          <VehicleCommands item={item} nodes={activeMap?.nodes ?? []} />
        </CardBody>
      </Card>

      <Card>
        <CardHeader
          title="Historique"
          actions={
            <div className="flex items-center gap-1 text-xs">
              <Input type="datetime-local" aria-label="Début" value={from} onChange={(e) => setFrom(e.target.value)} className="h-7 w-44" />
              <span className="text-fg-subtle">→</span>
              <Input type="datetime-local" aria-label="Fin" value={to} onChange={(e) => setTo(e.target.value)} className="h-7 w-44" />
            </div>
          }
        />
        <CardBody>
          {history.loading && !history.data ? <Spinner /> : history.error ? <ErrorBox message={errorMessage(history.error)} /> : <VehicleHistoryChart samples={history.data ?? []} />}
        </CardBody>
      </Card>

      {can(user, "vehicles.manage") ? (
        <Card>
          <CardHeader title="Configuration" subtitle={`Créé le ${fmtDateTime(vehicle.createdAt)} · modifié le ${fmtDateTime(vehicle.updatedAt)}`} />
          <CardBody>
            {models.data ? (
              <VehicleForm
                key={vehicle.updatedAt}
                initial={formInitial}
                models={models.data}
                submitLabel="Enregistrer"
                loading={saving}
                error={saveError}
                onSubmit={async (values) => {
                  setSaving(true);
                  setSaveError(null);
                  try {
                    await api.vehicles.update(vehicle.id, { ...valuesToVehicle(values, vehicle.siteId || siteId), id: vehicle.id });
                    toast.success("Configuration enregistrée", vehicle.name);
                    void fetched.refetch();
                  } catch (e) {
                    setSaveError(errorMessage(e));
                  } finally {
                    setSaving(false);
                  }
                }}
              />
            ) : (
              <Spinner />
            )}
          </CardBody>
        </Card>
      ) : (
        <Card>
          <CardHeader title="Configuration" />
          <CardBody>
            <DescriptionList
              items={[
                ["Fabricant", vehicle.manufacturer || "—"],
                ["N° de série", vehicle.serialNumber || "—"],
                ["Incertitude localisation", `${fmtNum(vehicle.localizationUncertaintyM, 2)} m`],
                ["Empreinte", vehicle.footprint ? `${vehicle.footprint.lengthM} × ${vehicle.footprint.widthM} m` : "modèle"],
                ["Tags", vehicle.tags?.join(", ") || "—"],
              ]}
            />
          </CardBody>
        </Card>
      )}
      {!live && !fetched.data && <EmptyState>Véhicule introuvable.</EmptyState>}

      <ConfirmDialog
        open={confirmDelete}
        onClose={() => setConfirmDelete(false)}
        onConfirm={doDelete}
        title="Supprimer le véhicule"
        message={`Supprimer définitivement le véhicule « ${vehicle.name} » ? Impossible si une mission est active.`}
        confirmLabel="Supprimer"
        danger
        loading={deleting}
      />
    </div>
  );
}
