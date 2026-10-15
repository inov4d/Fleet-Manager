"use client";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { useActiveMap } from "@/hooks/use-active-map";
import { useApi } from "@/hooks/use-api";
import { api } from "@/lib/api";
import { can } from "@/lib/auth";
import type { MapNode } from "@/lib/types";
import { useAuthStore } from "@/stores/auth";
import { useFleetStore } from "@/stores/fleet";
import { LiveMapDynamic } from "@/components/map/live-map-dynamic";
import { VehiclePanel } from "@/components/map/vehicle-panel";
import { MapLegend } from "@/components/map/map-legend";
import { Button } from "@/components/ui/button";
import { NODE_TYPE_LABELS, label } from "@/lib/labels";
import { fmtCoord } from "@/lib/format";

export default function MapPage() {
  const router = useRouter();
  const map = useActiveMap();
  const user = useAuthStore((s) => s.user);
  const models = useApi(() => api.vehicleModels.list(), []);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedNode, setSelectedNode] = useState<MapNode | null>(null);
  const vehicles = useFleetStore((s) => s.vehicles);
  const selected = useMemo(() => vehicles.find((v) => v.vehicle.id === selectedId) ?? null, [vehicles, selectedId]);
  const nodes = map?.nodes ?? [];

  return (
    <div className="absolute inset-0 flex">
      <div className="flex-1 relative min-w-0">
        <LiveMapDynamic
          map={map}
          models={models.data}
          selectedVehicleId={selectedId}
          onVehicleClick={(id) => {
            setSelectedId(id);
            setSelectedNode(null);
          }}
          onNodeClick={(n) => {
            setSelectedNode(n);
          }}
          onBackgroundClick={() => setSelectedNode(null)}
        />
        <div className="absolute top-2 left-2 rounded-md border border-border bg-surface/90 backdrop-blur px-2 py-1 max-w-[70%]">
          <div className="text-xs font-medium">{map ? `${map.name} · v${map.version}` : "Aucune carte active"}</div>
          <MapLegend />
        </div>
        {selectedNode && (
          <div className="absolute bottom-12 left-2 rounded-md border border-border bg-surface/95 backdrop-blur p-2 w-64 fm-fade-in" role="dialog" aria-label={`Nœud ${selectedNode.name}`}>
            <div className="text-xs font-semibold">{selectedNode.name || selectedNode.id}</div>
            <div className="text-[11px] text-fg-muted">
              {label(NODE_TYPE_LABELS, selectedNode.type)} · {fmtCoord(selectedNode.x)} ; {fmtCoord(selectedNode.y)}
            </div>
            <div className="flex gap-1 mt-2">
              {can(user, "missions.create") && (
                <Button size="xs" variant="primary" onClick={() => router.push(`/missions/new?destination=${encodeURIComponent(selectedNode.id)}`)}>
                  Créer une mission vers ce nœud
                </Button>
              )}
              <Button size="xs" variant="ghost" onClick={() => setSelectedNode(null)}>
                Fermer
              </Button>
            </div>
          </div>
        )}
      </div>
      {selected && <VehiclePanel item={selected} nodes={nodes} onClose={() => setSelectedId(null)} />}
    </div>
  );
}
