# KubePilot — Development Guide

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Repository Layout](#2-repository-layout)
3. [Local Development Setup](#3-local-development-setup)
4. [Running the Backend](#4-running-the-backend)
5. [Running the Frontend](#5-running-the-frontend)
6. [Database Migrations](#6-database-migrations)
7. [Connecting a Local Kubernetes Cluster](#7-connecting-a-local-kubernetes-cluster)
8. [Running Tests](#8-running-tests)
9. [Environment Variables Reference](#9-environment-variables-reference)
10. [Code Structure](#10-code-structure)
11. [Common Development Tasks](#11-common-development-tasks)

---

## 1. Prerequisites

Before starting, install the following tools:

| Tool | Version | Installation |
|---|---|---|
| Go | 1.22 or later | https://go.dev/dl/ |
| Node.js | 20 LTS or later | https://nodejs.org/ |
| Docker + Docker Compose | Docker 24+ | https://docs.docker.com/get-docker/ |
| kubectl | Any recent version | https://kubernetes.io/docs/tasks/tools/ |
| helm | 3.x | https://helm.sh/docs/intro/install/ |
| golangci-lint | 1.57+ (optional, for linting) | https://golangci-lint.run/usage/install/ |

Verify the key versions:

```bash
go version        # go version go1.22.x linux/amd64
node --version    # v20.x.x
docker compose version  # Docker Compose version v2.x.x
```

---

## 2. Repository Layout

```
kubepilot/
├── cmd/
│   └── server/
│       └── main.go           # Binary entrypoint; wires up DI, starts HTTP server
│
├── internal/
│   ├── api/                  # HTTP handlers and router setup
│   │   ├── router.go         # Chi router, middleware registration
│   │   ├── clusters.go       # /clusters handlers
│   │   ├── workloads.go      # /workloads handlers
│   │   ├── findings.go       # /findings handlers
│   │   ├── nodes.go          # /nodes handlers
│   │   ├── helm.go           # /helm/releases handlers
│   │   ├── scores.go         # /scores handlers
│   │   ├── overview.go       # /overview handler
│   │   ├── integrations.go   # /integrations handlers
│   │   ├── auth.go           # /auth handlers
│   │   ├── sse.go            # /events SSE handler
│   │   └── health.go         # /health handlers
│   │
│   ├── auth/                 # JWT creation, validation, RBAC middleware
│   │   ├── jwt.go
│   │   ├── middleware.go
│   │   └── rbac.go
│   │
│   ├── db/                   # Database layer
│   │   ├── db.go             # pgx pool setup, connection helpers
│   │   ├── queries/          # Type-safe SQL query functions (hand-written, no ORM)
│   │   │   ├── clusters.go
│   │   │   ├── workloads.go
│   │   │   ├── findings.go
│   │   │   └── ...
│   │   └── models/           # Go structs mirroring DB tables
│   │       ├── cluster.go
│   │       ├── workload.go
│   │       └── ...
│   │
│   ├── workers/              # Background worker pool
│   │   ├── pool.go           # Worker lifecycle manager
│   │   ├── collector/        # K8s Collector (client-go informers)
│   │   │   ├── collector.go
│   │   │   ├── deployment.go
│   │   │   ├── statefulset.go
│   │   │   ├── daemonset.go
│   │   │   ├── node.go
│   │   │   └── namespace.go
│   │   ├── imagewatcher/     # OCI registry tag polling
│   │   │   ├── watcher.go
│   │   │   ├── registry.go
│   │   │   └── semver.go
│   │   ├── helmwatcher/      # Helm repo index polling
│   │   │   ├── watcher.go
│   │   │   └── repo.go
│   │   └── scheduler/        # Cron job scheduler
│   │       └── scheduler.go
│   │
│   ├── scoring/              # Risk scoring engine
│   │   ├── engine.go         # Main scoring logic
│   │   ├── factors.go        # Factor definitions and weights
│   │   └── exceptions.go     # Exception rule evaluation
│   │
│   ├── events/               # Redis Pub/Sub event bus
│   │   ├── publisher.go
│   │   └── subscriber.go
│   │
│   └── config/               # Config loading from env vars
│       └── config.go
│
├── frontend/
│   ├── src/
│   │   ├── main.tsx          # React entry point
│   │   ├── App.tsx           # Router setup
│   │   ├── pages/            # Route-level page components
│   │   ├── components/       # Reusable UI components
│   │   ├── hooks/            # TanStack Query hooks
│   │   ├── stores/           # Zustand stores
│   │   ├── lib/              # API client, SSE client, utilities
│   │   └── types/            # TypeScript type definitions
│   ├── package.json
│   └── vite.config.ts
│
├── plugin/                   # Headlamp plugin
│   ├── src/
│   │   ├── index.tsx         # Plugin registration
│   │   ├── UpdatesTab.tsx    # Per-workload update widget
│   │   └── api.ts            # KubePilot API client
│   └── package.json
│
├── migrations/
│   ├── 001_initial_schema.sql
│   ├── 002_add_exception_rules.sql
│   └── ...
│
├── helm/
│   └── kubepilot/            # Helm chart
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│
├── docker-compose.yml        # Local dev: PostgreSQL + Redis
├── docker-compose.test.yml   # Integration test dependencies
├── Makefile                  # Common dev tasks
└── .env.example              # Template for local .env file
```

---

## 3. Local Development Setup

### Step 1: Clone and configure environment

```bash
git clone https://github.com/your-org/kubepilot.git
cd kubepilot
cp .env.example .env
```

Edit `.env` and set any values appropriate for your local environment. The defaults in `.env.example` work out of the box with the docker-compose setup.

### Step 2: Start PostgreSQL and Redis

```bash
docker compose up -d
```

This starts:
- PostgreSQL 16 on `localhost:5432` (database: `kubepilot`, user: `kubepilot`, password: `kubepilot`)
- Redis 7 on `localhost:6379`

Verify both are running:

```bash
docker compose ps
```

### Step 3: Apply database migrations

```bash
make migrate
# or manually:
go run ./cmd/migrate up
```

### Step 4: Start the backend

```bash
make dev-server
# or manually:
go run ./cmd/server
```

The API is now available at `http://localhost:8080`.

### Step 5: Start the frontend

In a separate terminal:

```bash
cd frontend
npm install
npm run dev
```

The frontend dev server starts at `http://localhost:5173` and proxies `/api` requests to `http://localhost:8080`.

---

## 4. Running the Backend

### Standard startup

```bash
go run ./cmd/server
```

The server reads configuration from environment variables (see [section 9](#9-environment-variables-reference)). In development, these can be loaded from the `.env` file using:

```bash
# If using direnv (recommended):
direnv allow

# Or manually export them:
export $(cat .env | grep -v '^#' | xargs)
go run ./cmd/server
```

### Hot reload (optional)

Install `air` for live reload on file changes:

```bash
go install github.com/air-verse/air@latest
air
```

The `.air.toml` config file is included in the repository.

### Build the binary

```bash
go build -o bin/kubepilot ./cmd/server
./bin/kubepilot
```

### Startup log output

On a successful start you should see:

```
INFO  kubepilot starting version=0.1.0 port=8080
INFO  database connected dsn=postgres://kubepilot:***@localhost:5432/kubepilot
INFO  redis connected addr=localhost:6379
INFO  migrations: all up to date
INFO  worker pool starting workers=5
INFO  http server listening addr=:8080
```

---

## 5. Running the Frontend

```bash
cd frontend
npm install      # first time only
npm run dev      # start Vite dev server with HMR
```

The Vite dev server runs at `http://localhost:5173`. It proxies the following paths to the backend:

- `/api/*` → `http://localhost:8080/api`
- `/events` → `http://localhost:8080/events` (SSE pass-through)

The proxy is configured in `frontend/vite.config.ts`. If you change the backend port, update it there.

### Building for production

```bash
npm run build
```

Output goes to `frontend/dist/`. The Go backend serves this directory as static files in production mode.

### Frontend environment variables

Create `frontend/.env.local` for local overrides:

```
VITE_API_BASE_URL=http://localhost:8080
```

In production, `VITE_API_BASE_URL` is typically left empty (same-origin requests).

---

## 6. Database Migrations

### Running migrations

KubePilot uses a custom migration runner that applies sequential SQL files from the `migrations/` directory.

```bash
# Apply all pending migrations
go run ./cmd/migrate up

# Check current migration state
go run ./cmd/migrate status

# Roll back the last migration (if the migration has a down section)
go run ./cmd/migrate down
```

Via Make:

```bash
make migrate        # equivalent to: go run ./cmd/migrate up
make migrate-status
make migrate-down
```

### Writing a new migration

Migration files follow the naming convention `NNN_description.sql` where `NNN` is a zero-padded three-digit sequence number:

```
migrations/
├── 001_initial_schema.sql
├── 002_add_exception_rules.sql
├── 003_add_maintenance_windows.sql   ← new file
```

Each file contains SQL DDL statements. Migrations are applied in numeric order and tracked in a `schema_migrations` table.

**Rules:**
- Never modify an existing migration file. Always add a new one.
- Keep each migration focused on a single logical change.
- Include both schema changes and any required data migrations in the same file.
- Test the migration locally before committing: `go run ./cmd/migrate up`.

Example migration file:

```sql
-- 003_add_maintenance_windows.sql

CREATE TABLE maintenance_windows (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL,
    cluster_id       UUID REFERENCES clusters(id) ON DELETE CASCADE,
    namespace_id     UUID REFERENCES namespaces(id) ON DELETE CASCADE,
    cron_expression  TEXT NOT NULL,
    duration_minutes INTEGER NOT NULL CHECK (duration_minutes > 0),
    timezone         TEXT NOT NULL DEFAULT 'UTC',
    enabled          BOOLEAN NOT NULL DEFAULT true
);

CREATE INDEX maintenance_windows_cluster_id_idx ON maintenance_windows(cluster_id);
CREATE INDEX maintenance_windows_enabled_idx ON maintenance_windows(enabled) WHERE enabled = true;
```

---

## 7. Connecting a Local Kubernetes Cluster

### Using a local cluster (kind, minikube, k3d)

Start a local cluster with kind:

```bash
kind create cluster --name kubepilot-dev
```

Your kubeconfig will be updated with a new context. Set the path in your `.env`:

```
KUBECONFIG_PATH=/home/user/.kube/config
```

Then register the cluster via the API (or the UI once both services are running):

```bash
curl -X POST http://localhost:8080/api/v1/clusters \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "local-dev",
    "display_name": "Local Dev Cluster",
    "environment_id": "<environment-uuid>",
    "in_cluster": false
  }'
```

For in-cluster usage (when KubePilot itself runs inside the cluster), use `"in_cluster": true`. This makes the K8s Collector use the pod's service account token.

### Using a remote cluster

Store the kubeconfig in a Kubernetes Secret in the `kubepilot` namespace:

```bash
kubectl create secret generic kubeconfig-staging \
  --from-file=kubeconfig=/path/to/staging.kubeconfig \
  -n kubepilot
```

Register the cluster with the `kubeconfig_ref` pointing to this secret:

```bash
curl -X POST http://localhost:8080/api/v1/clusters \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "staging",
    "display_name": "Staging",
    "environment_id": "<env-uuid>",
    "kubeconfig_ref": "kubepilot/kubeconfig-staging"
  }'
```

The K8s Collector will attempt to connect immediately. Check the status with `GET /api/v1/clusters/<id>`.

### Default environment setup

On first run with an empty database, you need to create at least one Environment before registering clusters. Use the API:

```bash
curl -X POST http://localhost:8080/api/v1/environments \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Development",
    "slug": "dev",
    "criticality_weight": 0.3,
    "color": "#43a047"
  }'
```

Or, for convenience in development, seed data is available:

```bash
go run ./cmd/migrate seed
```

This creates default environments (prod, preprod, staging, dev) and a local admin user (`admin@kubepilot.local` / `changeme`).

---

## 8. Running Tests

### Backend tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test -v ./internal/scoring/...

# Run tests with race detector (recommended before committing)
go test -race ./...

# Run tests with coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Integration tests

Integration tests require PostgreSQL and Redis. They use `docker-compose.test.yml` which starts isolated containers on different ports to avoid conflicting with the dev environment:

```bash
docker compose -f docker-compose.test.yml up -d
go test -tags=integration ./...
docker compose -f docker-compose.test.yml down
```

Or via Make:

```bash
make test-integration
```

Integration tests are in files named `*_integration_test.go` and are gated behind the `integration` build tag.

### Frontend tests

```bash
cd frontend
npm run test           # Vitest in watch mode
npm run test:run       # Single run (CI mode)
npm run test:coverage  # With coverage report
```

Frontend tests use Vitest + React Testing Library. Test files are colocated with their components (`*.test.tsx`).

### Linting

```bash
# Backend
golangci-lint run ./...

# Frontend
npm run lint      # ESLint
npm run typecheck # TypeScript compiler check (no emit)
```

---

## 9. Environment Variables Reference

These variables are read by the backend on startup. In development, set them in your `.env` file. In production (Helm deployment), they are set in the Deployment spec via `valueFrom.secretKeyRef` or `valueFrom.configMapKeyRef`.

| Variable | Required | Default | Description |
|---|---|---|---|
| `DB_URL` | YES | — | PostgreSQL connection string. Format: `postgres://user:password@host:5432/dbname?sslmode=disable` |
| `REDIS_URL` | YES | — | Redis connection URL. Format: `redis://localhost:6379/0` |
| `JWT_SECRET_KEY_PATH` | YES | — | Path to the RSA private key file (PEM format) used for signing JWTs |
| `JWT_PUBLIC_KEY_PATH` | YES | — | Path to the RSA public key file (PEM format) used for verifying JWTs |
| `JWT_ACCESS_TTL` | NO | `3600` | Access token TTL in seconds |
| `JWT_REFRESH_TTL` | NO | `604800` | Refresh token TTL in seconds (7 days) |
| `PORT` | NO | `8080` | HTTP server listen port |
| `LOG_LEVEL` | NO | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | NO | `json` | Log format: `json` or `text` |
| `KUBECONFIG_PATH` | NO | `~/.kube/config` | Default kubeconfig path for non-in-cluster collectors |
| `WORKER_CONCURRENCY` | NO | `5` | Number of concurrent image-watcher workers |
| `IMAGE_CACHE_TTL` | NO | `300` | OCI registry tag cache TTL in seconds |
| `HELM_POLL_INTERVAL` | NO | `600` | Helm repository poll interval in seconds |
| `IMAGE_POLL_INTERVAL` | NO | `300` | Image tag poll interval in seconds |
| `FRONTEND_DIST_PATH` | NO | `./frontend/dist` | Path to the built frontend assets (served in production mode) |
| `CORS_ALLOWED_ORIGINS` | NO | `http://localhost:5173` | Comma-separated list of allowed CORS origins |
| `RATE_LIMIT_RPS` | NO | `100` | API rate limit per IP (requests per second) |
| `ENV` | NO | `development` | Environment name: `development`, `production`. Affects log format defaults and CORS strictness. |

**Local development `.env` example:**

```dotenv
DB_URL=postgres://kubepilot:kubepilot@localhost:5432/kubepilot?sslmode=disable
REDIS_URL=redis://localhost:6379/0
JWT_SECRET_KEY_PATH=./dev-keys/private.pem
JWT_PUBLIC_KEY_PATH=./dev-keys/public.pem
PORT=8080
LOG_LEVEL=debug
LOG_FORMAT=text
ENV=development
CORS_ALLOWED_ORIGINS=http://localhost:5173
```

Generate development JWT keys:

```bash
mkdir -p dev-keys
openssl genrsa -out dev-keys/private.pem 2048
openssl rsa -in dev-keys/private.pem -pubout -out dev-keys/public.pem
```

---

## 10. Code Structure

### `cmd/server/main.go`

The entrypoint. Loads config, initializes the database connection pool, creates the Redis client, instantiates the worker pool, wires up the HTTP router, and starts the server. Uses dependency injection by passing initialized components into handler constructors.

### `internal/config/`

Reads environment variables into a `Config` struct using `os.Getenv`. No third-party config library. Validation happens at startup and will call `log.Fatal` on missing required variables.

### `internal/db/`

Database access layer. Uses the `pgx/v5` driver directly — no ORM. Query functions in `internal/db/queries/` accept a `pgx.Pool` or `pgx.Tx` and return typed Go structs. All queries are written in plain SQL; no query builder. Complex list queries with optional filters are built by appending WHERE clauses using `pgx`'s named argument support.

### `internal/api/`

HTTP layer. Uses the `chi` router. Each file contains a group of related handlers. Handlers follow the pattern:
1. Extract and validate path parameters and query parameters
2. Call a DB query function
3. Serialize the result and write the response

Business logic (scoring, event publishing) stays in dedicated packages, not in handlers.

### `internal/workers/`

All background workers. The `pool.go` file manages worker goroutines and handles graceful shutdown. Each worker has a `Start(ctx context.Context)` method that blocks until the context is cancelled. Workers use a shared work queue (Go channel or Redis-backed queue for durability).

### `internal/scoring/`

The scoring engine. `engine.go` exposes a `Compute(ctx, finding FindingContext) (RiskScore, error)` function. It is pure (no side effects) and accepts all required context via the `FindingContext` struct, making it easily testable. `engine.go` also handles exception rule evaluation. `factors.go` defines the `Factor` type and all factor weights as package-level constants.

### `internal/events/`

Thin wrapper around `go-redis/v9` Pub/Sub. `publisher.go` exposes `Publish(ctx, event Event) error`. `subscriber.go` exposes a `Subscribe(ctx, channel) (<-chan Event, error)` channel-based API. The SSE handler in `internal/api/sse.go` uses the subscriber.

### `frontend/src/lib/api.ts`

Typed API client for the backend. Uses `fetch` with automatic JWT header injection from the Zustand auth store. Each API method returns a typed Promise and throws an `ApiError` with the error code on non-2xx responses.

### `frontend/src/lib/sse.ts`

SSE client wrapper. Creates an `EventSource`, maps each event type to a callback, handles reconnection backoff, and exposes a cleanup function. Used by `App.tsx` to set up the global event listener that calls `queryClient.invalidateQueries()`.

### `frontend/src/hooks/`

TanStack Query hooks for each API resource. For example, `useFindings(filters)` wraps `GET /api/v1/findings`, `useFinding(id)` wraps `GET /api/v1/findings/:id`. Mutation hooks (e.g., `useUpdateFindingStatus`) use `useMutation` and call `queryClient.invalidateQueries` on success.

---

## 11. Common Development Tasks

### Add a new API endpoint

1. Write the SQL query in `internal/db/queries/<entity>.go`.
2. Add the Go handler function in `internal/api/<entity>.go`.
3. Register the route in `internal/api/router.go`.
4. Add the TypeScript API method in `frontend/src/lib/api.ts`.
5. Add the TanStack Query hook in `frontend/src/hooks/use<Entity>.ts`.
6. Update `docs/api-spec.md`.

### Add a new database table

1. Write a new migration file in `migrations/NNN_description.sql`.
2. Apply it locally: `go run ./cmd/migrate up`.
3. Add the Go struct in `internal/db/models/<entity>.go`.
4. Add query functions in `internal/db/queries/<entity>.go`.
5. Update `docs/data-model.md`.

### Add a new scoring factor

1. Add the factor constant in `internal/scoring/factors.go` with its weight (all weights must sum to 1.0 across all factors in the formula).
2. Implement the factor value calculation in `internal/scoring/engine.go` inside the `computeFactors` function.
3. Write a unit test in `internal/scoring/engine_test.go` with the new factor included.
4. Update `docs/scoring.md` with the new factor row.

### Add a new worker

1. Create a new package under `internal/workers/<workername>/`.
2. Implement a struct with a `Start(ctx context.Context) error` method.
3. Register it in the `pool.go` worker pool startup.
4. Wire any dependencies (DB, Redis, config) in `cmd/server/main.go`.

### Regenerate JWT keys for development

```bash
openssl genrsa -out dev-keys/private.pem 2048
openssl rsa -in dev-keys/private.pem -pubout -out dev-keys/public.pem
```

Note: Never commit the `dev-keys/` directory. It is listed in `.gitignore`.

### Reset the local database

```bash
docker compose down -v   # removes the postgres volume
docker compose up -d
go run ./cmd/migrate up
go run ./cmd/migrate seed   # optional: re-add seed data
```

### Inspect the database directly

```bash
docker compose exec postgres psql -U kubepilot kubepilot
```

Or use a GUI tool pointed at `localhost:5432`, database `kubepilot`, user `kubepilot`, password `kubepilot`.

### Run a specific Go test by name

```bash
go test -run TestScoringEngine_ComputeFactors ./internal/scoring/...
```

### Check for dependency vulnerabilities

```bash
go list -m all | govulncheck ./...
cd frontend && npm audit
```

### Update Go dependencies

```bash
go get -u ./...
go mod tidy
```

### Build and load the Headlamp plugin for local testing

```bash
cd plugin
npm install
npm run build
# Copy the built plugin to Headlamp's plugin directory:
cp dist/main.js ~/.config/Headlamp/plugins/kubepilot/main.js
# Restart Headlamp
```

### Make targets reference

```
make dev-server       Start backend with air (live reload)
make dev-frontend     Start frontend dev server (cd frontend && npm run dev)
make migrate          Apply pending DB migrations
make migrate-status   Show migration status
make test             go test ./...
make test-race        go test -race ./...
make test-integration Run integration tests with docker-compose.test.yml
make lint             Run golangci-lint and frontend ESLint
make build            Build the production binary
make docker-build     Build the Docker image
make seed             Run database seed (dev environments + admin user)
```
