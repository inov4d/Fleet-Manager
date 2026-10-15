"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { Bell, ChevronDown, LogOut, Moon, Sun, User as UserIcon, Wifi, WifiOff } from "lucide-react";
import { api } from "@/lib/api";
import { fleetSocket } from "@/lib/ws";
import { ROLE_LABELS } from "@/lib/auth";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import { useFleetStore } from "@/stores/fleet";
import { useUiStore } from "@/stores/ui";
import { cn } from "@/lib/utils";
import { fmtTime } from "@/lib/format";

export function TopBar() {
  const router = useRouter();
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const siteId = useSiteStore((s) => s.siteId);
  const sites = useSiteStore((s) => s.sites);
  const setSiteId = useSiteStore((s) => s.setSiteId);
  const wsStatus = useFleetStore((s) => s.wsStatus);
  const lastTs = useFleetStore((s) => s.lastSnapshotTs);
  const alarms = useFleetStore((s) => s.alarms);
  const simulation = useFleetStore((s) => s.simulation);
  const theme = useUiStore((s) => s.theme);
  const toggleTheme = useUiStore((s) => s.toggleTheme);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [menuOpen]);

  const allowedSites = user?.siteIds?.length ? sites.filter((s) => user.siteIds.includes(s.id)) : sites;
  const siteOptions = allowedSites.length > 0 ? allowedSites : [{ id: siteId, name: siteId }];
  const critical = alarms.filter((a) => a.severity === "critical" || a.severity === "error").length;

  const doLogout = async () => {
    try {
      await api.auth.logout();
    } catch {
      /* ignore */
    }
    fleetSocket.disconnect();
    logout();
    router.replace("/login");
  };

  const wsLabel =
    wsStatus === "connected"
      ? "Temps réel connecté"
      : wsStatus === "reconnecting"
        ? "Reconnexion…"
        : wsStatus === "connecting"
          ? "Connexion…"
          : "Déconnecté";

  return (
    <header className="h-12 border-b border-border bg-surface flex items-center gap-3 px-3 shrink-0">
      <label className="flex items-center gap-2 text-xs text-fg-muted">
        <span className="hidden md:inline">Site</span>
        <select
          aria-label="Site"
          value={siteId}
          onChange={(e) => {
            setSiteId(e.target.value);
            fleetSocket.setSite(e.target.value);
          }}
          className="h-7 rounded-md border border-border bg-bg px-2 text-xs text-fg"
        >
          {siteOptions.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
      </label>

      {simulation?.running && (
        <span className="hidden md:inline-flex items-center rounded border border-warning/50 bg-warning-soft text-warning px-1.5 h-6 text-[11px] font-medium">
          Simulation ×{simulation.timeScale}
        </span>
      )}

      <div className="flex-1" />

      <div
        className={cn(
          "flex items-center gap-1.5 text-xs",
          wsStatus === "connected" ? "text-success" : wsStatus === "disconnected" ? "text-fg-muted" : "text-warning",
        )}
        role="status"
        aria-live="polite"
        title={lastTs ? `Dernier instantané : ${fmtTime(lastTs)}` : wsLabel}
      >
        {wsStatus === "connected" ? <Wifi className="size-4" aria-hidden /> : <WifiOff className="size-4" aria-hidden />}
        <span className="hidden sm:inline">{wsLabel}</span>
        {wsStatus !== "connected" && wsStatus !== "disconnected" && (
          <span className="size-2 rounded-full bg-warning animate-pulse" aria-hidden />
        )}
      </div>

      <Link
        href="/alarms"
        className={cn(
          "relative inline-flex items-center justify-center size-8 rounded-md hover:bg-surface-2",
          critical > 0 ? "text-danger" : alarms.length > 0 ? "text-warning" : "text-fg-muted",
        )}
        aria-label={`${alarms.length} alarmes actives`}
        title={`${alarms.length} alarmes actives`}
      >
        <Bell className="size-4" aria-hidden />
        {alarms.length > 0 && (
          <span className="absolute -top-0.5 -right-0.5 rounded-full bg-danger text-white text-[10px] font-semibold px-1 min-w-4 h-4 inline-flex items-center justify-center tabular">
            {alarms.length}
          </span>
        )}
      </Link>

      <button
        type="button"
        onClick={toggleTheme}
        aria-label={theme === "dark" ? "Passer au thème clair" : "Passer au thème sombre"}
        title={theme === "dark" ? "Thème clair" : "Thème sombre"}
        className="inline-flex items-center justify-center size-8 rounded-md text-fg-muted hover:bg-surface-2 hover:text-fg"
      >
        {theme === "dark" ? <Sun className="size-4" aria-hidden /> : <Moon className="size-4" aria-hidden />}
      </button>

      <div className="relative" ref={menuRef}>
        <button
          type="button"
          onClick={() => setMenuOpen((o) => !o)}
          aria-haspopup="menu"
          aria-expanded={menuOpen}
          className="inline-flex items-center gap-2 h-8 rounded-md px-2 hover:bg-surface-2 text-xs"
        >
          <span className="size-6 rounded-full bg-accent-soft text-accent inline-flex items-center justify-center">
            <UserIcon className="size-3.5" aria-hidden />
          </span>
          <span className="hidden md:flex flex-col items-start leading-tight">
            <span className="font-medium">{user?.displayName || user?.username}</span>
            <span className="text-[10px] text-fg-muted">{user ? ROLE_LABELS[user.role] : ""}</span>
          </span>
          <ChevronDown className="size-3.5 text-fg-muted" aria-hidden />
        </button>
        {menuOpen && (
          <div
            role="menu"
            className="absolute right-0 mt-1 w-48 rounded-md border border-border bg-surface shadow-lg py-1 z-20 fm-fade-in"
          >
            <div className="px-3 py-1.5 text-[11px] text-fg-muted border-b border-border">
              {user?.username} · {user ? ROLE_LABELS[user.role] : ""}
            </div>
            <button
              type="button"
              role="menuitem"
              onClick={doLogout}
              className="w-full text-left px-3 py-1.5 text-xs hover:bg-surface-2 flex items-center gap-2"
            >
              <LogOut className="size-3.5" aria-hidden /> Se déconnecter
            </button>
          </div>
        )}
      </div>
    </header>
  );
}
