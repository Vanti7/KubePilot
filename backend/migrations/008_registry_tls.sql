-- 008_registry_tls.sql
-- Per-registry TLS verification toggle, for self-signed registries (Harbor &
-- co). AutoMigrate adds the column on every backend (incl. SQLite); this file
-- keeps the raw PostgreSQL schema in parity.

ALTER TABLE image_registries
    ADD COLUMN IF NOT EXISTS tls_insecure boolean NOT NULL DEFAULT false;
