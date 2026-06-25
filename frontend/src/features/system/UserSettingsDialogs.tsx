import { createPortal } from 'react-dom';
import { Ban, Check, CircleCheck, Eye, EyeOff, Save, Trash2, X } from 'lucide-react';
import { useMemo, useState } from 'react';
import { AppTooltip } from '@/components/app-tooltip';
import type { SettingsUser, UserPermission, UserRole } from '@/lib/system-settings';
import {
  filterPermissionGroups,
  formToUser,
  isDefaultAdminUser,
  type GroupForm,
  type RoleForm,
  type UserForm,
} from './user-settings-utils';

export function UserEditorDialog({
  roles,
  form,
  busy,
  canManage,
  onClose,
  onFormChange,
  onSave,
  onDelete,
  onToggleDisabled,
}: {
  roles: UserRole[];
  form: UserForm;
  busy: string;
  canManage: boolean;
  onClose: () => void;
  onFormChange: React.Dispatch<React.SetStateAction<UserForm>>;
  onSave: () => void;
  onDelete: () => void;
  onToggleDisabled: (user: SettingsUser) => void;
}) {
  const defaultAdmin = isDefaultAdminUser(form);
  return (
    <DialogFrame
      title={form.id ? '编辑用户' : '新增用户'}
      subtitle={form.username || '配置平台账号和登录权限'}
      onClose={onClose}
      footer={
        canManage ? (
          <>
            {form.id ? (
              <DangerButton
                busy={busy === 'user'}
                disabled={!form.disabled || defaultAdmin}
                onClick={onDelete}
                label="删除"
              />
            ) : null}
            <StateButton
              disabled={!form.id || busy !== '' || defaultAdmin}
              active={form.disabled}
              onClick={() =>
                form.id && onToggleDisabled({ ...formToUser(form), disabled: form.disabled })
              }
            />
            <PrimaryButton busy={busy === 'user'} onClick={onSave} label="保存" />
          </>
        ) : null
      }
    >
      <SectionTitle title="基础信息" />
      <Input
        label="用户名"
        required
        value={form.username}
        disabled={!canManage || defaultAdmin}
        onChange={value => onFormChange(current => ({ ...current, username: value }))}
        placeholder="用户名"
      />
      <Input
        label="邮箱"
        required
        type="email"
        value={form.email}
        disabled={!canManage}
        onChange={value => onFormChange(current => ({ ...current, email: value }))}
        placeholder="用于验证找回密码身份"
      />
      <Input
        label="显示名称"
        value={form.displayName}
        disabled={!canManage}
        onChange={value => onFormChange(current => ({ ...current, displayName: value }))}
        placeholder="用户显示名称"
      />
      <Input
        label={form.id ? '新密码' : '密码'}
        required={!form.id}
        type="password"
        value={form.password}
        disabled={!canManage}
        onChange={value => onFormChange(current => ({ ...current, password: value }))}
        placeholder={form.id ? '留空则不修改' : '至少 6 个字符'}
      />
      <SectionTitle title="授权与状态" />
      <CheckList
        compact
        single
        maxVisibleItems={3}
        hideActions
        title="角色"
        items={roles.map(role => ({
          key: role.key,
          label: role.name || role.key,
          helper: role.description,
        }))}
        value={form.roleKeys}
        disabled={!canManage}
        onChange={value => onFormChange(current => ({ ...current, roleKeys: value }))}
      />
    </DialogFrame>
  );
}

export function GroupEditorDialog({
  users,
  roles,
  form,
  busy,
  canManage,
  onClose,
  onFormChange,
  onSave,
  onDelete,
  onToggleDisabled,
}: {
  users: SettingsUser[];
  roles: UserRole[];
  form: GroupForm;
  busy: string;
  canManage: boolean;
  onClose: () => void;
  onFormChange: React.Dispatch<React.SetStateAction<GroupForm>>;
  onSave: () => void;
  onDelete: () => void;
  onToggleDisabled: () => void;
}) {
  return (
    <DialogFrame
      title={form.id ? '编辑群组' : '新增群组'}
      subtitle={form.name || '批量组织成员和角色'}
      onClose={onClose}
      footer={
        canManage ? (
          <>
            {form.id ? (
              <DangerButton busy={busy === 'group'} onClick={onDelete} label="删除" />
            ) : null}
            <StateButton
              disabled={!form.id || busy !== ''}
              active={form.disabled}
              onClick={onToggleDisabled}
            />
            <PrimaryButton busy={busy === 'group'} onClick={onSave} label="保存" />
          </>
        ) : null
      }
    >
      <SectionTitle title="基础信息" />
      <Input
        label="群组名称"
        required
        value={form.name}
        disabled={!canManage}
        onChange={value => onFormChange(current => ({ ...current, name: value }))}
        placeholder="运维组"
      />
      <TextArea
        label="描述"
        value={form.description}
        disabled={!canManage}
        onChange={value => onFormChange(current => ({ ...current, description: value }))}
        placeholder="群组用途"
      />
      <SectionTitle title="成员与角色" />
      <CheckList
        compact
        maxVisibleItems={3}
        title="群组成员"
        items={users.map(user => ({
          key: user.id,
          label: user.displayName || user.username,
          helper: user.username,
        }))}
        value={form.memberIds}
        disabled={!canManage}
        onChange={value => onFormChange(current => ({ ...current, memberIds: value }))}
      />
      <CheckList
        compact
        single
        maxVisibleItems={3}
        hideActions
        title="群组角色"
        items={roles.map(role => ({
          key: role.key,
          label: role.name || role.key,
          helper: role.description,
        }))}
        value={form.roleKeys}
        disabled={!canManage}
        onChange={value => onFormChange(current => ({ ...current, roleKeys: value }))}
      />
    </DialogFrame>
  );
}

export function RoleEditorDialog({
  permissionGroups,
  form,
  busy,
  canManage,
  onClose,
  onFormChange,
  onSave,
  onDelete,
}: {
  permissionGroups: Record<string, UserPermission[]>;
  form: RoleForm;
  busy: string;
  canManage: boolean;
  onClose: () => void;
  onFormChange: React.Dispatch<React.SetStateAction<RoleForm>>;
  onSave: () => void;
  onDelete: () => void;
}) {
  const [permissionQuery, setPermissionQuery] = useState('');
  const filteredGroups = useMemo(
    () => filterPermissionGroups(permissionGroups, permissionQuery),
    [permissionGroups, permissionQuery]
  );
  const readonly = form.builtin || !canManage;
  return (
    <DialogFrame
      title={form.id ? '编辑角色' : '新增角色'}
      subtitle={form.builtin ? '内置角色只读' : form.key || '配置角色可用权限'}
      onClose={onClose}
      footer={
        canManage ? (
          <>
            {form.id && !form.builtin ? (
              <DangerButton busy={busy === 'role'} onClick={onDelete} label="删除" />
            ) : null}
            <PrimaryButton
              busy={busy === 'role'}
              disabled={form.builtin}
              onClick={onSave}
              label="保存角色"
            />
          </>
        ) : null
      }
    >
      <SectionTitle title="角色信息" />
      <Input
        label="角色标识"
        required
        value={form.key}
        disabled={readonly}
        onChange={value => onFormChange(current => ({ ...current, key: value }))}
        placeholder="custom-ops"
      />
      <Input
        label="角色名称"
        required
        value={form.name}
        disabled={readonly}
        onChange={value => onFormChange(current => ({ ...current, name: value }))}
        placeholder="自定义运维"
      />
      <TextArea
        label="描述"
        value={form.description}
        disabled={readonly}
        onChange={value => onFormChange(current => ({ ...current, description: value }))}
        placeholder="说明该角色可执行的操作"
      />
      <SectionTitle title="权限集合" />
      <Input
        label="搜索权限"
        value={permissionQuery}
        onChange={setPermissionQuery}
        placeholder="按名称、标识或描述搜索"
        disabled={form.builtin}
      />
      {Object.entries(filteredGroups).map(([category, items]) => (
        <CheckList
          compact
          key={category}
          title={category}
          items={items.map(permission => ({
            key: permission.key,
            label: permission.name,
            helper: `${permission.key} · ${permission.description}`,
          }))}
          value={form.permissions}
          disabled={readonly}
          onChange={value => onFormChange(current => ({ ...current, permissions: value }))}
        />
      ))}
      {Object.keys(filteredGroups).length === 0 ? (
        <div
          className="rounded-lg border p-3 text-sm"
          style={{
            borderColor: 'var(--zl-border)',
            color: 'var(--zl-text-muted)',
            background: 'rgba(255,255,255,0.03)',
          }}
        >
          没有匹配的权限
        </div>
      ) : null}
    </DialogFrame>
  );
}

function DialogFrame({
  title,
  subtitle,
  footer,
  onClose,
  children,
}: {
  title: string;
  subtitle: string;
  footer: React.ReactNode;
  onClose: () => void;
  children: React.ReactNode;
}) {
  const node = (
    <div
      className="zl-dialog-backdrop fixed inset-0 z-[1500] flex items-center justify-center px-3 py-5"
      role="presentation"
    >
      <div
        className="zl-dialog-panel flex max-h-[88vh] w-[min(92vw,720px)] flex-col overflow-hidden rounded-2xl shadow-2xl"
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div
          className="flex items-start justify-between gap-4 border-b p-5"
          style={{ borderColor: 'var(--zl-border)' }}
        >
          <div className="min-w-0">
            <h3 className="truncate text-base font-semibold" style={{ color: 'var(--zl-text)' }}>
              {title}
            </h3>
            <p className="mt-1 truncate text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              {subtitle}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="zl-action-button flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border"
            style={{
              borderColor: 'var(--zl-border)',
              color: 'var(--zl-text-muted)',
              background: 'rgba(255,255,255,0.04)',
            }}
            aria-label="关闭"
          >
            <X size={15} />
          </button>
        </div>
        <div className="zl-hidden-scrollbar min-h-0 flex-1 space-y-3 overflow-y-auto p-5">
          {children}
        </div>
        {footer ? (
          <div
            className="flex justify-end gap-2 border-t p-4"
            style={{ borderColor: 'var(--zl-border)', background: 'rgba(255,255,255,0.018)' }}
          >
            {footer}
          </div>
        ) : null}
      </div>
    </div>
  );
  return typeof document === 'undefined' ? node : createPortal(node, document.body);
}

function SectionTitle({ title }: { title: string }) {
  return (
    <div className="pt-1 text-sm font-semibold" style={{ color: 'var(--zl-text)' }}>
      {title}
    </div>
  );
}

function Input({
  label,
  value,
  onChange,
  placeholder,
  type = 'text',
  required = false,
  disabled = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  type?: string;
  required?: boolean;
  disabled?: boolean;
}) {
  const [passwordVisible, setPasswordVisible] = useState(false);
  const isPassword = type === 'password';
  const inputType = isPassword && passwordVisible ? 'text' : type;
  return (
    <label
      className="grid grid-cols-1 gap-1.5 text-xs sm:grid-cols-[64px_minmax(0,1fr)] sm:items-center sm:gap-3"
      style={{ color: 'var(--zl-text-muted)' }}
    >
      <span className="sm:text-right">
        {label}
        {required ? <span style={{ color: '#f87171' }}>*</span> : null}
      </span>
      <span className="relative block min-w-0">
        <input
          type={inputType}
          value={value}
          disabled={disabled}
          onChange={event => onChange(event.target.value)}
          placeholder={placeholder}
          className={`w-full rounded-lg px-3 py-2 text-sm outline-none disabled:opacity-60 ${isPassword ? 'pr-10' : ''}`}
          style={{
            background: 'var(--zl-control-bg)',
            border: '1px solid var(--zl-border)',
            color: 'var(--zl-text)',
          }}
        />
        {isPassword ? (
          <AppTooltip label={passwordVisible ? '隐藏密码' : '显示密码'} placement="top">
            <button
              type="button"
              disabled={disabled}
              onClick={event => {
                event.preventDefault();
                setPasswordVisible(current => !current);
              }}
              className="zl-action-button absolute right-2 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-md disabled:cursor-not-allowed disabled:opacity-50"
              style={{ color: 'var(--zl-text-muted)', background: 'rgba(255,255,255,0.035)' }}
              aria-label={passwordVisible ? '隐藏密码' : '显示密码'}
            >
              {passwordVisible ? <EyeOff size={15} /> : <Eye size={15} />}
            </button>
          </AppTooltip>
        ) : null}
      </span>
    </label>
  );
}

function TextArea({
  label,
  value,
  onChange,
  placeholder,
  disabled = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  disabled?: boolean;
}) {
  return (
    <label
      className="grid grid-cols-1 gap-1.5 text-xs sm:grid-cols-[64px_minmax(0,1fr)] sm:items-start sm:gap-3"
      style={{ color: 'var(--zl-text-muted)' }}
    >
      <span className="sm:pt-2 sm:text-right">{label}</span>
      <textarea
        value={value}
        disabled={disabled}
        onChange={event => onChange(event.target.value)}
        placeholder={placeholder}
        rows={3}
        className="w-full resize-y rounded-lg px-3 py-2 text-sm outline-none disabled:opacity-60"
        style={{
          background: 'var(--zl-control-bg)',
          border: '1px solid var(--zl-border)',
          color: 'var(--zl-text)',
        }}
      />
    </label>
  );
}

function CheckList({
  title,
  items,
  value,
  onChange,
  disabled = false,
  compact = false,
  single = false,
  hideActions = false,
  maxVisibleItems,
}: {
  title: string;
  items: Array<{ key: string; label: string; helper?: string }>;
  value: string[];
  onChange: (value: string[]) => void;
  disabled?: boolean;
  compact?: boolean;
  single?: boolean;
  hideActions?: boolean;
  maxVisibleItems?: number;
}) {
  const selected = new Set(value);
  const itemKeys = items.map(item => item.key);
  const allSelected = itemKeys.length > 0 && itemKeys.every(key => selected.has(key));
  const listMaxHeight = maxVisibleItems
    ? maxVisibleItems * 74 + Math.max(0, maxVisibleItems - 1) * 6 + 4
    : undefined;
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div className="text-sm font-semibold" style={{ color: 'var(--zl-text)' }}>
          {title}
        </div>
        {!hideActions ? (
          <div className="flex gap-1">
            <button
              type="button"
              disabled={disabled || itemKeys.length === 0 || allSelected}
              onClick={() => onChange(Array.from(new Set([...value, ...itemKeys])))}
              className="zl-action-button cursor-pointer rounded-md border px-2.5 py-1 text-[11px] transition disabled:cursor-not-allowed disabled:opacity-50"
              style={{
                borderColor: 'rgba(59,130,246,0.32)',
                color: 'var(--zl-accent-text)',
                background: 'rgba(59,130,246,0.08)',
              }}
            >
              全选
            </button>
            <button
              type="button"
              disabled={
                disabled || itemKeys.length === 0 || !itemKeys.some(key => selected.has(key))
              }
              onClick={() => onChange(value.filter(key => !itemKeys.includes(key)))}
              className="zl-action-button cursor-pointer rounded-md border px-2.5 py-1 text-[11px] transition disabled:cursor-not-allowed disabled:opacity-50"
              style={{
                borderColor: 'var(--zl-border)',
                color: 'var(--zl-text-muted)',
                background: 'rgba(255,255,255,0.04)',
              }}
            >
              清空
            </button>
          </div>
        ) : null}
      </div>
      <div
        className={`${maxVisibleItems ? '' : compact ? 'max-h-48' : 'max-h-52'} zl-hidden-scrollbar grid gap-1.5 overflow-y-auto pb-1 pr-1`}
        style={{ maxHeight: listMaxHeight }}
      >
        {items.map(item => {
          const checked = selected.has(item.key);
          const outlineColor = checked ? 'rgba(96,165,250,0.45)' : 'var(--zl-border)';
          return (
            <label
              key={item.key}
              className={`flex items-start gap-2 rounded-lg border border-transparent ${compact ? 'p-2' : 'p-2'} text-sm ${disabled ? 'cursor-not-allowed opacity-70' : 'cursor-pointer'}`}
              style={{
                background: checked ? 'rgba(59,130,246,0.1)' : 'rgba(255,255,255,0.026)',
                boxShadow: `inset 0 0 0 1px ${outlineColor}`,
                color: 'var(--zl-text)',
              }}
            >
              <input
                type={single ? 'radio' : 'checkbox'}
                disabled={disabled}
                checked={checked}
                onChange={event =>
                  onChange(
                    single
                      ? event.target.checked
                        ? [item.key]
                        : value
                      : event.target.checked
                        ? [...value.filter(key => key !== item.key), item.key]
                        : value.filter(key => key !== item.key)
                  )
                }
                className="mt-1"
              />
              <span className="min-w-0">
                <span className="block truncate">{item.label}</span>
                {item.helper ? (
                  <span
                    className="mt-0.5 block text-xs leading-4"
                    style={{ color: 'var(--zl-text-muted)' }}
                  >
                    {item.helper}
                  </span>
                ) : null}
              </span>
              {checked ? (
                <Check className="ml-auto shrink-0" size={15} style={{ color: '#93c5fd' }} />
              ) : null}
            </label>
          );
        })}
      </div>
    </div>
  );
}

function PrimaryButton({
  label,
  busy,
  onClick,
  disabled = false,
}: {
  label: string;
  busy: boolean;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={() => void onClick()}
      disabled={busy || disabled}
      className="zl-action-button flex items-center gap-2 rounded-lg border px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
      style={{
        borderColor: 'rgba(59,130,246,0.38)',
        color: 'var(--zl-accent-text)',
        background: 'rgba(59,130,246,0.1)',
      }}
    >
      <Save size={14} />
      {busy ? '保存中' : label}
    </button>
  );
}

function StateButton({
  active,
  disabled,
  onClick,
}: {
  active: boolean;
  disabled: boolean;
  onClick: () => void;
}) {
  const label = active ? '启用' : '禁用';
  const Icon = active ? CircleCheck : Ban;
  return (
    <button
      type="button"
      onClick={() => void onClick()}
      disabled={disabled}
      className="zl-action-button flex items-center gap-2 rounded-lg border px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
      style={{
        borderColor: active ? 'rgba(34,197,94,0.42)' : 'rgba(245,158,11,0.36)',
        color: active ? '#86efac' : '#fbbf24',
        background: active ? 'rgba(34,197,94,0.1)' : 'rgba(245,158,11,0.09)',
      }}
    >
      <Icon size={14} />
      {label}
    </button>
  );
}

function DangerButton({
  label,
  busy,
  onClick,
  disabled = false,
}: {
  label: string;
  busy: boolean;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={() => void onClick()}
      disabled={busy || disabled}
      className="zl-action-button flex items-center gap-2 rounded-lg border px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
      style={{
        borderColor: 'rgba(239,68,68,0.34)',
        color: '#f87171',
        background: 'rgba(239,68,68,0.08)',
      }}
    >
      <Trash2 size={14} />
      {label}
    </button>
  );
}
