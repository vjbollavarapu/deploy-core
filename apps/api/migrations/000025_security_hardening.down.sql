-- Phase B31 down.

DROP INDEX IF EXISTS sessions_family_id_idx;
ALTER TABLE sessions
    DROP COLUMN IF EXISTS replaced_by_session_id,
    DROP COLUMN IF EXISTS family_id;

DROP TRIGGER IF EXISTS audit_logs_forbid_delete ON audit_logs;
DROP FUNCTION IF EXISTS reject_audit_logs_delete();
