# Fleet Manager AGV 20T / 40T

Plateforme de gestion de flotte pour AGV industriels lourds (20 t et 40 t, quatre bogies
orientables, déplacement omnidirectionnel). De 1 à 40 AGV, un ou plusieurs sites,
fonctionnement local sans Internet.

**Ce dépôt contient du code fonctionnel** : un backend Go (`backend/`), une interface web
Next.js (`frontend/`), les fichiers de déploiement (`deploy/`) et la documentation (`docs/`).

## Démarrage rapide

Prérequis : Go 1.24, Node 22 (ou Docker).

```bash
# Backend avec 8 AGV simulés (API :8080, broker MQTT :1883)
make run-sim

# Interface web (autre terminal)
cd frontend && npm ci && npm run dev
# → http://localhost:3000, identifiants admin / admin
```

Ou tout-en-un : `cp deploy/.env.example deploy/.env && make up` puis http://localhost:3000.

## Fonctionnalités livrées

- Enregistrement et configuration des AGV (modèles 20T/40T configurables, gabarits, marges).
- Communication **VDA 5050 2.0.0** sur MQTT (broker embarqué ou externe) : états, ordres,
  mises à jour d'ordre base/horizon, instant actions, heartbeat, séquence, validation.
- Missions : transport, multi-arrêts, approvisionnement, évacuation, transfert, retour station,
  recharge, manuelle, externe ; états complets, dépendances, priorités, historique.
- **Attribution automatique explicable** (distance, temps, batterie, congestion, file,
  capacité, compatibilité géométrique, modèle requis) et réaffectation contrôlée (jamais
  automatique si une charge peut être à bord).
- **Gestion du trafic** : réservations nœud/arête/zone, enveloppes balayées des manœuvres
  omnidirectionnelles (charge, débordement, marges, incertitude de localisation), couloirs
  à sens unique, croisements, zones d'attente, détection de blocages et d'**interblocages**
  avec résolution, zones temporairement indisponibles, états incertains en perte de
  communication (jamais libérés sur délai).
- Planification A* à coûts pondérés (distance, temps, énergie, manœuvres, congestion, zones
  interdites, largeur de couloir), replanification ; interface prête pour CA*/MAPF.
- Batterie : recharge sur seuils, recharge d'opportunité, choix de station, retour en station.
- Alarmes (véhicule, communication, batterie, trafic, mission, maintenance), maintenance
  préventive par compteurs, KPI et séries temporelles.
- Simulation complète (AGV simulés parlant VDA 5050, scénarios, échelle de temps, pannes :
  perte de communication, blocage, défaut, arrêt d'urgence).
- Interface : tableau de bord, carte temps réel, éditeur de carte graphique, missions,
  trafic, alarmes, maintenance, analytique, simulation, administration, vue 3D.
- Utilisateurs, rôles, clés API, audit ; intégration REST / MQTT / webhooks, contrat OPC UA.
- Persistance mémoire (snapshot JSON) ou PostgreSQL/TimescaleDB ; multi-sites.

## Documentation

- [Architecture](docs/architecture.md) · [Trafic et réservations](docs/traffic.md)
- [VDA 5050](docs/vda5050.md) · [Intégration industrielle](docs/integration.md)
- [API](docs/api.md) · [Sécurité](docs/security.md) · [Déploiement](docs/deployment.md)
- [Phases et feuille de route](docs/roadmap.md)

## Tests

```bash
make test-backend   # Go : géométrie, planificateur, trafic (40 AGV), VDA 5050, simulateur, store, services, bout-en-bout
cd frontend && npm run lint && npm run build
```

## Structure

```
backend/   fleetd (Go) : cmd/fleetd, internal/{core,geometry,mapmgr,planner,traffic,vda5050,transport,gateway,sim,mission,alarm,maintenance,analytics,auth,audit,store,fleet,api,integration,config}
frontend/  Next.js 15 / React 19 / Tailwind / PixiJS / recharts / three
deploy/    docker-compose.yml, .env.example
docs/      documentation technique
```

## Avertissement sécurité

Le Fleet Manager coordonne le trafic et les missions ; il **n'est pas** une fonction de
sécurité. Les distances de sécurité, les arrêts et le comportement en perte de communication
relèvent de l'analyse de risques et des contrôleurs embarqués des AGV.
