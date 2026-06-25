import { BellRing, Network, SlidersHorizontal, UsersRound, type LucideIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { getStoredUser, userHasPermission } from '@/lib/auth';
import { AuthSettingsPanel } from './AuthSettingsPanel';
import { BaseSettingsPanel } from './BaseSettingsPanel';
import { EmailSettingsPanel } from './EmailSettingsPanel';
import { UserSettingsPanel } from './UserSettingsPanel';

type SettingsTab = 'base' | 'users' | 'auth' | 'notifications';

const tabs: Array<{
  id: SettingsTab;
  icon: LucideIcon;
  label: string;
  read: string;
  manage: string;
}> = [
  {
    id: 'base',
    icon: SlidersHorizontal,
    label: '基础配置',
    read: 'settings.base.read',
    manage: 'settings.base.manage',
  },
  {
    id: 'users',
    icon: UsersRound,
    label: '用户配置',
    read: 'settings.users.read',
    manage: 'settings.users.manage',
  },
  {
    id: 'auth',
    icon: Network,
    label: '认证配置',
    read: 'settings.auth.read',
    manage: 'settings.auth.manage',
  },
  {
    id: 'notifications',
    icon: BellRing,
    label: '通知配置',
    read: 'settings.notifications.read',
    manage: 'settings.notifications.manage',
  },
];

export function SystemSettingsPage() {
  const [tab, setTab] = useState<SettingsTab>('base');
  const user = getStoredUser();
  const visibleTabs = useMemo(
    () => tabs.filter(item => userHasPermission(user, item.read)),
    [user]
  );
  const active = useMemo(
    () => visibleTabs.find(item => item.id === tab) ?? visibleTabs[0],
    [tab, visibleTabs]
  );
  const canManage = active ? userHasPermission(user, active.manage) : false;
  const activeTab = active?.id ?? 'base';
  const activeLabel = active?.label ?? '系统配置';
  const ActiveIcon = active?.icon ?? SlidersHorizontal;

  if (!active) {
    return (
      <div
        className="flex h-full items-center justify-center rounded-xl border text-sm"
        style={{
          borderColor: 'var(--zl-border)',
          color: 'var(--zl-text-muted)',
          background: 'var(--zl-card)',
        }}
      >
        当前用户无权查看系统配置
      </div>
    );
  }

  return (
    <div data-cmp="SystemSettingsPage" className="flex h-full min-h-0 flex-col gap-5">
      <section
        className="zl-card-hover flex shrink-0 flex-wrap gap-2 rounded-xl p-3"
        style={{
          background: 'var(--zl-card)',
          border: '1px solid var(--zl-border)',
          boxShadow: 'var(--shadow-card)',
        }}
      >
        {visibleTabs.map(item => (
          <SettingsTabButton
            key={item.id}
            active={activeTab === item.id}
            icon={item.icon}
            label={item.label}
            onClick={() => setTab(item.id)}
          />
        ))}
      </section>

      <section
        className="zl-card-hover flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl p-5"
        style={{
          background: 'var(--zl-card)',
          border: '1px solid var(--zl-border)',
          boxShadow: 'var(--shadow-card)',
        }}
      >
        <div className="mb-5 flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h2 className="text-sm font-semibold" style={{ color: 'var(--zl-text)' }}>
              {activeLabel}
            </h2>
            <p className="mt-1 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              {sectionDescription(activeTab)}
            </p>
          </div>
          <div
            className="flex items-center gap-2 rounded-lg px-3 py-2 text-xs"
            style={{
              color: 'var(--zl-accent-text)',
              background: 'rgba(59,130,246,0.1)',
              border: '1px solid rgba(59,130,246,0.24)',
            }}
          >
            <ActiveIcon size={14} />
            {sectionBadge(activeTab)}
          </div>
        </div>
        {activeTab === 'base' ? <BaseSettingsPanel canManage={canManage} /> : null}
        {activeTab === 'users' ? <UserSettingsPanel canManage={canManage} /> : null}
        {activeTab === 'auth' ? <AuthSettingsPanel canManage={canManage} /> : null}
        {activeTab === 'notifications' ? <EmailSettingsPanel canManage={canManage} /> : null}
      </section>
    </div>
  );
}

function SettingsTabButton({
  active,
  icon: Icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: LucideIcon;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="zl-config-nav-card flex h-10 items-center gap-2 rounded-lg px-4 text-sm font-medium transition-all duration-200"
      data-active={active}
      style={{
        color: active ? 'var(--zl-accent-hover)' : 'var(--zl-text-muted)',
      }}
    >
      <Icon size={16} />
      {label}
    </button>
  );
}

function sectionDescription(tab: SettingsTab) {
  if (tab === 'base') return '维护站点名称、登录展示、安全时效和同步参数规划。';
  if (tab === 'users')
    return '维护平台用户、用户群组和角色权限。启用 AD/LDAP 后，外部账号也必须先在用户中创建并启用。';
  if (tab === 'auth') return '启用外部认证后，登录界面会显示对应登录方式。';
  return '用于找回密码验证码发送。';
}

function sectionBadge(tab: SettingsTab) {
  if (tab === 'base') return '系统基础';
  if (tab === 'users') return '用户权限';
  if (tab === 'auth') return '身份认证';
  return '找回密码邮件';
}
