/* PixiJS v8 renderer for the live map. World units are metres with y up; screen y is flipped (y_screen = -y). */
import { Application, Container, Graphics, Rectangle, Text, type FederatedPointerEvent } from "pixi.js";
import type { FleetMap, MapEdge, MapNode, MapZone, Mission, Point, Reservation, VehicleModel, VehicleWithState } from "@/lib/types";
import { AVAILABILITY_COLORS, NODE_TYPE_COLORS, ZONE_TYPE_COLORS } from "@/lib/labels";
import { clamp, vehicleColorHex } from "@/lib/utils";

export interface MapLayers {
  zones: boolean;
  edges: boolean;
  reservations: boolean;
  routes: boolean;
  labels: boolean;
}

export const DEFAULT_LAYERS: MapLayers = { zones: true, edges: true, reservations: true, routes: true, labels: true };

export interface SnapshotData {
  vehicles: VehicleWithState[];
  missions: Mission[];
  reservations: Reservation[];
}

export interface RendererCallbacks {
  onNodeClick?: (node: MapNode) => void;
  onVehicleClick?: (vehicleId: string) => void;
  onBackgroundClick?: () => void;
}

interface VehicleSprite {
  group: Container;
  body: Graphics;
  hud: Graphics;
  label: Text;
  key: string;
}

const UNCERTAIN_COLOR = 0xf97316;
const MIN_ZOOM = 1;
const MAX_ZOOM = 400;
const LABEL_FONT = "ui-sans-serif, system-ui, sans-serif";

export class MapRenderer {
  private app = new Application();
  private world = new Container();
  private gridLayer = new Graphics();
  private zonesLayer = new Container();
  private edgesLayer = new Graphics();
  private reservationsLayer = new Graphics();
  private routesLayer = new Graphics();
  private nodesLayer = new Container();
  private vehiclesLayer = new Container();
  private labelsLayer = new Container();

  private map: FleetMap | null = null;
  private nodeIndex = new Map<string, MapNode>();
  private edgeIndex = new Map<string, MapEdge>();
  private zoneIndex = new Map<string, MapZone>();
  private models = new Map<string, VehicleModel>();
  private vehicleSprites = new Map<string, VehicleSprite>();
  private nodeLabels: Text[] = [];
  private zoom = 20;
  private visible: MapLayers = { ...DEFAULT_LAYERS };
  private theme: "dark" | "light" = "dark";
  private snapshot: SnapshotData | null = null;
  private selectedVehicleId: string | null = null;
  private callbacks: RendererCallbacks = {};

  private ready = false;
  private disposed = false;
  private dirtyStatic = false;
  private dirtyDynamic = false;
  private fitted = false;

  private dragging = false;
  private dragMoved = false;
  private dragStart = { x: 0, y: 0 };
  private worldStart = { x: 0, y: 0 };

  private onWheel = (e: WheelEvent) => {
    e.preventDefault();
    const rect = this.app.canvas.getBoundingClientRect();
    const factor = Math.exp(-e.deltaY * 0.0012);
    this.zoomAt(e.clientX - rect.left, e.clientY - rect.top, factor);
  };

  setCallbacks(cb: RendererCallbacks) {
    this.callbacks = cb;
  }

  async init(container: HTMLElement, theme: "dark" | "light") {
    this.theme = theme;
    await this.app.init({
      background: theme === "dark" ? 0x0b0f14 : 0xf3f5f8,
      width: Math.max(1, container.clientWidth),
      height: Math.max(1, container.clientHeight),
      antialias: true,
      resolution: Math.min(window.devicePixelRatio || 1, 2),
      autoDensity: true,
      preference: "webgl",
    });
    if (this.disposed) {
      this.app.destroy(true, { children: true });
      return;
    }
    container.appendChild(this.app.canvas);
    this.app.canvas.style.display = "block";
    this.app.canvas.style.touchAction = "none";

    this.world.addChild(
      this.gridLayer,
      this.zonesLayer,
      this.edgesLayer,
      this.reservationsLayer,
      this.routesLayer,
      this.nodesLayer,
      this.vehiclesLayer,
      this.labelsLayer,
    );
    this.world.scale.set(this.zoom);
    this.app.stage.addChild(this.world);

    const stage = this.app.stage;
    stage.eventMode = "static";
    stage.hitArea = this.app.screen;
    stage.on("pointerdown", this.onPointerDown);
    stage.on("pointermove", this.onPointerMove);
    stage.on("pointerup", this.onPointerUp);
    stage.on("pointerupoutside", this.onPointerUp);
    stage.on("pointertap", () => {
      if (!this.dragMoved) this.callbacks.onBackgroundClick?.();
    });
    this.app.canvas.addEventListener("wheel", this.onWheel, { passive: false });

    this.app.ticker.add(this.tick);
    this.ready = true;
    this.dirtyStatic = true;
    this.dirtyDynamic = true;
  }

  destroy() {
    this.disposed = true;
    if (!this.ready) return;
    this.app.ticker.remove(this.tick);
    this.app.canvas.removeEventListener("wheel", this.onWheel);
    this.app.stage.removeAllListeners();
    this.vehicleSprites.clear();
    this.nodeLabels = [];
    this.app.destroy(true, { children: true, texture: true });
    this.ready = false;
  }

  resize(width: number, height: number) {
    if (!this.ready) return;
    this.app.renderer.resize(Math.max(1, width), Math.max(1, height));
    this.app.stage.hitArea = this.app.screen;
    if (!this.fitted) this.fitToMap();
  }

  setTheme(theme: "dark" | "light") {
    this.theme = theme;
    if (this.ready) this.app.renderer.background.color = theme === "dark" ? 0x0b0f14 : 0xf3f5f8;
    this.dirtyStatic = true;
    this.dirtyDynamic = true;
  }

  setVehicleModels(models: VehicleModel[]) {
    this.models = new Map(models.map((m) => [m.id, m]));
    this.dirtyDynamic = true;
  }

  setLayers(layers: MapLayers) {
    this.visible = { ...layers };
    this.zonesLayer.visible = layers.zones;
    this.edgesLayer.visible = layers.edges;
    this.reservationsLayer.visible = layers.reservations;
    this.routesLayer.visible = layers.routes;
    this.labelsLayer.visible = layers.labels;
    for (const s of this.vehicleSprites.values()) s.label.visible = layers.labels;
  }

  setSelectedVehicle(id: string | null) {
    this.selectedVehicleId = id;
    this.dirtyDynamic = true;
  }

  setMap(map: FleetMap | null) {
    const sameMap = this.map && map && this.map.id === map.id;
    this.map = map;
    this.nodeIndex = new Map(map?.nodes?.map((n) => [n.id, n]) ?? []);
    this.edgeIndex = new Map(map?.edges?.map((e) => [e.id, e]) ?? []);
    this.zoneIndex = new Map(map?.zones?.map((z) => [z.id, z]) ?? []);
    this.dirtyStatic = true;
    this.dirtyDynamic = true;
    if (!sameMap) this.fitted = false;
    if (this.ready && !this.fitted) this.fitToMap();
  }

  setSnapshot(data: SnapshotData) {
    this.snapshot = data;
    this.dirtyDynamic = true;
  }

  /** Fit the whole map (nodes + zones, or declared size) into the viewport. */
  fitToMap() {
    if (!this.ready) return;
    const b = this.bounds();
    const w = this.app.screen.width;
    const h = this.app.screen.height;
    const margin = 2;
    const bw = Math.max(1, b.maxX - b.minX + margin * 2);
    const bh = Math.max(1, b.maxY - b.minY + margin * 2);
    this.zoom = clamp(Math.min(w / bw, h / bh), MIN_ZOOM, MAX_ZOOM);
    this.world.scale.set(this.zoom);
    const cx = (b.minX + b.maxX) / 2;
    const cy = -(b.minY + b.maxY) / 2;
    this.world.position.set(w / 2 - cx * this.zoom, h / 2 - cy * this.zoom);
    this.fitted = true;
    this.onZoomChanged();
  }

  /** Centre the viewport on a world point (metres, y up) without changing zoom. */
  centerOn(x: number, y: number) {
    if (!this.ready) return;
    const w = this.app.screen.width;
    const h = this.app.screen.height;
    this.world.position.set(w / 2 - x * this.zoom, h / 2 + y * this.zoom);
  }

  private bounds() {
    const m = this.map;
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
    if (m) {
      for (const n of m.nodes ?? []) add(n.x, n.y);
      for (const z of m.zones ?? []) for (const p of z.polygon) add(p.x, p.y);
    }
    if (!Number.isFinite(minX)) {
      const ox = m?.originX ?? 0;
      const oy = m?.originY ?? 0;
      add(ox, oy);
      add(ox + (m?.widthM || 50), oy + (m?.heightM || 30));
    }
    return { minX, minY, maxX, maxY };
  }

  private zoomAt(sx: number, sy: number, factor: number) {
    const newZoom = clamp(this.zoom * factor, MIN_ZOOM, MAX_ZOOM);
    const f = newZoom / this.zoom;
    if (f === 1) return;
    const wx = this.world.position.x;
    const wy = this.world.position.y;
    this.world.position.set(sx - (sx - wx) * f, sy - (sy - wy) * f);
    this.zoom = newZoom;
    this.world.scale.set(this.zoom);
    this.onZoomChanged();
  }

  private onZoomChanged() {
    const s = 1 / this.zoom;
    for (const t of this.nodeLabels) t.scale.set(s);
    for (const v of this.vehicleSprites.values()) v.label.scale.set(s);
    this.dirtyStatic = true;
    this.dirtyDynamic = true;
  }

  private onPointerDown = (e: FederatedPointerEvent) => {
    this.dragging = true;
    this.dragMoved = false;
    this.dragStart = { x: e.global.x, y: e.global.y };
    this.worldStart = { x: this.world.position.x, y: this.world.position.y };
  };

  private onPointerMove = (e: FederatedPointerEvent) => {
    if (!this.dragging) return;
    const dx = e.global.x - this.dragStart.x;
    const dy = e.global.y - this.dragStart.y;
    if (!this.dragMoved && Math.hypot(dx, dy) > 3) this.dragMoved = true;
    if (this.dragMoved) this.world.position.set(this.worldStart.x + dx, this.worldStart.y + dy);
  };

  private onPointerUp = () => {
    this.dragging = false;
  };

  private tick = () => {
    if (!this.ready) return;
    if (this.dirtyStatic) {
      this.dirtyStatic = false;
      this.drawStatic();
    }
    if (this.dirtyDynamic) {
      this.dirtyDynamic = false;
      this.drawDynamic();
    }
  };

  /* ---------- static layers ---------- */

  private px(n: number) {
    return n / this.zoom;
  }

  private drawStatic() {
    const m = this.map;
    this.gridLayer.clear();
    this.edgesLayer.clear();
    this.zonesLayer.removeChildren().forEach((c) => c.destroy({ children: true }));
    this.nodesLayer.removeChildren().forEach((c) => c.destroy({ children: true }));
    this.labelsLayer.removeChildren().forEach((c) => c.destroy({ children: true }));
    this.nodeLabels = [];
    if (!m) return;

    this.drawGrid();
    for (const z of m.zones ?? []) this.drawZone(z);
    for (const e of m.edges ?? []) this.drawEdge(e);
    for (const n of m.nodes ?? []) this.drawNode(n);
  }

  private drawGrid() {
    const b = this.bounds();
    const pad = 5;
    const step = this.zoom < 6 ? 10 : this.zoom < 25 ? 5 : 1;
    const x0 = Math.floor((b.minX - pad) / step) * step;
    const x1 = Math.ceil((b.maxX + pad) / step) * step;
    const y0 = Math.floor((b.minY - pad) / step) * step;
    const y1 = Math.ceil((b.maxY + pad) / step) * step;
    const g = this.gridLayer;
    const color = this.theme === "dark" ? 0x1e2a3a : 0xd7dde6;
    for (let x = x0; x <= x1; x += step) {
      g.moveTo(x, -y0).lineTo(x, -y1);
    }
    for (let y = y0; y <= y1; y += step) {
      g.moveTo(x0, -y).lineTo(x1, -y);
    }
    g.stroke({ width: this.px(1), color, alpha: 0.6 });
    // origin cross
    g.moveTo(-1, 0).lineTo(1, 0).moveTo(0, -1).lineTo(0, 1).stroke({ width: this.px(1.5), color: 0x64748b, alpha: 0.8 });
  }

  private drawZone(z: MapZone) {
    if (z.polygon.length < 3) return;
    const color = ZONE_TYPE_COLORS[z.type] ?? 0x94a3b8;
    const pts = z.polygon.flatMap((p) => [p.x, -p.y]);
    const c = new Container();
    const g = new Graphics();
    g.poly(pts, true).fill({ color, alpha: z.enabled ? 0.14 : 0.06 });
    g.poly(pts, true).stroke({ width: this.px(1.5), color, alpha: z.enabled ? 0.9 : 0.5 });
    c.addChild(g);
    if (!z.enabled) {
      // hatched pattern clipped by the polygon
      const hatch = new Graphics();
      const mask = new Graphics();
      mask.poly(pts, true).fill(0xffffff);
      let minX = Infinity;
      let minY = Infinity;
      let maxX = -Infinity;
      let maxY = -Infinity;
      for (const p of z.polygon) {
        minX = Math.min(minX, p.x);
        maxX = Math.max(maxX, p.x);
        minY = Math.min(minY, -p.y);
        maxY = Math.max(maxY, -p.y);
      }
      const stepW = Math.max(this.px(10), 0.3);
      const span = maxX - minX + (maxY - minY);
      for (let d = 0; d <= span; d += stepW) {
        hatch.moveTo(minX + d, minY).lineTo(minX, minY + d);
        hatch.moveTo(maxX - d, maxY).lineTo(maxX, maxY - d);
      }
      hatch.stroke({ width: this.px(1), color, alpha: 0.5 });
      hatch.mask = mask;
      c.addChild(mask, hatch);
    }
    this.zonesLayer.addChild(c);
    // label at centroid
    const cx = z.polygon.reduce((a, p) => a + p.x, 0) / z.polygon.length;
    const cy = z.polygon.reduce((a, p) => a + p.y, 0) / z.polygon.length;
    const t = this.makeLabel(z.name, color, 10);
    t.alpha = 0.9;
    t.position.set(cx, -cy);
    this.labelsLayer.addChild(t);
    this.nodeLabels.push(t);
  }

  private drawEdge(e: MapEdge) {
    const a = this.nodeIndex.get(e.from);
    const b = this.nodeIndex.get(e.to);
    if (!a || !b) return;
    const g = this.edgesLayer;
    const color = e.enabled ? (this.theme === "dark" ? 0x64748b : 0x94a3b8) : 0x7f1d1d;
    const width = this.px(e.enabled ? 1.5 : 1.2);
    const x1 = a.x;
    const y1 = -a.y;
    const x2 = b.x;
    const y2 = -b.y;
    if (e.enabled) {
      g.moveTo(x1, y1).lineTo(x2, y2).stroke({ width, color, alpha: 0.9 });
    } else {
      dashedLine(g, x1, y1, x2, y2, this.px(8), this.px(6));
      g.stroke({ width, color: 0xef4444, alpha: 0.7 });
    }
    // arrows
    const s = this.px(7);
    const mx = (x1 + x2) / 2;
    const my = (y1 + y2) / 2;
    const ang = Math.atan2(y2 - y1, x2 - x1);
    arrow(g, mx, my, ang, s);
    g.fill({ color: e.enabled ? 0x94a3b8 : 0xef4444, alpha: 0.9 });
    if (e.bidirectional) {
      const t = 0.35;
      arrow(g, x1 + (x2 - x1) * t, y1 + (y2 - y1) * t, ang + Math.PI, s);
      arrow(g, x1 + (x2 - x1) * (1 - t), y1 + (y2 - y1) * (1 - t), ang, s);
      g.fill({ color: e.enabled ? 0x94a3b8 : 0xef4444, alpha: 0.9 });
    }
  }

  private drawNode(n: MapNode) {
    const color = NODE_TYPE_COLORS[n.type] ?? 0x94a3b8;
    const g = new Graphics();
    const r = Math.max(this.px(5), 0.15);
    if (n.type === "waypoint") {
      g.circle(0, 0, r * 0.8).fill({ color, alpha: 0.9 });
    } else {
      g.rect(-r, -r, 2 * r, 2 * r).fill({ color, alpha: 0.95 }).stroke({ width: this.px(1), color: 0x0b0f14, alpha: 0.6 });
    }
    if (n.theta !== null && n.theta !== undefined) {
      const L = r * 2.2;
      g.moveTo(0, 0)
        .lineTo(Math.cos(n.theta) * L, -Math.sin(n.theta) * L)
        .stroke({ width: this.px(1.5), color: 0xffffff, alpha: 0.8 });
    }
    g.position.set(n.x, -n.y);
    g.eventMode = "static";
    g.cursor = "pointer";
    g.hitArea = new Rectangle(-r * 1.6, -r * 1.6, r * 3.2, r * 3.2);
    g.on("pointertap", (ev: FederatedPointerEvent) => {
      ev.stopPropagation();
      if (!this.dragMoved) this.callbacks.onNodeClick?.(n);
    });
    this.nodesLayer.addChild(g);

    const t = this.makeLabel(n.name || n.id, this.theme === "dark" ? 0xcbd5e1 : 0x334155, 10);
    t.anchor.set(0.5, 0);
    t.position.set(n.x, -n.y + r * 1.4);
    this.labelsLayer.addChild(t);
    this.nodeLabels.push(t);
  }

  private makeLabel(text: string, fill: number, fontSize: number): Text {
    const t = new Text({
      text,
      style: { fontFamily: LABEL_FONT, fontSize, fill, align: "center" },
      resolution: 2,
    });
    t.anchor.set(0.5);
    t.scale.set(1 / this.zoom);
    return t;
  }

  /* ---------- dynamic layers ---------- */

  private drawDynamic() {
    const snap = this.snapshot;
    this.routesLayer.clear();
    this.reservationsLayer.clear();
    if (!snap) {
      for (const [id, s] of this.vehicleSprites) {
        s.group.destroy({ children: true });
        this.vehicleSprites.delete(id);
      }
      return;
    }
    this.drawReservations(snap.reservations);
    this.drawRoutes(snap.missions, snap.vehicles);
    this.drawVehicles(snap.vehicles);
  }

  private drawReservations(reservations: Reservation[]) {
    const g = this.reservationsLayer;
    for (const r of reservations) {
      if (r.state === "released") continue;
      const uncertain = r.state === "uncertain";
      const color = uncertain ? UNCERTAIN_COLOR : vehicleColorHex(r.vehicleId);
      const alpha = r.state === "requested" ? 0.25 : uncertain ? 0.8 : 0.55;
      if (r.resource.type === "node") {
        const n = this.nodeIndex.get(r.resource.id);
        if (!n) continue;
        const rad = Math.max(this.px(10), 0.4);
        g.circle(n.x, -n.y, rad).stroke({ width: this.px(2.5), color, alpha });
        if (uncertain) g.circle(n.x, -n.y, rad * 1.4).stroke({ width: this.px(1), color, alpha: 0.6 });
      } else if (r.resource.type === "edge") {
        const e = this.edgeIndex.get(r.resource.id);
        if (!e) continue;
        const a = this.nodeIndex.get(e.from);
        const b = this.nodeIndex.get(e.to);
        if (!a || !b) continue;
        g.moveTo(a.x, -a.y).lineTo(b.x, -b.y).stroke({ width: Math.max(this.px(6), e.widthM * 0.6), color, alpha: alpha * 0.6 });
      } else if (r.resource.type === "zone") {
        const z = this.zoneIndex.get(r.resource.id);
        if (!z || z.polygon.length < 3) continue;
        g.poly(z.polygon.flatMap((p) => [p.x, -p.y]), true).fill({ color, alpha: alpha * 0.35 });
      }
    }
  }

  private drawRoutes(missions: Mission[], vehicles: VehicleWithState[]) {
    const g = this.routesLayer;
    const poseById = new Map(vehicles.map((v) => [v.vehicle.id, v.state.pose]));
    for (const m of missions) {
      if (!m.vehicleId || !m.routeNodeIds || m.routeNodeIds.length === 0) continue;
      const color = vehicleColorHex(m.vehicleId);
      const pose = poseById.get(m.vehicleId);
      let first = true;
      if (pose) {
        g.moveTo(pose.x, -pose.y);
        first = false;
      }
      for (const id of m.routeNodeIds) {
        const n = this.nodeIndex.get(id);
        if (!n) continue;
        if (first) {
          g.moveTo(n.x, -n.y);
          first = false;
        } else {
          g.lineTo(n.x, -n.y);
        }
      }
      g.stroke({ width: this.px(3), color, alpha: 0.85, cap: "round", join: "round" });
      const dest = this.nodeIndex.get(m.routeNodeIds[m.routeNodeIds.length - 1]);
      if (dest) g.circle(dest.x, -dest.y, this.px(6)).stroke({ width: this.px(2), color, alpha: 0.9 });
    }
  }

  private drawVehicles(vehicles: VehicleWithState[]) {
    const seen = new Set<string>();
    for (const vs of vehicles) {
      const id = vs.vehicle.id;
      seen.add(id);
      const st = vs.state;
      if (!st?.pose) continue;
      let sprite = this.vehicleSprites.get(id);
      if (!sprite) {
        const group = new Container();
        const body = new Graphics();
        const hud = new Graphics();
        const label = this.makeLabel(vs.vehicle.name || id, this.theme === "dark" ? 0xffffff : 0x111827, 11);
        label.anchor.set(0.5, 1);
        label.visible = this.visible.labels;
        group.addChild(body, hud, label);
        body.eventMode = "static";
        body.cursor = "pointer";
        body.on("pointertap", (ev: FederatedPointerEvent) => {
          ev.stopPropagation();
          if (!this.dragMoved) this.callbacks.onVehicleClick?.(id);
        });
        this.vehiclesLayer.addChild(group);
        sprite = { group, body, hud, label, key: "" };
        this.vehicleSprites.set(id, sprite);
      }
      const { lengthM, widthM } = this.envelope(vs);
      const selected = this.selectedVehicleId === id;
      const key = [lengthM, widthM, st.availability, st.paused, st.charging, selected, this.zoom, st.safetyState?.eStop, this.theme].join("|");
      if (sprite.key !== key) {
        sprite.key = key;
        this.drawVehicleBody(sprite.body, lengthM, widthM, vs, selected);
      }
      sprite.group.position.set(st.pose.x, -st.pose.y);
      sprite.body.rotation = -(st.pose.theta || 0);
      const r = Math.hypot(lengthM, widthM) / 2;
      // HUD (battery bar) — not rotated
      const hud = sprite.hud;
      hud.clear();
      const bw = Math.max(lengthM, 1.5);
      const bh = Math.max(this.px(4), 0.12);
      const by = r + 0.2;
      const pct = clamp(st.batteryCharge ?? 0, 0, 100) / 100;
      const bcolor = pct < 0.15 ? 0xef4444 : pct < 0.3 ? 0xf59e0b : 0x22c55e;
      hud.rect(-bw / 2, by, bw, bh).fill({ color: 0x000000, alpha: 0.5 });
      hud.rect(-bw / 2, by, bw * pct, bh).fill({ color: bcolor, alpha: 0.95 });
      if (sprite.label.text !== (vs.vehicle.name || id)) sprite.label.text = vs.vehicle.name || id;
      sprite.label.position.set(0, -(r + 0.15));
      sprite.label.visible = this.visible.labels;
    }
    for (const [id, s] of this.vehicleSprites) {
      if (!seen.has(id)) {
        s.group.destroy({ children: true });
        this.vehicleSprites.delete(id);
      }
    }
  }

  private envelope(vs: VehicleWithState): { lengthM: number; widthM: number } {
    const env = vs.state.loadEnvelope;
    if (env && env.lengthM > 0 && env.widthM > 0) return env;
    const fp = vs.vehicle.footprint;
    if (fp && fp.lengthM > 0 && fp.widthM > 0) return fp;
    const model = this.models.get(vs.vehicle.modelId);
    if (model) return { lengthM: model.lengthM, widthM: model.widthM };
    return { lengthM: 6, widthM: 2.4 };
  }

  private drawVehicleBody(g: Graphics, L: number, W: number, vs: VehicleWithState, selected: boolean) {
    const st = vs.state;
    const fill = AVAILABILITY_COLORS[st.availability] ?? 0x6b7280;
    const outline = vehicleColorHex(vs.vehicle.id);
    g.clear();
    if (selected) {
      g.roundRect(-L / 2 - 0.4, -W / 2 - 0.4, L + 0.8, W + 0.8, 0.3).stroke({ width: this.px(2), color: 0xffffff, alpha: 0.9 });
    }
    g.roundRect(-L / 2, -W / 2, L, W, Math.min(0.2, W / 6))
      .fill({ color: fill, alpha: st.connection === "online" ? 0.85 : 0.45 })
      .stroke({ width: this.px(2), color: outline, alpha: 1 });
    // heading indicator (front = +x)
    const hs = Math.min(W * 0.4, L * 0.25);
    g.poly([L / 2, 0, L / 2 - hs, -hs * 0.8, L / 2 - hs, hs * 0.8], true).fill({ color: 0xffffff, alpha: 0.9 });
    if (st.paused) {
      const bw = Math.min(W, L) * 0.12;
      g.rect(-bw * 1.5, -bw * 2, bw, bw * 4).rect(bw * 0.5, -bw * 2, bw, bw * 4).fill({ color: 0x0b0f14, alpha: 0.8 });
    }
    if (st.safetyState?.eStop && st.safetyState.eStop !== "none") {
      g.circle(0, 0, Math.min(W, L) * 0.2).fill({ color: 0xef4444 }).stroke({ width: this.px(1.5), color: 0xffffff });
    }
    g.hitArea = new Rectangle(-L / 2, -W / 2, L, W);
  }
}

function dashedLine(g: Graphics, x1: number, y1: number, x2: number, y2: number, dash: number, gap: number) {
  const dx = x2 - x1;
  const dy = y2 - y1;
  const len = Math.hypot(dx, dy);
  if (len === 0) return;
  const ux = dx / len;
  const uy = dy / len;
  let t = 0;
  while (t < len) {
    const e = Math.min(t + dash, len);
    g.moveTo(x1 + ux * t, y1 + uy * t).lineTo(x1 + ux * e, y1 + uy * e);
    t = e + gap;
  }
}

function arrow(g: Graphics, x: number, y: number, ang: number, s: number) {
  const c = Math.cos(ang);
  const sn = Math.sin(ang);
  const tipX = x + c * s;
  const tipY = y + sn * s;
  const bx = x - c * s * 0.6;
  const by = y - sn * s * 0.6;
  const px = -sn * s * 0.6;
  const py = c * s * 0.6;
  g.poly([tipX, tipY, bx + px, by + py, bx - px, by - py], true);
}

export function polygonCentroid(points: Point[]): Point {
  if (points.length === 0) return { x: 0, y: 0 };
  return {
    x: points.reduce((a, p) => a + p.x, 0) / points.length,
    y: points.reduce((a, p) => a + p.y, 0) / points.length,
  };
}
