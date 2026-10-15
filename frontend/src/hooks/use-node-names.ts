"use client";
import { useCallback } from "react";
import { useFleetStore } from "@/stores/fleet";

/** Returns a resolver node id → node name based on the active map. */
export function useNodeName() {
  const map = useFleetStore((s) => s.activeMap);
  return useCallback(
    (id: string | null | undefined): string => {
      if (!id) return "—";
      const n = map?.nodes?.find((x) => x.id === id);
      return n?.name || id;
    },
    [map],
  );
}

/** Returns a resolver vehicle id → vehicle name based on the live snapshot. */
export function useVehicleName() {
  const vehicles = useFleetStore((s) => s.vehicles);
  return useCallback(
    (id: string | null | undefined): string => {
      if (!id) return "—";
      const v = vehicles.find((x) => x.vehicle.id === id);
      return v?.vehicle.name || id;
    },
    [vehicles],
  );
}
