DROP INDEX IF EXISTS servers_org_name_active_uidx;
ALTER TABLE servers
    DROP CONSTRAINT IF EXISTS servers_org_name_uidx;
ALTER TABLE servers
    ADD CONSTRAINT servers_org_name_uidx UNIQUE (organization_id, name);
