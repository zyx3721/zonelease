import { createFileRoute } from '@tanstack/react-router';
import { useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { AgentScopeToolbar } from '@/components/agent-scope-toolbar';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Download, Trash2, Power, Network, Search, RefreshCw } from 'lucide-react';
import { AddScopeDialog } from '@/features/dhcp/AddScopeDialog';
import { DhcpConfirmDialog } from '@/features/dhcp/DhcpConfirmDialog';
import { DhcpScopeDetailsTabs } from '@/features/dhcp/DhcpScopeDetailsTabs';
import { DhcpScopeExportDialog } from '@/features/dhcp/DhcpScopeExportDialog';
import { EditScopeDialog } from '@/features/dhcp/EditScopeDialog';
import { getStoredUser, userHasPermission } from '@/lib/auth';
import {
  useDB,
  toggleScope,
  removeScope,
  refreshScope,
  reloadDB,
  syncServer,
  waitRefreshTask,
  type DhcpExclusion,
  type DhcpLease,
  type DhcpReservation,
  type DhcpScope,
} from '@/lib/dns-dhcp-store';
import { taskToastDoneOptionsFor, taskToastOptions } from '@/lib/task-toast';

export const Route = createFileRoute('/_authenticated/dhcp')({
  component: DhcpPage,
});

const EMPTY_DHCP_EXCLUSIONS: DhcpExclusion[] = [];
type PendingScopeAction = 'toggle' | 'delete' | null;

function DhcpPage() {
  const db = useDB();
  const user = getStoredUser();
  const canManageDhcp = userHasPermission(user, 'dhcp.manage');
  const canRefresh = userHasPermission(user, 'refresh.manage');
  const canExport = userHasPermission(user, 'export.manage');
  const [selectedId, setSelectedId] = useState<string | null>(db.scopes[0]?.id ?? null);
  const [scopeQuery, setScopeQuery] = useState('');
  const dhcpAgents = useMemo(
    () => db.servers.filter(server => server.role === 'DHCP'),
    [db.servers]
  );
  const [selectedAgentId, setSelectedAgentId] = useState(dhcpAgents[0]?.id ?? '');
  const [syncingAgent, setSyncingAgent] = useState(false);
  const [refreshingScope, setRefreshingScope] = useState<string | null>(null);
  const [exportOpen, setExportOpen] = useState(false);
  const [exportLoading, setExportLoading] = useState(false);
  const [exportScopes, setExportScopes] = useState<DhcpScope[]>(db.scopes);
  const [exportExclusions, setExportExclusions] = useState<DhcpExclusion[]>(db.exclusions);
  const [exportLeases, setExportLeases] = useState<DhcpLease[]>(db.leases);
  const [exportReservations, setExportReservations] = useState<DhcpReservation[]>(db.reservations);
  const [pendingScopeAction, setPendingScopeAction] = useState<PendingScopeAction>(null);
  const [visibleScopeAction, setVisibleScopeAction] = useState<Exclude<
    PendingScopeAction,
    null
  > | null>(null);
  const [scopeActionLoading, setScopeActionLoading] = useState(false);

  const scopeDataById = useMemo(() => {
    const exclusionsByScope = new Map<string, DhcpExclusion[]>();
    const leasesByScope = new Map<string, DhcpLease[]>();
    const reservationsByScope = new Map<string, DhcpReservation[]>();

    for (const exclusion of db.exclusions) {
      const exclusionsForScope = exclusionsByScope.get(exclusion.scopeId);
      if (exclusionsForScope) {
        exclusionsForScope.push(exclusion);
      } else {
        exclusionsByScope.set(exclusion.scopeId, [exclusion]);
      }
    }

    for (const lease of db.leases) {
      const leasesForScope = leasesByScope.get(lease.scopeId);
      if (leasesForScope) {
        leasesForScope.push(lease);
      } else {
        leasesByScope.set(lease.scopeId, [lease]);
      }
    }

    for (const reservation of db.reservations) {
      const reservationsForScope = reservationsByScope.get(reservation.scopeId);
      if (reservationsForScope) {
        reservationsForScope.push(reservation);
      } else {
        reservationsByScope.set(reservation.scopeId, [reservation]);
      }
    }

    return { exclusionsByScope, leasesByScope, reservationsByScope };
  }, [db.exclusions, db.leases, db.reservations]);

  useEffect(() => {
    if (!dhcpAgents.length) {
      setSelectedAgentId('');
      return;
    }
    if (!selectedAgentId || !dhcpAgents.some(agent => agent.id === selectedAgentId)) {
      setSelectedAgentId(dhcpAgents[0].id);
    }
  }, [dhcpAgents, selectedAgentId]);

  const scopes = useMemo(
    () =>
      db.scopes
        .filter(scope => !selectedAgentId || scope.serverId === selectedAgentId)
        .filter(scope => {
          const normalizedScopeQuery = scopeQuery.trim().toLowerCase();
          if (!normalizedScopeQuery) return true;
          return [
            scope.name,
            scope.description,
            scope.subnet,
            scope.defaultGateway,
            scope.startRange,
            scope.endRange,
            scope.state,
          ]
            .join(' ')
            .toLowerCase()
            .includes(normalizedScopeQuery);
        })
        .sort(compareScopesBySubnet),
    [db.scopes, selectedAgentId, scopeQuery]
  );
  const scope = scopes.find(s => s.id === selectedId) ?? scopes[0];
  useEffect(() => {
    if (!scopes.length) {
      setSelectedId(null);
      return;
    }
    if (!selectedId || !scopes.some(scope => scope.id === selectedId)) {
      setSelectedId(scopes[0].id);
    }
  }, [scopes, selectedId]);

  const scopeId = scope?.id;
  const leases = scopeId ? (scopeDataById.leasesByScope.get(scopeId) ?? []) : [];
  const exclusions = useMemo(
    () =>
      scopeId
        ? (scopeDataById.exclusionsByScope.get(scopeId) ?? EMPTY_DHCP_EXCLUSIONS)
        : EMPTY_DHCP_EXCLUSIONS,
    [scopeDataById, scopeId]
  );
  const reservations = scopeId ? (scopeDataById.reservationsByScope.get(scopeId) ?? []) : [];
  const dialogScopeAction = pendingScopeAction ?? visibleScopeAction;
  const dialogScopeIsDelete = dialogScopeAction === 'delete';
  const dialogScopeIsActive = scope?.state === 'Active';

  function openScopeAction(action: Exclude<PendingScopeAction, null>) {
    setVisibleScopeAction(action);
    setPendingScopeAction(action);
  }

  async function handleConfirmedScopeAction() {
    if (!scope || !pendingScopeAction) return;
    const action = pendingScopeAction;
    const scopeSubnet = scope.subnet;
    const wasActive = scope.state === 'Active';
    setScopeActionLoading(true);
    try {
      if (action === 'toggle') {
        await toggleScope(scope.id);
        toast.success(`${scopeSubnet} 作用域已${wasActive ? '停用' : '启用'}`);
      } else {
        await removeScope(scope.id);
        toast.success(`${scopeSubnet} 作用域已删除`);
      }
      setPendingScopeAction(null);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'DHCP 作用域操作失败');
    } finally {
      setScopeActionLoading(false);
    }
  }

  async function handleAgentRefresh() {
    if (!selectedAgentId) return;
    const agentName = dhcpAgents.find(agent => agent.id === selectedAgentId)?.name ?? '当前 Agent';
    const toastId = toast.loading(`${agentName} 正在同步`, taskToastOptions);
    setSyncingAgent(true);
    try {
      const task = await syncServer(selectedAgentId);
      await waitRefreshTask(task.id);
      await reloadDB();
      toast.success(`${agentName} 同步完成`, taskToastDoneOptionsFor(toastId));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : 'Agent 同步任务失败',
        taskToastDoneOptionsFor(toastId)
      );
    } finally {
      setSyncingAgent(false);
    }
  }

  async function handleScopeRefresh(scopeId: string) {
    const scopeItem = db.scopes.find(item => item.id === scopeId);
    const scopeLabel = scopeItem?.subnet ?? scopeItem?.name ?? '当前作用域';
    const toastId = toast.loading(`${scopeLabel} 作用域正在刷新`, taskToastOptions);
    setRefreshingScope(scopeId);
    try {
      const task = await refreshScope(scopeId);
      await waitRefreshTask(task.id);
      await reloadDB();
      toast.success(`${scopeLabel} 作用域刷新完成`, taskToastDoneOptionsFor(toastId));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : '作用域刷新任务失败',
        taskToastDoneOptionsFor(toastId)
      );
    } finally {
      setRefreshingScope(current => (current === scopeId ? null : current));
    }
  }

  async function openExportDialog() {
    setExportOpen(true);
    setExportLoading(true);
    try {
      const state = await reloadDB();
      const scopesForAgent = state.scopes.filter(
        item => !selectedAgentId || item.serverId === selectedAgentId
      );
      const scopeIds = new Set(scopesForAgent.map(item => item.id));
      setExportScopes(scopesForAgent);
      setExportExclusions(state.exclusions.filter(item => scopeIds.has(item.scopeId)));
      setExportLeases(state.leases.filter(item => scopeIds.has(item.scopeId)));
      setExportReservations(state.reservations.filter(item => scopeIds.has(item.scopeId)));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '读取导出数据失败');
    } finally {
      setExportLoading(false);
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex shrink-0 flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="truncate text-lg font-semibold" style={{ color: 'var(--zl-text)' }}>
            DHCP 管理
          </h1>
          <p className="mt-1 text-sm" style={{ color: 'var(--zl-text-muted)' }}>
            作用域与租约
          </p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <AgentScopeToolbar
            agents={dhcpAgents}
            value={selectedAgentId}
            refreshing={syncingAgent}
            canRefresh={canRefresh}
            onChange={value => {
              setSelectedAgentId(value);
              setSelectedId(null);
            }}
            onRefresh={() => void handleAgentRefresh()}
          />
          {canExport ? (
            <Button
              type="button"
              variant="outline"
              className="zl-action-button h-10 gap-2"
              disabled={!selectedAgentId || exportLoading}
              onClick={() => void openExportDialog()}
              style={{
                borderColor: 'rgba(59,130,246,0.38)',
                color: 'var(--zl-accent-text)',
                background: 'rgba(59,130,246,0.1)',
              }}
            >
              {exportLoading ? (
                <RefreshCw className="h-4 w-4 animate-spin" />
              ) : (
                <Download className="h-4 w-4" />
              )}
              导出
            </Button>
          ) : null}
        </div>
      </div>
      <div className="grid min-h-0 flex-1 grid-cols-12 gap-6">
        <section
          className="zl-card-hover col-span-12 flex min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-card lg:col-span-4 xl:col-span-3"
          style={{ boxShadow: 'var(--shadow-card)' }}
        >
          <div className="flex shrink-0 items-center justify-between border-b border-border px-4 py-3">
            <div className="flex items-center gap-2">
              <Network className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">作用域</h2>
            </div>
            {canManageDhcp ? (
              <AddScopeDialog serverId={selectedAgentId} existingScopes={scopes} />
            ) : null}
          </div>
          <div className="shrink-0 border-b border-border px-4 py-3">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="搜索作用域"
                className="h-9 w-full pl-8"
                value={scopeQuery}
                onChange={event => setScopeQuery(event.target.value)}
              />
            </div>
          </div>
          <ul className="zonelease-scroll-area divide-y divide-border">
            {scopes.map(s => {
              const active = s.id === scope?.id;
              return (
                <li
                  key={s.id}
                  className={`px-4 py-3 cursor-pointer ${active ? 'bg-accent' : 'hover:bg-muted/60'}`}
                  onClick={() => setSelectedId(s.id)}
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="min-w-0">
                      <div className="text-sm font-medium truncate">{s.name}</div>
                      <div className="text-xs text-muted-foreground font-mono">{s.subnet}</div>
                    </div>
                    <Badge
                      variant={s.state === 'Active' ? 'default' : 'secondary'}
                      className={
                        s.state === 'Active'
                          ? 'bg-success text-success-foreground hover:bg-success'
                          : ''
                      }
                    >
                      {s.state}
                    </Badge>
                  </div>
                </li>
              );
            })}
            {scopes.length === 0 ? (
              <li className="px-4 py-10 text-center text-sm text-muted-foreground">
                未找到匹配作用域
              </li>
            ) : null}
          </ul>
        </section>

        <section className="col-span-12 flex min-h-0 flex-col gap-6 lg:col-span-8 xl:col-span-9">
          {scope ? (
            <div
              className="zl-card-hover rounded-xl border border-border bg-card p-5"
              style={{ boxShadow: 'var(--shadow-card)' }}
            >
              <div className="flex items-start justify-between gap-4 flex-wrap">
                <div>
                  <h2 className="text-base font-semibold">{scope.name}</h2>
                  <p className="mt-1 text-xs text-muted-foreground">
                    <span className="font-mono">{scope.subnet}</span>
                    <span className="mx-1">·</span>
                    <span>{displayText(scope.description)}</span>
                  </p>
                </div>
                {canRefresh || canManageDhcp ? (
                  <div className="flex flex-wrap gap-2">
                    {canRefresh ? (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={refreshingScope === scope.id}
                        onClick={() => void handleScopeRefresh(scope.id)}
                      >
                        <RefreshCw
                          className={`h-3.5 w-3.5 mr-1.5 ${refreshingScope === scope.id ? 'animate-spin' : ''}`}
                        />
                        刷新
                      </Button>
                    ) : null}
                    {canManageDhcp ? (
                      <>
                        <EditScopeDialog scope={scope} />
                        <Button
                          size="sm"
                          variant="outline"
                          className={
                            scope.state === 'Active'
                              ? 'border-amber-500/35 bg-amber-500/10 text-amber-700 hover:bg-amber-500/16 hover:text-amber-800 dark:text-amber-200 dark:hover:text-amber-100'
                              : undefined
                          }
                          onClick={() => openScopeAction('toggle')}
                        >
                          <Power className="h-3.5 w-3.5 mr-1.5" />
                          {scope.state === 'Active' ? '停用' : '启用'}
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          className="text-destructive hover:text-destructive"
                          onClick={() => openScopeAction('delete')}
                        >
                          <Trash2 className="h-3.5 w-3.5 mr-1.5" /> 删除
                        </Button>
                      </>
                    ) : null}
                  </div>
                ) : null}
              </div>
              <div className="mt-4 grid grid-cols-1 gap-4 text-sm sm:grid-cols-2 xl:grid-cols-4">
                <Stat label="状态" value={scope.state} />
                <Stat label="默认网关" value={displayText(scope.defaultGateway ?? '')} />
                <Stat
                  label="租期"
                  value={formatLeaseDuration(
                    scope.leaseDurationSeconds ?? scope.leaseDurationHours * 3600
                  )}
                />
                <Stat label="地址范围" value={scopeRangeText(scope.startRange, scope.endRange)} />
              </div>
            </div>
          ) : (
            <div
              className="zl-card-hover rounded-xl border border-border bg-card p-5"
              style={{ boxShadow: 'var(--shadow-card)' }}
            >
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div>
                  <h2 className="text-base font-semibold">未选择作用域</h2>
                  <p className="mt-1 text-xs text-muted-foreground">
                    当前 Agent 暂无 DHCP 作用域快照
                  </p>
                </div>
              </div>
            </div>
          )}

          <div
            className="zl-card-hover flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-border bg-card"
            style={{ boxShadow: 'var(--shadow-card)' }}
          >
            <DhcpScopeDetailsTabs
              scopeId={scope?.id}
              scopeStartRange={scope?.startRange}
              scopeEndRange={scope?.endRange}
              canManageDhcp={canManageDhcp}
              leases={leases}
              reservations={reservations}
              exclusions={exclusions}
            />
          </div>
        </section>
      </div>
      <DhcpConfirmDialog
        open={pendingScopeAction !== null}
        title={
          dialogScopeIsDelete
            ? '删除 DHCP 作用域'
            : dialogScopeIsActive
              ? '停用 DHCP 作用域'
              : '启用 DHCP 作用域'
        }
        description={
          dialogScopeIsDelete
            ? `将删除作用域 ${scope?.name ?? ''}`
            : `将${dialogScopeIsActive ? '停用' : '启用'}作用域 ${scope?.name ?? ''}`
        }
        confirmText={dialogScopeIsDelete ? '删除' : dialogScopeIsActive ? '停用' : '启用'}
        tone={dialogScopeIsDelete ? 'destructive' : dialogScopeIsActive ? 'warning' : 'default'}
        destructive={dialogScopeIsDelete}
        loading={scopeActionLoading}
        onOpenChange={open => {
          if (!open) setPendingScopeAction(null);
          if (open && pendingScopeAction) setVisibleScopeAction(pendingScopeAction);
        }}
        onConfirm={() => void handleConfirmedScopeAction()}
      />
      <DhcpScopeExportDialog
        open={exportOpen}
        loading={exportLoading}
        scopes={exportScopes}
        exclusions={exportExclusions}
        leases={exportLeases}
        reservations={exportReservations}
        onClose={() => setExportOpen(false)}
      />
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-0.5 text-sm font-medium">{value}</div>
    </div>
  );
}

function compareScopesBySubnet(
  a: { subnet: string; name: string },
  b: { subnet: string; name: string }
) {
  const subnetCompare = compareIPv4(scopeNetwork(a.subnet), scopeNetwork(b.subnet));
  if (subnetCompare !== 0) return subnetCompare;
  const maskCompare = scopePrefixLength(a.subnet) - scopePrefixLength(b.subnet);
  if (maskCompare !== 0) return maskCompare;
  return a.name.localeCompare(b.name, 'zh-Hans-CN', { numeric: true, sensitivity: 'base' });
}

function scopeNetwork(subnet: string) {
  return subnet.split('/')[0]?.trim() ?? '';
}

function scopePrefixLength(subnet: string) {
  const value = Number(subnet.split('/')[1]?.trim());
  return Number.isFinite(value) ? value : 0;
}

function compareIPv4(a: string, b: string) {
  const left = ipv4Parts(a);
  const right = ipv4Parts(b);
  if (!left || !right) {
    return a.localeCompare(b, 'zh-Hans-CN', { numeric: true, sensitivity: 'base' });
  }
  for (let i = 0; i < 4; i += 1) {
    const diff = left[i] - right[i];
    if (diff !== 0) return diff;
  }
  return 0;
}

function ipv4Parts(value: string) {
  const parts = value.split('.').map(part => Number(part));
  if (parts.length !== 4 || parts.some(part => !Number.isInteger(part) || part < 0 || part > 255)) {
    return null;
  }
  return parts;
}

function formatLeaseDuration(seconds: number) {
  if (seconds === -1) return '无限制';
  const totalSeconds = Number.isFinite(seconds) && seconds > 0 ? Math.round(seconds) : 0;
  if (totalSeconds <= 0) return '-';
  const totalMinutes = Math.max(1, Math.round(totalSeconds / 60));
  const days = Math.floor(totalMinutes / 1440);
  const remainingAfterDays = totalMinutes % 1440;
  const hours = Math.floor(remainingAfterDays / 60);
  const minutes = remainingAfterDays % 60;
  if (days > 0) return `${days} 天 ${hours} 时 ${minutes} 分`;
  if (hours > 0) return `${hours} 时 ${minutes} 分`;
  return `${minutes} 分`;
}

function displayText(value: string) {
  return value.trim() || '-';
}

function scopeRangeText(startRange: string, endRange: string) {
  if (!startRange.trim() && !endRange.trim()) return '-';
  return `${displayText(startRange)} - ${displayText(endRange)}`;
}
