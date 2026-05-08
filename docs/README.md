# KubePilot Documentation

KubePilot is a SysOps cockpit combining Kubernetes visibility with an update tracking and risk scoring engine. It follows a hybrid architecture: a standalone SysOps Cockpit web application backed by a Go API, plus a lightweight Headlamp plugin for in-cluster Kubernetes navigation.

---

## Quick Start

Pour installer KubePilot, consulter la [Procédure d'installation](./installation.md). Elle couvre le développement local, le déploiement Helm in-cluster et la connexion de clusters distants.

Pour contribuer au code, consulter le [Guide développeur](./development.md).

---

## Documentation Index

### [Installation](./installation.md)

Procédure d'installation complète : développement local avec docker-compose, déploiement in-cluster via Helm, connexion de clusters supplémentaires, récupération du token d'intégration K8s, référence des variables d'environnement et dépannage courant.

### [Architecture](./architecture.md)

The complete architectural overview of KubePilot. Covers the rationale for the hybrid architecture (standalone cockpit + Headlamp plugin), ASCII component diagrams, data flows for all major operations, the real-time event strategy using K8s Watch API and Server-Sent Events, the Helm-based in-cluster deployment model, and how the Headlamp plugin integrates via deep links and the KubePilot REST API.

Read this first if you are trying to understand how the system fits together before writing any code.

---

### [Data Model](./data-model.md)

Complete documentation for every entity in the PostgreSQL database. For each entity you will find:

- Its role in the system
- All fields with Go/SQL types, nullability, and constraints
- Relationships to other entities
- Database indexes
- How the entity is surfaced in the UI

Includes a full entity-relationship diagram in ASCII art. Essential reading before modifying the schema or adding new collectors.

---

### [API Specification](./api-spec.md)

The full REST API reference for the KubePilot backend (`/api/v1`). Documents every endpoint group:

- Clusters, Workloads, Findings (updates), Nodes, Helm Releases
- Risk Scores, Overview aggregations
- Integration Accounts
- Health and readiness probes
- Auth (local JWT + refresh)
- SSE real-time event stream (`GET /events`)

Each endpoint entry includes HTTP method, path, description, request body schema, response schema, and relevant error codes. Use this as the contract between frontend and backend.

---

### [Development Guide](./development.md)

Step-by-step instructions for setting up a local development environment:

- Prerequisites (Go 1.22+, Node 20+, Docker, kubectl, helm)
- Starting PostgreSQL and Redis via docker-compose
- Running the Go backend with environment variable reference
- Running the React frontend with Vite
- Applying database migrations
- Connecting a local Kubernetes cluster via kubeconfig
- Running backend and frontend tests
- Full codebase structure explanation (package by package)
- Common development tasks and tips

---

### [Scoring Model](./scoring.md)

Detailed specification of the risk scoring engine. Documents:

- The scoring formula: weighted factor sum multiplied by environment and exposure multipliers, normalized to 0–100
- Severity thresholds (Critical, High, Medium, Low, Info)
- Every scoring factor with its possible values, raw scores, and weight in the final formula
- Environment multipliers (prod → dev)
- Exposure multipliers (external ingress, internal service, headless)
- Three fully worked calculation examples
- How ExceptionRule records affect scores
- Special handling for `latest` tags (digest-only comparison)

---

## Project Layout (Top Level)

```
kubepilot/
├── cmd/
│   └── server/          # Go binary entrypoint
├── internal/            # Go packages (api, workers, scoring, db, …)
├── frontend/            # React + TypeScript SPA
├── plugin/              # Headlamp plugin (TypeScript)
├── migrations/          # SQL migration files
├── helm/                # Helm chart for in-cluster deployment
├── docker-compose.yml   # Local dev dependencies
└── docs/                # This documentation
```

---

## Contributing

Before contributing, read the [Architecture](./architecture.md) doc to understand the design principles, then the [Development Guide](./development.md) to get your environment running. All new backend endpoints must be reflected in [api-spec.md](./api-spec.md). Schema changes must be reflected in [data-model.md](./data-model.md).
