# Déploiement et exploitation

## Développement

```bash
make run-sim          # backend :8080 + broker :1883 + 8 AGV simulés
cd frontend && npm run dev   # interface sur http://localhost:3000 (admin / admin)
```

## Variables d'environnement (`fleetd`)

| Variable | Défaut | Rôle |
|---|---|---|
| `FM_HTTP_ADDR` | `:8080` | API REST + WebSocket |
| `FM_STORE` | `memory` | `memory` (snapshot JSON dans `FM_DATA_DIR`) ou `postgres` |
| `FM_DATABASE_URL` | `postgres://fleet:fleet@localhost:5432/fleet?sslmode=disable` | DSN PostgreSQL |
| `FM_DATA_DIR` | `./data` | Dossier des données en mode mémoire |
| `FM_MQTT_EMBEDDED` | `true` | Démarre le broker MQTT embarqué |
| `FM_MQTT_ADDR` | `:1883` | Écoute du broker embarqué (`FM_MQTT_WS_ADDR` pour WebSocket) |
| `FM_MQTT_URL` | `tcp://127.0.0.1:1883` | Broker utilisé par la passerelle (`memory` = bus interne, tests) |
| `FM_MQTT_USERNAME` / `FM_MQTT_PASSWORD` | vide | Identifiants (broker embarqué : crée l'utilisateur ; sinon : client) |
| `FM_VDA_INTERFACE` | `uagv` | Premier niveau des topics VDA 5050 |
| `FM_JWT_SECRET` | généré | Secret de signature des sessions |
| `FM_TOKEN_TTL` | `12h` | Durée des sessions |
| `FM_ADMIN_PASSWORD` | `admin` | Mot de passe initial du compte `admin` (premier démarrage) |
| `FM_CORS_ORIGINS` | `*` | Origines autorisées (séparées par des virgules) |
| `FM_TICK_HZ` | `10` | Fréquence de la boucle de contrôle |
| `FM_COMMS_TIMEOUT` | `5s` | Délai de perte de communication |
| `FM_SIM_AUTOSTART` / `FM_SIM_TIMESCALE` | `0` / `1` | Simulation au démarrage |
| `FM_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## Production locale (PC industriel / serveur)

```bash
cp deploy/.env.example deploy/.env   # adapter les secrets
make up                              # docker compose : fleetd + frontend + TimescaleDB
```

Ports : 3000 (interface), 8080 (API), 1883 (MQTT AGV). Les AGV se connectent au broker avec
`FM_MQTT_USERNAME` / `FM_MQTT_PASSWORD` et publient sur `uagv/v2/<fabricant>/<série>/…`.

## Sauvegarde

- Mode mémoire : `data/fleet.json` (configuration, missions, alarmes, utilisateurs, audit).
- PostgreSQL : `pg_dump` ; la télémétrie et les métriques sont des tables append-only.

## Tests

```bash
make test-backend     # Go : unitaires + 40 AGV + bout-en-bout simulation
FM_TEST_DATABASE_URL=postgres://… make test-postgres
cd frontend && npm run lint && npm run build
```
