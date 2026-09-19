-- Phase B12: faster reclaim of expired job leases.

CREATE INDEX IF NOT EXISTS jobs_reclaim_idx
    ON jobs (leased_until)
    WHERE status IN ('leased', 'running') AND leased_until IS NOT NULL;
