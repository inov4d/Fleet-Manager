import type { FleetMap, MapEdge, MapNode, MapZone, Point } from "@/lib/types";
import { uid } from "@/lib/utils";

export type Tool = "select" | "addNode" | "addEdge" | "drawZone" | "delete";

export type Selection = { kind: "node"; id: string } | { kind: "edge"; id: string } | { kind: "zone"; id: string } | null;

export const SNAP_M = 0.5;

export function snap(v: number, enabled: boolean): number {
  return enabled ? Math.round(v / SNAP_M) * SNAP_M : Math.round(v * 100) / 100;
}

export function newNode(p: Point, index: number): MapNode {
  return { id: uid("n"), name: `N${index}`, x: p.x, y: p.y, type: "waypoint", theta: null, chargerPowerKw: null, capacity: 1 };
}

export function newEdge(from: string, to: string): MapEdge {
  return { id: uid("e"), from, to, bidirectional: true, maxSpeedMs: 1.0, motion: "any", costFactor: 1.0, widthM: 3.0, enabled: true };
}

export function newZone(polygon: Point[], index: number): MapZone {
  return { id: uid("z"), name: `Zone ${index}`, type: "exclusive", polygon, capacity: 1, enabled: true, maxSpeedMs: null };
}

export function emptyMap(siteId: string, name: string): Partial<FleetMap> {
  return { siteId, name, widthM: 100, heightM: 60, originX: 0, originY: 0, nodes: [], edges: [], zones: [] };
}

export function removeFromMap(doc: FleetMap, sel: Selection): FleetMap {
  if (!sel) return doc;
  if (sel.kind === "node") {
    return {
      ...doc,
      nodes: doc.nodes.filter((n) => n.id !== sel.id),
      edges: doc.edges.filter((e) => e.from !== sel.id && e.to !== sel.id),
    };
  }
  if (sel.kind === "edge") return { ...doc, edges: doc.edges.filter((e) => e.id !== sel.id) };
  return { ...doc, zones: doc.zones.filter((z) => z.id !== sel.id) };
}

export function mapBounds(doc: FleetMap): { minX: number; minY: number; maxX: number; maxY: number } {
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  const add = (x: number, y: number) => {
    minX = Math.min(minX, x);
    minY = Math.min(minY, y);
    maxX = Math.max(maxX, x);
    maxY = Math.max(maxY, y);
  };
  for (const n of doc.nodes) add(n.x, n.y);
  for (const z of doc.zones) for (const p of z.polygon) add(p.x, p.y);
  if (!Number.isFinite(minX)) {
    add(doc.originX ?? 0, doc.originY ?? 0);
    add((doc.originX ?? 0) + (doc.widthM || 50), (doc.originY ?? 0) + (doc.heightM || 30));
  }
  return { minX, minY, maxX, maxY };
}
