# CLAUDE.md — KubePilot project memory

This file is the persistent memory for Claude Code working on this repository.
Read it at the start of every session. Update it whenever something notable changes.

> 📋 **Suivi des tâches & séquencement** : voir [`docs/workflow.md`](docs/workflow.md) — tableau de bord vivant (à mettre à jour à chaque session), maintenu en complément de ce fichier et du `CHANGELOG.md`.

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

**Version courante** : `v0.2.0-alpha.8` (2026-08-06)
**Canal** : alpha
**Branche principale** : `main`

### Périmètre livré (MVP scaffold)

#### Backend (Go 1.22)
- [x] Structure du module Go (`cmd/server`, `internal/{api,bootstrap,collector,config,models,scoring,store,watcher}`)
- [x] Configuration depuis variables d'environnement (`internal/config`)
- [x] 19 modèles GORM avec UUID, JSONB, enums (`internal/models`)
- [x] Migration SQL initiale (`migrations/001_initial.sql`)
- [x] Store — CRUD clusters, findings, workloads, helm releases, nodes (`internal/store`)
- [x] API REST Gin — 15 groupes d'endpoints + SSE (`internal/api`)
- [x] Middleware JWT auth + RBAC par rôle (`internal/api/middleware`)
- [x] Piste d'audit `action_logs` interrogeable (`GET /api/v1/action-logs`) + helper `handlers.RecordAction`
- [x] **Scale / rolling-restart** (`PATCH /api/v1/workloads/:id/scale`, `POST /api/v1/workloads/:id/restart`) — première action d'écriture sur un cluster, package `internal/k8sops`, RBAC in-cluster mis à jour (`kubepilot-gitops`)
- [x] **Helm upgrade/rollback réels** (`POST /api/v1/helm/:id/upgrade`, `.../rollback`) — SDK `helm.sh/helm/v3`, package `internal/helmops`, RBAC `cluster-admin` (`rbac.helmAdmin`, `kubepilot-gitops`)
- [x] **Deploy d'un manifest (server-side apply)** (`POST /api/v1/clusters/:id/manifests/apply`) — dernière brique du Pilotage MVP. Package `internal/k8sops/manifest.go` (dynamic client + RESTMapper depuis `CollectorManager.GetRESTConfig`), aucune nouvelle dépendance, aucune migration, aucun changement RBAC (réutilise le binding `cluster-admin` déjà accordé pour Helm)
- [x] **Updates : remédiation réelle** (`POST /api/v1/findings/:id/remediate`) — bump d'image (`k8sops.SetContainerImage`, strategic merge patch) ou upgrade Helm (`helmops.UpgradeChart`) selon le type de finding, plus re-vérification immédiate en tâche de fond (`ImageChecker`/`HelmChecker` sur les watchers existants) pour une résolution en secondes plutôt qu'en minutes
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
- [x] Helm chart — **hébergé dans le dépôt `kubepilot-gitops`** (`charts/kubepilot/`), pas ici (voir « Déploiement » plus bas)
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

- [x] `go.sum` généré (`go mod tidy` — fait, v0.2.0-alpha.1)
- [x] Vérification que le code compile sans erreurs (`go build ./...` + `go vet` OK)
- [x] **Accès registries effectif → findings réels** (20 findings image + 4 findings Helm sur un cluster réel)
- [x] Gestion des registries privés dans l'UI (Harbor, ECR, GCR, ACR) + dépôts de charts Helm
- [x] Nettoyage en cascade des `container_images` orphelines au `DeleteWorkloadsNotSeenSince`
- [x] Déploiement in-cluster opérationnel : Jenkins → Harbor → ArgoCD (voir `docs/installation.md` §2)
- [~] Tests unitaires backend : comparaison semver (`internal/watcher/tags_test.go`), scale/restart/deploy manifest/set-image (`internal/k8sops`), SSRF guard (`internal/netguard`), scoring engine (`internal/scoring/engine_test.go` — formule complète, placeholder fenêtre de maintenance épinglé), findings et upsert workload (`internal/store`, dont un test de non-régression direct sur le bug UUID fantôme du 2026-08-01) tous testés ; reste le reste du `store` (clusters, helm releases/repos, registries, namespaces, métriques, secrets)
- [ ] Tests d'intégration frontend (Playwright)
- [x] Seed data pour démo / développement local — mode démo (`DEMO_MODE=true`), voir « Périmètre livré »
- [x] Page Settings (gestion utilisateurs, infos système) — voir « Périmètre livré »
- [ ] Page History (audit log des actions) — `GET /api/v1/action-logs` existe déjà côté API, pas encore de page dédiée

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
- **`JWT_SECRET` sans valeur codée en dur** : si la variable d'env est absente, un secret aléatoire de 32 octets est généré à chaque démarrage (`config.generateRandomSecret`, `crypto/rand`) — un défaut fixe (`"change-me-in-production"`) aurait permis de forger un token admin contre n'importe quelle instance non configurée. Effet de bord assumé et loggé (`logger.Warn` au boot) : les sessions ne survivent pas à un redémarrage tant que `JWT_SECRET` n'est pas fixé explicitement.
- **`JWTAuth` revalide `role`/`is_active` en base à chaque requête** (`store.ActiveUserRole`, interface `middleware.ActiveUserRole`) — les claims du JWT ne sont plus la source de vérité pour le rôle. Sans ça, désactiver ou rétrograder un compte via `PATCH /auth/users/:id` n'aurait aucun effet avant l'expiration naturelle du token (jusqu'à 24h). Coût : un lookup DB par requête authentifiée, jugé acceptable à l'échelle homelab visée.

### SSRF — requêtes sortantes non authentifiées (depuis Unreleased)
- **`internal/netguard`** : client HTTP partagé pour toute requête sortante construite depuis une donnée que KubePilot ne contrôle pas totalement — registre/repository lu dans une spec de workload (`watcher/image.go`), URL de dépôt renvoyée par la recherche Artifact Hub (`watcher/helm_resolve.go`), URL de dépôt Helm saisie par un opérateur (`handlers/helm_repositories.go`, exposition moindre). Les deux premiers tournent sur une boucle temporisée **sans utilisateur authentifié dans la chaîne** — le point d'entrée SSRF n'a besoin d'aucune session KubePilot.
- Le dialer (`net.Dialer.Control`) refuse loopback/link-local/multicast **après résolution DNS**, sur l'IP réellement composée — résiste au DNS rebinding qu'un simple filtre sur le hostname ne bloquerait pas. Ça couvre `169.254.169.254` (métadonnées cloud AWS/GCP/Azure/OCI).
- **RFC1918 (`10/8`, `172.16/12`, `192.168/16`) reste volontairement autorisé** : les registries/dépôts de charts auto-hébergés sur un réseau privé sont l'usage principal de l'outil (Harbor à `10.0.60.152`), pas un cas à bloquer. Toute nouvelle requête sortante construite depuis une donnée externe doit passer par `netguard.NewHTTPClient`, jamais un `http.Client{}` nu.

### Exposition de secrets via l'API (depuis Unreleased)
- **`Values` d'une Helm release** (`GET /helm`, `GET /helm/:id`) : masquées (`nil`) pour tout rôle sous `operator` — elles contiennent couramment des mots de passe/clés d'API, et seul `operator`/`admin` peut de toute façon déclencher un upgrade qui en aurait l'usage.
- **`url` d'une intégration** (`GET /integrations`) : retirée de la réponse pour tous les rôles, remplacée par `has_url: bool` — pour Slack/PagerDuty/Teams, cette URL **est** le jeton d'authentification, pas un simple identifiant (même traitement que les identifiants de dépôt Helm : jamais renvoyés, seulement `has_credentials`).

### Scoring
Score = Σ(facteur × poids) × env_multiplier × exposure_multiplier, cap à 100.
Seuils : Critical ≥ 80, High 60-79, Medium 40-59, Low 20-39, Info < 20.
Voir `docs/scoring.md` pour la formule complète.

### In-cluster vs dev local
- En production (Helm) : `IN_CLUSTER=true`, le pod utilise son SA token.
- En dev local : `IN_CLUSTER=false`, fournir un kubeconfig via l'UI ou l'API.

### Stockage pluggable (depuis Unreleased)
- Drivers sélectionnables : `STORAGE_DRIVER` (`postgres`|`sqlite`) et `CACHE_DRIVER` (`redis`|`memory`).
- **`LOCAL_MODE=true`** = raccourci single-binary : SQLite (`SQLITE_PATH`, défaut `kubepilot.db`) + cache mémoire + auto-enregistrement du cluster depuis le kubeconfig (`KUBECONFIG_PATH`). Aucun PostgreSQL/Redis requis.
- Défauts : `DB_URL` vide ou `LOCAL_MODE=true` → SQLite + mémoire ; sinon PostgreSQL + Redis.
- **UUID** : assignés côté Go via un callback GORM `BeforeCreate` (`store/open.go`), pas via `gen_random_uuid()`. Ne **pas** réintroduire `default:gen_random_uuid()` dans les tags des modèles — ça casse SQLite. Les `migrations/*.sql` restent PostgreSQL-only (non exécutées par `AutoMigrate`).
- Le cache passe par l'interface `store.Cache` (`store/cache.go`). Ne plus utiliser `store.Redis` directement.

### Connexion cluster par SSH (depuis Unreleased)
- `Cluster.ConnectionMode == "ssh"` (ou `SSHHost` non vide) : le collector (`collector/ssh.go`) ouvre une session SSH (auth mot de passe), lit le kubeconfig du nœud (`cat`/`sudo -S cat`), construit le `rest.Config` depuis ce kubeconfig, puis **route `rest.Config.Dial` à travers le client SSH** (`sshTunnelDialer`) — donc l'API server `127.0.0.1:6443` du nœud est joint via le tunnel.
- Le `*ssh.Client` vit sur le `KubernetesCollector` et est fermé dans `Stop()`.
- Configuré en mode local par `SSH_HOST`/`SSH_USER`/`SSH_PASSWORD`/`SSH_PORT`/`SSH_KUBECONFIG_PATH`/`SSH_SUDO` ; le bootstrap enregistre alors le cluster en mode ssh.
- Clé d'hôte non épinglée (`InsecureIgnoreHostKey`) en alpha — à durcir (known_hosts) avant prod. `ssh_password` a le tag `json:"-"`.

### Déploiement — dépôt GitOps séparé
- **Le chart Helm ne vit PAS dans ce dépôt.** Il a été déplacé dans `kubepilot-gitops` (`charts/kubepilot/`) en mai 2026 (commit `070b901`), avec `envs/<env>/values.yaml` et les Applications ArgoCD. Le Jenkinsfile y est aussi (`ci/Jenkinsfile`, déplacé par `95a0ce0`). **Ne jamais recréer de `helm/` ici** — deux charts divergeraient.
- Chaîne : push `dev`/`staging` → Jenkins build back+front en parallèle → push Harbor `harbor.<domaine>/kubepilot/{backend,frontend}:v<VERSION>-alpha|beta.<BUILD>` → `sed` du tag dans le dépôt GitOps → ArgoCD sync. Le fichier `VERSION` à la racine fournit le `MAJOR.MINOR.PATCH`.
- **`nodes/proxy` est obligatoire** pour les métriques nœuds : le collector lit le Summary API du kubelet via le proxy de l'API server, **pas** `metrics.k8s.io`. Sans ce droit les jauges restent vides sans erreur.

### Origine des charts Helm (depuis Unreleased)
- Un secret de release Helm 3 contient le nom et la version du chart mais **pas son dépôt**. `HelmRepository` (dépôts configurables + 11 publics semés au premier démarrage) sert à retrouver l'origine.
- Un dépôt n'est retenu que si son `index.yaml` contient **exactement la version installée** — sans cette confirmation, `harbor` de Bitnami serait confondu avec celui de goharbor. Repli Artifact Hub via `HELM_AUTODISCOVER` (défaut `true`).
- Ne **jamais** remettre `repo_url` dans les `DoUpdates` de `UpsertHelmRelease` : le collector ne connaît pas cette valeur et l'écraserait à chaque passe.

### Comparaison de tags d'images (depuis Unreleased)
- `internal/watcher/tags.go` : un tag candidat doit avoir la **même forme** que le tag courant (préfixe `v`, nombre de composants numériques, variante de build) et rester dans la **continuité des majeures** (écart ≤ `maxMajorGap`). Sans ça, les tags parasites d'un autre schéma de version dans le même dépôt gagnent toutes les comparaisons.
- Les suffixes de pré-release (`rc`, `beta`, `dev`…) sont distingués des variantes de build (`alpine`, `oraclelinux`…) : les premiers suivent la règle semver, les secondes doivent correspondre exactement.

### Écriture sur un cluster — action_logs, RBAC, k8sops (depuis Unreleased)
- **`CollectorManager.GetClientset(clusterID)`** (`collector/manager.go`) est le seul point d'entrée pour obtenir un client K8s en dehors de la boucle de collecte — il réutilise le clientset déjà vivant du `KubernetesCollector` (même tunnel SSH auto-réparant pour les clusters en mode ssh), plutôt que d'ouvrir une connexion dédiée par écriture. Renvoie `(nil, false)` si aucun collector n'est actif pour ce cluster — le handler appelant doit alors répondre une erreur claire, jamais échouer en silence.
- **`internal/k8sops`** contient la logique K8s pure des actions d'écriture (aujourd'hui : `ScaleWorkload`, `RestartWorkload`), séparée du handler et du package `collector` (lecture seule). Prend un `kubernetes.Interface`, testable avec `k8s.io/client-go/kubernetes/fake` sans cluster réel.
- **`handlers.RecordAction`** (`api/handlers/action_logs.go`) : chaque endpoint d'écriture l'appelle une fois après sa mutation, avec `models.ActionLogStatusSuccess`/`Failure`. Le rôle reste `middleware.RequireRole` au routeur — pas de nouvelle mécanique RBAC, pas de wrapper/décorateur (ce codebase n'en a aucun, les handlers appellent leurs helpers explicitement).
- **Les workloads (`Deployment`/`StatefulSet`/`DaemonSet`) ET les Helm releases ne sont PAS surveillés en temps réel** — contrairement aux namespaces/nœuds/secrets (qui ont un vrai `Watch`, `collector/kubernetes.go`), `collectWorkloads` et `collectHelmReleases` ne sont que des `List` relancés toutes les 60s par le ticker de `runPeriodicCollection`. Après une mutation (scale/restart/helm upgrade/rollback), la DB ne reflète donc le changement qu'à la prochaine passe périodique — sauf à déclencher `ClusterOps.TriggerSync(clusterID)` juste après (même mécanisme que `POST /clusters/:id/sync`), ce que tous les handlers d'écriture font désormais. Toujours penser à ça pour toute future action d'écriture : sans ce trigger, le frontend peut refetch avant que la DB soit à jour.
- **RBAC in-cluster** (`kubepilot-gitops`) : `update`/`patch` sur `deployments`/`daemonsets`/`statefulsets` uniquement pour scale/restart (pas `replicasets`, jamais touché directement). Le rôle `-integration` (token pour une instance distante) reste volontairement lecture seule.
- **`CollectorManager.GetRESTConfig(clusterID)`** — miroir de `GetClientset` mais renvoie le `*rest.Config` brut (même tunnel SSH). Nécessaire pour tout ce qui a besoin de plus que le clientset typé (discovery client, RESTMapper) — c'est le cas du SDK Helm.
- **`internal/helmops`** — Helm upgrade/rollback via le SDK `helm.sh/helm/v3` (v3.14.4, choisi pour son pin `k8s.io/*` en v0.29.0, proche de nos v0.29.3). `restClientGetter` custom enveloppant un `*rest.Config` déjà construit (pattern standard pour brancher le SDK sans fichier kubeconfig). Résolution + téléchargement de chart **volontairement dupliqués** depuis `internal/watcher` (pas de cache nécessaire, déclenché par l'utilisateur) plutôt que de coupler les deux packages.
- **RBAC Helm = `cluster-admin`** (`kubepilot-gitops`, flag `rbac.helmAdmin`) — décision assumée : un chart peut toucher n'importe quelle ressource (CRD comprises), aucune liste de règles n'est fiable. Point sensible pour tout futur audit sécurité de ce déploiement.
- **`internal/k8sops/manifest.go`** — deploy d'un manifest brut (server-side apply). `BuildDynamicClient` dérive un dynamic client + RESTMapper d'un `*rest.Config` (même construction que `helmops.restClientGetter`, dupliquée volontairement — pas de type partagé, `restClientGetter` est lié à l'interface Helm SDK). `ApplyManifest` traite chaque document YAML indépendamment : un `Get` préalable distingue `created`/`updated` (nécessaire aussi car le fake dynamic client de client-go v0.29 ne supporte pas la création via `Patch(ApplyPatchType)` — voir `manifest_test.go`, un `Create` explicite est fait quand l'objet n'existe pas encore, un vrai cluster accepterait les deux mais celui-ci est aussi testable). Réutilise le binding `cluster-admin` de Helm — même surface arbitraire, pas de nouveau grant RBAC.
- **`POST /api/v1/findings/:id/remediate`** — la remédiation réelle d'un finding (par opposition au changement de statut, pur bookkeeping). Image : `k8sops.SetContainerImage`, **strategic merge patch** (`types.StrategicMergePatchType`), pas un merge-patch JSON classique — `spec.template.spec.containers` est une liste, un merge-patch la remplacerait entièrement et effacerait les autres conteneurs d'un pod multi-conteneurs (couvert par un test dédié). Helm : même séquence que `HelmHandler.UpgradeHelmRelease` (résolution dépôt + `helmops.UpgradeChart`), **dupliquée intentionnellement** plutôt que factorisée — même logique que la duplication watcher/helmops : ne pas risquer de changer le comportement de l'endpoint Helm déjà en prod.
- **`ImageChecker`/`HelmChecker`** (`handlers/clusters.go`) exposent `CheckImage`/`CheckRelease` — méthodes déjà existantes sur `*watcher.ImageWatcher`/`*watcher.HelmWatcher` (jusqu'ici seulement appelées par leur propre boucle périodique). `RemediateFinding` les rappelle en tâche de fond après un fix réussi (3 tentatives sur ~21s) pour que le finding passe à `resolved` en quelques secondes plutôt qu'au prochain passage du watcher (`WORKER_INTERVAL_SECONDS`, 300s par défaut) — **premier goroutine lancé directement depuis un handler** dans ce codebase (jusqu'ici tout l'async passait par `TriggerSync`/le déjà-async du collector). Piège rencontré : `imgWatcher`/`helmWatcher` sont `nil` en mode démo (`cmd/server/main.go`) — les passer tel quel comme interface produirait un nil typé (interface non-nil, valeur nil), pas un nil interface ; le garde `if h.imageChecker == nil` d'un handler le manquerait et paniquerait à l'appel. Toujours passer par `if ptr != nil { iface = ptr }` explicite avant de construire le routeur.

### MCP (Model Context Protocol)
- Sous-commande `kubepilot mcp` = serveur MCP JSON-RPC 2.0 sur stdio (`internal/mcp`), stdlib uniquement.
- En mode MCP, **stdout est réservé au JSON-RPC** : tous les logs vont sur stderr. Ne jamais écrire sur stdout dans ce chemin.
- Outils lecture : `list_clusters`, `list_findings`, `get_finding`, `findings_summary`, `top_risks`. Écriture `set_finding_status` seulement si `MCP_ALLOW_WRITES=true`.
- Partage le même `store.Store` que le serveur HTTP.

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
- Intégration Zabbix (sens du flux pas encore tranché — voir `docs/workflow.md` § Idées à explorer)
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
