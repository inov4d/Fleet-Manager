"use client";
import type { EdgeMotion, FleetMap, MapEdge, MapNode, MapZone, NodeType, ZoneType } from "@/lib/types";
import { EDGE_MOTION_LABELS, NODE_TYPE_LABELS, ZONE_TYPE_LABELS } from "@/lib/labels";
import type { Selection } from "@/components/editor/editor-types";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { Button } from "@/components/ui/button";

export function PropertiesPanel({
  doc,
  selection,
  onChangeMap,
  onChangeNode,
  onChangeEdge,
  onChangeZone,
  onDelete,
}: {
  doc: FleetMap;
  selection: Selection;
  onChangeMap: (patch: Partial<FleetMap>) => void;
  onChangeNode: (id: string, patch: Partial<MapNode>) => void;
  onChangeEdge: (id: string, patch: Partial<MapEdge>) => void;
  onChangeZone: (id: string, patch: Partial<MapZone>) => void;
  onDelete: (s: Selection) => void;
}) {
  const nodeName = (id: string) => doc.nodes.find((n) => n.id === id)?.name ?? id;
  const num = (s: string) => (s === "" ? 0 : Number(s));
  const numOrNull = (s: string) => (s === "" ? null : Number(s));

  if (selection?.kind === "node") {
    const n = doc.nodes.find((x) => x.id === selection.id);
    if (!n) return null;
    return (
      <div className="flex flex-col gap-2">
        <h3 className="text-xs font-semibold">Nœud <span className="text-fg-subtle font-normal">{n.id}</span></h3>
        <Field label="Nom" htmlFor="p-name"><Input id="p-name" value={n.name} onChange={(e) => onChangeNode(n.id, { name: e.target.value })} /></Field>
        <Field label="Type" htmlFor="p-type">
          <Select id="p-type" value={n.type} onChange={(e) => onChangeNode(n.id, { type: e.target.value as NodeType })} options={(Object.keys(NODE_TYPE_LABELS) as NodeType[]).map((k) => ({ value: k, label: NODE_TYPE_LABELS[k] }))} />
        </Field>
        <div className="grid grid-cols-2 gap-2">
          <Field label="x (m)" htmlFor="p-x"><Input id="p-x" type="number" step={0.1} value={n.x} onChange={(e) => onChangeNode(n.id, { x: num(e.target.value) })} /></Field>
          <Field label="y (m)" htmlFor="p-y"><Input id="p-y" type="number" step={0.1} value={n.y} onChange={(e) => onChangeNode(n.id, { y: num(e.target.value) })} /></Field>
        </div>
        <Field label="Orientation θ (°)" htmlFor="p-theta" hint="Vide = libre. Stockée en radians.">
          <Input id="p-theta" type="number" step={1} value={n.theta === null ? "" : Math.round((n.theta * 180) / Math.PI)} onChange={(e) => onChangeNode(n.id, { theta: e.target.value === "" ? null : (Number(e.target.value) * Math.PI) / 180 })} />
        </Field>
        <Field label="Puissance chargeur (kW)" htmlFor="p-kw">
          <Input id="p-kw" type="number" step={1} min={0} value={n.chargerPowerKw ?? ""} onChange={(e) => onChangeNode(n.id, { chargerPowerKw: numOrNull(e.target.value) })} disabled={n.type !== "charging"} />
        </Field>
        <Field label="Capacité" htmlFor="p-cap"><Input id="p-cap" type="number" min={1} step={1} value={n.capacity} onChange={(e) => onChangeNode(n.id, { capacity: Math.max(1, num(e.target.value)) })} /></Field>
        <p className="text-[11px] text-fg-subtle">{doc.edges.filter((e) => e.from === n.id || e.to === n.id).length} arête(s) connectée(s)</p>
        <Button size="sm" variant="danger" onClick={() => onDelete(selection)}>Supprimer le nœud</Button>
      </div>
    );
  }

  if (selection?.kind === "edge") {
    const e = doc.edges.find((x) => x.id === selection.id);
    if (!e) return null;
    return (
      <div className="flex flex-col gap-2">
        <h3 className="text-xs font-semibold">Arête <span className="text-fg-subtle font-normal">{e.id}</span></h3>
        <p className="text-xs">{nodeName(e.from)} <span className="text-fg-subtle">→</span> {nodeName(e.to)}</p>
        <Button size="xs" onClick={() => onChangeEdge(e.id, { from: e.to, to: e.from })}>Inverser le sens</Button>
        <Checkbox label="Bidirectionnelle" checked={e.bidirectional} onChange={(ev) => onChangeEdge(e.id, { bidirectional: ev.target.checked })} />
        <Checkbox label="Activée" checked={e.enabled} onChange={(ev) => onChangeEdge(e.id, { enabled: ev.target.checked })} />
        <Field label="Vitesse max (m/s)" htmlFor="p-speed"><Input id="p-speed" type="number" step={0.1} min={0} value={e.maxSpeedMs} onChange={(ev) => onChangeEdge(e.id, { maxSpeedMs: num(ev.target.value) })} /></Field>
        <Field label="Mouvement" htmlFor="p-motion">
          <Select id="p-motion" value={e.motion} onChange={(ev) => onChangeEdge(e.id, { motion: ev.target.value as EdgeMotion })} options={(Object.keys(EDGE_MOTION_LABELS) as EdgeMotion[]).map((k) => ({ value: k, label: EDGE_MOTION_LABELS[k] }))} />
        </Field>
        <Field label="Facteur de coût" htmlFor="p-cost"><Input id="p-cost" type="number" step={0.1} min={0} value={e.costFactor} onChange={(ev) => onChangeEdge(e.id, { costFactor: num(ev.target.value) })} /></Field>
        <Field label="Largeur (m)" htmlFor="p-width"><Input id="p-width" type="number" step={0.1} min={0} value={e.widthM} onChange={(ev) => onChangeEdge(e.id, { widthM: num(ev.target.value) })} /></Field>
        <Button size="sm" variant="danger" onClick={() => onDelete(selection)}>Supprimer l’arête</Button>
      </div>
    );
  }

  if (selection?.kind === "zone") {
    const z = doc.zones.find((x) => x.id === selection.id);
    if (!z) return null;
    const setPoint = (i: number, k: "x" | "y", v: number) => onChangeZone(z.id, { polygon: z.polygon.map((p, j) => (j === i ? { ...p, [k]: v } : p)) });
    return (
      <div className="flex flex-col gap-2">
        <h3 className="text-xs font-semibold">Zone <span className="text-fg-subtle font-normal">{z.id}</span></h3>
        <Field label="Nom" htmlFor="p-zname"><Input id="p-zname" value={z.name} onChange={(e) => onChangeZone(z.id, { name: e.target.value })} /></Field>
        <Field label="Type" htmlFor="p-ztype">
          <Select id="p-ztype" value={z.type} onChange={(e) => onChangeZone(z.id, { type: e.target.value as ZoneType })} options={(Object.keys(ZONE_TYPE_LABELS) as ZoneType[]).map((k) => ({ value: k, label: ZONE_TYPE_LABELS[k] }))} />
        </Field>
        <Field label="Capacité" htmlFor="p-zcap"><Input id="p-zcap" type="number" min={1} step={1} value={z.capacity} onChange={(e) => onChangeZone(z.id, { capacity: Math.max(1, num(e.target.value)) })} /></Field>
        <Field label="Vitesse max (m/s)" htmlFor="p-zspeed" hint="Vide = pas de limite."><Input id="p-zspeed" type="number" min={0} step={0.1} value={z.maxSpeedMs ?? ""} onChange={(e) => onChangeZone(z.id, { maxSpeedMs: numOrNull(e.target.value) })} /></Field>
        <Checkbox label="Activée" checked={z.enabled} onChange={(e) => onChangeZone(z.id, { enabled: e.target.checked })} />
        <div>
          <div className="text-xs font-medium text-fg-muted mb-1">Sommets ({z.polygon.length})</div>
          <div className="flex flex-col gap-1 max-h-48 overflow-y-auto">
            {z.polygon.map((p, i) => (
              <div key={i} className="grid grid-cols-[1.5rem_1fr_1fr_auto] gap-1 items-center">
                <span className="text-[10px] text-fg-subtle tabular">{i + 1}</span>
                <Input aria-label={`Sommet ${i + 1} x`} type="number" step={0.1} value={p.x} onChange={(e) => setPoint(i, "x", num(e.target.value))} className="h-7" />
                <Input aria-label={`Sommet ${i + 1} y`} type="number" step={0.1} value={p.y} onChange={(e) => setPoint(i, "y", num(e.target.value))} className="h-7" />
                <Button size="xs" variant="ghost" aria-label={`Supprimer le sommet ${i + 1}`} disabled={z.polygon.length <= 3} onClick={() => onChangeZone(z.id, { polygon: z.polygon.filter((_, j) => j !== i) })}>×</Button>
              </div>
            ))}
          </div>
        </div>
        <Button size="sm" variant="danger" onClick={() => onDelete(selection)}>Supprimer la zone</Button>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-xs font-semibold">Carte</h3>
      <Field label="Nom" htmlFor="p-mname"><Input id="p-mname" value={doc.name} onChange={(e) => onChangeMap({ name: e.target.value })} /></Field>
      <div className="grid grid-cols-2 gap-2">
        <Field label="Largeur (m)" htmlFor="p-mw"><Input id="p-mw" type="number" min={0} step={1} value={doc.widthM} onChange={(e) => onChangeMap({ widthM: num(e.target.value) })} /></Field>
        <Field label="Hauteur (m)" htmlFor="p-mh"><Input id="p-mh" type="number" min={0} step={1} value={doc.heightM} onChange={(e) => onChangeMap({ heightM: num(e.target.value) })} /></Field>
        <Field label="Origine x" htmlFor="p-mox"><Input id="p-mox" type="number" step={0.5} value={doc.originX} onChange={(e) => onChangeMap({ originX: num(e.target.value) })} /></Field>
        <Field label="Origine y" htmlFor="p-moy"><Input id="p-moy" type="number" step={0.5} value={doc.originY} onChange={(e) => onChangeMap({ originY: num(e.target.value) })} /></Field>
      </div>
      <p className="text-[11px] text-fg-subtle">{doc.nodes.length} nœuds · {doc.edges.length} arêtes · {doc.zones.length} zones · version {doc.version}</p>
      <p className="text-[11px] text-fg-subtle">Sélectionnez un élément sur la carte pour modifier ses propriétés.</p>
    </div>
  );
}
