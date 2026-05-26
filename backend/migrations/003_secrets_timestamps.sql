-- Migration 003: add k8s timestamps to secrets (for envs that already applied 002)
ALTER TABLE secrets ADD COLUMN IF NOT EXISTS k8s_created_at TIMESTAMPTZ;
ALTER TABLE secrets ADD COLUMN IF NOT EXISTS k8s_updated_at TIMESTAMPTZ;
