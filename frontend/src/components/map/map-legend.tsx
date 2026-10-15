import { AVAILABILITY_COLORS, AVAILABILITY_LABELS, NODE_TYPE_COLORS, NODE_TYPE_LABELS } from "@/lib/labels";
import { hexToCss } from "@/lib/utils";
import type { Availability, NodeType } from "@/lib/types";

export function MapLegend() {
  return (
    <div className="flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-fg-muted">
      {(Object.keys(AVAILABILITY_COLORS) as Availability[]).map((k) => (
        <span key={k} className="inline-flex items-center gap-1">
          <span className="inline-block size-2.5 rounded-sm" style={{ background: hexToCss(AVAILABILITY_COLORS[k]) }} aria-hidden />
          {AVAILABILITY_LABELS[k]}
        </span>
      ))}
      <span className="border-l border-border mx-1" aria-hidden />
      {(Object.keys(NODE_TYPE_COLORS) as NodeType[]).map((k) => (
        <span key={k} className="inline-flex items-center gap-1">
          <span className="inline-block size-2.5 rounded-full" style={{ background: hexToCss(NODE_TYPE_COLORS[k]) }} aria-hidden />
          {NODE_TYPE_LABELS[k]}
        </span>
      ))}
      <span className="inline-flex items-center gap-1">
        <span className="inline-block size-2.5 rounded-full border-2" style={{ borderColor: "#f97316" }} aria-hidden />
        Réservation incertaine
      </span>
    </div>
  );
}
