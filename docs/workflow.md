# Workflow KubePilot — tableau de bord

> Fichier **vivant** pour ne pas se perdre entre les sessions. À mettre à jour à chaque
> changement notable (en complément du `CHANGELOG.md` et de `CLAUDE.md`).
> Convention : `[ ]` à faire · `[~]` en cours · `[x]` fait.

---

## 🎯 Cap produit (rappel)

- **Vision** : « le control plane du control plane » — rendre Kubernetes simple et le **piloter** (déployer des manifests, éditer les `values.yaml` Helm, scale, restart), pas seulement l'observer.
- **Packaging (décidé)** : la ligne entre éditions n'est **pas** « agents on/off » mais **éphémère vs persistant** :
  - **Desktop** (SSH/kubeconfig, single-binary, SQLite local) — footprint zéro, collecte quand l'app est ouverte, *le land*. Ne pas le brider.
  - **In-cluster** (Helm, Postgres) — always-on, équipe, *l'expand* : historique, agents hôte, CVE système, actions/scheduled, notifications, maintenance windows.
  - Règle de décision par feature : **« a-t-elle besoin d'always-on/persistance ? »** → oui = in-cluster ; snapshot ponctuel = desktop.

---

## ✅ Fait récemment

- [x] **Helm : values, upgrade, rollback réels** (2026-08-02) — dernière brique du Pilotage MVP. `POST /api/v1/helm/:id/upgrade` (values et/ou version, un seul endpoint pour les deux) + `POST /api/v1/helm/:id/rollback`, SDK `helm.sh/helm/v3` (v3.14.4, pin k8s.io v0.29.0, compatible avec nos v0.29.3). Nouveau package `internal/helmops` (RESTClientGetter custom sur `CollectorManager.GetRESTConfig`, résolution+téléchargement de chart indépendant du watcher). RBAC : `ClusterRoleBinding` vers `cluster-admin` (décision assumée — aucune liste de règles ne couvre un chart arbitraire), poussé sur `gitops/dev`. Validé bout en bout sur Lab Cyllene (release headlamp jetable) : upgrade 0.42.0→0.44.0 + values confirmé par `helm history`, rollback vers révision 1 confirmé, `action_logs` OK, DB à jour en quelques secondes (`TriggerSync`, même correctif que scale/restart appliqué dès le départ cette fois)
  - **UI ajoutée après coup** (sur demande, page Helm réécrite) : formulaire upgrade (values JSON + version, bouton « Use latest » via rapprochement avec les findings Helm) et rollback (numéro de révision), masqués pour `viewer`. Corrigé au passage : le type frontend `HelmRelease` référençait des champs jamais renvoyés par l'API (`release_name`, `namespace_id`, `available_version`) — plusieurs champs du détail s'affichaient vides depuis un moment. Vérifié `tsc`+build propres et les vraies réponses API (Lab Cyllene) contre le nouveau type ; **pas de vérification visuelle en navigateur** — aucun outil de capture d'écran/browser disponible dans cet environnement
- [x] **Scale / rolling-restart** (2026-08-02) — première action d'écriture réelle de KubePilot sur un cluster : `PATCH /api/v1/workloads/:id/scale` + `POST /api/v1/workloads/:id/restart`, réutilisent le clientset déjà vivant du collector (`CollectorManager.GetClientset`), logique isolée et testée dans `internal/k8sops`. Boutons UI sur Inventory. RBAC `update`/`patch` poussé sur `gitops/dev`. Validé de bout en bout sur Lab Cyllene (deployment jetable, nettoyé après coup) : scale 1→3 confirmé par `kubectl`, restart confirmé par l'annotation `restartedAt`, le collector remet `replicas_desired` à jour tout seul, `action_logs` enregistre les deux
- [x] **Fondations audit trail** (2026-08-02) — `action_logs` interrogeable (`GET /api/v1/action-logs`, paginé/filtrable) + helper `handlers.RecordAction` (rôle via `middleware.RequireRole`, inchangé). Premier test du package `store` (`action_logs_test.go`)
- [x] **Métriques nœuds réparées en in-cluster** (2026-08-01) — le chart (dépôt `kubepilot-gitops`) n'accordait pas `nodes/proxy` : toutes les jauges restaient vides sans erreur. Corrigé et poussé sur `gitops/dev`
- [x] **Findings Helm réels** (2026-08-01) — le watcher ne produisait rien : un secret de release Helm n'enregistre **pas** son dépôt d'origine, donc `repo_url` restait vide. Résolution par confirmation de version (dépôts configurables + 11 publics semés + repli Artifact Hub) → 4 findings réels sur Lab Cyllene
- [x] **Faux positifs semver** (2026-08-01) — forme du tag + continuité des majeures, avec tests unitaires (`internal/watcher/tags_test.go`) : `mysql 8.0 → 9.7` (au lieu de `26.7`), `goharbor/redis-photon v2.14.3 → v2.15.1` (au lieu de `4.0`)
- [x] **Findings d'image réels** (2026-08-01) — 5 bugs cumulés corrigés, validés contre le cluster Lab Cyllene : 20 findings réels remontés (Traefik, CoreDNS, kube-proxy, metrics-server, Harbor…). Voir `CHANGELOG.md` pour le détail
- [x] Gestion des registries privés — page Registries, CRUD API, credentials + `tls_insecure` par registre
- [x] Page Settings (users + infos système), export CSV des findings, command palette `Ctrl+K`, détail cluster enrichi
- [x] Métriques système des nœuds — agentless via kubelet Summary API, time-series `node_metrics`, jauges + sparklines (tier desktop)
- [x] Vue globale ressources système sur le dashboard (style Proxmox/vCenter) — jauges CPU/RAM/disque agrégées + par cluster
- [x] Mode démo (`DEMO_MODE=true`) — seed multi-cluster, collectors désactivés, login `admin@kubepilot.local` / `demo`
- [x] Décision de packaging desktop/in-cluster

---

## 🔜 Séquencement proposé

### 1. ~~Rendre les findings réels~~ ✅ *(fait le 2026-08-01)*
- [x] Auth Docker Hub : token anonyme + préfixe `library/` cohérent scope/chemin
- [x] TLS/CA par registre (Harbor self-signed, `tls_insecure` par registre)
- [x] Nettoyage en cascade des `container_images` orphelines au `DeleteWorkloadsNotSeenSince`
- [x] Vérifié de bout en bout : 20 findings image réels sur Lab Cyllene
- [x] Faux positifs semver corrigés (forme du tag + continuité des majeures) + tests unitaires
- [x] Findings **Helm** réels validés : 4 sur Lab Cyllene (argo-cd, traefik, headlamp, metrics-server ; harbor déjà à jour)

### 2. ~~Prérequis tiering — Chart Helm~~ ✅
> ⚠️ Le chart vit dans le dépôt **`kubepilot-gitops`** (`charts/kubepilot/`), pas ici — déplacé
> volontairement en mai 2026 (`070b901`). Ne pas recréer de `helm/` dans ce dépôt.
- [x] Chaîne de déploiement opérationnelle : Jenkins → Harbor → ArgoCD (déployé en `v0.2.0-alpha.51`)
- [x] RBAC : `get nodes/proxy` (+ `nodes/stats`, `nodes/metrics`) ajouté au rôle in-cluster **et** au rôle du token d'intégration (2026-08-01)
- [x] `NODE_METRICS_*` et `HELM_AUTODISCOVER` exposés dans les values du chart
- [x] Vérifié en direct sur `kubepilot-dev` : `node_metrics` se remplit réellement (CPU/RAM/disque des 4 nœuds, échantillons à ~1 min d'intervalle) après la synchro ArgoCD — voir incident ci-dessous, il a fallu débloquer le rollout au passage
- ~~`envs/staging/values.yaml` sans `jwtSecret`~~ : non pertinent pour l'instant — **staging et prod n'existent pas encore**, seul `dev` est utilisé. À traiter le jour où staging est réellement déployé.

#### Incident annexe découvert et corrigé en vérifiant (2026-08-01, cluster Lab Cyllene, namespace `kubepilot-dev`)
Le rollout déclenché par notre push RBAC est resté bloqué sur deux problèmes **sans rapport avec le RBAC**, révélant que cet environnement tournait sur `v0.1.0-alpha.48` depuis **66 jours** (tous les rollouts suivants échouaient silencieusement) :
1. **`kubepilot-dev/harbor-registry-secret` périmé** (robot `$buildbot`, jamais rafraîchi depuis sa création le 24 mai) → `401` au pull. Ce secret n'est **pas géré par ArgoCD** (créé manuellement, cf. commentaire dans `values.yaml` du chart gitops) donc rien ne le resynchronise automatiquement. Corrigé en recopiant le `dockerconfigjson` valide de `ci-cd/harbor-kubepilot-credentials` (même robot account, secret différent, plus récent).
2. **507 lignes en double dans `nodes`** (jusqu'à 166 par nœud) — accumulées par l'ancien build qui n'avait pas encore le fix `ON CONFLICT` de `UpsertNode` (déjà corrigé dans le code actuel, cf. commentaire `internal/store/clusters.go:104` — pas une régression à craindre). Ces doublons faisaient échouer la création de l'index unique `uq_node` par l'`AutoMigrate` de la nouvelle version, qui elle applique enfin cette contrainte. Doublons supprimés (garder la ligne la plus récente par `cluster_id,name`), après avoir mis à zéro l'ancien ReplicaSet qui les recréait en boucle à chaque cycle de collecte.

**Leçon** : un environnement in-cluster qui accumule un retard de déploiement (ici via un secret non-géré par Argo) peut aussi accumuler des dérives de données qui ne surviennent qu'au moment où on rattrape enfin le retard — l'incident de RBAC a bien été corrigé, mais sa vérification a débusqué deux problèmes plus anciens et plus sérieux.

### 3. Pilotage MVP *(cœur de la vision, via l'API server)*
- [ ] Deploy d'un manifest (server-side apply) — **seule brique restante**
- [x] Édition `values.yaml` Helm + upgrade/rollback (Helm SDK Go)
- [x] Scale / rolling-restart — `edit ressource` (générique) reste à faire
- [x] Journalisation des actions (`action_logs`) + garde-fous (rôles) — branché sur scale/restart et helm upgrade/rollback ; reste deploy manifest

### 4. Plus tard — Agent hôte *(ancre in-cluster)*
- [ ] Inventaire OS/packages des nœuds (DaemonSet)
- [ ] Scan CVE **paquets système** (kernel, openssl, glibc…) → alimente le scoring
- [ ] Actions hôte (patch OS, reboot, drain) — séparable, V2/V3

### En continu
- [~] Tests backend — `internal/watcher/tags_test.go` (semver), `internal/store/action_logs_test.go`, `internal/k8sops/workloads_test.go` (clientset factice) faits ; **reste** scoring engine + le reste du store
- [x] Page Settings (utilisateurs, infos système)
- [ ] Page History (audit log)
- [x] UI registries privés (Harbor, ECR, GCR, ACR) + dépôts de charts Helm

---

## 🧠 Décisions & notes

- Métriques nœuds = **agentless** (kubelet), passe par le tunnel SSH ; l'agent custom n'est utile que pour l'inventaire OS/packages + CVE système + actions (→ in-cluster).
- **Origine d'un chart Helm** : une release Helm 3 n'enregistre pas son dépôt. KubePilot le retrouve en cherchant le chart dans les dépôts connus et en **confirmant la version installée** dans leur `index.yaml` (sinon `harbor` de Bitnami serait confondu avec celui de goharbor). Repli Artifact Hub via `HELM_AUTODISCOVER`.
- Le collector ne doit **jamais** écrire `repo_url` (il ne le connaît pas) : la colonne est exclue des `DoUpdates` de `UpsertHelmRelease`.
- **Sévérité des findings = score de risque**, pas type de mise à jour : `updateTypeSeverity` n'est que la valeur initiale, le scoring engine la recalcule. Les findings Helm scorent bas (pas de workload → pas de facteurs exposition/replicas) — à revoir si on veut les faire remonter.
- Statuts findings valides : `open, planned, ignored, approved, blocked, resolved`.
- UUID assignés côté Go (callback GORM), pas `gen_random_uuid()` (compat SQLite).
- Mode démo : ne **jamais** lancer les collectors/watchers/scoring (ils écraseraient/purgeraient le seed).
- **Instance in-cluster = `cluster-admin`** depuis le Helm upgrade/rollback (`kubepilot-gitops`, flag `rbac.helmAdmin`) — un chart peut toucher n'importe quelle ressource, aucune liste de règles n'est fiable. À garder en tête pour tout futur audit sécurité de ce déploiement.
- **`collectWorkloads` et `collectHelmReleases` n'ont pas de `Watch`** — juste un `List` toutes les 60s (`runPeriodicCollection`). Toute future action d'écriture sur l'un de ces deux types doit appeler `TriggerSync` après coup, sinon l'UI reste sur l'ancienne valeur jusqu'à la prochaine passe.

## 🛠️ Lancer / tester

```powershell
# Go n'est pas sur le PATH :
$env:Path = "C:\Users\folivanti\go-toolchain\go\bin;" + $env:Path

# Démo (stack parallèle, n'écrase pas l'instance réelle)
cd backend; $env:DEMO_MODE="true"; $env:PORT="8090"; $env:SQLITE_PATH="kubepilot-demo.db"; .\bin\kubepilot.exe
cd frontend; $env:VITE_PROXY_TARGET="http://localhost:8090"; npm run dev -- --port 3001
# → http://localhost:3001  (admin@kubepilot.local / demo)
```

## 📌 Rappels process

- **Changelog obligatoire** : toute modif notable → section `[Unreleased]` du `CHANGELOG.md`.
- **Versioning** : SemVer + cycle alpha/beta/rc/stable (cf. CLAUDE.md). Version courante : `v0.2.0-alpha.5`.
- **Commits** : Conventional Commits (`type(scope): message`).
