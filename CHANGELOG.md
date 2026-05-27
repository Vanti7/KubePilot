# Changelog — KubePilot

Toutes les modifications notables de ce projet sont documentées dans ce fichier.

Format basé sur [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/).
Versioning selon [Semantic Versioning 2.0.0](https://semver.org/lang/fr/).

---

## Cycle de release

| Canal | Tag exemple | Usage |
|---|---|---|
| **rolling** | `v0.1.0-dev.20260509` | Branche `main` entre deux releases — early adopters, homelabs |
| **alpha** | `v0.1.0-alpha.1` | Fonctionnalités en cours, API instable — développement uniquement |
| **beta** | `v0.1.0-beta.1` | Feature-complete, en test — staging et contributeurs |
| **rc** | `v0.1.0-rc.1` | Release candidate, correctifs seulement — pré-production |
| **stable** | `v0.1.0` | Release validée et supportée — production |

---

## [Unreleased]

### Added
- Endpoint `GET /api/v1/namespaces` — listing des namespaces avec filtre `cluster_id` (manquait dans le router, causait des 404 sur les pages Inventory et Secrets)
- Endpoints `GET/POST/DELETE /api/v1/integrations` + `POST /api/v1/integrations/:id/test` — CRUD complet des comptes d'intégration (manquaient dans le router, causaient des 404 + crash page Integrations)
- Migration `004_finding_unique_constraint.sql` — index uniques `(cluster_id, container_image_id)` et `(cluster_id, helm_release_id)` sur `update_findings`

### Added (précédent)
- Gestion des Secrets Kubernetes — affichage des secrets avec namespace, type, noms de clés et timestamps K8s (valeurs jamais stockées)
  - Modèle `Secret` GORM avec `k8s_created_at` et `k8s_updated_at` (timestamps natifs K8s)
  - Migration SQL `002_secrets.sql` — table `secrets` avec contrainte unique `(cluster_id, namespace_name, name)`
  - Migration SQL `003_secrets_timestamps.sql` — ajout `k8s_created_at` / `k8s_updated_at` pour envs ayant déjà appliqué 002
  - Store `store/secrets.go` — `ListSecrets`, `UpsertSecret`, `DeleteSecretsNotSeenSince`
  - Handler `GET /api/v1/secrets` avec filtres `cluster_id`, `namespace`, `type`
  - Collecteur K8s — `collectSecrets` + `watchSecrets` ; `k8s_created_at` depuis `creationTimestamp`, `k8s_updated_at` depuis `max(managedFields[*].time)` ; secrets Helm exclus
  - Page frontend `/secrets` — table avec colonnes Created/Modified colorées selon l'ancienneté (vert ≤30j, jaune ≤90j, orange ≤180j, rouge >180j)
  - Lien "Secrets" dans la sidebar (icône `KeyRound`)

### Fixed
- **Bug critique** : `UpsertFinding` utilisait `ON CONFLICT (cluster_id, kind, current_version)` sans contrainte UNIQUE correspondante dans la BDD → PostgreSQL rejetait silencieusement tous les inserts, aucun finding n'était jamais créé. Corrigé : les clés de conflict sont maintenant `(cluster_id, container_image_id)` pour les findings image et `(cluster_id, helm_release_id)` pour les findings Helm
- Node handler : `capacity` et `allocatable` sont maintenant sérialisés en objets `{cpu, memory}` au lieu de champs plats — corrige le crash `TypeError: Cannot convert undefined or null to object` dans le slide-over des nœuds

### Fixed
- `backend/internal/api/middleware/auth.go` — JWT accepte désormais `?token=` query param (requis pour SSE via EventSource qui ne peut pas envoyer de headers)
- `backend/internal/collector/kubernetes.go` — le collecteur utilise désormais `rest.InClusterConfig()` (SA token + CA cert montés par le pod) quand `KubeconfigRef` est vide et `TLSInsecure` est false ; la branche `APIEndpoint` sans CA cert causait `x509: certificate signed by unknown authority` pour le cluster enregistré en bootstrap
- `frontend/src/types/index.ts` — `FindingSummary` : structure plate `{total,critical,high,...}` au lieu de `{by_severity:{...}}` inexistant dans le backend ; `OverviewData` : champs alignés sur la réponse réelle (`cluster_status`, `findings_summary`, `findings_per_cluster`, `top_findings`) ; `UpdateFinding.latest_version` au lieu de `available_version`
- `frontend/src/api/client.ts` — `getClusters()` déballe `data.data` (backend enveloppe dans `{"data":[],"total":N}`)
- `frontend/src/components/Sidebar.tsx` — accès `summary.critical` au lieu de `summary.by_severity.critical` (TypeError fatal)
- `frontend/src/pages/Overview.tsx` — accès champs overview alignés avec le backend
- `frontend/src/pages/Updates.tsx`, `ClusterDetail.tsx`, `Inventory.tsx`, `FindingDetail.tsx` — `latest_version` au lieu de `available_version`

---

## [0.1.0-alpha.2] — 2026-05-24

> Corrections du scaffold initial : routing API, compilation TypeScript, pipeline CI/CD, déploiement Helm.
> **Canal : alpha** — ne pas utiliser en production.

### Added
- `Jenkinsfile` — pipeline CI/CD : build parallèle backend/frontend, push Harbor, mise à jour GitOps via `sed`
- `VERSION` — fichier de version de base lu par Jenkins pour composer les tags semver (`v{VERSION}-alpha.{BUILD}`)

### Changed
- Helm chart déplacé du repo source vers le repo GitOps (`kubepilot-gitops/charts/kubepilot/`) — le repo GitHub ne contient plus que le code applicatif et la CI
- `frontend/nginx.conf` — URL backend hardcodée `http://backend:8080` remplacée par `${BACKEND_URL}` (résolu via envsubst au démarrage du container)
- `frontend/Dockerfile` — `nginx.conf` copié dans `/etc/nginx/templates/` pour le traitement automatique par envsubst

### Fixed
- `backend/internal/api/router.go` — routes auth déplacées sous `/api/v1` (elles étaient enregistrées à `/auth/*` alors que le frontend cible `/api/v1/auth/*`), corrige le 404 à la connexion
- `helm/kubepilot/templates/deployment.yaml` — ajout de `BACKEND_URL` en variable d'env du container frontend (injecté depuis le nom de service Helm)
- `helm/kubepilot/values.yaml` — ajout du champ `imagePullSecrets` pour Harbor (secret à créer manuellement dans chaque namespace)
- Frontend TypeScript : suppression des imports `React` inutiles dans 9 fichiers (TS6133 — `react-jsx` transform gère l'injection automatique)
- Frontend TypeScript : ajout de `src/vite-env.d.ts` (`/// <reference types="vite/client" />`) pour résoudre `import.meta.env` (TS2339)

---

## [0.1.0-alpha.1] — 2026-05-09

> Première version du scaffold MVP. Pose les fondations du produit :
> architecture hybride, backend Go complet, frontend React fonctionnel,
> déploiement Helm in-cluster, documentation initiale.
> **Canal : alpha** — ne pas utiliser en production.

### Added

#### Projet
- `CLAUDE.md` — mémoire persistante du projet pour Claude Code, conventions, état d'avancement, règles de changelog
- `CHANGELOG.md` — ce fichier, historique versionné avec cycle alpha/beta/rc/stable/rolling
- `README.md` — présentation du projet, architecture ASCII, démarrage rapide, index de la documentation
- `Makefile` — commandes `dev-infra`, `dev-backend`, `dev-frontend`, `build-*`, `test-*`, `lint-*`, `clean`
- `.gitignore` — Go, Node, secrets, kubeconfig, IDE
- `docker-compose.yml` — stack locale PostgreSQL 15 + Redis 7 + backend + frontend

#### Backend (Go 1.22)
- Module Go `github.com/kubepilot/backend` avec toutes les dépendances (Gin, GORM, client-go, Masterminds/semver, zap, redis, jwt, bcrypt, helm SDK, robfig/cron)
- **`internal/config`** — configuration complète depuis variables d'environnement (`DB_URL`, `REDIS_URL`, `JWT_SECRET`, `PORT`, `LOG_LEVEL`, `WORKER_INTERVAL_SECONDS`, `HEADLAMP_URL`, `TLS_INSECURE`, `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME`, `IN_CLUSTER`, `CLUSTER_NAME`)
- **`internal/models`** — 19 modèles GORM avec UUID (`gen_random_uuid()`), JSONB, enums : `Environment`, `Cluster`, `Namespace`, `Node`, `Workload`, `WorkloadSource`, `ContainerImage`, `ImageRegistry`, `ImageTagObservation`, `HelmRelease`, `UpdateFinding`, `RiskScore`, `Ownership`, `MaintenanceWindow`, `ActionLog`, `ExceptionRule`, `IntegrationAccount`, `User`, `UserClusterRole`
- **`migrations/001_initial.sql`** — schéma PostgreSQL complet avec extensions, types ENUM, indexes, clés étrangères, valeurs par défaut
- **`internal/store`** — couche d'accès données : `Store`, CRUD clusters/namespaces/nodes/environments, findings (list filtrée + paginée, upsert, update statut, résumé par sévérité), workloads + images, helm releases
- **`internal/bootstrap`** — bootstrap first-run idempotent : seed des 4 environnements par défaut (prod/preprod/staging/dev), création du compte admin depuis env vars ou mot de passe auto-généré affiché dans les logs, auto-enregistrement du cluster local en mode `IN_CLUSTER=true`
- **`internal/api`** — routeur Gin avec CORS, middleware JWT HS256, middleware RBAC par rôle ; 14 groupes d'endpoints REST + SSE (`/auth`, `/api/v1/clusters`, `/workloads`, `/findings`, `/nodes`, `/helm`, `/overview`, `/integrations`, `/health`, `/events`)
- **`internal/api/handlers/auth`** — Login, RefreshToken, Me, CreateUser (admin seulement), Setup (first-run), SetupStatus
- **`internal/api/handlers/findings`** — ListFindings (filtrée par cluster/severity/status/kind/namespace, paginée), GetFinding, UpdateFindingStatus (statuts : `open`, `planned`, `ignored`, `approved`, `blocked`, `resolved`), GetFindingSummary
- **`internal/api/handlers/overview`** — dashboard agrégé : counts clusters par statut, findings par sévérité, top 5 critiques, fraîcheur des données
- **`internal/api/handlers/events`** — SSE avec `EventBus` goroutine-safe, fan-out sur tous les clients connectés, keepalive 30s
- **`internal/collector/kubernetes`** — collecteur K8s par cluster : Watch API Deployments/DaemonSets/StatefulSets/Nodes/Namespaces, extraction images conteneur, décodage secrets Helm 3 (base64 + gzip + JSON), support kubeconfig ou in-cluster config
- **`internal/collector/manager`** — `CollectorManager` : réconciliation des collectors avec la DB toutes les 60s, démarrage/arrêt automatique
- **`internal/watcher/image`** — polling OCI registry (spec Distribution v2), auth Docker Hub anonyme, comparaison semver via `Masterminds/semver`, gestion tag `latest` par digest, cache Redis 30min, création `UpdateFinding`
- **`internal/watcher/helm`** — polling `index.yaml` des repos Helm, extraction dernière version stable (filtre pre-release), cache Redis 30min, création `UpdateFinding`
- **`internal/scoring`** — moteur de scoring : 7 facteurs pondérés (update_type 25%, age 20%, CVSS 20%, criticality 15%, health 10%, rollback 5%, maintenance 5%) × env_multiplier × exposure_multiplier, cap 100, mapping sévérité, stockage JSONB du détail par facteur, méthode `ScoreAll` batch
- Dockerfile multi-stage backend : `golang:1.22-alpine` → `alpine:3.19`, utilisateur non-root

#### Frontend (React 18 + TypeScript)
- Setup Vite 5, React 18, TypeScript strict, Tailwind CSS 3, Tanstack Query v5, React Router v6, Radix UI, lucide-react, axios
- Thème sombre — `surface-base: #0f1117`, `surface-panel: #1a1d27`, `surface-elevated: #252836`
- **`src/types/index.ts`** — types TypeScript complets de tous les domaines
- **`src/api/client.ts`** — client axios avec intercepteur JWT, redirect 401 vers login, fonctions typées pour tous les endpoints
- **`src/contexts/AuthContext`** — gestion JWT localStorage, login/logout, validation au montage
- **`src/contexts/ClusterContext`** — sélection multi-cluster persistée en localStorage
- **`src/components/Layout`** — sidebar fixe 220px + topbar (ClusterSelector + user menu) + zone principale scrollable
- **`src/components/Sidebar`** — navigation avec icônes, état actif, badge count critical+high sur Updates
- **`src/components/DataTable`** — table générique typée avec skeleton loading, tri, multi-select, état vide
- **`src/components/SlideOver`** — panneau latéral droit avec transition CSS, backdrop, fermeture ESC
- **`src/components/SeverityBadge`** / **`StatusBadge`** — badges colorés pour sévérité et statut de finding
- **`src/components/FindingStatusMenu`** — dropdown Radix pour changement de statut inline avec invalidation cache
- **`src/components/FindingDetail`** — contenu slide-over : score breakdown, diff de version, changelog, commande helm upgrade
- **`src/pages/Overview`** — 4 stat cards + top 5 findings critiques + table clusters avec counts par sévérité
- **`src/pages/Updates`** — triage principal : filtres multi, bulk actions, table dense avec score bar, menu actions, pagination
- **`src/pages/Inventory`** — panneau arbre namespace + table workloads avec badge updates
- **`src/pages/HelmPage`** — releases Helm avec version installée → disponible + commande upgrade
- **`src/pages/Nodes`** — table nodes avec rôle, OS, kubelet version, conditions Ready/NotReady
- **`src/pages/Integrations`** — gestion comptes d'intégration + modal d'ajout
- **`src/pages/ClusterDetail`** — détail cluster avec findings scopés
- **`src/pages/Login`** — formulaire centré dark + écran setup first-run
- **`src/hooks/useSSE`** — EventSource avec reconnect exponentiel
- **`src/utils/formatting`** — `formatAge`, `scoreToColor`, `buildHeadlampURL`, etc.
- Dockerfile frontend : Node 20 builder → nginx alpine, `nginx.conf` avec proxy `/api` + SSE `/events`

#### Helm chart (`helm/kubepilot/`)
- `Chart.yaml` — chart v0.1.0, dépendances Bitnami postgresql et redis
- `values.yaml` — configuration complète : image, frontend image, SA, RBAC, config app, admin first-run, secret, service, ingress, resources, autoscaling, postgresql intégré, redis intégré, postgresql externe, redis externe, integrationToken
- Templates : `_helpers.tpl`, `deployment.yaml` (backend + frontend), `service.yaml`, `serviceaccount.yaml`, `clusterrole.yaml` + binding, `secret.yaml` (JWT + admin + DB connection string), `ingress.yaml`, `integration-token.yaml` (SA + Secret token long-lived + ClusterRole read-only + ClusterRoleBinding), `NOTES.txt`
- `integration-token.yaml` — ServiceAccount `kubepilot-integration` avec token `kubernetes.io/service-account-token` et ClusterRole lecture seule (namespaces, nodes, pods, deployments, daemonsets, statefulsets, secrets Helm, ingresses, metrics)

#### Documentation
- `docs/README.md` — index de la documentation avec descriptions
- `docs/architecture.md` — architecture hybride, rationale, schéma ASCII, 4 flux de données, stratégie temps réel, déploiement Helm, intégration Headlamp
- `docs/data-model.md` — 19 entités : rôle, tous les champs avec types SQL, relations, indexes, usage UI ; diagramme ERD ASCII
- `docs/api-spec.md` — 14 groupes d'endpoints REST avec méthode, path, body, réponse JSON, codes d'erreur ; table des événements SSE
- `docs/scoring.md` — formule complète, tableau des facteurs et pondérations, multiplicateurs, 3 exemples chiffrés pas-à-pas, effets des ExceptionRules, gestion du tag `latest`
- `docs/development.md` — prérequis, setup docker-compose, démarrage backend/frontend, migrations, connexion cluster local, tests, variables d'env, structure des packages, Make targets
- `docs/installation.md` — 6 sections : dev local, déploiement Helm minimal et avec Ingress/TLS, BDD externe, connexion clusters distants (manifeste SA + token), token d'intégration local, référence variables d'env, dépannage (PG, collector, images, SSE, logs)

### Changed

*(aucun — première version)*

### Fixed

- Doublon `gopkg.in/yaml.v2` dans `backend/go.mod` supprimé
- Statuts de findings alignés avec la spec (`planned`, `approved`, `blocked` au lieu de `acknowledged`, `in_progress`) dans `models.go`, `store/findings.go`, `handlers/findings.go`, `migrations/001_initial.sql`
- Champs `acknowledged_at`/`acknowledged_by_id` renommés en `status_changed_at`/`status_changed_by_id` pour cohérence avec le workflow de triage

---

[Unreleased]: https://github.com/Vanti7/KubePilot/compare/v0.1.0-alpha.1...HEAD
[0.1.0-alpha.1]: https://github.com/Vanti7/KubePilot/releases/tag/v0.1.0-alpha.1
