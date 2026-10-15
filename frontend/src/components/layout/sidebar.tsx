"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Activity,
  BarChart3,
  Bell,
  Box,
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  FlaskConical,
  LayoutDashboard,
  Map as MapIcon,
  PencilRuler,
  Settings,
  Truck,
  Wrench,
} from "lucide-react";
import { useUiStore } from "@/stores/ui";
import { useFleetStore } from "@/stores/fleet";
import { cn } from "@/lib/utils";

interface NavItem {
  href: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  badge?: "alarms";
}

const NAV: NavItem[] = [
  { href: "/", label: "Tableau de bord", icon: LayoutDashboard },
  { href: "/map", label: "Carte en direct", icon: MapIcon },
  { href: "/vehicles", label: "Véhicules", icon: Truck },
  { href: "/missions", label: "Missions", icon: ClipboardList },
  { href: "/traffic", label: "Trafic", icon: Activity },
  { href: "/alarms", label: "Alarmes", icon: Bell, badge: "alarms" },
  { href: "/map-editor", label: "Éditeur de carte", icon: PencilRuler },
  { href: "/maintenance", label: "Maintenance", icon: Wrench },
  { href: "/analytics", label: "Analytique", icon: BarChart3 },
  { href: "/simulation", label: "Simulation", icon: FlaskConical },
  { href: "/3d", label: "Vue 3D", icon: Box },
  { href: "/settings", label: "Paramètres", icon: Settings },
];

export function Sidebar() {
  const pathname = usePathname();
  const collapsed = useUiStore((s) => s.sidebarCollapsed);
  const toggle = useUiStore((s) => s.toggleSidebar);
  const alarmCount = useFleetStore((s) => s.alarms.length);

  return (
    <nav
      aria-label="Navigation principale"
      className={cn(
        "flex flex-col border-r border-border bg-surface shrink-0 transition-[width] duration-150",
        collapsed ? "w-12" : "w-52",
      )}
    >
      <div className={cn("flex items-center h-12 border-b border-border px-3 gap-2", collapsed && "justify-center px-0")}>
        <div className="size-7 rounded bg-accent-soft text-accent flex items-center justify-center shrink-0">
          <Truck className="size-4" aria-hidden />
        </div>
        {!collapsed && (
          <div className="leading-tight min-w-0">
            <div className="text-[13px] font-semibold truncate">Fleet Manager</div>
            <div className="text-[10px] text-fg-muted truncate">Supervision AGV</div>
          </div>
        )}
      </div>
      <ul className="flex-1 py-2 overflow-y-auto">
        {NAV.map((item) => {
          const active = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
          const Icon = item.icon;
          return (
            <li key={item.href}>
              <Link
                href={item.href}
                aria-current={active ? "page" : undefined}
                title={collapsed ? item.label : undefined}
                className={cn(
                  "flex items-center gap-2.5 mx-1.5 my-0.5 rounded-md px-2 h-8 text-[13px] transition-colors",
                  active ? "bg-accent-soft text-accent font-medium" : "text-fg-muted hover:bg-surface-2 hover:text-fg",
                  collapsed && "justify-center px-0",
                )}
              >
                <Icon className="size-4 shrink-0" aria-hidden />
                {!collapsed && <span className="truncate flex-1">{item.label}</span>}
                {item.badge === "alarms" && alarmCount > 0 && (
                  <span
                    className={cn(
                      "rounded-full bg-danger text-white text-[10px] font-semibold px-1.5 min-w-4 h-4 inline-flex items-center justify-center tabular",
                      collapsed && "absolute ml-5 -mt-5",
                    )}
                    aria-label={`${alarmCount} alarmes actives`}
                  >
                    {alarmCount}
                  </span>
                )}
              </Link>
            </li>
          );
        })}
      </ul>
      <button
        type="button"
        onClick={toggle}
        aria-label={collapsed ? "Déplier la navigation" : "Replier la navigation"}
        className="h-9 border-t border-border text-fg-muted hover:text-fg flex items-center justify-center"
      >
        {collapsed ? <ChevronRight className="size-4" /> : <ChevronLeft className="size-4" />}
      </button>
    </nav>
  );
}
