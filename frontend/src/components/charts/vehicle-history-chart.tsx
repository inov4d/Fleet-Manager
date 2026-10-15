"use client";
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { TelemetrySample } from "@/lib/types";
import { fmtTime } from "@/lib/format";
import { axisProps, chartColors, tooltipStyle } from "@/components/charts/chart-theme";
import { EmptyState } from "@/components/ui/card";

/** Two single-axis charts (battery %, speed m/s) — one measure per axis. */
export function VehicleHistoryChart({ samples }: { samples: TelemetrySample[] }) {
  if (samples.length === 0) return <EmptyState>Aucun échantillon sur la période.</EmptyState>;
  const data = samples.map((s) => ({ t: new Date(s.ts).getTime(), battery: s.batteryCharge, speed: s.speed }));
  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
      <figure>
        <figcaption className="text-xs text-fg-muted mb-1">Batterie (%)</figcaption>
        <div className="h-48">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
              <CartesianGrid stroke={chartColors.grid} vertical={false} />
              <XAxis dataKey="t" type="number" domain={["dataMin", "dataMax"]} tickFormatter={(v: number) => fmtTime(new Date(v).toISOString())} {...axisProps} />
              <YAxis domain={[0, 100]} width={36} {...axisProps} />
              <Tooltip {...tooltipStyle} labelFormatter={(v) => fmtTime(new Date(Number(v)).toISOString())} formatter={(v) => [`${Number(v).toFixed(0)} %`, "Batterie"]} />
              <Line type="monotone" dataKey="battery" stroke={chartColors.c2} strokeWidth={2} dot={false} isAnimationActive={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </figure>
      <figure>
        <figcaption className="text-xs text-fg-muted mb-1">Vitesse (m/s)</figcaption>
        <div className="h-48">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={data} margin={{ top: 8, right: 12, left: 0, bottom: 0 }}>
              <CartesianGrid stroke={chartColors.grid} vertical={false} />
              <XAxis dataKey="t" type="number" domain={["dataMin", "dataMax"]} tickFormatter={(v: number) => fmtTime(new Date(v).toISOString())} {...axisProps} />
              <YAxis width={36} {...axisProps} />
              <Tooltip {...tooltipStyle} labelFormatter={(v) => fmtTime(new Date(Number(v)).toISOString())} formatter={(v) => [`${Number(v).toFixed(2)} m/s`, "Vitesse"]} />
              <Line type="monotone" dataKey="speed" stroke={chartColors.c1} strokeWidth={2} dot={false} isAnimationActive={false} />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </figure>
    </div>
  );
}
