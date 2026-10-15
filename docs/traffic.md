# Gestion du trafic et réservations

## Représentation

- **Topologie** : nœuds, arêtes (mono ou bidirectionnelles, vitesse, type de mouvement
  autorisé, largeur de couloir, facteur de coût), zones polygonales typées
  (`exclusive`, `singleLane`, `bidirectional`, `crossing`, `waiting`, `forbidden`,
  `speedLimit`, `charging`) avec capacité.
- **Géométrie réelle** : chaque véhicule a un gabarit effectif
  = max(corps, charge) + 2 × (marge opérationnelle + incertitude de localisation).
  Pour un déplacement d'un nœud à l'autre, le Traffic Manager calcule **l'enveloppe balayée**
  de la manœuvre (translation + rotation combinées, échantillonnée puis enveloppe convexe
  gonflée de la flèche d'arc), pas seulement le centre du véhicule.
- **Orientation de parcours** : `mapmgr.TravelHeading` choisit l'orientation du corps selon le
  type de mouvement de l'arête (`forward` : aligné ou en marche arrière, `lateral` : cap
  conservé, `any` : la rotation minimale parmi les quatre alignements). La même règle est
  utilisée par le simulateur, le constructeur d'ordres (champ `orientation` GLOBAL de l'arête
  VDA 5050) et le Traffic Manager, ce qui garantit la cohérence.

## Réservations

Une réservation = { propriétaire, ressource (nœud/arête/zone), état, séquence, horodatages }.

États : `requested` → `granted` (segment accordé) → `active` (le véhicule y est physiquement)
→ `released`. `uncertain` quand la communication est perdue.

Règles :

1. Un segment (arête + nœud d'arrivée + zones touchées par l'enveloppe balayée) n'est accordé
   que si chaque ressource reste sous sa capacité et si l'enveloppe ne coupe ni le corps d'un
   autre véhicule ni une manœuvre déjà accordée.
2. Les zones `singleLane` imposent un sens de circulation commun aux véhicules présents.
3. Les segments sont accordés dans l'ordre jusqu'au premier refus, dans la limite de la
   fenêtre d'anticipation (`lookaheadSegments`). Les nœuds accordés deviennent la **base
   libérée** de l'ordre VDA 5050 ; le reste est envoyé comme **horizon** non libéré.
4. Une ressource n'est libérée que lorsque la position rapportée du véhicule montre qu'il l'a
   quittée (nœud atteint et corps hors de la zone). **Aucune libération sur expiration de
   délai.**
5. Perte de communication : toutes les réservations du véhicule passent `uncertain`, aucune
   nouvelle mission ne lui est attribuée, les autres véhicules gardent une distance
   supplémentaire de son dernier corps connu. Seule une libération manuelle après vérification
   physique (`POST /traffic/reservations/{id}/release` avec `confirmPhysicalState`) peut les
   lever ; le retour de la communication les restaure.
6. Une zone désactivée (temporairement indisponible) refuse tout nouvel accord et est évitée
   par le planificateur ; les véhicules qui y sont restent propriétaires de leur position.

## Blocages et interblocages

- **Véhicule bloqué** : attente d'une ressource ou absence de progression sur un segment accordé
  au-delà de `blockedTimeoutS` → alarme `VEHICLE_BLOCKED`, état `blocked`.
- **Replanification** : après `ReplanAfter` (8 s par défaut) d'attente sur une ressource, la
  route restante est recalculée en évitant la ressource contestée, avec coût de congestion
  (réservations des autres véhicules). La replanification n'est envoyée qu'au véhicule à
  l'arrêt en fin de base (un nouvel ordre VDA 5050 doit commencer là où se trouve l'AGV).
- **Interblocage** : cycle dans le graphe d'attente (A attend B qui attend A…). Le véhicule
  de plus faible priorité est replanifié en évitant les ressources du cycle ; sans alternative,
  il se replie vers un nœud libre (`waiting`/`parking`/`waypoint`) proche avant de reprendre.
  Chaque interblocage est journalisé avec sa résolution.

## Sécurité

Les réservations coordonnent le trafic ; elles ne sont **pas** une fonction de sécurité. Les
distances de sécurité, champs laser et arrêts relèvent de l'analyse de risques et des
dispositifs embarqués. Le Fleet Manager ne considère jamais qu'une commande réseau constitue
un arrêt de sécurité.
