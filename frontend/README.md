# Fleet Manager — Frontend

Interface web de supervision du système de gestion de flotte d’AGV omnidirectionnels (20 t / 40 t).
Construite avec **Next.js 15 (App Router)**, **React 19**, **TypeScript strict**, **Tailwind CSS v4**,
**PixiJS 8** (carte 2D temps réel), **Recharts** (graphiques), **three.js** (vue 3D) et **Zustand** (état client).

Le contrat d’API de référence est `../docs/api.md` : chaque appel, chaque type et chaque message WebSocket
du frontend en découle (`src/lib/types.ts`, `src/lib/api.ts`, `src/lib/ws.ts`).

## Scripts

| Commande            | Rôle                                                              |
|---------------------|-------------------------------------------------------------------|
| `npm install`       | Installe les dépendances (Node 22 requis).                        |
| `npm run dev`       | Serveur de développement sur http://localhost:3000.               |
| `npm run build`     | Build de production (sortie `standalone` pour Docker).            |
| `npm run start`     | Démarre le build de production.                                   |
| `npm run lint`      | ESLint (config Next + TypeScript).                                |
| `npm run typecheck` | `tsc --noEmit`.                                                   |

## Variables d’environnement

| Variable               | Défaut                      | Description                                                                                             |
|------------------------|-----------------------------|---------------------------------------------------------------------------------------------------------|
| `NEXT_PUBLIC_API_URL`  | `http://localhost:8080`     | Base du backend ; `/api/v1` est ajouté automatiquement. **Vide** ⇒ chemin relatif `/api/v1` (proxy).   |
| `NEXT_PUBLIC_WS_URL`   | `ws://localhost:8080/ws`    | URL du WebSocket temps réel. **Vide** ⇒ `ws(s)://<hôte courant>/ws`.                                    |
| `BACKEND_URL`          | `http://localhost:8080`     | Cible des *rewrites* de développement (`/api/*` → backend) définis dans `next.config.ts`.               |

Les variables `NEXT_PUBLIC_*` sont figées **au build** (voir `.env.example`). En développement, laisser
`NEXT_PUBLIC_API_URL=` vide permet d’appeler `/api/v1` en relatif, proxifié vers le backend par Next
(les WebSockets ne sont pas proxifiés : garder `NEXT_PUBLIC_WS_URL` pointant sur le backend).

## Docker

```bash
docker build -t fleet-manager-frontend \
  --build-arg NEXT_PUBLIC_API_URL= \
  --build-arg NEXT_PUBLIC_WS_URL=wss://fleet.example.com/ws .
docker run -p 3000:3000 fleet-manager-frontend
```

L’image (multi-étapes, `node:22-alpine`) ne contient que la sortie `standalone` de Next.js.

## Authentification et rôles

* `/login` → `POST /auth/login` ; le jeton et l’utilisateur sont conservés dans `localStorage` (store `fm-auth`).
* Toute réponse **401** déconnecte et renvoie vers `/login`.
* Les droits d’affichage sont calculés par `can(user, permission)` (`src/lib/auth.ts`) en miroir du résumé des
  rôles du contrat : `viewer` < `operator` < `supervisor` < `admin` ; `integration` = création de missions.

## Temps réel

`src/lib/ws.ts` ouvre `GET /ws?token=…`, envoie `{"type":"subscribe","siteId"}` à la connexion et à chaque
changement de site, se reconnecte avec un backoff exponentiel (1 s → 30 s) et pousse :

* les **snapshots** (~4 Hz) dans `src/stores/fleet.ts` (véhicules, missions, réservations, alarmes, KPI, simulation) ;
* les **événements** dans `src/stores/events.ts` (500 max) avec un toast pour `alarm.raised`, `deadlock.detected`,
  `mission.failed` et un rechargement de la carte active sur `map.activated`.

La carte PixiJS (`src/components/map/map-renderer.ts`) s’abonne directement au store (sans re-rendu React)
et redessine les couches dynamiques à chaque snapshot ; les coordonnées carte (mètres, y vers le haut) sont
converties en écran par inversion de l’axe y.

## Structure

```
frontend/
├── Dockerfile, .dockerignore, .env.example
├── next.config.ts            # output standalone + rewrites /api/* → backend (dev)
├── src/
│   ├── app/
│   │   ├── layout.tsx        # racine (thème avant peinture)
│   │   ├── login/            # page de connexion
│   │   └── (app)/            # pages authentifiées (sidebar + barre supérieure)
│   │       ├── page.tsx      # Tableau de bord
│   │       ├── map/          # Carte en direct (PixiJS)
│   │       ├── vehicles/     # liste, [id] détail, new
│   │       ├── missions/     # liste, [id] détail, new
│   │       ├── map-editor/   # Éditeur de carte (SVG)
│   │       ├── traffic/ alarms/ maintenance/ analytics/ simulation/ settings/ 3d/
│   ├── components/
│   │   ├── ui/               # boutons, champs, dialogues, tableaux, badges, onglets, toasts
│   │   ├── layout/           # app-shell, sidebar, topbar
│   │   ├── map/              # map-renderer (Pixi), live-map, panneau véhicule, légende
│   │   ├── editor/           # canvas SVG, panneau de propriétés, liste des cartes
│   │   ├── charts/           # graphiques Recharts
│   │   ├── vehicles/ missions/ alarms/ settings/ three/
│   ├── hooks/                # use-api, use-active-map, use-node-names, use-now
│   ├── lib/                  # api.ts, ws.ts, types.ts, auth.ts, labels.ts, format.ts, utils.ts
│   └── stores/               # auth, site, fleet, events, ui (zustand)
```

## Accessibilité et ergonomie

Thème sombre par défaut avec bascule claire, styles de focus clavier, libellés ARIA sur les boutons-icônes,
tableaux sémantiques, onglets navigables au clavier, dialogues natifs `<dialog>`. Conçu pour un écran de
salle de contrôle 1920×1080 et utilisable en largeur tablette (barre latérale repliable).
