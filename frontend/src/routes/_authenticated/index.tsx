import { createFileRoute } from '@tanstack/react-router';
import { StatCard } from '@/components/stat-card';
import { AgentRoleBadge } from '@/components/agent-role-badge';
import { useDB, pingServer } from '@/lib/dns-dhcp-store';
import {
  Globe,
  Network,
  Server,
  ScrollText,
  Activity,
  RefreshCw,
  Wifi,
  WifiOff,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { getStoredUser, userHasPermission } from '@/lib/auth';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useMemo, useRef, useState } from 'react';
import { toast } from 'sonner';
import { taskToastDoneOptionsFor, taskToastOptionsFor } from '@/lib/task-toast';

export const Route = createFileRoute('/_authenticated/')({
  component: Dashboard,
});

function Dashboard() {
  const db = useDB();
  const canManageServers = userHasPermission(getStoredUser(), 'servers.manage');
  const [checkingServers, setCheckingServers] = useState<Record<string, boolean>>({});
  const [activityLimit, setActivityLimit] = useState('10');
  const checkingRef = useRef<Record<string, boolean>>({});
  const agentStats = useMemo(() => buildAgentStats(db), [db]);
  const totalLeases = db.leases.length;
  const onlineServers = db.servers.filter(s => s.status === 'Online').length;

  async function checkServer(id: string, name: string) {
    if (checkingRef.current[id]) return;
    const toastId = `dashboard-server-ping-${id}`;
    checkingRef.current = { ...checkingRef.current, [id]: true };
    setCheckingServers(current => ({ ...current, [id]: true }));
    toast.loading(`${name} 正在检查连通性`, taskToastOptionsFor(toastId));
    try {
      const result = await pingServer(id);
      const detail = result.detail ? `，${result.detail}` : '';
      if (result.status === 'Online') {
        toast.success(`${name} 连通性检查完成${detail}`, taskToastDoneOptionsFor(toastId));
      } else {
        toast.error(`${name} 连通性检查失败${detail}`, taskToastDoneOptionsFor(toastId));
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : `${name} 连通性检查失败`,
        taskToastDoneOptionsFor(toastId)
      );
    } finally {
      checkingRef.current = { ...checkingRef.current, [id]: false };
      setCheckingServers(current => ({ ...current, [id]: false }));
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="grid shrink-0 gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label="DNS 区域"
          value={db.zones.length}
          hint={`${db.records.length} 条记录`}
          tone="info"
          icon={<Globe className="h-5 w-5" />}
        />
        <StatCard
          label="DHCP 作用域"
          value={db.scopes.length}
          hint={`${db.scopes.filter(s => s.state === 'Active').length} 个活动`}
          tone="success"
          icon={<Network className="h-5 w-5" />}
        />
        <StatCard
          label="DHCP 租约"
          value={totalLeases}
          hint={`${db.reservations.length} 个保留`}
          tone="default"
          icon={<Activity className="h-5 w-5" />}
        />
        <StatCard
          label="在线服务器"
          value={`${onlineServers} / ${db.servers.length}`}
          hint="最近一次健康检查"
          tone={onlineServers === db.servers.length ? 'success' : 'warning'}
          icon={<Server className="h-5 w-5" />}
        />
      </div>

      <div className="grid min-h-0 flex-1 gap-6 grid-cols-1 lg:grid-cols-3">
        <div
          className="zl-card-hover flex min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-card lg:col-span-2"
          style={{ boxShadow: 'var(--shadow-card)' }}
        >
          <div className="px-5 py-4 border-b border-border flex items-center justify-between">
            <div>
              <h2 className="text-sm font-semibold">服务器状态</h2>
              <p className="text-xs text-muted-foreground">
                {canManageServers
                  ? '显示最近一次健康检查结果，点击刷新可手动检查'
                  : '显示最近一次健康检查结果'}
              </p>
            </div>
          </div>
          <div className="zl-hidden-scrollbar grid min-h-0 flex-1 content-start gap-3 overflow-auto p-4 sm:grid-cols-2">
            {db.servers.map(s => {
              const stats = agentStats[s.id] ?? emptyAgentStats();
              const showDns = s.role === 'DNS';
              const showDhcp = s.role === 'DHCP';
              const failureCount = s.failureCount ?? 0;

              return (
                <div
                  key={s.id}
                  className="zl-server-status-card rounded-lg border border-border bg-muted/20 p-3"
                >
                  <div className="flex items-start gap-3">
                    <div
                      className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border ${
                        s.status === 'Online'
                          ? 'border-success/25 bg-success/10 text-success'
                          : s.status === 'Offline'
                            ? 'border-destructive/25 bg-destructive/10 text-destructive'
                            : 'border-border bg-background/60 text-muted-foreground'
                      }`}
                    >
                      {s.status === 'Online' ? (
                        <Wifi className="h-4 w-4" />
                      ) : (
                        <WifiOff className="h-4 w-4" />
                      )}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <div className="truncate text-sm font-medium">{s.name}</div>
                          <div className="mt-0.5 truncate text-xs text-muted-foreground">
                            {s.agentUrl}
                          </div>
                        </div>
                        {canManageServers ? (
                          <Button
                            size="icon"
                            variant="ghost"
                            className="h-8 w-8 shrink-0"
                            disabled={checkingServers[s.id]}
                            onClick={() => void checkServer(s.id, s.name)}
                            aria-label={`检查 ${s.name} 连通性`}
                          >
                            <RefreshCw
                              className={`h-3.5 w-3.5 ${checkingServers[s.id] ? 'animate-spin' : ''}`}
                            />
                          </Button>
                        ) : null}
                      </div>
                      <div className="mt-3 flex items-center justify-between gap-2">
                        <AgentRoleBadge role={s.role} />
                        <span
                          className={`text-xs font-medium ${
                            s.status === 'Online'
                              ? 'text-success'
                              : s.status === 'Offline'
                                ? 'text-destructive'
                                : 'text-muted-foreground'
                          }`}
                        >
                          {s.status}
                        </span>
                      </div>
                      <div className="mt-2 flex items-center justify-between gap-2 text-xs text-muted-foreground">
                        <span>最近检查</span>
                        <span className="font-mono">{formatLastChecked(s.lastChecked)}</span>
                      </div>
                      <div className="mt-2 flex items-center justify-between gap-2 text-xs text-muted-foreground">
                        <span>连续失败</span>
                        <span
                          className={`font-mono ${
                            failureCount > 0 ? 'text-destructive' : 'text-success'
                          }`}
                        >
                          {failureCount} 次
                        </span>
                      </div>
                    </div>
                  </div>
                  <div className="mt-3 grid gap-2 text-xs sm:grid-cols-2">
                    {showDns ? (
                      <AgentStatLine
                        label="DNS"
                        value={`区域 ${stats.zoneCount} / 记录 ${stats.recordCount}`}
                      />
                    ) : null}
                    {showDhcp ? (
                      <AgentStatLine
                        label="DHCP"
                        value={`作用域 ${stats.scopeCount} / 租约 ${stats.leaseCount} / 保留 ${stats.reservationCount}`}
                      />
                    ) : null}
                  </div>
                  <div
                    className={`mt-3 h-1 rounded-full ${
                      s.status === 'Online'
                        ? 'bg-success'
                        : s.status === 'Offline'
                          ? 'bg-destructive'
                          : 'bg-muted-foreground'
                    }`}
                  />
                </div>
              );
            })}
          </div>
        </div>

        <div
          className="zl-card-hover flex min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-card"
          style={{ boxShadow: 'var(--shadow-card)' }}
        >
          <div className="px-5 py-4 border-b border-border flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2">
              <ScrollText className="h-4 w-4 shrink-0 text-muted-foreground" />
              <h2 className="truncate text-sm font-semibold">最近活动</h2>
            </div>
            <Select value={activityLimit} onValueChange={setActivityLimit}>
              <SelectTrigger className="h-8 w-[86px] px-2 text-xs font-medium">
                <SelectValue />
              </SelectTrigger>
              <SelectContent align="end" className="min-w-0">
                <SelectItem value="10" className="text-xs font-medium">
                  10 条
                </SelectItem>
                <SelectItem value="20" className="text-xs font-medium">
                  20 条
                </SelectItem>
                <SelectItem value="30" className="text-xs font-medium">
                  30 条
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <ul className="zl-hidden-scrollbar min-h-0 flex-1 divide-y divide-border overflow-auto">
            {db.audit.slice(0, Number(activityLimit)).map(a => (
              <li key={a.id} className="px-5 py-3">
                <div className="flex items-center gap-2 text-sm">
                  <span className="font-medium truncate">{a.action}</span>
                  <Badge variant="outline" className="text-[10px] py-0">
                    {a.module}
                  </Badge>
                </div>
                <div className="mt-0.5 text-xs text-muted-foreground truncate">
                  {a.target} · {new Date(a.ts).toLocaleString()}
                </div>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  );
}

interface AgentStats {
  zoneCount: number;
  recordCount: number;
  scopeCount: number;
  leaseCount: number;
  reservationCount: number;
}

function emptyAgentStats(): AgentStats {
  return {
    zoneCount: 0,
    recordCount: 0,
    scopeCount: 0,
    leaseCount: 0,
    reservationCount: 0,
  };
}

function formatLastChecked(value: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  const pad = (item: number) => String(item).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

function ensureAgentStats(stats: Record<string, AgentStats>, serverId: string) {
  if (!stats[serverId]) {
    stats[serverId] = emptyAgentStats();
  }
  return stats[serverId];
}

function buildAgentStats(db: ReturnType<typeof useDB>) {
  const stats: Record<string, AgentStats> = {};
  const zoneServerIds = new Map<string, string>();
  const scopeServerIds = new Map<string, string>();

  db.servers.forEach(server => {
    stats[server.id] = emptyAgentStats();
  });

  db.zones.forEach(zone => {
    zoneServerIds.set(zone.id, zone.serverId);
    ensureAgentStats(stats, zone.serverId).zoneCount += 1;
  });

  db.records.forEach(record => {
    const serverId = zoneServerIds.get(record.zoneId);
    if (serverId) ensureAgentStats(stats, serverId).recordCount += 1;
  });

  db.scopes.forEach(scope => {
    scopeServerIds.set(scope.id, scope.serverId);
    ensureAgentStats(stats, scope.serverId).scopeCount += 1;
  });

  db.leases.forEach(lease => {
    const serverId = scopeServerIds.get(lease.scopeId);
    if (serverId) ensureAgentStats(stats, serverId).leaseCount += 1;
  });

  db.reservations.forEach(reservation => {
    const serverId = scopeServerIds.get(reservation.scopeId);
    if (serverId) ensureAgentStats(stats, serverId).reservationCount += 1;
  });

  return stats;
}

function AgentStatLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-border bg-background/30 px-2.5 py-2">
      <div className="text-[10px] font-semibold uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-[11px] font-medium text-foreground">{value}</div>
    </div>
  );
}
