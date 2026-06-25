import { CheckCircle2, Network, Save, ToggleLeft, ToggleRight, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import {
  fetchAuthProviders,
  testAuthProvider,
  updateAuthProvider,
  type AuthProvider,
} from '@/lib/system-settings';
import {
  ActionButton,
  ConfigField,
  EnableToggle,
  SettingsDetailHeader,
  SettingsDetailPanel,
  SettingsSplitLayout,
  cardStyle,
  type SettingsField,
} from './settings-primitives';

type AuthProviderId = 'ldap';

const authProviderMeta: Record<
  AuthProviderId,
  {
    name: string;
    description: string;
    icon: React.ElementType;
    color: string;
    requiredFields: SettingsField[];
    optionalFields: SettingsField[];
  }
> = {
  ldap: {
    name: 'AD/LDAP',
    description: '通过企业目录服务实现统一身份认证，支持 AD/LDAP 登录',
    icon: Network,
    color: '#38bdf8',
    requiredFields: [
      { key: 'host', label: '服务器地址', placeholder: 'ldap.example.com', required: true },
      { key: 'port', label: '端口', placeholder: '389', required: true, inputMode: 'numeric' },
      { key: 'baseDN', label: 'Base DN', placeholder: 'dc=example,dc=com', required: true },
      {
        key: 'userFilter',
        label: '用户过滤器',
        placeholder: '(sAMAccountName={username})',
        required: true,
      },
      {
        key: 'bindDN',
        label: '绑定 DN',
        placeholder: 'cn=readonly,dc=example,dc=com',
        required: true,
      },
      {
        key: 'bindPassword',
        label: '绑定密码',
        placeholder: '请输入绑定账号密码',
        required: true,
        type: 'password',
      },
    ],
    optionalFields: [
      { key: 'useTLS', label: '启用 LDAPS', type: 'checkbox' },
      { key: 'startTLS', label: '启用 STARTTLS', type: 'checkbox' },
      { key: 'insecureSkipVerify', label: '跳过证书校验', type: 'checkbox' },
      { key: 'timeoutSeconds', label: '超时时间', placeholder: '8', type: 'number' },
      { key: 'groupFilter', label: '用户组过滤器', placeholder: 'cn=ops,dc=example,dc=com' },
    ],
  },
};

const authProviderOrder: AuthProviderId[] = ['ldap'];

export function AuthSettingsPanel({ canManage = true }: { canManage?: boolean }) {
  const [providers, setProviders] = useState<Record<string, AuthProvider>>({});
  const [selected, setSelected] = useState<AuthProviderId>('ldap');
  const [form, setForm] = useState<Record<string, unknown>>({});
  const [name, setName] = useState('AD/LDAP');
  const [enabled, setEnabled] = useState(false);
  const [busy, setBusy] = useState('');

  const load = useCallback(async () => {
    try {
      const response = await fetchAuthProviders();
      setProviders(Object.fromEntries(response.items.map(item => [item.id, item])));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '读取认证配置失败');
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const provider = providers[selected];
    setEnabled(provider?.enabled ?? false);
    setName(provider?.name ?? authProviderMeta[selected].name);
    setForm(normalizeConfig(provider?.config));
  }, [providers, selected]);

  const cards = useMemo(
    () =>
      authProviderOrder.map(id => ({ id, meta: authProviderMeta[id], provider: providers[id] })),
    [providers]
  );
  const meta = authProviderMeta[selected];
  const Icon = meta.icon;

  async function save() {
    const displayName = name.trim();
    if (!displayName) {
      toast.error('显示名称不能为空');
      return;
    }
    const { config, error } = prepareAuthConfig(selected, form, enabled);
    if (error) {
      toast.error(error);
      return;
    }
    setBusy('save');
    try {
      const saved = await updateAuthProvider(selected, { name: displayName, enabled, config });
      setProviders(current => ({ ...current, [selected]: saved }));
      toast.success('认证配置已保存');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '保存认证配置失败');
    } finally {
      setBusy('');
    }
  }

  async function test() {
    setBusy('test');
    try {
      const result = await testAuthProvider(selected);
      toast.success(`认证连接测试通过，成功匹配 ${result.matchedUsers} 个用户`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '认证连接测试失败');
    } finally {
      setBusy('');
    }
  }

  function clearConfig() {
    setEnabled(false);
    setForm({});
  }

  function updateField(field: SettingsField, value: unknown) {
    setForm(current => {
      const next = { ...current, [field.key]: value };
      if (field.key === 'useTLS' && value === true) {
        next.startTLS = false;
        next.port = 636;
      }
      if (field.key === 'startTLS' && value === true) {
        next.useTLS = false;
        next.port = 389;
      }
      return next;
    });
  }

  return (
    <SettingsSplitLayout
      sidebarLabel="认证配置"
      sidebar={cards.map(({ id, meta: itemMeta, provider }) => (
        <AuthProviderCard
          key={id}
          meta={itemMeta}
          active={selected === id}
          enabled={provider?.enabled ?? false}
          onClick={() => setSelected(id)}
        />
      ))}
    >
      <SettingsDetailPanel
        header={
          <SettingsDetailHeader
            icon={Icon}
            color={meta.color}
            title={meta.name}
            subtitle={enabled ? '已启用' : '未启用'}
            active={enabled}
          />
        }
        actions={
          canManage ? (
            <>
              <ActionButton
                icon={<Trash2 size={14} />}
                label="清空配置"
                tone="danger"
                disabled={busy !== ''}
                onClick={clearConfig}
              />
              <ActionButton
                icon={<Save size={14} />}
                label="保存"
                busy={busy === 'save'}
                disabled={busy !== '' && busy !== 'save'}
                onClick={save}
              />
              <ActionButton
                icon={<CheckCircle2 size={14} />}
                label="测试"
                tone="success"
                busy={busy === 'test'}
                disabled={busy !== '' || !enabled}
                onClick={test}
              />
            </>
          ) : null
        }
      >
        <p className="mb-4 text-sm leading-6" style={{ color: 'var(--zl-text-muted)' }}>
          {meta.description}
        </p>
        <EnableToggle
          enabled={enabled}
          disabled={!canManage}
          onChange={setEnabled}
          label="启用认证"
          enabledText="登录页将显示该认证方式"
          disabledText="关闭后不会显示在登录页"
        />
        <div className="mt-4">
          <ConfigField
            field={{ key: 'name', label: '显示名称', placeholder: 'AD/LDAP', required: true }}
            value={name}
            disabled={!canManage}
            onChange={value => setName(String(value ?? ''))}
          />
        </div>
        <div className="mt-4 space-y-3">
          <SectionTitle title="必填配置" />
          {meta.requiredFields.map(field => (
            <ConfigField
              key={field.key}
              field={field}
              value={displayValue(field, form[field.key])}
              secretConfigured={secretConfigured(field, form)}
              disabled={!canManage}
              onChange={value => updateField(field, value)}
            />
          ))}
        </div>
        <div className="mt-5 space-y-3">
          <SectionTitle title="可选配置" />
          {meta.optionalFields.map(field => (
            <ConfigField
              key={field.key}
              field={field}
              value={displayValue(field, form[field.key])}
              secretConfigured={secretConfigured(field, form)}
              disabled={!canManage}
              onChange={value => updateField(field, value)}
            />
          ))}
        </div>
      </SettingsDetailPanel>
    </SettingsSplitLayout>
  );
}

function AuthProviderCard({
  meta,
  active,
  enabled,
  onClick,
}: {
  meta: (typeof authProviderMeta)[AuthProviderId];
  active: boolean;
  enabled: boolean;
  onClick: () => void;
}) {
  const Icon = meta.icon;
  return (
    <button
      type="button"
      onClick={onClick}
      className="zl-config-side-card flex w-full items-start gap-3 rounded-lg p-3 text-left transition-all duration-200"
      data-active={active}
      style={cardStyle(active)}
    >
      <div
        className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
        style={{
          color: meta.color,
          background: 'rgba(255,255,255,0.05)',
          border: '1px solid rgba(255,255,255,0.08)',
        }}
      >
        <Icon size={19} />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-3">
          <span className="truncate text-sm font-semibold">{meta.name}</span>
          {enabled ? (
            <ToggleRight size={18} className="shrink-0" style={{ color: '#86efac' }} />
          ) : (
            <ToggleLeft size={18} className="shrink-0" style={{ color: 'var(--zl-text-muted)' }} />
          )}
        </div>
        <p
          className="mt-1 line-clamp-2 text-xs leading-5"
          style={{ color: 'var(--zl-text-muted)' }}
        >
          {meta.description}
        </p>
      </div>
    </button>
  );
}

function SectionTitle({ title }: { title: string }) {
  return (
    <div className="text-xs font-semibold" style={{ color: 'var(--zl-text)' }}>
      {title}
    </div>
  );
}

function normalizeConfig(config: Record<string, unknown> | undefined) {
  return typeof config === 'object' && config !== null ? { ...config } : {};
}

function displayValue(field: SettingsField, value: unknown) {
  if (field.type === 'checkbox') return Boolean(value);
  if (field.type === 'number') return value ?? '';
  return String(value ?? '');
}

function secretConfigured(field: SettingsField, form: Record<string, unknown>) {
  return (
    field.type === 'password' &&
    Boolean(form[`has${field.key[0].toUpperCase()}${field.key.slice(1)}`])
  );
}

function removeSecretPresenceMarkers(config: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(config).filter(([key]) => !/^has[A-Z]/.test(key)));
}

function removeEmptyConfigValues(config: Record<string, unknown>) {
  return Object.fromEntries(
    Object.entries(config).filter(([, value]) => {
      if (typeof value === 'string') return value.trim() !== '';
      if (Array.isArray(value)) return value.length > 0;
      return value !== undefined && value !== null && value !== '';
    })
  );
}

function prepareAuthConfig(
  id: AuthProviderId,
  form: Record<string, unknown>,
  enabled: boolean
): { config: Record<string, unknown>; error: string } {
  const next = { ...form };
  if (!enabled)
    return { config: removeEmptyConfigValues(removeSecretPresenceMarkers(next)), error: '' };
  if (id === 'ldap') {
    if (Boolean(next.useTLS) && Boolean(next.startTLS))
      return { config: {}, error: 'LDAPS 与 StartTLS 不能同时启用' };
    const missing = authProviderMeta[id].requiredFields.find(field => {
      if (field.type === 'number') return !Number(next[field.key]);
      if (field.type === 'password' && secretConfigured(field, next)) return false;
      return !String(next[field.key] ?? '').trim();
    });
    if (missing) return { config: {}, error: `${missing.label}不能为空` };
    const port = parsePort(next.port);
    if (!port) return { config: {}, error: '端口需为 1 到 65535 之间的整数' };
    next.port = port;
  }
  return { config: removeEmptyConfigValues(removeSecretPresenceMarkers(next)), error: '' };
}

function parsePort(value: unknown) {
  const text = String(value ?? '').trim();
  if (!/^\d+$/.test(text)) return 0;
  const port = Number(text);
  if (!Number.isInteger(port) || port < 1 || port > 65535) return 0;
  return port;
}
