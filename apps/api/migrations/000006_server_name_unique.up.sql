-- Phase B6: active-only unique server names within an organization.

ALTER TABLE servers DROP CONSTRAINT IF EXISTS servers_org_name_uidx;
CREATE UNIQUE INDEX IF NOT EXISTS servers_org_name_active_uidx
    ON servers (organization_id, name)
    WHERE deleted_at IS NULL;
