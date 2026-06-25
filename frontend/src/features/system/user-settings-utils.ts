import type { SettingsUser, UserGroup, UserPermission, UserRole } from '@/lib/system-settings';

export const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export type UserForm = {
  id: string;
  username: string;
  email: string;
  password: string;
  displayName: string;
  roleKeys: string[];
  disabled: boolean;
  source: 'local' | 'ldap';
};

export type RoleForm = {
  id: string;
  key: string;
  name: string;
  description: string;
  permissions: string[];
  builtin: boolean;
};

export type GroupForm = {
  id: string;
  name: string;
  description: string;
  disabled: boolean;
  memberIds: string[];
  roleKeys: string[];
};

export const emptyUserForm: UserForm = {
  id: '',
  username: '',
  email: '',
  password: '',
  displayName: '',
  roleKeys: ['viewer'],
  disabled: false,
  source: 'local',
};

export const emptyRoleForm: RoleForm = {
  id: '',
  key: '',
  name: '',
  description: '',
  permissions: [],
  builtin: false,
};

export const emptyGroupForm: GroupForm = {
  id: '',
  name: '',
  description: '',
  disabled: false,
  memberIds: [],
  roleKeys: ['viewer'],
};

export function userToForm(user: SettingsUser): UserForm {
  const directRoles = user.directRoles ?? user.roles;
  return {
    id: user.id,
    username: user.username,
    email: user.email,
    password: '',
    displayName: user.displayName,
    roleKeys: directRoles?.length ? directRoles.map(role => role.key) : [user.role || 'viewer'],
    disabled: user.disabled,
    source: user.source || 'local',
  };
}

export function formToUser(form: UserForm): SettingsUser {
  return {
    id: form.id,
    username: form.username,
    email: form.email,
    displayName: form.displayName,
    role: form.roleKeys[0] || 'viewer',
    source: form.source,
    roles: [],
    disabled: form.disabled,
    created_at: '',
    updated_at: '',
    permissions: [],
  };
}

export function roleToForm(role: UserRole): RoleForm {
  return {
    id: role.id,
    key: role.key,
    name: role.name,
    description: role.description,
    permissions: role.permissions,
    builtin: role.builtin,
  };
}

export function groupToForm(group: UserGroup): GroupForm {
  return {
    id: group.id,
    name: group.name,
    description: group.description,
    disabled: group.disabled,
    memberIds: group.members?.map(user => user.id) ?? [],
    roleKeys: group.roles?.map(role => role.key) ?? [],
  };
}

export function upsertById<T extends { id: string }>(items: T[], item: T) {
  return items.some(current => current.id === item.id)
    ? items.map(current => (current.id === item.id ? item : current))
    : [...items, item];
}

export function groupPermissions(permissions: UserPermission[]) {
  return permissions.reduce<Record<string, UserPermission[]>>((groups, permission) => {
    const category = permission.category || '其他';
    groups[category] = [...(groups[category] ?? []), permission];
    return groups;
  }, {});
}

export function filterPermissionGroups(
  groups: Record<string, UserPermission[]>,
  query: string
): Record<string, UserPermission[]> {
  const keyword = query.trim().toLowerCase();
  if (!keyword) return groups;
  return Object.fromEntries(
    Object.entries(groups)
      .map(([category, items]) => [
        category,
        items.filter(item =>
          [item.key, item.name, item.description, item.category]
            .join(' ')
            .toLowerCase()
            .includes(keyword)
        ),
      ])
      .filter(([, items]) => items.length > 0)
  );
}

export function roleNames(roles?: UserRole[]) {
  if (!roles?.length) return '未分配角色';
  return roles.map(role => role.name || role.key).join(' / ');
}

export function userSearchText(user: SettingsUser) {
  return [user.username, user.displayName, user.email, user.source, roleNames(user.roles)]
    .join(' ')
    .toLowerCase();
}

export function groupSearchText(group: UserGroup) {
  return [
    group.name,
    group.description,
    group.members?.map(user => user.username).join(' '),
    roleNames(group.roles),
  ]
    .join(' ')
    .toLowerCase();
}

export function roleSearchText(role: UserRole) {
  return [role.key, role.name, role.description, role.permissions.join(' ')]
    .join(' ')
    .toLowerCase();
}

export function formatDateTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString();
}

export function formatLastLogin(value?: string) {
  return value ? formatDateTime(value) : '从未登录';
}

export function isDefaultAdminUser(user: { username?: string }) {
  return user.username === 'admin';
}
