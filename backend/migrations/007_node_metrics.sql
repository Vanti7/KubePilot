-- 007_node_metrics.sql
-- Time-series of node resource usage, sampled from the kubelet Summary API on
-- each collection pass. Append-only (never upserted), bounded by the
-- NODE_METRICS_RETENTION_HOURS purge. Created by GORM AutoMigrate on every
-- backend (incl. SQLite); this file keeps the raw PostgreSQL schema in parity.

CREATE TABLE IF NOT EXISTS node_metrics (
    id                       UUID PRIMARY KEY,
    cluster_id               UUID NOT NULL,
    node_id                  UUID NOT NULL,
    node_name                TEXT NOT NULL,
    timestamp                TIMESTAMPTZ NOT NULL,
    cpu_usage_nano_cores     BIGINT,
    cpu_usage_percent        DOUBLE PRECISION,
    memory_working_set_bytes BIGINT,
    memory_usage_bytes       BIGINT,
    memory_usage_percent     DOUBLE PRECISION,
    fs_used_bytes            BIGINT,
    fs_capacity_bytes        BIGINT,
    fs_used_percent          DOUBLE PRECISION,
    network_rx_bytes         BIGINT,
    network_tx_bytes         BIGINT,
    network_rx_rate          DOUBLE PRECISION,
    network_tx_rate          DOUBLE PRECISION,
    pods_running             INTEGER,
    created_at               TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_node_metric_node_ts
    ON node_metrics (node_id, timestamp);

CREATE INDEX IF NOT EXISTS idx_node_metrics_cluster_id
    ON node_metrics (cluster_id);
