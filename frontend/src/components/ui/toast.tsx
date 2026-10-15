"use client";
import Link from "next/link";
import { AlertTriangle, CheckCircle2, Info, X, XCircle } from "lucide-react";
import { useUiStore, type ToastKind } from "@/stores/ui";
import { cn } from "@/lib/utils";

const ICONS: Record<ToastKind, React.ReactNode> = {
  info: <Info className="size-4 text-info" aria-hidden />,
  success: <CheckCircle2 className="size-4 text-success" aria-hidden />,
  warning: <AlertTriangle className="size-4 text-warning" aria-hidden />,
  error: <XCircle className="size-4 text-danger" aria-hidden />,
};

const BORDERS: Record<ToastKind, string> = {
  info: "border-info/40",
  success: "border-success/40",
  warning: "border-warning/50",
  error: "border-danger/50",
};

export function ToastViewport() {
  const toasts = useUiStore((s) => s.toasts);
  const dismiss = useUiStore((s) => s.dismissToast);
  return (
    <div className="pointer-events-none fixed bottom-3 right-3 z-50 flex w-80 flex-col gap-2" aria-live="polite" aria-atomic="false">
      {toasts.map((t) => (
        <div
          key={t.id}
          role={t.kind === "error" || t.kind === "warning" ? "alert" : "status"}
          className={cn(
            "pointer-events-auto fm-fade-in flex items-start gap-2 rounded-md border bg-surface px-3 py-2 shadow-lg",
            BORDERS[t.kind],
          )}
        >
          <div className="mt-0.5 shrink-0">{ICONS[t.kind]}</div>
          <div className="min-w-0 flex-1">
            <div className="text-xs font-semibold">{t.title}</div>
            {t.message && <div className="text-[11px] text-fg-muted break-words">{t.message}</div>}
            {t.href && (
              <Link href={t.href} className="text-[11px] text-accent hover:underline">
                Voir
              </Link>
            )}
          </div>
          <button
            type="button"
            onClick={() => dismiss(t.id)}
            aria-label="Fermer la notification"
            className="text-fg-subtle hover:text-fg"
          >
            <X className="size-3.5" />
          </button>
        </div>
      ))}
    </div>
  );
}
