DROP INDEX IF EXISTS managed_databases_env_dns_alias_active_uidx;
ALTER TABLE managed_databases DROP COLUMN IF EXISTS dns_alias;
