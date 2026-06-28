import { api } from './auth';

export type ListResponse<T> = {
  items: T[];
  total: number;
};

export type SystemBaseConfig = {
  siteName: string;
  loginName: string;
  appName: string;
  appSubtitle: string;
  iconData: string;
  resetCodeTtlMinutes: number;
  resetCaptchaTtlMinutes: number;
  passwordResetSendCooldownMinutes: number;
  passwordResetRateLimitMinutes: number;
  runtimeSyncConcurrency: number;
  dnsRecordConcurrency: number;
  dhcpScopeConcurrency: number;
  operationRefreshDelaySeconds: number;
  agentOfflineFailureCount: number;
  agentConnectionTimeoutSeconds: number;
  agentOperationTimeoutSeconds: number;
  agentFullSyncTimeoutSeconds: number;
  agentHealthCheckIntervalMinutes: number;
  agentHealthCheckConcurrency: number;
};

export type NotificationChannel = {
  id: 'email';
  name: string;
  enabled: boolean;
  passwordResetEnabled: boolean;
  config: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
  updatedAt?: string;
};

export type NotificationTemplatePreview = {
  problemSubject: string;
  problemText: string;
  recoverySubject: string;
  recoveryText: string;
  contentType?: string;
};

export type AuthProvider = {
  id: 'ldap';
  type: string;
  name: string;
  enabled: boolean;
  config: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
};

export type UserRole = {
  id: string;
  key: string;
  name: string;
  description: string;
  permissions: string[];
  builtin: boolean;
  created_at: string;
  updated_at: string;
};

export type UserPermission = {
  key: string;
  name: string;
  description: string;
  category: string;
  impliedReadPermission?: string;
};

export type SettingsUser = {
  id: string;
  username: string;
  email: string;
  displayName: string;
  role: string;
  source: 'local' | 'ldap';
  roles?: UserRole[];
  directRoles?: UserRole[];
  disabled: boolean;
  lastLoginAt?: string;
  created_at: string;
  updated_at: string;
  permissions: string[];
};

export type ManagedUserPayload = {
  username: string;
  email: string;
  password?: string;
  displayName: string;
  roleKeys: string[];
  disabled: boolean;
};

export type UserRolePayload = {
  key: string;
  name: string;
  description: string;
  permissions: string[];
};

export type UserGroup = {
  id: string;
  name: string;
  description: string;
  disabled: boolean;
  members?: SettingsUser[];
  roles?: UserRole[];
  created_at: string;
  updated_at: string;
};

export type UserGroupPayload = {
  name: string;
  description: string;
  disabled: boolean;
  memberIds: string[];
  roleKeys: string[];
};

export function fetchSystemBaseConfig() {
  return api<SystemBaseConfig>('/api/settings/base');
}

export function fetchPublicSystemBaseConfig() {
  return api<SystemBaseConfig>('/api/public/base', { auth: false });
}

export function updateSystemBaseConfig(body: SystemBaseConfig) {
  return api<SystemBaseConfig>('/api/settings/base', {
    method: 'PUT',
    body: JSON.stringify(body),
  });
}

export function fetchNotificationChannels() {
  return api<ListResponse<NotificationChannel>>('/api/settings/notifications');
}

export function updateNotificationChannel(
  id: string,
  payload: {
    enabled: boolean;
    passwordResetEnabled: boolean;
    clearConfig?: boolean;
    config: Record<string, unknown>;
  }
) {
  return api<NotificationChannel>(`/api/settings/notifications/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export function testNotificationChannel(id: string, to?: string) {
  return api<{ status: string }>(`/api/settings/notifications/${encodeURIComponent(id)}/test`, {
    method: 'POST',
    body: to ? JSON.stringify({ to }) : undefined,
  });
}

export function previewNotificationChannel(
  id: string,
  payload: { enabled: boolean; passwordResetEnabled: boolean; config: Record<string, unknown> }
) {
  return api<NotificationTemplatePreview>(
    `/api/settings/notifications/${encodeURIComponent(id)}/preview`,
    { method: 'POST', body: JSON.stringify(payload) }
  );
}

export function fetchAuthProviders() {
  return api<ListResponse<AuthProvider>>('/api/settings/auth-providers');
}

export function updateAuthProvider(
  id: string,
  payload: { name: string; enabled: boolean; config: Record<string, unknown> }
) {
  return api<AuthProvider>(`/api/settings/auth-providers/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export function testAuthProvider(id: string) {
  return api<{ status: string; matchedUsers: number }>(
    `/api/settings/auth-providers/${encodeURIComponent(id)}/test`,
    { method: 'POST' }
  );
}

export function fetchSettingsUsers() {
  return api<ListResponse<SettingsUser>>('/api/settings/users');
}

export function createSettingsUser(payload: ManagedUserPayload) {
  return api<SettingsUser>('/api/settings/users', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function updateSettingsUser(id: string, payload: ManagedUserPayload) {
  return api<SettingsUser>(`/api/settings/users/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export function deleteSettingsUser(id: string) {
  return api<void>(`/api/settings/users/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export function updateSettingsUserDisabled(id: string, disabled: boolean) {
  return api<SettingsUser>(`/api/settings/users/${encodeURIComponent(id)}/disabled`, {
    method: 'POST',
    body: JSON.stringify({ disabled }),
  });
}

export function fetchUserRoles() {
  return api<ListResponse<UserRole>>('/api/settings/roles');
}

export function createUserRole(payload: UserRolePayload) {
  return api<UserRole>('/api/settings/roles', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function updateUserRole(id: string, payload: UserRolePayload) {
  return api<UserRole>(`/api/settings/roles/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export function deleteUserRole(id: string) {
  return api<{ status: string }>(`/api/settings/roles/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

export function fetchUserGroups() {
  return api<ListResponse<UserGroup>>('/api/settings/user-groups');
}

export function createUserGroup(payload: UserGroupPayload) {
  return api<UserGroup>('/api/settings/user-groups', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export function updateUserGroup(id: string, payload: UserGroupPayload) {
  return api<UserGroup>(`/api/settings/user-groups/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export function deleteUserGroup(id: string) {
  return api<{ status: string }>(`/api/settings/user-groups/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

export function fetchUserPermissions() {
  return api<ListResponse<UserPermission>>('/api/settings/permissions');
}
