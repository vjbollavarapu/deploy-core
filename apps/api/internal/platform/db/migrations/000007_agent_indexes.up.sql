-- Phase B7: agent token/credential lookup indexes + heartbeat retention helper index.

CREATE UNIQUE INDEX IF NOT EXISTS server_agents_registration_token_hash_uidx
    ON server_agents (registration_token_hash)
    WHERE registration_token_hash IS NOT NULL
      AND registration_used_at IS NULL
      AND registration_revoked_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS server_agents_credential_hash_uidx
    ON server_agents (credential_hash)
    WHERE status = 'active' AND credential_hash <> '';
