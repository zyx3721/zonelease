import { createFileRoute } from '@tanstack/react-router';
import {
  AlertTriangle,
  CheckCircle2,
  PlugZap,
  RefreshCw,
  Save,
  Server,
  Trash2,
  Wifi,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import { AppTooltip } from '@/components/app-tooltip';
import { AgentRoleBadge } from '@/components/agent-role-badge';
import { getStoredUser, userHasPermission } from '@/lib/auth';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  addServer,
  pingServer,
  probeServer,
  reloadDB,
  removeServer,
  syncServer,
  useDB,
  waitRefreshTask,
  type ServerConfig,
} from '@/lib/dns-dhcp-store';
import { taskToastDoneOptionsFor, taskToastOptions } from '@/lib/task-toast';

export const Route = createFileRoute('/_authenticated/settings')({
  component: SettingsPage,
});

type AgentForm = {
  name: string;
  agentUrl: string;
  apiKey: string;
  role: ServerConfig['role'] | '';
  tlsInsecure: boolean;
};

type VerifiedAgent = {
  agentUrl: string;
  apiKey: string;
  role: ServerConfig['role'];
  tlsInsecure: boolean;
};

function SettingsPage() {
  const db = useDB();
  const user = getStoredUser();
  const canManageServers = userHasPermission(user, 'servers.manage');
  const [form, setForm] = useState<AgentForm>({
    name: '',
    agentUrl: '',
    apiKey: '',
    role: '',
    tlsInsecure: false,
  });
  const [busy, setBusy] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<ServerConfig | null>(null);
  const [visibleDeleteTarget, setVisibleDeleteTarget] = useState<ServerConfig | null>(null);
  const [probeResult, setProbeResult] = useState<{
    type: 'success' | 'error';
    message: string;
  } | null>(null);
  const [verifiedAgent, setVerifiedAgent] = useState<VerifiedAgent | null>(null);
  const role = form.role as ServerConfig['role'];
  const formReady = form.name.trim() !== '' && form.agentUrl.trim() !== '' && form.role !== '';
  const canProbeAgent = formReady;
  const canCreateAgent =
    formReady &&
    verifiedAgent !== null &&
    verifiedAgent.agentUrl === form.agentUrl.trim() &&
    verifiedAgent.apiKey === form.apiKey.trim() &&
    verifiedAgent.role === role &&
    verifiedAgent.tlsInsecure === form.tlsInsecure;

  useEffect(() => {
    if (deleteTarget) setVisibleDeleteTarget(deleteTarget);
  }, [deleteTarget]);

  async function handleCreateAgent() {
    if (!canCreateAgent) {
      setProbeResult({ type: 'error', message: '请先测试 Agent 连接并确认成功' });
      return;
    }
    setBusy('create');
    setProbeResult(null);
    try {
      const created = await addServer({
        name: form.name.trim(),
        role,
        agentUrl: form.agentUrl.trim(),
        apiKey: form.apiKey.trim(),
        tlsInsecure: form.tlsInsecure,
      });
      setForm({ name: '', agentUrl: '', apiKey: '', role: '', tlsInsecure: false });
      setVerifiedAgent(null);
      const toastId = toast.loading(`${created.name} 已保存，开始同步`, taskToastOptions);
      await handleSyncSavedAgent(created, toastId);
    } catch (error) {
      setProbeResult({
        type: 'error',
        message: error instanceof Error ? error.message : 'Agent 操作失败',
      });
    } finally {
      setBusy('');
    }
  }

  async function handleProbeAgent() {
    if (!canProbeAgent) {
      setProbeResult({ type: 'error', message: 'Agent 名称、地址和角色不能为空' });
      return;
    }
    setBusy('probe');
    setProbeResult(null);
    setVerifiedAgent(null);
    try {
      const result = await probeServer({
        name: form.name.trim(),
        role,
        agentUrl: form.agentUrl.trim(),
        apiKey: form.apiKey.trim(),
        tlsInsecure: form.tlsInsecure,
      });
      if (result.status !== 'Online') {
        setProbeResult({ type: 'error', message: result.detail || 'Agent 连接测试失败' });
        return;
      }
      setVerifiedAgent({
        agentUrl: form.agentUrl.trim(),
        apiKey: form.apiKey.trim(),
        role,
        tlsInsecure: form.tlsInsecure,
      });
      setProbeResult({ type: 'success', message: 'Agent 连接测试成功' });
    } catch (error) {
      setProbeResult({
        type: 'error',
        message: error instanceof Error ? error.message : 'Agent 连接测试失败',
      });
    } finally {
      setBusy('');
    }
  }

  async function handleTestSavedAgent(agent: ServerConfig) {
    setBusy(`${agent.id}:test`);
    const toastId = toast.loading(`${agent.name} 正在测试连接`, taskToastOptions);
    try {
      const result = await pingServer(agent.id);
      if (result.status === 'Online') {
        toast.success(`${agent.name} 连接测试成功`, taskToastDoneOptionsFor(toastId));
      } else {
        toast.error(
          result.detail || `${agent.name} 连接测试失败`,
          taskToastDoneOptionsFor(toastId)
        );
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : 'Agent 连接测试失败',
        taskToastDoneOptionsFor(toastId)
      );
    } finally {
      setBusy('');
    }
  }

  async function handleSyncSavedAgent(agent: ServerConfig, existingToastId?: string | number) {
    setBusy(`${agent.id}:sync`);
    const toastId = existingToastId ?? toast.loading(`${agent.name} 正在同步`, taskToastOptions);
    try {
      const task = await syncServer(agent.id);
      await waitRefreshTask(task.id);
      await Promise.all([reloadDB(), reloadDB({ includeDns: true })]);
      toast.success(`${agent.name} 同步完成`, taskToastDoneOptionsFor(toastId));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : 'Agent 同步失败',
        taskToastDoneOptionsFor(toastId)
      );
    } finally {
      setBusy('');
    }
  }

  async function confirmDeleteAgent() {
    if (!deleteTarget) return;
    const agent = deleteTarget;
    setBusy(`${agent.id}:delete`);
    try {
      await removeServer(agent.id);
      setDeleteTarget(null);
      toast.success(`${agent.name} 已删除`);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '删除 Agent 失败');
    } finally {
      setBusy('');
    }
  }

  return (
    <div className="space-y-6">
      {canManageServers ? (
        <section
          className="zl-card-hover rounded-xl p-5"
          style={{
            background: 'var(--zl-card)',
            border: '1px solid var(--zl-border)',
            boxShadow: 'var(--shadow-card)',
          }}
        >
          <div className="mb-4 flex items-center justify-between gap-4">
            <div>
              <h2
                className="flex items-center gap-2 text-sm font-semibold"
                style={{ color: 'var(--zl-text)' }}
              >
                <PlugZap size={16} />
                Agent 接入
              </h2>
              <p className="mt-1 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
                接入 Windows DNS / DHCP Agent 后，可同步真实服务器资源并执行管理操作
              </p>
            </div>
            <span className="text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              {db.servers.length} 个 Agent
            </span>
          </div>

          <div className="grid grid-cols-1 gap-3 xl:grid-cols-[1fr_1.35fr_0.9fr_0.9fr_auto_auto_auto]">
            <AgentInput
              value={form.name}
              onChange={value => setForm(current => ({ ...current, name: value }))}
              placeholder="Agent 名称"
            />
            <AgentInput
              value={form.agentUrl}
              onChange={value => {
                setVerifiedAgent(null);
                setProbeResult(null);
                setForm(current => ({ ...current, agentUrl: value }));
              }}
              placeholder="http://127.0.0.1:9443"
            />
            <AgentInput
              value={form.apiKey}
              onChange={value => {
                setVerifiedAgent(null);
                setProbeResult(null);
                setForm(current => ({ ...current, apiKey: value }));
              }}
              placeholder="API Key"
              type="password"
            />
            <Select
              value={form.role}
              onValueChange={value => {
                setVerifiedAgent(null);
                setProbeResult(null);
                setForm(current => ({ ...current, role: value as ServerConfig['role'] }));
              }}
            >
              <SelectTrigger className="font-normal">
                <SelectValue placeholder="请选择角色" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="role-placeholder" disabled className="font-normal">
                  请选择角色
                </SelectItem>
                <SelectItem value="DNS" className="font-normal">
                  DNS
                </SelectItem>
                <SelectItem value="DHCP" className="font-normal">
                  DHCP
                </SelectItem>
              </SelectContent>
            </Select>
            <label
              className="zl-action-button inline-flex h-10 cursor-pointer items-center gap-2 rounded-lg border px-3 text-sm font-semibold"
              style={{
                background: form.tlsInsecure
                  ? 'rgba(45,212,191,0.12)'
                  : 'var(--zl-control-bg-soft)',
                borderColor: form.tlsInsecure ? 'rgba(45,212,191,0.34)' : 'var(--zl-border)',
                color: form.tlsInsecure ? '#34d399' : 'var(--zl-text-muted)',
              }}
            >
              <input
                type="checkbox"
                checked={form.tlsInsecure}
                onChange={event => {
                  setVerifiedAgent(null);
                  setProbeResult(null);
                  setForm(current => ({ ...current, tlsInsecure: event.target.checked }));
                }}
                className="h-4 w-4 accent-[var(--zl-accent)]"
              />
              跳过 TLS 校验
            </label>
            <button
              type="button"
              onClick={() => void handleProbeAgent()}
              disabled={busy === 'probe' || !canProbeAgent}
              className="zl-action-button flex h-10 items-center justify-center gap-2 rounded-lg border px-4 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-60"
              style={{
                color: 'var(--zl-accent-text)',
                borderColor: 'rgba(59,130,246,0.35)',
                background: 'rgba(59,130,246,0.08)',
              }}
            >
              <Wifi size={15} />
              {busy === 'probe' ? '测试中' : '测试'}
            </button>
            <button
              type="button"
              onClick={() => void handleCreateAgent()}
              disabled={busy === 'create' || !canCreateAgent}
              className="zl-action-button flex h-10 items-center justify-center gap-2 rounded-lg border px-4 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-60"
              style={{
                color: '#34d399',
                borderColor: 'rgba(16,185,129,0.35)',
                background: 'rgba(16,185,129,0.08)',
              }}
            >
              <Save size={15} />
              {busy === 'create' ? '保存中' : '保存'}
            </button>
          </div>

          {probeResult ? <AgentProbeResult result={probeResult} /> : null}
        </section>
      ) : null}

      <section
        className="zl-card-hover rounded-xl p-5"
        style={{
          background: 'var(--zl-card)',
          border: '1px solid var(--zl-border)',
          boxShadow: 'var(--shadow-card)',
        }}
      >
        <div className="mb-4 flex items-center justify-between gap-4">
          <div>
            <h2
              className="flex items-center gap-2 text-sm font-semibold"
              style={{ color: 'var(--zl-text)' }}
            >
              <Server size={16} />
              Windows 服务器
            </h2>
            <p className="mt-1 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              管理已登记的 DNS / DHCP Agent，支持连接测试、同步和删除
            </p>
          </div>
        </div>

        <div className="space-y-2">
          {db.servers.map(agent => (
            <div
              key={agent.id}
              className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-lg px-3 py-3 text-sm"
              style={{
                background: 'var(--zl-control-bg-soft)',
                border: '1px solid var(--zl-border)',
                color: 'var(--zl-text)',
              }}
            >
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate font-medium">{agent.name}</span>
                  <AgentRoleBadge role={agent.role} />
                  <StatusBadge status={agent.status} />
                </div>
                <div
                  className="mt-1 truncate font-mono text-xs"
                  style={{ color: 'var(--zl-text-muted)' }}
                >
                  {agent.agentUrl}
                </div>
              </div>
              {canManageServers ? (
                <div className="flex items-center justify-end gap-2">
                  <AgentIconButton
                    label="测试连接"
                    busy={busy === `${agent.id}:test`}
                    onClick={() => void handleTestSavedAgent(agent)}
                  >
                    <Wifi size={13} />
                  </AgentIconButton>
                  <AgentIconButton
                    label="同步 Agent"
                    variant="sync"
                    busy={busy === `${agent.id}:sync`}
                    onClick={() => void handleSyncSavedAgent(agent)}
                  >
                    <RefreshCw
                      size={13}
                      className={busy === `${agent.id}:sync` ? 'animate-spin' : ''}
                    />
                  </AgentIconButton>
                  <AgentIconButton
                    label="删除 Agent"
                    danger
                    busy={busy === `${agent.id}:delete`}
                    onClick={() => setDeleteTarget(agent)}
                  >
                    <Trash2 size={13} />
                  </AgentIconButton>
                </div>
              ) : null}
            </div>
          ))}
          {db.servers.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border px-4 py-10 text-center text-sm text-muted-foreground">
              尚未添加任何 Agent
            </div>
          ) : null}
        </div>
      </section>

      <Dialog open={Boolean(deleteTarget)} onOpenChange={open => !open && setDeleteTarget(null)}>
        <DialogContent className="max-w-xl border-red-400/35 p-7 sm:p-8">
          <DialogHeader className="pr-10 text-left">
            <div className="flex items-start gap-4">
              <span
                className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl border"
                style={{
                  background: 'rgba(239,68,68,0.12)',
                  borderColor: 'rgba(239,68,68,0.3)',
                  color: '#f87171',
                }}
              >
                <AlertTriangle size={24} />
              </span>
              <div className="min-w-0">
                <DialogTitle className="text-2xl font-semibold">删除 Agent</DialogTitle>
                <DialogDescription className="mt-2 text-base leading-6">
                  将删除接入登记，并清理该 Agent 同步的 DNS / DHCP 快照数据
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>
          {deleteTarget || visibleDeleteTarget ? (
            <div
              className="mt-2 rounded-xl border p-4"
              style={{
                background: 'var(--zl-control-bg-soft)',
                borderColor: 'var(--zl-border)',
                color: 'var(--zl-text)',
              }}
            >
              <div className="truncate text-lg font-medium">
                {(deleteTarget ?? visibleDeleteTarget)?.name}
              </div>
              <div
                className="mt-2 truncate font-mono text-sm"
                style={{ color: 'var(--zl-text-muted)' }}
              >
                {(deleteTarget ?? visibleDeleteTarget)?.agentUrl}
              </div>
            </div>
          ) : null}
          <DialogFooter className="mt-2 gap-3 sm:space-x-0">
            <button
              type="button"
              className="zl-action-button rounded-lg border px-6 py-3 text-base disabled:cursor-not-allowed disabled:opacity-60"
              onClick={() => setDeleteTarget(null)}
              disabled={Boolean(deleteTarget && busy === `${deleteTarget.id}:delete`)}
            >
              取消
            </button>
            <button
              type="button"
              className="zl-action-button zl-danger-button rounded-lg border px-6 py-3 text-base font-semibold disabled:cursor-not-allowed disabled:opacity-60"
              onClick={() => void confirmDeleteAgent()}
              disabled={Boolean(deleteTarget && busy === `${deleteTarget.id}:delete`)}
              style={{
                background: 'rgba(239,68,68,0.12)',
                borderColor: 'rgba(239,68,68,0.35)',
                color: 'var(--zl-status-red-text)',
              }}
            >
              {deleteTarget && busy === `${deleteTarget.id}:delete` ? '删除中' : '确认删除'}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function AgentInput({
  value,
  onChange,
  placeholder,
  type = 'text',
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  type?: string;
}) {
  return (
    <input
      value={value}
      type={type}
      onChange={event => onChange(event.target.value)}
      placeholder={placeholder}
      className="h-10 rounded-lg px-3 text-sm outline-none"
      style={{
        background: 'var(--zl-control-bg)',
        border: '1px solid var(--zl-border)',
        color: 'var(--zl-text)',
      }}
    />
  );
}

function AgentProbeResult({ result }: { result: { type: 'success' | 'error'; message: string } }) {
  const success = result.type === 'success';
  return (
    <div
      className="mt-4 flex items-center gap-2 rounded-lg p-3 text-sm"
      style={{
        background: success ? 'rgba(16,185,129,0.1)' : 'rgba(239,68,68,0.1)',
        border: success ? '1px solid rgba(16,185,129,0.28)' : '1px solid rgba(239,68,68,0.28)',
        color: success ? '#34d399' : '#f87171',
      }}
    >
      {success ? <CheckCircle2 size={15} /> : null}
      {result.message}
    </div>
  );
}

function StatusBadge({ status }: { status: ServerConfig['status'] }) {
  const color =
    status === 'Online' ? '#34d399' : status === 'Offline' ? '#f87171' : 'var(--zl-text-muted)';
  return (
    <span
      className="inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-[11px]"
      style={{ color, background: 'rgba(255,255,255,0.04)' }}
    >
      <span className="h-1.5 w-1.5 rounded-full" style={{ background: color }} />
      {status}
    </span>
  );
}

function AgentIconButton({
  children,
  label,
  busy,
  danger,
  variant,
  onClick,
}: {
  children: React.ReactNode;
  label: string;
  busy?: boolean;
  danger?: boolean;
  variant?: 'sync';
  onClick: () => void;
}) {
  const color = danger ? '#f87171' : variant === 'sync' ? '#34d399' : 'var(--zl-accent-text)';
  const borderColor = danger
    ? 'rgba(239,68,68,0.42)'
    : variant === 'sync'
      ? 'rgba(16,185,129,0.35)'
      : 'rgba(59,130,246,0.35)';
  const background = danger
    ? 'rgba(239,68,68,0.08)'
    : variant === 'sync'
      ? 'rgba(16,185,129,0.08)'
      : 'rgba(59,130,246,0.08)';
  return (
    <AppTooltip label={label} placement="top">
      <button
        type="button"
        aria-label={label}
        disabled={busy}
        onClick={onClick}
        className="zl-action-button flex h-8 w-8 items-center justify-center rounded-md border transition-colors disabled:opacity-45"
        style={{ color, borderColor, background }}
      >
        {children}
      </button>
    </AppTooltip>
  );
}
