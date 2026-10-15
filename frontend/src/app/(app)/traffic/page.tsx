"use client";
import { useMemo, useState } from "react";
import { RefreshCw, Unlock } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { can } from "@/lib/auth";
import type { BlockedVehicle, Reservation, TrafficSettings } from "@/lib/types";
import { fmtDateTime, fmtRelative } from "@/lib/format";
import { useApi, useInterval } from "@/hooks/use-api";
import { useNow } from "@/hooks/use-now";
import { useNodeName, useVehicleName } from "@/hooks/use-node-names";
import { useAuthStore } from "@/stores/auth";
import { useFleetStore } from "@/stores/fleet";
import { useSiteStore } from "@/stores/site";
import { toast } from "@/stores/ui";
import { PageHeader, Card, CardHeader, CardBody, EmptyState, ErrorBox } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input } from "@/components/ui/input";
import { Badge, ReservationBadge } from "@/components/ui/badge";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { vehicleColorCss } from "@/lib/utils";

export default function TrafficPage() {
  const siteId = useSiteStore((s) => s.siteId);
  const user = useAuthStore((s) => s.user);
  const now = useNow(1000);
  const nodeName = useNodeName();
  const vehicleName = useVehicleName();
  const liveReservations = useFleetStore((s) => s.reservations);
  const vehicles = useFleetStore((s) => s.vehicles);
  const deadlocks = useApi(() => api.traffic.deadlocks(siteId), [siteId]);
  const blocked = useApi(() => api.traffic.blocked(), [siteId]);
  const settings = useApi(() => api.settings.getTraffic(), []);
  useInterval(() => {
    void deadlocks.refetch();
    void blocked.refetch();
  }, 5000);
  const [toRelease, setToRelease] = useState<Reservation | null>(null);
  const [verified, setVerified] = useState(false);
  const [releasing, setReleasing] = useState(false);

  const grouped = useMemo(() => {
    const g = new Map<string, Reservation[]>();
    for (const r of liveReservations) {
      const arr = g.get(r.vehicleId) ?? [];
      arr.push(r);
      g.set(r.vehicleId, arr);
    }
    for (const arr of g.values()) arr.sort((a, b) => a.sequence - b.sequence);
    return Array.from(g.entries()).sort((a, b) => vehicleName(a[0]).localeCompare(vehicleName(b[0])));
  }, [liveReservations, vehicleName]);

  const blockedList: BlockedVehicle[] = useMemo(() => {
    const fromApi = (blocked.data ?? []).map((b) => (typeof b === "string" ? { vehicleId: b } : b));
    if (fromApi.length > 0) return fromApi;
    return vehicles.filter((v) => v.state.availability === "blocked").map((v) => ({ vehicleId: v.vehicle.id }));
  }, [blocked.data, vehicles]);

  const resourceLabel = (r: Reservation) =>
    r.resource.type === "node" ? `Nœud ${nodeName(r.resource.id)}` : r.resource.type === "edge" ? `Arête ${r.resource.id}` : `Zone ${r.resource.id}`;

  const release = async () => {
    if (!toRelease) return;
    if (!verified) {
      toast.warning("Vérification requise", "Confirmez la présence physique avant de libérer la réservation.");
      return;
    }
    setReleasing(true);
    try {
      await api.traffic.release(toRelease.id);
      toast.success("Réservation libérée", toRelease.id);
      setToRelease(null);
      setVerified(false);
    } catch (e) {
      toast.error("Libération refusée", errorMessage(e));
    } finally {
      setReleasing(false);
    }
  };

  const uncertainCount = liveReservations.filter((r) => r.state === "uncertain").length;

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title="Trafic"
        subtitle={`${liveReservations.length} réservations · ${uncertainCount} incertaines · ${blockedList.length} véhicules bloqués`}
        actions={
          <Button size="sm" variant="ghost" onClick={() => { void deadlocks.refetch(); void blocked.refetch(); }}>
            <RefreshCw className="size-3.5" /> Actualiser
          </Button>
        }
      />
      <div className="grid grid-cols-1 xl:grid-cols-3 gap-3">
        <Card className="xl:col-span-2">
          <CardHeader title="Réservations par véhicule" subtitle="Temps réel (instantané WebSocket)" />
          {grouped.length === 0 ? (
            <EmptyState>Aucune réservation active.</EmptyState>
          ) : (
            <Table>
              <THead>
                <tr>
                  <Th>Véhicule</Th>
                  <Th className="text-right">Séq.</Th>
                  <Th>Ressource</Th>
                  <Th>État</Th>
                  <Th>Demandée</Th>
                  <Th>Accordée</Th>
                  <Th>Expire</Th>
                  <Th className="text-right">Action</Th>
                </tr>
              </THead>
              <TBody>
                {grouped.map(([vid, list]) =>
                  list.map((r, i) => (
                    <Tr key={r.id}>
                      <Td>
                        {i === 0 && (
                          <span className="inline-flex items-center gap-1.5 font-medium">
                            <span className="size-2.5 rounded-sm" style={{ background: vehicleColorCss(vid) }} aria-hidden />
                            {vehicleName(vid)}
                            <span className="text-fg-subtle font-normal">({list.length})</span>
                          </span>
                        )}
                      </Td>
                      <Td className="text-right tabular">{r.sequence}</Td>
                      <Td>{resourceLabel(r)}</Td>
                      <Td><ReservationBadge value={r.state} /></Td>
                      <Td className="text-fg-muted whitespace-nowrap">{fmtRelative(r.requestedAt, now)}</Td>
                      <Td className="text-fg-muted whitespace-nowrap">{fmtRelative(r.grantedAt, now)}</Td>
                      <Td className="text-fg-muted whitespace-nowrap">{r.expiresAt ? fmtRelative(r.expiresAt, now) : "—"}</Td>
                      <Td className="text-right">
                        {r.state === "uncertain" && can(user, "traffic.release") && (
                          <Button size="xs" variant="outline" onClick={() => { setToRelease(r); setVerified(false); }}>
                            <Unlock className="size-3" /> Libérer
                          </Button>
                        )}
                      </Td>
                    </Tr>
                  )),
                )}
              </TBody>
            </Table>
          )}
        </Card>

        <div className="flex flex-col gap-3">
          <Card>
            <CardHeader title="Véhicules bloqués" />
            {blockedList.length === 0 ? (
              <EmptyState className="py-4">Aucun véhicule bloqué.</EmptyState>
            ) : (
              <ul className="divide-y divide-border text-xs">
                {blockedList.map((b) => (
                  <li key={b.vehicleId} className="px-3 py-1.5 flex items-center justify-between gap-2">
                    <span className="font-medium">{vehicleName(b.vehicleId)}</span>
                    <span className="text-fg-muted truncate">{b.reason ?? (b.resourceId ? `ressource ${b.resourceId}` : "")} {b.since ? fmtRelative(b.since, now) : ""}</span>
                  </li>
                ))}
              </ul>
            )}
          </Card>

          <Card>
            <CardHeader title="Interblocages" subtitle={`${(deadlocks.data ?? []).filter((d) => !d.resolvedAt).length} non résolus`} />
            {deadlocks.error && <ErrorBox message={errorMessage(deadlocks.error)} />}
            {(deadlocks.data ?? []).length === 0 ? (
              <EmptyState className="py-4">Aucun interblocage détecté.</EmptyState>
            ) : (
              <ul className="divide-y divide-border text-xs max-h-72 overflow-y-auto">
                {(deadlocks.data ?? []).map((d) => (
                  <li key={d.id} className="px-3 py-2">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium">{d.vehicleIds.map(vehicleName).join(" ↔ ")}</span>
                      {d.resolvedAt ? <Badge tone="success">résolu</Badge> : <Badge tone="danger">actif</Badge>}
                    </div>
                    <div className="text-[11px] text-fg-muted">
                      {fmtDateTime(d.detectedAt)} · ressources : {d.resourceIds.join(", ") || "—"}
                    </div>
                    {d.resolution && <div className="text-[11px]">{d.resolution}</div>}
                  </li>
                ))}
              </ul>
            )}
          </Card>

          <TrafficSettingsCard initial={settings.data} canEdit={can(user, "settings.traffic")} onSaved={() => void settings.refetch()} />
        </div>
      </div>

      <ConfirmDialog
        open={!!toRelease}
        onClose={() => setToRelease(null)}
        onConfirm={release}
        title="Libérer une réservation incertaine"
        message={
          toRelease ? (
            <span>
              Libérer la réservation <strong>{resourceLabel(toRelease)}</strong> du véhicule <strong>{vehicleName(toRelease.vehicleId)}</strong> ? Cette action est consignée dans le journal d’audit.
            </span>
          ) : null
        }
        confirmLabel="Libérer"
        danger
        loading={releasing}
      >
        <div className="mt-3">
          <Checkbox label="Présence physique vérifiée sur site (confirmPhysicalState)" checked={verified} onChange={(e) => setVerified(e.target.checked)} />
          {!verified && <p className="text-[11px] text-warning mt-1">Cochez la case pour confirmer la vérification physique.</p>}
        </div>
      </ConfirmDialog>
    </div>
  );
}

function TrafficSettingsCard({ initial, canEdit, onSaved }: { initial: TrafficSettings | undefined; canEdit: boolean; onSaved: () => void }) {
  const [form, setForm] = useState<TrafficSettings | null>(null);
  const [saving, setSaving] = useState(false);
  const value = form ?? initial ?? null;
  if (!value) return null;
  const set = (patch: Partial<TrafficSettings>) => setForm({ ...value, ...patch });
  const save = async () => {
    setSaving(true);
    try {
      await api.settings.putTraffic(value);
      toast.success("Paramètres trafic enregistrés");
      setForm(null);
      onSaved();
    } catch (e) {
      toast.error("Enregistrement refusé", errorMessage(e));
    } finally {
      setSaving(false);
    }
  };
  return (
    <Card>
      <CardHeader title="Paramètres trafic" />
      <CardBody className="grid grid-cols-2 gap-2">
        <Field label="Segments d’anticipation" htmlFor="t-look"><Input id="t-look" type="number" min={0} value={value.lookaheadSegments} onChange={(e) => set({ lookaheadSegments: Number(e.target.value) })} disabled={!canEdit} /></Field>
        <Field label="Délai blocage (s)" htmlFor="t-blocked"><Input id="t-blocked" type="number" min={0} value={value.blockedTimeoutS} onChange={(e) => set({ blockedTimeoutS: Number(e.target.value) })} disabled={!canEdit} /></Field>
        <Field label="Délai comms (s)" htmlFor="t-comms"><Input id="t-comms" type="number" min={0} value={value.commsTimeoutS} onChange={(e) => set({ commsTimeoutS: Number(e.target.value) })} disabled={!canEdit} /></Field>
        <Field label="Échantillons balayage" htmlFor="t-swept"><Input id="t-swept" type="number" min={1} value={value.swept?.samples ?? 0} onChange={(e) => set({ swept: { samples: Number(e.target.value) } })} disabled={!canEdit} /></Field>
        {canEdit && (
          <div className="col-span-2 flex justify-end">
            <Button size="sm" variant="primary" onClick={save} loading={saving} disabled={!form}>Enregistrer</Button>
          </div>
        )}
      </CardBody>
    </Card>
  );
}
