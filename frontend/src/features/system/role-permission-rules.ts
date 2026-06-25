import type { UserPermission } from '@/lib/system-settings';

export function impliedReadPermissionsFrom(permissions: UserPermission[]) {
  return permissions.reduce<Record<string, string>>((rules, permission) => {
    if (permission.impliedReadPermission) {
      rules[permission.key] = permission.impliedReadPermission;
    }
    return rules;
  }, {});
}

export function normalizeRolePermissions(
  permissions: string[],
  impliedReadPermissions: Record<string, string>
) {
  const normalized = new Set<string>();
  for (const permission of permissions) {
    if (!permission) continue;
    normalized.add(permission);
    const implied = impliedReadPermissions[permission];
    if (implied) normalized.add(implied);
  }
  return Array.from(normalized).sort();
}
