-- Phase B9: soft-delete-safe application slug uniqueness within an environment.

ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_env_slug_uidx;
CREATE UNIQUE INDEX IF NOT EXISTS applications_env_slug_active_uidx
    ON applications (environment_id, slug)
    WHERE deleted_at IS NULL;
