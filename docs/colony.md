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
| `fleet/<site>/board/conflicts/<vehicleId>` (retenu) | centre → agent | `{"vehicleId","resource":{type,id},"heldBy":[],"since","active"}` |
| `fleet/<site>/agents/<vehicleId>/inbox` | agent → agent | `Proposal {"nonce","from","to","resource","yieldCost","waitedS","at"}` ou `Answer {"nonce","from","to","yieldCost","iYield"}` |
| `fleet/<site>/board/yields` | agent → centre | `{"nonce","vehicleId","beneficiary","resource","myCost","peerCost","reason","at"}` |

L'agent lit sa propre position sur le topic VDA 5050 `state` de son contrôleur. Toute
implémentation respectant ces messages (Python, ROS 2, automate…) peut remplacer l'agent de
référence : le centre ne suppose rien de son intelligence.

## Négociation locale des cessions de passage

Quand un agent attend une ressource (nœud, arête, zone) tenue par un voisin, le centre lui
publie seulement *qui* le bloque (`fleet/<site>/board/conflicts/<vehicleId>`, retenu). Les
deux agents règlent alors le conflit **entre eux**, pair à pair :

1. Après `NegotiateAfter` (4 s) d'attente, l'agent bloqué envoie une **proposition** dans la
   boîte du voisin (`fleet/<site>/agents/<vehicleId>/inbox`) avec son propre **coût de cession** :
   `0` s'il est inactif ; sinon `10 + urgence de sa tâche + 20 si une charge est à bord`, minoré
   par le temps déjà attendu (un agent qui a longtemps attendu cède moins volontiers).
2. Le voisin répond avec son propre coût. Les deux appliquent la **même règle déterministe**
   (`DecideYield`) : le moins coûteux cède, égalité tranchée par l'identifiant. Aucun arbitre
   n'est nécessaire ; chacun journalise la négociation (`negotiations` dans son instantané).
3. Celui qui cède publie la concession sur `fleet/<site>/board/yields`. Le centre l'exécute en
   sécurité : un agent au travail recule ou se replanifie en évitant le chemin du bénéficiaire
   (mécanisme de repli existant, réservations toujours vérifiées) ; un agent inactif reçoit une
   mission « pas de côté » vers le nœud libre le plus proche hors du chemin du bénéficiaire.
4. En mode colonie, le centre **laisse 30 s** à la négociation locale avant d'appliquer sa
   propre résolution d'interblocage ; une cession négociée clôt l'interblocage correspondant
   (« negotiated: X gives way to Y »). Un voisin qui n'est pas un agent (ou injoignable) ne
   répond pas : la proposition expire en 3 s et le centre reprend la main au bout des 30 s.

Les cessions apparaissent dans l'onglet Répartiteur (« Cessions de passage négociées ») et
dans `GET /colony/board` (`stats.negotiated`, `stats.yields`, `agents[].negotiations`).

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
- Le trafic, les pertes de communication et la garde sur la charge à bord restent gérés par le
  centre : la décentralisation porte sur le **choix des tâches** et sur **qui cède le passage**,
  pas sur la sécurité de l'espace partagé (les réservations restent l'autorité unique).

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
