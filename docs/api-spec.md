# KubePilot — REST API Specification

## Table of Contents

1. [Overview](#1-overview)
2. [Authentication](#2-authentication)
3. [Common Conventions](#3-common-conventions)
4. [Error Codes](#4-error-codes)
5. [Clusters](#5-clusters)
6. [Workloads](#6-workloads)
7. [Findings (Updates)](#7-findings-updates)
8. [Nodes](#8-nodes)
9. [Helm Releases](#9-helm-releases)
10. [Scores](#10-scores)
11. [Overview](#11-overview)
12. [Integrations](#12-integrations)
13. [Health](#13-health)
14. [Auth](#14-auth)
15. [SSE Event Stream](#15-sse-event-stream)

---

## 1. Overview

**Base URL:** `/api/v1`

All requests and responses use `Content-Type: application/json` unless otherwise noted. Dates and timestamps are ISO 8601 strings in UTC (e.g., `"2024-03-15T10:30:00Z"`). UUIDs are lowercase hyphenated strings.

The API is versioned in the path. Breaking changes will introduce `/api/v2`.

---

## 2. Authentication

All endpoints except `POST /auth/login`, `POST /auth/refresh`, `GET /health`, and `GET /health/ready` require a valid JWT bearer token.

Include the token in the `Authorization` header:

```
Authorization: Bearer <token>
```

Tokens are RS256-signed JWTs with the following standard claims plus KubePilot-specific claims:

```json
{
  "sub": "user-uuid",
  "email": "user@example.com",
  "name": "Alice",
  "roles": [
    { "cluster_id": "uuid-or-null", "role": "editor" }
  ],
  "iat": 1710000000,
  "exp": 1710003600
}
```

Access tokens expire after 1 hour. Use `POST /auth/refresh` to obtain a new access token using the refresh token (7-day TTL, stored as an HttpOnly cookie).

---

## 3. Common Conventions

### Pagination

List endpoints that may return large result sets support cursor-based pagination via query parameters:

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | integer | 50 | Maximum items to return (max: 200) |
| `cursor` | string | — | Opaque cursor from the previous response's `next_cursor` field |

List responses include:

```json
{
  "items": [...],
  "total": 1234,
  "next_cursor": "eyJ..." 
}
```

`next_cursor` is omitted when there are no more results.

### Filtering

Filter parameters are applied with AND semantics unless noted. Array values (e.g., `severity=critical,high`) are comma-separated.

### Sorting

Endpoints that support sorting accept `sort` (field name) and `order` (`asc` or `desc`) query parameters. Default sort is documented per endpoint.

---

## 4. Error Codes

All errors return a JSON body:

```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "cluster with id 'abc' not found",
    "details": {}
  }
}
```

| HTTP Status | Code | Meaning |
|---|---|---|
| 400 | `BAD_REQUEST` | Invalid request body or query parameter |
| 401 | `UNAUTHORIZED` | Missing or invalid JWT |
| 403 | `FORBIDDEN` | Authenticated but insufficient role for this action |
| 404 | `NOT_FOUND` | Entity does not exist |
| 409 | `CONFLICT` | Unique constraint violation (e.g., duplicate cluster name) |
| 422 | `VALIDATION_ERROR` | Request was well-formed but semantically invalid |
| 429 | `RATE_LIMITED` | Too many requests |
| 500 | `INTERNAL_ERROR` | Unexpected server error |
| 503 | `SERVICE_UNAVAILABLE` | Dependency (DB, Redis) is unavailable |

---

## 5. Clusters

### GET /clusters

List all registered clusters with their current status.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `environment_id` | uuid | Filter by environment |
| `status` | string | Filter by status: `connected`, `disconnected`, `error`, `pending` |
| `limit` | integer | Pagination |
| `cursor` | string | Pagination |

**Response 200:**

```json
{
  "items": [
    {
      "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "name": "prod-eu-west-1",
      "display_name": "Production EU West",
      "endpoint": "https://k8s-api.prod.example.com",
      "version": "v1.29.3",
      "environment": {
        "id": "uuid",
        "name": "Production",
        "slug": "production",
        "color": "#e53935"
      },
      "status": "connected",
      "last_seen_at": "2024-03-15T10:30:00Z",
      "summary": {
        "workload_count": 142,
        "finding_count": 23,
        "critical_count": 2,
        "high_count": 8
      }
    }
  ],
  "total": 5,
  "next_cursor": null
}
```

**Error Codes:** 401, 500

---

### GET /clusters/:id

Get full details for a single cluster.

**Path Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `id` | uuid | Cluster ID |

**Response 200:**

```json
{
  "id": "uuid",
  "name": "prod-eu-west-1",
  "display_name": "Production EU West",
  "endpoint": "https://k8s-api.prod.example.com",
  "version": "v1.29.3",
  "environment": { "id": "uuid", "name": "Production", "slug": "production", "color": "#e53935" },
  "status": "connected",
  "last_seen_at": "2024-03-15T10:30:00Z",
  "annotations": { "team": "platform", "region": "eu-west-1" },
  "summary": {
    "namespace_count": 18,
    "node_count": 12,
    "workload_count": 142,
    "finding_count": 23,
    "critical_count": 2,
    "high_count": 8,
    "medium_count": 9,
    "low_count": 4
  }
}
```

**Error Codes:** 401, 403, 404, 500

---

### POST /clusters

Register a new cluster. Accepts either a kubeconfig reference or an in-cluster flag.

**Request Body:**

```json
{
  "name": "staging-eu",
  "display_name": "Staging EU",
  "environment_id": "uuid",
  "kubeconfig_ref": "kubepilot/secret-staging-kubeconfig",
  "annotations": { "team": "platform" }
}
```

For in-cluster registration (no kubeconfig needed):

```json
{
  "name": "primary",
  "display_name": "Primary In-Cluster",
  "environment_id": "uuid",
  "in_cluster": true
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | YES | Unique cluster name |
| `display_name` | string | NO | UI-friendly name |
| `environment_id` | uuid | YES | Environment to assign to |
| `kubeconfig_ref` | string | NO | K8s Secret reference (`namespace/secret-name`) |
| `in_cluster` | boolean | NO | Use in-cluster service account credentials |
| `annotations` | object | NO | Arbitrary key-value metadata |

**Response 201:** Full cluster object (same schema as GET /clusters/:id)

**Error Codes:** 400, 401, 403, 409 (name already exists), 422, 500

---

### PUT /clusters/:id

Update cluster configuration. Cannot change the cluster name.

**Request Body:** Same fields as POST, all optional.

**Response 200:** Updated cluster object.

**Error Codes:** 400, 401, 403, 404, 422, 500

---

### DELETE /clusters/:id

Remove a cluster and all its associated data (namespaces, nodes, workloads, findings) via cascade.

**Response 204:** No content.

**Error Codes:** 401, 403, 404, 500

---

### POST /clusters/:id/sync

Force an immediate full re-sync of the cluster (list all resources, reconcile with DB). This triggers the K8s Collector to re-list all resources rather than relying on the Watch stream. Useful after a collector reconnection or suspected missed events.

**Response 202:**

```json
{
  "message": "sync initiated",
  "cluster_id": "uuid",
  "initiated_at": "2024-03-15T10:30:00Z"
}
```

**Error Codes:** 401, 403, 404, 409 (sync already in progress), 500

---

## 6. Workloads

### GET /workloads

List workloads with optional filtering. Returns a summary view suitable for list rendering.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `cluster_id` | uuid | Filter by cluster |
| `namespace` | string | Filter by namespace name |
| `kind` | string | Filter by kind: `Deployment`, `StatefulSet`, `DaemonSet`, `CronJob`, `Pod` |
| `has_updates` | boolean | If `true`, only return workloads with at least one open finding |
| `severity` | string | Filter by max finding severity: `critical`, `high`, `medium`, `low` |
| `search` | string | Substring match on workload name |
| `sort` | string | Sort field: `name`, `score`, `last_observed_at` (default: `score`) |
| `order` | string | `asc` or `desc` (default: `desc`) |
| `limit` | integer | Pagination |
| `cursor` | string | Pagination |

**Response 200:**

```json
{
  "items": [
    {
      "id": "uuid",
      "cluster_id": "uuid",
      "cluster_name": "prod-eu-west-1",
      "namespace": "payments",
      "name": "payment-api",
      "kind": "Deployment",
      "replicas_desired": 3,
      "replicas_ready": 3,
      "last_observed_at": "2024-03-15T10:30:00Z",
      "source_type": "helm",
      "max_severity": "high",
      "open_finding_count": 2,
      "score": 72.4
    }
  ],
  "total": 142,
  "next_cursor": "eyJ..."
}
```

**Error Codes:** 401, 500

---

### GET /workloads/:id

Full workload detail including container images and associated findings.

**Response 200:**

```json
{
  "id": "uuid",
  "cluster_id": "uuid",
  "cluster_name": "prod-eu-west-1",
  "namespace": "payments",
  "name": "payment-api",
  "kind": "Deployment",
  "uid": "k8s-uid-string",
  "replicas_desired": 3,
  "replicas_ready": 3,
  "labels": { "app": "payment-api", "team": "payments" },
  "annotations": {},
  "last_observed_at": "2024-03-15T10:30:00Z",
  "source": {
    "source_type": "helm",
    "helm_release_id": "uuid",
    "helm_release_name": "payment-api"
  },
  "containers": [
    {
      "id": "uuid",
      "container_name": "api",
      "image_ref": "ghcr.io/example/payment-api:1.4.2",
      "registry": "ghcr.io",
      "repository": "example/payment-api",
      "tag": "1.4.2",
      "digest": "sha256:abc...",
      "is_init_container": false
    }
  ],
  "ownership": {
    "owner_name": "Payments Team",
    "owner_email": "payments@example.com",
    "team": "payments"
  },
  "findings": [
    {
      "id": "uuid",
      "kind": "image",
      "current_version": "1.4.2",
      "available_version": "1.5.0",
      "update_type": "minor",
      "status": "open",
      "score": 72.4,
      "severity": "high",
      "first_detected_at": "2024-03-10T08:00:00Z"
    }
  ]
}
```

**Error Codes:** 401, 403, 404, 500

---

### GET /clusters/:cluster_id/namespaces/:namespace/workloads

Convenience scoped list of workloads within a specific cluster namespace. Accepts the same filter and pagination parameters as `GET /workloads` except `cluster_id` and `namespace` (which are taken from the path).

**Response 200:** Same schema as `GET /workloads`.

**Error Codes:** 401, 403, 404 (cluster or namespace not found), 500

---

## 7. Findings (Updates)

### GET /findings

List update findings with scoring information. This is the primary operational view.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `severity` | string | Comma-separated: `critical,high,medium,low,info` |
| `status` | string | Comma-separated: `open,planned,approved,ignored,blocked` |
| `kind` | string | Comma-separated: `image,helm,node,k8s-api` |
| `cluster_id` | uuid | Filter by cluster |
| `namespace` | string | Filter by namespace name |
| `update_type` | string | `patch`, `minor`, `major`, `unknown` |
| `is_breaking` | boolean | Filter breaking changes only |
| `search` | string | Substring match on target name or versions |
| `sort` | string | `score`, `first_detected_at`, `last_confirmed_at` (default: `score`) |
| `order` | string | `asc` or `desc` (default: `desc`) |
| `limit` | integer | Pagination |
| `cursor` | string | Pagination |

**Response 200:**

```json
{
  "items": [
    {
      "id": "uuid",
      "kind": "image",
      "target_id": "uuid",
      "target_kind": "Workload",
      "target_name": "payment-api",
      "target_namespace": "payments",
      "cluster_id": "uuid",
      "cluster_name": "prod-eu-west-1",
      "current_version": "1.4.2",
      "available_version": "1.5.0",
      "update_type": "minor",
      "is_breaking": false,
      "changelog_url": "https://github.com/example/payment-api/releases/tag/v1.5.0",
      "status": "open",
      "status_changed_at": null,
      "status_changed_by": null,
      "first_detected_at": "2024-03-10T08:00:00Z",
      "last_confirmed_at": "2024-03-15T10:00:00Z",
      "score": {
        "value": 72.4,
        "severity": "high",
        "computed_at": "2024-03-15T10:05:00Z"
      }
    }
  ],
  "total": 23,
  "next_cursor": null
}
```

**Error Codes:** 401, 500

---

### GET /findings/:id

Full finding detail including score breakdown.

**Response 200:**

```json
{
  "id": "uuid",
  "kind": "image",
  "target_id": "uuid",
  "target_kind": "Workload",
  "target_name": "payment-api",
  "target_namespace": "payments",
  "cluster_id": "uuid",
  "cluster_name": "prod-eu-west-1",
  "current_version": "1.4.2",
  "available_version": "1.5.0",
  "update_type": "minor",
  "is_breaking": false,
  "changelog_url": "https://github.com/example/...",
  "status": "open",
  "status_changed_by": null,
  "status_changed_at": null,
  "first_detected_at": "2024-03-10T08:00:00Z",
  "last_confirmed_at": "2024-03-15T10:00:00Z",
  "score": {
    "value": 72.4,
    "severity": "high",
    "computed_at": "2024-03-15T10:05:00Z",
    "factors": {
      "update_type":          { "raw": 15, "weight": 0.25, "contribution": 3.75 },
      "age_days":             { "raw": 8,  "weight": 0.20, "contribution": 1.6  },
      "cvss_max":             { "raw": 0,  "weight": 0.20, "contribution": 0    },
      "service_criticality":  { "raw": 10, "weight": 0.15, "contribution": 1.5  },
      "current_health":       { "raw": 0,  "weight": 0.10, "contribution": 0    },
      "rollback_possible":    { "raw": 0,  "weight": 0.05, "contribution": 0    },
      "maintenance_window":   { "raw": 5,  "weight": 0.05, "contribution": 0.25 },
      "env_multiplier":       1.0,
      "exposure_multiplier":  1.2,
      "raw_sum":              7.1,
      "normalized_score":     72.4
    }
  },
  "exception_rule": null,
  "action_history": [
    {
      "user_name": "Alice",
      "action_type": "FINDING_STATUS_CHANGE",
      "payload": { "old_status": "open", "new_status": "planned" },
      "created_at": "2024-03-12T09:00:00Z"
    }
  ]
}
```

**Error Codes:** 401, 403, 404, 500

---

### PATCH /findings/:id/status

Update the operator workflow status of a finding. Requires `editor` or `admin` role on the finding's cluster.

**Request Body:**

```json
{
  "status": "planned",
  "note": "Scheduled for next maintenance window on Saturday"
}
```

| Field | Type | Required | Values |
|---|---|---|---|
| `status` | string | YES | `open`, `planned`, `approved`, `ignored`, `blocked` |
| `note` | string | NO | Optional comment recorded in the action log |

**Response 200:**

```json
{
  "id": "uuid",
  "status": "planned",
  "status_changed_by": "uuid",
  "status_changed_at": "2024-03-15T11:00:00Z"
}
```

**Error Codes:** 400, 401, 403, 404, 422, 500

---

### GET /findings/summary

Aggregated counts of findings by severity and status, grouped by cluster. Used for the Overview page charts.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `cluster_id` | uuid | Scope to a single cluster |
| `environment_id` | uuid | Scope to an environment |

**Response 200:**

```json
{
  "totals": {
    "critical": 2,
    "high": 8,
    "medium": 9,
    "low": 4,
    "info": 0
  },
  "by_status": {
    "open":     { "critical": 2, "high": 6, "medium": 7, "low": 3 },
    "planned":  { "critical": 0, "high": 2, "medium": 2, "low": 1 },
    "approved": { "critical": 0, "high": 0, "medium": 0, "low": 0 },
    "ignored":  { "critical": 0, "high": 0, "medium": 0, "low": 0 },
    "blocked":  { "critical": 0, "high": 0, "medium": 0, "low": 0 }
  },
  "by_cluster": [
    {
      "cluster_id": "uuid",
      "cluster_name": "prod-eu-west-1",
      "critical": 2,
      "high": 5,
      "medium": 4,
      "low": 2
    }
  ]
}
```

**Error Codes:** 401, 500

---

## 8. Nodes

### GET /nodes

List nodes across all visible clusters.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `cluster_id` | uuid | Filter by cluster |
| `role` | string | `control-plane`, `worker`, `etcd` |
| `limit` | integer | Pagination |
| `cursor` | string | Pagination |

**Response 200:**

```json
{
  "items": [
    {
      "id": "uuid",
      "cluster_id": "uuid",
      "cluster_name": "prod-eu-west-1",
      "name": "node-001",
      "role": "worker",
      "os_image": "Ubuntu 22.04.3 LTS",
      "kernel_version": "5.15.0-94-generic",
      "kubelet_version": "v1.29.3",
      "container_runtime": "containerd://1.7.11",
      "ready": true,
      "finding_count": 1
    }
  ],
  "total": 12,
  "next_cursor": null
}
```

**Error Codes:** 401, 500

---

### GET /nodes/:id

Full node detail including conditions, capacity, taints, and associated findings.

**Response 200:**

```json
{
  "id": "uuid",
  "cluster_id": "uuid",
  "cluster_name": "prod-eu-west-1",
  "name": "node-001",
  "role": "worker",
  "os_image": "Ubuntu 22.04.3 LTS",
  "kernel_version": "5.15.0-94-generic",
  "kubelet_version": "v1.29.3",
  "container_runtime": "containerd://1.7.11",
  "capacity": { "cpu": "8", "memory": "32Gi" },
  "allocatable": { "cpu": "7800m", "memory": "30Gi" },
  "conditions": [
    { "type": "Ready", "status": "True", "message": "" },
    { "type": "MemoryPressure", "status": "False", "message": "" }
  ],
  "taints": [],
  "labels": { "topology.kubernetes.io/zone": "eu-west-1a" },
  "last_seen_at": "2024-03-15T10:30:00Z",
  "findings": []
}
```

**Error Codes:** 401, 403, 404, 500

---

## 9. Helm Releases

### GET /helm/releases

List Helm releases across all visible clusters.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `cluster_id` | uuid | Filter by cluster |
| `namespace` | string | Filter by namespace name |
| `has_updates` | boolean | Only releases with open chart-version findings |
| `status` | string | Helm release status filter |
| `search` | string | Substring match on release name or chart name |
| `limit` | integer | Pagination |
| `cursor` | string | Pagination |

**Response 200:**

```json
{
  "items": [
    {
      "id": "uuid",
      "cluster_id": "uuid",
      "cluster_name": "prod-eu-west-1",
      "namespace": "ingress-nginx",
      "release_name": "ingress-nginx",
      "chart_name": "ingress-nginx",
      "chart_version": "4.9.1",
      "app_version": "1.10.0",
      "status": "deployed",
      "last_deployed_at": "2024-02-01T12:00:00Z",
      "open_finding_count": 1,
      "max_severity": "medium"
    }
  ],
  "total": 34,
  "next_cursor": null
}
```

**Error Codes:** 401, 500

---

### GET /helm/releases/:id

Full Helm release detail.

**Response 200:**

```json
{
  "id": "uuid",
  "cluster_id": "uuid",
  "cluster_name": "prod-eu-west-1",
  "namespace": "ingress-nginx",
  "release_name": "ingress-nginx",
  "chart_name": "ingress-nginx",
  "chart_version": "4.9.1",
  "app_version": "1.10.0",
  "repo_url": "https://kubernetes.github.io/ingress-nginx",
  "status": "deployed",
  "last_deployed_at": "2024-02-01T12:00:00Z",
  "ownership": {
    "owner_name": "Platform Team",
    "team": "platform"
  },
  "findings": [
    {
      "id": "uuid",
      "kind": "helm",
      "current_version": "4.9.1",
      "available_version": "4.10.0",
      "update_type": "minor",
      "status": "open",
      "score": 48.2,
      "severity": "medium"
    }
  ]
}
```

**Error Codes:** 401, 403, 404, 500

---

## 10. Scores

### GET /scores

List RiskScore records joined with their findings, sorted by score descending. Useful for a "top risks" view.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `severity` | string | Comma-separated severity filter |
| `cluster_id` | uuid | Scope by cluster |
| `status` | string | Finding status filter (default: `open`) |
| `limit` | integer | Pagination |
| `cursor` | string | Pagination |

**Response 200:**

```json
{
  "items": [
    {
      "finding_id": "uuid",
      "target_name": "payment-api",
      "target_kind": "Workload",
      "target_namespace": "payments",
      "cluster_name": "prod-eu-west-1",
      "kind": "image",
      "current_version": "1.4.2",
      "available_version": "1.5.0",
      "update_type": "minor",
      "status": "open",
      "score": 72.4,
      "severity": "high",
      "computed_at": "2024-03-15T10:05:00Z"
    }
  ],
  "total": 23,
  "next_cursor": null
}
```

**Error Codes:** 401, 500

---

### GET /workloads/:id/score

Score summary for a specific workload, aggregating all its open findings.

**Response 200:**

```json
{
  "workload_id": "uuid",
  "workload_name": "payment-api",
  "max_score": 72.4,
  "max_severity": "high",
  "finding_scores": [
    {
      "finding_id": "uuid",
      "kind": "image",
      "current_version": "1.4.2",
      "available_version": "1.5.0",
      "score": 72.4,
      "severity": "high"
    }
  ]
}
```

**Error Codes:** 401, 403, 404, 500

---

## 11. Overview

### GET /overview

Returns aggregated statistics for the dashboard overview page. Designed as a single efficient query for the initial page load.

**Query Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `environment_id` | uuid | Scope to environment |

**Response 200:**

```json
{
  "generated_at": "2024-03-15T10:30:00Z",
  "clusters": {
    "total": 5,
    "connected": 4,
    "disconnected": 0,
    "error": 1
  },
  "workloads": {
    "total": 320,
    "with_updates": 48
  },
  "helm_releases": {
    "total": 72,
    "with_updates": 11
  },
  "nodes": {
    "total": 45,
    "ready": 44
  },
  "findings": {
    "total": 59,
    "open": 42,
    "by_severity": {
      "critical": 3,
      "high": 12,
      "medium": 18,
      "low": 9,
      "info": 0
    },
    "by_update_type": {
      "major": 4,
      "minor": 22,
      "patch": 33,
      "unknown": 0
    }
  },
  "top_findings": [
    {
      "finding_id": "uuid",
      "target_name": "payment-api",
      "cluster_name": "prod-eu-west-1",
      "score": 88.1,
      "severity": "critical"
    }
  ]
}
```

**Error Codes:** 401, 500

---

## 12. Integrations

### GET /integrations

List all configured integration accounts.

**Response 200:**

```json
{
  "items": [
    {
      "id": "uuid",
      "name": "prod-slack",
      "integration_type": "slack",
      "config": { "channel": "#platform-alerts" },
      "enabled": true,
      "last_tested_at": "2024-03-14T09:00:00Z",
      "last_test_status": "ok"
    }
  ],
  "total": 3
}
```

**Note:** `credentials_ref` is never returned in responses.

**Error Codes:** 401, 403 (admin only), 500

---

### POST /integrations

Add a new integration account. Requires `admin` role.

**Request Body:**

```json
{
  "name": "prod-slack",
  "integration_type": "slack",
  "config": {
    "channel": "#platform-alerts",
    "mention_on_critical": true
  },
  "credentials_ref": "kubepilot/secret-slack-token",
  "enabled": true
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | YES | Unique name |
| `integration_type` | string | YES | One of the valid integration type values |
| `config` | object | YES | Non-sensitive integration configuration |
| `credentials_ref` | string | NO | K8s Secret reference |
| `enabled` | boolean | NO | Default: true |

**Response 201:** Full integration object.

**Error Codes:** 400, 401, 403, 409 (name exists), 422, 500

---

### PUT /integrations/:id

Update an existing integration account. Requires `admin` role.

**Request Body:** Same as POST, all fields optional.

**Response 200:** Updated integration object.

**Error Codes:** 400, 401, 403, 404, 422, 500

---

### DELETE /integrations/:id

Remove an integration account. Requires `admin` role.

**Response 204:** No content.

**Error Codes:** 401, 403, 404, 500

---

### POST /integrations/:id/test

Test the connection for an integration account. For Slack, sends a test message. For registries, performs an authenticated ping. Updates `last_tested_at` and `last_test_status`.

**Response 200:**

```json
{
  "status": "ok",
  "message": "Connection successful",
  "tested_at": "2024-03-15T11:00:00Z"
}
```

Or on failure:

```json
{
  "status": "error",
  "message": "Authentication failed: 401 Unauthorized",
  "tested_at": "2024-03-15T11:00:00Z"
}
```

**Error Codes:** 401, 403, 404, 500

---

## 13. Health

### GET /health

Liveness probe. Returns 200 if the process is alive.

**Response 200:**

```json
{
  "status": "ok",
  "version": "1.2.0",
  "commit": "abc1234"
}
```

**Auth required:** No

---

### GET /health/ready

Readiness probe. Returns 200 only if all critical dependencies (PostgreSQL, Redis) are reachable.

**Response 200:**

```json
{
  "status": "ready",
  "checks": {
    "postgres": "ok",
    "redis": "ok"
  }
}
```

**Response 503:**

```json
{
  "status": "not_ready",
  "checks": {
    "postgres": "ok",
    "redis": "error: connection refused"
  }
}
```

**Auth required:** No

---

### GET /health/connectors

Returns the current status of all cluster collectors and external service connectors.

**Auth required:** Yes (any authenticated user)

**Response 200:**

```json
{
  "collectors": [
    {
      "cluster_id": "uuid",
      "cluster_name": "prod-eu-west-1",
      "status": "connected",
      "last_event_at": "2024-03-15T10:30:00Z",
      "error": null
    }
  ],
  "watchers": {
    "image_watcher": { "status": "running", "queue_depth": 12 },
    "helm_watcher":  { "status": "running", "queue_depth": 0 },
    "job_scheduler": { "status": "running", "next_run_at": "2024-03-15T11:00:00Z" }
  }
}
```

**Error Codes:** 401, 500

---

## 14. Auth

### POST /auth/login

Authenticate with local credentials and receive a JWT access token plus a refresh token cookie.

**Request Body:**

```json
{
  "email": "alice@example.com",
  "password": "securepassword"
}
```

**Response 200:**

```json
{
  "access_token": "eyJ...",
  "expires_in": 3600,
  "token_type": "Bearer",
  "user": {
    "id": "uuid",
    "email": "alice@example.com",
    "name": "Alice",
    "provider": "local"
  }
}
```

The refresh token is set as an `HttpOnly; Secure; SameSite=Strict` cookie named `kubepilot_refresh`.

**Error Codes:** 400, 401 (invalid credentials), 500

---

### POST /auth/refresh

Exchange the refresh token cookie for a new access token. The refresh token must be present as the `kubepilot_refresh` cookie.

**Request Body:** None required (refresh token read from cookie).

**Response 200:**

```json
{
  "access_token": "eyJ...",
  "expires_in": 3600,
  "token_type": "Bearer"
}
```

**Error Codes:** 401 (missing or expired refresh token), 500

---

### GET /auth/me

Return the current authenticated user's profile and their cluster role assignments.

**Response 200:**

```json
{
  "id": "uuid",
  "email": "alice@example.com",
  "name": "Alice",
  "provider": "local",
  "created_at": "2024-01-10T09:00:00Z",
  "last_login_at": "2024-03-15T10:00:00Z",
  "roles": [
    { "cluster_id": null, "cluster_name": null, "role": "admin" }
  ]
}
```

**Error Codes:** 401, 500

---

## 15. SSE Event Stream

### GET /events

Opens a Server-Sent Events stream. The connection remains open and the server pushes typed events as they occur. Clients should use the browser `EventSource` API or an equivalent SSE client.

**Auth required:** Yes — pass the JWT as a query parameter since `EventSource` does not support custom headers:

```
GET /api/v1/events?token=<access_token>
```

**Response:** `Content-Type: text/event-stream` with `Cache-Control: no-cache` and `Connection: keep-alive`.

**Event format:**

```
event: <event-type>
data: <json-payload>
id: <event-id>

```

**Keepalive:** The server sends a comment line every 30 seconds:

```
: keepalive

```

**Event Types:**

| Event Type | Payload | Description |
|---|---|---|
| `workload.updated` | `{ "cluster_id": "uuid", "workload_id": "uuid", "kind": "Deployment", "name": "...", "namespace": "..." }` | A workload was created, modified, or deleted |
| `finding.created` | `{ "finding_id": "uuid", "severity": "high", "target_kind": "Workload", "target_id": "uuid" }` | A new update finding was detected |
| `finding.updated` | `{ "finding_id": "uuid", "severity": "medium", "status": "open" }` | An existing finding was re-scored or updated |
| `finding.status_changed` | `{ "finding_id": "uuid", "old_status": "open", "new_status": "planned", "changed_by": "uuid" }` | An operator changed the finding status |
| `score.updated` | `{ "finding_id": "uuid", "score": 72.4, "severity": "high" }` | The risk score for a finding was recomputed |
| `cluster.status_changed` | `{ "cluster_id": "uuid", "old_status": "connected", "new_status": "error" }` | A cluster collector changed connectivity state |
| `sync.completed` | `{ "cluster_id": "uuid", "duration_ms": 1240 }` | A full cluster sync finished |

**Example SSE session:**

```
event: finding.created
data: {"finding_id":"3fa85f64-5717-4562-b3fc-2c963f66afa6","severity":"high","target_kind":"Workload","target_id":"uuid"}
id: 1710000001

event: score.updated
data: {"finding_id":"3fa85f64-5717-4562-b3fc-2c963f66afa6","score":72.4,"severity":"high"}
id: 1710000002

: keepalive

event: finding.status_changed
data: {"finding_id":"3fa85f64-5717-4562-b3fc-2c963f66afa6","old_status":"open","new_status":"planned","changed_by":"user-uuid"}
id: 1710000003
```

**Reconnection:** `EventSource` automatically reconnects. The server respects the `Last-Event-ID` header on reconnection and replays events from Redis Streams with IDs greater than the provided value (up to 500 events buffered in Redis, TTL 10 minutes).

**Error Codes:** 401 (invalid token query param), 500
