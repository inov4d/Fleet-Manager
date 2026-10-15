import { create } from "zustand";
import type { Alarm, FleetMap, Kpis, Mission, Reservation, SimulationStatus, Snapshot, VehicleWithState } from "@/lib/types";

export type WsStatus = "disconnected" | "connecting" | "connected" | "reconnecting";

interface FleetState {
  wsStatus: WsStatus;
  lastSnapshotTs: string | null;
  vehicles: VehicleWithState[];
  missions: Mission[];
  reservations: Reservation[];
  alarms: Alarm[];
  kpis: Kpis | null;
  simulation: SimulationStatus | null;
  /** Active map of the current site, loaded through the API. */
  activeMap: FleetMap | null;
  /** Incremented on `map.activated` events so consumers refetch the active map. */
  mapReloadToken: number;
  setWsStatus: (s: WsStatus) => void;
  setSnapshot: (ts: string, data: Snapshot) => void;
  setActiveMap: (map: FleetMap | null) => void;
  bumpMapReload: () => void;
  reset: () => void;
}

export const useFleetStore = create<FleetState>()((set) => ({
  wsStatus: "disconnected",
  lastSnapshotTs: null,
  vehicles: [],
  missions: [],
  reservations: [],
  alarms: [],
  kpis: null,
  simulation: null,
  activeMap: null,
  mapReloadToken: 0,
  setWsStatus: (wsStatus) => set({ wsStatus }),
  setSnapshot: (ts, data) =>
    set({
      lastSnapshotTs: ts,
      vehicles: data.vehicles ?? [],
      missions: data.missions ?? [],
      reservations: data.reservations ?? [],
      alarms: data.alarms ?? [],
      kpis: data.kpis ?? null,
      simulation: data.simulation ?? null,
    }),
  setActiveMap: (activeMap) => set({ activeMap }),
  bumpMapReload: () => set((s) => ({ mapReloadToken: s.mapReloadToken + 1 })),
  reset: () =>
    set({
      vehicles: [],
      missions: [],
      reservations: [],
      alarms: [],
      kpis: null,
      simulation: null,
      lastSnapshotTs: null,
    }),
}));
