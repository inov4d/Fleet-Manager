# Cybersécurité

- Authentification : utilisateurs locaux (bcrypt), sessions JWT HS256 (`FM_JWT_SECRET`,
  généré et persisté s'il est absent), durée `FM_TOKEN_TTL` (12 h).
- Autorisation : rôles `viewer`, `operator`, `supervisor`, `admin`, `integration` ; matrice
  dans `internal/auth/auth.go`, vérifiée côté serveur sur chaque route et reflétée dans l'UI.
- Clés API hachées (SHA-256) en base ; révocables.
- Journal d'audit de chaque action modifiante (utilisateur, action, cible, IP).
- MQTT : broker embarqué avec authentification optionnelle (`FM_MQTT_USERNAME/PASSWORD`) ;
  en production, utiliser un broker TLS externe (`FM_MQTT_URL=ssl://…`) et des identifiants par
  véhicule, et isoler le réseau AGV (VLAN).
- HTTP : placer un reverse proxy TLS devant `:8080` ; restreindre `FM_CORS_ORIGINS` aux origines
  de l'interface.
- Fonctionnement hors ligne : aucune dépendance Internet (polices et bibliothèques embarquées
  dans le build Next.js, broker et base locaux).
- Les commandes réseau ne sont jamais considérées comme des arrêts de sécurité ; la sécurité
  fonctionnelle reste embarquée.
- À faire avant mise en production (voir feuille de route) : rotation des secrets, politiques
  de mots de passe, verrouillage après échecs, SSO/LDAP, durcissement des conteneurs, revue
  des dépendances (`npm audit`, `govulncheck`).
