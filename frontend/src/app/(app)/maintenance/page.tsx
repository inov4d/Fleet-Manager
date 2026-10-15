"use client";
import { useMemo, useState } from "react";
import { ClipboardCheck, Pencil, Plus, Trash2 } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { can } from "@/lib/auth";
import type { MaintenanceMetric, MaintenancePlan, MaintenanceStatus } from "@/lib/types";
import { MAINTENANCE_METRIC_LABELS, label } from "@/lib/labels";
import { fmtDateTime, fmtNum } from "@/lib/format";
import { useApi, useInterval } from "@/hooks/use-api";
import { useVehicleName } from "@/hooks/use-node-names";
import { useAuthStore } from "@/stores/auth";
import { useFleetStore } from "@/stores/fleet";
import { useSiteStore } from "@/stores/site";
import { toast } from "@/stores/ui";
import { PageHeader, Card, CardHeader, EmptyState, ErrorBox, ProgressBar, Spinner } from "@/components/ui/card";
import { Button, IconButton } from "@/components/ui/button";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { MaintenanceBadge } from "@/components/ui/badge";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";

interface PlanForm {
  name: string;
  scope: "all" | "vehicle" | "model";
  vehicleId: string;
  modelId: string;
  metric: MaintenanceMetric;
  intervalValue: number;
  dueSoonFraction: number;
}

const emptyPlan: PlanForm = { name: "", scope: "all", vehicleId: "", modelId: "", metric: "operatingHours", intervalValue: 500, dueSoonFraction: 0.9 };

export default function MaintenancePage() {
  const siteId = useSiteStore((s) => s.siteId);
  const user = useAuthStore((s) => s.user);
  const canManage = can(user, "maintenance.manage");
  const vehicles = useFleetStore((s) => s.vehicles);
  const vehicleName = useVehicleName();
  const plans = useApi(() => api.maintenance.plans(), []);
  const status = useApi(() => api.maintenance.status(siteId), [siteId]);
  const models = useApi(() => api.vehicleModels.list(), []);
  useInterval(() => void status.refetch(), 30000);

  const [editing, setEditing] = useState<{ id: string | null; form: PlanForm } | null>(null);
  const [toDelete, setToDelete] = useState<MaintenancePlan | null>(null);
  const [record, setRecord] = useState<{ vehicleId: string; planId: string; notes: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const planById = useMemo(() => new Map((plans.data ?? []).map((p) => [p.id, p])), [plans.data]);
  const rows = useMemo(() => {
    const order = { overdue: 0, dueSoon: 1, ok: 2 };
    return [...(status.data ?? [])].sort((a, b) => order[a.state] - order[b.state] || a.remaining - b.remaining);
  }, [status.data]);
  const counts = rows.reduce<Record<MaintenanceStatus["state"], number>>((acc, r) => { acc[r.state]++; return acc; }, { ok: 0, dueSoon: 0, overdue: 0 });

  const openEdit = (p?: MaintenancePlan) =>
    setEditing({
      id: p?.id ?? null,
      form: p
        ? { name: p.name, scope: p.vehicleId ? "vehicle" : p.modelId ? "model" : "all", vehicleId: p.vehicleId ?? "", modelId: p.modelId ?? "", metric: p.metric, intervalValue: p.intervalValue, dueSoonFraction: p.dueSoonFraction }
        : { ...emptyPlan },
    });

  const savePlan = async () => {
    if (!editing) return;
    const f = editing.form;
    const body: Partial<MaintenancePlan> = {
      name: f.name.trim(),
      vehicleId: f.scope === "vehicle" ? f.vehicleId : null,
      modelId: f.scope === "model" ? f.modelId : null,
      metric: f.metric,
      intervalValue: f.intervalValue,
      dueSoonFraction: f.dueSoonFraction,
    };
    setBusy(true);
    try {
      if (editing.id) await api.maintenance.updatePlan(editing.id, body);
      else await api.maintenance.createPlan(body);
      toast.success("Plan enregistré", body.name);
      setEditing(null);
      void plans.refetch();
      void status.refetch();
    } catch (e) {
      toast.error("Enregistrement refusé", errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const deletePlan = async () => {
    if (!toDelete) return;
    setBusy(true);
    try {
      await api.maintenance.removePlan(toDelete.id);
      toast.success("Plan supprimé", toDelete.name);
      setToDelete(null);
      void plans.refetch();
      void status.refetch();
    } catch (e) {
      toast.error("Suppression refusée", errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const saveRecord = async () => {
    if (!record) return;
    setBusy(true);
    try {
      await api.maintenance.record(record);
      toast.success("Maintenance enregistrée", "Compteur réinitialisé");
      setRecord(null);
      void status.refetch();
      void plans.refetch();
    } catch (e) {
      toast.error("Enregistrement refusé", errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const setF = (patch: Partial<PlanForm>) => setEditing((e) => (e ? { ...e, form: { ...e.form, ...patch } } : e));

  return (
    <div className="flex flex-col gap-3">
      <PageHeader
        title="Maintenance"
        subtitle={`${counts.overdue} en retard · ${counts.dueSoon} bientôt · ${counts.ok} OK`}
        actions={
          canManage && (
            <>
              <Button size="sm" onClick={() => setRecord({ vehicleId: vehicles[0]?.vehicle.id ?? "", planId: plans.data?.[0]?.id ?? "", notes: "" })} disabled={!plans.data?.length}>
                <ClipboardCheck className="size-3.5" /> Enregistrer une maintenance
              </Button>
              <Button size="sm" variant="primary" onClick={() => openEdit()}>
                <Plus className="size-3.5" /> Nouveau plan
              </Button>
            </>
          )
        }
      />

      <Card>
        <CardHeader title="État par véhicule" subtitle="Compteurs courants et prochaines échéances" />
        {status.error && <ErrorBox message={errorMessage(status.error)} />}
        {status.loading && !status.data ? (
          <Spinner />
        ) : rows.length === 0 ? (
          <EmptyState>Aucun plan applicable.</EmptyState>
        ) : (
          <Table>
            <THead>
              <tr>
                <Th>État</Th>
                <Th>Véhicule</Th>
                <Th>Plan</Th>
                <Th>Métrique</Th>
                <Th className="text-right">Valeur</Th>
                <Th className="text-right">Échéance</Th>
                <Th className="text-right">Restant</Th>
                <Th className="w-40">Avancement</Th>
                <Th className="text-right">Action</Th>
              </tr>
            </THead>
            <TBody>
              {rows.map((r) => {
                const plan = planById.get(r.planId);
                const interval = plan?.intervalValue ?? r.nextDueValue;
                const pct = interval > 0 ? Math.max(0, Math.min(100, ((interval - r.remaining) / interval) * 100)) : 0;
                return (
                  <Tr key={`${r.planId}-${r.vehicleId}`}>
                    <Td><MaintenanceBadge value={r.state} /></Td>
                    <Td className="font-medium">{vehicleName(r.vehicleId)}</Td>
                    <Td>{plan?.name ?? r.planId}</Td>
                    <Td>{plan ? label(MAINTENANCE_METRIC_LABELS, plan.metric) : "—"}</Td>
                    <Td className="text-right tabular">{fmtNum(r.currentValue, 1)}</Td>
                    <Td className="text-right tabular">{fmtNum(r.nextDueValue, 1)}</Td>
                    <Td className={`text-right tabular ${r.remaining < 0 ? "text-danger" : ""}`}>{fmtNum(r.remaining, 1)}</Td>
                    <Td><ProgressBar value={pct} tone={r.state === "overdue" ? "danger" : r.state === "dueSoon" ? "warning" : "success"} label="Avancement" /></Td>
                    <Td className="text-right">
                      {canManage && (
                        <Button size="xs" onClick={() => setRecord({ vehicleId: r.vehicleId, planId: r.planId, notes: "" })}>
                          <ClipboardCheck className="size-3" /> Effectuée
                        </Button>
                      )}
                    </Td>
                  </Tr>
                );
              })}
            </TBody>
          </Table>
        )}
      </Card>

      <Card>
        <CardHeader title="Plans de maintenance" subtitle={`${plans.data?.length ?? 0} plans`} />
        {plans.error && <ErrorBox message={errorMessage(plans.error)} />}
        {(plans.data ?? []).length === 0 ? (
          <EmptyState>Aucun plan défini.</EmptyState>
        ) : (
          <Table>
            <THead>
              <tr>
                <Th>Nom</Th>
                <Th>Portée</Th>
                <Th>Métrique</Th>
                <Th className="text-right">Intervalle</Th>
                <Th className="text-right">Seuil « bientôt »</Th>
                <Th>Dernière réalisation</Th>
                {canManage && <Th className="text-right">Actions</Th>}
              </tr>
            </THead>
            <TBody>
              {(plans.data ?? []).map((p) => (
                <Tr key={p.id}>
                  <Td className="font-medium">{p.name}</Td>
                  <Td>{p.vehicleId ? `Véhicule ${vehicleName(p.vehicleId)}` : p.modelId ? `Modèle ${p.modelId}` : "Tous les véhicules"}</Td>
                  <Td>{label(MAINTENANCE_METRIC_LABELS, p.metric)}</Td>
                  <Td className="text-right tabular">{fmtNum(p.intervalValue, 0)}</Td>
                  <Td className="text-right tabular">{Math.round(p.dueSoonFraction * 100)} %</Td>
                  <Td className="text-fg-muted">{fmtDateTime(p.lastDoneAt)} {p.lastDoneValue ? `(${fmtNum(p.lastDoneValue, 0)})` : ""}</Td>
                  {canManage && (
                    <Td className="text-right whitespace-nowrap">
                      <IconButton label="Modifier" size="xs" onClick={() => openEdit(p)}><Pencil className="size-3.5" /></IconButton>
                      <IconButton label="Supprimer" size="xs" className="text-danger" onClick={() => setToDelete(p)}><Trash2 className="size-3.5" /></IconButton>
                    </Td>
                  )}
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </Card>

      <Dialog
        open={!!editing}
        onClose={() => setEditing(null)}
        title={editing?.id ? "Modifier le plan" : "Nouveau plan de maintenance"}
        footer={
          <>
            <Button variant="ghost" onClick={() => setEditing(null)}>Annuler</Button>
            <Button variant="primary" onClick={savePlan} loading={busy} disabled={!editing?.form.name.trim()}>Enregistrer</Button>
          </>
        }
      >
        {editing && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Nom" htmlFor="pl-name" required className="col-span-2"><Input id="pl-name" value={editing.form.name} onChange={(e) => setF({ name: e.target.value })} /></Field>
            <Field label="Portée" htmlFor="pl-scope">
              <Select id="pl-scope" value={editing.form.scope} onChange={(e) => setF({ scope: e.target.value as PlanForm["scope"] })} options={[{ value: "all", label: "Tous les véhicules" }, { value: "model", label: "Un modèle" }, { value: "vehicle", label: "Un véhicule" }]} />
            </Field>
            {editing.form.scope === "vehicle" && (
              <Field label="Véhicule" htmlFor="pl-veh"><Select id="pl-veh" value={editing.form.vehicleId} onChange={(e) => setF({ vehicleId: e.target.value })} placeholder="Choisir…" options={vehicles.map((v) => ({ value: v.vehicle.id, label: v.vehicle.name }))} /></Field>
            )}
            {editing.form.scope === "model" && (
              <Field label="Modèle" htmlFor="pl-model"><Select id="pl-model" value={editing.form.modelId} onChange={(e) => setF({ modelId: e.target.value })} placeholder="Choisir…" options={(models.data ?? []).map((m) => ({ value: m.id, label: m.name }))} /></Field>
            )}
            {editing.form.scope === "all" && <div />}
            <Field label="Métrique" htmlFor="pl-metric">
              <Select id="pl-metric" value={editing.form.metric} onChange={(e) => setF({ metric: e.target.value as MaintenanceMetric })} options={(Object.keys(MAINTENANCE_METRIC_LABELS) as MaintenanceMetric[]).map((k) => ({ value: k, label: MAINTENANCE_METRIC_LABELS[k] }))} />
            </Field>
            <Field label="Intervalle" htmlFor="pl-int"><Input id="pl-int" type="number" min={1} value={editing.form.intervalValue} onChange={(e) => setF({ intervalValue: Number(e.target.value) })} /></Field>
            <Field label="Seuil « bientôt » (0–1)" htmlFor="pl-frac"><Input id="pl-frac" type="number" min={0} max={1} step={0.05} value={editing.form.dueSoonFraction} onChange={(e) => setF({ dueSoonFraction: Number(e.target.value) })} /></Field>
          </div>
        )}
      </Dialog>

      <Dialog
        open={!!record}
        onClose={() => setRecord(null)}
        title="Enregistrer une maintenance"
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setRecord(null)}>Annuler</Button>
            <Button variant="primary" onClick={saveRecord} loading={busy} disabled={!record?.vehicleId || !record?.planId}>Enregistrer</Button>
          </>
        }
      >
        {record && (
          <div className="flex flex-col gap-3">
            <Field label="Véhicule" htmlFor="rc-veh"><Select id="rc-veh" value={record.vehicleId} onChange={(e) => setRecord({ ...record, vehicleId: e.target.value })} placeholder="Choisir…" options={vehicles.map((v) => ({ value: v.vehicle.id, label: v.vehicle.name }))} /></Field>
            <Field label="Plan" htmlFor="rc-plan"><Select id="rc-plan" value={record.planId} onChange={(e) => setRecord({ ...record, planId: e.target.value })} placeholder="Choisir…" options={(plans.data ?? []).map((p) => ({ value: p.id, label: p.name }))} /></Field>
            <Field label="Notes" htmlFor="rc-notes"><Textarea id="rc-notes" value={record.notes} onChange={(e) => setRecord({ ...record, notes: e.target.value })} /></Field>
            <p className="text-[11px] text-fg-subtle">L’enregistrement réinitialise le compteur du plan pour ce véhicule.</p>
          </div>
        )}
      </Dialog>

      <ConfirmDialog open={!!toDelete} onClose={() => setToDelete(null)} onConfirm={deletePlan} title="Supprimer le plan" message={`Supprimer le plan « ${toDelete?.name} » ?`} confirmLabel="Supprimer" danger loading={busy} />
    </div>
  );
}
