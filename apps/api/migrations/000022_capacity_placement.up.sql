-- Phase B28: server capacity allocation counters for placement.

ALTER TABLE servers
    ADD COLUMN IF NOT EXISTS cpu_allocated_millis INTEGER NOT NULL DEFAULT 0
        CHECK (cpu_allocated_millis >= 0),
    ADD COLUMN IF NOT EXISTS memory_allocated_bytes BIGINT NOT NULL DEFAULT 0
        CHECK (memory_allocated_bytes >= 0),
    ADD COLUMN IF NOT EXISTS disk_allocated_bytes BIGINT NOT NULL DEFAULT 0
        CHECK (disk_allocated_bytes >= 0);

COMMENT ON COLUMN servers.cpu_cores IS 'Total CPU capacity in cores (converted to millis as cores*1000 for placement)';
COMMENT ON COLUMN servers.cpu_allocated_millis IS 'Sum of application CPU reservations currently targeting this server';
COMMENT ON COLUMN servers.memory_allocated_bytes IS 'Sum of application memory reservations currently targeting this server';
COMMENT ON COLUMN servers.disk_allocated_bytes IS 'Sum of application disk reservations currently targeting this server';
