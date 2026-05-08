-- KubePilot initial database schema
-- Compatible with PostgreSQL 14+

-- ---------------------------------------------------------------------------
-- Extensions
-- ---------------------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ---------------------------------------------------------------------------
-- ENUM types
-- ---------------------------------------------------------------------------
DO $$ BEGIN
    CREATE TYPE cluster_status AS ENUM ('unknown', 'healthy', 'degraded', 'unreachable');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE workload_kind AS ENUM ('Deployment', 'DaemonSet', 'StatefulSet');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE workload_health AS ENUM ('healthy', 'degraded', 'warning', 'unknown');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE finding_kind AS ENUM ('image', 'helm', 'node');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE update_type AS ENUM ('patch', 'minor', 'major', 'unknown');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE finding_severity AS ENUM ('critical', 'high', 'medium', 'low', 'info');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE finding_status AS ENUM ('open', 'planned', 'ignored', 'approved', 'blocked', 'resolved');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE user_role AS ENUM ('admin', 'operator', 'viewer');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ---------------------------------------------------------------------------
-- environments
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS environments (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT        NOT NULL,
    slug                TEXT        NOT NULL,
    criticality_weight  FLOAT8      NOT NULL DEFAULT 1.0,
    color               TEXT        NOT NULL DEFAULT '#6B7280',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_environments_name UNIQUE (name),
    CONSTRAINT uq_environments_slug UNIQUE (slug)
);

-- ---------------------------------------------------------------------------
-- clusters
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS clusters (
    id              UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    environment_id  UUID            REFERENCES environments(id) ON DELETE SET NULL,
    name            TEXT            NOT NULL,
    slug            TEXT            NOT NULL,
    provider        TEXT            NOT NULL DEFAULT 'unknown',
    region          TEXT,
    k8s_version     TEXT,
    status          TEXT            NOT NULL DEFAULT 'unknown',
    last_seen_at    TIMESTAMPTZ,
    kubeconfig_ref  TEXT,
    api_endpoint    TEXT,
    tls_insecure    BOOLEAN         NOT NULL DEFAULT FALSE,
    annotations     JSONB           NOT NULL DEFAULT '{}',
    labels          JSONB           NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_clusters_name UNIQUE (name),
    CONSTRAINT uq_clusters_slug UNIQUE (slug)
);

CREATE INDEX IF NOT EXISTS idx_clusters_environment_id ON clusters(environment_id);
CREATE INDEX IF NOT EXISTS idx_clusters_status ON clusters(status);

-- ---------------------------------------------------------------------------
-- namespaces
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS namespaces (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id  UUID        NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'Active',
    labels      JSONB       NOT NULL DEFAULT '{}',
    annotations JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_namespaces_cluster_name UNIQUE (cluster_id, name)
);

CREATE INDEX IF NOT EXISTS idx_namespaces_cluster_id ON namespaces(cluster_id);

-- ---------------------------------------------------------------------------
-- nodes
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS nodes (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id          UUID        NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    name                TEXT        NOT NULL,
    role                TEXT        NOT NULL DEFAULT 'worker',
    status              TEXT        NOT NULL DEFAULT 'unknown',
    k8s_version         TEXT,
    os_image            TEXT,
    kernel_version      TEXT,
    container_runtime   TEXT,
    arch                TEXT,
    capacity_cpu        TEXT,
    capacity_memory     TEXT,
    allocatable_cpu     TEXT,
    allocatable_memory  TEXT,
    labels              JSONB       NOT NULL DEFAULT '{}',
    taints              JSONB       NOT NULL DEFAULT '[]',
    conditions          JSONB       NOT NULL DEFAULT '[]',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_nodes_cluster_name UNIQUE (cluster_id, name)
);

CREATE INDEX IF NOT EXISTS idx_nodes_cluster_id ON nodes(cluster_id);
CREATE INDEX IF NOT EXISTS idx_nodes_status ON nodes(status);

-- ---------------------------------------------------------------------------
-- workloads
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workloads (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id       UUID        NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    namespace_id     UUID        REFERENCES namespaces(id) ON DELETE SET NULL,
    name             TEXT        NOT NULL,
    namespace_name   TEXT        NOT NULL,
    kind             TEXT        NOT NULL,
    replicas_desired INTEGER     NOT NULL DEFAULT 0,
    replicas_ready   INTEGER     NOT NULL DEFAULT 0,
    health_status    TEXT        NOT NULL DEFAULT 'unknown',
    labels           JSONB       NOT NULL DEFAULT '{}',
    annotations      JSONB       NOT NULL DEFAULT '{}',
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_workloads_cluster_ns_name_kind UNIQUE (cluster_id, namespace_name, name, kind)
);

CREATE INDEX IF NOT EXISTS idx_workloads_cluster_id     ON workloads(cluster_id);
CREATE INDEX IF NOT EXISTS idx_workloads_namespace_id   ON workloads(namespace_id);
CREATE INDEX IF NOT EXISTS idx_workloads_namespace_name ON workloads(namespace_name);
CREATE INDEX IF NOT EXISTS idx_workloads_health_status  ON workloads(health_status);

-- ---------------------------------------------------------------------------
-- image_registries
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS image_registries (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT        NOT NULL,
    host            TEXT        NOT NULL,
    type            TEXT        NOT NULL DEFAULT 'generic',
    credentials_ref TEXT,
    auth_config     JSONB       NOT NULL DEFAULT '{}',
    rate_limit_rpm  INTEGER     NOT NULL DEFAULT 60,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_image_registries_name UNIQUE (name),
    CONSTRAINT uq_image_registries_host UNIQUE (host)
);

-- ---------------------------------------------------------------------------
-- container_images
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS container_images (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workload_id      UUID        NOT NULL REFERENCES workloads(id) ON DELETE CASCADE,
    container_name   TEXT        NOT NULL,
    image            TEXT        NOT NULL,
    registry         TEXT        NOT NULL,
    repository       TEXT        NOT NULL,
    tag              TEXT        NOT NULL,
    digest           TEXT,
    is_init_container BOOLEAN    NOT NULL DEFAULT FALSE,
    registry_id      UUID        REFERENCES image_registries(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_container_images_workload_container UNIQUE (workload_id, container_name)
);

CREATE INDEX IF NOT EXISTS idx_container_images_workload_id  ON container_images(workload_id);
CREATE INDEX IF NOT EXISTS idx_container_images_registry_id  ON container_images(registry_id);
CREATE INDEX IF NOT EXISTS idx_container_images_registry     ON container_images(registry);
CREATE INDEX IF NOT EXISTS idx_container_images_repository   ON container_images(repository);

-- ---------------------------------------------------------------------------
-- image_tag_observations
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS image_tag_observations (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    image_id     UUID        NOT NULL REFERENCES container_images(id) ON DELETE CASCADE,
    tag          TEXT        NOT NULL,
    digest       TEXT,
    published_at TIMESTAMPTZ,
    observed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_latest    BOOLEAN     NOT NULL DEFAULT FALSE,
    CONSTRAINT uq_image_tag_obs_image_tag UNIQUE (image_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_image_tag_obs_image_id ON image_tag_observations(image_id);

-- ---------------------------------------------------------------------------
-- helm_releases
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS helm_releases (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id       UUID        NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    namespace_name   TEXT        NOT NULL,
    name             TEXT        NOT NULL,
    chart_name       TEXT        NOT NULL,
    chart_version    TEXT        NOT NULL,
    app_version      TEXT,
    repo_url         TEXT,
    status           TEXT        NOT NULL DEFAULT 'deployed',
    revision         INTEGER     NOT NULL DEFAULT 1,
    values           JSONB       NOT NULL DEFAULT '{}',
    last_deployed_at TIMESTAMPTZ,
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_helm_releases_cluster_ns_name UNIQUE (cluster_id, namespace_name, name)
);

CREATE INDEX IF NOT EXISTS idx_helm_releases_cluster_id     ON helm_releases(cluster_id);
CREATE INDEX IF NOT EXISTS idx_helm_releases_namespace_name ON helm_releases(namespace_name);

-- ---------------------------------------------------------------------------
-- workload_sources
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS workload_sources (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workload_id     UUID        NOT NULL REFERENCES workloads(id) ON DELETE CASCADE,
    helm_release_id UUID        REFERENCES helm_releases(id) ON DELETE SET NULL,
    source_type     TEXT        NOT NULL DEFAULT 'unknown',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_workload_sources_workload UNIQUE (workload_id)
);

CREATE INDEX IF NOT EXISTS idx_workload_sources_helm_release_id ON workload_sources(helm_release_id);

-- ---------------------------------------------------------------------------
-- users
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL,
    name          TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'viewer',
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_users_email UNIQUE (email)
);

-- ---------------------------------------------------------------------------
-- update_findings
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS update_findings (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id          UUID        NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    namespace_name      TEXT,
    workload_id         UUID        REFERENCES workloads(id) ON DELETE SET NULL,
    helm_release_id     UUID        REFERENCES helm_releases(id) ON DELETE SET NULL,
    container_image_id  UUID        REFERENCES container_images(id) ON DELETE SET NULL,
    kind                TEXT        NOT NULL,
    update_type         TEXT        NOT NULL,
    severity            TEXT        NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'open',
    current_version     TEXT        NOT NULL,
    latest_version      TEXT        NOT NULL,
    title               TEXT        NOT NULL,
    description         TEXT,
    release_notes       TEXT,
    cves                JSONB       NOT NULL DEFAULT '[]',
    metadata            JSONB       NOT NULL DEFAULT '{}',
    first_detected_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_observed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status_changed_at   TIMESTAMPTZ,
    status_changed_by_id UUID       REFERENCES users(id) ON DELETE SET NULL,
    planned_for         TIMESTAMPTZ,
    status_reason       TEXT,
    resolved_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_update_findings_cluster_id        ON update_findings(cluster_id);
CREATE INDEX IF NOT EXISTS idx_update_findings_workload_id       ON update_findings(workload_id);
CREATE INDEX IF NOT EXISTS idx_update_findings_helm_release_id   ON update_findings(helm_release_id);
CREATE INDEX IF NOT EXISTS idx_update_findings_container_image_id ON update_findings(container_image_id);
CREATE INDEX IF NOT EXISTS idx_update_findings_kind              ON update_findings(kind);
CREATE INDEX IF NOT EXISTS idx_update_findings_severity          ON update_findings(severity);
CREATE INDEX IF NOT EXISTS idx_update_findings_status            ON update_findings(status);
CREATE INDEX IF NOT EXISTS idx_update_findings_namespace_name    ON update_findings(namespace_name);
CREATE INDEX IF NOT EXISTS idx_update_findings_first_detected_at ON update_findings(first_detected_at DESC);

-- ---------------------------------------------------------------------------
-- risk_scores
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS risk_scores (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    finding_id           UUID        NOT NULL REFERENCES update_findings(id) ON DELETE CASCADE,
    score                FLOAT8      NOT NULL,
    severity             TEXT        NOT NULL,
    factors              JSONB       NOT NULL DEFAULT '{}',
    env_multiplier       FLOAT8      NOT NULL DEFAULT 1.0,
    exposure_multiplier  FLOAT8      NOT NULL DEFAULT 1.0,
    computed_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_risk_scores_finding_id UNIQUE (finding_id)
);

CREATE INDEX IF NOT EXISTS idx_risk_scores_finding_id ON risk_scores(finding_id);
CREATE INDEX IF NOT EXISTS idx_risk_scores_score      ON risk_scores(score DESC);

-- ---------------------------------------------------------------------------
-- ownerships
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ownerships (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    workload_id  UUID        REFERENCES workloads(id) ON DELETE CASCADE,
    cluster_id   UUID        REFERENCES clusters(id) ON DELETE CASCADE,
    team_name    TEXT        NOT NULL,
    contact_info TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ownerships_workload_id ON ownerships(workload_id);
CREATE INDEX IF NOT EXISTS idx_ownerships_cluster_id  ON ownerships(cluster_id);

-- ---------------------------------------------------------------------------
-- maintenance_windows
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS maintenance_windows (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id  UUID        REFERENCES clusters(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    cron_expr   TEXT        NOT NULL,
    duration_minutes INTEGER NOT NULL DEFAULT 60,
    timezone    TEXT        NOT NULL DEFAULT 'UTC',
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_maintenance_windows_cluster_id ON maintenance_windows(cluster_id);
CREATE INDEX IF NOT EXISTS idx_maintenance_windows_is_active  ON maintenance_windows(is_active);

-- ---------------------------------------------------------------------------
-- action_logs
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS action_logs (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        REFERENCES users(id) ON DELETE SET NULL,
    action      TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,
    entity_id   TEXT        NOT NULL,
    details     JSONB       NOT NULL DEFAULT '{}',
    ip_address  TEXT,
    user_agent  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_action_logs_user_id     ON action_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_action_logs_entity_type ON action_logs(entity_type);
CREATE INDEX IF NOT EXISTS idx_action_logs_entity_id   ON action_logs(entity_id);
CREATE INDEX IF NOT EXISTS idx_action_logs_created_at  ON action_logs(created_at DESC);

-- ---------------------------------------------------------------------------
-- exception_rules
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS exception_rules (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id     UUID        REFERENCES clusters(id) ON DELETE CASCADE,
    workload_id    UUID        REFERENCES workloads(id) ON DELETE CASCADE,
    finding_kind   TEXT,
    image_pattern  TEXT,
    reason         TEXT        NOT NULL,
    expires_at     TIMESTAMPTZ,
    created_by_id  UUID        REFERENCES users(id) ON DELETE SET NULL,
    metadata       JSONB       NOT NULL DEFAULT '{}',
    is_active      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_exception_rules_cluster_id  ON exception_rules(cluster_id);
CREATE INDEX IF NOT EXISTS idx_exception_rules_workload_id ON exception_rules(workload_id);
CREATE INDEX IF NOT EXISTS idx_exception_rules_is_active   ON exception_rules(is_active);

-- ---------------------------------------------------------------------------
-- integration_accounts
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS integration_accounts (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    type         TEXT        NOT NULL,
    config       JSONB       NOT NULL DEFAULT '{}',
    secret_ref   TEXT,
    is_enabled   BOOLEAN     NOT NULL DEFAULT TRUE,
    last_sync_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_integration_accounts_name UNIQUE (name)
);

-- ---------------------------------------------------------------------------
-- user_cluster_roles
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_cluster_roles (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cluster_id UUID        NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    role       TEXT        NOT NULL DEFAULT 'viewer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_cluster_roles UNIQUE (user_id, cluster_id)
);

CREATE INDEX IF NOT EXISTS idx_user_cluster_roles_user_id    ON user_cluster_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_cluster_roles_cluster_id ON user_cluster_roles(cluster_id);
