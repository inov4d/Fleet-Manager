import type { HTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Card({ className, children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("rounded-lg border border-border bg-surface", className)} {...rest}>
      {children}
    </div>
  );
}

export function CardHeader({
  title,
  actions,
  className,
  subtitle,
}: {
  title: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex items-center justify-between gap-2 border-b border-border px-3 py-2", className)}>
      <div className="min-w-0">
        <h2 className="text-[13px] font-semibold truncate">{title}</h2>
        {subtitle && <p className="text-[11px] text-fg-muted truncate">{subtitle}</p>}
      </div>
      {actions && <div className="flex items-center gap-1 shrink-0">{actions}</div>}
    </div>
  );
}

export function CardBody({ className, children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("p-3", className)} {...rest}>
      {children}
    </div>
  );
}

export function PageHeader({
  title,
  subtitle,
  actions,
}: {
  title: string;
  subtitle?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-2 mb-3">
      <div>
        <h1 className="text-lg font-semibold leading-tight">{title}</h1>
        {subtitle && <p className="text-xs text-fg-muted">{subtitle}</p>}
      </div>
      {actions && <div className="flex items-center gap-2 flex-wrap">{actions}</div>}
    </div>
  );
}

export function KpiTile({
  label,
  value,
  unit,
  hint,
  tone = "neutral",
  icon,
}: {
  label: string;
  value: ReactNode;
  unit?: string;
  hint?: ReactNode;
  tone?: "neutral" | "success" | "warning" | "danger" | "accent";
  icon?: ReactNode;
}) {
  const toneCls =
    tone === "success"
      ? "text-success"
      : tone === "warning"
        ? "text-warning"
        : tone === "danger"
          ? "text-danger"
          : tone === "accent"
            ? "text-accent"
            : "text-fg";
  return (
    <Card className="p-3 flex flex-col gap-1 min-w-0">
      <div className="flex items-center justify-between gap-2 text-[11px] uppercase tracking-wide text-fg-muted">
        <span className="truncate">{label}</span>
        {icon && <span className="text-fg-subtle shrink-0">{icon}</span>}
      </div>
      <div className={cn("text-2xl font-semibold tabular leading-none", toneCls)}>
        {value}
        {unit && <span className="text-sm font-normal text-fg-muted ml-1">{unit}</span>}
      </div>
      {hint && <div className="text-[11px] text-fg-subtle truncate">{hint}</div>}
    </Card>
  );
}

export function ProgressBar({
  value,
  tone = "accent",
  className,
  label,
}: {
  value: number;
  tone?: "accent" | "success" | "warning" | "danger";
  className?: string;
  label?: string;
}) {
  const pct = Math.max(0, Math.min(100, value));
  const color =
    tone === "success" ? "bg-success" : tone === "warning" ? "bg-warning" : tone === "danger" ? "bg-danger" : "bg-accent";
  return (
    <div
      className={cn("h-1.5 w-full rounded bg-surface-3 overflow-hidden", className)}
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(pct)}
      aria-label={label}
    >
      <div className={cn("h-full rounded", color)} style={{ width: `${pct}%` }} />
    </div>
  );
}

export function BatteryBar({ value, charging }: { value: number; charging?: boolean }) {
  const tone = value < 15 ? "danger" : value < 30 ? "warning" : "success";
  return (
    <div className="flex items-center gap-2 min-w-24">
      <ProgressBar value={value} tone={tone} className="w-16" label="Batterie" />
      <span className="tabular text-xs w-10">
        {Math.round(value)} %{charging ? "⚡" : ""}
      </span>
    </div>
  );
}

export function EmptyState({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("py-8 text-center text-sm text-fg-muted", className)}>{children}</div>;
}

export function Spinner({ className }: { className?: string }) {
  return (
    <div className={cn("flex items-center justify-center py-8 text-fg-muted text-sm", className)} role="status">
      <span className="size-4 rounded-full border-2 border-border-strong border-t-accent animate-spin mr-2" aria-hidden />
      Chargement…
    </div>
  );
}

export function ErrorBox({ message }: { message: string }) {
  return (
    <div className="rounded-md border border-danger/40 bg-danger-soft text-danger text-xs px-3 py-2" role="alert">
      {message}
    </div>
  );
}

export function DescriptionList({ items, className }: { items: Array<[string, ReactNode]>; className?: string }) {
  return (
    <dl className={cn("grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs", className)}>
      {items.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-fg-muted whitespace-nowrap">{k}</dt>
          <dd className="min-w-0 truncate tabular">{v}</dd>
        </div>
      ))}
    </dl>
  );
}
