-- Reverse of 000013_registries.

DROP INDEX IF EXISTS registries_org_name_active_uidx;

ALTER TABLE registries
    DROP COLUMN IF EXISTS username,
    DROP COLUMN IF EXISTS credential_algorithm;

ALTER TABLE registries
    ADD CONSTRAINT registries_org_name_uidx UNIQUE (organization_id, name);
