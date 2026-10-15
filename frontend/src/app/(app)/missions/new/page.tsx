"use client";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useState, type FormEvent } from "react";
import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { fromLocalInput } from "@/lib/format";
import { MISSION_TYPE_LABELS, STEP_ACTION_LABELS, ZONE_TYPE_LABELS } from "@/lib/labels";
import { TERMINAL_MISSION_STATUSES, type MissionCreate, type MissionType, type StepAction } from "@/lib/types";
import { useActiveMap } from "@/hooks/use-active-map";
import { useApi } from "@/hooks/use-api";
import { useSiteStore } from "@/stores/site";
import { useFleetStore } from "@/stores/fleet";
import { toast } from "@/stores/ui";
import { PageHeader, Card, CardHeader, CardBody, ErrorBox, Spinner } from "@/components/ui/card";
import { Button, IconButton } from "@/components/ui/button";
import { Checkbox, Field, Input, Select, Slider, Textarea } from "@/components/ui/input";

interface StepDraft {
  nodeId: string;
  action: StepAction;
  durationS: number;
}

export default function NewMissionPage() {
  return (
    <Suspense fallback={<Spinner />}>
      <NewMissionForm />
    </Suspense>
  );
}

function NewMissionForm() {
  const router = useRouter();
  const search = useSearchParams();
  const siteId = useSiteStore((s) => s.siteId);
  const map = useActiveMap();
  const liveMissions = useFleetStore((s) => s.missions);
  const models = useApi(() => api.vehicleModels.list(), []);
  const openMissions = useApi(() => api.missions.list({ siteId, limit: 200 }), [siteId]);

  const nodes = useMemo(() => [...(map?.nodes ?? [])].sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id)), [map]);
  const nodeOptions = nodes.map((n) => ({ value: n.id, label: `${n.name || n.id} (${n.type})` }));
  const zones = map?.zones ?? [];

  const [type, setType] = useState<MissionType>("transport");
  const [name, setName] = useState("");
  const [origin, setOrigin] = useState(search.get("origin") ?? "");
  const [destination, setDestination] = useState(search.get("destination") ?? "");
  const [steps, setSteps] = useState<StepDraft[]>([]);
  const [priority, setPriority] = useState(5);
  const [loadPresent, setLoadPresent] = useState(false);
  const [massKg, setMassKg] = useState(0);
  const [dims, setDims] = useState({ lengthM: 0, widthM: 0, heightM: 0 });
  const [loadDesc, setLoadDesc] = useState("");
  const [requiredModelId, setRequiredModelId] = useState("");
  const [deadline, setDeadline] = useState("");
  const [dependencies, setDependencies] = useState<string[]>([]);
  const [avoidZones, setAvoidZones] = useState<string[]>([]);
  const [maxSpeed, setMaxSpeed] = useState("");
  const [externalRef, setExternalRef] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const candidates = useMemo(() => {
    const byId = new Map<string, { id: string; name: string; status: string }>();
    for (const m of openMissions.data ?? []) if (!TERMINAL_MISSION_STATUSES.includes(m.status)) byId.set(m.id, { id: m.id, name: m.name, status: m.status });
    for (const m of liveMissions) byId.set(m.id, { id: m.id, name: m.name, status: m.status });
    return Array.from(byId.values());
  }, [openMissions.data, liveMissions]);

  const addStep = () => setSteps((s) => [...s, { nodeId: nodes[0]?.id ?? "", action: "move", durationS: 0 }]);
  const updStep = (i: number, patch: Partial<StepDraft>) => setSteps((s) => s.map((st, j) => (j === i ? { ...st, ...patch } : st)));
  const moveStep = (i: number, dir: -1 | 1) =>
    setSteps((s) => {
      const j = i + dir;
      if (j < 0 || j >= s.length) return s;
      const copy = [...s];
      [copy[i], copy[j]] = [copy[j], copy[i]];
      return copy;
    });

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    if (!origin || !destination) {
      setError("Origine et destination sont obligatoires.");
      return;
    }
    const originAction: StepAction = loadPresent ? "pick" : "move";
    const destAction: StepAction = type === "charge" ? "charge" : loadPresent ? "drop" : "move";
    const allSteps = [
      { nodeId: origin, action: originAction, durationS: 0 },
      ...steps,
      { nodeId: destination, action: destAction, durationS: 0 },
    ].map((s, i) => ({ seq: i + 1, ...s }));
    const body: MissionCreate = {
      siteId,
      name: name.trim() || `${MISSION_TYPE_LABELS[type]} ${new Date().toLocaleTimeString("fr-FR")}`,
      type: steps.length > 0 && type === "transport" ? "multiStop" : type,
      priority,
      origin,
      destination,
      steps: allSteps,
      load: { present: loadPresent, massKg: loadPresent ? massKg : 0, ...dims, description: loadDesc },
      requiredModelId,
      source: "operator",
      dependencies,
      constraints: { avoidZoneIds: avoidZones, maxSpeedMs: maxSpeed ? Number(maxSpeed) : null },
      deadline: fromLocalInput(deadline),
      ...(externalRef ? { externalRef } : {}),
    };
    setLoading(true);
    try {
      const created = await api.missions.create(body);
      toast.success("Mission créée", created.name || created.id);
      router.push(`/missions/${encodeURIComponent(created.id)}`);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const toggleIn = (arr: string[], id: string) => (arr.includes(id) ? arr.filter((x) => x !== id) : [...arr, id]);

  return (
    <form onSubmit={submit} className="max-w-5xl flex flex-col gap-3">
      <PageHeader
        title="Nouvelle mission"
        subtitle={map ? `Carte active : ${map.name}` : "Aucune carte active — impossible de sélectionner des nœuds"}
        actions={
          <Button type="submit" variant="primary" loading={loading} disabled={!map}>
            Créer la mission
          </Button>
        }
      />
      {error && <ErrorBox message={error} />}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-3">
        <Card className="lg:col-span-2">
          <CardHeader title="Trajet" />
          <CardBody className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <Field label="Type" htmlFor="m-type">
              <Select
                id="m-type"
                value={type}
                onChange={(e) => setType(e.target.value as MissionType)}
                options={(Object.keys(MISSION_TYPE_LABELS) as MissionType[]).map((k) => ({ value: k, label: MISSION_TYPE_LABELS[k] }))}
              />
            </Field>
            <Field label="Nom" htmlFor="m-name" hint="Généré automatiquement si vide.">
              <Input id="m-name" value={name} onChange={(e) => setName(e.target.value)} />
            </Field>
            <Field label="Nœud d’origine" htmlFor="m-origin" required>
              <Select id="m-origin" value={origin} onChange={(e) => setOrigin(e.target.value)} placeholder="Choisir…" options={nodeOptions} required />
            </Field>
            <Field label="Nœud de destination" htmlFor="m-dest" required>
              <Select id="m-dest" value={destination} onChange={(e) => setDestination(e.target.value)} placeholder="Choisir…" options={nodeOptions} required />
            </Field>
            <div className="md:col-span-2">
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs font-medium text-fg-muted">Étapes intermédiaires ({steps.length})</span>
                <Button size="xs" onClick={addStep} disabled={nodes.length === 0}>
                  <Plus className="size-3" /> Ajouter une étape
                </Button>
              </div>
              {steps.length === 0 ? (
                <p className="text-[11px] text-fg-subtle">Aucune étape intermédiaire : mission origine → destination.</p>
              ) : (
                <ol className="flex flex-col gap-1">
                  {steps.map((s, i) => (
                    <li key={i} className="grid grid-cols-[auto_1fr_10rem_6rem_auto] items-center gap-1.5 rounded border border-border px-2 py-1">
                      <span className="text-[11px] text-fg-muted tabular w-5">{i + 2}.</span>
                      <Select aria-label={`Nœud de l’étape ${i + 2}`} value={s.nodeId} onChange={(e) => updStep(i, { nodeId: e.target.value })} options={nodeOptions} className="h-7" />
                      <Select
                        aria-label="Action"
                        value={s.action}
                        onChange={(e) => updStep(i, { action: e.target.value as StepAction })}
                        options={(Object.keys(STEP_ACTION_LABELS) as StepAction[]).map((k) => ({ value: k, label: STEP_ACTION_LABELS[k] }))}
                        className="h-7"
                      />
                      <Input aria-label="Durée (s)" type="number" min={0} value={s.durationS} onChange={(e) => updStep(i, { durationS: Number(e.target.value) })} className="h-7" title="Durée (s)" />
                      <span className="inline-flex gap-0.5">
                        <IconButton label="Monter" size="xs" onClick={() => moveStep(i, -1)}><ArrowUp className="size-3" /></IconButton>
                        <IconButton label="Descendre" size="xs" onClick={() => moveStep(i, 1)}><ArrowDown className="size-3" /></IconButton>
                        <IconButton label="Supprimer l’étape" size="xs" onClick={() => setSteps((st) => st.filter((_, j) => j !== i))} className="text-danger"><Trash2 className="size-3" /></IconButton>
                      </span>
                    </li>
                  ))}
                </ol>
              )}
            </div>
            <Field label={`Priorité : ${priority}`} htmlFor="m-prio" className="md:col-span-2">
              <Slider id="m-prio" min={1} max={10} step={1} value={priority} onChange={(e) => setPriority(Number(e.target.value))} valueLabel={priority >= 8 ? "haute" : priority <= 3 ? "basse" : "normale"} />
            </Field>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Charge" />
          <CardBody className="flex flex-col gap-3">
            <Checkbox label="Charge présente" checked={loadPresent} onChange={(e) => setLoadPresent(e.target.checked)} />
            <Field label="Masse (kg)" htmlFor="m-mass">
              <Input id="m-mass" type="number" min={0} step={10} value={massKg} onChange={(e) => setMassKg(Number(e.target.value))} disabled={!loadPresent} />
            </Field>
            <div className="grid grid-cols-3 gap-2">
              {(["lengthM", "widthM", "heightM"] as const).map((k) => (
                <Field key={k} label={k === "lengthM" ? "Long. (m)" : k === "widthM" ? "Larg. (m)" : "Haut. (m)"} htmlFor={`m-${k}`}>
                  <Input id={`m-${k}`} type="number" min={0} step={0.1} value={dims[k]} onChange={(e) => setDims((d) => ({ ...d, [k]: Number(e.target.value) }))} disabled={!loadPresent} />
                </Field>
              ))}
            </div>
            <Field label="Description" htmlFor="m-ldesc">
              <Textarea id="m-ldesc" value={loadDesc} onChange={(e) => setLoadDesc(e.target.value)} disabled={!loadPresent} />
            </Field>
          </CardBody>
        </Card>

        <Card className="lg:col-span-3">
          <CardHeader title="Contraintes et dépendances" />
          <CardBody className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <Field label="Modèle requis" htmlFor="m-model">
              <Select
                id="m-model"
                value={requiredModelId}
                onChange={(e) => setRequiredModelId(e.target.value)}
                placeholder="Indifférent"
                options={(models.data ?? []).map((m) => ({ value: m.id, label: m.name }))}
              />
            </Field>
            <Field label="Échéance" htmlFor="m-deadline">
              <Input id="m-deadline" type="datetime-local" value={deadline} onChange={(e) => setDeadline(e.target.value)} />
            </Field>
            <Field label="Vitesse max (m/s)" htmlFor="m-speed" hint="Vide = vitesse des arêtes.">
              <Input id="m-speed" type="number" min={0} step={0.1} value={maxSpeed} onChange={(e) => setMaxSpeed(e.target.value)} />
            </Field>
            <Field label="Référence externe" htmlFor="m-ext">
              <Input id="m-ext" value={externalRef} onChange={(e) => setExternalRef(e.target.value)} />
            </Field>
            <fieldset>
              <legend className="text-xs font-medium text-fg-muted mb-1">Dépendances (missions ouvertes)</legend>
              <div className="max-h-36 overflow-y-auto rounded border border-border p-2 flex flex-col gap-1">
                {candidates.length === 0 && <span className="text-[11px] text-fg-subtle">Aucune mission ouverte.</span>}
                {candidates.map((m) => (
                  <Checkbox key={m.id} label={`${m.name || m.id} (${m.status})`} checked={dependencies.includes(m.id)} onChange={() => setDependencies((d) => toggleIn(d, m.id))} />
                ))}
              </div>
            </fieldset>
            <fieldset>
              <legend className="text-xs font-medium text-fg-muted mb-1">Zones à éviter</legend>
              <div className="max-h-36 overflow-y-auto rounded border border-border p-2 flex flex-col gap-1">
                {zones.length === 0 && <span className="text-[11px] text-fg-subtle">Aucune zone sur la carte.</span>}
                {zones.map((z) => (
                  <Checkbox key={z.id} label={`${z.name} (${ZONE_TYPE_LABELS[z.type] ?? z.type})`} checked={avoidZones.includes(z.id)} onChange={() => setAvoidZones((a) => toggleIn(a, z.id))} />
                ))}
              </div>
            </fieldset>
          </CardBody>
        </Card>
      </div>
    </form>
  );
}
