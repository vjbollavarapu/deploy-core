-- Phase B30: desired-state reconciliation support.

-- Expand jobs.type for webhook/replicas/desired-state jobs.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check CHECK (type IN (
    'DEPLOYMENT_EXECUTION',
    'BACKUP',
    'RESTORE',
    'CERTIFICATE_OPERATION',
    'NOTIFICATION_DELIVERY',
    'WEBHOOK_DELIVERY',
    'REPLICAS_RECONCILE',
    'DESIRED_STATE_RECONCILE'
));

-- Restart / reconcile backoff on observed replica slots.
ALTER TABLE application_replicas
    ADD COLUMN IF NOT EXISTS restart_attempt_count INTEGER NOT NULL DEFAULT 0
        CHECK (restart_attempt_count >= 0),
    ADD COLUMN IF NOT EXISTS next_restart_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_reconcile_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS observed_exit_code INTEGER;

COMMENT ON COLUMN application_replicas.restart_attempt_count IS
    'Consecutive restart attempts by the desired-state reconciler; reset on healthy RUNNING';
COMMENT ON COLUMN application_replicas.next_restart_at IS
    'Earliest time the reconciler may attempt another restart (backoff)';
COMMENT ON COLUMN application_replicas.last_reconcile_at IS
    'Last time the desired-state loop touched this replica slot';
COMMENT ON COLUMN application_replicas.observed_exit_code IS
    'Last observed container exit code from agent reports (NULL if unknown)';
