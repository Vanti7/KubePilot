# KubePilot — Data Model

## Table of Contents

1. [Overview](#1-overview)
2. [Entity Reference](#2-entity-reference)
   - [Environment](#21-environment)
   - [Cluster](#22-cluster)
   - [Namespace](#23-namespace)
   - [Node](#24-node)
   - [Workload](#25-workload)
   - [WorkloadSource](#26-workloadsource)
   - [ContainerImage](#27-containerimage)
   - [ImageRegistry](#28-imageregistry)
   - [ImageTagObservation](#29-imagetagobservation)
   - [HelmRelease](#210-helmrelease)
   - [UpdateFinding](#211-updatefinding)
   - [RiskScore](#212-riskscore)
   - [Ownership](#213-ownership)
   - [MaintenanceWindow](#214-maintenancewindow)
   - [ActionLog](#215-actionlog)
   - [ExceptionRule](#216-exceptionrule)
   - [IntegrationAccount](#217-integrationaccount)
   - [User](#218-user)
   - [UserClusterRole](#219-userclusterrole)
3. [Entity Relationship Diagram](#3-entity-relationship-diagram)
4. [Enum Reference](#4-enum-reference)
5. [General Schema Conventions](#5-general-schema-conventions)

---

## 1. Overview

KubePilot uses a single PostgreSQL database. All entities are stored in the `public` schema. UUIDs are used as primary keys throughout (PostgreSQL `uuid` type, generated with `gen_random_uuid()`). Timestamps are stored as `timestamptz` (UTC).

JSON columns use PostgreSQL `jsonb` for indexing and operator support.

---

## 2. Entity Reference

---

### 2.1 Environment

**Role:** An Environment groups clusters by operational stage (production, staging, dev, etc.). It carries a `criticality_weight` that the scoring engine uses as the environment multiplier. Every cluster belongs to exactly one environment.

**Table:** `environments`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK, default gen_random_uuid() | Surrogate primary key |
| `name` | `text` | NO | UNIQUE, NOT NULL | Human-readable name, e.g. "Production" |
| `slug` | `text` | NO | UNIQUE, NOT NULL, CHECK slug ~ '^[a-z0-9-]+$' | URL-safe identifier used in API paths and scoring lookup |
| `criticality_weight` | `numeric(4,2)` | NO | NOT NULL, CHECK between 0.0 and 2.0 | Multiplier applied to all risk scores for clusters in this environment (prod=1.0, preprod=0.7, staging=0.5, dev=0.3) |
| `color` | `text` | YES | NULL allowed | Hex color code for UI badge, e.g. "#e53935" |

**Relationships:**
- One Environment → many Clusters

**Indexes:**
- `environments_pkey` on `id` (implicit)
- `environments_slug_key` on `slug` (implicit from UNIQUE)

**UI Usage:** Environment badges appear on the cluster list, cluster detail, and findings list. The Overview page groups stats by environment.

---

### 2.2 Cluster

**Role:** Represents a single Kubernetes cluster registered with KubePilot. The K8s Collector maintains one informer set per cluster. Status reflects whether the collector is actively connected.

**Table:** `clusters`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `name` | `text` | NO | UNIQUE, NOT NULL | Internal identifier, matches kubeconfig context name |
| `display_name` | `text` | YES | | Human-friendly name shown in the UI |
| `kubeconfig_ref` | `text` | YES | | Reference to a Kubernetes Secret in the `kubepilot` namespace containing the kubeconfig (format: `namespace/secret-name`). NULL for the in-cluster primary cluster |
| `endpoint` | `text` | YES | | API server URL, populated from kubeconfig or discovery |
| `version` | `text` | YES | | Kubernetes server version string (e.g. "v1.29.3"), updated on each sync |
| `environment_id` | `uuid` | NO | FK → environments.id, NOT NULL | Environment this cluster belongs to |
| `last_seen_at` | `timestamptz` | YES | | Timestamp of the most recent successful connection or resource event |
| `status` | `text` | NO | NOT NULL, CHECK IN ('connected','disconnected','error','pending') | Current collector connectivity status |
| `annotations` | `jsonb` | YES | | Arbitrary key-value metadata (team, owner, region, etc.) stored as a JSON object |

**Relationships:**
- Belongs to one Environment
- One Cluster → many Namespaces
- One Cluster → many Nodes
- One Cluster → many Workloads
- One Cluster → many HelmReleases
- One Cluster → many MaintenanceWindows

**Indexes:**
- `clusters_pkey` on `id`
- `clusters_name_key` on `name` (UNIQUE)
- `clusters_environment_id_idx` on `environment_id`
- `clusters_status_idx` on `status`

**UI Usage:** Cluster list page, cluster detail page, filter dropdowns on findings/workloads pages.

---

### 2.3 Namespace

**Role:** A Kubernetes Namespace within a cluster, mirrored from the K8s API. Used to scope workloads, Helm releases, and maintenance windows.

**Table:** `namespaces`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `cluster_id` | `uuid` | NO | FK → clusters.id, NOT NULL | Owning cluster |
| `name` | `text` | NO | NOT NULL | Kubernetes namespace name |
| `labels` | `jsonb` | YES | | Kubernetes labels as JSON object |
| `annotations` | `jsonb` | YES | | Kubernetes annotations as JSON object |
| `phase` | `text` | YES | CHECK IN ('Active','Terminating') | Kubernetes namespace phase |

**Constraints:**
- UNIQUE (`cluster_id`, `name`)

**Relationships:**
- Belongs to one Cluster
- One Namespace → many Workloads
- One Namespace → many HelmReleases

**Indexes:**
- `namespaces_pkey` on `id`
- `namespaces_cluster_id_name_key` on (`cluster_id`, `name`) (UNIQUE)
- `namespaces_cluster_id_idx` on `cluster_id`

**UI Usage:** Namespace filter on workloads and Helm releases pages.

---

### 2.4 Node

**Role:** Represents a Kubernetes node within a cluster. Tracks OS, kernel, kubelet versions to surface node-level update findings.

**Table:** `nodes`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `cluster_id` | `uuid` | NO | FK → clusters.id, NOT NULL | Owning cluster |
| `name` | `text` | NO | NOT NULL | Kubernetes node name |
| `role` | `text` | NO | NOT NULL, CHECK IN ('control-plane','worker','etcd') | Derived from node labels (`node-role.kubernetes.io/*`) |
| `os_image` | `text` | YES | | e.g. "Ubuntu 22.04.3 LTS" |
| `kernel_version` | `text` | YES | | e.g. "5.15.0-94-generic" |
| `kubelet_version` | `text` | YES | | e.g. "v1.29.3" |
| `container_runtime` | `text` | YES | | e.g. "containerd://1.7.11" |
| `capacity` | `jsonb` | YES | | CPU and memory capacity (raw K8s resource quantities as JSON) |
| `allocatable` | `jsonb` | YES | | Allocatable CPU and memory as JSON |
| `conditions` | `jsonb` | YES | | Array of K8s NodeCondition objects (type, status, message) |
| `taints` | `jsonb` | YES | | Array of K8s taint objects |
| `labels` | `jsonb` | YES | | Node labels as JSON object |
| `last_seen_at` | `timestamptz` | YES | | Last time a Watch event was received for this node |

**Constraints:**
- UNIQUE (`cluster_id`, `name`)

**Relationships:**
- Belongs to one Cluster

**Indexes:**
- `nodes_pkey` on `id`
- `nodes_cluster_id_name_key` on (`cluster_id`, `name`) (UNIQUE)
- `nodes_cluster_id_idx` on `cluster_id`
- `nodes_role_idx` on `role`

**UI Usage:** Node list on cluster detail page. Node conditions used in exposure calculation for scoring.

---

### 2.5 Workload

**Role:** Central entity representing a Kubernetes workload (Deployment, StatefulSet, DaemonSet, CronJob, or standalone Pod). This is the primary target for image update findings. Workloads are discovered and kept up-to-date by the K8s Collector.

**Table:** `workloads`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `cluster_id` | `uuid` | NO | FK → clusters.id, NOT NULL | Owning cluster |
| `namespace_id` | `uuid` | NO | FK → namespaces.id, NOT NULL | Owning namespace |
| `name` | `text` | NO | NOT NULL | Kubernetes resource name |
| `kind` | `text` | NO | NOT NULL, CHECK IN ('Deployment','StatefulSet','DaemonSet','CronJob','Pod') | Kubernetes resource kind |
| `uid` | `text` | NO | NOT NULL | Kubernetes UID (stable across renames, used for Headlamp plugin lookups) |
| `replicas_desired` | `integer` | YES | | Desired replica count (null for DaemonSet/CronJob) |
| `replicas_ready` | `integer` | YES | | Ready replica count |
| `labels` | `jsonb` | YES | | Kubernetes labels |
| `annotations` | `jsonb` | YES | | Kubernetes annotations |
| `owner_references` | `jsonb` | YES | | Kubernetes ownerReferences array (for ReplicaSet → Deployment traversal) |
| `last_observed_at` | `timestamptz` | NO | NOT NULL | Timestamp of last Watch event |

**Constraints:**
- UNIQUE (`cluster_id`, `namespace_id`, `name`, `kind`)

**Relationships:**
- Belongs to one Cluster
- Belongs to one Namespace
- One Workload → many ContainerImages
- One Workload → one WorkloadSource
- One Workload → many UpdateFindings (via `target_id` where `target_kind='Workload'`)
- One Workload → many Ownership records

**Indexes:**
- `workloads_pkey` on `id`
- `workloads_cluster_ns_name_kind_key` on (`cluster_id`, `namespace_id`, `name`, `kind`) (UNIQUE)
- `workloads_uid_idx` on `uid` (used by Headlamp plugin)
- `workloads_cluster_id_idx` on `cluster_id`
- `workloads_namespace_id_idx` on `namespace_id`
- `workloads_kind_idx` on `kind`

**UI Usage:** Workload list with update status badges. Workload detail page shows images, findings, score, and source (Helm/Argo CD).

---

### 2.6 WorkloadSource

**Role:** Tracks how a workload is managed — raw Kubernetes manifest, Helm release, or Argo CD Application. This determines which update pathways apply and informs the risk score (managed workloads are easier to roll back).

**Table:** `workload_sources`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `workload_id` | `uuid` | NO | FK → workloads.id, UNIQUE, NOT NULL | One-to-one with Workload |
| `source_type` | `text` | NO | NOT NULL, CHECK IN ('helm','argocd','raw','kustomize','unknown') | Management type |
| `source_ref` | `text` | YES | | For Argo CD: the app name. For Kustomize: the overlay path. Free-form reference string. |
| `helm_release_id` | `uuid` | YES | FK → helm_releases.id | Set when source_type = 'helm' |
| `argocd_app_name` | `text` | YES | | Argo CD Application name when source_type = 'argocd' |

**Relationships:**
- Belongs to one Workload (1:1)
- Optionally references one HelmRelease

**Indexes:**
- `workload_sources_workload_id_key` on `workload_id` (UNIQUE)
- `workload_sources_helm_release_id_idx` on `helm_release_id`

**UI Usage:** "Managed by" indicator on workload detail page. Source type affects rollback risk factor in scoring.

---

### 2.7 ContainerImage

**Role:** Represents an individual container (or init container) within a workload, with its image reference as deployed. One workload can have multiple containers. Each ContainerImage is the direct input to the Image Watcher.

**Table:** `container_images`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `workload_id` | `uuid` | NO | FK → workloads.id, NOT NULL | Owning workload |
| `container_name` | `text` | NO | NOT NULL | Container name within the pod spec |
| `image_ref` | `text` | NO | NOT NULL | Full image reference as specified in the pod spec, e.g. `docker.io/library/nginx:1.25.3` |
| `registry` | `text` | NO | NOT NULL | Parsed registry host, e.g. `docker.io` |
| `repository` | `text` | NO | NOT NULL | Parsed repository path, e.g. `library/nginx` |
| `tag` | `text` | YES | | Parsed tag, e.g. `1.25.3`. NULL if digest-only reference |
| `digest` | `text` | YES | | Image digest (sha256:…) if known |
| `is_init_container` | `boolean` | NO | NOT NULL, DEFAULT false | True for initContainers |

**Constraints:**
- UNIQUE (`workload_id`, `container_name`)

**Relationships:**
- Belongs to one Workload
- Referenced by ImageTagObservation (registry + repository + tag lookup)

**Indexes:**
- `container_images_pkey` on `id`
- `container_images_workload_id_container_name_key` on (`workload_id`, `container_name`) (UNIQUE)
- `container_images_registry_repository_idx` on (`registry`, `repository`)

**UI Usage:** Container list on workload detail. Badge indicates init vs. regular container. Image findings link back through this entity.

---

### 2.8 ImageRegistry

**Role:** Configuration record for a container registry that KubePilot is allowed to query. Controls authentication, rate limiting, and privacy settings.

**Table:** `image_registries`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `name` | `text` | NO | UNIQUE, NOT NULL | Human-readable name, e.g. "Docker Hub" |
| `url` | `text` | NO | UNIQUE, NOT NULL | Registry base URL, e.g. `https://registry-1.docker.io` |
| `auth_type` | `text` | NO | NOT NULL, CHECK IN ('none','basic','token','ecr','gcr','acr') | Authentication method |
| `credentials_ref` | `text` | YES | | Reference to K8s Secret (`namespace/secret-name`) holding credentials. Never stored inline. |
| `rate_limit_rps` | `integer` | YES | CHECK > 0 | Max requests per second to this registry (used by Image Watcher throttle) |
| `is_private` | `boolean` | NO | NOT NULL, DEFAULT false | Whether this is a private registry requiring auth for all operations |

**Relationships:**
- One ImageRegistry → many ImageTagObservations

**Indexes:**
- `image_registries_pkey` on `id`
- `image_registries_url_key` on `url` (UNIQUE)

**UI Usage:** Integrations page, registry configuration. Rate limit and status shown in connector health.

---

### 2.9 ImageTagObservation

**Role:** Records the existence and metadata of a specific image tag as observed in an OCI registry. This is the append-only evidence table used for update detection. The Image Watcher writes here; the Scoring Engine reads here.

**Table:** `image_tag_observations`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `registry_id` | `uuid` | NO | FK → image_registries.id, NOT NULL | Registry where this tag was observed |
| `repository` | `text` | NO | NOT NULL | Repository path, e.g. `library/nginx` |
| `tag` | `text` | NO | NOT NULL | Tag string, e.g. `1.26.0` |
| `semver_parsed` | `text` | YES | | Normalized semver string if the tag is parseable as semver (for comparison). NULL for non-semver tags. |
| `pushed_at` | `timestamptz` | YES | | Tag push timestamp from registry manifest metadata, if available |
| `observed_at` | `timestamptz` | NO | NOT NULL, DEFAULT now() | When KubePilot first observed this tag |
| `digest` | `text` | YES | | Image manifest digest at observation time (sha256:…). Used for `latest` tag change detection. |

**Constraints:**
- UNIQUE (`registry_id`, `repository`, `tag`)

**Relationships:**
- Belongs to one ImageRegistry

**Indexes:**
- `image_tag_observations_pkey` on `id`
- `image_tag_observations_registry_repo_tag_key` on (`registry_id`, `repository`, `tag`) (UNIQUE)
- `image_tag_observations_repository_idx` on `repository`
- `image_tag_observations_semver_idx` on `semver_parsed` WHERE `semver_parsed IS NOT NULL`

**UI Usage:** "Available version" field on image findings. Tag history timeline on workload detail.

---

### 2.10 HelmRelease

**Role:** Represents a Helm release deployed in a cluster namespace. The K8s Collector reads Helm release metadata from the cluster (stored as Secrets by Helm 3). The Helm Watcher checks for new chart versions and creates findings.

**Table:** `helm_releases`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `cluster_id` | `uuid` | NO | FK → clusters.id, NOT NULL | Owning cluster |
| `namespace_id` | `uuid` | NO | FK → namespaces.id, NOT NULL | Namespace where the release is installed |
| `release_name` | `text` | NO | NOT NULL | Helm release name |
| `chart_name` | `text` | NO | NOT NULL | Chart name (without version), e.g. `nginx-ingress` |
| `chart_version` | `text` | NO | NOT NULL | Deployed chart version, e.g. `4.9.1` |
| `app_version` | `text` | YES | | Application version embedded in the chart, e.g. `1.10.0` |
| `repo_url` | `text` | YES | | Helm repository URL this chart was sourced from |
| `values_hash` | `text` | YES | | SHA256 of the deployed values.yaml (for change detection) |
| `status` | `text` | NO | NOT NULL, CHECK IN ('deployed','failed','pending-install','pending-upgrade','superseded','uninstalling') | Helm release status |
| `last_deployed_at` | `timestamptz` | YES | | Timestamp of last successful helm install/upgrade |

**Constraints:**
- UNIQUE (`cluster_id`, `namespace_id`, `release_name`)

**Relationships:**
- Belongs to one Cluster
- Belongs to one Namespace
- Referenced by WorkloadSource (helm_release_id)
- One HelmRelease → many UpdateFindings (via `target_id` where `target_kind='HelmRelease'`)

**Indexes:**
- `helm_releases_pkey` on `id`
- `helm_releases_cluster_ns_name_key` on (`cluster_id`, `namespace_id`, `release_name`) (UNIQUE)
- `helm_releases_cluster_id_idx` on `cluster_id`
- `helm_releases_chart_name_idx` on `chart_name`

**UI Usage:** Helm releases list, Helm release detail page. "Managed by" badge on workload detail.

---

### 2.11 UpdateFinding

**Role:** The central operational entity. A Finding represents a detected available update for a target (Workload, HelmRelease, or Node). Findings have a lifecycle status managed by operators. Each Finding has at most one active RiskScore.

**Table:** `update_findings`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `kind` | `text` | NO | NOT NULL, CHECK IN ('image','helm','node','k8s-api') | Type of update finding |
| `target_id` | `uuid` | NO | NOT NULL | UUID of the target entity (Workload, HelmRelease, or Node) |
| `target_kind` | `text` | NO | NOT NULL, CHECK IN ('Workload','HelmRelease','Node') | Discriminator for target_id polymorphism |
| `current_version` | `text` | NO | NOT NULL | Currently deployed version (tag, chart version, or k8s version) |
| `available_version` | `text` | NO | NOT NULL | Newest available version detected |
| `update_type` | `text` | NO | NOT NULL, CHECK IN ('patch','minor','major','unknown') | Semver update type, or 'unknown' for non-semver |
| `is_breaking` | `boolean` | NO | NOT NULL, DEFAULT false | True if changelog analysis or known breakage data indicates a breaking change |
| `changelog_url` | `text` | YES | | URL to relevant changelog or release notes |
| `status` | `text` | NO | NOT NULL, DEFAULT 'open', CHECK IN ('open','planned','approved','ignored','blocked') | Operator workflow status |
| `status_changed_by` | `uuid` | YES | FK → users.id | User who last changed the status |
| `status_changed_at` | `timestamptz` | YES | | When the status was last changed |
| `first_detected_at` | `timestamptz` | NO | NOT NULL, DEFAULT now() | When this finding was first created |
| `last_confirmed_at` | `timestamptz` | NO | NOT NULL, DEFAULT now() | When the available version was last re-confirmed by the watchers |

**Constraints:**
- UNIQUE (`target_id`, `target_kind`, `kind`, `available_version`)

**Relationships:**
- Polymorphic target: Workload, HelmRelease, or Node
- One UpdateFinding → one RiskScore
- Referenced by ActionLog

**Indexes:**
- `update_findings_pkey` on `id`
- `update_findings_target_idx` on (`target_id`, `target_kind`)
- `update_findings_status_idx` on `status`
- `update_findings_kind_idx` on `kind`
- `update_findings_first_detected_idx` on `first_detected_at`

**UI Usage:** Findings list (main operational view). Color-coded by severity from the associated RiskScore. Status dropdown for workflow actions.

---

### 2.12 RiskScore

**Role:** The computed risk score for a Finding. Recomputed by the Scoring Engine whenever the Finding or any of its context data changes. Stores the full factor breakdown for auditability.

**Table:** `risk_scores`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `finding_id` | `uuid` | NO | FK → update_findings.id, UNIQUE, NOT NULL | One-to-one with UpdateFinding |
| `score` | `numeric(5,2)` | NO | NOT NULL, CHECK between 0 and 100 | Final normalized score |
| `severity` | `text` | NO | NOT NULL, CHECK IN ('critical','high','medium','low','info') | Derived severity label from score thresholds |
| `factors` | `jsonb` | NO | NOT NULL | JSON object containing each factor name, raw value, weight, and weighted contribution. Full breakdown for UI display. |
| `computed_at` | `timestamptz` | NO | NOT NULL, DEFAULT now() | When this score was last computed |

**Relationships:**
- Belongs to one UpdateFinding (1:1)

**Indexes:**
- `risk_scores_pkey` on `id`
- `risk_scores_finding_id_key` on `finding_id` (UNIQUE)
- `risk_scores_score_idx` on `score DESC` (for sorted queries)
- `risk_scores_severity_idx` on `severity`

**UI Usage:** Score column in findings list. Score breakdown panel on finding detail page. Severity filter. Overview page severity distribution chart.

---

### 2.13 Ownership

**Role:** Maps a target entity (Workload or HelmRelease) to an owner identity (person, team, or service account). Used by the scoring engine for service criticality and by the notification engine for routing alerts.

**Table:** `ownerships`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `target_id` | `uuid` | NO | NOT NULL | UUID of the owned entity |
| `target_kind` | `text` | NO | NOT NULL, CHECK IN ('Workload','HelmRelease') | Discriminator for target_id |
| `owner_name` | `text` | NO | NOT NULL | Owner display name (person or team name) |
| `owner_email` | `text` | YES | | Owner contact email for alert routing |
| `team` | `text` | YES | | Team name for grouping in the UI |
| `source` | `text` | NO | NOT NULL, CHECK IN ('annotation','manual','imported') | How this ownership record was established |

**Relationships:**
- Polymorphic target: Workload or HelmRelease

**Indexes:**
- `ownerships_pkey` on `id`
- `ownerships_target_idx` on (`target_id`, `target_kind`)
- `ownerships_team_idx` on `team`

**UI Usage:** Owner column on workload and findings list. Owner filter. Alert routing for notifications.

---

### 2.14 MaintenanceWindow

**Role:** Defines a recurring time window during which updates to a specific cluster or namespace are considered "low risk" (inside-window factor reduces the score). Used by the scoring engine and the Job Scheduler.

**Table:** `maintenance_windows`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `name` | `text` | NO | NOT NULL | Human-readable name, e.g. "Saturday night prod window" |
| `cluster_id` | `uuid` | YES | FK → clusters.id | Scope: specific cluster. NULL means applies globally. |
| `namespace_id` | `uuid` | YES | FK → namespaces.id | Scope: specific namespace within the cluster. NULL means cluster-wide. |
| `cron_expression` | `text` | NO | NOT NULL | Standard 5-field cron expression for window start, e.g. `0 22 * * 6` |
| `duration_minutes` | `integer` | NO | NOT NULL, CHECK > 0 | Duration of the maintenance window in minutes |
| `timezone` | `text` | NO | NOT NULL, DEFAULT 'UTC' | IANA timezone for cron interpretation, e.g. `Europe/Paris` |
| `enabled` | `boolean` | NO | NOT NULL, DEFAULT true | Whether this window is active |

**Relationships:**
- Optionally scoped to one Cluster
- Optionally scoped to one Namespace

**Indexes:**
- `maintenance_windows_pkey` on `id`
- `maintenance_windows_cluster_id_idx` on `cluster_id`
- `maintenance_windows_enabled_idx` on `enabled` WHERE `enabled = true`

**UI Usage:** Maintenance windows page. "Inside window" indicator on finding detail when a window is currently active.

---

### 2.15 ActionLog

**Role:** Immutable append-only audit trail of every operator action taken through the KubePilot API. Never deleted or updated. Used for compliance, debugging, and change history display.

**Table:** `action_logs`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `user_id` | `uuid` | YES | FK → users.id | User who performed the action. NULL for system-generated actions (e.g. auto-expiry of exceptions). |
| `action_type` | `text` | NO | NOT NULL | Action type string, e.g. `FINDING_STATUS_CHANGE`, `CLUSTER_SYNC`, `EXCEPTION_CREATED` |
| `target_id` | `uuid` | YES | | UUID of the affected entity |
| `target_kind` | `text` | YES | | Entity type string |
| `payload` | `jsonb` | YES | | Action-specific data (e.g. `{ "old_status": "open", "new_status": "planned" }`) |
| `result` | `text` | NO | NOT NULL, CHECK IN ('success','failure') | Outcome |
| `message` | `text` | YES | | Human-readable description or error message |
| `created_at` | `timestamptz` | NO | NOT NULL, DEFAULT now() | When the action was recorded |

**Relationships:**
- Belongs to one User (nullable for system actions)

**Indexes:**
- `action_logs_pkey` on `id`
- `action_logs_user_id_idx` on `user_id`
- `action_logs_target_idx` on (`target_id`, `target_kind`)
- `action_logs_created_at_idx` on `created_at DESC`

**UI Usage:** Audit log panel on finding detail page. Global audit log page for admins. "Changed by" attribution on finding status.

---

### 2.16 ExceptionRule

**Role:** Changes how the scoring engine treats findings matching a given scope. Rules can be time-limited (`expires_at`) and/or toggled off (`is_active`) without deleting them. `internal/scoring.ScoreFinding` evaluates active, non-expired rules after computing the base score and applies at most one — the most specific scope match wins (workload > image_pattern > namespace > cluster > global). See `docs/scoring.md` §9.

**Table:** `exception_rules`

Scope is expressed as discrete nullable columns rather than a generic `scope_kind`/`scope_selector` pair — a rule's scope is whichever of `workload_id`, `image_pattern`, or `cluster_id`(+`namespace_name`) is set; a rule with none of them set is global.

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `name` | `text` | NO | NOT NULL | Human-readable label, shown on the "Exception Applied" badge |
| `rule_type` | `text` | NO | NOT NULL, default `'suppress'` | Effect on a matched finding: `suppress` (score 0, severity info), `reduce_severity` (down one band), `accept_risk` (status forced to `ignored`) — see `models.ExceptionRuleType*` |
| `cluster_id` | `uuid` | YES | FK → clusters.id, ON DELETE CASCADE | Cluster scope (with `namespace_name` unset) or cluster+namespace scope (with it set) |
| `namespace_name` | `text` | YES | | Narrows a cluster-scoped rule to one namespace |
| `workload_id` | `uuid` | YES | FK → workloads.id, ON DELETE CASCADE | Workload scope — most specific, always wins over the others when multiple rules match |
| `finding_kind` | `text` | YES | | Optional additional filter on `update_findings.kind` (`image`/`helm`), independent of scope |
| `image_pattern` | `text` | YES | | Glob (`path.Match` semantics) matched against `registry/repository:tag` — image findings only |
| `reason` | `text` | NO | NOT NULL | Mandatory justification text for audit trail |
| `expires_at` | `timestamptz` | YES | | Rule expiry time. NULL = permanent. Expired rules have no effect. |
| `created_by_id` | `uuid` | YES | FK → users.id, ON DELETE SET NULL | User who created this rule, if created through the API |
| `metadata` | `jsonb` | NO | NOT NULL, default `'{}'` | Free-form extra context, not read by the scoring engine |
| `is_active` | `boolean` | NO | NOT NULL, default `true` | Manual on/off switch, independent of `expires_at` |

**Relationships:**
- Optionally scoped to one Cluster
- Optionally scoped to one Workload
- Optionally created by one User

**Indexes:**
- `exception_rules_pkey` on `id`
- `idx_exception_rules_cluster_id` on `cluster_id`
- `idx_exception_rules_workload_id` on `workload_id`
- `idx_exception_rules_is_active` on `is_active`
- `idx_exception_rules_rule_type` on `rule_type`

**Not yet built:** there is no REST endpoint or UI for managing exception rules — only `store.CreateExceptionRule`/`store.ListActiveExceptionRules`, used by the scoring engine and by tests. Rows have to be inserted directly today.

**UI Usage:** Exception rules management page. "Exception applied" indicator on finding detail. Expiry countdown badge.

---

### 2.17 IntegrationAccount

**Role:** Configuration for an external integration (Slack, PagerDuty, Jira, OCI registry, Argo CD). Credentials are stored in referenced Kubernetes Secrets, never inline in the database.

**Table:** `integration_accounts`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `name` | `text` | NO | UNIQUE, NOT NULL | Human-readable name, e.g. "prod-slack" |
| `integration_type` | `text` | NO | NOT NULL, CHECK IN ('slack','pagerduty','jira','oci_registry','argocd','generic_webhook') | Integration system type |
| `config` | `jsonb` | NO | NOT NULL | Non-sensitive configuration (e.g. Slack channel, Jira project key, webhook URL). No secrets stored here. |
| `credentials_ref` | `text` | YES | | Reference to K8s Secret holding tokens/passwords (`namespace/secret-name`) |
| `enabled` | `boolean` | NO | NOT NULL, DEFAULT true | Whether this integration is active |
| `last_tested_at` | `timestamptz` | YES | | When the connection was last tested |
| `last_test_status` | `text` | YES | CHECK IN ('ok','error','untested') | Result of the most recent connection test |

**Relationships:** None (standalone configuration entity)

**Indexes:**
- `integration_accounts_pkey` on `id`
- `integration_accounts_name_key` on `name` (UNIQUE)
- `integration_accounts_integration_type_idx` on `integration_type`

**UI Usage:** Integrations settings page. Status badges for each integration. "Test connection" button maps to `POST /integrations/:id/test`.

---

### 2.18 User

**Role:** Represents a human user who can log in to KubePilot. Supports local JWT login and OIDC/SSO providers. User records are created on first successful login.

**Table:** `users`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `email` | `text` | NO | UNIQUE, NOT NULL | User email address (primary identity key) |
| `name` | `text` | YES | | Display name |
| `provider` | `text` | NO | NOT NULL, CHECK IN ('local','google','github','oidc','ldap') | Authentication provider |
| `external_id` | `text` | YES | | Subject identifier from the external provider |
| `created_at` | `timestamptz` | NO | NOT NULL, DEFAULT now() | Account creation time |
| `last_login_at` | `timestamptz` | YES | | Most recent login timestamp |

**Constraints:**
- UNIQUE (`provider`, `external_id`) WHERE `external_id IS NOT NULL`

**Relationships:**
- One User → many UserClusterRoles
- One User → many ActionLog entries
- One User → many ExceptionRules (created_by)

**Indexes:**
- `users_pkey` on `id`
- `users_email_key` on `email` (UNIQUE)
- `users_provider_external_id_idx` on (`provider`, `external_id`)

**UI Usage:** User profile page. "Changed by" attribution in audit logs. Role management page for admins.

---

### 2.19 UserClusterRole

**Role:** RBAC assignment granting a user a specific role on a cluster. A user with no cluster role assignments can only see the overview. This model uses per-cluster roles; a global admin role is expressed as a UserClusterRole with `cluster_id = NULL`.

**Table:** `user_cluster_roles`

| Field | SQL Type | Nullable | Constraints | Description |
|---|---|---|---|---|
| `id` | `uuid` | NO | PK | Surrogate primary key |
| `user_id` | `uuid` | NO | FK → users.id, NOT NULL | Assigned user |
| `cluster_id` | `uuid` | YES | FK → clusters.id | Specific cluster scope. NULL = global role. |
| `role` | `text` | NO | NOT NULL, CHECK IN ('viewer','editor','admin') | Role level: viewer (read-only), editor (can change finding status, create exceptions), admin (full access including user management) |

**Constraints:**
- UNIQUE (`user_id`, `cluster_id`)

**Relationships:**
- Belongs to one User
- Optionally scoped to one Cluster

**Indexes:**
- `user_cluster_roles_pkey` on `id`
- `user_cluster_roles_user_id_cluster_id_key` on (`user_id`, `cluster_id`) (UNIQUE)
- `user_cluster_roles_user_id_idx` on `user_id`

**UI Usage:** User management page. Role badge on user list. RBAC enforced on all mutating API endpoints.

---

## 3. Entity Relationship Diagram

```
┌─────────────────┐         ┌──────────────────────────────┐
│   Environment   │         │           Cluster            │
│─────────────────│1       *│──────────────────────────────│
│ id              ├─────────┤ id                           │
│ name            │         │ name                         │
│ slug            │         │ display_name                 │
│ criticality_    │         │ environment_id (FK)          │
│   weight        │         │ status                       │
│ color           │         │ version                      │
└─────────────────┘         └────────┬─────────────────────┘
                                     │1
                        ┌────────────┼────────────────┐
                        │            │                │
                       *▼           *▼               *▼
              ┌──────────────┐ ┌──────────┐  ┌───────────────┐
              │  Namespace   │ │   Node   │  │  HelmRelease  │
              │──────────────│ │──────────│  │───────────────│
              │ id           │ │ id       │  │ id            │
              │ cluster_id   │ │cluster_id│  │ cluster_id    │
              │ name         │ │ name     │  │ namespace_id  │
              │ labels       │ │ role     │  │ release_name  │
              │ phase        │ │ os_image │  │ chart_name    │
              └──────┬───────┘ └──────────┘  │ chart_version │
                     │1                       └───────┬───────┘
                     │                               *│
                    *▼                                │
              ┌──────────────────────────────┐        │
              │          Workload            │        │
              │──────────────────────────────│        │
              │ id                           │        │
              │ cluster_id                   │        │
              │ namespace_id (FK)            │        │
              │ name, kind, uid              │        │
              │ replicas_desired/ready       │        │
              └──────┬─────────────┬─────────┘        │
                     │1            │1                  │
           ┌─────────┘           ┌─┘                  │
          *▼                    1▼                     │
  ┌───────────────────┐  ┌──────────────────┐         │
  │  ContainerImage   │  │  WorkloadSource  │         │
  │───────────────────│  │──────────────────│         │
  │ id                │  │ id               │         │
  │ workload_id (FK)  │  │ workload_id (FK) │         │
  │ container_name    │  │ source_type      │         │
  │ image_ref         │  │ helm_release_id ─┼─────────┘
  │ registry          │  │ argocd_app_name  │
  │ repository        │  └──────────────────┘
  │ tag, digest       │
  └───────────────────┘
          │*
          │ (registry + repository lookup)
         1▼
  ┌────────────────────────┐
  │     ImageRegistry      │
  │────────────────────────│
  │ id                     │
  │ name, url              │
  │ auth_type              │
  │ credentials_ref        │
  └────────────┬───────────┘
               │1
              *▼
  ┌────────────────────────────┐
  │   ImageTagObservation      │
  │────────────────────────────│
  │ id                         │
  │ registry_id (FK)           │
  │ repository, tag            │
  │ semver_parsed              │
  │ pushed_at, observed_at     │
  │ digest                     │
  └────────────────────────────┘


  UpdateFinding (polymorphic target: Workload | HelmRelease | Node)
  ┌───────────────────────────────────────────┐
  │            UpdateFinding                  │
  │───────────────────────────────────────────│
  │ id                                        │
  │ kind (image|helm|node|k8s-api)            │
  │ target_id + target_kind (polymorphic FK)  │
  │ current_version / available_version       │
  │ update_type, is_breaking                  │
  │ status                                    │
  │ first_detected_at / last_confirmed_at     │
  └──────────────┬────────────────────────────┘
                 │1
                1▼
  ┌──────────────────────────────────────────┐
  │              RiskScore                   │
  │──────────────────────────────────────────│
  │ id                                       │
  │ finding_id (FK, UNIQUE)                  │
  │ score (0-100)                            │
  │ severity                                 │
  │ factors (jsonb)                          │
  │ computed_at                              │
  └──────────────────────────────────────────┘


  ┌──────────────┐     ┌────────────────────────┐
  │    User      │1   *│   UserClusterRole       │
  │──────────────│     │────────────────────────│
  │ id           ├─────┤ user_id (FK)            │
  │ email        │     │ cluster_id (FK, NULL=*) │
  │ name         │     │ role                   │
  │ provider     │     └────────────────────────┘
  └──────┬───────┘
         │1
         ├──────────────────┐
        *▼                 *▼
  ┌──────────────┐  ┌──────────────────────┐
  │  ActionLog   │  │   ExceptionRule      │
  │──────────────│  │──────────────────────│
  │ user_id (FK) │  │ created_by (FK)      │
  │ action_type  │  │ scope_kind/selector  │
  │ target_id/   │  │ rule_type            │
  │   kind       │  │ expires_at           │
  │ payload      │  └──────────────────────┘
  │ result       │
  └──────────────┘


  ┌─────────────────────────────────┐
  │        MaintenanceWindow        │
  │─────────────────────────────────│
  │ id                              │
  │ cluster_id (FK, nullable)       │
  │ namespace_id (FK, nullable)     │
  │ cron_expression                 │
  │ duration_minutes                │
  │ timezone, enabled               │
  └─────────────────────────────────┘

  ┌────────────────────────────────┐
  │       IntegrationAccount       │
  │────────────────────────────────│
  │ id                             │
  │ name, integration_type         │
  │ config (jsonb)                 │
  │ credentials_ref                │
  │ enabled                        │
  │ last_test_status               │
  └────────────────────────────────┘

  ┌───────────────────────────────────┐
  │           Ownership               │
  │───────────────────────────────────│
  │ id                                │
  │ target_id + target_kind           │
  │   (polymorphic: Workload |        │
  │    HelmRelease)                   │
  │ owner_name, owner_email, team     │
  │ source                            │
  └───────────────────────────────────┘
```

---

## 4. Enum Reference

| Enum name | Values |
|---|---|
| cluster.status | `connected`, `disconnected`, `error`, `pending` |
| namespace.phase | `Active`, `Terminating` |
| node.role | `control-plane`, `worker`, `etcd` |
| workload.kind | `Deployment`, `StatefulSet`, `DaemonSet`, `CronJob`, `Pod` |
| workload_source.source_type | `helm`, `argocd`, `raw`, `kustomize`, `unknown` |
| image_registry.auth_type | `none`, `basic`, `token`, `ecr`, `gcr`, `acr` |
| helm_release.status | `deployed`, `failed`, `pending-install`, `pending-upgrade`, `superseded`, `uninstalling` |
| update_finding.kind | `image`, `helm`, `node`, `k8s-api` |
| update_finding.target_kind | `Workload`, `HelmRelease`, `Node` |
| update_finding.update_type | `patch`, `minor`, `major`, `unknown` |
| update_finding.status | `open`, `planned`, `approved`, `ignored`, `blocked` |
| risk_score.severity | `critical`, `high`, `medium`, `low`, `info` |
| ownership.source | `annotation`, `manual`, `imported` |
| exception_rule.scope_kind | `global`, `cluster`, `namespace`, `workload`, `image_pattern` |
| exception_rule.rule_type | `suppress`, `reduce_severity`, `accept_risk` |
| integration_account.integration_type | `slack`, `pagerduty`, `jira`, `oci_registry`, `argocd`, `generic_webhook` |
| integration_account.last_test_status | `ok`, `error`, `untested` |
| user.provider | `local`, `google`, `github`, `oidc`, `ldap` |
| user_cluster_role.role | `viewer`, `editor`, `admin` |
| action_log.result | `success`, `failure` |

---

## 5. General Schema Conventions

- **Primary keys:** UUID v4, generated by `gen_random_uuid()`. Never use sequential integers — they leak record counts and complicate multi-cluster merges.
- **Timestamps:** All `timestamptz` columns store UTC. Timezone conversion is the responsibility of the frontend.
- **JSON columns:** All `jsonb`. Never `json`. Enables GIN indexing and `@>` operator queries.
- **Soft deletes:** Not used. Entities are hard-deleted. Historical context is preserved through ActionLog and immutable observation tables (ImageTagObservation).
- **Foreign keys:** All foreign keys have explicit `ON DELETE` behavior. Most use `ON DELETE CASCADE` (e.g., deleting a cluster cascades to namespaces, nodes, workloads). Exceptions: ActionLog uses `ON DELETE SET NULL` for user_id to preserve audit records after user deletion.
- **Migrations:** Sequential numbered SQL files in `migrations/`. Applied by a Kubernetes Job on every `helm upgrade`. Never edit an existing migration file; always add a new one.
