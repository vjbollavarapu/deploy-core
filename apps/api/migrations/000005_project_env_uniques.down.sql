DROP INDEX IF EXISTS environments_project_slug_active_uidx;
ALTER TABLE environments
    DROP CONSTRAINT IF EXISTS environments_project_slug_uidx;
ALTER TABLE environments
    ADD CONSTRAINT environments_project_slug_uidx UNIQUE (project_id, slug);

DROP INDEX IF EXISTS projects_org_slug_active_uidx;
ALTER TABLE projects
    DROP CONSTRAINT IF EXISTS projects_org_slug_uidx;
ALTER TABLE projects
    ADD CONSTRAINT projects_org_slug_uidx UNIQUE (organization_id, slug);
