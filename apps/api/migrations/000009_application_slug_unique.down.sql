DROP INDEX IF EXISTS applications_env_slug_active_uidx;
ALTER TABLE applications
    DROP CONSTRAINT IF EXISTS applications_env_slug_uidx;
ALTER TABLE applications
    ADD CONSTRAINT applications_env_slug_uidx UNIQUE (environment_id, slug);
