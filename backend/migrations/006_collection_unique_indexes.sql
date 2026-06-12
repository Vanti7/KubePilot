-- 006_collection_unique_indexes.sql
-- Unique indexes backing the collector upserts (clause.OnConflict). These were
-- previously only implied by the Postgres schema; they are now declared on the
-- GORM models so AutoMigrate creates them on every backend (incl. SQLite).
-- This file keeps the raw PostgreSQL schema in parity.

CREATE UNIQUE INDEX IF NOT EXISTS uq_node
    ON nodes (cluster_id, name);

CREATE UNIQUE INDEX IF NOT EXISTS uq_workload
    ON workloads (cluster_id, namespace_name, name, kind);

CREATE UNIQUE INDEX IF NOT EXISTS uq_container_image
    ON container_images (workload_id, container_name);

CREATE UNIQUE INDEX IF NOT EXISTS uq_image_tag
    ON image_tag_observations (image_id, tag);

CREATE UNIQUE INDEX IF NOT EXISTS uq_helm_release
    ON helm_releases (cluster_id, namespace_name, name);

CREATE UNIQUE INDEX IF NOT EXISTS uq_secret
    ON secrets (cluster_id, namespace_name, name);
