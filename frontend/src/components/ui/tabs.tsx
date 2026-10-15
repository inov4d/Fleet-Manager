"use client";
import { useId, type KeyboardEvent, type ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface TabItem<K extends string = string> {
  key: K;
  label: ReactNode;
  disabled?: boolean;
}

export function Tabs<K extends string>({
  tabs,
  value,
  onChange,
  className,
}: {
  tabs: TabItem<K>[];
  value: K;
  onChange: (k: K) => void;
  className?: string;
}) {
  const id = useId();
  const enabled = tabs.filter((t) => !t.disabled);

  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const idx = enabled.findIndex((t) => t.key === value);
    if (idx < 0) return;
    let next: number | null = null;
    if (e.key === "ArrowRight") next = (idx + 1) % enabled.length;
    if (e.key === "ArrowLeft") next = (idx - 1 + enabled.length) % enabled.length;
    if (e.key === "Home") next = 0;
    if (e.key === "End") next = enabled.length - 1;
    if (next !== null) {
      e.preventDefault();
      onChange(enabled[next].key);
      const el = document.getElementById(`${id}-tab-${enabled[next].key}`);
      el?.focus();
    }
  };

  return (
    <div role="tablist" className={cn("flex items-center gap-1 border-b border-border overflow-x-auto", className)} onKeyDown={onKey}>
      {tabs.map((t) => {
        const active = t.key === value;
        return (
          <button
            key={t.key}
            id={`${id}-tab-${t.key}`}
            role="tab"
            type="button"
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            disabled={t.disabled}
            onClick={() => onChange(t.key)}
            className={cn(
              "px-3 h-8 text-xs font-medium whitespace-nowrap border-b-2 -mb-px transition-colors",
              active ? "border-accent text-fg" : "border-transparent text-fg-muted hover:text-fg",
              t.disabled && "opacity-40 cursor-not-allowed",
            )}
          >
            {t.label}
          </button>
        );
      })}
    </div>
  );
}

/** Filter chips (multi or single) used above tables. */
export function Chips<K extends string>({
  items,
  value,
  onChange,
  className,
}: {
  items: Array<{ key: K; label: ReactNode; count?: number }>;
  value: K[];
  onChange: (v: K[]) => void;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-wrap gap-1", className)} role="group">
      {items.map((it) => {
        const active = value.includes(it.key);
        return (
          <button
            key={it.key}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(active ? value.filter((v) => v !== it.key) : [...value, it.key])}
            className={cn(
              "h-6 rounded-full border px-2 text-[11px] font-medium inline-flex items-center gap-1 transition-colors",
              active ? "bg-accent-soft border-accent text-accent" : "bg-surface-2 border-border text-fg-muted hover:text-fg",
            )}
          >
            {it.label}
            {it.count !== undefined && <span className="tabular opacity-70">{it.count}</span>}
          </button>
        );
      })}
    </div>
  );
}
