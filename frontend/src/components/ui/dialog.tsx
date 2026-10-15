"use client";
import { useEffect, useRef, type ReactNode } from "react";
import { X } from "lucide-react";
import { Button, IconButton } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export interface DialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  footer?: ReactNode;
  size?: "sm" | "md" | "lg";
}

const SIZES = { sm: "max-w-sm", md: "max-w-lg", lg: "max-w-3xl" };

/** Accessible modal built on the native <dialog> element. */
export function Dialog({ open, onClose, title, children, footer, size = "md" }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    else if (!open && el.open) el.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === ref.current) onClose();
      }}
      className={cn(
        "m-auto w-[calc(100%-2rem)] rounded-lg border border-border bg-surface text-fg p-0 shadow-2xl backdrop:bg-black/60",
        SIZES[size],
      )}
      aria-labelledby="fm-dialog-title"
    >
      {open && (
        <div className="flex flex-col max-h-[85vh]">
          <div className="flex items-center justify-between gap-2 border-b border-border px-4 py-2.5">
            <h2 id="fm-dialog-title" className="text-sm font-semibold">
              {title}
            </h2>
            <IconButton label="Fermer" onClick={onClose} size="xs">
              <X className="size-4" />
            </IconButton>
          </div>
          <div className="px-4 py-3 overflow-y-auto">{children}</div>
          {footer && <div className="flex justify-end gap-2 border-t border-border px-4 py-2.5">{footer}</div>}
        </div>
      )}
    </dialog>
  );
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  message,
  confirmLabel = "Confirmer",
  danger,
  loading,
  children,
}: {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  danger?: boolean;
  loading?: boolean;
  children?: ReactNode;
}) {
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={title}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Annuler
          </Button>
          <Button variant={danger ? "danger" : "primary"} onClick={onConfirm} loading={loading}>
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div className="text-sm text-fg-muted">{message}</div>
      {children}
    </Dialog>
  );
}
