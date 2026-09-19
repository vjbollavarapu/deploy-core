-- Phase B29 down.

DROP TRIGGER IF EXISTS application_replicas_set_updated_at ON application_replicas;
DROP TABLE IF EXISTS application_replicas;
