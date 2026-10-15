import { create } from "zustand";

export type Theme = "dark" | "light";
export type ToastKind = "info" | "success" | "warning" | "error";

export interface Toast {
  id: number;
  kind: ToastKind;
  title: string;
  message?: string;
  href?: string;
}

interface UiState {
  theme: Theme;
  sidebarCollapsed: boolean;
  toasts: Toast[];
  setTheme: (t: Theme) => void;
  toggleTheme: () => void;
  toggleSidebar: () => void;
  pushToast: (t: Omit<Toast, "id">, ttlMs?: number) => void;
  dismissToast: (id: number) => void;
}

let toastSeq = 0;

function applyTheme(theme: Theme) {
  if (typeof document === "undefined") return;
  document.documentElement.setAttribute("data-theme", theme);
  try {
    localStorage.setItem("fm-theme", theme);
  } catch {
    /* ignore storage errors */
  }
}

export const useUiStore = create<UiState>()((set, get) => ({
  theme: "dark",
  sidebarCollapsed: false,
  toasts: [],
  setTheme: (theme) => {
    applyTheme(theme);
    set({ theme });
  },
  toggleTheme: () => get().setTheme(get().theme === "dark" ? "light" : "dark"),
  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
  pushToast: (t, ttlMs = 6000) => {
    const id = ++toastSeq;
    set((s) => ({ toasts: [...s.toasts.slice(-5), { ...t, id }] }));
    if (ttlMs > 0) {
      setTimeout(() => get().dismissToast(id), ttlMs);
    }
  },
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((x) => x.id !== id) })),
}));

/** Convenience for non-React code. */
export const toast = {
  info: (title: string, message?: string) => useUiStore.getState().pushToast({ kind: "info", title, message }),
  success: (title: string, message?: string) => useUiStore.getState().pushToast({ kind: "success", title, message }),
  warning: (title: string, message?: string) => useUiStore.getState().pushToast({ kind: "warning", title, message }),
  error: (title: string, message?: string) => useUiStore.getState().pushToast({ kind: "error", title, message }, 9000),
};
