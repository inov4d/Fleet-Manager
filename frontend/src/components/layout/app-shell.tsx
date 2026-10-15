"use client";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api, setUnauthorizedHandler } from "@/lib/api";
import { fleetSocket } from "@/lib/ws";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import { useFleetStore } from "@/stores/fleet";
import { useUiStore } from "@/stores/ui";
import { Sidebar } from "@/components/layout/sidebar";
import { TopBar } from "@/components/layout/topbar";
import { ToastViewport } from "@/components/ui/toast";
import { Spinner } from "@/components/ui/card";

/** Authenticated shell: guards the route, boots the WebSocket, renders sidebar + top bar. */
export function AppShell({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const token = useAuthStore((s) => s.token);
  const hydrated = useAuthStore((s) => s.hydrated);
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const setUser = useAuthStore((s) => s.setUser);
  const siteId = useSiteStore((s) => s.siteId);
  const setSites = useSiteStore((s) => s.setSites);
  const setSiteId = useSiteStore((s) => s.setSiteId);
  const resetFleet = useFleetStore((s) => s.reset);
  const collapsed = useUiStore((s) => s.sidebarCollapsed);
  const setTheme = useUiStore((s) => s.setTheme);
  const [verified, setVerified] = useState(false);

  // Sync the theme store with the attribute applied by the inline script.
  useEffect(() => {
    const t = document.documentElement.getAttribute("data-theme");
    if (t === "light") setTheme("light");
  }, [setTheme]);

  // 401 anywhere → back to login.
  useEffect(() => {
    setUnauthorizedHandler(() => {
      logout();
      fleetSocket.disconnect();
      if (window.location.pathname !== "/login") router.replace("/login");
    });
    return () => setUnauthorizedHandler(() => undefined);
  }, [logout, router]);

  // Route guard + token validation.
  useEffect(() => {
    if (!hydrated) return;
    if (!token) {
      router.replace("/login");
      return;
    }
    let cancelled = false;
    api.auth
      .me()
      .then((u) => {
        if (cancelled) return;
        setUser(u);
        setVerified(true);
      })
      .catch(() => {
        /* a 401 is handled by the unauthorized handler; other errors keep the cached user */
        if (!cancelled) setVerified(true);
      });
    return () => {
      cancelled = true;
    };
  }, [hydrated, token, router, setUser]);

  // Sites list + make sure the selected site is valid.
  useEffect(() => {
    if (!verified) return;
    api.sites
      .list()
      .then((sites) => {
        setSites(sites);
        const allowed = user?.siteIds?.length ? sites.filter((s) => user.siteIds.includes(s.id)) : sites;
        if (allowed.length > 0 && !allowed.some((s) => s.id === siteId)) setSiteId(allowed[0].id);
      })
      .catch(() => undefined);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [verified]);

  // WebSocket lifecycle.
  useEffect(() => {
    if (!verified || !token) return;
    fleetSocket.connect(token, siteId);
    return () => {
      fleetSocket.disconnect();
      resetFleet();
    };
  }, [verified, token, resetFleet, siteId]);

  if (!hydrated || !token || !verified) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Spinner />
      </div>
    );
  }

  const fullBleed = pathname === "/map" || pathname === "/3d";

  return (
    <div className="h-screen flex overflow-hidden">
      <Sidebar />
      <div className="flex-1 flex flex-col min-w-0">
        <TopBar />
        <main className={fullBleed ? "flex-1 min-h-0 relative" : "flex-1 min-h-0 overflow-y-auto p-3 md:p-4"}>
          <div className={fullBleed ? "absolute inset-0" : collapsed ? "max-w-[1800px]" : "max-w-[1800px]"}>{children}</div>
        </main>
      </div>
      <ToastViewport />
    </div>
  );
}
