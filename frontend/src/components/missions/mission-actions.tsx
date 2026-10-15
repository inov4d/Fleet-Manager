"use client";
import { useState } from "react";
import { ArrowUpDown, Pause, Play, Shuffle, Trash2, XCircle } from "lucide-react";
import { api, errorMessage } from "@/lib/api";
import { can } from "@/lib/auth";
import { TERMINAL_MISSION_STATUSES, type Mission } from "@/lib/types";
import { useAuthStore } from "@/stores/auth";
import { useFleetStore } from "@/stores/fleet";
import { toast } from "@/stores/ui";
import { Button, IconButton } from "@/components/ui/button";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Select, Slider } from "@/components/ui/input";

type Action = "cancel" | "suspend" | "resume" | "reassign" | "priority" | "delete";

export function MissionActions({ mission, compact, onChanged }: { mission: Mission; compact?: boolean; onChanged?: () => void }) {
  const user = useAuthStore((s) => s.user);
  const vehicles = useFleetStore((s) => s.vehicles);
  const [busy, setBusy] = useState<Action | null>(null);
  const [dialog, setDialog] = useState<Action | null>(null);
  const [priority, setPriority] = useState(mission.priority);
  const [vehicleId, setVehicleId] = useState("");
  const [confirmLoad, setConfirmLoad] = useState(false);

  const terminal = TERMINAL_MISSION_STATUSES.includes(mission.status);
  const canCancel = can(user, "missions.cancel") && !terminal;
  const canManage = can(user, "missions.manage") && !terminal;
  const canDelete = can(user, "missions.manage") && terminal;

  const run = async (action: Action) => {
    setBusy(action);
    try {
      switch (action) {
        case "cancel":
          await api.missions.cancel(mission.id);
          break;
        case "suspend":
          await api.missions.suspend(mission.id);
          break;
        case "resume":
          await api.missions.resume(mission.id);
          break;
        case "reassign":
          await api.missions.reassign(mission.id, { vehicleId: vehicleId || undefined, confirmLoadState: confirmLoad || undefined });
          break;
        case "priority":
          await api.missions.setPriority(mission.id, priority);
          break;
        case "delete":
          await api.missions.remove(mission.id);
          break;
      }
      toast.success("Action effectuée", `${mission.name || mission.id} · ${action}`);
      setDialog(null);
      onChanged?.();
    } catch (e) {
      toast.error("Action refusée", errorMessage(e));
    } finally {
      setBusy(null);
    }
  };

  if (!canCancel && !canManage && !canDelete) return null;
  const size = compact ? "xs" : "sm";

  return (
    <div className="inline-flex items-center gap-1 justify-end">
      {canManage && mission.status !== "suspended" && (mission.status === "executing" || mission.status === "assigned" || mission.status === "planned") && (
        <IconButton label="Suspendre" size={size} onClick={() => run("suspend")} loading={busy === "suspend"}>
          <Pause className="size-3.5" />
        </IconButton>
      )}
      {canManage && mission.status === "suspended" && (
        <IconButton label="Reprendre" size={size} onClick={() => run("resume")} loading={busy === "resume"}>
          <Play className="size-3.5" />
        </IconButton>
      )}
      {canManage && (
        <IconButton label="Réaffecter" size={size} onClick={() => setDialog("reassign")}>
          <Shuffle className="size-3.5" />
        </IconButton>
      )}
      {canManage && (
        <IconButton label="Modifier la priorité" size={size} onClick={() => { setPriority(mission.priority); setDialog("priority"); }}>
          <ArrowUpDown className="size-3.5" />
        </IconButton>
      )}
      {canCancel && (
        <IconButton label="Annuler la mission" size={size} onClick={() => setDialog("cancel")} className="text-danger">
          <XCircle className="size-3.5" />
        </IconButton>
      )}
      {canDelete && (
        <IconButton label="Supprimer la mission" size={size} onClick={() => setDialog("delete")} className="text-danger">
          <Trash2 className="size-3.5" />
        </IconButton>
      )}

      <ConfirmDialog
        open={dialog === "cancel"}
        onClose={() => setDialog(null)}
        onConfirm={() => run("cancel")}
        title="Annuler la mission"
        message={`Annuler la mission « ${mission.name || mission.id} » ? Le véhicule s’arrêtera à la prochaine position sûre.`}
        confirmLabel="Annuler la mission"
        danger
        loading={busy === "cancel"}
      />
      <ConfirmDialog
        open={dialog === "delete"}
        onClose={() => setDialog(null)}
        onConfirm={() => run("delete")}
        title="Supprimer la mission"
        message="Cette mission terminée sera supprimée définitivement de l’historique."
        confirmLabel="Supprimer"
        danger
        loading={busy === "delete"}
      />
      <Dialog
        open={dialog === "reassign"}
        onClose={() => setDialog(null)}
        title="Réaffecter la mission"
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setDialog(null)}>Annuler</Button>
            <Button variant="primary" onClick={() => run("reassign")} loading={busy === "reassign"}>Réaffecter</Button>
          </>
        }
      >
        <div className="flex flex-col gap-3">
          <Field label="Véhicule cible" hint="Laisser vide pour laisser le répartiteur choisir.">
            <Select
              value={vehicleId}
              onChange={(e) => setVehicleId(e.target.value)}
              placeholder="Automatique"
              options={vehicles.map((v) => ({ value: v.vehicle.id, label: `${v.vehicle.name} (${v.state.availability})` }))}
            />
          </Field>
          {mission.load?.present && (
            <Checkbox
              label="Je confirme que l’état physique de la charge a été vérifié (confirmLoadState)"
              checked={confirmLoad}
              onChange={(e) => setConfirmLoad(e.target.checked)}
            />
          )}
        </div>
      </Dialog>
      <Dialog
        open={dialog === "priority"}
        onClose={() => setDialog(null)}
        title="Priorité de la mission"
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setDialog(null)}>Annuler</Button>
            <Button variant="primary" onClick={() => run("priority")} loading={busy === "priority"}>Appliquer</Button>
          </>
        }
      >
        <Field label={`Priorité (1 = basse, 10 = haute)`}>
          <Slider min={1} max={10} step={1} value={priority} onChange={(e) => setPriority(Number(e.target.value))} valueLabel={String(priority)} />
        </Field>
      </Dialog>
    </div>
  );
}
