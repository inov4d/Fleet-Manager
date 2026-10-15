"use client";
import { useRef, useState } from "react";
import { CheckCircle2, Copy, Download, FilePlus2, Trash2, Upload } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import type { FleetMapSummary } from "@/lib/types";
import { fmtDateTime } from "@/lib/format";
import { downloadJson } from "@/lib/utils";
import { toast } from "@/stores/ui";
import { Badge } from "@/components/ui/badge";
import { Button, IconButton } from "@/components/ui/button";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { emptyMap } from "@/components/editor/editor-types";

export function MapsList({
  maps,
  siteId,
  currentId,
  canManage,
  onOpen,
  onChanged,
}: {
  maps: FleetMapSummary[];
  siteId: string;
  currentId: string | null;
  canManage: boolean;
  onOpen: (id: string) => void;
  onChanged: () => void;
}) {
  const [newOpen, setNewOpen] = useState(false);
  const [newName, setNewName] = useState("");
  const [toDelete, setToDelete] = useState<FleetMapSummary | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const guard = async (key: string, fn: () => Promise<void>, okMsg: string) => {
    setBusy(key);
    try {
      await fn();
      toast.success(okMsg);
      onChanged();
    } catch (e) {
      toast.error("Échec", errorMessage(e));
    } finally {
      setBusy(null);
    }
  };

  const create = () =>
    guard(
      "new",
      async () => {
        const m = await api.maps.create(emptyMap(siteId, newName.trim() || "Nouvelle carte"));
        setNewOpen(false);
        setNewName("");
        onOpen(m.id);
      },
      "Carte créée",
    );

  const duplicate = (id: string) =>
    guard(
      `dup-${id}`,
      async () => {
        const full = await api.maps.get(id);
        const copy = await api.maps.create({ ...full, id: undefined, name: `${full.name} (copie)`, active: false, version: 0 });
        onOpen(copy.id);
      },
      "Carte dupliquée",
    );

  const exportMap = (m: FleetMapSummary) =>
    guard(
      `exp-${m.id}`,
      async () => {
        const data = await api.maps.exportMap(m.id);
        downloadJson(`${m.name.replace(/[^\w-]+/g, "_")}-v${m.version}.json`, data);
      },
      "Carte exportée",
    );

  const importFile = async (file: File) => {
    const text = await file.text();
    await guard(
      "import",
      async () => {
        const m = await api.maps.importMap(text);
        onOpen(m.id);
      },
      "Carte importée",
    );
  };

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between gap-1 px-2 py-1.5 border-b border-border">
        <span className="text-xs font-semibold">Cartes du site</span>
        {canManage && (
          <span className="flex gap-0.5">
            <IconButton label="Nouvelle carte" size="xs" onClick={() => setNewOpen(true)}><FilePlus2 className="size-3.5" /></IconButton>
            <IconButton label="Importer une carte (JSON)" size="xs" onClick={() => fileRef.current?.click()} loading={busy === "import"}><Upload className="size-3.5" /></IconButton>
            <input
              ref={fileRef}
              type="file"
              accept="application/json,.json"
              className="hidden"
              aria-hidden
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) void importFile(f);
                e.target.value = "";
              }}
            />
          </span>
        )}
      </div>
      <ul className="flex-1 overflow-y-auto divide-y divide-border">
        {maps.length === 0 && <li className="p-3 text-[11px] text-fg-subtle">Aucune carte pour ce site.</li>}
        {maps.map((m) => (
          <li key={m.id} className={cn("px-2 py-1.5 text-xs", currentId === m.id && "bg-accent-soft/50")}>
            <button type="button" onClick={() => onOpen(m.id)} className="w-full text-left" aria-current={currentId === m.id ? "true" : undefined}>
              <div className="flex items-center justify-between gap-1">
                <span className="font-medium truncate">{m.name}</span>
                {m.active && <Badge tone="success">active</Badge>}
              </div>
              <div className="text-[10px] text-fg-subtle">v{m.version} · {fmtDateTime(m.updatedAt)}</div>
            </button>
            {canManage && (
              <div className="flex gap-0.5 mt-1">
                <IconButton label="Activer" size="xs" disabled={m.active} loading={busy === `act-${m.id}`} onClick={() => guard(`act-${m.id}`, async () => { await api.maps.activate(m.id); }, "Carte activée")}>
                  <CheckCircle2 className="size-3.5" />
                </IconButton>
                <IconButton label="Dupliquer" size="xs" loading={busy === `dup-${m.id}`} onClick={() => duplicate(m.id)}><Copy className="size-3.5" /></IconButton>
                <IconButton label="Exporter" size="xs" loading={busy === `exp-${m.id}`} onClick={() => exportMap(m)}><Download className="size-3.5" /></IconButton>
                <IconButton label="Supprimer" size="xs" disabled={m.active} onClick={() => setToDelete(m)} className="text-danger"><Trash2 className="size-3.5" /></IconButton>
              </div>
            )}
          </li>
        ))}
      </ul>
      <Dialog
        open={newOpen}
        onClose={() => setNewOpen(false)}
        title="Nouvelle carte"
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setNewOpen(false)}>Annuler</Button>
            <Button variant="primary" onClick={create} loading={busy === "new"}>Créer</Button>
          </>
        }
      >
        <Field label="Nom" htmlFor="nm-name"><Input id="nm-name" value={newName} onChange={(e) => setNewName(e.target.value)} autoFocus /></Field>
      </Dialog>
      <ConfirmDialog
        open={!!toDelete}
        onClose={() => setToDelete(null)}
        title="Supprimer la carte"
        message={`Supprimer définitivement la carte « ${toDelete?.name} » ?`}
        confirmLabel="Supprimer"
        danger
        loading={busy === "del"}
        onConfirm={() => {
          const m = toDelete;
          if (!m) return;
          void guard("del", async () => { await api.maps.remove(m.id); setToDelete(null); }, "Carte supprimée");
        }}
      />
    </div>
  );
}
