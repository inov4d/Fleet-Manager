"use client";
import dynamic from "next/dynamic";
import { useActiveMap } from "@/hooks/use-active-map";
import { useApi } from "@/hooks/use-api";
import { api } from "@/lib/api";
import { Spinner } from "@/components/ui/card";
import { MapLegend } from "@/components/map/map-legend";

const ThreeView = dynamic(() => import("@/components/three/three-view").then((m) => m.ThreeView), { ssr: false, loading: () => <Spinner className="h-full" /> });

export default function ThreeDPage() {
  const map = useActiveMap();
  const models = useApi(() => api.vehicleModels.list(), []);
  return (
    <div className="absolute inset-0">
      <ThreeView map={map} models={models.data} />
      <div className="absolute top-2 left-2 rounded-md border border-border bg-surface/90 backdrop-blur px-2 py-1 max-w-[70%]">
        <div className="text-xs font-medium">Vue 3D {map ? `· ${map.name}` : "· aucune carte active"}</div>
        <p className="text-[11px] text-fg-muted">Glisser : orbite · molette : zoom · clic droit : panoramique</p>
        <MapLegend />
      </div>
    </div>
  );
}
