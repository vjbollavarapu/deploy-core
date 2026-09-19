-- Reverse of 000012_git_providers.

DROP TABLE IF EXISTS git_webhook_deliveries;
DROP TABLE IF EXISTS git_repositories;

ALTER TABLE application_configs
    DROP COLUMN IF EXISTS git_connection_id,
    DROP COLUMN IF EXISTS auto_deploy_enabled;

ALTER TABLE git_connections
    DROP COLUMN IF EXISTS credential_algorithm,
    DROP COLUMN IF EXISTS webhook_secret_key_id,
    DROP COLUMN IF EXISTS webhook_secret_nonce,
    DROP COLUMN IF EXISTS webhook_secret_ciphertext;
