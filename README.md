# KubePilot

Cockpit d'exploitation SysOps pour Kubernetes — suivi des mises à jour, scoring des risques, inventaire multi-cluster.

---

## Présentation

KubePilot répond à une question quotidienne : **"Parmi tout ce qui tourne sur mes clusters, qu'est-ce qui représente un risque réel aujourd'hui ?"**

L'outil agrège l'état de vos clusters Kubernetes, détecte les images et charts Helm en retard de version, calcule un score de priorité contextualisé (environnement, criticité, ancienneté, CVEs), et permet à l'opérateur de trier, planifier et documenter les mises à jour — sans script maison, sans tableur croisé.

KubePilot s'intègre avec [Headlamp](https://headlamp.dev) via des liens profonds : navigation bas niveau (logs, exec, resource editor) dans Headlamp, gouvernance des versions dans KubePilot.

---

## Fonctionnalités MVP

| Domaine | Fonctionnalité |
|---|---|
| **Inventaire** | Clusters, namespaces, workloads (Deployment, DaemonSet, StatefulSet), nodes |
| **Détection updates** | Images OCI (polling registry), releases Helm (index.yaml) |
| **Scoring** | 7 facteurs pondérés × multiplicateur environnement × exposition |
| **Triage** | Statuts planned / ignored / approved / blocked par finding |
| **Navigation** | Lien profond vers Headlamp par ressource |
| **Multi-cluster** | Sélecteur global, filtres par cluster/environnement |
| **Temps réel** | K8s Watch API pour la collecte, SSE pour le push frontend |
| **Auth** | JWT local, endpoint `/auth/setup` pour le premier compte admin |
| **Déploiement** | Helm chart in-cluster, docker-compose pour le dev local |

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    KUBEPILOT PLATFORM                   │
│                                                         │
│  React Frontend          Headlamp Plugin (léger)        │
│  (Overview, Updates,  ←──── widget Update Status        │
│   Inventory, Helm,          + deep links)               │
│   Nodes, Intégrations)                                  │
│         │ REST + SSE                                     │
│  ┌──────▼────────────────────────────────────────┐      │
│  │           Go Backend API (Gin)                │      │
│  │  /api/v1/clusters  /findings  /workloads      │      │
│  │  /helm  /nodes  /overview  /auth  /events     │      │
│  └──────┬──────────────────────┬─────────────────┘      │
│         │                      │                        │
│  ┌──────▼──────┐    ┌──────────▼───────────────────┐   │
│  │ PostgreSQL  │    │        Worker Pool             │   │
│  │             │    │  K8s Collector (Watch API)     │   │
│  │  clusters   │◄───│  Image Watcher (OCI registry) │   │
│  │  workloads  │    │  Helm Watcher (index.yaml)     │   │
│  │  findings   │    │  Scoring Engine                │   │
│  │  scores     │    └──────────────────────────────  ┘   │
│  └─────────────┘                                        │
│  ┌─────────────┐    ┌──────────────────────────────┐    │
│  │    Redis    │    │       Connecteurs externes     │    │
│  │  tag cache  │    │  K8s API · OCI Registries     │    │
│  │  sessions   │    │  Helm repos · (Argo CD V2)    │    │
│  └─────────────┘    └──────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

---

## Stack technique

| Composant | Technologie |
|---|---|
| Backend | Go 1.22, Gin, GORM, client-go, Masterminds/semver |
| Frontend | React 18, TypeScript, Vite, Tailwind CSS, Tanstack Query |
| Base de données | PostgreSQL 15 |
| Cache | Redis 7 |
| Déploiement | Helm chart (namespace `kubepilot`), Docker Compose (dev) |

---

## Démarrage rapide

### Dev local

```bash
# 1. Démarrer PostgreSQL + Redis
make dev-infra

# 2. Configurer le backend
cp backend/.env.example backend/.env

# 3. Démarrer le backend  (dans un terminal)
make dev-backend
# → http://localhost:8080/health

# 4. Démarrer le frontend (dans un autre terminal)
make dev-frontend
# → http://localhost:3000
```

Au premier démarrage, un compte `admin@kubepilot.local` est créé automatiquement.  
Si `ADMIN_PASSWORD` n'est pas défini, le mot de passe est généré et affiché dans les logs du backend.

### Déploiement Kubernetes (GitOps)

Le chart Helm vit dans le dépôt **`kubepilot-gitops`** (`charts/kubepilot/`), avec les valeurs
par environnement et les Applications ArgoCD. Un push sur `dev` déclenche Jenkins, qui build et
pousse les images sur Harbor puis met à jour le tag dans `envs/dev/values.yaml` — ArgoCD
synchronise. Il n'y a **rien à installer à la main**.

Voir [docs/installation.md](docs/installation.md) pour la chaîne complète.

---

## Documentation

| Document | Contenu |
|---|---|
| [docs/installation.md](docs/installation.md) | Procédure d'installation complète (dev local, Helm, multi-cluster) |
| [docs/architecture.md](docs/architecture.md) | Architecture hybride, flux de données, stratégie temps réel |
| [docs/data-model.md](docs/data-model.md) | Modèle de données — 19 entités avec attributs et relations |
| [docs/api-spec.md](docs/api-spec.md) | Spécification REST API (14 groupes d'endpoints + SSE) |
| [docs/scoring.md](docs/scoring.md) | Modèle de scoring — formule, pondérations, exemples chiffrés |
| [docs/development.md](docs/development.md) | Guide développeur — structure du code, tests, variables d'environnement |

---

## Structure du projet

```
KubePilot/
├── backend/                  Go backend (API + workers)
│   ├── cmd/server/           Point d'entrée
│   ├── internal/
│   │   ├── api/              Handlers Gin + middleware JWT
│   │   ├── bootstrap/        First-run : admin + cluster local
│   │   ├── collector/        K8s Watch collector par cluster
│   │   ├── config/           Configuration depuis env vars
│   │   ├── models/           Modèles GORM
│   │   ├── scoring/          Moteur de scoring des risques
│   │   ├── store/            Couche d'accès base de données
│   │   └── watcher/          Image watcher + Helm watcher
│   └── migrations/           Migration SQL initiale
├── frontend/                 React 18 + TypeScript
│   └── src/
│       ├── api/              Client axios typé
│       ├── components/       Composants réutilisables
│       ├── contexts/         AuthContext, ClusterContext
│       ├── hooks/            useFindings, useClusters, useSSE
│       ├── pages/            Overview, Updates, Inventory, Helm, Nodes…
│       └── types/            Types TypeScript des domaines
├── docs/                     Documentation
├── docker-compose.yml        Stack locale (PG + Redis + backend + frontend)
└── Makefile                  Commandes de développement
```

---

## Roadmap

**V2** — CVE scanning (Trivy/Grype), plugin Headlamp actif, inventaire OS/packages, Argo CD, maintenance windows, notifications Slack/webhook, RBAC multi-rôles.

**V3** — Génération PR/MR GitOps, déclenchement Ansible, Proxmox/Docker standalone, multi-tenant, SLA tracking.

---

## Licence

À définir.
