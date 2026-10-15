import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { Site } from "@/lib/types";

interface SiteState {
  siteId: string;
  sites: Site[];
  setSiteId: (id: string) => void;
  setSites: (sites: Site[]) => void;
}

export const useSiteStore = create<SiteState>()(
  persist(
    (set) => ({
      siteId: "site-default",
      sites: [],
      setSiteId: (siteId) => set({ siteId }),
      setSites: (sites) => set({ sites }),
    }),
    { name: "fm-site", partialize: (s) => ({ siteId: s.siteId }) },
  ),
);
