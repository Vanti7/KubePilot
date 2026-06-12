-- 005_cluster_ssh.sql
-- Adds SSH connection support to clusters: KubePilot can SSH to a node, read its
-- kubeconfig and tunnel Kubernetes API traffic through the SSH connection.
-- Applied automatically by GORM AutoMigrate; this file is for PostgreSQL parity.

ALTER TABLE clusters
    ADD COLUMN IF NOT EXISTS connection_mode      TEXT    NOT NULL DEFAULT 'kubeconfig',
    ADD COLUMN IF NOT EXISTS ssh_host             TEXT,
    ADD COLUMN IF NOT EXISTS ssh_port             INTEGER NOT NULL DEFAULT 22,
    ADD COLUMN IF NOT EXISTS ssh_user             TEXT,
    ADD COLUMN IF NOT EXISTS ssh_password         TEXT,
    ADD COLUMN IF NOT EXISTS ssh_kubeconfig_path  TEXT,
    ADD COLUMN IF NOT EXISTS ssh_sudo             BOOLEAN NOT NULL DEFAULT false;
