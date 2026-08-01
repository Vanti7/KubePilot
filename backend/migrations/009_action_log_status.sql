-- 009_action_log_status.sql
-- Outcome of each recorded action (success/failure) — action_logs previously
-- had no way to distinguish a completed write from one that failed partway.
-- AutoMigrate adds the column on every backend (incl. SQLite); this file
-- keeps the raw PostgreSQL schema in parity.

ALTER TABLE action_logs
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'success';

CREATE INDEX IF NOT EXISTS idx_action_logs_status ON action_logs(status);
