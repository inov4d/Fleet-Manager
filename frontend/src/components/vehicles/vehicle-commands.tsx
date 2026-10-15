"use client";
import { useState } from "react";
import { BatteryCharging, Eraser, Pause, Play, Power, PowerOff, Wrench, XCircle, MapPin } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { can } from "@/lib/auth";
import type { MapNode, VehicleCommand, VehicleWithState } from "@/lib/types";
import { useAuthStore } from "@/stores/auth";
import { toast } from "@/stores/ui";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/dialog";
import { Select } from "@/components/ui/input";

export function VehicleCommands({ item, nodes, compact }: { item: VehicleWithState; nodes?: MapNode[]; compact?: boolean }) {
  const user = useAuthStore((s) => s.user);
  const [busy, setBusy] = useState<VehicleCommand | null>(null);
  const [confirmCancel, setConfirmCancel] = useState(false);
  const [sendNode, setSendNode] = useState("");
  const { vehicle, state } = item;
  const basic = can(user, "vehicles.command");
  const full = can(user, "vehicles.command.all");

  const run = async (command: VehicleCommand, nodeId?: string) => {
    setBusy(command);
    try {
      await api.vehicles.command(vehicle.id, { command, nodeId });
      toast.success("Commande acceptée", `${vehicle.name} · ${command}`);
    } catch (e) {
      toast.error("Commande refusée", errorMessage(e));
    } finally {
      setBusy(null);
      setConfirmCancel(false);
    }
  };

  if (!basic) return <p className="text-[11px] text-fg-subtle">Aucune commande disponible pour votre rôle.</p>;
  const size = compact ? "sm" : "md";

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap gap-1.5">
        {state.paused ? (
          <Button size={size} variant="primary" onClick={() => run("resume")} loading={busy === "resume"}>
            <Play className="size-3.5" /> Reprendre
          </Button>
        ) : (
          <Button size={size} onClick={() => run("pause")} loading={busy === "pause"}>
            <Pause className="size-3.5" /> Pause
          </Button>
        )}
        <Button size={size} variant="danger" onClick={() => setConfirmCancel(true)} disabled={!state.missionId} loading={busy === "cancelMission"}>
          <XCircle className="size-3.5" /> Annuler mission
        </Button>
        {full && (
          <>
            <Button size={size} onClick={() => run("sendToCharge")} loading={busy === "sendToCharge"}>
              <BatteryCharging className="size-3.5" /> Envoyer en charge
            </Button>
            <Button size={size} onClick={() => run("clearErrors")} loading={busy === "clearErrors"} disabled={state.errors.length === 0}>
              <Eraser className="size-3.5" /> Effacer erreurs
            </Button>
            {vehicle.enabled ? (
              <Button size={size} onClick={() => run("disable")} loading={busy === "disable"}>
                <PowerOff className="size-3.5" /> Désactiver
              </Button>
            ) : (
              <Button size={size} onClick={() => run("enable")} loading={busy === "enable"}>
                <Power className="size-3.5" /> Activer
              </Button>
            )}
            {state.availability === "maintenance" ? (
              <Button size={size} onClick={() => run("endMaintenance")} loading={busy === "endMaintenance"}>
                <Wrench className="size-3.5" /> Fin maintenance
              </Button>
            ) : (
              <Button size={size} onClick={() => run("maintenance")} loading={busy === "maintenance"}>
                <Wrench className="size-3.5" /> Maintenance
              </Button>
            )}
          </>
        )}
      </div>
      {full && nodes && nodes.length > 0 && (
        <div className="flex items-center gap-1.5">
          <Select
            aria-label="Nœud de destination"
            value={sendNode}
            onChange={(e) => setSendNode(e.target.value)}
            placeholder="Envoyer vers un nœud…"
            options={nodes.map((n) => ({ value: n.id, label: n.name || n.id }))}
            className="h-7 text-xs"
          />
          <Button size="sm" disabled={!sendNode} onClick={() => run("sendToNode", sendNode)} loading={busy === "sendToNode"}>
            <MapPin className="size-3.5" /> Envoyer
          </Button>
        </div>
      )}
      <ConfirmDialog
        open={confirmCancel}
        onClose={() => setConfirmCancel(false)}
        onConfirm={() => run("cancelMission")}
        title="Annuler la mission en cours"
        message={`Annuler la mission en cours du véhicule ${vehicle.name} ?`}
        confirmLabel="Annuler la mission"
        danger
        loading={busy === "cancelMission"}
      />
    </div>
  );
}
