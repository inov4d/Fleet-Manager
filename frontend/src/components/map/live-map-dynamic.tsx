"use client";
import dynamic from "next/dynamic";
import { Spinner } from "@/components/ui/card";

/** Client-only wrapper (PixiJS needs the DOM). */
export const LiveMapDynamic = dynamic(() => import("@/components/map/live-map").then((m) => m.LiveMap), {
  ssr: false,
  loading: () => <Spinner className="h-full" />,
});
