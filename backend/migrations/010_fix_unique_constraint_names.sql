-- 010_fix_unique_constraint_names.sql
-- Renomme les contraintes UNIQUE créées par 001_initial.sql (uq_*) vers la
-- convention par défaut de GORM (uni_*). Sans ce renommage, AutoMigrate
-- (migrator.MigrateColumnUnique) tente sans garde `DROP CONSTRAINT uni_...`
-- à chaque démarrage, ce qui échoue puisque la contrainte s'appelle uq_...
-- et fait planter le serveur (`logger.Fatal("auto migrate", ...)`).

ALTER TABLE environments        RENAME CONSTRAINT uq_environments_name       TO uni_environments_name;
ALTER TABLE environments        RENAME CONSTRAINT uq_environments_slug       TO uni_environments_slug;
ALTER TABLE clusters            RENAME CONSTRAINT uq_clusters_name           TO uni_clusters_name;
ALTER TABLE clusters            RENAME CONSTRAINT uq_clusters_slug           TO uni_clusters_slug;
ALTER TABLE image_registries    RENAME CONSTRAINT uq_image_registries_name   TO uni_image_registries_name;
ALTER TABLE image_registries    RENAME CONSTRAINT uq_image_registries_host   TO uni_image_registries_host;
ALTER TABLE workload_sources    RENAME CONSTRAINT uq_workload_sources_workload TO uni_workload_sources_workload_id;
ALTER TABLE users               RENAME CONSTRAINT uq_users_email             TO uni_users_email;
ALTER TABLE integration_accounts RENAME CONSTRAINT uq_integration_accounts_name TO uni_integration_accounts_name;
ALTER TABLE user_cluster_roles  RENAME CONSTRAINT uq_user_cluster_roles      TO idx_user_cluster;

-- risk_scores.finding_id est unique via un index taggé `uniqueIndex` (nom
-- explicite non requis par GORM ici) : convertir la contrainte de table en
-- index unique évite que MigrateColumnUnique la voie comme une contrainte
-- "orpheline" à supprimer (information_schema.table_constraints ne liste
-- que les contraintes, pas les index uniques nus).
ALTER TABLE risk_scores DROP CONSTRAINT IF EXISTS uq_risk_scores_finding_id;
DROP INDEX IF EXISTS idx_risk_scores_finding_id;
CREATE UNIQUE INDEX IF NOT EXISTS uni_risk_scores_finding_id ON risk_scores(finding_id);
