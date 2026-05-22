# CLAUDE.md — KubePilot project memory

This file is the persistent memory for Claude Code working on this repository.
Read it at the start of every session. Update it whenever something notable changes.

---

## Règle obligatoire — Changelog

**Chaque changement notable doit être documenté dans `CHANGELOG.md`.**

Un changement est notable s'il touche :
- une fonctionnalité utilisateur (ajout, modification, suppression)
- le schéma de base de données (migration)
- une API publique (endpoint ajouté, modifié, supprimé, breaking change)
- une dépendance majeure (ajout, mise à jour majeure, suppression)
- la configuration (nouvelle variable d'env, nouveau paramètre Helm)
- la sécurité (correctif de vulnérabilité, changement auth/RBAC)
- le déploiement (Helm chart, Dockerfile, docker-compose)

Ne pas documenter : refactoring interne sans impact fonctionnel, reformatage de code, corrections de typos dans les commentaires.

### Format d'entrée changelog

Toujours ajouter sous la section `[Unreleased]` en haut du CHANGELOG.md, dans la sous-section appropriée (`Added`, `Changed`, `Fixed`, `Removed`, `Security`, `Deprecated`).

Exemple :
```markdown
## [Unreleased]

### Added
- Endpoint `GET /api/v1/findings/export` — export CSV des findings ouverts
- Variable d'env `EXPORT_MAX_ROWS` (défaut : 10000)
```

Lors d'une release (stable ou pre-release), transformer `[Unreleased]` en version datée et créer un nouveau `[Unreleased]` vide au-dessus.

---

## Versioning sémantique

KubePilot suit [Semantic Versioning 2.0.0](https://semver.org/lang/fr/) avec la convention de cycle suivante :

### Format de version

```
MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]
```

| Segment | Règle |
|---|---|
| `MAJOR` | Changement incompatible de l'API publique ou du schéma de données |
| `MINOR` | Nouvelle fonctionnalité rétrocompatible |
| `PATCH` | Correction de bug rétrocompatible |
| `PRERELEASE` | Identifiant de cycle (voir tableau ci-dessous) |

### Cycle de release

| Tag | Exemple | Signification | Stabilité |
|---|---|---|---|
| `alpha` | `v0.2.0-alpha.1` | Fonctionnalité en cours, API instable, données peuvent être perdues | Développement uniquement |
| `beta` | `v0.2.0-beta.1` | Fonctionnalité complète, tests en cours, migrations possibles | Test / staging |
| `rc` | `v0.2.0-rc.1` | Release candidate, plus de nouvelles features, correctifs seulement | Pré-production |
| *(aucun)* | `v0.2.0` | **Stable** — validée, supportée, recommandée pour la production | Production |

### Canal rolling

Le canal **rolling** correspond à la branche `dev` / `main` entre deux releases stables. Il porte le tag `v0.x.y-dev.YYYYMMDD` (ex: `v0.1.0-dev.20260509`). Les homelabs et early adopters peuvent utiliser ce canal à leurs risques.

### Règles de bump

- Bug fix sur une release stable → `PATCH` (ex: `v0.1.0` → `v0.1.1`)
- Nouvelle feature MVP ou V2 → `MINOR` (ex: `v0.1.x` → `v0.2.0`)
- Breaking change API ou migration destructive → `MAJOR` (ex: `v0.x.y` → `v1.0.0`)
- Itération sur une pre-release → incrémenter le numéro de pre-release (ex: `alpha.1` → `alpha.2`)
- Promotion alpha → beta → rc → stable : changer l'identifiant, remettre le compteur à 1

---

## État actuel du projet

**Version courante** : `v0.1.0-alpha.1` (2026-05-09)
**Canal** : alpha
**Branche principale** : `main`

### Périmètre livré (MVP scaffold)

#### Backend (Go 1.22)
- [x] Structure du module Go (`cmd/server`, `internal/{api,bootstrap,collector,config,models,scoring,store,watcher}`)
- [x] Configuration depuis variables d'environnement (`internal/config`)
- [x] 19 modèles GORM avec UUID, JSONB, enums (`internal/models`)
- [x] Migration SQL initiale (`migrations/001_initial.sql`)
- [x] Store — CRUD clusters, findings, workloads, helm releases, nodes (`internal/store`)
- [x] API REST Gin — 14 groupes d'endpoints + SSE (`internal/api`)
- [x] Middleware JWT auth + RBAC par rôle (`internal/api/middleware`)
- [x] Bootstrap first-run — seed environments, création admin, auto-enregistrement cluster local (`internal/bootstrap`)
- [x] K8s Collector — Watch API Deployments/DaemonSets/StatefulSets/Nodes/Namespaces + décodage secrets Helm (`internal/collector`)
- [x] Image Watcher — polling OCI registry, semver comparison, cache Redis (`internal/watcher/image.go`)
- [x] Helm Watcher — polling index.yaml, version stable latest, cache Redis (`internal/watcher/helm.go`)
- [x] Scoring Engine — 7 facteurs pondérés × env multiplier × exposure multiplier (`internal/scoring`)
- [x] Endpoints auth : `/auth/login`, `/auth/setup`, `/auth/refresh`, `/auth/me`
- [x] Événements SSE avec EventBus goroutine-safe
- [x] Graceful shutdown SIGTERM/SIGINT
- [x] Dockerfile multi-stage (golang:1.22-alpine → alpine:3.19)

#### Frontend (React 18 + TypeScript)
- [x] Setup Vite + Tailwind CSS + Tanstack Query + React Router v6
- [x] Thème sombre (`surface-base: #0f1117`, `surface-panel: #1a1d27`)
- [x] AuthContext (JWT localStorage) + ClusterContext (multi-select persisté)
- [x] Layout avec sidebar fixe 220px + topbar cluster selector
- [x] Page Overview — stat cards + top findings + cluster status table
- [x] Page Updates — table de triage avec filtres, bulk actions, slide-over detail
- [x] Page Inventory — arbre namespace + table workloads
- [x] Page Helm — releases avec version installée vs disponible
- [x] Page Nodes — table nodes avec statut conditions
- [x] Page Integrations — gestion des comptes d'intégration
- [x] Page ClusterDetail
- [x] Page Login + écran setup first-run
- [x] Composants : DataTable, SlideOver, SeverityBadge, StatusBadge, FindingDetail, FindingStatusMenu, ClusterSelector
- [x] Hooks : useClusters, useFindings, useSSE (EventSource avec reconnect exponentiel)
- [x] Client API axios typé (`src/api/client.ts`)
- [x] Types TypeScript complets (`src/types/index.ts`)
- [x] Formatters utilitaires (`src/utils/formatting.ts`)

#### Infrastructure
- [x] `docker-compose.yml` — PostgreSQL 15 + Redis 7 + backend + frontend
- [x] Helm chart (`helm/kubepilot/`) — deployment, service, SA, RBAC, ClusterRole, secret, ingress, integration-token, NOTES.txt
- [x] `Makefile` — dev-infra, dev-backend, dev-frontend, test, lint, build, clean
- [x] `.gitignore`

#### Documentation
- [x] `README.md` racine
- [x] `docs/README.md` — index documentation
- [x] `docs/architecture.md` — architecture hybride, schéma ASCII, flux de données
- [x] `docs/data-model.md` — 19 entités avec attributs, relations, index
- [x] `docs/api-spec.md` — 14 groupes d'endpoints REST + SSE
- [x] `docs/scoring.md` — formule, facteurs, exemples chiffrés
- [x] `docs/development.md` — guide développeur
- [x] `docs/installation.md` — procédure complète (local, Helm, multi-cluster)
- [x] `CHANGELOG.md` — historique versionné
- [x] `CLAUDE.md` — ce fichier

### Ce qui manque avant la première beta

- [ ] `go.sum` généré (nécessite `go mod tidy` dans `backend/`)
- [ ] Tests unitaires backend (scoring engine, semver comparison, store)
- [ ] Tests d'intégration frontend (Playwright)
- [ ] Seed data pour démo / développement local
- [ ] Page Settings (gestion utilisateurs, variables globales)
- [ ] Gestion des registries privés dans l'UI (Harbor, ECR, GCR, ACR)
- [ ] Page History (audit log des actions)
- [ ] Vérification que le code compile sans erreurs (`go build ./...`)

---

## Architecture — décisions clés

### Statuts de findings
Les statuts valides sont : `open`, `planned`, `ignored`, `approved`, `blocked`, `resolved`.
Ne **pas** utiliser `acknowledged` ou `in_progress` — ces valeurs ont été remplacées.

### Clés primaires
Toutes les entités utilisent des UUID v4 générés par PostgreSQL (`gen_random_uuid()`).
Ne pas utiliser d'entiers auto-incrémentés.

### Enums
Les enums sont des constantes Go (`const`) côté backend et des union types TypeScript côté frontend.
Ils ne sont **pas** des types ENUM PostgreSQL natifs (sauf dans la migration SQL initiale pour compatibilité) — GORM les stocke comme TEXT pour la flexibilité des migrations.

### JSONB
Les champs flexibles (labels, annotations, conditions, factors, config) sont stockés en JSONB.
Utiliser `gorm.io/datatypes.JSON` côté Go, `Record<string, any>` côté TypeScript.

### Auth
- JWT HS256, durée 24h, refresh token sans rotation en MVP.
- Endpoint `/auth/setup` : actif uniquement quand `users` table est vide.
- Rôles : `admin` > `operator` > `viewer`.
- **Pas d'OIDC en MVP** — prévu en V2 via Dex.

### Scoring
Score = Σ(facteur × poids) × env_multiplier × exposure_multiplier, cap à 100.
Seuils : Critical ≥ 80, High 60-79, Medium 40-59, Low 20-39, Info < 20.
Voir `docs/scoring.md` pour la formule complète.

### In-cluster vs dev local
- En production (Helm) : `IN_CLUSTER=true`, le pod utilise son SA token.
- En dev local : `IN_CLUSTER=false`, fournir un kubeconfig via l'UI ou l'API.

---

## Roadmap

### V2 (prochaine minor)
- CVE scanning via Trivy/Grype, intégration dans le score
- Plugin Headlamp actif (widget Update Status)
- Maintenance windows (cron + planification)
- Notifications (Slack, webhook, email digest)
- RBAC multi-rôles par cluster/namespace
- Registries privés complets (Harbor, ECR, GCR, ACR)
- Intégration Argo CD (lecture seule)
- Inventaire OS/packages nœuds (DaemonSet agent ou SSH)
- OIDC via Dex

### V3 (future major)
- Génération PR/MR GitOps pour mise à jour des values Helm
- Déclenchement playbook Ansible (AWX/Semaphore)
- Inventaire Proxmox / Docker standalone
- Multi-tenant (organisations, équipes, périmètres isolés)
- SLA tracking des délais de mise à jour
- API publique versionnée

---

## Conventions de code

### Go
- Pas de commentaires redondants — seulement si le WHY est non-évident.
- Erreurs wrappées avec `fmt.Errorf("contexte: %w", err)`.
- Logging structuré avec `zap.Logger` — pas de `log.Printf`.
- Context propagé dans toutes les fonctions d'accès DB et réseau.
- Upserts GORM avec `clause.OnConflict` — jamais de "delete + insert".

### TypeScript/React
- Composants fonctionnels uniquement, pas de classes.
- Types explicites — pas de `any` sauf pour les payloads JSONB documentés.
- `useQuery` de Tanstack Query pour tous les appels API — pas de `useEffect` + `fetch`.
- Tailwind classes directement dans le JSX — pas de fichiers CSS séparés sauf `index.css`.

### Commits
Format : `type(scope): message` (Conventional Commits)
Types : `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `ci`
Exemples :
```
feat(scoring): add CVSS factor to risk score computation
fix(collector): reconnect K8s Watch on 410 Gone response
docs(installation): add Traefik SSE configuration example
chore(deps): bump client-go to v0.30.0
```
