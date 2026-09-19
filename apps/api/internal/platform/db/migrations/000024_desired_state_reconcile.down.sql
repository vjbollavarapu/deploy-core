-- Phase B30 down.

ALTER TABLE application_replicas
    DROP COLUMN IF EXISTS observed_exit_code,
    DROP COLUMN IF EXISTS last_reconcile_at,
    DROP COLUMN IF EXISTS next_restart_at,
    DROP COLUMN IF EXISTS restart_attempt_count;

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN (
    'DEPLOYMENT_EXECUTION',
    'BACKUP',
    'RESTORE',
    'CERTIFICATE_OPERATION',
    'NOTIFICATION_DELIVERY'
));
