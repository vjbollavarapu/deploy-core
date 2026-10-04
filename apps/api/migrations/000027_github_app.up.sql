-- GitHub App installation identity. PAT rows keep sealed credentials.
-- The GitHub App private key is process configuration and is not stored here.

ALTER TABLE git_connections
    ADD COLUMN IF NOT EXISTS auth_mode TEXT NOT NULL DEFAULT 'pat',
    ADD COLUMN IF NOT EXISTS installation_id BIGINT,
    ADD COLUMN IF NOT EXISTS account_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS account_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS repository_selection TEXT NOT NULL DEFAULT '';

ALTER TABLE git_connections
    DROP CONSTRAINT IF EXISTS git_connections_auth_mode_check;
ALTER TABLE git_connections
    ADD CONSTRAINT git_connections_auth_mode_check
    CHECK (auth_mode IN ('pat', 'github_app'));

ALTER TABLE git_connections
    DROP CONSTRAINT IF EXISTS git_connections_account_type_check;
ALTER TABLE git_connections
    ADD CONSTRAINT git_connections_account_type_check
    CHECK (account_type IN ('', 'User', 'Organization'));

ALTER TABLE git_connections
    DROP CONSTRAINT IF EXISTS git_connections_repository_selection_check;
ALTER TABLE git_connections
    ADD CONSTRAINT git_connections_repository_selection_check
    CHECK (repository_selection IN ('', 'all', 'selected'));

ALTER TABLE git_connections
    ALTER COLUMN credential_ciphertext DROP NOT NULL,
    ALTER COLUMN credential_nonce DROP NOT NULL,
    ALTER COLUMN credential_key_id DROP NOT NULL;

ALTER TABLE git_connections
    DROP CONSTRAINT IF EXISTS git_connections_credential_mode_check;
ALTER TABLE git_connections
    ADD CONSTRAINT git_connections_credential_mode_check
    CHECK (
        (
            auth_mode = 'pat'
            AND credential_ciphertext IS NOT NULL
            AND credential_nonce IS NOT NULL
            AND credential_key_id IS NOT NULL
            AND length(credential_key_id) > 0
        )
        OR
        (
            auth_mode = 'github_app'
            AND installation_id IS NOT NULL
            AND credential_ciphertext IS NULL
            AND credential_nonce IS NULL
            AND credential_key_id IS NULL
        )
    );

CREATE UNIQUE INDEX IF NOT EXISTS git_connections_live_installation_id_uidx
    ON git_connections (installation_id)
    WHERE deleted_at IS NULL AND installation_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS git_repositories_connection_external_id_uidx
    ON git_repositories (connection_id, external_id)
    WHERE external_id <> '';
