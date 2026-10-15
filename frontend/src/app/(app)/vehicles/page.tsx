"use client";
import Link from "next/link";
import { useMemo, useState } from "react";
import { Plus, Search } from "lucide-react";
import { useFleetStore } from "@/stores/fleet";
import { useAuthStore } from "@/stores/auth";
import { can } from "@/lib/auth";
import { fmtCoord, fmtRelative } from "@/lib/format";
import { AVAILABILITY_LABELS, CONNECTION_LABELS } from "@/lib/labels";
import type { Availability, VehicleConnection } from "@/lib/types";
import { useNow } from "@/hooks/use-now";
import { useNodeName } from "@/hooks/use-node-names";
import { PageHeader, Card, BatteryBar, EmptyState } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Table, TBody, Td, Th, THead, Tr } from "@/components/ui/table";
import { AvailabilityBadge, ConnectionBadge } from "@/components/ui/badge";
import { Chips } from "@/components/ui/tabs";

export default function VehiclesPage() {
  const vehicles = useFleetStore((s) => s.vehicles);
  const wsStatus = useFleetStore((s) => s.wsStatus);
  const user = useAuthStore((s) => s.user);
  const now = useNow(2000);
  const nodeName = useNodeName();
  const [q, setQ] = useState("");
  const [avail, setAvail] = useState<Availability[]>([]);
  const [conn, setConn] = useState<string>("");
  const [model, setModel] = useState<string>("");

  const models = useMemo(() => Array.from(new Set(vehicles.map((v) => v.vehicle.modelId))).sort(), [vehicles]);
  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return vehicles.filter((v) => {
      if (needle && !`${v.vehicle.name} ${v.vehicle.id} ${v.vehicle.serialNumber}`.toLowerCase().includes(needle)) return false;
      if (avail.length && !avail.includes(v.state.availability)) return false;
      if (conn && v.state.connection !== conn) return false;
      if (model && v.vehicle.modelId !== model) return false;
      return true;
    });
  }, [vehicles, q, avail, conn, model]);

  const counts = vehicles.reduce<Partial<Record<Availability, number>>>((acc, v) => {
    acc[v.state.availability] = (acc[v.state.availability] ?? 0) + 1;
    return acc;
  }, {});

  return (
    <div>
      <PageHeader
        title="Véhicules"
        subtitle={`${vehicles.length} véhicules · données temps réel${wsStatus !== "connected" ? " (connexion temps réel indisponible)" : ""}`}
        actions={
          can(user, "vehicles.manage") && (
            <Link href="/vehicles/new">
              <Button variant="primary" size="sm">
                <Plus className="size-3.5" /> Nouveau véhicule
              </Button>
            </Link>
          )
        }
      />
      <Card>
        <div className="flex flex-wrap items-center gap-2 p-2 border-b border-border">
          <div className="relative">
            <Search className="size-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-fg-subtle" aria-hidden />
            <Input aria-label="Rechercher" placeholder="Rechercher…" value={q} onChange={(e) => setQ(e.target.value)} className="pl-7 w-56" />
          </div>
          <Select
            aria-label="Connexion"
            value={conn}
            onChange={(e) => setConn(e.target.value)}
            placeholder="Toutes connexions"
            options={(Object.keys(CONNECTION_LABELS) as VehicleConnection[]).map((k) => ({ value: k, label: CONNECTION_LABELS[k] }))}
            className="w-44"
          />
          <Select
            aria-label="Modèle"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            placeholder="Tous modèles"
            options={models.map((m) => ({ value: m, label: m }))}
            className="w-40"
          />
          <Chips
            items={(Object.keys(AVAILABILITY_LABELS) as Availability[]).map((k) => ({ key: k, label: AVAILABILITY_LABELS[k], count: counts[k] ?? 0 }))}
            value={avail}
            onChange={setAvail}
          />
        </div>
        {filtered.length === 0 ? (
          <EmptyState>Aucun véhicule ne correspond aux filtres.</EmptyState>
        ) : (
          <Table>
            <THead>
              <tr>
                <Th>Nom</Th>
                <Th>Modèle</Th>
                <Th>Connexion</Th>
                <Th>Disponibilité</Th>
                <Th>Batterie</Th>
                <Th>Mission</Th>
                <Th>Position</Th>
                <Th>Dernier nœud</Th>
                <Th>Dernière vue</Th>
              </tr>
            </THead>
            <TBody>
              {filtered.map(({ vehicle, state }) => (
                <Tr key={vehicle.id}>
                  <Td>
                    <Link href={`/vehicles/${encodeURIComponent(vehicle.id)}`} className="font-medium hover:underline">
                      {vehicle.name}
                    </Link>
                    <div className="text-[10px] text-fg-subtle">{vehicle.id}{vehicle.protocol === "simulated" ? " · simulé" : ""}</div>
                  </Td>
                  <Td>{vehicle.modelId}</Td>
                  <Td><ConnectionBadge value={state.connection} /></Td>
                  <Td>
                    <AvailabilityBadge value={state.availability} />
                    {state.paused && <span className="ml-1 text-[10px] text-warning">pause</span>}
                  </Td>
                  <Td><BatteryBar value={state.batteryCharge} charging={state.charging} /></Td>
                  <Td>
                    {state.missionId ? (
                      <Link href={`/missions/${encodeURIComponent(state.missionId)}`} className="hover:underline">{state.missionId}</Link>
                    ) : (
                      <span className="text-fg-subtle">—</span>
                    )}
                  </Td>
                  <Td className="tabular whitespace-nowrap">{fmtCoord(state.pose?.x)} ; {fmtCoord(state.pose?.y)}</Td>
                  <Td>{nodeName(state.lastNodeId)}</Td>
                  <Td className="text-fg-muted whitespace-nowrap">{fmtRelative(state.lastSeen, now)}</Td>
                </Tr>
              ))}
            </TBody>
          </Table>
        )}
      </Card>
    </div>
  );
}
