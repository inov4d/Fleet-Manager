"use client";
import { Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { TimeseriesPoint } from "@/lib/types";
import { fmtDateTime, fmtTime } from "@/lib/format";
import { axisProps, tooltipStyle } from "@/components/charts/chart-theme";
import { EmptyState, Spinner } from "@/components/ui/card";

export function TimeseriesChart({
  title,
  unit,
  data,
  color,
  kind = "line",
  loading,
  domain,
}: {
  title: string;
  unit?: string;
  data: TimeseriesPoint[] | undefined;
  color: string;
  kind?: "line" | "bar";
  loading?: boolean;
  domain?: [number, number];
}) {
  const points = (data ?? []).map((p) => ({ t: new Date(p.ts).getTime(), value: p.value }));
  const spanMs = points.length > 1 ? points[points.length - 1].t - points[0].t : 0;
  const tickFmt = (v: number) => (spanMs > 36 * 3600_000 ? fmtDateTime(new Date(v).toISOString()).slice(0, 5) : fmtTime(new Date(v).toISOString()).slice(0, 5));
  const fmtVal = (v: unknown) => [`${Number(v).toLocaleString("fr-FR", { maximumFractionDigits: 1 })}${unit ? ` ${unit}` : ""}`, title] as [string, string];
  return (
    <figure className="rounded-lg border border-border bg-surface p-3">
      <figcaption className="text-xs font-medium mb-1">
        {title} {unit && <span className="text-fg-muted font-normal">({unit})</span>}
      </figcaption>
      <div className="h-52">
        {loading && !data ? (
          <Spinner className="h-full" />
        ) : points.length === 0 ? (
          <EmptyState className="h-full flex items-center justify-center">Aucune donnée.</EmptyState>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            {kind === "bar" ? (
              <BarChart data={points} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                <CartesianGrid stroke="var(--chart-grid)" vertical={false} />
                <XAxis dataKey="t" tickFormatter={tickFmt} {...axisProps} minTickGap={24} />
                <YAxis width={40} {...axisProps} domain={domain} allowDecimals={false} />
                <Tooltip {...tooltipStyle} cursor={{ fill: "var(--surface-3)" }} labelFormatter={(v) => fmtDateTime(new Date(Number(v)).toISOString())} formatter={fmtVal} />
                <Bar dataKey="value" fill={color} radius={[4, 4, 0, 0]} isAnimationActive={false} />
              </BarChart>
            ) : (
              <LineChart data={points} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                <CartesianGrid stroke="var(--chart-grid)" vertical={false} />
                <XAxis dataKey="t" type="number" domain={["dataMin", "dataMax"]} tickFormatter={tickFmt} {...axisProps} minTickGap={24} />
                <YAxis width={40} {...axisProps} domain={domain} />
                <Tooltip {...tooltipStyle} labelFormatter={(v) => fmtDateTime(new Date(Number(v)).toISOString())} formatter={fmtVal} />
                <Line type="monotone" dataKey="value" stroke={color} strokeWidth={2} dot={false} activeDot={{ r: 4 }} isAnimationActive={false} />
              </LineChart>
            )}
          </ResponsiveContainer>
        )}
      </div>
    </figure>
  );
}
