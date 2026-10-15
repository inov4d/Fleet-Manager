"use client";
import { useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import type { Site } from "@/lib/types";
import { fmtDateTime } from "@/lib/format";
import { useApi } from "@/hooks/use-api";
import { useSiteStore } from "@/stores/site";
import { toast } from "@/stores/ui";
import { Card, CardHeader, EmptyState, ErrorBox } from "@/components/ui/card";
import { Button, IconButton } from "@/components/ui/button";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";

type Draft = { id: string; name: string; description: string; timezone: string };

export function SitesAdmin() {
  const q = useApi(() => api.sites.list(), []);
  const setSites = useSiteStore((s) => s.setSites);
  const [editing, setEditing] = useState<{ isNew: boolean; draft: Draft } | null>(null);
  const [toDelete, setToDelete] = useState<Site | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = async () => {
    await q.refetch();
    try {
      setSites(await api.sites.list());
    } catch {
      /* ignore */
    }
  };

  const save = async () => {
    if (!editing) return;
    setBusy(true);
    try {
      const d = editing.draft;
      if (editing.isNew) await api.sites.create({ ...(d.id ? { id: d.id } : {}), name: d.name, description: d.description, timezone: d.timezone });
      else await api.sites.update(d.id, { name: d.name, description: d.description, timezone: d.timezone });
      toast.success("Site enregistré", d.name);
      setEditing(null);
      await refresh();
    } catch (e) {
      toast.error("Enregistrement refusé", errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!toDelete) return;
    setBusy(true);
    try {
      await api.sites.remove(toDelete.id);
      toast.success("Site supprimé", toDelete.name);
      setToDelete(null);
      await refresh();
    } catch (e) {
      toast.error("Suppression refusée", errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const set = (patch: Partial<Draft>) => setEditing((e) => (e ? { ...e, draft: { ...e.draft, ...patch } } : e));

  return (
    <Card>
      <CardHeader
        title="Sites"
        actions={
          <Button size="sm" variant="primary" onClick={() => setEditing({ isNew: true, draft: { id: "", name: "", description: "", timezone: "Europe/Paris" } })}>
            <Plus className="size-3.5" /> Nouveau site
          </Button>
        }
      />
      {q.error && <ErrorBox message={errorMessage(q.error)} />}
      {(q.data ?? []).length === 0 ? (
        <EmptyState>Aucun site.</EmptyState>
      ) : (
        <Table>
          <THead><tr><Th>ID</Th><Th>Nom</Th><Th>Description</Th><Th>Fuseau</Th><Th>Créé</Th><Th className="text-right">Actions</Th></tr></THead>
          <TBody>
            {(q.data ?? []).map((s) => (
              <Tr key={s.id}>
                <Td className="font-mono text-[11px]">{s.id}</Td>
                <Td className="font-medium">{s.name}</Td>
                <Td className="text-fg-muted">{s.description}</Td>
                <Td>{s.timezone}</Td>
                <Td className="text-fg-muted">{fmtDateTime(s.createdAt)}</Td>
                <Td className="text-right whitespace-nowrap">
                  <IconButton label="Modifier" size="xs" onClick={() => setEditing({ isNew: false, draft: { id: s.id, name: s.name, description: s.description, timezone: s.timezone } })}><Pencil className="size-3.5" /></IconButton>
                  <IconButton label="Supprimer" size="xs" className="text-danger" onClick={() => setToDelete(s)} disabled={s.id === "site-default"}><Trash2 className="size-3.5" /></IconButton>
                </Td>
              </Tr>
            ))}
          </TBody>
        </Table>
      )}
      <Dialog
        open={!!editing}
        onClose={() => setEditing(null)}
        title={editing?.isNew ? "Nouveau site" : "Modifier le site"}
        size="sm"
        footer={<><Button variant="ghost" onClick={() => setEditing(null)}>Annuler</Button><Button variant="primary" onClick={save} loading={busy} disabled={!editing?.draft.name.trim()}>Enregistrer</Button></>}
      >
        {editing && (
          <div className="flex flex-col gap-3">
            {editing.isNew && <Field label="Identifiant" htmlFor="s-id" hint="Optionnel."><Input id="s-id" value={editing.draft.id} onChange={(e) => set({ id: e.target.value })} /></Field>}
            <Field label="Nom" htmlFor="s-name" required><Input id="s-name" value={editing.draft.name} onChange={(e) => set({ name: e.target.value })} /></Field>
            <Field label="Description" htmlFor="s-desc"><Input id="s-desc" value={editing.draft.description} onChange={(e) => set({ description: e.target.value })} /></Field>
            <Field label="Fuseau horaire" htmlFor="s-tz"><Input id="s-tz" value={editing.draft.timezone} onChange={(e) => set({ timezone: e.target.value })} placeholder="Europe/Paris" /></Field>
          </div>
        )}
      </Dialog>
      <ConfirmDialog open={!!toDelete} onClose={() => setToDelete(null)} onConfirm={remove} title="Supprimer le site" message={`Supprimer le site « ${toDelete?.name} » ?`} confirmLabel="Supprimer" danger loading={busy} />
    </Card>
  );
}
