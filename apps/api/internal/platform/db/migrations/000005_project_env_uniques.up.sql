-- Phase B5: active-only unique slugs for soft-deleted projects/environments.

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_org_slug_uidx;
CREATE UNIQUE INDEX IF NOT EXISTS projects_org_slug_active_uidx
    ON projects (organization_id, slug)
    WHERE deleted_at IS NULL;

ALTER TABLE environments DROP CONSTRAINT IF EXISTS environments_project_slug_uidx;
CREATE UNIQUE INDEX IF NOT EXISTS environments_project_slug_active_uidx
    ON environments (project_id, slug)
    WHERE deleted_at IS NULL;
