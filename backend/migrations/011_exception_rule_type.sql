-- 011_exception_rule_type.sql
-- ExceptionRule gained the fields the scoring engine needs to actually apply
-- a rule: rule_type (which of suppress/reduce_severity/accept_risk to run —
-- there was previously no column for this at all), name (shown on the
-- "Exception Applied" badge), and namespace_name (the "namespace" scope from
-- docs/scoring.md §9 had no way to be expressed — only cluster/workload/
-- image_pattern/global existed). AutoMigrate adds these columns on every
-- backend (incl. SQLite); this file keeps the raw PostgreSQL schema in parity.

ALTER TABLE exception_rules
    ADD COLUMN IF NOT EXISTS name           TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS rule_type      TEXT NOT NULL DEFAULT 'suppress',
    ADD COLUMN IF NOT EXISTS namespace_name TEXT;

CREATE INDEX IF NOT EXISTS idx_exception_rules_rule_type ON exception_rules(rule_type);
