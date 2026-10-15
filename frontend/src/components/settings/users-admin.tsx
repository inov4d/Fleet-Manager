"use client";
import { useState } from "react";
import { KeyRound, Pencil, Plus, Trash2 } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { ROLE_LABELS } from "@/lib/auth";
import type { Role, User, UserCreate } from "@/lib/types";
import { fmtDateTime } from "@/lib/format";
import { useApi } from "@/hooks/use-api";
import { useAuthStore } from "@/stores/auth";
import { useSiteStore } from "@/stores/site";
import { toast } from "@/stores/ui";
import { Card, CardHeader, EmptyState, ErrorBox } from "@/components/ui/card";
import { Button, IconButton } from "@/components/ui/button";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";

const ROLES = Object.keys(ROLE_LABELS) as Role[];

export function UsersAdmin() {
  const q = useApi(() => api.users.list(), []);
  const sites = useSiteStore((s) => s.sites);
  const me = useAuthStore((s) => s.user);
  const [editing, setEditing] = useState<{ id: string | null; draft: UserCreate } | null>(null);
  const [pwd, setPwd] = useState<{ user: User; password: string } | null>(null);
  const [toDelete, setToDelete] = useState<User | null>(null);
  const [busy, setBusy] = useState(false);

  const wrap = async (fn: () => Promise<void>, ok: string) => {
    setBusy(true);
    try {
      await fn();
      toast.success(ok);
      void q.refetch();
    } catch (e) {
      toast.error("Action refusée", errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const save = () =>
    wrap(async () => {
      if (!editing) return;
      const d = editing.draft;
      if (editing.id) await api.users.update(editing.id, { username: d.username, displayName: d.displayName, role: d.role, siteIds: d.siteIds, active: d.active });
      else await api.users.create(d);
      setEditing(null);
    }, "Utilisateur enregistré");

  const set = (patch: Partial<UserCreate>) => setEditing((e) => (e ? { ...e, draft: { ...e.draft, ...patch } } : e));

  return (
    <Card>
      <CardHeader
        title="Utilisateurs"
        actions={
          <Button size="sm" variant="primary" onClick={() => setEditing({ id: null, draft: { username: "", displayName: "", role: "viewer", siteIds: [], password: "", active: true } })}>
            <Plus className="size-3.5" /> Nouvel utilisateur
          </Button>
        }
      />
      {q.error && <ErrorBox message={errorMessage(q.error)} />}
      {(q.data ?? []).length === 0 ? (
        <EmptyState>Aucun utilisateur.</EmptyState>
      ) : (
        <Table>
          <THead><tr><Th>Identifiant</Th><Th>Nom affiché</Th><Th>Rôle</Th><Th>Sites</Th><Th>Actif</Th><Th>Créé</Th><Th className="text-right">Actions</Th></tr></THead>
          <TBody>
            {(q.data ?? []).map((u) => (
              <Tr key={u.id}>
                <Td className="font-medium">{u.username}{u.id === me?.id && <span className="text-fg-subtle"> (vous)</span>}</Td>
                <Td>{u.displayName}</Td>
                <Td><Badge tone={u.role === "admin" ? "danger" : u.role === "supervisor" ? "warning" : "neutral"}>{ROLE_LABELS[u.role] ?? u.role}</Badge></Td>
                <Td className="text-fg-muted">{u.siteIds?.length ? u.siteIds.join(", ") : "tous"}</Td>
                <Td>{u.active ? <Badge tone="success">oui</Badge> : <Badge tone="muted">non</Badge>}</Td>
                <Td className="text-fg-muted">{fmtDateTime(u.createdAt)}</Td>
                <Td className="text-right whitespace-nowrap">
                  <IconButton label="Modifier" size="xs" onClick={() => setEditing({ id: u.id, draft: { username: u.username, displayName: u.displayName, role: u.role, siteIds: u.siteIds ?? [], active: u.active } })}><Pencil className="size-3.5" /></IconButton>
                  <IconButton label="Réinitialiser le mot de passe" size="xs" onClick={() => setPwd({ user: u, password: "" })}><KeyRound className="size-3.5" /></IconButton>
                  <IconButton label="Supprimer" size="xs" className="text-danger" onClick={() => setToDelete(u)} disabled={u.id === me?.id}><Trash2 className="size-3.5" /></IconButton>
                </Td>
              </Tr>
            ))}
          </TBody>
        </Table>
      )}

      <Dialog
        open={!!editing}
        onClose={() => setEditing(null)}
        title={editing?.id ? "Modifier l’utilisateur" : "Nouvel utilisateur"}
        size="sm"
        footer={<><Button variant="ghost" onClick={() => setEditing(null)}>Annuler</Button><Button variant="primary" onClick={save} loading={busy} disabled={!editing?.draft.username.trim() || (!editing?.id && !editing?.draft.password)}>Enregistrer</Button></>}
      >
        {editing && (
          <div className="flex flex-col gap-3">
            <Field label="Identifiant" htmlFor="u-name" required><Input id="u-name" value={editing.draft.username} onChange={(e) => set({ username: e.target.value })} autoComplete="off" /></Field>
            <Field label="Nom affiché" htmlFor="u-disp"><Input id="u-disp" value={editing.draft.displayName} onChange={(e) => set({ displayName: e.target.value })} /></Field>
            <Field label="Rôle" htmlFor="u-role"><Select id="u-role" value={editing.draft.role} onChange={(e) => set({ role: e.target.value as Role })} options={ROLES.map((r) => ({ value: r, label: ROLE_LABELS[r] }))} /></Field>
            {!editing.id && <Field label="Mot de passe" htmlFor="u-pwd" required><Input id="u-pwd" type="password" value={editing.draft.password ?? ""} onChange={(e) => set({ password: e.target.value })} autoComplete="new-password" /></Field>}
            <fieldset>
              <legend className="text-xs font-medium text-fg-muted mb-1">Sites autorisés (aucun = tous)</legend>
              <div className="flex flex-col gap-1">
                {sites.map((s) => (
                  <Checkbox key={s.id} label={s.name} checked={editing.draft.siteIds.includes(s.id)} onChange={(e) => set({ siteIds: e.target.checked ? [...editing.draft.siteIds, s.id] : editing.draft.siteIds.filter((x) => x !== s.id) })} />
                ))}
              </div>
            </fieldset>
            <Checkbox label="Compte actif" checked={editing.draft.active ?? true} onChange={(e) => set({ active: e.target.checked })} />
          </div>
        )}
      </Dialog>

      <Dialog
        open={!!pwd}
        onClose={() => setPwd(null)}
        title={`Nouveau mot de passe — ${pwd?.user.username ?? ""}`}
        size="sm"
        footer={<><Button variant="ghost" onClick={() => setPwd(null)}>Annuler</Button><Button variant="primary" loading={busy} disabled={!pwd || pwd.password.length < 4} onClick={() => wrap(async () => { if (pwd) await api.users.setPassword(pwd.user.id, pwd.password); setPwd(null); }, "Mot de passe modifié")}>Appliquer</Button></>}
      >
        {pwd && <Field label="Mot de passe" htmlFor="p-new"><Input id="p-new" type="password" value={pwd.password} onChange={(e) => setPwd({ ...pwd, password: e.target.value })} autoComplete="new-password" autoFocus /></Field>}
      </Dialog>

      <ConfirmDialog open={!!toDelete} onClose={() => setToDelete(null)} onConfirm={() => wrap(async () => { if (toDelete) await api.users.remove(toDelete.id); setToDelete(null); }, "Utilisateur supprimé")} title="Supprimer l’utilisateur" message={`Supprimer l’utilisateur « ${toDelete?.username} » ?`} confirmLabel="Supprimer" danger loading={busy} />
    </Card>
  );
}
