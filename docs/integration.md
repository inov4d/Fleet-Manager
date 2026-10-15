# Intégration industrielle (MES / WMS / ERP / automates)

## REST avec clé API

Créer une clé : `POST /api/v1/api-keys` `{"name":"erp","source":"erp","siteId":"site-default"}`
(rôle `integration` par défaut). La clé n'est affichée qu'une fois.

Envoyer une mission : `POST /api/v1/integration/missions` avec l'en-tête `X-API-Key`.
`externalRef` rend l'appel idempotent (une mission active portant la même référence est
renvoyée au lieu d'être recréée). Suivi : `GET /api/v1/integration/missions/{externalRef}`.

## MQTT

- Demandes : publier une mission JSON sur `fleet/<siteId>/missions/request`
  (`source` par défaut `plc`). Refus publiés sur `fleet/<siteId>/missions/rejected`.
- Événements : `fleet/<siteId>/events` (`mission.created|updated|completed|failed`,
  `alarm.raised|cleared`, `vehicle.connection`, `deadlock.detected|resolved`,
  `map.activated`, `simulation.*`).

## Webhooks

`POST /api/v1/integration/webhooks` `{"url":"https://mes/hook","events":["mission.completed"],"secret":"…"}`.
Chaque événement est posté en JSON avec l'en-tête `X-Fleet-Signature` (HMAC-SHA256 du corps).

## OPC UA

Contrat `integration.OPCUAAdapter` (publication des variables véhicules/missions, méthode
`RequestMission`). L'exposition serveur OPC UA est planifiée (feuille de route, phase 3) ; le
contrat permet de préparer le mapping d'espace d'adressage dès maintenant.

## Multi-sites

Toutes les entités portent `siteId`. Une clé API peut être restreinte à un site.
