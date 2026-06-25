ALTER TABLE dhcp_scopes
  ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS dhcp_exclusions (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  scope_id UUID NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,
  start_ip TEXT NOT NULL,
  end_ip TEXT NOT NULL,
  external_id TEXT NOT NULL DEFAULT '',
  last_synced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_dhcp_exclusions_scope_range
  ON dhcp_exclusions(scope_id, start_ip, end_ip);
