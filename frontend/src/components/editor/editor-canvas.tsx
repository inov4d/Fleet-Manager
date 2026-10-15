"use client";
import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type WheelEvent as ReactWheelEvent } from "react";
import type { FleetMap, Point, RouteResult } from "@/lib/types";
import { NODE_TYPE_COLORS, ZONE_TYPE_COLORS } from "@/lib/labels";
import { hexToCss, cn } from "@/lib/utils";
import { mapBounds, snap, type Selection, type Tool } from "@/components/editor/editor-types";

export interface EditorCanvasProps {
  doc: FleetMap;
  tool: Tool;
  snapEnabled: boolean;
  selection: Selection;
  route: RouteResult | null;
  onSelect: (s: Selection) => void;
  onAddNode: (p: Point) => void;
  onAddEdge: (from: string, to: string) => void;
  onAddZone: (polygon: Point[]) => void;
  onMoveNode: (id: string, p: Point, final: boolean) => void;
  onDelete: (s: Selection) => void;
  fitToken: number;
}

interface View {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** SVG map editor. SVG y = -world y (the group is flipped), labels are flipped back. */
export function EditorCanvas(props: EditorCanvasProps) {
  const { doc, tool, snapEnabled, selection, route, onSelect, onAddNode, onAddEdge, onAddZone, onMoveNode, onDelete, fitToken } = props;
  const svgRef = useRef<SVGSVGElement>(null);
  const [view, setView] = useState<View>({ x: -5, y: -35, w: 60, h: 40 });
  const [size, setSize] = useState({ w: 800, h: 600 });
  const [edgeFrom, setEdgeFrom] = useState<string | null>(null);
  const [zonePts, setZonePts] = useState<Point[]>([]);
  const [cursor, setCursor] = useState<Point | null>(null);
  const drag = useRef<{ kind: "pan" | "node"; id?: string; start: Point; viewStart: View; moved: boolean } | null>(null);

  const pxPerM = size.w / view.w;
  const sw = (px: number) => px / pxPerM;

  // Size tracking
  useEffect(() => {
    const el = svgRef.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => {
      const r = entries[0]?.contentRect;
      if (r && r.width > 0) setSize({ w: r.width, h: r.height });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Keep aspect ratio of the view synced with the element size.
  useEffect(() => {
    setView((v) => ({ ...v, h: (v.w * size.h) / size.w }));
  }, [size]);

  const fit = useCallback(() => {
    const b = mapBounds(doc);
    const margin = 3;
    const bw = b.maxX - b.minX + margin * 2;
    const bh = b.maxY - b.minY + margin * 2;
    const aspect = size.w / size.h;
    let w = bw;
    let h = bw / aspect;
    if (h < bh) {
      h = bh;
      w = bh * aspect;
    }
    const cx = (b.minX + b.maxX) / 2;
    const cy = -(b.minY + b.maxY) / 2;
    setView({ x: cx - w / 2, y: cy - h / 2, w, h });
  }, [doc, size]);

  useEffect(() => {
    fit();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fitToken, doc.id, size.w > 0]);

  const toWorld = useCallback(
    (clientX: number, clientY: number): Point => {
      const svg = svgRef.current;
      if (!svg) return { x: 0, y: 0 };
      const rect = svg.getBoundingClientRect();
      const sx = view.x + ((clientX - rect.left) / rect.width) * view.w;
      const sy = view.y + ((clientY - rect.top) / rect.height) * view.h;
      return { x: sx, y: -sy };
    },
    [view],
  );

  const onWheel = (e: ReactWheelEvent<SVGSVGElement>) => {
    const svg = svgRef.current;
    if (!svg) return;
    const rect = svg.getBoundingClientRect();
    const fx = (e.clientX - rect.left) / rect.width;
    const fy = (e.clientY - rect.top) / rect.height;
    const factor = Math.exp(e.deltaY * 0.0012);
    setView((v) => {
      const w = Math.min(2000, Math.max(2, v.w * factor));
      const h = (w * size.h) / size.w;
      return { x: v.x + fx * (v.w - w), y: v.y + fy * (v.h - h), w, h };
    });
  };

  const onPointerDown = (e: ReactPointerEvent<SVGSVGElement>) => {
    if (e.button !== 0) return;
    const target = e.target as SVGElement;
    const nodeId = target.getAttribute("data-node");
    const edgeId = target.getAttribute("data-edge");
    const zoneId = target.getAttribute("data-zone");
    const p = toWorld(e.clientX, e.clientY);
    (e.currentTarget as SVGSVGElement).setPointerCapture(e.pointerId);

    if (tool === "select" && nodeId) {
      onSelect({ kind: "node", id: nodeId });
      drag.current = { kind: "node", id: nodeId, start: p, viewStart: view, moved: false };
      return;
    }
    // Default: pan (clicks are resolved on pointer up when not moved).
    drag.current = { kind: "pan", start: { x: e.clientX, y: e.clientY }, viewStart: view, moved: false };
    void edgeId;
    void zoneId;
  };

  const onPointerMove = (e: ReactPointerEvent<SVGSVGElement>) => {
    const p = toWorld(e.clientX, e.clientY);
    setCursor({ x: snap(p.x, snapEnabled), y: snap(p.y, snapEnabled) });
    const d = drag.current;
    if (!d) return;
    if (d.kind === "node" && d.id) {
      d.moved = true;
      onMoveNode(d.id, { x: snap(p.x, snapEnabled), y: snap(p.y, snapEnabled) }, false);
    } else if (d.kind === "pan") {
      const dx = e.clientX - d.start.x;
      const dy = e.clientY - d.start.y;
      if (Math.hypot(dx, dy) > 3) d.moved = true;
      if (d.moved) {
        setView({ ...d.viewStart, x: d.viewStart.x - (dx / size.w) * d.viewStart.w, y: d.viewStart.y - (dy / size.h) * d.viewStart.h });
      }
    }
  };

  const onPointerUp = (e: ReactPointerEvent<SVGSVGElement>) => {
    const d = drag.current;
    drag.current = null;
    if (!d) return;
    const p = toWorld(e.clientX, e.clientY);
    const sp = { x: snap(p.x, snapEnabled), y: snap(p.y, snapEnabled) };
    if (d.kind === "node" && d.id) {
      if (d.moved) onMoveNode(d.id, sp, true);
      return;
    }
    if (d.moved) return; // it was a pan
    // Click resolution
    const target = e.target as SVGElement;
    const nodeId = target.getAttribute("data-node");
    const edgeId = target.getAttribute("data-edge");
    const zoneId = target.getAttribute("data-zone");
    const hit: Selection = nodeId ? { kind: "node", id: nodeId } : edgeId ? { kind: "edge", id: edgeId } : zoneId ? { kind: "zone", id: zoneId } : null;

    switch (tool) {
      case "select":
        onSelect(hit);
        break;
      case "addNode":
        if (!nodeId) onAddNode(sp);
        break;
      case "addEdge":
        if (nodeId) {
          if (edgeFrom && edgeFrom !== nodeId) {
            onAddEdge(edgeFrom, nodeId);
            setEdgeFrom(null);
          } else {
            setEdgeFrom(nodeId);
          }
        } else {
          setEdgeFrom(null);
        }
        break;
      case "drawZone":
        setZonePts((pts) => [...pts, sp]);
        break;
      case "delete":
        if (hit) onDelete(hit);
        break;
    }
  };

  const onDoubleClick = () => {
    if (tool === "drawZone" && zonePts.length >= 3) {
      onAddZone(zonePts);
      setZonePts([]);
    }
  };

  useEffect(() => {
    setEdgeFrom(null);
    setZonePts([]);
  }, [tool]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setZonePts([]);
        setEdgeFrom(null);
        onSelect(null);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onSelect]);

  const nodeById = new Map(doc.nodes.map((n) => [n.id, n]));
  const r = sw(6);
  const fontSize = sw(11);
  const gridStep = pxPerM < 6 ? 10 : pxPerM < 25 ? 5 : pxPerM < 80 ? 1 : 0.5;
  const gx0 = Math.floor(view.x / gridStep) * gridStep;
  const gy0 = Math.floor(view.y / gridStep) * gridStep;
  const gridLines: number[] = [];
  for (let x = gx0; x <= view.x + view.w; x += gridStep) gridLines.push(x);
  const gridLinesY: number[] = [];
  for (let y = gy0; y <= view.y + view.h; y += gridStep) gridLinesY.push(y);

  const routeEdges = new Set(route?.edgeIds ?? []);
  const routeNodes = new Set(route?.nodeIds ?? []);
  const cursorClass = tool === "select" ? "cursor-default" : tool === "delete" ? "cursor-not-allowed" : "cursor-crosshair";

  return (
    <svg
      ref={svgRef}
      className={cn("w-full h-full bg-bg select-none touch-none", cursorClass)}
      viewBox={`${view.x} ${view.y} ${view.w} ${view.h}`}
      onWheel={onWheel}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onDoubleClick={onDoubleClick}
      role="application"
      aria-label="Éditeur de carte"
    >
      <defs>
        <pattern id="fm-hatch" patternUnits="userSpaceOnUse" width={sw(8)} height={sw(8)} patternTransform="rotate(45)">
          <line x1="0" y1="0" x2="0" y2={sw(8)} stroke="currentColor" strokeWidth={sw(1)} />
        </pattern>
        <marker id="fm-arrow" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z" fill="var(--fg-muted)" />
        </marker>
        <marker id="fm-arrow-route" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse">
          <path d="M 0 0 L 10 5 L 0 10 z" fill="var(--accent)" />
        </marker>
      </defs>
      {/* grid (in SVG coords) */}
      <g stroke="var(--chart-grid)" strokeWidth={sw(1)} opacity={0.7}>
        {gridLines.map((x) => (
          <line key={`gx${x}`} x1={x} y1={view.y} x2={x} y2={view.y + view.h} />
        ))}
        {gridLinesY.map((y) => (
          <line key={`gy${y}`} x1={view.x} y1={y} x2={view.x + view.w} y2={y} />
        ))}
      </g>
      {/* map extent */}
      <rect x={doc.originX ?? 0} y={-((doc.originY ?? 0) + (doc.heightM || 0))} width={doc.widthM || 0} height={doc.heightM || 0} fill="none" stroke="var(--border-strong)" strokeWidth={sw(1)} strokeDasharray={`${sw(6)} ${sw(4)}`} />
      {/* origin */}
      <g stroke="var(--fg-subtle)" strokeWidth={sw(1.5)}>
        <line x1={-1} y1={0} x2={1} y2={0} />
        <line x1={0} y1={-1} x2={0} y2={1} />
      </g>

      {/* zones */}
      {doc.zones.map((z) => {
        const color = hexToCss(ZONE_TYPE_COLORS[z.type] ?? 0x94a3b8);
        const pts = z.polygon.map((p) => `${p.x},${-p.y}`).join(" ");
        const sel = selection?.kind === "zone" && selection.id === z.id;
        const cx = z.polygon.reduce((a, p) => a + p.x, 0) / Math.max(1, z.polygon.length);
        const cy = z.polygon.reduce((a, p) => a + p.y, 0) / Math.max(1, z.polygon.length);
        return (
          <g key={z.id}>
            <polygon data-zone={z.id} points={pts} fill={color} fillOpacity={z.enabled ? 0.15 : 0.06} stroke={sel ? "var(--fg)" : color} strokeWidth={sw(sel ? 2.5 : 1.5)} strokeDasharray={z.enabled ? undefined : `${sw(5)} ${sw(4)}`} className="cursor-pointer" />
            {!z.enabled && <polygon points={pts} fill="url(#fm-hatch)" style={{ color }} opacity={0.5} pointerEvents="none" />}
            <text x={cx} y={-cy} transform={`translate(${cx},${-cy}) scale(1,-1) translate(${-cx},${cy})`} fontSize={fontSize} fill={color} textAnchor="middle" pointerEvents="none">
              {z.name}
            </text>
          </g>
        );
      })}

      {/* edges */}
      {doc.edges.map((e) => {
        const a = nodeById.get(e.from);
        const b = nodeById.get(e.to);
        if (!a || !b) return null;
        const sel = selection?.kind === "edge" && selection.id === e.id;
        const onRoute = routeEdges.has(e.id);
        const color = onRoute ? "var(--accent)" : sel ? "var(--fg)" : e.enabled ? "var(--fg-muted)" : "var(--danger)";
        const dx = b.x - a.x;
        const dy = b.y - a.y;
        const len = Math.hypot(dx, dy) || 1;
        const off = r * 1.2;
        const x1 = a.x + (dx / len) * off;
        const y1 = a.y + (dy / len) * off;
        const x2 = b.x - (dx / len) * off;
        const y2 = b.y - (dy / len) * off;
        return (
          <g key={e.id}>
            <line data-edge={e.id} x1={x1} y1={-y1} x2={x2} y2={-y2} stroke="transparent" strokeWidth={sw(12)} className="cursor-pointer" />
            <line
              x1={x1}
              y1={-y1}
              x2={x2}
              y2={-y2}
              stroke={color}
              strokeWidth={sw(onRoute ? 3 : sel ? 2.5 : 1.5)}
              strokeDasharray={e.enabled ? undefined : `${sw(6)} ${sw(4)}`}
              markerEnd={onRoute ? "url(#fm-arrow-route)" : "url(#fm-arrow)"}
              markerStart={e.bidirectional ? (onRoute ? "url(#fm-arrow-route)" : "url(#fm-arrow)") : undefined}
              pointerEvents="none"
            />
          </g>
        );
      })}

      {/* zone draft */}
      {zonePts.length > 0 && (
        <g pointerEvents="none">
          <polyline points={[...zonePts, cursor ?? zonePts[zonePts.length - 1]].map((p) => `${p.x},${-p.y}`).join(" ")} fill="var(--accent)" fillOpacity={0.1} stroke="var(--accent)" strokeWidth={sw(1.5)} strokeDasharray={`${sw(4)} ${sw(3)}`} />
          {zonePts.map((p, i) => (
            <circle key={i} cx={p.x} cy={-p.y} r={r * 0.6} fill="var(--accent)" />
          ))}
        </g>
      )}

      {/* edge draft */}
      {edgeFrom && cursor && nodeById.get(edgeFrom) && (
        <line x1={nodeById.get(edgeFrom)!.x} y1={-nodeById.get(edgeFrom)!.y} x2={cursor.x} y2={-cursor.y} stroke="var(--accent)" strokeWidth={sw(1.5)} strokeDasharray={`${sw(4)} ${sw(3)}`} pointerEvents="none" />
      )}

      {/* nodes */}
      {doc.nodes.map((n) => {
        const sel = selection?.kind === "node" && selection.id === n.id;
        const color = hexToCss(NODE_TYPE_COLORS[n.type] ?? 0x94a3b8);
        const onRoute = routeNodes.has(n.id);
        const isFrom = edgeFrom === n.id;
        return (
          <g key={n.id}>
            {(sel || onRoute || isFrom) && <circle cx={n.x} cy={-n.y} r={r * 1.8} fill="none" stroke={onRoute ? "var(--accent)" : "var(--fg)"} strokeWidth={sw(1.5)} pointerEvents="none" />}
            {n.type === "waypoint" ? (
              <circle data-node={n.id} cx={n.x} cy={-n.y} r={r} fill={color} stroke="var(--bg)" strokeWidth={sw(1)} className="cursor-pointer" />
            ) : (
              <rect data-node={n.id} x={n.x - r} y={-n.y - r} width={2 * r} height={2 * r} fill={color} stroke="var(--bg)" strokeWidth={sw(1)} className="cursor-pointer" />
            )}
            {n.theta !== null && n.theta !== undefined && (
              <line x1={n.x} y1={-n.y} x2={n.x + Math.cos(n.theta) * r * 2.5} y2={-(n.y + Math.sin(n.theta) * r * 2.5)} stroke="var(--fg)" strokeWidth={sw(1.5)} pointerEvents="none" />
            )}
            <text x={n.x} y={-n.y} transform={`translate(${n.x},${-n.y}) scale(1,-1) translate(${-n.x},${n.y})`} dy={-r * 1.6} fontSize={fontSize} fill="var(--fg-muted)" textAnchor="middle" pointerEvents="none">
              {n.name}
            </text>
          </g>
        );
      })}

      {/* cursor coordinates */}
      {cursor && (
        <text x={view.x + sw(8)} y={view.y + view.h - sw(8)} fontSize={fontSize} fill="var(--fg-subtle)" pointerEvents="none">
          x {cursor.x.toFixed(2)} · y {cursor.y.toFixed(2)} m
        </text>
      )}
    </svg>
  );
}
