-- Migration 004 — Unique indexes for update_findings
-- Fixes: UpsertFinding used ON CONFLICT (cluster_id, kind, current_version) but
-- no such constraint existed → PostgreSQL rejected every insert silently.
--
-- Two composite unique indexes replace the broken fallback:
--   • uq_finding_by_image : one finding per container image per cluster
--   • uq_finding_by_helm  : one finding per Helm release per cluster
--
-- PostgreSQL NULL semantics (NULL != NULL in unique indexes) ensure that:
--   • Helm findings  (container_image_id = NULL) never conflict on the image index.
--   • Image findings (helm_release_id = NULL)    never conflict on the Helm index.

CREATE UNIQUE INDEX IF NOT EXISTS uq_finding_by_image
    ON update_findings (cluster_id, container_image_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_finding_by_helm
    ON update_findings (cluster_id, helm_release_id);
