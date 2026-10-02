-- Stable private DNS alias for a managed database. Generated once at create
-- and never recomputed from a later display-name change.

ALTER TABLE managed_databases ADD COLUMN dns_alias TEXT;

UPDATE managed_databases
SET dns_alias = 'db-' || substr(replace(id::text, '-', ''), 1, 20)
WHERE dns_alias IS NULL OR btrim(dns_alias) = '';

ALTER TABLE managed_databases ALTER COLUMN dns_alias SET NOT NULL;

CREATE UNIQUE INDEX managed_databases_env_dns_alias_active_uidx
    ON managed_databases (environment_id, dns_alias)
    WHERE deleted_at IS NULL;
