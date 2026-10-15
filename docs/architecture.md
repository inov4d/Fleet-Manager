# Architecture logicielle

## Principe

Fleet Manager est un **monolithe modulaire** : un seul processus Go (`fleetd`) qui embarque
tous les services sous forme de packages aux interfaces explicites, plus une interface web
Next.js. Chaque module peut être extrait en service séparé plus tard sans changer le modèle
de données (les frontières sont des interfaces Go, et les messages véhicules passent déjà
par MQTT).

```
┌──────────────── fleetd (Go) ─────────────────────────────────────────────┐
│  API REST + WebSocket (internal/api)      Auth / RBAC / Audit             │
│  ┌──────────── Fleet Core (internal/fleet) ───────────────────────────┐   │
│  │ Vehicle Registry · Mission Manager · Task Scheduler (dispatcher)   │   │
│  │ Route Planner · Traffic Manager · Resource Reservation Manager     │   │
│  │ Battery / Charging Manager · Alarm Manager · Maintenance Manager   │   │
│  │ Map Manager · Analytics Engine · Simulation Engine                 │   │
│  └─────────────────────────────────┬──────────────────────────────────┘   │
│  Vehicle Communication Gateway (VDA 5050 2.0.0)   Industrial Integration  │
│  Store (mémoire+JSON | PostgreSQL/TimescaleDB)    (REST clés API, MQTT,   │
│  Broker MQTT embarqué (Mochi)                      webhooks, OPC UA*)     │
└──────────────────────────────────────────────────────────────────────────┘
          ▲ MQTT (VDA 5050)                     ▲ HTTP / WS
   AGV réels ou simulés                  Frontend Next.js / MES / WMS / ERP
```

\* OPC UA : contrat d'adaptateur défini (`internal/integration/opcua.go`), serveur en feuille de route.

## Modules (backend/internal)

| Package | Service du cahier des charges | Rôle |
|---|---|---|
| `core` | — | Modèle de domaine partagé (véhicules, missions, cartes, réservations, alarmes…), horloge simulable |
| `geometry` | Traffic Manager | Empreintes orientées, enveloppes balayées des manœuvres omnidirectionnelles, intersections de polygones |
| `mapmgr` | Map Manager | Validation des cartes, graphe orienté, zones ↔ nœuds/arêtes, carte démo |
| `planner` | Route Planner | A* sur états (nœud, arête entrante) avec coûts distance/temps/énergie/manœuvre/congestion, zones interdites, largeur de couloir vs gabarit. Interface `Planner` pour CA*/MAPF |
| `traffic` | Traffic Manager + Resource Reservation Manager | Réservations nœud/arête/zone avec capacités, sens unique, conflits géométriques d'enveloppes, états incertains, blocages, interblocages |
| `vda5050` | Vehicle Communication Gateway | Modèle de messages VDA 5050 2.0.0, topics, validation, contrôle de séquence |
| `transport` | — | Bus mémoire, client MQTT (Paho), broker embarqué (Mochi) |
| `gateway` | Vehicle Communication Gateway | Enregistrement des véhicules, heartbeat, doublons, envoi d'ordres / instant actions, construction d'ordres (base libérée / horizon) |
| `sim` | Simulation Engine | AGV simulés parlant VDA 5050 (cinématique omnidirectionnelle 4 bogies, batterie, actions, pannes) |
| `mission` | Mission Manager + Task Scheduler | Cycle de vie des missions, dépendances, réaffectation contrôlée, moteur d'attribution explicable (mode central) |
| `colony` | Task Scheduler décentralisé | Tableau de tâches partagé, arbitrage des revendications, agent embarqué open source (`cmd/agvagent`) — mode colonie, voir `docs/colony.md` |
| `battery` (dans `fleet`) | Battery + Charging Manager | Seuils, recharge d'opportunité, choix de station, retour en station |
| `alarm` | Alarm Manager | Alarmes dédupliquées, acquittement, historique |
| `maintenance` | Maintenance Manager | Plans par compteur (km, heures, missions, calendrier), statuts, interventions |
| `analytics` | Analytics Engine | KPI, utilisation, séries temporelles |
| `auth`, `audit` | Authentication & Authorization, Audit Logger | Utilisateurs bcrypt, JWT, clés API, matrice de rôles, journal d'audit |
| `store` | — | Interface de persistance, implémentations mémoire (snapshot JSON) et PostgreSQL |
| `fleet` | Fleet Core | Boucle de contrôle 10 Hz qui orchestre tout, contrôleur de simulation |
| `api` | — | REST (`docs/api.md`), WebSocket (snapshots 4 Hz + événements) |
| `integration` | Industrial Integration Gateway | Topics MQTT `fleet/<site>/…`, contrat OPC UA |

## Boucle de contrôle (Fleet Core, 10 Hz)

1. Détection des pertes de communication (heartbeat gateway).
2. Rafraîchissement de l'état dérivé de chaque véhicule (disponibilité, gabarit effectif,
   compteurs) et observation par le Traffic Manager (libération des ressources quittées,
   uniquement sur position rapportée).
3. Exécutions : progression (sequenceId VDA 5050 = 2 × index de route), fin des étapes
   (actions FINISHED), demande de réservations sur la fenêtre d'anticipation, envoi
   d'ordres / mises à jour d'ordre (base = nœuds réservés, horizon = reste), replanification
   quand une ressource reste refusée.
4. Détection d'interblocages (graphe d'attente) → replanification ou repli du véhicule le
   moins prioritaire.
5. Véhicules bloqués (pas de progression).
6. Attribution des missions en attente (missions épinglées puis libres), gestion batterie,
   retour en station.
7. Télémétrie, échantillons analytiques, évaluation de la maintenance, persistance des compteurs.
8. Génération de missions du scénario de simulation.

Le cœur n'a aucun contrôle temps réel des moteurs : il planifie à l'échelle de la flotte et
le contrôleur embarqué exécute la navigation et la sécurité.

## Données

- Configuration et historique : `store.Store` (mémoire + snapshot `data/fleet.json`, ou
  PostgreSQL ; TimescaleDB détecté automatiquement pour la télémétrie et les métriques).
- État temps réel (positions, réservations, exécutions) : mémoire du processus ; reconstruit
  à partir des véhicules au redémarrage (les missions affectées repassent en attente, les
  véhicules simulés sont supprimés).

## Évolutivité 1 → 40 AGV

- Test de charge `traffic.TestFortyVehiclesInvariants` : 40 véhicules, 400 cycles, invariants
  de capacité et d'enveloppes vérifiés, < 2 s.
- Les coûts dominants (intersections de polygones) sont O(véhicules × segments) par cycle ;
  à 40 véhicules la boucle reste sous quelques millisecondes.
- Multi-sites : chaque site a sa carte active et son graphe ; les entités sont scopées par
  `siteId`.

## Séparation future en services

Les candidats naturels sont : la passerelle véhicules (déjà découplée par MQTT), la simulation
(mêmes messages), et l'analytique (lecture seule du store). Le Traffic Manager et le Mission
Manager doivent rester co-localisés avec le Fleet Core (cohérence transactionnelle des
réservations).
