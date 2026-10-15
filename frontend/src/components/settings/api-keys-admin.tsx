"use client";
import { useState } from "react";
import { Copy, Plus, Trash2 } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { ROLE_LABELS } from "@/lib/auth";
import type { ApiKey, Role } from "@/lib/types";
import { fmtDateTime } from "@/lib/format";
import { useApi } from "@/hooks/use-api";
import { toast } from "@/stores/ui";
import { Card, CardHeader, EmptyState, ErrorBox } from "@/components/ui/card";
import { Button, IconButton } from "@/components/ui/button";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";

export function ApiKeysAdmin() {
  const q = useApi(() => api.apiKeys.list(), []);
  const [creating, setCreating] = useState<{ name: string; role: Role } | null>(null);
  const [created, setCreated] = useState<ApiKey | null>(null);
  const [toDelete, setToDelete] = useState<ApiKey | null>(null);
  const [busy, setBusy] = useState(false);

  const create = async () => {
    if (!creating) return;
    setBusy(true);
    try {
      const k = await api.apiKeys.create(creating);
      setCreating(null);
      setCreated(k);
      void q.refetch();
    } catch (e) {
      toast.error("Création refusée", errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!toDelete) return;
    setBusy(true);
    try {
      await api.apiKeys.remove(toDelete.id);
      toast.success("Clé révoquée", toDelete.name);
      setToDelete(null);
      void q.refetch();
    } catch (e) {
      toast.error("Révocation refusée", errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const copy = async (s: string) => {
    try {
      await navigator.clipboard.writeText(s);
      toast.success("Clé copiée dans le presse-papiers");
    } catch {
      toast.warning("Copie impossible", "Sélectionnez et copiez la clé manuellement.");
    }
  };

  return (
    <Card>
      <CardHeader
        title="Clés API"
        subtitle="Pour les systèmes externes (en-tête X-API-Key)"
        actions={
          <Button size="sm" variant="primary" onClick={() => setCreating({ name: "", role: "integration" })}>
            <Plus className="size-3.5" /> Nouvelle clé
          </Button>
        }
      />
      {q.error && <ErrorBox message={errorMessage(q.error)} />}
      {(q.data ?? []).length === 0 ? (
        <EmptyState>Aucune clé API.</EmptyState>
      ) : (
        <Table>
          <THead><tr><Th>Nom</Th><Th>Rôle</Th><Th>ID</Th><Th>Créée</Th><Th className="text-right">Actions</Th></tr></THead>
          <TBody>
            {(q.data ?? []).map((k) => (
              <Tr key={k.id}>
                <Td className="font-medium">{k.name}</Td>
                <Td><Badge>{ROLE_LABELS[k.role] ?? k.role}</Badge></Td>
                <Td className="font-mono text-[11px] text-fg-muted">{k.id}</Td>
                <Td className="text-fg-muted">{fmtDateTime(k.createdAt)}</Td>
                <Td className="text-right"><IconButton label="Révoquer" size="xs" className="text-danger" onClick={() => setToDelete(k)}><Trash2 className="size-3.5" /></IconButton></Td>
              </Tr>
            ))}
          </TBody>
        </Table>
      )}

      <Dialog
        open={!!creating}
        onClose={() => setCreating(null)}
        title="Nouvelle clé API"
        size="sm"
        footer={<><Button variant="ghost" onClick={() => setCreating(null)}>Annuler</Button><Button variant="primary" onClick={create} loading={busy} disabled={!creating?.name.trim()}>Créer</Button></>}
      >
        {creating && (
          <div className="flex flex-col gap-3">
            <Field label="Nom" htmlFor="k-name" required><Input id="k-name" value={creating.name} onChange={(e) => setCreating({ ...creating, name: e.target.value })} autoFocus /></Field>
            <Field label="Rôle" htmlFor="k-role"><Select id="k-role" value={creating.role} onChange={(e) => setCreating({ ...creating, role: e.target.value as Role })} options={(Object.keys(ROLE_LABELS) as Role[]).map((r) => ({ value: r, label: ROLE_LABELS[r] }))} /></Field>
          </div>
        )}
      </Dialog>

      <Dialog open={!!created} onClose={() => setCreated(null)} title="Clé créée" size="sm" footer={<Button variant="primary" onClick={() => setCreated(null)}>J’ai copié la clé</Button>}>
        {created && (
          <div className="flex flex-col gap-2">
            <p className="text-xs text-warning">Cette clé n’est affichée qu’une seule fois. Conservez-la en lieu sûr.</p>
            <div className="flex items-center gap-1">
              <code className="flex-1 rounded border border-border bg-bg px-2 py-1.5 text-[11px] break-all select-all">{created.key ?? "(clé non renvoyée par le serveur)"}</code>
              {created.key && <IconButton label="Copier la clé" onClick={() => copy(created.key ?? "")}><Copy className="size-4" /></IconButton>}
            </div>
          </div>
        )}
      </Dialog>

      <ConfirmDialog open={!!toDelete} onClose={() => setToDelete(null)} onConfirm={remove} title="Révoquer la clé" message={`Révoquer la clé « ${toDelete?.name} » ? Les systèmes qui l’utilisent perdront l’accès.`} confirmLabel="Révoquer" danger loading={busy} />
    </Card>
  );
}
