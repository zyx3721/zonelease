import { CheckCircle2, Mail, Save, Send, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import {
  fetchNotificationChannels,
  testNotificationChannel,
  updateNotificationChannel,
  type NotificationChannel,
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

type EmailForm = Record<string, unknown>;

const defaultEmailForm: EmailForm = {
  smtpHost: '',
  smtpPort: '',
  username: '',
  password: '',
  from: '',
  fromName: 'ZoneLease',
  useTLS: false,
  startTLS: false,
  allowInsecureAuth: false,
};

const emailFields: SettingsField[] = [
  { key: 'smtpHost', label: 'SMTP 主机', placeholder: 'smtp.example.com', required: true },
  { key: 'smtpPort', label: 'SMTP 端口', placeholder: '465', required: true, inputMode: 'numeric' },
  { key: 'username', label: '用户名', placeholder: 'zonelease@example.com', required: true },
  { key: 'password', label: '密码', placeholder: 'SMTP 授权码', required: true, type: 'password' },
  { key: 'from', label: '发件人', placeholder: 'zonelease@example.com', required: true },
  { key: 'fromName', label: '发件人名称', placeholder: 'ZoneLease' },
  { key: 'useTLS', label: '启用 TLS/SSL', type: 'checkbox' },
  { key: 'startTLS', label: '启用 STARTTLS', type: 'checkbox' },
  {
    key: 'allowInsecureAuth',
    label: '允许明文认证',
    helper: '仅在 SMTP 服务明确要求明文认证时启用',
    type: 'checkbox',
  },
];

export function EmailSettingsPanel({ canManage = true }: { canManage?: boolean }) {
  const [channel, setChannel] = useState<NotificationChannel | null>(null);
  const [form, setForm] = useState<EmailForm>(defaultEmailForm);
  const [passwordResetEnabled, setPasswordResetEnabled] = useState(false);
  const [busy, setBusy] = useState('');
  const [clearConfigRequested, setClearConfigRequested] = useState(false);
  const [testDialogOpen, setTestDialogOpen] = useState(false);
  const [testEmail, setTestEmail] = useState('');

  const load = useCallback(async () => {
    try {
      const response = await fetchNotificationChannels();
      const email = response.items.find(item => item.id === 'email') ?? null;
      setChannel(email);
      setPasswordResetEnabled(email?.passwordResetEnabled ?? false);
      setForm(normalizeEmailConfig(email?.config));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '读取通知配置失败');
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function save() {
    const nextPasswordResetEnabled = passwordResetEnabled;
    const { config, error } = prepareEmailConfig(form, nextPasswordResetEnabled);
    if (error) {
      toast.error(error);
      return;
    }
    setBusy('save');
    try {
      const saved = await updateNotificationChannel('email', {
        enabled: false,
        passwordResetEnabled: nextPasswordResetEnabled,
        clearConfig: clearConfigRequested,
        config,
      });
      setChannel(saved);
      setForm(normalizeEmailConfig(saved.config));
      setClearConfigRequested(false);
      toast.success('邮件配置已保存');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '保存邮件配置失败');
    } finally {
      setBusy('');
    }
  }

  async function sendTest() {
    const to = testEmail.trim();
    if (!to) {
      toast.error('请输入测试邮箱');
      return;
    }
    setBusy('test');
    try {
      await testNotificationChannel('email', to);
      setTestDialogOpen(false);
      toast.success('测试邮件已发送');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '测试邮件发送失败');
    } finally {
      setBusy('');
    }
  }

  function clearConfig() {
    setPasswordResetEnabled(false);
    setForm(defaultEmailForm);
    setClearConfigRequested(true);
  }

  function updateField(field: SettingsField, value: unknown) {
    setClearConfigRequested(false);
    setForm(current => {
      const next = { ...current, [field.key]: value };
      if (field.key === 'useTLS' && value === true) {
        next.startTLS = false;
        next.allowInsecureAuth = false;
        next.smtpPort = 465;
      }
      if (field.key === 'startTLS' && value === true) {
        next.useTLS = false;
        next.allowInsecureAuth = false;
        next.smtpPort = 587;
      }
      if (field.key === 'allowInsecureAuth' && value === true) {
        next.useTLS = false;
        next.startTLS = false;
      }
      if (field.key === 'smtpPort') {
        const port = parsePort(value);
        if (port === 465) {
          next.useTLS = true;
          next.startTLS = false;
          next.allowInsecureAuth = false;
        } else if (port === 587) {
          next.useTLS = false;
          next.startTLS = true;
          next.allowInsecureAuth = false;
        }
      }
      return next;
    });
  }

  return (
    <SettingsSplitLayout
      sidebarLabel="通知"
      sidebar={
        <button
          type="button"
          className="zl-config-side-card flex w-full items-start gap-3 rounded-lg p-3 text-left transition-all duration-200"
          data-active="true"
          style={cardStyle(true)}
        >
          <div
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
            style={{
              color: '#10b981',
              background: 'rgba(255,255,255,0.05)',
              border: '1px solid rgba(255,255,255,0.08)',
            }}
          >
            <Mail size={19} />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex items-center justify-between gap-3">
              <span className="truncate text-sm font-semibold">{channel?.name || '邮件'}</span>
              <UsageIcon enabled={passwordResetEnabled} label="密" />
            </div>
            <p
              className="mt-1 line-clamp-2 text-xs leading-5"
              style={{ color: 'var(--zl-text-muted)' }}
            >
              通过 SMTP 发送找回密码验证码。
            </p>
          </div>
        </button>
      }
    >
      <SettingsDetailPanel
        header={
          <SettingsDetailHeader
            icon={Mail}
            color="#10b981"
            title="邮件"
            subtitle={passwordResetEnabled ? '找回密码已启用' : '未启用'}
            active={passwordResetEnabled}
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
                icon={<Send size={14} />}
                label="测试"
                tone="success"
                busy={busy === 'test'}
                disabled={busy !== '' && busy !== 'test'}
                onClick={() => {
                  setTestEmail('');
                  setTestDialogOpen(true);
                }}
              />
            </>
          ) : null
        }
      >
        <p className="mb-5 text-sm leading-6" style={{ color: 'var(--zl-text-muted)' }}>
          通过 SMTP 发送找回密码验证码。
        </p>
        <section className="space-y-3">
          <EnableToggle
            enabled={passwordResetEnabled}
            disabled={!canManage}
            onChange={value => {
              setClearConfigRequested(false);
              setPasswordResetEnabled(value);
            }}
            label="找回密码"
            enabledText="可用于发送找回密码验证码"
            disabledText="关闭后不参与找回密码"
          />
          <div className="space-y-3">
            {emailFields.slice(0, 6).map(field => (
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
        </section>
        <section className="mt-5 space-y-3">
          <div className="space-y-3">
            {emailFields.slice(6).map(field => (
              <ConfigField
                key={field.key}
                field={field}
                value={displayValue(field, form[field.key])}
                disabled={!canManage}
                onChange={value => updateField(field, value)}
              />
            ))}
          </div>
        </section>
      </SettingsDetailPanel>
      <Dialog open={testDialogOpen} onOpenChange={setTestDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>发送测试邮件</DialogTitle>
            <DialogDescription>
              输入临时测试邮箱，仅用于本次发送，不会保存到邮件配置。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <label
              className="text-xs font-medium"
              style={{ color: 'var(--zl-text-muted)' }}
              htmlFor="email-test-recipient"
            >
              测试邮箱
            </label>
            <Input
              id="email-test-recipient"
              type="email"
              inputMode="email"
              value={testEmail}
              placeholder="admin@example.com"
              disabled={busy === 'test'}
              onChange={event => setTestEmail(event.target.value)}
              onKeyDown={event => {
                if (event.key === 'Enter') {
                  event.preventDefault();
                  void sendTest();
                }
              }}
            />
          </div>
          <DialogFooter>
            <button
              type="button"
              className="zl-action-button rounded-lg border px-4 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
              disabled={busy === 'test'}
              onClick={() => setTestDialogOpen(false)}
            >
              取消
            </button>
            <button
              type="button"
              className="zl-action-button rounded-lg border px-4 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
              style={{
                borderColor: 'rgba(16,185,129,0.35)',
                color: '#34d399',
                background: 'rgba(16,185,129,0.08)',
              }}
              disabled={busy === 'test'}
              onClick={() => void sendTest()}
            >
              {busy === 'test' ? '发送中' : '发送测试'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSplitLayout>
  );
}

function UsageIcon({ enabled, label }: { enabled: boolean; label: string }) {
  return (
    <span
      className="inline-flex h-5 w-5 items-center justify-center rounded-full border text-[10px] font-semibold"
      style={{
        color: enabled ? 'var(--zl-status-green-text)' : 'var(--zl-text-muted)',
        borderColor: enabled ? 'rgba(16,185,129,0.52)' : 'var(--zl-border)',
        background: enabled ? 'rgba(16,185,129,0.14)' : 'rgba(148,163,184,0.08)',
      }}
    >
      {enabled ? <CheckCircle2 size={12} /> : label}
    </span>
  );
}

function normalizeEmailConfig(config: Record<string, unknown> | undefined): EmailForm {
  const next = { ...defaultEmailForm, ...(config ?? {}) };
  delete next.to;
  if (next.fromAddress && !next.from) next.from = next.fromAddress;
  if (typeof next.useTls === 'boolean') next.useTLS = next.useTls;
  if (typeof next.startTls === 'boolean') next.startTLS = next.startTls;
  return next;
}

function displayValue(field: SettingsField, value: unknown) {
  if (field.type === 'checkbox') return Boolean(value);
  return String(value ?? '');
}

function secretConfigured(field: SettingsField, form: EmailForm) {
  return (
    field.type === 'password' &&
    Boolean(form[`has${field.key[0].toUpperCase()}${field.key.slice(1)}`])
  );
}

function prepareEmailConfig(
  form: EmailForm,
  enabled: boolean
): { config: Record<string, unknown>; error: string } {
  const next = removeSecretPresenceMarkers({ ...form });
  if (!enabled) return { config: removeEmptyConfigValues(next), error: '' };
  if (Boolean(next.useTLS) && Boolean(next.startTLS))
    return { config: {}, error: 'TLS 与 STARTTLS 不能同时启用' };
  for (const field of emailFields) {
    if (!field.required) continue;
    if (field.type === 'password' && secretConfigured(field, form)) continue;
    if (!String(next[field.key] ?? '').trim())
      return { config: {}, error: `${field.label}不能为空` };
    if (field.key === 'smtpPort') {
      const port = parsePort(next.smtpPort);
      if (!port) return { config: {}, error: 'SMTP 端口需为 1 到 65535 之间的整数' };
      next.smtpPort = port;
    }
  }
  if (Boolean(next.useTLS)) {
    next.smtpPort = 465;
    next.allowInsecureAuth = false;
  } else if (Boolean(next.startTLS)) {
    next.smtpPort = 587;
    next.allowInsecureAuth = false;
  }
  return { config: removeEmptyConfigValues(next), error: '' };
}

function removeSecretPresenceMarkers(config: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(config).filter(([key]) => !/^has[A-Z]/.test(key)));
}

function removeEmptyConfigValues(config: Record<string, unknown>) {
  return Object.fromEntries(
    Object.entries(config).filter(([, value]) =>
      typeof value === 'string'
        ? value.trim() !== ''
        : value !== undefined && value !== null && value !== ''
    )
  );
}

function parsePort(value: unknown) {
  const text = String(value ?? '').trim();
  if (!/^\d+$/.test(text)) return 0;
  const port = Number(text);
  return Number.isInteger(port) && port >= 1 && port <= 65535 ? port : 0;
}
