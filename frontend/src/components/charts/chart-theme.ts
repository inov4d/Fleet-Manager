/* Shared recharts styling (reads CSS tokens so charts follow the theme). */
export const chartColors = {
  c1: "var(--chart-1)",
  c2: "var(--chart-2)",
  c3: "var(--chart-3)",
  c4: "var(--chart-4)",
  c5: "var(--chart-5)",
  grid: "var(--chart-grid)",
  text: "var(--fg-muted)",
};

export const axisProps = {
  tick: { fontSize: 11, fill: "var(--fg-muted)" },
  axisLine: { stroke: "var(--chart-grid)" },
  tickLine: false as const,
};

export const tooltipStyle = {
  contentStyle: {
    background: "var(--surface)",
    border: "1px solid var(--border)",
    borderRadius: 6,
    fontSize: 12,
    color: "var(--fg)",
  },
  labelStyle: { color: "var(--fg-muted)", fontSize: 11 },
  itemStyle: { color: "var(--fg)" },
};
