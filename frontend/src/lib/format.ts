const dtf = new Intl.DateTimeFormat("fr-FR", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
});
const tf = new Intl.DateTimeFormat("fr-FR", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
const nf1 = new Intl.NumberFormat("fr-FR", { maximumFractionDigits: 1 });
const nf0 = new Intl.NumberFormat("fr-FR", { maximumFractionDigits: 0 });
const nf2 = new Intl.NumberFormat("fr-FR", { maximumFractionDigits: 2 });

export function fmtDateTime(ts: string | null | undefined): string {
  if (!ts) return "—";
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return "—";
  return dtf.format(d);
}

export function fmtTime(ts: string | null | undefined): string {
  if (!ts) return "—";
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return "—";
  return tf.format(d);
}

export function fmtRelative(ts: string | null | undefined, now = Date.now()): string {
  if (!ts) return "—";
  const d = new Date(ts).getTime();
  if (Number.isNaN(d)) return "—";
  const diff = Math.round((now - d) / 1000);
  if (diff < 0) return "dans " + fmtDuration(-diff);
  if (diff < 5) return "à l’instant";
  return "il y a " + fmtDuration(diff);
}

export function fmtDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds)) return "—";
  const s = Math.round(seconds);
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ${s % 60 ? `${s % 60} s` : ""}`.trim();
  const h = Math.floor(m / 60);
  if (h < 48) return `${h} h ${m % 60 ? `${m % 60} min` : ""}`.trim();
  const d = Math.floor(h / 24);
  return `${d} j ${h % 24 ? `${h % 24} h` : ""}`.trim();
}

export function fmtNum(v: number | null | undefined, digits: 0 | 1 | 2 = 1): string {
  if (v === null || v === undefined || !Number.isFinite(v)) return "—";
  return (digits === 0 ? nf0 : digits === 1 ? nf1 : nf2).format(v);
}

export function fmtPct(v: number | null | undefined, digits: 0 | 1 = 0): string {
  if (v === null || v === undefined || !Number.isFinite(v)) return "—";
  return `${fmtNum(v, digits)} %`;
}

export function fmtDistance(m: number | null | undefined): string {
  if (m === null || m === undefined || !Number.isFinite(m)) return "—";
  if (Math.abs(m) >= 1000) return `${fmtNum(m / 1000, 2)} km`;
  return `${fmtNum(m, 1)} m`;
}

export function fmtCoord(v: number | null | undefined): string {
  if (v === null || v === undefined || !Number.isFinite(v)) return "—";
  return nf2.format(v);
}

export function fmtDeg(rad: number | null | undefined): string {
  if (rad === null || rad === undefined || !Number.isFinite(rad)) return "—";
  return `${fmtNum((rad * 180) / Math.PI, 0)}°`;
}

/** Local datetime-local input value (YYYY-MM-DDTHH:mm) from a Date. */
export function toLocalInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fromLocalInput(v: string): string | null {
  if (!v) return null;
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}
