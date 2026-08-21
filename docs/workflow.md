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

- [x] **Exception rules : endpoint REST + page de gestion** (2026-08-21) — suite du lot précédent, qui n'avait volontairement pas d'API/UI. `GET/POST/PUT/DELETE /api/v1/exception-rules` (lecture tout rôle, écriture `operator`/`admin`), nouveaux `store.ListExceptionRules`/`GetExceptionRule`/`UpdateExceptionRule`/`DeleteExceptionRule`. Page **Exceptions** (`/exception-rules`) : table + formulaire (5 scopes, sélecteur de workload par cluster, pattern d'image), pause/reprise via le même `PUT`. Pas de `RecordAction` (CRUD de config, même traitement que dépôts Helm/registries/intégrations). `tsc`/build frontend propres, `go build/vet/test` propres, validé en direct (mode démo) : création globale + scopée cluster, jointure `cluster` dans la liste, `rule_type` invalide → 400, pause confirmée, `viewer` 200 en lecture / 403 en écriture, suppression confirmée
- [x] **Scoring engine — CVSS, fenêtres de maintenance, exception rules** (2026-08-06) — les 3 briques manquantes face à `docs/scoring.md` (trouvées en écrivant les tests de scoring plus tôt dans la journée) sont implémentées : facteur CVSS lu depuis `UpdateFinding.CVEs`, `isWindowActive` évalue pour de vrai la planification cron (`robfig/cron/v3`, nouvelle dépendance), `ExceptionRule` gagne `RuleType`/`Name`/`NamespaceName` (migration `011_exception_rule_type.sql`) et ses 3 effets sont appliqués dans `ScoreFinding` avec priorité par spécificité de scope. `internal/scoring/engine_test.go` passe de 5 à 15 tests, dont une reproduction exacte de l'Example 1 chiffré de la doc (82.6, critical). Voir décision détaillée ci-dessous et `CHANGELOG.md`. **Reste** : annotation `kubepilot/criticality` vs `kubepilot.io/criticality` de la doc toujours pas réconciliée (cosmétique, pas fonctionnel)
- [x] **Passe de sécurité complète** (2026-08-06) — revue en 3 phases (identification par agents parallèles → filtration indépendante par vulnérabilité → seuil de confiance ≥8), 7 failles confirmées et corrigées :
  - **SSRF non authentifié** dans `ImageWatcher.fetchTags` et `HelmWatcher.discoverViaArtifactHub` — les deux tournent en boucle temporisée, sans utilisateur dans la chaîne. Nouveau package `internal/netguard` : `net.Dialer.Control` refuse loopback/link-local/multicast (dont `169.254.169.254`, métadonnées cloud) en validant l'IP **après** résolution DNS (résiste au rebinding). RFC1918 volontairement autorisé — c'est l'usage principal de l'outil, pas une menace. Branché aussi sur `probeHelmRepository` (exposition moindre, `operator`). Tests unitaires (`netguard_test.go`)
  - **`JWT_SECRET` par défaut codé en dur** → secret aléatoire de 32 octets généré au démarrage si absent (`crypto/rand`), `logger.Warn` explicite. Compromis assumé : sessions non persistées entre redémarrages tant que `JWT_SECRET` n'est pas fixé
  - **`Values` Helm exposées à `viewer`** (mots de passe/clés dans les values) et **URL de webhook d'intégration exposée à `viewer`** (l'URL **est** le jeton d'auth pour Slack/PagerDuty) — masquées/retirées, même traitement que les identifiants de dépôt Helm
  - **Aucune révocation de session** : `PATCH /auth/users/:id` (désactiver/rétrograder un compte) n'avait aucun effet avant l'expiration du token (24h). `JWTAuth` relit désormais `role`/`is_active` en base à chaque requête (`store.ActiveUserRole`) au lieu de faire confiance aux claims signées à la connexion
  - `go build/vet/test` propres, `tsc` propre (aucun changement frontend requis — les DTOs consommés n'exposaient déjà pas les champs retirés). **Pas encore re-testé en live sur Lab Cyllene** après ce lot (à faire avant de considérer la passe close)
- [x] **Updates : un vrai bouton "Fix now"** (2026-08-02) — jusqu'ici `PATCH /findings/:id/status` ne posait qu'une étiquette (aucun appel `k8sops`/`helmops`). Nouveau `POST /api/v1/findings/:id/remediate` : bump réel du tag d'image (nouvelle capacité `k8sops.SetContainerImage`, patch **strategic merge** — un merge-patch classique aurait effacé les autres conteneurs d'un pod multi-conteneurs, testé explicitement) ou upgrade Helm vers `latest_version` (réutilise `helmops.UpgradeChart`, séquence dupliquée intentionnellement depuis `HelmHandler.UpgradeHelmRelease` plutôt que factorisée — même précédent que watcher/helmops pour ne pas risquer l'existant). Aucun changement RBAC. Bonus : re-vérification immédiate en tâche de fond après le fix (3 tentatives sur ~21s, `CheckImage`/`CheckRelease` — méthodes déjà exportées des watchers, jusqu'ici seulement appelées depuis leur propre boucle) pour que le finding passe à `resolved` en quelques secondes plutôt qu'au prochain passage du watcher (jusqu'à 5 min, `WORKER_INTERVAL_SECONDS`). UI : bouton "Fix now → v{latest}" dans `FindingDetail`, remplace l'ancienne commande Helm à copier-coller (non actionnable). `go build/vet/test` + `tsc`/build frontend propres ; validé sur Lab Cyllene (namespace jetable `kubepilot-fixnow-test`) : fix image (nginx 1.24→1.31, confirmé par `kubectl`) et fix Helm (headlamp 0.43.0→0.44.0, confirmé par `helm history`) tous deux appliqués réellement, les deux findings passés à `resolved` en ~20s (re-vérification rapide, pas les 5 min du cycle normal), `action_logs` corrects (`update_image`/`helm_upgrade` avec `finding_id`), remédiation d'un finding déjà résolu rejetée en 400, rôle `viewer` bloqué en 403. Ressources jetables nettoyées après coup
  - **Bug de sécurité trouvé et corrigé au passage** : `Cluster.KubeconfigRef` (le kubeconfig complet, clé privée incluse — pas qu'une « référence » malgré le nom) n'avait pas le tag `json:"-"` contrairement à `SSHPassword` — `GET /api/v1/clusters` le renvoyait donc en clair. Découvert en listant les clusters pendant ce smoke test (un enregistrement portait un kubeconfig Teleport multi-clusters personnel). Corrigé (`json:"-"`), vérifié que l'API ne renvoie plus le champ
- [x] **Deploy d'un manifest (server-side apply)** (2026-08-02) — dernière brique du Pilotage MVP. `POST /api/v1/clusters/:id/manifests/apply` (`{"manifest", "namespace"?, "dry_run"?, "force"?}`), mécanisme natif `kubectl apply --server-side` (PATCH `types.ApplyPatchType`, field manager `kubepilot`). Un ou plusieurs documents YAML traités indépendamment (comme `kubectl apply -f multi.yaml`) — un document invalide n'empêche pas les autres. Nouveau `internal/k8sops/manifest.go` (dynamic client + RESTMapper construits depuis `CollectorManager.GetRESTConfig`, déjà exposé pour Helm) : **aucune nouvelle dépendance, aucune migration, aucun changement RBAC** — le binding `cluster-admin` déjà accordé pour Helm (`rbac.helmAdmin`) couvre déjà ce cas (même surface arbitraire qu'un chart). UI : page Deploy (`/deploy`, sidebar `operator`/`admin`), preview dry-run puis apply. Testé (dynamic client factice + `testrestmapper`, y compris un contournement documenté d'une limitation connue du fake client de client-go pour `ApplyPatchType`) puis validé sur Lab Cyllene (namespace jetable, ConfigMap + Deployment nginx) : dry-run confirmé sans effet réel (contrairement au client factice, l'API server respecte bien `DryRun`), apply réel confirmé `created`, ré-apply confirmé `updated`, document sans namespace correctement isolé en erreur par-document sans bloquer le reste. Namespace jetable nettoyé après coup
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

### 3. Pilotage MVP *(cœur de la vision, via l'API server)* ✅
- [x] Deploy d'un manifest (server-side apply)
- [x] Édition `values.yaml` Helm + upgrade/rollback (Helm SDK Go)
- [x] Scale / rolling-restart — `edit ressource` (générique) reste à faire
- [x] Journalisation des actions (`action_logs`) + garde-fous (rôles) — branché sur scale/restart, helm upgrade/rollback et deploy manifest

### 4. Plus tard — Agent hôte *(ancre in-cluster)*
- [ ] Inventaire OS/packages des nœuds (DaemonSet)
- [ ] Scan CVE **paquets système** (kernel, openssl, glibc…) → alimente le scoring
- [ ] Actions hôte (patch OS, reboot, drain) — séparable, V2/V3

### En continu
- [~] Tests backend — `internal/watcher/tags_test.go` (semver), `internal/store/action_logs_test.go`, `internal/k8sops/workloads_test.go` (clientset factice), `internal/netguard/netguard_test.go` (SSRF) faits ; **2026-08-06** : `internal/scoring/engine_test.go` (formule complète contre les cas par défaut et le cas annoté/environnement, calculée à la main et recoupée avec le code — voir décision ci-dessous sur l'écart docs/scoring.md ; placeholder fenêtre de maintenance épinglé ; `ScoreAll` ignore bien les findings non actifs), `internal/store/findings_test.go` (non-régression `first_detected_at` préservé au conflit, `ResolveActiveFindingForImage` épargne un finding `ignored`, jointures `ListFindings`), `internal/store/workloads_test.go` (non-régression directe sur le bug UUID fantôme du 2026-08-01) ; **reste** le reste du store (clusters, helm releases/repos, registries, namespaces, métriques, secrets)
- [x] Page Settings (utilisateurs, infos système)
- [ ] Page History (audit log)
- [x] UI registries privés (Harbor, ECR, GCR, ACR) + dépôts de charts Helm

---

## 💡 Idées à explorer *(pas encore cadrées — à affiner avant de séquencer)*

- **Intégration Zabbix** (2026-08-02) — idée à chaud, direction pas encore tranchée :
  - *Sortant* : pousser les findings / le score de risque comme items/triggers Zabbix (alerting unifié avec le reste du homelab déjà sous Zabbix ?)
  - *Entrant* : consommer les hôtes/métriques déjà suivis par Zabbix comme source d'inventaire complémentaire (recoupement avec les nœuds K8s ?)
  - À rapprocher de « Notifications (Slack, webhook, email digest) » et « Intégration Argo CD (lecture seule) » déjà en V2 (`CLAUDE.md`) — même famille (intégration système externe). Reste à clarifier : sens du flux, tier concerné (in-cluster only ?), authentification API Zabbix.
  - **Affiné le 2026-08-06** : voir l'idée « Relais enrichi Alertmanager ↔ Zabbix » ci-dessous, qui propose une troisième direction (ni findings→Zabbix, ni inventaire→KubePilot) pour le *sortant*.

- **Relais enrichi Alertmanager (Karma) ↔ Zabbix, ack/downtime bidirectionnel** (2026-08-06) — idée à chaud, **V2 ou V3** (pas encore séquencée). Contexte : les devops utilisent le dashboard Karma (prymitive/karma) au-dessus d'Alertmanager ; le besoin est de faire remonter ces alertes dans Zabbix (déjà le hub d'alerting du homelab) et de pouvoir ack/downtime depuis Zabbix en répercutant l'état vers Alertmanager — pas juste un passthrough.
  - *Aller* (Alertmanager → KubePilot → Zabbix) : cohérent avec le pattern watcher existant (`ImageWatcher`/`HelmWatcher`) — poller `/api/v2/alerts` d'Alertmanager (Karma lui-même n'a pas d'alertes propres, il ne fait que lire Alertmanager), enrichir avec l'inventaire déjà en base (cluster/namespace/workload/score de risque), puis pousser vers Zabbix. Zabbix n'a pas d'API « créer un event » à la volée : il faut des trapper items pré-provisionnés (par host/clé) + un trigger qui bascule en PROBLEM à réception — donc un peu de templating côté Zabbix, pas qu'un appel API.
  - *Retour* (ack/downtime Zabbix → KubePilot → Alertmanager) : nécessite un **premier endpoint webhook entrant** dans KubePilot (tout ce qui existe aujourd'hui est sortant : Slack/PagerDuty/Teams) — appelé par une Action Zabbix (media type webhook) sur ack/maintenance. KubePilot traduit ça en silence Alertmanager (`POST /api/v2/silences`, matchers sur les labels d'origine, durée = fenêtre de downtime si c'est une maintenance).
  - **Mapping sémantique pas 1:1** à assumer explicitement : un ack Zabbix ne coupe pas les notifications, une silence Alertmanager si — c'est l'équivalent le plus proche, pas un vrai équivalent.
  - Plus gros que l'intégration Zabbix « simple » déjà notée : deux flux nouveaux dont un inbound inédit dans le codebase. À rapprocher de « Notifications » (V2, `CLAUDE.md`) pour le sens sortant, mais le retour ack/downtime dépasse ce cadre.

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
- **Piège Go — nil typé dans une interface** (`cmd/server/main.go`, câblage `ImageChecker`/`HelmChecker`) : `imgWatcher`/`helmWatcher` sont `nil` en mode démo. Les passer directement en tant qu'interface (`var imageChecker handlers.ImageChecker = imgWatcher`) produirait une interface **non-nil** (type concret `*ImageWatcher`, valeur nil) — le garde `if h.imageChecker == nil` d'un handler ne l'intercepterait pas, et l'appel de méthode paniquerait. Toujours passer par un `if ptr != nil { iface = ptr }` explicite pour garder l'interface elle-même à nil.
- **Findings — `store.IsActiveFindingStatus`** (`open`/`planned`/`approved`) définit maintenant ce qu'une action de remédiation (ou l'auto-résolution du watcher) accepte comme statut de départ. Toute nouvelle action d'écriture sur un finding doit vérifier ce statut plutôt que d'inventer sa propre liste.
- **SSRF — pourquoi RFC1918 reste autorisé** (`internal/netguard`) : bloquer les plages privées casserait l'usage principal de l'outil (registries/dépôts Helm auto-hébergés sur un LAN, ex. Harbor à `10.0.60.152`). Le blocage cible loopback/link-local/metadata/multicast — la vraie menace (pivot interne, métadonnées cloud), pas le réseau privé en général. Toute future requête sortante construite depuis une donnée non fiable (registre, URL de dépôt, webhook...) doit passer par `netguard.NewHTTPClient`, pas un `http.Client{}` nu.
- **`docs/scoring.md` vs `internal/scoring/engine.go` — écart trouvé le 2026-08-06, comblé le même jour** (« Implémente ces briques » — les 3 lacunes fonctionnelles sont closes ; voir `CHANGELOG.md` pour le détail) :
  - CVSS (poids 20 %, câblé à 0 en dur) → lit désormais `UpdateFinding.CVEs`, prend le max, formule `(cvss/10)×20` exacte. Personne n'alimente encore `CVEs` (Trivy/Grype reste V2) : la brique est prête, pas encore nourrie.
  - `isWindowActive` (toujours `false`) → évaluation cron réelle (`github.com/robfig/cron/v3`, nouvelle dépendance), fuseau horaire + durée pris en compte.
  - `ExceptionRule` (aucune logique, **et aucune colonne `rule_type`** malgré la doc §9) → modèle complété (`Name`, `RuleType`, `NamespaceName` — migration `011_exception_rule_type.sql`), scope matching + les 3 effets (`suppress`/`reduce_severity`/`accept_risk`) implémentés dans `ScoreFinding`. Endpoint REST + page **Exceptions** ajoutés le 2026-08-21 (voir entrée « Fait récemment » et `CHANGELOG.md`).
  - `internal/scoring/engine_test.go` reproduit maintenant l'Example 1 chiffré de la doc (§8) exactement — score 82.6, critical — bout en bout, preuve que le code suit la doc et pas l'inverse.
  - **Toujours pas réconcilié** (mineur, pas une des 3 briques demandées) : clé d'annotation `kubepilot/criticality` dans le code vs `kubepilot.io/criticality` dans la doc (`.io` manquant) ; signature `Compute(ctx, FindingContext)` pure de la doc §12 vs la vraie méthode `ScoreFinding(finding, workload, cluster)` qui écrit en base directement — les deux sont des divergences de forme, pas de comportement, laissées telles quelles.
- **Auth — le rôle vient de la base à chaque requête, plus des claims JWT** (`middleware.JWTAuth` + `store.ActiveUserRole`) : un changement de rôle ou une désactivation via `PATCH /auth/users/:id` doit s'appliquer immédiatement, pas seulement à l'expiration du token (24h). Coût : un lookup DB par requête authentifiée — acceptable à l'échelle homelab de cet outil, pas nécessairement à revisiter tant que ça ne devient pas un goulot mesuré.

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
- **Versioning** : SemVer + cycle alpha/beta/rc/stable (cf. CLAUDE.md). Version courante : `v0.2.0-alpha.10`.
- **Commits** : Conventional Commits (`type(scope): message`).
