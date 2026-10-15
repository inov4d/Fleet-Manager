"use client";
import { useEffect } from "react";
import { api } from "@/lib/api";
import { useFleetStore } from "@/stores/fleet";
import { useSiteStore } from "@/stores/site";

/** Loads the active map of the current site into the fleet store (refetched on `map.activated`). */
export function useActiveMap() {
  const siteId = useSiteStore((s) => s.siteId);
  const reload = useFleetStore((s) => s.mapReloadToken);
  const activeMap = useFleetStore((s) => s.activeMap);
  const setActiveMap = useFleetStore((s) => s.setActiveMap);

  useEffect(() => {
    let cancelled = false;
    api.maps
      .active(siteId)
      .then((m) => {
        if (!cancelled) setActiveMap(m);
      })
      .catch(() => {
        if (!cancelled) setActiveMap(null);
      });
    return () => {
      cancelled = true;
    };
  }, [siteId, reload, setActiveMap]);

  return activeMap;
}
