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

- [x] Métriques système des nœuds — agentless via kubelet Summary API, time-series `node_metrics`, jauges + sparklines (tier desktop)
- [x] Vue globale ressources système sur le dashboard (style Proxmox/vCenter) — jauges CPU/RAM/disque agrégées + par cluster
- [x] Mode démo (`DEMO_MODE=true`) — seed multi-cluster, collectors désactivés, login `admin@kubepilot.local` / `demo`
- [x] Décision de packaging desktop/in-cluster

---

## 🔜 Séquencement proposé

### 1. Maintenant — Rendre les findings réels *(tier desktop, débloque l'utilité)*
> Sans ça l'app reste une vitrine d'inventaire (cf. CLAUDE.md, marqué bloquant).
- [ ] Auth Docker Hub : corriger le token anonyme (renvoie 401)
- [ ] TLS/CA par registre (Harbor self-signed, `TLS_INSECURE` par registre)
- [ ] Nettoyage en cascade des `container_images` orphelines au `DeleteWorkloadsNotSeenSince`
- [ ] Vérifier de bout en bout qu'un finding réel remonte (image + helm)

### 2. Prérequis tiering — Chart Helm
> ⚠️ Listé « livré » dans CLAUDE.md mais **absent du repo** (`helm/`). Tout le tier in-cluster en dépend.
- [ ] (Re)créer le chart `helm/kubepilot/` (deployment, SA, RBAC/ClusterRole, secret, ingress, NOTES)
- [ ] RBAC : autoriser `get nodes/proxy` (+ `nodes/stats`) pour les métriques en in-cluster
- [ ] Vérifier déploiement in-cluster (collecte persistante + historique)

### 3. Ensuite — Pilotage MVP *(cœur de la vision, via l'API server)*
- [ ] Deploy d'un manifest (server-side apply)
- [ ] Édition `values.yaml` Helm + upgrade/rollback (Helm SDK Go)
- [ ] Scale / rolling-restart / edit ressource
- [ ] Journalisation des actions (`action_logs`) + garde-fous (rôles)

### 4. Plus tard — Agent hôte *(ancre in-cluster)*
- [ ] Inventaire OS/packages des nœuds (DaemonSet)
- [ ] Scan CVE **paquets système** (kernel, openssl, glibc…) → alimente le scoring
- [ ] Actions hôte (patch OS, reboot, drain) — séparable, V2/V3

### En continu
- [ ] Tests backend (scoring engine, semver, store)
- [ ] Page Settings (utilisateurs, variables globales)
- [ ] Page History (audit log)
- [ ] UI registries privés (Harbor, ECR, GCR, ACR)

---

## 🧠 Décisions & notes

- Métriques nœuds = **agentless** (kubelet), passe par le tunnel SSH ; l'agent custom n'est utile que pour l'inventaire OS/packages + CVE système + actions (→ in-cluster).
- Statuts findings valides : `open, planned, ignored, approved, blocked, resolved`.
- UUID assignés côté Go (callback GORM), pas `gen_random_uuid()` (compat SQLite).
- Mode démo : ne **jamais** lancer les collectors/watchers/scoring (ils écraseraient/purgeraient le seed).

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
- **Versioning** : SemVer + cycle alpha/beta/rc/stable (cf. CLAUDE.md). Version courante : `v0.2.0-alpha.1`.
- **Commits** : Conventional Commits (`type(scope): message`).
