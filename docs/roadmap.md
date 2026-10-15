# Phases de développement et jalons

| Phase | Contenu | État |
|---|---|---|
| **1 — Fondations** | Modèle de domaine, géométrie, cartes, planificateur A*, Traffic Manager (réservations, enveloppes, interblocages), tests unitaires et test de charge 40 AGV | ✅ livré |
| **2 — Communication et simulation** | VDA 5050 2.0.0, broker MQTT embarqué, passerelle (heartbeat, séquence, ordres), simulateur d'AGV omnidirectionnels, pannes injectables | ✅ livré |
| **3 — Fleet Core et services** | Missions (types, états, dépendances, attribution explicable, réaffectation contrôlée), batterie/recharge, alarmes, maintenance, analytique, auth/RBAC/audit, persistance mémoire + PostgreSQL, API REST/WS, intégration REST/MQTT/webhooks | ✅ livré |
| **4 — Interface** | Next.js : tableau de bord, carte temps réel PixiJS, éditeur de carte, missions, trafic, alarmes, maintenance, analytique, simulation, administration, vue 3D | ✅ livré (validation visuelle sur site à faire) |
| **5 — Industrialisation** | Serveur OPC UA (contrat prêt), planification coopérative (CA*/MAPF via l'interface `Planner`), réservations temporelles, import de plans (DXF/PNG en fond de carte), tests E2E navigateur (Playwright), packaging appliance, SSO | ⏳ à venir |
| **6 — Mise en service** | Adaptateur contrôleur propriétaire, validation avec AGV réels, réglage des gabarits et marges issus de l'analyse de risques, haute disponibilité | ⏳ à venir |

Chaque jalon livre une application exécutable (`make run-sim` / `docker compose up`), des
tests automatisés (`make test`) et cette documentation.

## Limites connues de cette version

- Les réservations sont spatiales (pas de réservation temporelle) ; l'interface `Planner`
  permet d'ajouter un planificateur coopératif.
- Les véhicules simulés disparaissent au redémarrage ; les missions affectées repassent en file.
- OPC UA : contrat uniquement.
- Les gabarits, vitesses et consommations des modèles AGV 20T / 40T sont des valeurs
  d'ingénierie par défaut, modifiables via `PUT /vehicle-models/{id}`.
