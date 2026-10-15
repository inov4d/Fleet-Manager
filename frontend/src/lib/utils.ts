/** Join class names, ignoring falsy values. */
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ");
}

export function clamp(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v));
}

/** Stable hue (0-360) from a string, used for per-vehicle colors. */
export function hashHue(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return Math.abs(h) % 360;
}

export function hslToHex(h: number, s: number, l: number): number {
  const sat = s / 100;
  const lig = l / 100;
  const k = (n: number) => (n + h / 30) % 12;
  const a = sat * Math.min(lig, 1 - lig);
  const f = (n: number) => lig - a * Math.max(-1, Math.min(k(n) - 3, Math.min(9 - k(n), 1)));
  const r = Math.round(255 * f(0));
  const g = Math.round(255 * f(8));
  const b = Math.round(255 * f(4));
  return (r << 16) | (g << 8) | b;
}

export function hexToCss(hex: number): string {
  return `#${hex.toString(16).padStart(6, "0")}`;
}

export function vehicleHue(vehicleId: string): number {
  return hashHue(vehicleId);
}

export function vehicleColorHex(vehicleId: string): number {
  return hslToHex(vehicleHue(vehicleId), 70, 55);
}

export function vehicleColorCss(vehicleId: string): string {
  return hexToCss(vehicleColorHex(vehicleId));
}

export function uid(prefix: string): string {
  return `${prefix}-${Math.random().toString(36).slice(2, 8)}`;
}

export function downloadJson(filename: string, data: unknown) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
