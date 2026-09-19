-- Phase B31: security hardening — audit DELETE forbid + refresh session family.

CREATE OR REPLACE FUNCTION reject_audit_logs_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only'
        USING ERRCODE = 'restrict_violation';
END;
$$;

DROP TRIGGER IF EXISTS audit_logs_forbid_delete ON audit_logs;
CREATE TRIGGER audit_logs_forbid_delete
    BEFORE DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION reject_audit_logs_delete();

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS family_id UUID,
    ADD COLUMN IF NOT EXISTS replaced_by_session_id UUID REFERENCES sessions (id) ON DELETE SET NULL;

UPDATE sessions SET family_id = id WHERE family_id IS NULL;

ALTER TABLE sessions
    ALTER COLUMN family_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS sessions_family_id_idx ON sessions (family_id)
    WHERE revoked_at IS NULL;

COMMENT ON COLUMN sessions.family_id IS
    'Refresh-token rotation family; reuse of a revoked token revokes the whole family';
COMMENT ON COLUMN sessions.replaced_by_session_id IS
    'Session that superseded this one during refresh rotation';
