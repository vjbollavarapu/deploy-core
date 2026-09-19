-- Phase B10: align secret algorithm default with AES-256-GCM envelope encryption.

ALTER TABLE secrets
    ALTER COLUMN algorithm SET DEFAULT 'AES-256-GCM';
