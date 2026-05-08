# KubePilot — Architecture

## Table of Contents

1. [Vision](#1-vision)
2. [Hybrid Architecture Rationale](#2-hybrid-architecture-rationale)
3. [Component Overview](#3-component-overview)
4. [Component Diagram](#4-component-diagram)
5. [Data Flows](#5-data-flows)
6. [Real-Time Strategy](#6-real-time-strategy)
7. [Deployment Model](#7-deployment-model)
8. [Headlamp Integration](#8-headlamp-integration)
9. [Security Considerations](#9-security-considerations)

---

## 1. Vision

KubePilot gives platform engineers and SREs a single cockpit to answer three questions across all their Kubernetes clusters:

1. **What is running?** — workloads, images, Helm releases, nodes, namespaces.
2. **What needs updating?** — container image tag changes, new chart versions, K8s node upgrades.
3. **How risky is each update?** — risk score computed from update type, age, CVE surface, environment criticality, service exposure, and operational context.

The system is deliberately opinionated: every observable update becomes a *Finding*, every Finding gets a *RiskScore*, and the operator workflow (plan → approve → ignore → block) is tracked with a full audit log.

---

## 2. Hybrid Architecture Rationale

### Why not a Headlamp-plugin-only approach?

A pure Headlamp plugin runs in the browser and calls the Kubernetes API directly through the Headlamp backend proxy. This works well for real-time resource inspection but has fundamental limitations for KubePilot's use case:

- **Cross-cluster aggregation** — Headlamp manages one cluster context at a time. Aggregating findings across 10 clusters requires a dedicated backend.
- **Persistent state** — Update findings, risk scores, ownership metadata, exception rules, and audit logs must survive browser sessions and be shared across team members. A browser-side plugin has no persistent store.
- **Background workers** — Polling OCI registries, watching Helm repositories, running the scoring engine, and scheduling jobs require a long-running server process, not a plugin.
- **Rate-limit management** — OCI registry polling must respect rate limits and cache results. A per-browser approach would create one polling loop per open tab.
- **Notification and integration layer** — Sending Slack alerts or creating Jira tickets requires server-side credentials that must not be exposed to the browser.

### Why not a standalone-only approach?

A fully standalone web app without any Headlamp integration loses the best-in-class Kubernetes navigation that Headlamp provides:

- Headlamp already handles multi-cluster kubeconfig management, RBAC-aware resource browsing, log streaming, exec sessions, and CRD visualization.
- Rewriting all of that in a bespoke frontend would take months and produce an inferior result.
- Platform teams that already run Headlamp would have to context-switch between two tools.

### The hybrid decision

KubePilot ships as two complementary surfaces:

| Surface | Role |
|---|---|
| **SysOps Cockpit** (standalone SPA) | Update tracking, risk scores, action workflow, cross-cluster overview |
| **Headlamp Plugin** (lightweight) | In-cluster navigation with an update-status widget that deep-links back to the Cockpit |

The plugin reads data from the KubePilot API. It never duplicates backend logic. The Cockpit is the authoritative surface for anything requiring persistent state or cross-cluster context.

---

## 3. Component Overview

### Frontend (SysOps Cockpit)

- React 18 + TypeScript + Vite
- UI component library: shadcn/ui (Radix + Tailwind)
- State management: TanStack Query (server state) + Zustand (UI state)
- Real-time updates via SSE (`EventSource`)
- Routing: React Router v6
- Charts: Recharts
- Talks exclusively to the KubePilot Go backend via `/api/v1`

### Backend API (Go)

- Go 1.22+, Chi router, standard `net/http`
- JWT authentication (RS256), middleware stack: logging, recovery, CORS, rate-limiting
- Business logic organized into domain packages under `internal/`
- Exposes REST endpoints and one SSE endpoint
- Manages the worker pool lifecycle

### Worker Pool

Five background workers run as goroutines managed by the backend process:

| Worker | Responsibility |
|---|---|
| **K8s Collector** | Watches cluster resources via the K8s Watch API; upserts Namespaces, Nodes, Workloads, ContainerImages, HelmReleases into PostgreSQL |
| **Image Watcher** | Polls OCI registries for new tags on all observed images; writes ImageTagObservation records; triggers scoring |
| **Helm Watcher** | Polls configured Helm repositories for new chart versions; compares with deployed releases; creates Findings |
| **Scoring Engine** | Recomputes RiskScore records when Findings or context data change; publishes score-change events to Redis |
| **Job Scheduler** | Cron-like scheduler that triggers periodic full re-syncs, maintenance-window checks, and notification dispatches |

### PostgreSQL

Primary persistent store. Stores all entities described in the data model. Schema managed by sequential SQL migrations under `migrations/`.

### Redis

Used for:
- SSE fan-out: backend publishes events to a Redis Pub/Sub channel; SSE handler subscribes and forwards to connected browsers
- Worker coordination: distributed locks for singleton workers in multi-replica deployments
- Short-lived cache for OCI registry tag lists (TTL: 5 minutes by default)

### Headlamp Plugin

- TypeScript plugin loaded by the Headlamp shell
- Registers a sidebar section and a per-workload detail tab
- Calls `GET /api/v1/workloads?uid=<k8s-uid>` to fetch update status for the currently viewed resource
- Renders a compact update-status widget (severity badge + finding count + link to Cockpit)
- No write operations; read-only consumer of the KubePilot API

### External Connectors

| Connector | Protocol | Direction |
|---|---|---|
| Kubernetes API | Watch + REST (client-go) | Inbound (pull) |
| OCI Registries | HTTPS (registry v2 API) | Inbound (pull) |
| Helm Repositories | HTTPS (index.yaml) | Inbound (pull) |
| Argo CD API | REST | Inbound (pull) |
| Slack | Webhook / Web API | Outbound (push) |
| PagerDuty | Events API v2 | Outbound (push) |
| Jira | REST API v3 | Outbound (push) |

---

## 4. Component Diagram

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                              User's Browser                                      │
│                                                                                  │
│  ┌──────────────────────────────────┐    ┌──────────────────────────────────┐   │
│  │      SysOps Cockpit (SPA)        │    │      Headlamp Shell              │   │
│  │  React + TanStack Query          │    │  ┌────────────────────────────┐  │   │
│  │  SSE EventSource ─────────────────────►  │  KubePilot Headlamp Plugin │  │   │
│  │                                  │    │  │  (TypeScript, read-only)   │  │   │
│  │  /overview  /findings  /clusters │    │  └────────────┬───────────────┘  │   │
│  └──────────────┬───────────────────┘    └───────────────┼──────────────────┘   │
└─────────────────┼─────────────────────────────────────────┼────────────────────┘
                  │ REST /api/v1                             │ REST /api/v1
                  ▼                                          ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          KubePilot Backend (Go)                                 │
│                                                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────┐   │
│  │  HTTP Server  (Chi router, JWT middleware, CORS, rate-limit)             │   │
│  │  REST Handlers  +  SSE Handler  +  Auth Handlers                        │   │
│  └──────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
│  ┌──────────────────────────────────────────────────────────────────────────┐   │
│  │  Worker Pool                                                             │   │
│  │                                                                          │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────────┐  │   │
│  │  │  K8s Collector  │  │  Image Watcher  │  │      Helm Watcher       │  │   │
│  │  │  (client-go)    │  │  (OCI v2 API)   │  │  (index.yaml polling)   │  │   │
│  │  └────────┬────────┘  └────────┬────────┘  └───────────┬─────────────┘  │   │
│  │           │                    │                        │                │   │
│  │  ┌────────▼──────────────────────────────────────────── ▼─────────────┐  │   │
│  │  │               Scoring Engine  (recomputes on change)               │  │   │
│  │  └─────────────────────────────────────────────────────────────────────┘  │   │
│  │                                                                          │   │
│  │  ┌──────────────────────────────────────────────────────────────────┐   │   │
│  │  │  Job Scheduler  (cron: full re-sync, maintenance windows, alerts) │   │   │
│  │  └──────────────────────────────────────────────────────────────────┘   │   │
│  └──────────────────────────────────────────────────────────────────────────┘   │
│                                                                                  │
│  ┌────────────────────────────┐    ┌──────────────────────────────────────┐     │
│  │  PostgreSQL                │    │  Redis                               │     │
│  │  (primary data store)      │    │  (SSE pub/sub + worker locks + cache)│     │
│  └────────────────────────────┘    └──────────────────────────────────────┘     │
└───────┬──────────────────┬────────────────────────┬────────────────────────────┘
        │                  │                         │
        ▼                  ▼                         ▼
┌───────────────┐  ┌────────────────────┐  ┌────────────────────────────────────┐
│ Kubernetes    │  │  OCI Registries    │  │  Helm Repositories                 │
│ API Server    │  │  (Docker Hub,      │  │  (stable, bitnami, custom)         │
│ (Watch + REST)│  │  GHCR, ECR, …)    │  │                                    │
└───────────────┘  └────────────────────┘  └────────────────────────────────────┘
        │
        └── Argo CD API (optional, for app sync status)
```

---

## 5. Data Flows

### 5.1 Kubernetes Collection Flow

```
K8s API Server
    │
    │  Watch stream (client-go informer)
    ▼
K8s Collector goroutine
    │
    ├── Parse resource event (ADDED / MODIFIED / DELETED)
    │
    ├── Upsert Namespace / Node / Workload / ContainerImage / HelmRelease
    │   └── PostgreSQL (ON CONFLICT DO UPDATE)
    │
    ├── Detect image tag changes vs. previous observation
    │   └── Enqueue image tag lookup task → Image Watcher queue
    │
    └── Publish "workload.updated" event → Redis Pub/Sub
            │
            ▼
        SSE Handler fans out to all connected browser clients
```

### 5.2 Image Update Detection Flow

```
Image Watcher goroutine (triggered by queue OR periodic scheduler)
    │
    ├── Check Redis cache for recent tag list (TTL 5 min)
    │   ├── Cache HIT  → use cached list
    │   └── Cache MISS → call OCI Registry v2 API (GET /v2/{repo}/tags/list)
    │                     → store result in Redis with TTL
    │
    ├── Compare available tags against current deployed tag
    │   ├── Semver comparison (major/minor/patch classification)
    │   └── For "latest" tag: compare digest (GET /v2/{repo}/manifests/latest)
    │
    ├── If newer tag found:
    │   ├── Upsert ImageTagObservation
    │   ├── Upsert UpdateFinding (kind=IMAGE, status=open)
    │   └── Enqueue scoring task → Scoring Engine queue
    │
    └── Publish "finding.created" or "finding.updated" → Redis Pub/Sub
```

### 5.3 Frontend Query Flow

```
Browser (SysOps Cockpit)
    │
    │  GET /api/v1/findings?severity=critical&status=open
    ▼
HTTP Handler
    │
    ├── Validate JWT
    ├── Parse query params → build SQL WHERE clause
    ├── Execute SELECT with JOIN (findings + risk_scores + workloads + clusters)
    ├── Serialize response JSON
    └── Return 200 with paginated result

Browser also holds open:
    GET /api/v1/events  (SSE)
        │
        ▼
    SSE Handler subscribes to Redis Pub/Sub channel "kubepilot:events"
        │
        └── Forwards events as "data: {...}\n\n" to EventSource
```

### 5.4 Operator Action Flow

```
Browser operator clicks "Mark as Planned"
    │
    │  PATCH /api/v1/findings/{id}/status  { "status": "planned" }
    ▼
HTTP Handler
    │
    ├── Validate JWT + RBAC (role must be editor or admin)
    ├── Update findings.status + status_changed_by + status_changed_at
    ├── Insert ActionLog record (user, action=FINDING_STATUS_CHANGE, payload)
    ├── Publish "finding.status_changed" event → Redis Pub/Sub
    └── Return 200 with updated Finding

Redis Pub/Sub → SSE Handler → all connected browsers receive the update
```

---

## 6. Real-Time Strategy

### Collection side: Kubernetes Watch API

The K8s Collector uses `client-go` informers which maintain a Watch connection (HTTP chunked transfer) to the API server. Each resource type (Deployments, StatefulSets, DaemonSets, Nodes, Namespaces, Pods) has its own informer. Informers handle reconnection, list+watch bootstrapping, and resource-version tracking automatically.

This means the database is updated within seconds of any change in the cluster, with no polling latency for K8s resources.

### Frontend push: Server-Sent Events

The backend exposes `GET /api/v1/events`, which upgrades the connection to an SSE stream. The SSE handler:

1. Subscribes to the `kubepilot:events` Redis Pub/Sub channel.
2. Forwards every message as a typed SSE event (e.g., `event: finding.created`).
3. Sends a keepalive comment (`: keepalive`) every 30 seconds to prevent proxy timeouts.
4. On client disconnect, unsubscribes and releases the goroutine.

Event types pushed via SSE:

| Event | Payload |
|---|---|
| `workload.updated` | `{ cluster_id, workload_id, kind, name, namespace }` |
| `finding.created` | `{ finding_id, severity, target_kind, target_id }` |
| `finding.updated` | `{ finding_id, severity, status }` |
| `finding.status_changed` | `{ finding_id, old_status, new_status, changed_by }` |
| `score.updated` | `{ finding_id, score, severity }` |
| `cluster.status_changed` | `{ cluster_id, old_status, new_status }` |
| `sync.completed` | `{ cluster_id, duration_ms }` |

TanStack Query on the frontend listens to SSE events and calls `queryClient.invalidateQueries()` for the relevant query keys, triggering a background refetch.

### Why SSE over WebSockets?

- SSE is unidirectional (server → client), which matches the use case exactly. Clients send mutations via regular REST calls, not over the event stream.
- SSE is simpler to implement, load-balance, and proxy than WebSockets.
- SSE connections are plain HTTP/1.1, compatible with every reverse proxy and Kubernetes Ingress controller without configuration changes.
- Native browser `EventSource` API handles reconnection automatically.

---

## 7. Deployment Model

KubePilot is deployed in-cluster via a Helm chart located at `helm/kubepilot/`.

### Namespace

All resources are deployed into the `kubepilot` namespace.

### Helm Chart Components

| Chart Component | Kubernetes Resource |
|---|---|
| `kubepilot-api` | Deployment (Go backend) |
| `kubepilot-frontend` | Deployment (Nginx serving built SPA) + ConfigMap (nginx.conf) |
| `kubepilot-postgres` | StatefulSet + PersistentVolumeClaim |
| `kubepilot-redis` | Deployment + Service |
| `kubepilot-migrations` | Job (runs on helm upgrade, applies pending migrations) |
| RBAC | ClusterRole, ClusterRoleBinding, ServiceAccount |
| `kubepilot-ingress` | Ingress (configurable, off by default) |
| Secrets | ExternalSecret or plain Secret for DB credentials, JWT keys |

### RBAC

The `kubepilot-api` ServiceAccount needs the following ClusterRole rules to operate:

```yaml
rules:
  - apiGroups: [""]
    resources: [namespaces, nodes, pods, services, endpoints, configmaps]
    verbs: [get, list, watch]
  - apiGroups: [apps]
    resources: [deployments, statefulsets, daemonsets, replicasets]
    verbs: [get, list, watch]
  - apiGroups: [batch]
    resources: [jobs, cronjobs]
    verbs: [get, list, watch]
  - apiGroups: [helm.sh]
    resources: [releases]
    verbs: [get, list, watch]
```

For multi-cluster deployments, the additional clusters are registered via kubeconfig secrets stored in the `kubepilot` namespace. The K8s Collector for each remote cluster uses those credentials.

### Multi-Replica Considerations

The Job Scheduler and per-cluster collectors use Redis-based distributed locks (`SET NX PX`) to ensure singleton execution when multiple backend replicas are running. The SSE fan-out via Redis Pub/Sub means any replica can serve SSE connections; events from any worker will reach all connected clients.

---

## 8. Headlamp Integration

### Plugin Architecture

The Headlamp plugin (`plugin/`) is a TypeScript package built with the Headlamp plugin SDK. It is loaded by the Headlamp shell on startup.

### Registration Points

1. **Sidebar section** — "KubePilot Updates" item in the Headlamp navigation sidebar, linking to the Cockpit's `/findings` page (opens in new tab).
2. **Workload detail tab** — An additional "Updates" tab injected into every Deployment, StatefulSet, and DaemonSet detail view in Headlamp.

### Workload Updates Tab Behavior

When a user opens a workload detail page in Headlamp, the plugin:

1. Reads the workload's UID from the Headlamp resource object.
2. Calls `GET {KUBEPILOT_API_URL}/api/v1/workloads?uid={uid}` to fetch the workload's findings and score.
3. Renders a compact widget:
   - Severity badge (Critical / High / Medium / Low / None)
   - Count of open findings
   - List of finding summaries (image tag or chart version + update type)
   - "View in KubePilot" button (deep link to `/workloads/{id}` in the Cockpit)

### Configuration

The plugin reads the KubePilot API URL from a ConfigMap or from a Headlamp plugin config entry. The typical value is `https://kubepilot.internal` or the cluster-internal service URL.

### Authentication

When Headlamp is running with OIDC and the user is authenticated, the plugin attaches the bearer token from the Headlamp session to calls to the KubePilot API. KubePilot validates the same OIDC tokens if configured to do so, enabling SSO without a separate login.

---

## 9. Security Considerations

- JWT RS256 tokens: private key stored in a Kubernetes Secret, never in environment variables directly. The backend loads it at startup.
- All inter-service traffic within the cluster uses Kubernetes Services (no external exposure required for backend ↔ DB ↔ Redis).
- Registry credentials are stored in Kubernetes Secrets referenced by `ImageRegistry.credentials_ref`. They are never returned in API responses.
- The `ActionLog` table provides a full audit trail of every operator action with user identity and payload.
- ExceptionRule `expires_at` is enforced by the scoring engine; expired exceptions have no effect.
- Rate limiting is applied at the HTTP middleware layer (per-IP token bucket) and at the registry polling layer (configurable RPS per registry).
