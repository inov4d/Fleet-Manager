# Mode colonie — attribution décentralisée des tâches

## Principe

Le Fleet Manager peut fonctionner comme une fourmilière : il ne décide plus qui fait quoi.
Il publie un **tableau de tâches partagé** (stigmergie) et chaque AGV embarque un **agent
autonome open source** qui observe le tableau, évalue chaque tâche avec ses propres
informations (position, batterie, charge, gabarit, estimation de trajet sur la carte partagée)
et **revendique la tâche la plus opportune**. Le centre ne fait que :

1. publier le tableau, la carte et les fiches véhicules ;
2. arbitrer les revendications simultanées (la moins coûteuse l'emporte dans une courte fenêtre) ;
3. vérifier l'éligibilité comme filet de sécurité (site, modèle, capacité, batterie, trajet) ;
4. continuer à protéger l'espace partagé (réservations, interblocages, pertes de communication).

Activation : `PUT /api/v1/settings/dispatcher` avec `"mode": "colony"` (ou l'onglet
« Répartiteur » de l'interface). Le mode `central` reste disponible.

## Règles locales de l'agent (`internal/colony/agent.go`, `cmd/agvagent`)

- **Seuil de réponse** : pour chaque tâche ouverte, l'agent calcule un coût `c` (temps de
  trajet jusqu'à l'origine + mission + manutention + pénalité de batterie) avec son
  planificateur local. Il hésite pendant `délai = k · c / urgence` (borné entre 100 ms et 3 s)
  puis revendique si la tâche est encore ouverte. Les agents les mieux placés revendiquent en
  premier ; la colonie converge sans classement central.
- **Urgence** (phéromone de la tâche) : priorité + minutes d'attente, doublée après l'échéance.
  Une tâche qui attend attire de plus en plus d'agents.
- **Une tâche à la fois** : l'agent ne revendique rien tant qu'il transporte ou exécute.
- **Fatigue** : batterie sous le seuil bas et agent inactif → demande de recharge ; sous le
  seuil critique → demande immédiate. L'agent ne prend pas une tâche qui le laisserait sous la
  réserve d'énergie.
- **Repos** : inactif longtemps sans tâche ouverte → demande de retour en station.
- **Renoncement** : `GiveBack` rend une tâche au tableau (le centre refuse si une charge est à bord).

Tous les seuils sont des variables d'environnement de `agvagent` (`AGV_LOW_BATTERY`,
`AGV_CRITICAL_BATTERY`, `AGV_CLAIM_SCALE`…). Les AGV simulés exécutent exactement le même agent
dans le processus `fleetd`.

## Protocole (MQTT, JSON)

| Topic | Sens | Contenu |
|---|---|---|
| `fleet/<site>/board/tasks` (retenu) | centre → agents | `{"ts","siteId","tasks":[{id,type,origin,destination,nodes,priority,urgency,massKg,loadPresent,loadLengthM,loadWidthM,requiredModelId,deadline,createdAt}]}` |
| `fleet/<site>/board/claims` | agent → centre | `{"claimId","taskId","vehicleId","cost","at"}` |
| `fleet/<site>/board/grants/<vehicleId>` | centre → agent | `{"claimId","taskId","vehicleId","granted","reason","competing"}` |
| `fleet/<site>/board/requests` | agent → centre | `{"vehicleId","type":"charge"\|"park","reason"}` |
| `fleet/<site>/board/releases` | agent → centre | `{"vehicleId","taskId","reason"}` |
| `fleet/<site>/board/assignments/<vehicleId>` (retenu) | centre → agent | `{"vehicleId","taskId","status":"idle"\|"assigned"\|"executing","ts"}` |
| `fleet/<site>/map` (retenu) | centre → agents | carte active (même JSON que l'API) |
| `fleet/<site>/vehicles/<vehicleId>` (retenu) | centre → agent | `{"id","siteId","modelId","model":{…},"enabled"}` |

L'agent lit sa propre position sur le topic VDA 5050 `state` de son contrôleur. Toute
implémentation respectant ces messages (Python, ROS 2, automate…) peut remplacer l'agent de
référence : le centre ne suppose rien de son intelligence.

## Arbitrage et sécurité

- Fenêtre de revendication : 300 ms après la première revendication d'une tâche, le centre
  accorde la revendication de coût minimal parmi celles reçues et répond aux autres (`outbid`).
- Éligibilité centrale : mêmes contrôles que le répartiteur (site, modèle requis, charge utile,
  géométrie, batterie, existence d'un trajet). Une revendication inéligible est refusée avec la
  raison.
- Une tâche accordée est épinglée au véhicule 15 s ; si l'exécution ne démarre pas (ex. perte de
  communication), elle revient sur le tableau.
- L'explication d'affectation de la mission liste les revendications concurrentes et le coût
  gagnant (`assignment.candidates`), et l'historique note « claimed by … on the colony board ».
- Le trafic, les interblocages, les pertes de communication et la garde sur la charge à bord
  restent gérés par le centre : la décentralisation porte sur le **choix des tâches**, pas sur
  la sécurité de l'espace partagé.

## Déploiement sur un AGV réel

```bash
AGV_MQTT_URL=tcp://fleet-server:1883 AGV_SITE=site-default \
AGV_VEHICLE_ID=agv-12 AGV_MANUFACTURER=inov4d AGV_SERIAL=AGV-012 \
AGV_MQTT_USERNAME=agv AGV_MQTT_PASSWORD=… ./agvagent
```

`agvagent` est un binaire Go statique (`go build ./cmd/agvagent`) qui tourne sur le PC
embarqué à côté du contrôleur VDA 5050 ; il journalise ses décisions (coût, urgence, délai,
raison) pour l'explicabilité.

## Observation

`GET /api/v1/colony/board?siteId=` renvoie les tâches ouvertes, les agents en cours de
processus (véhicules simulés) avec leurs dernières décisions, et les dernières revendications
accordées/refusées. L'onglet « Répartiteur » de l'interface affiche ce tableau en direct.
