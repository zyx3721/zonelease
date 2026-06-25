ALTER TABLE dhcp_scopes
  ADD COLUMN IF NOT EXISTS lease_duration_seconds INT NOT NULL DEFAULT 86400;

UPDATE dhcp_scopes
SET lease_duration_seconds = lease_duration_hours * 3600
WHERE lease_duration_seconds = 86400
  AND lease_duration_hours > 0
  AND lease_duration_seconds <> -1;
