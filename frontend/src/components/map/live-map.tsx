"use client";
import { useEffect, useRef, useState } from "react";
import { Maximize2 } from "lucide-react";
import { MapRenderer, DEFAULT_LAYERS, type MapLayers } from "@/components/map/map-renderer";
import type { FleetMap, MapNode, VehicleModel } from "@/lib/types";
import { useFleetStore } from "@/stores/fleet";
import { useUiStore } from "@/stores/ui";
import { IconButton } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export interface LiveMapProps {
  map: FleetMap | null;
  models?: VehicleModel[];
  className?: string;
  /** Hide the toolbar overlay and ignore clicks (dashboard). */
  readOnly?: boolean;
  selectedVehicleId?: string | null;
  onVehicleClick?: (id: string) => void;
  onNodeClick?: (node: MapNode) => void;
  onBackgroundClick?: () => void;
  layers?: MapLayers;
  onLayersChange?: (l: MapLayers) => void;
}

const LAYER_LABELS: Record<keyof MapLayers, string> = {
  zones: "Zones",
  edges: "Arêtes",
  reservations: "Réservations",
  routes: "Itinéraires",
  labels: "Étiquettes",
};

/** Live 2D map (PixiJS). Snapshots are pushed from the fleet store at ~4 Hz without React re-renders. */
export function LiveMap({
  map,
  models,
  className,
  readOnly,
  selectedVehicleId,
  onVehicleClick,
  onNodeClick,
  onBackgroundClick,
  layers: layersProp,
  onLayersChange,
}: LiveMapProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rendererRef = useRef<MapRenderer | null>(null);
  const [ready, setReady] = useState(false);
  const [internalLayers, setInternalLayers] = useState<MapLayers>(DEFAULT_LAYERS);
  const layers = layersProp ?? internalLayers;
  const theme = useUiStore((s) => s.theme);
  const callbacksRef = useRef({ onVehicleClick, onNodeClick, onBackgroundClick });
  callbacksRef.current = { onVehicleClick, onNodeClick, onBackgroundClick };

  // Create / destroy the Pixi application.
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const renderer = new MapRenderer();
    rendererRef.current = renderer;
    let cancelled = false;
    renderer.setCallbacks({
      onVehicleClick: (id) => {
        if (!readOnly) callbacksRef.current.onVehicleClick?.(id);
      },
      onNodeClick: (n) => {
        if (!readOnly) callbacksRef.current.onNodeClick?.(n);
      },
      onBackgroundClick: () => callbacksRef.current.onBackgroundClick?.(),
    });
    renderer
      .init(el, useUiStore.getState().theme)
      .then(() => {
        if (!cancelled) setReady(true);
      })
      .catch((err: unknown) => console.error("Pixi init failed", err));

    const ro = new ResizeObserver((entries) => {
      const r = entries[0]?.contentRect;
      if (r) renderer.resize(r.width, r.height);
    });
    ro.observe(el);

    return () => {
      cancelled = true;
      ro.disconnect();
      renderer.destroy();
      rendererRef.current = null;
      setReady(false);
    };
  }, [readOnly]);

  useEffect(() => {
    if (ready) rendererRef.current?.setMap(map);
  }, [map, ready]);

  useEffect(() => {
    if (ready && models) rendererRef.current?.setVehicleModels(models);
  }, [models, ready]);

  useEffect(() => {
    if (ready) rendererRef.current?.setLayers(layers);
  }, [layers, ready]);

  useEffect(() => {
    if (ready) rendererRef.current?.setSelectedVehicle(selectedVehicleId ?? null);
  }, [selectedVehicleId, ready]);

  useEffect(() => {
    if (ready) rendererRef.current?.setTheme(theme);
  }, [theme, ready]);

  // Push snapshots straight into the renderer (no React re-render at 4 Hz).
  useEffect(() => {
    if (!ready) return;
    const push = () => {
      const s = useFleetStore.getState();
      rendererRef.current?.setSnapshot({ vehicles: s.vehicles, missions: s.missions, reservations: s.reservations });
    };
    push();
    const unsub = useFleetStore.subscribe((s, prev) => {
      if (s.vehicles !== prev.vehicles || s.missions !== prev.missions || s.reservations !== prev.reservations) push();
    });
    return unsub;
  }, [ready]);

  const setLayers = (l: MapLayers) => {
    if (onLayersChange) onLayersChange(l);
    else setInternalLayers(l);
  };

  return (
    <div className={cn("relative w-full h-full min-h-40 overflow-hidden bg-bg", className)}>
      <div ref={containerRef} className="absolute inset-0" aria-label="Carte en direct" role="img" />
      {!map && (
        <div className="absolute inset-0 flex items-center justify-center text-xs text-fg-muted pointer-events-none">
          Aucune carte active pour ce site
        </div>
      )}
      <div className="absolute top-2 right-2 flex flex-col items-end gap-1">
        <IconButton label="Ajuster à la carte" variant="secondary" onClick={() => rendererRef.current?.fitToMap()}>
          <Maximize2 className="size-4" />
        </IconButton>
      </div>
      {!readOnly && (
        <div className="absolute bottom-2 left-2 flex flex-wrap gap-1 rounded-md border border-border bg-surface/90 backdrop-blur px-2 py-1" role="group" aria-label="Calques">
          {(Object.keys(LAYER_LABELS) as Array<keyof MapLayers>).map((k) => (
            <label key={k} className="inline-flex items-center gap-1 text-[11px] cursor-pointer select-none">
              <input type="checkbox" checked={layers[k]} onChange={(e) => setLayers({ ...layers, [k]: e.target.checked })} />
              {LAYER_LABELS[k]}
            </label>
          ))}
        </div>
      )}
    </div>
  );
}
