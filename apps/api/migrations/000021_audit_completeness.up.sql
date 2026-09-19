-- Phase B27: enforce append-only audit_logs (reject UPDATE).
-- Deletes are not exposed via API; application code only INSERTs.

CREATE OR REPLACE FUNCTION reject_audit_logs_update()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only'
        USING ERRCODE = 'restrict_violation';
END;
$$;

DROP TRIGGER IF EXISTS audit_logs_forbid_update ON audit_logs;
CREATE TRIGGER audit_logs_forbid_update
    BEFORE UPDATE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION reject_audit_logs_update();

-- Remove prior delete trigger if a previous revision installed it.
DROP TRIGGER IF EXISTS audit_logs_forbid_delete ON audit_logs;
DROP FUNCTION IF EXISTS reject_audit_logs_mutation();

CREATE INDEX IF NOT EXISTS audit_logs_action_created_idx
    ON audit_logs (action, created_at DESC);

CREATE INDEX IF NOT EXISTS audit_logs_org_action_created_idx
    ON audit_logs (organization_id, action, created_at DESC);
