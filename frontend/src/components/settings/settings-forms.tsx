"use client";
import { useState } from "react";
import { api, errorMessage } from "@/lib/api";
import type { BatterySettings, DispatcherSettings, TrafficSettings } from "@/lib/types";
import { useApi } from "@/hooks/use-api";
import { toast } from "@/stores/ui";
import { Card, CardHeader, CardBody, ErrorBox, Spinner } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Field, Input, Select, Slider } from "@/components/ui/input";

/** Generic settings form shell: loads with `load`, saves with `save`. */
function useSettingsForm<T>(load: () => Promise<T>, save: (v: T) => Promise<T>) {
  const q = useApi(load, []);
  const [draft, setDraft] = useState<T | null>(null);
  const [saving, setSaving] = useState(false);
  const value = draft ?? q.data ?? null;
  const doSave = async () => {
    if (!value) return;
    setSaving(true);
    try {
      await save(value);
      toast.success("Paramètres enregistrés");
      setDraft(null);
      void q.refetch();
    } catch (e) {
      toast.error("Enregistrement refusé", errorMessage(e));
    } finally {
      setSaving(false);
    }
  };
  return { value, setDraft, dirty: draft !== null, saving, doSave, loading: q.loading, error: q.error, reset: () => setDraft(null) };
}

export function DispatcherSettingsForm({ canEdit }: { canEdit: boolean }) {
  const f = useSettingsForm<DispatcherSettings>(api.settings.getDispatcher, api.settings.putDispatcher);
  if (f.error) return <ErrorBox message={errorMessage(f.error)} />;
  if (!f.value) return <Spinner />;
  const v = f.value;
  const setW = (k: keyof DispatcherSettings["weights"], n: number) => f.setDraft({ ...v, weights: { ...v.weights, [k]: n } });
  const W: Array<[keyof DispatcherSettings["weights"], string]> = [
    ["distance", "Distance"],
    ["travelTime", "Temps de trajet"],
    ["battery", "Batterie"],
    ["congestion", "Congestion"],
    ["queue", "File d’attente"],
  ];
  return (
    <Card>
      <CardHeader title="Répartiteur" subtitle="Mode d’attribution et pondération des critères" />
      <CardBody className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="md:col-span-2">
          <Field label="Mode d’attribution" htmlFor="d-mode" hint={v.mode === "colony" ? "Colonie : chaque AGV embarque un agent autonome qui choisit sa tâche sur le tableau partagé ; le centre arbitre les revendications simultanées et vérifie l’éligibilité." : "Central : le répartiteur du Fleet Manager note les candidats et affecte les missions."}>
            <Select
              id="d-mode"
              value={v.mode ?? "central"}
              onChange={(e) => f.setDraft({ ...v, mode: e.target.value as DispatcherSettings["mode"] })}
              disabled={!canEdit}
              options={[
                { value: "central", label: "Central (répartiteur)" },
                { value: "colony", label: "Colonie (agents autonomes, fourmilière)" },
              ]}
            />
          </Field>
        </div>
        <div className="flex flex-col gap-3">
          {W.map(([k, labelText]) => (
            <Field key={k} label={`${labelText} : ${v.weights[k]}`} htmlFor={`w-${k}`}>
              <Slider id={`w-${k}`} min={0} max={10} step={0.1} value={v.weights[k]} onChange={(e) => setW(k, Number(e.target.value))} disabled={!canEdit} />
            </Field>
          ))}
        </div>
        <div className="flex flex-col gap-3">
          <Field label="Batterie minimale pour accepter une mission (%)" htmlFor="d-min"><Input id="d-min" type="number" min={0} max={100} value={v.minBatteryPct} onChange={(e) => f.setDraft({ ...v, minBatteryPct: Number(e.target.value) })} disabled={!canEdit} /></Field>
          <Field label="Réserve de batterie (%)" htmlFor="d-res"><Input id="d-res" type="number" min={0} max={100} value={v.reserveBatteryPct} onChange={(e) => f.setDraft({ ...v, reserveBatteryPct: Number(e.target.value) })} disabled={!canEdit} /></Field>
          <Field label="Segments d’anticipation" htmlFor="d-look"><Input id="d-look" type="number" min={0} value={v.lookaheadSegments} onChange={(e) => f.setDraft({ ...v, lookaheadSegments: Number(e.target.value) })} disabled={!canEdit} /></Field>
        </div>
        {canEdit && (
          <div className="md:col-span-2 flex justify-end gap-2">
            <Button variant="ghost" onClick={f.reset} disabled={!f.dirty}>Réinitialiser</Button>
            <Button variant="primary" onClick={f.doSave} loading={f.saving} disabled={!f.dirty}>Enregistrer</Button>
          </div>
        )}
      </CardBody>
    </Card>
  );
}

export function BatterySettingsForm({ canEdit }: { canEdit: boolean }) {
  const f = useSettingsForm<BatterySettings>(api.settings.getBattery, api.settings.putBattery);
  if (f.error) return <ErrorBox message={errorMessage(f.error)} />;
  if (!f.value) return <Spinner />;
  const v = f.value;
  const fields: Array<[keyof BatterySettings, string, string]> = [
    ["lowPct", "Seuil bas (%)", "Déclenche un retour en charge après la mission."],
    ["criticalPct", "Seuil critique (%)", "Retour en charge immédiat, alarme."],
    ["targetPct", "Cible de charge (%)", "Niveau auquel la charge est considérée terminée."],
    ["opportunityPct", "Charge d’opportunité (%)", "En dessous, un véhicule inactif va se charger."],
    ["idleBeforeChargeS", "Inactivité avant charge (s)", "Durée d’inactivité avant charge d’opportunité."],
  ];
  return (
    <Card>
      <CardHeader title="Batterie" subtitle="Seuils de gestion de la charge" />
      <CardBody className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {fields.map(([k, labelText, hint]) => (
          <Field key={k} label={labelText} htmlFor={`b-${k}`} hint={hint}>
            <Input id={`b-${k}`} type="number" min={0} value={v[k]} onChange={(e) => f.setDraft({ ...v, [k]: Number(e.target.value) })} disabled={!canEdit} />
          </Field>
        ))}
        {canEdit && (
          <div className="md:col-span-2 flex justify-end gap-2">
            <Button variant="ghost" onClick={f.reset} disabled={!f.dirty}>Réinitialiser</Button>
            <Button variant="primary" onClick={f.doSave} loading={f.saving} disabled={!f.dirty}>Enregistrer</Button>
          </div>
        )}
      </CardBody>
    </Card>
  );
}

export function TrafficSettingsForm({ canEdit }: { canEdit: boolean }) {
  const f = useSettingsForm<TrafficSettings>(api.settings.getTraffic, api.settings.putTraffic);
  if (f.error) return <ErrorBox message={errorMessage(f.error)} />;
  if (!f.value) return <Spinner />;
  const v = f.value;
  return (
    <Card>
      <CardHeader title="Trafic" subtitle="Réservations, délais et balayage" />
      <CardBody className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Field label="Segments d’anticipation" htmlFor="t-look" hint="Nombre de segments réservés en avance."><Input id="t-look" type="number" min={0} value={v.lookaheadSegments} onChange={(e) => f.setDraft({ ...v, lookaheadSegments: Number(e.target.value) })} disabled={!canEdit} /></Field>
        <Field label="Délai blocage (s)" htmlFor="t-blk" hint="Durée immobile avant de déclarer un véhicule bloqué."><Input id="t-blk" type="number" min={0} value={v.blockedTimeoutS} onChange={(e) => f.setDraft({ ...v, blockedTimeoutS: Number(e.target.value) })} disabled={!canEdit} /></Field>
        <Field label="Délai communication (s)" htmlFor="t-com" hint="Sans message, la connexion est considérée perdue."><Input id="t-com" type="number" min={0} value={v.commsTimeoutS} onChange={(e) => f.setDraft({ ...v, commsTimeoutS: Number(e.target.value) })} disabled={!canEdit} /></Field>
        <Field label="Échantillons de balayage" htmlFor="t-sw" hint="Finesse du volume balayé pour la détection de conflits."><Input id="t-sw" type="number" min={1} value={v.swept?.samples ?? 0} onChange={(e) => f.setDraft({ ...v, swept: { samples: Number(e.target.value) } })} disabled={!canEdit} /></Field>
        {canEdit && (
          <div className="md:col-span-2 flex justify-end gap-2">
            <Button variant="ghost" onClick={f.reset} disabled={!f.dirty}>Réinitialiser</Button>
            <Button variant="primary" onClick={f.doSave} loading={f.saving} disabled={!f.dirty}>Enregistrer</Button>
          </div>
        )}
      </CardBody>
    </Card>
  );
}
