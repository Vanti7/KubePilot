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
- **Helm : édition des values, upgrade, rollback réels** — dernière brique du Pilotage MVP, la plus proche du cœur de la vision produit. Nouvelle dépendance `helm.sh/helm/v3` (v3.14.4, pin `k8s.io/*` en v0.29.0 — compatible sans rétrograder nos v0.29.3 existants). `POST /api/v1/helm/:id/upgrade` (`{"chart_version"?, "values"?}` — les deux optionnels : version vide = garde la version installée, values absentes = garde les values actuelles au lieu de les effacer ; un seul endpoint sert donc à la fois « éditer les values » et « changer de version », Helm n'a pas de distinction) et `POST /api/v1/helm/:id/rollback` (`{"revision"}`). Écriture réservée `operator`/`admin`, journalisée via `RecordAction`
  - **Téléchargement de chart indépendant du watcher existant** : un upgrade re-rend toujours le chart complet (pas de concept Helm pour « juste changer les values »), donc même une édition de values télécharge le chart correspondant depuis le dépôt déjà résolu (`HelmRelease.RepoURL`). Nouveau package `internal/helmops`, volontairement séparé de la logique de `internal/watcher` (pas de cache nécessaire, déclenché par l'utilisateur et non en boucle)
  - **`CollectorManager.GetRESTConfig`** (miroir de `GetClientset`) expose le `*rest.Config` par cluster (tunnel SSH inclus) — nécessaire au SDK Helm (discovery client, RESTMapper), qui a besoin de plus que le clientset typé utilisé par scale/restart
  - **Même correctif que le scale/restart de ce matin, appliqué dès le départ cette fois** : `collectHelmReleases` n'est lui non plus qu'un `List` toutes les 60s (pas de `Watch`) — les deux endpoints déclenchent un `TriggerSync` immédiat après une action réussie
  - **RBAC** (dépôt `kubepilot-gitops`) — décision de sécurité assumée : contrairement à scale/restart (3 types de ressources précis), un chart Helm peut créer/modifier/supprimer n'importe quelle ressource (CRD comprises), donc aucune liste de règles ne peut être fiable. Nouveau `ClusterRoleBinding` liant le ServiceAccount `kubepilot` au `ClusterRole` natif `cluster-admin`, derrière le flag `rbac.helmAdmin` (défaut `true`, réversible sans toucher au reste du chart). Le rôle `-integration` (lecture seule pour une instance distante) reste inchangé
  - Validé de bout en bout sur Lab Cyllene (release `headlamp` jetable, installée puis désinstallée par `helm` CLI) : upgrade 0.42.0 → 0.44.0 avec nouvelles values confirmé par `helm history`/`helm get values`, rollback vers la révision 1 confirmé (« Rollback to 1 » dans l'historique), `action_logs` enregistre les deux, la DB KubePilot se met à jour en quelques secondes grâce au `TriggerSync`
- **Scale et rolling-restart des workloads** — première action d'écriture réelle de KubePilot sur un cluster (jusqu'ici 100% lecture seule). `PATCH /api/v1/workloads/:id/scale` (`{"replicas": N}`, rejette les DaemonSets) et `POST /api/v1/workloads/:id/restart` (Deployment/StatefulSet/DaemonSet — même mécanisme que `kubectl rollout restart`, annotation `restartedAt` sur le pod template). Écriture réservée `operator`/`admin`, chaque tentative journalisée via `handlers.RecordAction` (voir fondations `action_logs` ci-dessous). Réutilise le clientset déjà vivant du collector pour le cluster (nouveau `CollectorManager.GetClientset`) plutôt que d'ouvrir une connexion dédiée — le même tunnel SSH auto-réparant sert donc aussi aux écritures. Logique K8s isolée dans le nouveau package `internal/k8sops` (testé avec un clientset factice, aucun cluster requis). **UI** : boutons Scale/Restart sur la page Inventory (masqués pour le rôle `viewer`)
- **UI figée après un scale/restart** : les workloads ne sont **pas** surveillés en temps réel par le collector (contrairement aux namespaces/nœuds/secrets qui ont un vrai `Watch`) — ils n'étaient relistés que toutes les 60s, donc la DB ne reflétait le changement qu'à la prochaine passe périodique. Les deux endpoints déclenchent désormais une collecte immédiate en arrière-plan (`CollectorManager.TriggerSync`, même mécanisme que `POST /clusters/:id/sync`) ; le frontend refait un `invalidateQueries` à 0s/1,5s/4s pour laisser le temps à cette passe d'arriver avant de rafraîchir l'affichage
  - **RBAC** (dépôt `kubepilot-gitops`) : `update`/`patch` ajoutés sur `deployments`/`daemonsets`/`statefulsets` du rôle in-cluster — sans ça, l'instance in-cluster ne peut pas encore écrire (Lab Cyllene en SSH utilise un `admin.conf` cluster-admin donc n'était pas concerné). Le rôle `-integration` (token pour une instance distante) reste volontairement en lecture seule
- **Fondations de la piste d'audit (`action_logs`)** : nouvel endpoint `GET /api/v1/action-logs` (paginé, filtrable par `user_id`/`action`/`entity_type`/`entity_id`/`status`) et helper `handlers.RecordAction` que les endpoints d'écriture appellent pour journaliser chaque tentative — première utilisation concrète : scale/restart ci-dessus. Champ `status` (`success`/`failure`) ajouté à `ActionLog`, qui n'avait jusqu'ici aucun moyen de distinguer une action réussie d'un échec (migration `009_action_log_status.sql`). L'application du rôle reste `middleware.RequireRole`, inchangée
- **Findings Helm réels** — le watcher Helm ne produisait *aucun* finding : un secret de release Helm 3 enregistre le nom et la version du chart mais **pas le dépôt d'origine**, `repo_url` restait donc toujours vide et `ListAllHelmReleases` filtrait sur `repo_url != ''` → liste vide à chaque passe. Le dépôt est désormais **résolu** par KubePilot, et 4 findings Helm réels remontent sur le cluster Lab Cyllene (argo-cd 9.5.0 → 10.2.2, traefik 39.0.7 → 41.1.0, headlamp 0.42.0 → 0.44.0, metrics-server 3.13.0 → 3.13.1 ; harbor 1.19.1 déjà à jour)
  - **Dépôts de charts configurables** : nouveau modèle `HelmRepository` (nom, URL, identifiants, `tls_insecure`) + API `GET/POST/PUT/DELETE /api/v1/helm-repositories` et `POST /api/v1/helm-repositories/:id/test` (télécharge `index.yaml`, renvoie le nombre de charts). Écriture réservée `operator`/`admin` ; les identifiants ne sont **jamais** renvoyés (seulement `username` + `has_credentials`)
  - **10 dépôts publics semés au premier démarrage** (argo, cert-manager, grafana, harbor, headlamp, ingress-nginx, longhorn, metrics-server, prometheus-community, traefik) — uniquement si la table est vide, pour qu'un dépôt supprimé ne réapparaisse pas au redémarrage. Les agrégateurs type Bitnami sont volontairement exclus : leur `index.yaml` pèse des dizaines de Mo, au-delà du plafond de téléchargement (10 Mo, désormais signalé explicitement au lieu d'échouer en erreur de syntaxe YAML)
  - **Résolution par confirmation de version** : un dépôt n'est retenu que si son index contient **exactement la version installée** — c'est ce qui distingue le `harbor` 1.19.1 de goharbor du `harbor` 27.0.3 de Bitnami. À défaut, repli sur une correspondance par nom de chart seul
  - **Découverte via Artifact Hub** (`HELM_AUTODISCOVER`, défaut `true`) pour les charts qu'aucun dépôt configuré ne publie ; les candidats sans confirmation de version sont écartés. Mettre à `false` pour une résolution 100 % hors-ligne
  - **UI** : section « Chart repositories » sur la page Registries (ajout, édition, test, suppression ; badge `built-in` sur les dépôts semés)
- **Droit RBAC `nodes/proxy` accordé au chart** (dépôt `kubepilot-gitops`, `charts/kubepilot/`) : le chart n'accordait que `metrics.k8s.io`, alors que le collector lit les métriques nœuds sur le Summary API de chaque kubelet via le proxy de l'API server (`/api/v1/nodes/<name>/proxy/stats/summary`). Chaque échantillon était refusé — les jauges CPU/RAM/disque et les sparklines restaient **vides en in-cluster** alors qu'elles fonctionnaient en mode desktop via le tunnel SSH. Ajouté au rôle in-cluster **et** au rôle du token d'intégration (une instance distante lit les métriques de la même façon), derrière `rbac.nodeMetrics`
  - `NODE_METRICS_ENABLED`, `NODE_METRICS_RETENTION_HOURS` et `HELM_AUTODISCOVER` sont désormais câblés dans le Deployment backend du chart — ils restaient sur leurs valeurs par défaut compilées, non configurables depuis les values
- **Sparklines de nœud interactives** : survol des graphiques d'historique (détail d'un nœud) → guide vertical, point et valeur formatée (%, débit) à la position pointée
- **Command palette** (`Ctrl/Cmd+K`, ou bouton dans la barre du haut) : navigation rapide vers les pages et les clusters, au clavier (flèches + Entrée). L'item « Settings » du menu utilisateur, jusqu'ici inopérant, ouvre désormais la page Settings
- **Export CSV des findings** : bouton « Export » sur la page Updates et endpoint `GET /api/v1/findings/export` (mêmes filtres que la liste, pagination interne plafonnée par `EXPORT_MAX_ROWS`, défaut `10000`). Colonnes : cluster, namespace, kind, resource, versions, type, sévérité, score, statut, date de détection, titre
- **Détail cluster enrichi** : la page d'un cluster affiche désormais un panneau **System Resources** (jauges radiales CPU / mémoire / disque, nœuds prêts, pods) et la **liste de ses nœuds** (rôle, kubelet, statut Ready), en plus des findings. Nouvel endpoint `GET /api/v1/clusters/:id/resources`. Le composant `ResourceGauge` est désormais partagé entre le dashboard et le détail cluster
- **Page Settings** (`/settings`, entrée sidebar réservée aux admins) — gestion des utilisateurs et infos système, accès restreint au rôle `admin`
  - **Onglet Users** : liste, création, changement de rôle (admin/operator/viewer), activation/désactivation et suppression des comptes. Garde-fous : impossible de rétrograder/désactiver/supprimer le **dernier admin actif** ni son **propre** compte
  - **API** : `GET /api/v1/auth/users`, `PATCH /api/v1/auth/users/:id` (rôle / `is_active`), `DELETE /api/v1/auth/users/:id` — admin only
  - **Onglet System** : configuration runtime non sensible (version, drivers storage/cache, local/demo/in-cluster, intervalle de collecte, rétention des métriques, MCP) via le nouvel endpoint `GET /api/v1/settings` (admin only) — **aucun secret exposé** (JWT, DB URL, identifiants SSH/registry jamais renvoyés)
- **Gestion des registries privés** : nouvelle page **Registries** (`/registries`, entrée sidebar) pour configurer identifiants et TLS des registres privés/self-signed (Harbor, GHCR, Quay, ECR, GCR, ACR, OCI générique) — sans quoi le watcher d'images ne pouvait scanner que les registres publics et ne produisait aucun finding pour ces images
  - **API** : `GET/POST/PUT/DELETE /api/v1/registries` + `POST /api/v1/registries/:id/test` (sonde `GET /v2/` avec auth → joignabilité). Écriture réservée `operator`/`admin`. Les identifiants ne sont **jamais** renvoyés par l'API (seulement `username` + `has_credentials`)
  - **Rattachement automatique** : les `container_images` sont liées à un registre configuré par host (à la collecte et lors de la création/màj d'un registre) ; le watcher utilise alors ses identifiants (basic auth) et son réglage TLS
  - **TLS par registre** : champ `tls_insecure` (modèle `ImageRegistry`, migration `008_registry_tls.sql`) — le watcher utilise un client HTTP dédié `InsecureSkipVerify` uniquement pour les registres ainsi marqués (Harbor self-signed)
  - **UI** : bandeau d'incitation sur la page Updates quand aucun finding et aucun registre configuré, pointant vers `/registries`
- **Métriques système des nœuds (agentless)** : collecte CPU / mémoire / disque / réseau de chaque nœud via le Summary API du kubelet (`/api/v1/nodes/<name>/proxy/stats/summary`, proxifié par l'API server — fonctionne aussi à travers le tunnel SSH). Stockées en time-series dans la nouvelle table `node_metrics`, échantillonnées à chaque passe de collecte
- Endpoint `GET /api/v1/nodes/:id/metrics?since=<RFC3339|durée>` — historique d'usage d'un nœud (défaut : dernière heure)
- Champ `metrics` (dernier échantillon : `cpu_usage_percent`, `memory_usage_percent`, `fs_used_percent`, débits réseau, `pods_running`) ajouté à `GET /api/v1/nodes`
- Variables d'env `NODE_METRICS_ENABLED` (défaut `true`) et `NODE_METRICS_RETENTION_HOURS` (défaut `168` = 7 j, purge automatique des échantillons)
- UI page Nodes : jauges CPU / mémoire / disque par nœud (rafraîchies toutes les 30 s) + sparklines d'historique (6 h) dans le détail du nœud
- **Vue globale des ressources système sur le dashboard** (à la Proxmox/vCenter) : jauges radiales CPU / mémoire / disque agrégées à l'échelle du parc (capacité vs usage live), compteurs nœuds prêts et pods en cours, et répartition par cluster. Agrégation exposée dans `GET /api/v1/overview` (champs `system_resources` et `resources_per_cluster`)
- **Mode démo** (`DEMO_MODE=true`) : seed d'un jeu de données synthétique multi-cluster (3 clusters, 11 nœuds avec métriques time-series sur 6 h, namespaces, findings + risk scores) et désactivation des collectors/watchers/scoring pour ne pas l'écraser. Login démo `admin@kubepilot.local` / `demo` (mot de passe par défaut en mode démo). Cible du proxy Vite surchargeable via `VITE_PROXY_TARGET`

### Changed
- **Panneau System Resources du dashboard** : affiché en permanence avec un état vide explicite (« No node metrics collected yet ») au lieu d'être masqué quand aucune métrique n'est encore collectée
- **Bruit de logs du watcher d'images** : les erreurs attendues/environnementales (TLS self-signed `x509`, `401`/`403` d'auth registry, hôte injoignable, références orphelines `record not found`) sont désormais loggées en `debug` au lieu de `warn` — elles inondaient les logs à chaque cycle. Les erreurs réellement inattendues restent en `warn`

### Fixed
- **Faux positifs de comparaison semver du watcher d'images** : `findNewerTag` retenait n'importe quel tag analysable comme semver, y compris ceux d'une numérotation sans rapport publiée dans le même dépôt — `mysql:8.0` se voyait proposer `26.7` (un tag isolé au milieu des lignes 5.x–9.x) et `goharbor/redis-photon:v2.14.3` se voyait proposer `4.0` (la version de Redis embarquée, pas celle de Harbor). Trois garde-fous, couverts par des tests unitaires (`internal/watcher/tags_test.go`) :
  - **forme du tag** : préfixe (`v`), nombre de composants numériques et variante de build doivent coïncider — `8.0` n'est plus comparé à `8.0.36`, ni `1.27.3-alpine` à `1.28.0-bookworm`. Les suffixes de pré-release (`rc`, `beta`, `dev`…) restent distingués des variantes (`alpine`, `oraclelinux`…), donc `v1.2.3-rc1` → `v1.2.3` fonctionne toujours
  - **continuité des majeures** : à partir de la majeure courante, on ne franchit pas un écart supérieur à 5 dans les majeures présentes — `8.0` remonte désormais `9.7`, plus `26.7`
  - `findNewerTag`, `classifyUpdate` et `updateTypeSeverity` déplacés de `watcher/image.go` vers `watcher/tags.go`
- **`repo_url` écrasé à chaque collecte** : `UpsertHelmRelease` incluait `repo_url` dans ses `DoUpdates`, or le collector ne connaît pas cette valeur (elle est résolue par le watcher) — chaque passe de collecte remettait donc à zéro la résolution, obligeant à tout recommencer au cycle suivant. La colonne est retirée de la clause de mise à jour
- **Crash à l'ouverture d'un finding** (`Cannot read properties of undefined (reading 'toLowerCase')`) : `FindingDetail` passait `finding.target_kind` à `buildHeadlampURL`, un champ que l'API **ne renvoie pas**. Le type TypeScript `UpdateFinding` avait divergé de `store.FindingWithScore` et déclarait six champs inexistants (`target_id`, `target_kind`, `is_breaking`, `changelog_url`, `last_confirmed_at`, objet imbriqué `risk_score`). Seul `target_kind` provoquait un crash ; les autres échouaient en silence — c'est pourquoi **toutes les sévérités et tous les scores de l'app s'affichaient en `—`** : ils lisaient `finding.risk_score` alors que l'API envoie `score` / `score_severity` à plat. Le type reflète désormais le DTO et les cinq consommateurs (Updates, Overview, ClusterDetail, Inventory, FindingDetail) lisent les vrais champs
  - `GET /api/v1/findings` expose en plus `workload_kind` (kind Kubernetes — `Deployment`/`DaemonSet`/`StatefulSet`, distinct de `kind` qui vaut `image`/`helm`), `score_factors` (détail par facteur du score, qui n'avait jamais pu s'afficher) et `helm_release_name` (les findings Helm n'ont pas de workload : la commande `helm upgrade` suggérée interpolait `undefined`)
- **Aucun finding d'image n'était jamais créé** — cinq bugs cumulés empêchaient le watcher d'images de produire le moindre finding, alors même que la détection de tags fonctionnait (1190 observations enregistrées pour 0 finding sur un cluster réel). Corrigés ensemble ; le même cluster remonte désormais des findings réels (Traefik, CoreDNS, kube-proxy, metrics-server, Harbor…)
  - **`UpsertWorkload` fabriquait un UUID fantôme à chaque passe** *(cause racine)* : `ON CONFLICT DO UPDATE` ne renvoie pas la clé primaire de la ligne survivante, donc le `uuid.New()` généré côté Go restait en mémoire et **toutes les `container_images` étaient rattachées à un workload inexistant**. À chaque collecte, 100 % des images devenaient orphelines ; le watcher détectait bien une version plus récente puis échouait sur `GetWorkload` (`record not found`, classée « erreur attendue » donc jamais remontée) sans créer de finding. L'ID de la ligne existante est maintenant relu avant l'upsert
  - **Colonne `cves` absente en base** : le champ Go `CVEs` sans tag `column:` était mappé par GORM sur `cv_es`, alors que `UpsertFinding` référence `"cves"` dans sa clause `ON CONFLICT` et que `migrations/001_initial.sql` crée `cves`. Résultat : `SQL logic error: no such column: excluded.cves` sur **chaque** upsert de finding. Tag `column:cves` ajouté au modèle
  - **Images officielles Docker Hub systématiquement en 401** (`traefik`, `nginx`, `haproxy`, `mysql`, `busybox`…) : le token Bearer était demandé avec le scope `library/<image>` mais la requête rejouée utilisait le chemin sans préfixe → token hors scope, refus permanent. Le préfixe `library/` est désormais appliqué au chemin et au scope de façon cohérente
  - **GHCR / ECR Public en 403** : sur un 401, le watcher réclamait un token à `auth.docker.io` **quel que soit le registre**, puis le présentait à `ghcr.io` ou `ecr-public.aws.com` qui ne le reconnaissent pas. Le repli Docker Hub est maintenant réservé à Docker Hub
  - **`container_images` orphelines jamais nettoyées** : `DeleteWorkloadsNotSeenSince` supprimait les workloads périmés sans leurs images ni observations (`AutoMigrate` ne crée pas les `ON DELETE CASCADE` de la migration PostgreSQL). La purge est désormais en cascade — images, observations de tags, et résolution des findings encore ouverts sur ces workloads
- **Tableau « Cluster Status » du dashboard figé** : le statut de chaque cluster était codé en dur à `healthy` et les colonnes High/Medium toujours à 0 (le payload `findings_per_cluster` ne renvoyait que `open` et `critical`). `GET /api/v1/overview` expose désormais `status`, `high` et `medium` par cluster, et l'UI les affiche réellement (statut via composant partagé `ClusterStatusBadge`)
- **Entrée de menu « Risks » morte** : la sidebar pointait vers `/risks` sans route correspondante (redirection silencieuse vers Overview). `/risks` ouvre maintenant la vue de triage préfiltrée sur les findings `critical` + `high`
- **Filtre namespace des workloads inopérant** : le frontend envoyait `namespace_id` (UUID) mais `GET /api/v1/workloads` ne lisait que `namespace` (nom) → le filtre était ignoré. Le handler accepte désormais `namespace_id`, résolu côté store en `(cluster_id, namespace_name)` via sous-requête (les workloads ne stockent que le nom du namespace), scopé par cluster pour éviter les collisions de noms inter-clusters
- **Connexion SSH instable** : le `ssh.Client` était créé une seule fois ; dès que le tunnel tombait (blip réseau, coupure des connexions longues par un firewall), **tous** les appels K8s (watch + list) échouaient en boucle sans jamais se rétablir. Remplacé par un `sshTunnel` auto-réparant — keepalive `keepalive@openssh.com` toutes les 20 s pour éviter la coupure sur inactivité, et reconnexion automatique au prochain `Dial` quand la connexion est morte

## [0.2.0-alpha.1] — 2026-06-12

> Mode local single-binary (SQLite + cache mémoire), serveur MCP, connexion cluster par SSH avec tunnel d'API,
> et corrections de robustesse (collecte SQLite, UI). Première version utilisable sans PostgreSQL/Redis ni
> accès réseau direct à l'API server.
> **Canal : alpha** — ne pas utiliser en production.

### Added
- **Connexion cluster par SSH** (`connection_mode = "ssh"`) — KubePilot se connecte en SSH à un nœud, lit son kubeconfig et **fait transiter tout le trafic de l'API Kubernetes dans la connexion SSH**. Permet d'atteindre un cluster dont l'API server (6443/443) n'est pas joignable directement depuis le poste
  - Champs `Cluster` : `connection_mode`, `ssh_host`, `ssh_port` (défaut 22), `ssh_user`, `ssh_password` (jamais renvoyé en API), `ssh_kubeconfig_path`, `ssh_sudo`
  - **UI** : nouvelle page **Clusters** (`/clusters`, entrée sidebar) — liste des clusters (env, mode de connexion, statut, dernière collecte) avec actions Sync/Delete, et modal d'ajout avec sélecteur de mode **Kubeconfig / SSH** (champs SSH conditionnels : host, port, user, password, chemin kubeconfig, sudo)
  - **API** : `POST` / `PUT /api/v1/clusters` acceptent désormais `connection_mode` + `ssh_host`/`ssh_port`/`ssh_user`/`ssh_password`/`ssh_kubeconfig_path`/`ssh_sudo` ; validation 400 si `ssh_host`/`ssh_user` manquants en mode ssh
  - Variables d'env (mode local) : `SSH_HOST`, `SSH_PORT`, `SSH_USER`, `SSH_PASSWORD`, `SSH_KUBECONFIG_PATH`, `SSH_SUDO` — quand `SSH_HOST` est défini, le cluster local est enregistré en mode SSH au bootstrap
  - Lecture du kubeconfig distant via `cat` (ou `sudo -S cat` si `ssh_sudo`), avec fallback sur les emplacements courants (`/etc/rancher/k3s/k3s.yaml`, `~/.kube/config`, `/etc/kubernetes/admin.conf`)
  - Auth par mot de passe ; clé d'hôte non épinglée en alpha (`InsecureIgnoreHostKey`) — usage réseau de confiance / bastion
  - Migration `005_cluster_ssh.sql` (parité PostgreSQL ; AutoMigrate applique les colonnes)
- **Mode local single-binary (`LOCAL_MODE=true`)** — KubePilot tourne sur un poste sans PostgreSQL ni Redis, pour tester rapidement contre un cluster
  - Stockage SQLite (driver pur-Go `glebarez/sqlite`, sans CGO) sélectionné automatiquement quand `LOCAL_MODE=true` ou quand `DB_URL` est vide ; chemin configurable via `SQLITE_PATH` (défaut `kubepilot.db`)
  - Cache en mémoire (`MemoryCache` avec TTL + janitor) en remplacement de Redis ; abstraction `store.Cache` (impl. `RedisCache` / `MemoryCache`) — les watchers ne dépendent plus de Redis directement
  - Auto-enregistrement du cluster local depuis le kubeconfig de l'utilisateur (`KUBECONFIG_PATH` ou règles de chargement par défaut) — le kubeconfig fusionné est stocké base64 dans `KubeconfigRef`
  - Sélection de driver explicite possible : `STORAGE_DRIVER` (`postgres`|`sqlite`), `CACHE_DRIVER` (`redis`|`memory`)
  - Modèles rendus agnostiques du SGBD : génération des UUID v4 côté Go via un callback GORM `BeforeCreate` (plus de dépendance à `gen_random_uuid()` PostgreSQL)
- **Serveur MCP (Model Context Protocol)** — sous-commande `kubepilot mcp` exposant les findings et scores de risque à un assistant IA (Claude, Cursor, …) via JSON-RPC 2.0 sur stdio (stdlib uniquement, sans dépendance)
  - Outils en lecture : `list_clusters`, `list_findings`, `get_finding`, `findings_summary`, `top_risks`
  - Outil en écriture `set_finding_status` (activé seulement si `MCP_ALLOW_WRITES=true`)
  - Logs redirigés vers stderr en mode MCP pour garder stdout propre pour le canal JSON-RPC
- Endpoint `GET /api/v1/namespaces` — listing des namespaces avec filtre `cluster_id` (manquait dans le router, causait des 404 sur les pages Inventory et Secrets)
- Endpoints `GET/POST/DELETE /api/v1/integrations` + `POST /api/v1/integrations/:id/test` — CRUD complet des comptes d'intégration (manquaient dans le router, causaient des 404 + crash page Integrations)
- Migration `004_finding_unique_constraint.sql` — index uniques `(cluster_id, container_image_id)` et `(cluster_id, helm_release_id)` sur `update_findings`
- Migration `006_collection_unique_indexes.sql` + index uniques déclarés sur les modèles GORM (`uq_node`, `uq_workload`, `uq_container_image`, `uq_image_tag`, `uq_helm_release`, `uq_secret`) — requis pour que les upserts `clause.OnConflict` du collecteur fonctionnent aussi sur SQLite (ces contraintes n'existaient que dans les migrations SQL PostgreSQL)
- `ErrorBoundary` React — tout crash de rendu affiche désormais un message d'erreur lisible (message + stack + bouton Reload) au lieu d'un écran noir muet

### Changed
- `GET /health/connectors` — clés `postgres`/`redis` renommées en `database`/`cache` (le backend supporte désormais SQLite + cache mémoire en plus de PostgreSQL + Redis)

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
- **Bug (collector)** : duplication des nodes — `UpsertNode` utilisait `Where(cluster_id,name).FirstOrCreate(node)`, mais `nodeFromK8s` posait un `ID` aléatoire à chaque passe que GORM ajoutait à la condition du `First` (`… AND id=<random>`) → jamais trouvé, une nouvelle ligne créée à chaque cycle. Passé en `clause.OnConflict` sur `(cluster_id, name)` (index `uq_node`), l'`id` existant est préservé
- **Bug (frontend)** : `ClusterSelector` plantait (écran noir global, le sélecteur étant dans la topbar) sur un cluster au statut `unknown` — absent de la table de styles → `dot.color` sur `undefined`. Ajout de l'entrée `unknown` + fallback
- **Bug (frontend)** : `Object.keys(...)` sur des champs JSON `null` (labels workload, capacity/allocatable node, factors du risk score) plantait les panneaux de détail — guards `?? {}` (le backend sérialise les maps vides en `null`)
- **Bug (frontend)** : l'arbre namespace d'Inventory/Secrets affichait un nom de cluster vide (`display_name` jamais renvoyé par le backend) → fallback sur `name`
- **Bug critique (collector)** : fuite de goroutines de watch — `collectAll` (toutes les 60 s) relançait à chaque passe un nouveau watcher namespaces/nodes/secrets sans arrêter les précédents → accumulation illimitée de goroutines, connexions watch et écritures DB. Les watchers sont désormais démarrés **une seule fois** et se reconnectent en interne quand l'API server ferme le canal de watch
- **Bug critique (collector)** : panic nil-pointer sur `*Deployment.Spec.Replicas` / `*StatefulSet.Spec.Replicas` (champ optionnel pouvant être nil côté API K8s) — comme la collecte tourne dans une goroutine sans `recover()`, cela faisait tomber tout le process. Ajout de `replicaCount()` qui retourne 1 par défaut quand le pointeur est nil
- **Bug critique (scoring)** : l'étape de normalisation (`docs/scoring.md §7`, `MaxPossibleSum = 21.6`) était omise — la somme pondérée brute (~0-21) écrasait la sévérité initiale et plafonnait quasiment tous les findings en `info`/`low`. Le score est désormais normalisé sur 0-100 et arrondi à une décimale ; valeurs de criticité de service alignées sur la doc (critical=15, high=10, medium=5, low=2)
- **Bug (store)** : le filtre `namespace_id` de `ListFindings` n'était appliqué qu'au `Count` (total) et pas à la requête des lignes → total et résultats incohérents, filtre namespace sans effet. Le filtre est désormais appliqué aux deux requêtes
- **Bug (watchers)** : les findings image/Helm n'étaient jamais clôturés automatiquement — quand une ressource repasse à jour, les findings actifs (`open`/`planned`/`approved`) sont désormais passés à `resolved` (`ResolveActiveFindingForImage` / `ResolveActiveFindingForHelm`)
- **Bug (API)** : `POST /api/v1/clusters/:id/sync` ne faisait que réinitialiser le statut sans déclencher de collecte. Le manager de collecteurs expose désormais `TriggerSync` qui lance une passe de collecte immédiate en arrière-plan pour le cluster ; la réponse inclut `collector_active`
- **Bug (frontend)** : `createCluster()` envoyait `{display_name, endpoint, kubeconfig}` ignorés par le backend (qui attend `api_endpoint`, `kubeconfig_ref`) → cluster créé sans endpoint ni kubeconfig. Payload aligné sur le contrat backend
- **Robustesse (SSE)** : `EventBus.Subscribe`/`Unsubscribe` pouvaient bloquer indéfiniment après `Stop()` (envoi sur un channel plus jamais lu) ; ils sont désormais protégés par le channel `quit`
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

[Unreleased]: https://github.com/Vanti7/KubePilot/compare/v0.2.0-alpha.1...HEAD
[0.2.0-alpha.1]: https://github.com/Vanti7/KubePilot/compare/v0.1.0-alpha.2...v0.2.0-alpha.1
[0.1.0-alpha.2]: https://github.com/Vanti7/KubePilot/compare/v0.1.0-alpha.1...v0.1.0-alpha.2
[0.1.0-alpha.1]: https://github.com/Vanti7/KubePilot/releases/tag/v0.1.0-alpha.1
