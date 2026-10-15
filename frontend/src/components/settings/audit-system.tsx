"use client";
import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { fmtDateTime, fmtDuration } from "@/lib/format";
import { useApi, useInterval } from "@/hooks/use-api";
import { Card, CardHeader, CardBody, DescriptionList, EmptyState, ErrorBox, Spinner } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { apiBaseUrl, wsUrl } from "@/lib/api";

export function AuditLog() {
  const [user, setUser] = useState("");
  const [action, setAction] = useState("");
  const [limit, setLimit] = useState(200);
  const q = useApi(() => api.audit.list({ limit, user: user || undefined, action: action || undefined }), [limit, user, action]);
  return (
    <Card>
      <CardHeader
        title="Journal d’audit"
        actions={
          <div className="flex items-center gap-1">
            <Input aria-label="Utilisateur" placeholder="Utilisateur" value={user} onChange={(e) => setUser(e.target.value)} className="h-7 w-36" />
            <Input aria-label="Action" placeholder="Action" value={action} onChange={(e) => setAction(e.target.value)} className="h-7 w-36" />
            <Select aria-label="Limite" value={String(limit)} onChange={(e) => setLimit(Number(e.target.value))} options={[100, 200, 500, 1000].map((n) => ({ value: String(n), label: `${n}` }))} className="h-7 w-20" />
            <Button size="sm" variant="ghost" onClick={() => void q.refetch()} loading={q.loading}><RefreshCw className="size-3.5" /></Button>
          </div>
        }
      />
      {q.error && <ErrorBox message={errorMessage(q.error)} />}
      {q.loading && !q.data ? (
        <Spinner />
      ) : (q.data ?? []).length === 0 ? (
        <EmptyState>Aucune entrée.</EmptyState>
      ) : (
        <Table>
          <THead><tr><Th>Date</Th><Th>Utilisateur</Th><Th>Action</Th><Th>Cible</Th><Th>Détails</Th><Th>IP</Th></tr></THead>
          <TBody>
            {(q.data ?? []).map((a) => (
              <Tr key={a.id}>
                <Td className="whitespace-nowrap text-fg-muted">{fmtDateTime(a.at)}</Td>
                <Td className="font-medium">{a.username || a.userId}</Td>
                <Td><Badge tone="neutral">{a.action}</Badge></Td>
                <Td className="font-mono text-[11px]">{a.target}</Td>
                <Td className="max-w-md truncate text-fg-muted" title={a.details}>{a.details}</Td>
                <Td className="font-mono text-[11px] text-fg-subtle">{a.ip}</Td>
              </Tr>
            ))}
          </TBody>
        </Table>
      )}
    </Card>
  );
}

export function SystemInfoCard() {
  const q = useApi(() => api.system.info(), []);
  useInterval(() => void q.refetch(), 15000);
  const i = q.data;
  return (
    <Card>
      <CardHeader title="Système" actions={<Button size="xs" variant="ghost" onClick={() => void q.refetch()}><RefreshCw className="size-3.5" /></Button>} />
      <CardBody>
        {q.error && <ErrorBox message={errorMessage(q.error)} />}
        {!i ? (
          <Spinner />
        ) : (
          <DescriptionList
            items={[
              ["Version backend", i.version],
              ["Disponibilité", fmtDuration(i.uptimeS)],
              ["Stockage", i.store],
              ["MQTT", `${i.mqtt?.embedded ? "embarqué" : "externe"} · ${i.mqtt?.url ?? "—"}`],
              ["Véhicules en ligne", String(i.vehiclesOnline)],
              ["Heure serveur", fmtDateTime(i.time)],
              ["API (frontend)", apiBaseUrl()],
              ["WebSocket (frontend)", wsUrl()],
            ]}
          />
        )}
      </CardBody>
    </Card>
  );
}
