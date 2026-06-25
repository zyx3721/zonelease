import { Image, KeyRound, RotateCcw, RadioTower, Save, Settings2, UploadCloud } from 'lucide-react';
import { useCallback, useEffect, useRef, useState, type DragEvent } from 'react';
import { toast } from 'sonner';
import {
  type SystemBaseConfig,
  fetchSystemBaseConfig,
  updateSystemBaseConfig,
} from '@/lib/system-settings';
import { defaultBaseConfig, normalizeBaseConfig, setBaseConfigSnapshot } from '@/lib/branding';
import { AgentPanel } from './AgentSettingsPanel';
import {
  ActionButton,
  ConfigField,
  NumberControl,
  SettingsDetailHeader,
  SettingsDetailPanel,
  SettingsSplitLayout,
  cardStyle,
} from './settings-primitives';

type BaseTab = 'brand' | 'security' | 'sync' | 'agent';

const cards = [
  {
    id: 'brand' as const,
    title: '品牌标识',
    description: '网站名称、登录展示和图标',
    icon: Image,
    color: '#38bdf8',
  },
  {
    id: 'security' as const,
    title: '安全时效',
    description: '找回密码验证码、发送冷却与限流窗口',
    icon: KeyRound,
    color: '#22c55e',
  },
  {
    id: 'sync' as const,
    title: '同步参数',
    description: '控制全量同步、DNS 区域、DHCP 作用域并发和操作后延迟刷新',
    icon: Settings2,
    color: '#f59e0b',
  },
  {
    id: 'agent' as const,
    title: 'Agent 判定',
    description: '控制离线阈值、自动健康检查并发、检查间隔和超时时间',
    icon: RadioTower,
    color: '#8b5cf6',
  },
];

export function BaseSettingsPanel({ canManage = true }: { canManage?: boolean }) {
  const [active, setActive] = useState<BaseTab>('brand');
  const [form, setForm] = useState<SystemBaseConfig>(defaultBaseConfig);
  const [saved, setSaved] = useState<SystemBaseConfig>(defaultBaseConfig);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const config = normalizeBaseConfig(await fetchSystemBaseConfig());
      setForm(config);
      setSaved(config);
      setBaseConfigSnapshot(config);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '读取基础配置失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const selected = cards.find(card => card.id === active) ?? cards[0];
  const ActiveIcon = selected.icon;
  async function save() {
    const next = normalizeBaseConfig({ ...saved, ...pickTabPatch(form, active) });
    const error = validateTab(active, next);
    if (error) {
      toast.error(error);
      return;
    }
    setBusy(true);
    try {
      const updated = normalizeBaseConfig(await updateSystemBaseConfig(next));
      setForm(updated);
      setSaved(updated);
      setBaseConfigSnapshot(updated);
      toast.success('基础配置已保存');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存基础配置失败');
    } finally {
      setBusy(false);
    }
  }

  function update(patch: Partial<SystemBaseConfig>) {
    setForm(current => ({ ...current, ...patch }));
  }

  function selectTab(tab: BaseTab) {
    setActive(tab);
    setForm(saved);
  }

  async function updateFile(file: File | undefined) {
    if (!file) return;
    if (!file.type.startsWith('image/')) {
      toast.error('请选择图片文件');
      return;
    }
    if (file.size > 256 * 1024) {
      toast.error('图标文件不能超过 256KB');
      return;
    }
    update({ iconData: await readFileAsDataURL(file) });
  }

  function onDrop(event: DragEvent<HTMLButtonElement>) {
    event.preventDefault();
    setDragging(false);
    void updateFile(event.dataTransfer.files[0]);
  }

  return (
    <SettingsSplitLayout
      sidebarLabel="基础配置"
      sidebar={cards.map(card => (
        <BaseCard
          key={card.id}
          card={card}
          active={active === card.id}
          onClick={() => selectTab(card.id)}
        />
      ))}
    >
      <SettingsDetailPanel
        header={
          <SettingsDetailHeader
            icon={ActiveIcon}
            color={selected.color}
            title={selected.title}
            subtitle={selected.description}
          />
        }
        actions={
          canManage ? (
            <>
              <ActionButton
                icon={<RotateCcw size={14} />}
                label="恢复默认"
                tone="muted"
                disabled={loading || busy}
                onClick={() =>
                  setForm(current => ({ ...current, ...pickTabPatch(defaultBaseConfig, active) }))
                }
              />
              <ActionButton
                icon={<Save size={14} />}
                label="保存"
                busy={busy}
                disabled={loading}
                onClick={save}
              />
            </>
          ) : null
        }
      >
        {loading ? (
          <div className="text-sm" style={{ color: 'var(--zl-text-muted)' }}>
            正在加载基础配置
          </div>
        ) : (
          <>
            {active === 'brand' ? (
              <BrandPanel
                form={form}
                dragging={dragging}
                fileInputRef={fileInputRef}
                setDragging={setDragging}
                onDrop={onDrop}
                onFile={updateFile}
                onUpdate={update}
                disabled={!canManage}
              />
            ) : null}
            {active === 'security' ? (
              <SecurityPanel form={form} onUpdate={update} disabled={!canManage} />
            ) : null}
            {active === 'sync' ? (
              <SyncPanel form={form} onUpdate={update} disabled={!canManage} />
            ) : null}
            {active === 'agent' ? (
              <AgentPanel form={form} onUpdate={update} disabled={!canManage} />
            ) : null}
          </>
        )}
      </SettingsDetailPanel>
    </SettingsSplitLayout>
  );
}

function BaseCard({
  card,
  active,
  onClick,
}: {
  card: (typeof cards)[number];
  active: boolean;
  onClick: () => void;
}) {
  const Icon = card.icon;
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
          color: card.color,
          background: 'rgba(255,255,255,0.05)',
          border: '1px solid rgba(255,255,255,0.08)',
        }}
      >
        <Icon size={19} />
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-semibold">{card.title}</div>
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

function BrandPanel({
  form,
  dragging,
  fileInputRef,
  setDragging,
  onDrop,
  onFile,
  onUpdate,
  disabled,
}: {
  form: SystemBaseConfig;
  dragging: boolean;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  setDragging: (value: boolean) => void;
  onDrop: (event: DragEvent<HTMLButtonElement>) => void;
  onFile: (file: File | undefined) => void;
  onUpdate: (patch: Partial<SystemBaseConfig>) => void;
  disabled?: boolean;
}) {
  return (
    <div className="space-y-5">
      <div className="grid gap-4 lg:grid-cols-2">
        <ConfigField
          field={{ key: 'siteName', label: '网站名称', placeholder: 'ZoneLease' }}
          value={form.siteName}
          disabled={disabled}
          onChange={value => onUpdate({ siteName: String(value) })}
        />
        <ConfigField
          field={{
            key: 'loginName',
            label: '认证页品牌名称',
            placeholder: 'ZoneLease',
          }}
          value={form.loginName}
          disabled={disabled}
          onChange={value => onUpdate({ loginName: String(value) })}
        />
        <ConfigField
          field={{
            key: 'appName',
            label: '控制台品牌名称',
            placeholder: 'ZoneLease',
          }}
          value={form.appName}
          disabled={disabled}
          onChange={value => onUpdate({ appName: String(value) })}
        />
        <ConfigField
          field={{
            key: 'appSubtitle',
            label: '控制台品牌副标题',
            placeholder: 'DNS / DHCP Control',
          }}
          value={form.appSubtitle}
          disabled={disabled}
          onChange={value => onUpdate({ appSubtitle: String(value) })}
        />
      </div>
      <div className="grid gap-4 lg:grid-cols-[260px_minmax(0,1fr)]">
        <button
          type="button"
          disabled={disabled}
          onClick={() => {
            if (!disabled) fileInputRef.current?.click();
          }}
          onDragOver={event => {
            event.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
          className="zl-action-button flex min-h-44 flex-col items-center justify-center gap-3 rounded-xl border border-dashed p-4"
          style={{
            borderColor: dragging ? 'rgba(96,165,250,0.78)' : 'var(--zl-border)',
            background: dragging ? 'rgba(59,130,246,0.12)' : 'rgba(255,255,255,0.026)',
            color: 'var(--zl-text-muted)',
          }}
        >
          <img
            src={form.iconData || '/favicon.svg'}
            alt="系统图标预览"
            className="h-16 w-16 rounded-xl object-contain"
          />
          <span className="flex items-center gap-2 text-sm">
            <UploadCloud size={15} />
            拖动或点击上传
          </span>
          <span className="text-xs">支持图片，建议 256KB 内</span>
        </button>
        <BrandPreview config={form} />
      </div>
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        disabled={disabled}
        className="hidden"
        onChange={event => void onFile(event.target.files?.[0])}
      />
    </div>
  );
}

function BrandPreview({ config }: { config: SystemBaseConfig }) {
  return (
    <div
      className="flex min-h-44 flex-col rounded-xl border p-5"
      style={{ borderColor: 'var(--zl-border)', background: 'rgba(255,255,255,0.026)' }}
    >
      <div className="text-sm font-semibold" style={{ color: 'var(--zl-text-muted)' }}>
        实时预览
      </div>
      <div className="flex flex-1 items-center">
        <div className="flex min-w-0 items-center gap-4">
          <img
            src={config.iconData || '/favicon.svg'}
            alt={config.appName}
            className="h-16 w-16 shrink-0 rounded-xl object-contain"
          />
          <div className="min-w-0">
            <div className="zl-gradient-text truncate text-xl font-bold">{config.appName}</div>
            <div
              className="mt-1 truncate text-sm tracking-widest"
              style={{ color: 'var(--zl-text-muted)' }}
            >
              {config.appSubtitle}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function SecurityPanel({
  form,
  onUpdate,
  disabled,
}: {
  form: SystemBaseConfig;
  onUpdate: (patch: Partial<SystemBaseConfig>) => void;
  disabled?: boolean;
}) {
  return (
    <div className="grid gap-3 lg:grid-cols-2">
      <NumberControl
        label="找回密码验证码有效期"
        unit="分钟"
        value={form.resetCodeTtlMinutes}
        min={1}
        max={60}
        disabled={disabled}
        onChange={value => onUpdate({ resetCodeTtlMinutes: value })}
      />
      <NumberControl
        label="图形验证码有效期"
        unit="分钟"
        value={form.resetCaptchaTtlMinutes}
        min={1}
        max={10}
        disabled={disabled}
        onChange={value => onUpdate({ resetCaptchaTtlMinutes: value })}
      />
      <NumberControl
        label="发送冷却时间"
        description={`验证码发送后 ${form.passwordResetSendCooldownMinutes} 分钟内不可重复请求`}
        unit="分钟"
        value={form.passwordResetSendCooldownMinutes}
        min={0.5}
        max={10}
        step={0.5}
        disabled={disabled}
        onChange={value =>
          onUpdate({
            passwordResetSendCooldownMinutes: value,
          })
        }
      />
      <NumberControl
        label="频率限制统计窗口"
        description={`验证码在 ${form.passwordResetRateLimitMinutes} 分钟内最多请求 5 次`}
        unit="分钟"
        value={form.passwordResetRateLimitMinutes}
        min={5}
        max={10}
        disabled={disabled}
        onChange={value => onUpdate({ passwordResetRateLimitMinutes: value })}
      />
    </div>
  );
}

function SyncPanel({
  form,
  onUpdate,
  disabled,
}: {
  form: SystemBaseConfig;
  onUpdate: (patch: Partial<SystemBaseConfig>) => void;
  disabled?: boolean;
}) {
  return (
    <div className="grid gap-3 lg:grid-cols-2">
      <NumberControl
        label="全量同步并发"
        description={`全量刷新时最多同时同步 ${form.runtimeSyncConcurrency} 个 Agent`}
        unit="个"
        value={form.runtimeSyncConcurrency}
        min={1}
        max={20}
        disabled={disabled}
        onChange={value => onUpdate({ runtimeSyncConcurrency: value })}
      />
      <NumberControl
        label="DNS 区域并发"
        description={`单个 DNS Agent 内最多同时采集 ${form.dnsRecordConcurrency} 个区域记录`}
        unit="个"
        value={form.dnsRecordConcurrency}
        min={1}
        max={50}
        disabled={disabled}
        onChange={value => onUpdate({ dnsRecordConcurrency: value })}
      />
      <NumberControl
        label="DHCP 作用域并发"
        description={`单个 DHCP Agent 内最多同时采集 ${form.dhcpScopeConcurrency} 个作用域详情`}
        unit="个"
        value={form.dhcpScopeConcurrency}
        min={1}
        max={50}
        disabled={disabled}
        onChange={value => onUpdate({ dhcpScopeConcurrency: value })}
      />
      <NumberControl
        label="操作后刷新等待"
        description={`DNS / DHCP 操作成功后，同一目标静默 ${form.operationRefreshDelaySeconds} 秒再执行二次同步`}
        unit="秒"
        value={form.operationRefreshDelaySeconds}
        min={1}
        max={60}
        disabled={disabled}
        onChange={value => onUpdate({ operationRefreshDelaySeconds: value })}
      />
    </div>
  );
}

function pickTabPatch(config: SystemBaseConfig, tab: BaseTab): Partial<SystemBaseConfig> {
  if (tab === 'brand') {
    const siteName = withDefault(config.siteName, defaultBaseConfig.siteName);
    const loginName = withDefault(config.loginName, defaultBaseConfig.loginName);
    const appName = withDefault(config.appName, defaultBaseConfig.appName);
    const appSubtitle = withDefault(config.appSubtitle, defaultBaseConfig.appSubtitle);
    const iconData = withDefault(config.iconData, defaultBaseConfig.iconData);
    return {
      siteName,
      loginName,
      appName,
      appSubtitle,
      iconData,
    };
  }
  if (tab === 'security') {
    return {
      resetCodeTtlMinutes: config.resetCodeTtlMinutes,
      resetCaptchaTtlMinutes: config.resetCaptchaTtlMinutes,
      passwordResetSendCooldownMinutes: config.passwordResetSendCooldownMinutes,
      passwordResetRateLimitMinutes: config.passwordResetRateLimitMinutes,
    };
  }
  if (tab === 'sync') {
    return {
      runtimeSyncConcurrency: config.runtimeSyncConcurrency,
      dnsRecordConcurrency: config.dnsRecordConcurrency,
      dhcpScopeConcurrency: config.dhcpScopeConcurrency,
      operationRefreshDelaySeconds: config.operationRefreshDelaySeconds,
    };
  }
  return {
    agentOfflineFailureCount: config.agentOfflineFailureCount,
    agentOperationTimeoutSeconds: config.agentOperationTimeoutSeconds,
    agentFullSyncTimeoutSeconds: config.agentFullSyncTimeoutSeconds,
    agentHealthCheckIntervalMinutes: config.agentHealthCheckIntervalMinutes,
    agentHealthCheckConcurrency: config.agentHealthCheckConcurrency,
  };
}

function validateTab(tab: BaseTab, _config: SystemBaseConfig) {
  if (tab === 'brand') return '';
  return '';
}

function withDefault(value: string, fallback: string) {
  return value.trim() || fallback;
}

function readFileAsDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ''));
    reader.onerror = () => reject(new Error('读取图标失败'));
    reader.readAsDataURL(file);
  });
}
