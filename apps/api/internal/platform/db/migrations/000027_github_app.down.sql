DROP INDEX IF EXISTS git_repositories_connection_external_id_uidx;
DROP INDEX IF EXISTS git_connections_live_installation_id_uidx;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM git_connections WHERE auth_mode = 'github_app') THEN
        RAISE EXCEPTION 'cannot roll back GitHub App connections while github_app rows exist';
    END IF;
END $$;

ALTER TABLE git_connections
    DROP CONSTRAINT IF EXISTS git_connections_credential_mode_check;

ALTER TABLE git_connections
    ALTER COLUMN credential_ciphertext SET NOT NULL,
    ALTER COLUMN credential_nonce SET NOT NULL,
    ALTER COLUMN credential_key_id SET NOT NULL;

ALTER TABLE git_connections
    DROP CONSTRAINT IF EXISTS git_connections_repository_selection_check,
    DROP CONSTRAINT IF EXISTS git_connections_account_type_check,
    DROP CONSTRAINT IF EXISTS git_connections_auth_mode_check;

ALTER TABLE git_connections
    DROP COLUMN IF EXISTS repository_selection,
    DROP COLUMN IF EXISTS account_type,
    DROP COLUMN IF EXISTS account_id,
    DROP COLUMN IF EXISTS installation_id,
    DROP COLUMN IF EXISTS auth_mode;
