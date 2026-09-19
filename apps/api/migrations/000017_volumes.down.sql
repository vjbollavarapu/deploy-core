-- Reverse of 000017_volumes.

DROP TABLE IF EXISTS volumes;

-- Restore B22 agent command operation set (without volume attach/detach/remove extras).
ALTER TABLE agent_commands DROP CONSTRAINT IF EXISTS agent_commands_operation_check;
ALTER TABLE agent_commands ADD CONSTRAINT agent_commands_operation_check CHECK (
    operation IN (
        'DEPLOY_REVISION',
        'STOP_CONTAINER',
        'START_CONTAINER',
        'RESTART_CONTAINER',
        'REMOVE_CONTAINER',
        'FETCH_LOGS',
        'STREAM_LOGS',
        'BUILD_IMAGE',
        'PULL_IMAGE',
        'CREATE_NETWORK',
        'CREATE_VOLUME',
        'RUN_HEALTH_CHECK',
        'CREATE_BACKUP',
        'RESTORE_BACKUP',
        'PROVISION_DATABASE',
        'START_DATABASE',
        'STOP_DATABASE'
    )
);
