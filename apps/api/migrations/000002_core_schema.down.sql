-- Reverse of 000002_core_schema.

DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS registries;
DROP TABLE IF EXISTS git_connections;
DROP TABLE IF EXISTS secrets;
DROP TABLE IF EXISTS environment_variables;
DROP TABLE IF EXISTS certificates;
DROP TABLE IF EXISTS domains;

ALTER TABLE IF EXISTS deployments DROP COLUMN IF EXISTS target_revision_id;
ALTER TABLE IF EXISTS deployments DROP COLUMN IF EXISTS active_revision_id;

DROP TABLE IF EXISTS revisions;
DROP TABLE IF EXISTS deployment_events;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS application_configs;
DROP TABLE IF EXISTS applications;
DROP TABLE IF EXISTS server_heartbeats;
DROP TABLE IF EXISTS server_agents;
DROP TABLE IF EXISTS servers;
DROP TABLE IF EXISTS environments;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS member_roles;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;

DROP FUNCTION IF EXISTS set_updated_at();
