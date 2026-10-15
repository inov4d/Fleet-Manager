"use client";
import { useEffect } from "react";
import { api, errorMessage } from "@/lib/api";
import { useApi } from "@/hooks/use-api";
import { useSiteStore } from "@/stores/site";
import { Card, CardHeader, CardBody, ErrorBox, Spinner } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

/** Live view of the ant-colony board: open tasks, agents and recent claims. */
export function ColonyPanel() {
  const siteId = useSiteStore((s) => s.siteId);
  const q = useApi(() => api.settings.getColony(siteId), [siteId]);
  const refetch = q.refetch;
  useEffect(() => {
    const id = window.setInterval(() => void refetch(), 2000);
    return () => window.clearInterval(id);
  }, [refetch]);
  if (q.error) return <ErrorBox message={errorMessage(q.error)} />;
  if (!q.data) return <Spinner />;
  const d = q.data;
  return (
    <Card>
      <CardHeader
        title="Tableau de la colonie"
        subtitle={d.mode === "colony" ? `${d.tasks.length} tâche(s) ouvertes · ${d.stats.granted} revendications accordées · ${d.stats.rejected} refusées · ${d.stats.negotiated} cessions de passage négociées` : "Mode central actif : le tableau est vide, le répartiteur affecte les missions."}
      />
      <CardBody className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div>
          <h4 className="text-sm font-medium mb-2">Tâches ouvertes</h4>
          {d.tasks.length === 0 ? (
            <p className="text-sm text-muted">Aucune tâche en attente.</p>
          ) : (
            <table className="w-full text-sm">
              <thead><tr className="text-left text-muted"><th>Tâche</th><th>Trajet</th><th>Prio.</th><th>Urgence</th><th>Masse</th></tr></thead>
              <tbody>
                {d.tasks.map((t) => (
                  <tr key={t.id} className="border-t border-border">
                    <td className="py-1 font-mono text-xs">{t.id.slice(0, 8)}</td>
                    <td>{t.origin} → {t.destination}</td>
                    <td>{t.priority}</td>
                    <td>{t.urgency.toFixed(1)}</td>
                    <td>{Math.round(t.massKg)} kg</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
        <div>
          <h4 className="text-sm font-medium mb-2">Agents embarqués (véhicules simulés)</h4>
          {d.agents.length === 0 ? (
            <p className="text-sm text-muted">Aucun agent en cours dans ce processus (les AGV réels exécutent <code>agvagent</code> à bord).</p>
          ) : (
            <table className="w-full text-sm">
              <thead><tr className="text-left text-muted"><th>Véhicule</th><th>État</th><th>Batterie</th><th>Nœud</th><th>Revend. / gagnées</th><th>Cédé / passé</th></tr></thead>
              <tbody>
                {d.agents.map((a) => (
                  <tr key={a.vehicleId} className="border-t border-border">
                    <td className="py-1">{a.vehicleId}</td>
                    <td><Badge tone={a.assignment.status === "executing" ? "info" : a.assignment.status === "assigned" ? "warning" : "neutral"}>{a.assignment.status || "idle"}</Badge></td>
                    <td>{Math.round(a.battery)} %</td>
                    <td>{a.node || "—"}</td>
                    <td>{a.claims} / {a.wins}</td>
                    <td>{a.yielded} / {a.passed}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {d.stats.yields.length > 0 && (
            <div className="mt-3">
              <h4 className="text-sm font-medium mb-1">Cessions de passage négociées entre voisins</h4>
              <ul className="text-xs text-muted space-y-1">
                {d.stats.yields.slice(-6).reverse().map((y) => (
                  <li key={y.nonce + y.vehicleId}>
                    <span className="text-warning">{y.vehicleId}</span> cède {y.resource.type} {y.resource.id} à {y.beneficiary} (coût {y.myCost.toFixed(1)}, chaîne de {y.chain?.length ?? 2} : {(y.chain ?? []).join(" → ")})
                  </li>
                ))}
              </ul>
            </div>
          )}
          {d.stats.recent.length > 0 && (
            <div className="mt-3">
              <h4 className="text-sm font-medium mb-1">Dernières décisions</h4>
              <ul className="text-xs text-muted space-y-1">
                {d.stats.recent.slice(-8).reverse().map((g) => (
                  <li key={g.claimId}>
                    <span className={g.granted ? "text-success" : ""}>{g.vehicleId}</span> · tâche {g.taskId.slice(0, 8)} · {g.granted ? "accordée" : "refusée"} ({g.reason}{g.competing > 1 ? `, ${g.competing} candidats` : ""})
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      </CardBody>
    </Card>
  );
}
