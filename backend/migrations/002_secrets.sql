-- Migration 002: add secrets table
CREATE TABLE IF NOT EXISTS secrets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id      UUID        NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    namespace_name  TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    type            TEXT        NOT NULL DEFAULT 'Opaque',
    keys            JSONB       NOT NULL DEFAULT '[]',
    k8s_created_at  TIMESTAMPTZ,
    k8s_updated_at  TIMESTAMPTZ,
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_secret_cluster_ns_name UNIQUE (cluster_id, namespace_name, name)
);

CREATE INDEX IF NOT EXISTS idx_secrets_cluster_id     ON secrets (cluster_id);
CREATE INDEX IF NOT EXISTS idx_secrets_namespace_name ON secrets (namespace_name);
