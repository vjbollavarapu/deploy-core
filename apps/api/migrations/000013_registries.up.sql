-- Phase B17: container registries — soft-delete-safe name uniqueness + credential algorithm.

ALTER TABLE registries
    ADD COLUMN IF NOT EXISTS credential_algorithm TEXT NOT NULL DEFAULT 'AES-256-GCM',
    ADD COLUMN IF NOT EXISTS username TEXT NOT NULL DEFAULT '';

ALTER TABLE registries DROP CONSTRAINT IF EXISTS registries_org_name_uidx;

CREATE UNIQUE INDEX IF NOT EXISTS registries_org_name_active_uidx
    ON registries (organization_id, name)
    WHERE deleted_at IS NULL;
