-- 平台用户账号表，保存本地登录用户、角色、状态和登录时间
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  username TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL,
  display_name TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'admin',
  source TEXT NOT NULL DEFAULT 'local',
  disabled BOOLEAN NOT NULL DEFAULT FALSE,
  last_login_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 用户登录会话表，保存 Bearer Token 哈希、过期时间和最近活跃时间
CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 找回密码请求表，保存重置验证码、发送渠道、有效期和使用状态
CREATE TABLE IF NOT EXISTS password_reset_requests (
  token_hash TEXT PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash TEXT NOT NULL DEFAULT '',
  channel TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 系统基础配置表，保存控制台展示、安全时长和运行态展示配置
CREATE TABLE IF NOT EXISTS system_settings (
  key TEXT PRIMARY KEY,
  value JSONB NOT NULL DEFAULT '{}',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 通知媒介配置表，保存外部通知和找回密码验证码发送媒介
CREATE TABLE IF NOT EXISTS notification_channels (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  password_reset_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  config JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 外部认证提供方表，保存 AD/LDAP 等登录方式的启用状态与配置
CREATE TABLE IF NOT EXISTS auth_providers (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  name TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  config JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 角色表，保存内置角色和自定义角色的权限集合
CREATE TABLE IF NOT EXISTS roles (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  permissions TEXT[] NOT NULL DEFAULT '{}',
  builtin BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 用户角色关联表，保存用户直接分配的角色
CREATE TABLE IF NOT EXISTS user_roles (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role_id)
);

-- 用户组表，保存用户分组及其启用状态
CREATE TABLE IF NOT EXISTS user_groups (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  disabled BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 用户组成员表，保存用户与用户组的成员关系
CREATE TABLE IF NOT EXISTS user_group_members (
  group_id UUID NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (group_id, user_id)
);

-- 用户组角色关联表，保存用户组继承的角色
CREATE TABLE IF NOT EXISTS user_group_roles (
  group_id UUID NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
  role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (group_id, role_id)
);

-- Windows DNS/DHCP Agent 服务器登记表，保存名称、角色、Agent 地址、API Key 和健康状态
CREATE TABLE IF NOT EXISTS servers (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  name TEXT NOT NULL UNIQUE,
  host TEXT NOT NULL,
  role TEXT NOT NULL,
  agent_url TEXT NOT NULL,
  api_key TEXT NOT NULL DEFAULT '',
  tls_insecure BOOLEAN NOT NULL DEFAULT false,
  status TEXT NOT NULL DEFAULT 'Unknown',
  failure_count INT NOT NULL DEFAULT 0,
  last_checked TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM servers
    GROUP BY agent_url
    HAVING COUNT(*) > 1
  ) THEN
    CREATE UNIQUE INDEX IF NOT EXISTS idx_servers_agent_url_unique ON servers(agent_url);
  END IF;
END $$;

-- DHCP 作用域快照表，保存从 DHCP Agent 同步或平台维护的作用域配置
CREATE TABLE IF NOT EXISTS dhcp_scopes (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  name TEXT NOT NULL,
  subnet TEXT NOT NULL,
  start_range TEXT NOT NULL,
  end_range TEXT NOT NULL,
  lease_duration_hours INT NOT NULL DEFAULT 24,
  lease_duration_seconds INT NOT NULL DEFAULT 86400,
  state TEXT NOT NULL DEFAULT 'Active',
  server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  external_id TEXT NOT NULL DEFAULT '',
  last_synced_at TIMESTAMPTZ,
  sync_status TEXT NOT NULL DEFAULT 'idle',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- DHCP 租约快照表，保存作用域下客户端租约、状态和过期时间
CREATE TABLE IF NOT EXISTS dhcp_leases (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  scope_id UUID NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,
  ip TEXT NOT NULL,
  mac TEXT NOT NULL,
  hostname TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'Active',
  expires_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  external_id TEXT NOT NULL DEFAULT '',
  last_synced_at TIMESTAMPTZ
);

-- DHCP 保留地址快照表，保存作用域下固定 IP、MAC、名称和描述
CREATE TABLE IF NOT EXISTS dhcp_reservations (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  scope_id UUID NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,
  ip TEXT NOT NULL,
  mac TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  external_id TEXT NOT NULL DEFAULT '',
  last_synced_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- DNS 区域快照表，保存从 DNS Agent 同步的区域属性和同步状态
CREATE TABLE IF NOT EXISTS dns_zones (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL DEFAULT 'Primary',
  reverse BOOLEAN NOT NULL DEFAULT FALSE,
  dynamic_update TEXT NOT NULL DEFAULT 'None',
  server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  sync_status TEXT NOT NULL DEFAULT 'idle',
  last_synced_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(server_id, name)
);

-- DNS 记录快照表，保存指定 DNS 区域下的记录名称、类型、值和 TTL
CREATE TABLE IF NOT EXISTS dns_records (
  id TEXT PRIMARY KEY,
  zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  value TEXT NOT NULL,
  ttl INT NOT NULL DEFAULT 3600,
  create_ptr BOOLEAN NOT NULL DEFAULT FALSE,
  last_synced_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 操作审计表，记录用户对服务器、DNS、DHCP、刷新和认证相关操作
CREATE TABLE IF NOT EXISTS audit_entries (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  ts TIMESTAMPTZ NOT NULL DEFAULT now(),
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  username TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  target TEXT NOT NULL,
  module TEXT NOT NULL,
  result TEXT NOT NULL,
  ip_address TEXT NOT NULL DEFAULT '',
  detail TEXT NOT NULL DEFAULT ''
);

-- 刷新任务表，记录全量刷新、区域刷新等后台同步任务状态和任务载荷
CREATE TABLE IF NOT EXISTS refresh_tasks (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  type TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'queued',
  payload JSONB NOT NULL DEFAULT '{}',
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ
);

-- 站内通知表，保存右上角通知中心展示、已读和清空状态
CREATE TABLE IF NOT EXISTS notifications (
  id UUID PRIMARY KEY DEFAULT (md5(random()::text || clock_timestamp()::text)::uuid),
  level TEXT NOT NULL DEFAULT 'info',
  title TEXT NOT NULL,
  message TEXT NOT NULL DEFAULT '',
  source_type TEXT NOT NULL DEFAULT '',
  source_id TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}',
  read_at TIMESTAMPTZ,
  dismissed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dhcp_leases_scope_id ON dhcp_leases(scope_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_dhcp_scopes_server_external ON dhcp_scopes(server_id, external_id) WHERE external_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_dhcp_leases_scope_ip ON dhcp_leases(scope_id, ip);
CREATE UNIQUE INDEX IF NOT EXISTS idx_dhcp_reservations_scope_ip ON dhcp_reservations(scope_id, ip);
CREATE INDEX IF NOT EXISTS idx_dns_zones_server_id ON dns_zones(server_id);
CREATE INDEX IF NOT EXISTS idx_dns_records_zone_id ON dns_records(zone_id);
CREATE INDEX IF NOT EXISTS idx_audit_entries_ts ON audit_entries(ts DESC);
CREATE INDEX IF NOT EXISTS idx_refresh_tasks_created_at ON refresh_tasks(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_unread ON notifications(created_at DESC) WHERE read_at IS NULL AND dismissed_at IS NULL;

INSERT INTO auth_providers(id, type, name, enabled, config)
VALUES('ldap', 'ldap', 'AD/LDAP', FALSE, '{}')
ON CONFLICT (id) DO NOTHING;

INSERT INTO roles(key, name, description, permissions, builtin)
VALUES
  ('admin', 'admin', '系统配置、Agent 管理、DNS/DHCP 资源和刷新等所有操作', ARRAY[
    'dashboard.read','servers.read','servers.manage',
    'dns.read','dns.manage','dhcp.read','dhcp.manage',
    'refresh.manage','audit.read',
    'settings.base.read','settings.base.manage',
    'settings.users.read','settings.users.manage',
    'settings.auth.read','settings.auth.manage',
    'settings.notifications.read','settings.notifications.manage'
  ], TRUE),
  ('operator', 'operator', 'DNS/DHCP 资源和 Agent 日常操作，不能修改系统配置', ARRAY[
    'dashboard.read','servers.read','servers.manage',
    'dns.read','dns.manage','dhcp.read','dhcp.manage',
    'refresh.manage','audit.read',
    'settings.base.read','settings.users.read','settings.auth.read','settings.notifications.read'
  ], TRUE),
  ('viewer', 'viewer', '只读查看仪表板、Agent、DNS/DHCP 资源和审计记录', ARRAY[
    'dashboard.read','servers.read','dns.read','dhcp.read','audit.read',
    'settings.base.read','settings.users.read','settings.auth.read','settings.notifications.read'
  ], TRUE)
ON CONFLICT (key) DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  permissions = EXCLUDED.permissions,
  builtin = TRUE,
  updated_at = now();

INSERT INTO user_roles(user_id, role_id)
SELECT u.id, r.id
FROM users u
JOIN roles r ON r.key = COALESCE(NULLIF(u.role, ''), 'viewer')
ON CONFLICT DO NOTHING;

INSERT INTO system_settings(key, value)
VALUES('base', '{
  "siteName": "ZoneLease",
  "loginName": "ZoneLease",
  "appName": "ZoneLease",
  "appSubtitle": "DNS / DHCP Control",
  "iconData": "/favicon.svg",
  "resetCodeTtlMinutes": 10,
  "resetCaptchaTtlMinutes": 1,
  "passwordResetSendCooldownMinutes": 0.5,
  "passwordResetRateLimitMinutes": 5,
  "runtimeSyncConcurrency": 3,
  "dnsRecordConcurrency": 3,
  "dhcpScopeConcurrency": 5,
  "operationRefreshDelaySeconds": 10,
  "agentOfflineFailureCount": 3,
  "agentOperationTimeoutSeconds": 20,
  "agentFullSyncTimeoutSeconds": 300,
  "agentHealthCheckIntervalMinutes": 1,
  "agentHealthCheckConcurrency": 1
}'::jsonb)
ON CONFLICT (key) DO NOTHING;

INSERT INTO notification_channels(id, name, enabled, password_reset_enabled, config)
VALUES
  ('email', '邮件媒介', FALSE, FALSE, '{}')
ON CONFLICT (id) DO NOTHING;
