-- Bootstrap migration for DeployCore control plane.
-- Business tables are introduced in later phases.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS schema_bootstrap (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    bootstrapped_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO schema_bootstrap (id) VALUES (TRUE)
ON CONFLICT (id) DO NOTHING;
