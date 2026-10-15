import { create } from "zustand";
import type { FleetEvent, StoredEvent } from "@/lib/types";

const CAP = 500;
let seq = 0;

interface EventsState {
  events: StoredEvent[];
  push: (ts: string, event: FleetEvent) => StoredEvent;
  clear: () => void;
}

export const useEventsStore = create<EventsState>()((set) => ({
  events: [],
  push: (ts, event) => {
    const stored: StoredEvent = { ...event, ts, id: ++seq };
    set((s) => {
      const next = [stored, ...s.events];
      if (next.length > CAP) next.length = CAP;
      return { events: next };
    });
    return stored;
  },
  clear: () => set({ events: [] }),
}));
