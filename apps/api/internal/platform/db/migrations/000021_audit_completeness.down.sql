-- Reverse of 000021_audit_completeness.

DROP INDEX IF EXISTS audit_logs_org_action_created_idx;
DROP INDEX IF EXISTS audit_logs_action_created_idx;
DROP TRIGGER IF EXISTS audit_logs_forbid_update ON audit_logs;
DROP TRIGGER IF EXISTS audit_logs_forbid_delete ON audit_logs;
DROP FUNCTION IF EXISTS reject_audit_logs_update();
DROP FUNCTION IF EXISTS reject_audit_logs_mutation();
