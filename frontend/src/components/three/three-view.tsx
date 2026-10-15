"use client";
import { useEffect, useRef } from "react";
import * as THREE from "three";
import { OrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import type { FleetMap, VehicleModel, VehicleWithState } from "@/lib/types";
import { AVAILABILITY_COLORS, NODE_TYPE_COLORS, ZONE_TYPE_COLORS } from "@/lib/labels";
import { useFleetStore } from "@/stores/fleet";
import { useUiStore } from "@/stores/ui";

/** Map x → three x, map y → three -z (y-up world, top view keeps map y pointing up on screen). */
export function ThreeView({ map, models }: { map: FleetMap | null; models: VehicleModel[] | undefined }) {
  const ref = useRef<HTMLDivElement>(null);
  const modelsRef = useRef<Map<string, VehicleModel>>(new Map());
  modelsRef.current = new Map((models ?? []).map((m) => [m.id, m]));
  const theme = useUiStore((s) => s.theme);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.setSize(el.clientWidth, el.clientHeight);
    el.appendChild(renderer.domElement);
    const scene = new THREE.Scene();
    scene.background = new THREE.Color(theme === "dark" ? 0x0b0f14 : 0xf3f5f8);
    const camera = new THREE.PerspectiveCamera(50, el.clientWidth / el.clientHeight, 0.1, 5000);
    const controls = new OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.maxPolarAngle = Math.PI / 2 - 0.05;
    scene.add(new THREE.HemisphereLight(0xffffff, 0x334155, 1.2));
    const sun = new THREE.DirectionalLight(0xffffff, 1.5);
    sun.position.set(50, 100, 30);
    scene.add(sun);

    // Static map geometry
    const staticGroup = new THREE.Group();
    scene.add(staticGroup);
    let cx = 0;
    let cz = 0;
    if (map) {
      const xs = map.nodes.map((n) => n.x).concat(map.zones.flatMap((z) => z.polygon.map((p) => p.x)));
      const ys = map.nodes.map((n) => n.y).concat(map.zones.flatMap((z) => z.polygon.map((p) => p.y)));
      const minX = xs.length ? Math.min(...xs) : 0;
      const maxX = xs.length ? Math.max(...xs) : map.widthM || 50;
      const minY = ys.length ? Math.min(...ys) : 0;
      const maxY = ys.length ? Math.max(...ys) : map.heightM || 30;
      cx = (minX + maxX) / 2;
      cz = -(minY + maxY) / 2;
      const w = Math.max(10, maxX - minX + 10);
      const h = Math.max(10, maxY - minY + 10);
      const ground = new THREE.Mesh(new THREE.PlaneGeometry(w, h), new THREE.MeshStandardMaterial({ color: theme === "dark" ? 0x121821 : 0xe3e8ef }));
      ground.rotation.x = -Math.PI / 2;
      ground.position.set(cx, -0.02, cz);
      staticGroup.add(ground);
      const grid = new THREE.GridHelper(Math.max(w, h), Math.round(Math.max(w, h) / 5), 0x35435a, 0x253040);
      grid.position.set(cx, 0, cz);
      staticGroup.add(grid);

      for (const z of map.zones) {
        if (z.polygon.length < 3) continue;
        const shape = new THREE.Shape(z.polygon.map((p) => new THREE.Vector2(p.x, -p.y)));
        const geom = new THREE.ShapeGeometry(shape);
        const mesh = new THREE.Mesh(geom, new THREE.MeshBasicMaterial({ color: ZONE_TYPE_COLORS[z.type] ?? 0x94a3b8, transparent: true, opacity: z.enabled ? 0.25 : 0.1, side: THREE.DoubleSide }));
        mesh.rotation.x = Math.PI / 2; // Vector2.y → -z after rotation
        mesh.position.y = 0.01;
        staticGroup.add(mesh);
      }
      const nodeById = new Map(map.nodes.map((n) => [n.id, n]));
      const edgePts: number[] = [];
      for (const e of map.edges) {
        const a = nodeById.get(e.from);
        const b = nodeById.get(e.to);
        if (!a || !b) continue;
        edgePts.push(a.x, 0.03, -a.y, b.x, 0.03, -b.y);
      }
      const edgeGeom = new THREE.BufferGeometry();
      edgeGeom.setAttribute("position", new THREE.Float32BufferAttribute(edgePts, 3));
      staticGroup.add(new THREE.LineSegments(edgeGeom, new THREE.LineBasicMaterial({ color: 0x64748b })));
      const nodeGeom = new THREE.CylinderGeometry(0.35, 0.35, 0.1, 16);
      for (const n of map.nodes) {
        const m = new THREE.Mesh(nodeGeom, new THREE.MeshStandardMaterial({ color: NODE_TYPE_COLORS[n.type] ?? 0x94a3b8 }));
        m.position.set(n.x, 0.05, -n.y);
        staticGroup.add(m);
      }
    }
    camera.position.set(cx + 30, 40, cz + 40);
    controls.target.set(cx, 0, cz);
    controls.update();

    // Vehicles
    const vehicleGroup = new THREE.Group();
    scene.add(vehicleGroup);
    const meshes = new Map<string, THREE.Mesh>();
    const dims = (v: VehicleWithState) => {
      const model = modelsRef.current.get(v.vehicle.modelId);
      const env = v.state.loadEnvelope;
      return {
        L: env?.lengthM || v.vehicle.footprint?.lengthM || model?.lengthM || 6,
        W: env?.widthM || v.vehicle.footprint?.widthM || model?.widthM || 2.4,
        H: model?.heightM || 0.5,
      };
    };
    const update = (vehicles: VehicleWithState[]) => {
      const seen = new Set<string>();
      for (const v of vehicles) {
        seen.add(v.vehicle.id);
        let mesh = meshes.get(v.vehicle.id);
        const { L, W, H } = dims(v);
        const h = Math.max(H, 0.3);
        if (!mesh) {
          mesh = new THREE.Mesh(new THREE.BoxGeometry(L, h, W), new THREE.MeshStandardMaterial({ color: 0x3b82f6 }));
          const nose = new THREE.Mesh(new THREE.ConeGeometry(W * 0.2, W * 0.4, 8), new THREE.MeshStandardMaterial({ color: 0xffffff }));
          nose.rotation.z = -Math.PI / 2;
          nose.position.set(L / 2 - W * 0.2, h / 2 + 0.05, 0);
          mesh.add(nose);
          vehicleGroup.add(mesh);
          meshes.set(v.vehicle.id, mesh);
        }
        (mesh.material as THREE.MeshStandardMaterial).color.setHex(AVAILABILITY_COLORS[v.state.availability] ?? 0x6b7280);
        mesh.position.set(v.state.pose?.x ?? 0, h / 2, -(v.state.pose?.y ?? 0));
        mesh.rotation.y = v.state.pose?.theta ?? 0;
      }
      for (const [id, m] of meshes) {
        if (!seen.has(id)) {
          vehicleGroup.remove(m);
          m.geometry.dispose();
          (m.material as THREE.Material).dispose();
          meshes.delete(id);
        }
      }
    };
    update(useFleetStore.getState().vehicles);
    const unsub = useFleetStore.subscribe((s, prev) => {
      if (s.vehicles !== prev.vehicles) update(s.vehicles);
    });

    let raf = 0;
    const loop = () => {
      controls.update();
      renderer.render(scene, camera);
      raf = requestAnimationFrame(loop);
    };
    loop();
    const ro = new ResizeObserver(() => {
      const w = el.clientWidth;
      const h = el.clientHeight;
      if (w === 0 || h === 0) return;
      renderer.setSize(w, h);
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
    });
    ro.observe(el);

    return () => {
      cancelAnimationFrame(raf);
      unsub();
      ro.disconnect();
      controls.dispose();
      scene.traverse((o) => {
        if (o instanceof THREE.Mesh) {
          o.geometry.dispose();
          const mat = o.material as THREE.Material | THREE.Material[];
          if (Array.isArray(mat)) mat.forEach((m) => m.dispose());
          else mat.dispose();
        }
      });
      renderer.dispose();
      el.removeChild(renderer.domElement);
    };
  }, [map, theme]);

  return <div ref={ref} className="absolute inset-0" role="img" aria-label="Vue 3D de la flotte" />;
}
