"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { Hexagon, Maximize2, MousePointer2, Redo2, Route, Save, Spline, Trash2, Undo2, Waypoints } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { can } from "@/lib/auth";
import type { FleetMap, MapEdge, MapNode, MapZone, Point, RouteResult } from "@/lib/types";
import { useApi } from "@/hooks/use-api";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import { toast } from "@/stores/ui";
import { Button, IconButton } from "@/components/ui/button";
import { Checkbox, Select } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/card";
import { EditorCanvas } from "@/components/editor/editor-canvas";
import { PropertiesPanel } from "@/components/editor/properties-panel";
import { MapsList } from "@/components/editor/maps-list";
import { newEdge, newNode, newZone, removeFromMap, type Selection, type Tool } from "@/components/editor/editor-types";
import { cn } from "@/lib/utils";
import { fmtNum } from "@/lib/format";

const TOOLS: Array<{ key: Tool; label: string; icon: React.ComponentType<{ className?: string }>; shortcut: string }> = [
  { key: "select", label: "Sélection / déplacement", icon: MousePointer2, shortcut: "V" },
  { key: "addNode", label: "Ajouter un nœud", icon: Waypoints, shortcut: "N" },
  { key: "addEdge", label: "Ajouter une arête (2 nœuds)", icon: Spline, shortcut: "E" },
  { key: "drawZone", label: "Dessiner une zone (double-clic pour terminer)", icon: Hexagon, shortcut: "Z" },
  { key: "delete", label: "Supprimer", icon: Trash2, shortcut: "D" },
];

const HISTORY_CAP = 100;

interface EditorHistory {
  doc: FleetMap | null;
  past: FleetMap[];
  future: FleetMap[];
}

export default function MapEditorPage() {
  const siteId = useSiteStore((s) => s.siteId);
  const user = useAuthStore((s) => s.user);
  const canManage = can(user, "maps.manage");
  const list = useApi(() => api.maps.list(siteId), [siteId]);
  const [hist, setHist] = useState<EditorHistory>({ doc: null, past: [], future: [] });
  const { doc, past, future } = hist;
  const [savedDoc, setSavedDoc] = useState<FleetMap | null>(null);
  const [tool, setTool] = useState<Tool>("select");
  const [snapEnabled, setSnapEnabled] = useState(true);
  const [selection, setSelection] = useState<Selection>(null);
  const [saving, setSaving] = useState(false);
  const [fitToken, setFitToken] = useState(0);
  const [route, setRoute] = useState<RouteResult | null>(null);
  const [routeFrom, setRouteFrom] = useState("");
  const [routeTo, setRouteTo] = useState("");
  const [routing, setRouting] = useState(false);
  const dragBase = useRef<FleetMap | null>(null);

  const dirty = doc !== null && savedDoc !== null && doc !== savedDoc;

  const open = useCallback(async (id: string) => {
    try {
      const m = await api.maps.get(id);
      m.nodes ??= [];
      m.edges ??= [];
      m.zones ??= [];
      setHist({ doc: m, past: [], future: [] });
      setSavedDoc(m);
      setSelection(null);
      setRoute(null);
      setFitToken((t) => t + 1);
    } catch (e) {
      toast.error("Impossible d’ouvrir la carte", errorMessage(e));
    }
  }, []);

  // Open the active map by default.
  useEffect(() => {
    if (!doc && list.data && list.data.length > 0) {
      const active = list.data.find((m) => m.active) ?? list.data[0];
      void open(active.id);
    }
    if (list.data && list.data.length === 0) setHist({ doc: null, past: [], future: [] });
  }, [list.data, doc, open]);

  useEffect(() => {
    setHist({ doc: null, past: [], future: [] });
    setSavedDoc(null);
  }, [siteId]);

  /** Apply a mutation and push the previous state onto the undo stack. */
  const commit = useCallback((updater: (d: FleetMap) => FleetMap) => {
    setHist((h) => {
      if (!h.doc) return h;
      const next = updater(h.doc);
      if (next === h.doc) return h;
      return { doc: next, past: [...h.past.slice(-HISTORY_CAP + 1), h.doc], future: [] };
    });
  }, []);

  /** Update the document without a history entry (live drag). */
  const patchLive = useCallback((updater: (d: FleetMap) => FleetMap) => {
    setHist((h) => (h.doc ? { ...h, doc: updater(h.doc) } : h));
  }, []);

  const undo = useCallback(() => {
    setHist((h) => {
      if (h.past.length === 0 || !h.doc) return h;
      const prev = h.past[h.past.length - 1];
      return { doc: prev, past: h.past.slice(0, -1), future: [...h.future, h.doc] };
    });
  }, []);

  const redo = useCallback(() => {
    setHist((h) => {
      if (h.future.length === 0 || !h.doc) return h;
      const next = h.future[h.future.length - 1];
      return { doc: next, past: [...h.past, h.doc], future: h.future.slice(0, -1) };
    });
  }, []);

  // Keyboard shortcuts.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "z") {
        e.preventDefault();
        if (e.shiftKey) redo();
        else undo();
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "y") {
        e.preventDefault();
        redo();
        return;
      }
      if (!canManage) return;
      const k = e.key.toLowerCase();
      if (k === "v") setTool("select");
      else if (k === "n") setTool("addNode");
      else if (k === "e") setTool("addEdge");
      else if (k === "z") setTool("drawZone");
      else if (k === "d") setTool("delete");
      else if ((e.key === "Delete" || e.key === "Backspace") && selection) {
        commit((d) => removeFromMap(d, selection));
        setSelection(null);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [undo, redo, selection, commit, canManage]);

  const save = async () => {
    if (!doc) return;
    setSaving(true);
    try {
      const saved = await api.maps.update(doc.id, doc);
      const merged = { ...doc, ...saved, nodes: saved.nodes ?? doc.nodes, edges: saved.edges ?? doc.edges, zones: saved.zones ?? doc.zones };
      setHist((h) => ({ ...h, doc: merged }));
      setSavedDoc(merged);
      toast.success("Carte enregistrée", `${merged.name} · v${merged.version}`);
      void list.refetch();
    } catch (e) {
      toast.error("Enregistrement refusé", errorMessage(e));
    } finally {
      setSaving(false);
    }
  };

  const testRoute = async () => {
    if (!doc || !routeFrom || !routeTo) return;
    setRouting(true);
    try {
      const r = await api.maps.route(doc.id, { from: routeFrom, to: routeTo });
      setRoute(r);
      toast.info("Itinéraire calculé", `${r.nodeIds.length} nœuds · ${fmtNum(r.lengthM, 1)} m`);
    } catch (e) {
      setRoute(null);
      toast.error("Aucun itinéraire", errorMessage(e));
    } finally {
      setRouting(false);
    }
  };

  const nodeOptions = (doc?.nodes ?? []).map((n) => ({ value: n.id, label: n.name || n.id }));

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)] gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-lg font-semibold">Éditeur de carte</h1>
        {doc && (
          <span className="text-xs text-fg-muted flex items-center gap-1">
            {doc.name} · v{doc.version} {doc.active && <Badge tone="success">active</Badge>} {dirty && <Badge tone="warning">modifications non enregistrées</Badge>}
          </span>
        )}
        <div className="flex-1" />
        {canManage && doc && (
          <>
            <div className="flex items-center gap-0.5 rounded-md border border-border bg-surface p-0.5" role="toolbar" aria-label="Outils">
              {TOOLS.map((t) => (
                <IconButton
                  key={t.key}
                  label={`${t.label} (${t.shortcut})`}
                  size="sm"
                  variant={tool === t.key ? "primary" : "ghost"}
                  aria-pressed={tool === t.key}
                  onClick={() => setTool(t.key)}
                >
                  <t.icon className="size-4" />
                </IconButton>
              ))}
            </div>
            <Checkbox label="Aimant 0,5 m" checked={snapEnabled} onChange={(e) => setSnapEnabled(e.target.checked)} />
            <IconButton label="Annuler (Ctrl+Z)" size="sm" onClick={undo} disabled={past.length === 0}><Undo2 className="size-4" /></IconButton>
            <IconButton label="Rétablir (Ctrl+Y)" size="sm" onClick={redo} disabled={future.length === 0}><Redo2 className="size-4" /></IconButton>
          </>
        )}
        <IconButton label="Ajuster à la carte" size="sm" onClick={() => setFitToken((t) => t + 1)}><Maximize2 className="size-4" /></IconButton>
        {canManage && doc && (
          <Button size="sm" variant="primary" onClick={save} loading={saving} disabled={!dirty}>
            <Save className="size-3.5" /> Enregistrer
          </Button>
        )}
      </div>

      <div className="flex-1 min-h-0 grid grid-cols-[14rem_1fr_18rem] gap-2">
        <aside className="rounded-lg border border-border bg-surface overflow-hidden min-h-0">
          <MapsList maps={list.data ?? []} siteId={siteId} currentId={doc?.id ?? null} canManage={canManage} onOpen={open} onChanged={() => void list.refetch()} />
        </aside>
        <div className="rounded-lg border border-border overflow-hidden relative min-h-0">
          {doc ? (
            <EditorCanvas
              doc={doc}
              tool={canManage ? tool : "select"}
              snapEnabled={snapEnabled}
              selection={selection}
              route={route}
              fitToken={fitToken}
              onSelect={setSelection}
              onAddNode={(p: Point) => {
                commit((d) => ({ ...d, nodes: [...d.nodes, newNode(p, d.nodes.length + 1)] }));
              }}
              onAddEdge={(from, to) => {
                commit((d) => (d.edges.some((e) => e.from === from && e.to === to) ? d : { ...d, edges: [...d.edges, newEdge(from, to)] }));
              }}
              onAddZone={(polygon) => {
                commit((d) => ({ ...d, zones: [...d.zones, newZone(polygon, d.zones.length + 1)] }));
              }}
              onMoveNode={(id, p, final) => {
                const move = (d: FleetMap) => ({ ...d, nodes: d.nodes.map((n) => (n.id === id ? { ...n, x: p.x, y: p.y } : n)) });
                if (!final) {
                  // live drag: no history entry, remember the base state once
                  if (!dragBase.current) dragBase.current = doc;
                  patchLive(move);
                } else {
                  const base = dragBase.current;
                  dragBase.current = null;
                  setHist((h) => {
                    if (!h.doc) return h;
                    const next = move(h.doc);
                    return base ? { doc: next, past: [...h.past.slice(-HISTORY_CAP + 1), base], future: [] } : { ...h, doc: next };
                  });
                }
              }}
              onDelete={(s) => {
                commit((d) => removeFromMap(d, s));
                setSelection(null);
              }}
            />
          ) : (
            <EmptyState className="h-full flex items-center justify-center">{list.loading ? "Chargement…" : "Sélectionnez ou créez une carte."}</EmptyState>
          )}
          {doc && (
            <div className="absolute top-2 left-2 flex items-center gap-1 rounded-md border border-border bg-surface/90 backdrop-blur px-2 py-1">
              <Route className="size-3.5 text-fg-muted" aria-hidden />
              <Select aria-label="Nœud de départ" value={routeFrom} onChange={(e) => setRouteFrom(e.target.value)} placeholder="Départ" options={nodeOptions} className="h-7 w-36 text-xs" />
              <Select aria-label="Nœud d’arrivée" value={routeTo} onChange={(e) => setRouteTo(e.target.value)} placeholder="Arrivée" options={nodeOptions} className="h-7 w-36 text-xs" />
              <Button size="sm" onClick={testRoute} loading={routing} disabled={!routeFrom || !routeTo || dirty} title={dirty ? "Enregistrez la carte avant de tester" : undefined}>
                Tester un itinéraire
              </Button>
              {route && (
                <span className="text-[11px] text-fg-muted">
                  {route.nodeIds.length} nœuds · {fmtNum(route.lengthM, 1)} m
                  <button type="button" className="ml-1 underline" onClick={() => setRoute(null)}>effacer</button>
                </span>
              )}
            </div>
          )}
        </div>
        <aside className={cn("rounded-lg border border-border bg-surface p-3 overflow-y-auto min-h-0", !canManage && "opacity-80")}>
          {doc ? (
            <PropertiesPanel
              doc={doc}
              selection={selection}
              onChangeMap={(patch) => commit((d) => ({ ...d, ...patch }))}
              onChangeNode={(id, patch: Partial<MapNode>) => commit((d) => ({ ...d, nodes: d.nodes.map((n) => (n.id === id ? { ...n, ...patch } : n)) }))}
              onChangeEdge={(id, patch: Partial<MapEdge>) => commit((d) => ({ ...d, edges: d.edges.map((e) => (e.id === id ? { ...e, ...patch } : e)) }))}
              onChangeZone={(id, patch: Partial<MapZone>) => commit((d) => ({ ...d, zones: d.zones.map((z) => (z.id === id ? { ...z, ...patch } : z)) }))}
              onDelete={(s) => {
                commit((d) => removeFromMap(d, s));
                setSelection(null);
              }}
            />
          ) : (
            <p className="text-[11px] text-fg-subtle">Aucune carte ouverte.</p>
          )}
          {!canManage && <p className="mt-3 text-[11px] text-warning">Lecture seule : votre rôle ne permet pas de modifier les cartes.</p>}
        </aside>
      </div>
    </div>
  );
}
