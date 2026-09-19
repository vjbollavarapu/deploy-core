-- Reverse of 000022_capacity_placement.

ALTER TABLE servers
    DROP COLUMN IF EXISTS disk_allocated_bytes,
    DROP COLUMN IF EXISTS memory_allocated_bytes,
    DROP COLUMN IF EXISTS cpu_allocated_millis;
