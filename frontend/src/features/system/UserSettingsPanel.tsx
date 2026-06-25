import {
  Plus,
  Search,
  ShieldCheck,
  ToggleLeft,
  ToggleRight,
  UserCog,
  UsersRound,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import {
  createSettingsUser,
  createUserGroup,
  createUserRole,
  deleteSettingsUser,
  deleteUserGroup,
  deleteUserRole,
  fetchSettingsUsers,
  fetchUserGroups,
  fetchUserPermissions,
  fetchUserRoles,
  updateSettingsUser,
  updateSettingsUserDisabled,
  updateUserGroup,
  updateUserRole,
  type SettingsUser,
  type UserGroup,
  type UserPermission,
  type UserRole,
} from '@/lib/system-settings';
import { GroupEditorDialog, RoleEditorDialog, UserEditorDialog } from './UserSettingsDialogs';
import { impliedReadPermissionsFrom, normalizeRolePermissions } from './role-permission-rules';
import {
  emailPattern,
  emptyGroupForm,
  emptyRoleForm,
  emptyUserForm,
  filterPermissionGroups,
  formToUser,
  formatDateTime,
  formatLastLogin,
  groupPermissions,
  groupSearchText,
  groupToForm,
  isDefaultAdminUser,
  roleNames,
  roleSearchText,
  roleToForm,
  type GroupForm,
  type RoleForm,
  type UserForm,
  upsertById,
  userSearchText,
  userToForm,
} from './user-settings-utils';

type UserConfigTab = 'users' | 'groups' | 'roles';

const userConfigCards: Array<{
  id: UserConfigTab;
  title: string;
  description: string;
  icon: React.ElementType;
  color: string;
}> = [
  {
    id: 'users',
    title: '用户',
    description: '维护本地账号和允许 AD/LDAP 登录的用户',
    icon: UserCog,
    color: '#38bdf8',
  },
  {
    id: 'groups',
    title: '用户群组',
    description: '按团队聚合用户，并统一分配角色',
    icon: UsersRound,
    color: '#22c55e',
  },
  {
    id: 'roles',
    title: '用户角色',
    description: '维护内置角色和自定义权限集合',
    icon: ShieldCheck,
    color: '#f59e0b',
  },
];

export function UserSettingsPanel({ canManage = true }: { canManage?: boolean }) {
  const [active, setActive] = useState<UserConfigTab>('users');
  const [users, setUsers] = useState<SettingsUser[]>([]);
  const [roles, setRoles] = useState<UserRole[]>([]);
  const [groups, setGroups] = useState<UserGroup[]>([]);
  const [permissions, setPermissions] = useState<UserPermission[]>([]);
  const [userForm, setUserForm] = useState<UserForm>(emptyUserForm);
  const [roleForm, setRoleForm] = useState<RoleForm>(emptyRoleForm);
  const [groupForm, setGroupForm] = useState<GroupForm>(emptyGroupForm);
  const [dialog, setDialog] = useState<UserConfigTab | null>(null);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const [userResponse, roleResponse, groupResponse, permissionResponse] = await Promise.all([
        fetchSettingsUsers(),
        fetchUserRoles(),
        fetchUserGroups(),
        fetchUserPermissions(),
      ]);
      setUsers(userResponse.items);
      setRoles(roleResponse.items);
      setGroups(groupResponse.items);
      setPermissions(permissionResponse.items);
    } catch (err) {
      const message = err instanceof Error ? err.message : '读取用户配置失败';
      toast.error(message);
      setError(message.includes('当前用户无权执行此操作') ? '' : message);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const selectedCard = userConfigCards.find(card => card.id === active) ?? userConfigCards[0];
  const permissionGroups = useMemo(() => groupPermissions(permissions), [permissions]);
  const impliedReadPermissions = useMemo(
    () => impliedReadPermissionsFrom(permissions),
    [permissions]
  );

  async function saveUser() {
    const username = userForm.username.trim();
    const email = userForm.email.trim();
    if (!username) return toast.error('用户名不能为空');
    if (!emailPattern.test(email)) return toast.error('请输入有效的邮箱地址');
    if (!userForm.id && userForm.password.length < 6) return toast.error('密码至少 6 个字符');
    setBusy('user');
    try {
      const payload = {
        username,
        password: userForm.password,
        email,
        displayName: userForm.displayName.trim() || username,
        roleKeys: userForm.roleKeys.length ? userForm.roleKeys : ['viewer'],
        disabled: userForm.disabled,
      };
      const saved = userForm.id
        ? await updateSettingsUser(userForm.id, payload)
        : await createSettingsUser(payload);
      setUsers(current => upsertById(current, saved));
      setUserForm(userToForm(saved));
      setDialog(null);
      toast.success('用户配置已保存');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存用户配置失败');
    } finally {
      setBusy('');
    }
  }

  async function toggleUserDisabled(user: SettingsUser) {
    if (isDefaultAdminUser(user)) return toast.error('默认管理员不能禁用');
    setBusy(`user-${user.id}`);
    try {
      const saved = await updateSettingsUserDisabled(user.id, !user.disabled);
      setUsers(current => upsertById(current, saved));
      if (userForm.id === saved.id) setUserForm(userToForm(saved));
      toast.success(saved.disabled ? '用户已禁用' : '用户已启用');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '更新用户状态失败');
    } finally {
      setBusy('');
    }
  }

  async function removeUser() {
    if (!userForm.id) return;
    if (isDefaultAdminUser(userForm)) return toast.error('默认管理员不能删除');
    setBusy('user');
    try {
      await deleteSettingsUser(userForm.id);
      setUsers(current => current.filter(user => user.id !== userForm.id));
      setUserForm(emptyUserForm);
      setDialog(null);
      toast.success('用户已删除');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除用户失败');
    } finally {
      setBusy('');
    }
  }

  async function saveRole() {
    if (roleForm.builtin) return toast.error('内置角色不可修改');
    if (!roleForm.key.trim() || !roleForm.name.trim()) return toast.error('角色标识和名称不能为空');
    setBusy('role');
    try {
      const payload = {
        key: roleForm.key.trim(),
        name: roleForm.name.trim(),
        description: roleForm.description.trim(),
        permissions: normalizeRolePermissions(roleForm.permissions, impliedReadPermissions),
      };
      const saved = roleForm.id
        ? await updateUserRole(roleForm.id, payload)
        : await createUserRole(payload);
      setRoles(current => upsertById(current, saved));
      setRoleForm(roleToForm(saved));
      setDialog(null);
      toast.success('用户角色已保存');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存用户角色失败');
    } finally {
      setBusy('');
    }
  }

  async function removeRole() {
    if (!roleForm.id || roleForm.builtin) return;
    setBusy('role');
    try {
      await deleteUserRole(roleForm.id);
      setRoles(current => current.filter(role => role.id !== roleForm.id));
      setRoleForm(emptyRoleForm);
      setDialog(null);
      toast.success('用户角色已删除');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除用户角色失败');
    } finally {
      setBusy('');
    }
  }

  async function saveGroup() {
    if (!groupForm.name.trim()) return toast.error('用户群组名称不能为空');
    setBusy('group');
    try {
      const payload = {
        name: groupForm.name.trim(),
        description: groupForm.description.trim(),
        disabled: groupForm.disabled,
        memberIds: groupForm.memberIds,
        roleKeys: groupForm.roleKeys,
      };
      const saved = groupForm.id
        ? await updateUserGroup(groupForm.id, payload)
        : await createUserGroup(payload);
      setGroups(current => upsertById(current, saved));
      setGroupForm(groupToForm(saved));
      setDialog(null);
      toast.success('用户群组已保存');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存用户群组失败');
    } finally {
      setBusy('');
    }
  }

  async function toggleGroupDisabled() {
    if (!groupForm.id) return;
    setBusy('group');
    try {
      const saved = await updateUserGroup(groupForm.id, {
        name: groupForm.name.trim(),
        description: groupForm.description.trim(),
        disabled: !groupForm.disabled,
        memberIds: groupForm.memberIds,
        roleKeys: groupForm.roleKeys,
      });
      setGroups(current => upsertById(current, saved));
      setGroupForm(groupToForm(saved));
      toast.success(saved.disabled ? '用户群组已禁用' : '用户群组已启用');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '更新用户群组状态失败');
    } finally {
      setBusy('');
    }
  }

  async function removeGroup() {
    if (!groupForm.id) return;
    setBusy('group');
    try {
      await deleteUserGroup(groupForm.id);
      setGroups(current => current.filter(group => group.id !== groupForm.id));
      setGroupForm(emptyGroupForm);
      setDialog(null);
      toast.success('用户群组已删除');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除用户群组失败');
    } finally {
      setBusy('');
    }
  }

  return (
    <>
      <div className="grid min-h-0 flex-1 grid-cols-1 items-stretch gap-4 overflow-hidden xl:grid-cols-[430px_minmax(0,1fr)]">
        <nav
          className="zl-hidden-scrollbar max-h-64 min-h-0 space-y-2 overflow-y-auto rounded-lg p-2 xl:max-h-none"
          style={{ background: 'rgba(255,255,255,0.035)', border: '1px solid var(--zl-border)' }}
          aria-label="用户配置"
        >
          {userConfigCards.map(card => (
            <UserConfigCard
              key={card.id}
              card={card}
              active={active === card.id}
              count={
                card.id === 'users'
                  ? users.length
                  : card.id === 'groups'
                    ? groups.length
                    : roles.length
              }
              onClick={() => setActive(card.id)}
            />
          ))}
        </nav>
        <aside
          className="flex min-h-0 flex-col overflow-hidden rounded-lg p-4"
          style={{ background: 'rgba(255,255,255,0.035)', border: '1px solid var(--zl-border)' }}
        >
          <PanelHeader card={selectedCard} />
          {error ? (
            <div
              className="mb-4 rounded-lg p-3 text-sm"
              style={{
                background: 'rgba(245,158,11,0.1)',
                border: '1px solid rgba(245,158,11,0.25)',
                color: '#f59e0b',
              }}
            >
              {error}
            </div>
          ) : null}
          <div className="min-h-0 flex-1 overflow-hidden">
            {active === 'users' ? (
              <UsersEditor
                canManage={canManage}
                users={users}
                roles={roles}
                form={userForm}
                onCreate={() => {
                  setUserForm(emptyUserForm);
                  setDialog('users');
                }}
                onSelect={next => {
                  setUserForm(next);
                  setDialog('users');
                }}
              />
            ) : null}
            {active === 'groups' ? (
              <GroupsEditor
                canManage={canManage}
                groups={groups}
                form={groupForm}
                onCreate={() => {
                  setGroupForm(emptyGroupForm);
                  setDialog('groups');
                }}
                onSelect={next => {
                  setGroupForm(next);
                  setDialog('groups');
                }}
              />
            ) : null}
            {active === 'roles' ? (
              <RolesEditor
                canManage={canManage}
                roles={roles}
                form={roleForm}
                onCreate={() => {
                  setRoleForm(emptyRoleForm);
                  setDialog('roles');
                }}
                onSelect={next => {
                  setRoleForm(next);
                  setDialog('roles');
                }}
              />
            ) : null}
          </div>
        </aside>
      </div>
      {dialog === 'users' ? (
        <UserEditorDialog
          canManage={canManage}
          roles={roles}
          form={userForm}
          busy={busy}
          onClose={() => setDialog(null)}
          onFormChange={setUserForm}
          onSave={saveUser}
          onDelete={removeUser}
          onToggleDisabled={toggleUserDisabled}
        />
      ) : null}
      {dialog === 'groups' ? (
        <GroupEditorDialog
          canManage={canManage}
          users={users}
          roles={roles}
          form={groupForm}
          busy={busy}
          onClose={() => setDialog(null)}
          onFormChange={setGroupForm}
          onSave={saveGroup}
          onDelete={removeGroup}
          onToggleDisabled={toggleGroupDisabled}
        />
      ) : null}
      {dialog === 'roles' ? (
        <RoleEditorDialog
          canManage={canManage}
          permissionGroups={permissionGroups}
          form={roleForm}
          busy={busy}
          onClose={() => setDialog(null)}
          onFormChange={setRoleForm}
          onSave={saveRole}
          onDelete={removeRole}
        />
      ) : null}
    </>
  );
}

function PanelHeader({ card }: { card: (typeof userConfigCards)[number] }) {
  const Icon = card.icon;
  return (
    <div className="mb-4 flex items-center gap-3">
      <div
        className="flex h-10 w-10 items-center justify-center rounded-lg"
        style={{
          color: card.color,
          background: 'rgba(255,255,255,0.05)',
          border: '1px solid rgba(255,255,255,0.08)',
        }}
      >
        <Icon size={19} />
      </div>
      <div className="min-w-0">
        <div className="truncate text-sm font-semibold" style={{ color: 'var(--zl-text)' }}>
          {card.title}
        </div>
        <div className="mt-0.5 truncate text-xs" style={{ color: 'var(--zl-text-muted)' }}>
          {card.description}
        </div>
      </div>
    </div>
  );
}

function UserConfigCard({
  card,
  active,
  count,
  onClick,
}: {
  card: (typeof userConfigCards)[number];
  active: boolean;
  count: number;
  onClick: () => void;
}) {
  const Icon = card.icon;
  return (
    <button
      type="button"
      onClick={onClick}
      className="zl-config-side-card flex w-full items-start gap-3 rounded-lg p-3 text-left transition-all duration-200"
      data-active={active}
      style={{
        background: active ? 'rgba(59,130,246,0.12)' : 'transparent',
        border: active ? '1px solid rgba(96,165,250,0.56)' : '1px solid transparent',
        color: 'var(--zl-text)',
      }}
    >
      <div
        className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
        style={{
          color: card.color,
          background: 'rgba(255,255,255,0.05)',
          border: '1px solid rgba(255,255,255,0.08)',
        }}
      >
        <Icon size={19} />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-3">
          <span className="truncate text-sm font-semibold">{card.title}</span>
          <span className="text-xs" style={{ color: 'var(--zl-text-muted)' }}>
            {count}
          </span>
        </div>
        <p
          className="mt-1 line-clamp-2 text-xs leading-5"
          style={{ color: 'var(--zl-text-muted)' }}
        >
          {card.description}
        </p>
      </div>
    </button>
  );
}

function UsersEditor({
  users,
  roles,
  form,
  canManage,
  onCreate,
  onSelect,
}: {
  users: SettingsUser[];
  roles: UserRole[];
  form: UserForm;
  canManage: boolean;
  onCreate: () => void;
  onSelect: (form: UserForm) => void;
}) {
  const [query, setQuery] = useState('');
  const filtered = useMemo(
    () => users.filter(user => userSearchText(user).includes(query.trim().toLowerCase())),
    [query, users]
  );
  return (
    <ManagementShell
      title="用户"
      count={users.length}
      query={query}
      queryPlaceholder="搜索用户名、显示名或角色"
      onQueryChange={setQuery}
      onCreate={onCreate}
      createLabel="新增用户"
      canCreate={canManage}
    >
      <ListPane
        emptyText="暂无用户"
        filteredEmptyText="没有匹配的用户"
        total={users.length}
        filtered={filtered.length}
      >
        {filtered.map(user => (
          <CompactRow
            key={user.id}
            active={form.id === user.id}
            title={user.displayName || user.username}
            meta={`${user.username} · ${directRoleName(user.role, roles)}`}
            extra={`创建时间：${formatDateTime(user.created_at)} · 最近登录：${formatLastLogin(user.lastLoginAt)}`}
            badge={user.disabled ? '禁用' : '启用'}
            enabled={!user.disabled}
            onClick={() => onSelect(userToForm(user))}
          />
        ))}
      </ListPane>
    </ManagementShell>
  );
}

function GroupsEditor({
  groups,
  form,
  canManage,
  onCreate,
  onSelect,
}: {
  groups: UserGroup[];
  form: GroupForm;
  canManage: boolean;
  onCreate: () => void;
  onSelect: (form: GroupForm) => void;
}) {
  const [query, setQuery] = useState('');
  const filtered = useMemo(
    () => groups.filter(group => groupSearchText(group).includes(query.trim().toLowerCase())),
    [groups, query]
  );
  return (
    <ManagementShell
      title="用户群组"
      count={groups.length}
      query={query}
      queryPlaceholder="搜索群组、描述、成员或角色"
      onQueryChange={setQuery}
      onCreate={onCreate}
      createLabel="新增群组"
      canCreate={canManage}
    >
      <ListPane
        emptyText="暂无用户群组"
        filteredEmptyText="没有匹配的群组"
        total={groups.length}
        filtered={filtered.length}
      >
        {filtered.map(group => (
          <CompactRow
            key={group.id}
            active={form.id === group.id}
            title={group.name}
            meta={`${group.members?.length ?? 0} 个用户 · ${roleNames(group.roles)}`}
            extra={group.description || '未填写描述'}
            badge={`${group.roles?.length ?? 0} 角色`}
            enabled={!group.disabled}
            onClick={() => onSelect(groupToForm(group))}
          />
        ))}
      </ListPane>
    </ManagementShell>
  );
}

function RolesEditor({
  roles,
  form,
  canManage,
  onCreate,
  onSelect,
}: {
  roles: UserRole[];
  form: RoleForm;
  canManage: boolean;
  onCreate: () => void;
  onSelect: (form: RoleForm) => void;
}) {
  const [query, setQuery] = useState('');
  const filteredRoles = useMemo(
    () => roles.filter(role => roleSearchText(role).includes(query.trim().toLowerCase())),
    [query, roles]
  );
  return (
    <ManagementShell
      title="用户角色"
      count={roles.length}
      query={query}
      queryPlaceholder="搜索角色名称、标识或描述"
      onQueryChange={setQuery}
      onCreate={onCreate}
      createLabel="新增角色"
      canCreate={canManage}
    >
      <ListPane
        emptyText="暂无用户角色"
        filteredEmptyText="没有匹配的角色"
        total={roles.length}
        filtered={filteredRoles.length}
      >
        {filteredRoles.map(role => (
          <CompactRow
            key={role.id}
            active={form.id === role.id}
            title={role.name}
            meta={`${role.key} · ${role.permissions.length} 项权限`}
            extra={role.description || '未填写描述'}
            badge={role.builtin ? '内置' : '自定义'}
            onClick={() => onSelect(roleToForm(role))}
          />
        ))}
      </ListPane>
    </ManagementShell>
  );
}

function ManagementShell({
  title,
  count,
  query,
  queryPlaceholder,
  createLabel,
  canCreate,
  onQueryChange,
  onCreate,
  children,
}: {
  title: string;
  count: number;
  query: string;
  queryPlaceholder: string;
  createLabel: string;
  canCreate: boolean;
  onQueryChange: (value: string) => void;
  onCreate: () => void;
  children: React.ReactNode;
}) {
  return (
    <div
      className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border"
      style={{ borderColor: 'var(--zl-border)', background: 'rgba(255,255,255,0.018)' }}
    >
      <div
        className="flex flex-wrap items-center justify-between gap-3 border-b p-3"
        style={{ borderColor: 'var(--zl-border)' }}
      >
        <div className="flex items-baseline gap-2">
          <span className="text-sm font-semibold" style={{ color: 'var(--zl-text)' }}>
            {title}
          </span>
          <span className="text-xs" style={{ color: 'var(--zl-text-muted)' }}>
            {count} 项
          </span>
        </div>
        <div className="flex min-w-0 flex-1 items-center justify-end gap-2">
          <SearchBox value={query} placeholder={queryPlaceholder} onChange={onQueryChange} />
          {canCreate ? (
            <button
              type="button"
              onClick={onCreate}
              className="zl-action-button flex h-9 shrink-0 items-center gap-2 rounded-lg border px-3 text-sm"
              style={{
                borderColor: 'rgba(59,130,246,0.38)',
                color: 'var(--zl-accent-text)',
                background: 'rgba(59,130,246,0.1)',
              }}
            >
              <Plus size={14} />
              {createLabel}
            </button>
          ) : null}
        </div>
      </div>
      {children}
    </div>
  );
}

function SearchBox({
  value,
  placeholder,
  onChange,
}: {
  value: string;
  placeholder: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="relative min-w-[220px] max-w-sm flex-1">
      <Search
        size={14}
        className="absolute left-3 top-1/2 -translate-y-1/2"
        style={{ color: 'var(--zl-text-muted)' }}
      />
      <input
        value={value}
        onChange={event => onChange(event.target.value)}
        placeholder={placeholder}
        className="h-9 w-full rounded-lg pl-9 pr-3 text-sm outline-none"
        style={{
          background: 'var(--zl-control-bg)',
          border: '1px solid var(--zl-border)',
          color: 'var(--zl-text)',
        }}
      />
    </div>
  );
}

function ListPane({
  children,
  emptyText,
  filteredEmptyText,
  total,
  filtered,
}: {
  children: React.ReactNode;
  emptyText: string;
  filteredEmptyText: string;
  total: number;
  filtered: number;
}) {
  return (
    <div className="zl-hidden-scrollbar min-h-0 flex-1 overflow-y-auto p-3">
      {total === 0 ? (
        <EmptyState text={emptyText} />
      ) : filtered === 0 ? (
        <EmptyState text={filteredEmptyText} />
      ) : (
        <div className="space-y-2">{children}</div>
      )}
    </div>
  );
}

function CompactRow({
  title,
  meta,
  extra,
  badge,
  enabled,
  active,
  onClick,
}: {
  title: string;
  meta: string;
  extra: string;
  badge: string;
  enabled?: boolean;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="zl-action-button group grid min-h-[74px] w-full grid-cols-[3px_minmax(0,1fr)_auto] items-center gap-3 rounded-lg border p-0 pr-3 text-left"
      style={{
        background: active ? 'rgba(59,130,246,0.11)' : 'rgba(255,255,255,0.026)',
        borderColor: active ? 'rgba(96,165,250,0.52)' : 'var(--zl-border)',
        color: 'var(--zl-text)',
      }}
    >
      <span
        className="h-full rounded-l-lg"
        style={{ background: active ? '#60a5fa' : 'transparent' }}
      />
      <span className="min-w-0 py-2">
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate text-sm font-semibold">{title}</span>
          <span
            className="shrink-0 rounded-md border px-1.5 py-0.5 text-[11px]"
            style={{
              color: 'var(--zl-text-muted)',
              borderColor: 'var(--zl-border)',
              background: 'rgba(255,255,255,0.028)',
            }}
          >
            {badge}
          </span>
        </span>
        <span className="mt-1 block truncate text-xs" style={{ color: 'var(--zl-text-muted)' }}>
          {meta}
        </span>
        <span className="mt-0.5 block truncate text-xs" style={{ color: 'var(--zl-text-muted)' }}>
          {extra}
        </span>
      </span>
      {typeof enabled === 'boolean' ? (
        enabled ? (
          <ToggleRight size={18} style={{ color: '#86efac' }} />
        ) : (
          <ToggleLeft size={18} style={{ color: 'var(--zl-text-muted)' }} />
        )
      ) : (
        <span
          className="h-2 w-2 rounded-full"
          style={{ background: active ? '#60a5fa' : 'var(--zl-border)' }}
        />
      )}
    </button>
  );
}

function EmptyState({ text }: { text: string }) {
  return (
    <div
      className="flex min-h-48 items-center justify-center rounded-lg border text-sm"
      style={{
        borderColor: 'var(--zl-border)',
        color: 'var(--zl-text-muted)',
        background: 'rgba(255,255,255,0.02)',
      }}
    >
      {text}
    </div>
  );
}

function directRoleName(roleKey: string | undefined, roles: UserRole[]) {
  if (!roleKey) return '未分配角色';
  const role = roles.find(item => item.key === roleKey);
  return role?.name || roleKey;
}
