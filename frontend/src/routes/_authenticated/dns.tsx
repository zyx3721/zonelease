import { createFileRoute } from '@tanstack/react-router';
import { useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { AppTooltip } from '@/components/app-tooltip';
import { AgentScopeToolbar } from '@/components/agent-scope-toolbar';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Download, Trash2, Globe, Search, RefreshCw, SquarePen } from 'lucide-react';
import { DnsAddRecordDialog } from '@/features/dns/DnsAddRecordDialog';
import { DnsAddZoneDialog } from '@/features/dns/DnsAddZoneDialog';
import { DnsDeleteConfirmDialog } from '@/features/dns/DnsDeleteConfirmDialog';
import { DnsEditRecordDialog } from '@/features/dns/DnsEditRecordDialog';
import { DnsZoneExportDialog } from '@/features/dns/DnsZoneExportDialog';
import { SortableRecordHeader } from '@/features/dns/SortableRecordHeader';
import { recordTypeTone, zoneTone } from '@/features/dns/colors';
import { sortRecords, sortZones } from '@/features/dns/sort';
import type { DnsSortKey, PendingDnsDelete, SortDirection } from '@/features/dns/types';
import { getStoredUser, userHasPermission } from '@/lib/auth';
import {
  useDB,
  removeZone,
  removeRecord,
  refreshZone,
  reloadDB,
  syncServer,
  waitRefreshTask,
  type DnsRecord,
  type DnsRecordType,
} from '@/lib/dns-dhcp-store';
import { taskToastDoneOptionsFor, taskToastOptions } from '@/lib/task-toast';

export const Route = createFileRoute('/_authenticated/dns')({
  component: DnsPage,
});

const RECORD_RENDER_BATCH_SIZE = 200;

function DnsPage() {
  const db = useDB({ includeDns: true });
  const canManageDns = userHasPermission(getStoredUser(), 'dns.manage');
  const [selectedZone, setSelectedZone] = useState<string | null>(db.zones[0]?.id ?? null);
  const [query, setQuery] = useState('');
  const [zoneQuery, setZoneQuery] = useState('');
  const [refreshingZone, setRefreshingZone] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<PendingDnsDelete | null>(null);
  const [editingRecord, setEditingRecord] = useState<DnsRecord | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [exportLoading, setExportLoading] = useState(false);
  const [exportZones, setExportZones] = useState(db.zones);
  const [exportRecords, setExportRecords] = useState(db.records);
  const dnsAgents = useMemo(() => db.servers.filter(server => server.role === 'DNS'), [db.servers]);
  const [selectedAgentId, setSelectedAgentId] = useState(dnsAgents[0]?.id ?? '');
  const [syncingAgent, setSyncingAgent] = useState(false);
  const [recordSort, setRecordSort] = useState<{ key: DnsSortKey; direction: SortDirection }>({
    key: 'name',
    direction: null,
  });
  const [visibleRecordCount, setVisibleRecordCount] = useState(RECORD_RENDER_BATCH_SIZE);

  const recordCountByZone = useMemo(() => {
    const counts = new Map<string, number>();
    for (const record of db.records) {
      counts.set(record.zoneId, (counts.get(record.zoneId) ?? 0) + 1);
    }
    return counts;
  }, [db.records]);

  useEffect(() => {
    if (!dnsAgents.length) {
      setSelectedAgentId('');
      return;
    }
    if (!selectedAgentId || !dnsAgents.some(agent => agent.id === selectedAgentId)) {
      setSelectedAgentId(dnsAgents[0].id);
    }
  }, [dnsAgents, selectedAgentId]);

  const sortedZones = useMemo(() => {
    const normalizedQuery = zoneQuery.trim().toLowerCase();
    return sortZones(
      db.zones
        .filter(zone => !selectedAgentId || zone.serverId === selectedAgentId)
        .filter(zone => {
          if (!normalizedQuery) return true;
          return [zone.name, zone.type, zone.reverse ? '反向' : '正向']
            .join(' ')
            .toLowerCase()
            .includes(normalizedQuery);
        })
    );
  }, [db.zones, selectedAgentId, zoneQuery]);
  const zone = sortedZones.find(z => z.id === selectedZone) ?? sortedZones[0];

  useEffect(() => {
    if (!sortedZones.length) {
      setSelectedZone(null);
      return;
    }
    if (!selectedZone || !sortedZones.some(z => z.id === selectedZone)) {
      setSelectedZone(sortedZones[0].id);
    }
  }, [sortedZones, selectedZone]);
  const records = useMemo(() => {
    const items = db.records
      .filter(r => r.zoneId === zone?.id)
      .filter(r => (query ? r.name.includes(query) || r.value.includes(query) : true));
    return sortRecords(items, recordSort);
  }, [db.records, zone?.id, query, recordSort]);
  const visibleRecords = useMemo(
    () => records.slice(0, visibleRecordCount),
    [records, visibleRecordCount]
  );
  const hasMoreRecords = visibleRecordCount < records.length;

  useEffect(() => {
    setVisibleRecordCount(RECORD_RENDER_BATCH_SIZE);
  }, [zone?.id, query, recordSort]);

  useEffect(() => {
    setQuery('');
  }, [zone?.id]);

  async function handleZoneRefresh(zoneId: string) {
    const zoneName = db.zones.find(item => item.id === zoneId)?.name ?? '当前区域';
    const toastId = toast.loading(`${zoneName} 正在刷新`, taskToastOptions);
    setRefreshingZone(zoneId);
    try {
      const task = await refreshZone(zoneId);
      await waitRefreshTask(task.id);
      await reloadDB({ includeDns: true });
      toast.success(`${zoneName} 刷新完成`, taskToastDoneOptionsFor(toastId));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : '区域刷新任务失败',
        taskToastDoneOptionsFor(toastId)
      );
    } finally {
      setRefreshingZone(current => (current === zoneId ? null : current));
    }
  }

  async function handleAgentRefresh() {
    if (!selectedAgentId) return;
    const agentName = dnsAgents.find(agent => agent.id === selectedAgentId)?.name ?? '当前 Agent';
    const toastId = toast.loading(`${agentName} 正在同步`, taskToastOptions);
    setSyncingAgent(true);
    try {
      const task = await syncServer(selectedAgentId);
      await waitRefreshTask(task.id);
      await reloadDB({ includeDns: true });
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

  async function openExportDialog() {
    setExportOpen(true);
    setExportLoading(true);
    try {
      const state = await reloadDB({ includeDns: true });
      const zonesForAgent = state.zones.filter(
        item => !selectedAgentId || item.serverId === selectedAgentId
      );
      const zoneIds = new Set(zonesForAgent.map(item => item.id));
      setExportZones(zonesForAgent);
      setExportRecords(state.records.filter(item => zoneIds.has(item.zoneId)));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '读取导出数据失败');
    } finally {
      setExportLoading(false);
    }
  }

  function toggleRecordSort(key: DnsSortKey) {
    setRecordSort(current => {
      if (current.key !== key) return { key, direction: 'asc' };
      if (current.direction === null) return { key, direction: 'asc' };
      if (current.direction === 'asc') return { key, direction: 'desc' };
      return { key, direction: null };
    });
  }

  async function confirmDelete() {
    if (!pendingDelete) return;
    setDeleting(true);
    try {
      if (pendingDelete.kind === 'zone') {
        await removeZone(pendingDelete.id);
        toast.success(`${pendingDelete.name} 已删除`);
      } else {
        await removeRecord(pendingDelete.id);
        toast.success(`${pendingDelete.name} ${pendingDelete.detail} 记录已删除`);
      }
      setPendingDelete(null);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '删除失败');
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex shrink-0 flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="truncate text-lg font-semibold" style={{ color: 'var(--zl-text)' }}>
            DNS 管理
          </h1>
          <p className="mt-1 text-sm" style={{ color: 'var(--zl-text-muted)' }}>
            区域与记录
          </p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <AgentScopeToolbar
            agents={dnsAgents}
            value={selectedAgentId}
            refreshing={syncingAgent}
            onChange={value => {
              setSelectedAgentId(value);
              setSelectedZone(null);
            }}
            onRefresh={() => void handleAgentRefresh()}
          />
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
        </div>
      </div>
      <div className="grid min-h-0 flex-1 grid-cols-12 gap-6">
        {/* Zones list */}
        <section
          className="zl-card-hover col-span-12 flex min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-card lg:col-span-4 xl:col-span-3"
          style={{ boxShadow: 'var(--shadow-card)' }}
        >
          <div className="flex shrink-0 items-center justify-between border-b border-border px-4 py-3">
            <div className="flex items-center gap-2">
              <Globe className="h-4 w-4 text-muted-foreground" />
              <h2 className="text-sm font-semibold">DNS 区域</h2>
            </div>
            {canManageDns ? <DnsAddZoneDialog serverId={selectedAgentId} /> : null}
          </div>
          <div className="shrink-0 border-b border-border px-4 py-3">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="搜索区域"
                className="h-9 w-full pl-8"
                value={zoneQuery}
                onChange={event => setZoneQuery(event.target.value)}
              />
            </div>
          </div>
          <ul className="zonelease-scroll-area divide-y divide-border">
            {sortedZones.map(z => {
              const active = z.id === zone?.id;
              const count = recordCountByZone.get(z.id) ?? 0;
              const tone = zoneTone(z);
              return (
                <li
                  key={z.id}
                  className={`px-4 py-3 cursor-pointer flex items-center justify-between gap-2 ${
                    active ? 'bg-accent' : 'hover:bg-muted/60'
                  }`}
                  style={{
                    background: active ? tone.background : undefined,
                  }}
                  onClick={() => setSelectedZone(z.id)}
                >
                  <div className="min-w-0">
                    <div className="text-sm font-medium truncate" style={{ color: tone.text }}>
                      {z.name}
                    </div>
                    <div className="text-xs" style={{ color: tone.muted }}>
                      {z.type} · {z.reverse ? '反向' : '正向'} · {count} 记录
                    </div>
                  </div>
                  <div className="flex items-center gap-1">
                    {canManageDns ? (
                      <>
                        <AppTooltip label="刷新区域记录" placement="top">
                          <Button
                            size="icon"
                            variant="ghost"
                            className="h-7 w-7"
                            disabled={refreshingZone === z.id}
                            style={{ color: tone.text }}
                            onClick={e => {
                              e.stopPropagation();
                              void handleZoneRefresh(z.id);
                            }}
                          >
                            <RefreshCw
                              className={`h-3.5 w-3.5 ${refreshingZone === z.id ? 'animate-spin' : ''}`}
                            />
                          </Button>
                        </AppTooltip>
                        <AppTooltip label="删除区域" placement="top">
                          <Button
                            size="icon"
                            variant="ghost"
                            className="h-7 w-7 hover:text-destructive"
                            style={{ color: tone.text }}
                            onClick={e => {
                              e.stopPropagation();
                              setPendingDelete({
                                kind: 'zone',
                                id: z.id,
                                name: z.name,
                                detail: `${z.type} · ${z.reverse ? '反向' : '正向'} · ${count} 记录`,
                              });
                            }}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </AppTooltip>
                      </>
                    ) : null}
                  </div>
                </li>
              );
            })}
            {sortedZones.length === 0 ? (
              <li className="px-4 py-10 text-center text-sm text-muted-foreground">
                未找到匹配区域
              </li>
            ) : null}
          </ul>
        </section>

        {/* Records */}
        <section
          className="zl-card-hover col-span-12 flex min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-card lg:col-span-8 xl:col-span-9"
          style={{ boxShadow: 'var(--shadow-card)' }}
        >
          <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-border px-5 py-4">
            <div className="flex-1 min-w-[180px]">
              <h2 className="text-sm font-semibold">{zone?.name ?? '—'}</h2>
              <p className="text-xs text-muted-foreground">
                {zone ? `${zone.type} · 动态更新: ${zone.dynamicUpdate}` : '未选择区域'}
              </p>
            </div>
            <div className="relative">
              <Search className="h-4 w-4 absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="搜索记录..."
                className="pl-8 w-56"
                value={query}
                onChange={e => setQuery(e.target.value)}
              />
            </div>
            {zone && canManageDns ? (
              zone.reverse ? (
                <AppTooltip label="反向区域不支持在此新建记录" placement="bottom">
                  <span>
                    <Button size="sm" disabled aria-label="反向区域不支持新建记录">
                      新建记录
                    </Button>
                  </span>
                </AppTooltip>
              ) : (
                <DnsAddRecordDialog zoneId={zone.id} />
              )
            ) : null}
          </div>

          <div className="zonelease-scroll-area">
            <table className="w-full text-sm">
              <thead className="bg-muted/50 text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <SortableRecordHeader
                    label="名称"
                    sortKey="name"
                    active={recordSort.key === 'name' ? recordSort.direction : null}
                    onSort={toggleRecordSort}
                  />
                  <SortableRecordHeader
                    label="类型"
                    sortKey="type"
                    active={recordSort.key === 'type' ? recordSort.direction : null}
                    onSort={toggleRecordSort}
                  />
                  <SortableRecordHeader
                    label="值"
                    sortKey="value"
                    active={recordSort.key === 'value' ? recordSort.direction : null}
                    onSort={toggleRecordSort}
                  />
                  <th className="px-5 py-2.5 text-center font-medium">TTL</th>
                  <th className="px-5 py-2.5 text-center font-medium">更新时间</th>
                  <th className="px-5 py-2.5" />
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {visibleRecords.map(r => {
                  const recordEditable = zone?.reverse
                    ? r.type === 'PTR'
                    : r.type === 'A' || r.type === 'CNAME';
                  const editDisabledLabel = zone?.reverse
                    ? '仅支持编辑 PTR 记录'
                    : '仅支持编辑 A 和 CNAME 记录';
                  const deleteDisabledLabel = zone?.reverse
                    ? '仅支持删除 PTR 记录'
                    : '仅支持删除 A 和 CNAME 记录';
                  return (
                    <tr key={r.id} className="hover:bg-muted/40">
                      <td className="px-5 py-3 text-center font-medium">{r.name}</td>
                      <td className="px-5 py-3 text-center">
                        <RecordTypeBadge type={r.type} />
                      </td>
                      <td className="px-5 py-3 text-center font-mono text-xs">{r.value}</td>
                      <td className="px-5 py-3 text-center text-muted-foreground">{r.ttl}s</td>
                      <td className="px-5 py-3 text-center text-xs text-muted-foreground">
                        {new Date(r.updatedAt).toLocaleString()}
                      </td>
                      <td className="px-5 py-3 text-center">
                        {canManageDns ? (
                          <div className="flex items-center justify-center gap-1">
                            <AppTooltip
                              label={recordEditable ? '编辑记录' : editDisabledLabel}
                              placement="top"
                            >
                              <span className={recordEditable ? undefined : 'cursor-not-allowed'}>
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  className="h-7 w-7 text-muted-foreground hover:text-info disabled:pointer-events-none disabled:opacity-45"
                                  aria-label={recordEditable ? '编辑记录' : editDisabledLabel}
                                  disabled={!recordEditable}
                                  onClick={() => setEditingRecord(r)}
                                >
                                  <SquarePen className="h-3.5 w-3.5" />
                                </Button>
                              </span>
                            </AppTooltip>
                            <AppTooltip
                              label={recordEditable ? '删除记录' : deleteDisabledLabel}
                              placement="top"
                            >
                              <span className={recordEditable ? undefined : 'cursor-not-allowed'}>
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  className="h-7 w-7 text-muted-foreground hover:text-destructive disabled:pointer-events-none disabled:opacity-45"
                                  aria-label={recordEditable ? '删除记录' : deleteDisabledLabel}
                                  disabled={!recordEditable}
                                  onClick={() =>
                                    setPendingDelete({
                                      kind: 'record',
                                      id: r.id,
                                      name: r.name,
                                      detail: `${r.type} ${r.value}`,
                                    })
                                  }
                                >
                                  <Trash2 className="h-3.5 w-3.5" />
                                </Button>
                              </span>
                            </AppTooltip>
                          </div>
                        ) : null}
                      </td>
                    </tr>
                  );
                })}
                {hasMoreRecords ? (
                  <tr>
                    <td colSpan={6} className="px-5 py-4">
                      <div className="flex flex-wrap items-center justify-center gap-3 text-sm text-muted-foreground">
                        <span>
                          已显示 {visibleRecords.length} / {records.length} 条
                        </span>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            setVisibleRecordCount(count => count + RECORD_RENDER_BATCH_SIZE)
                          }
                        >
                          加载更多
                        </Button>
                      </div>
                    </td>
                  </tr>
                ) : null}
                {records.length === 0 && (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-5 py-10 text-center text-sm text-muted-foreground"
                    >
                      暂无记录
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </section>
      </div>

      <DnsDeleteConfirmDialog
        target={pendingDelete}
        busy={deleting}
        onClose={() => setPendingDelete(null)}
        onConfirm={() => void confirmDelete()}
      />
      <DnsEditRecordDialog record={editingRecord} onClose={() => setEditingRecord(null)} />
      <DnsZoneExportDialog
        open={exportOpen}
        loading={exportLoading}
        zones={exportZones}
        records={exportRecords}
        onClose={() => setExportOpen(false)}
      />
    </div>
  );
}

function RecordTypeBadge({ type }: { type: DnsRecordType }) {
  const tone = recordTypeTone(type);
  return (
    <Badge
      variant="outline"
      style={{
        color: tone.color,
        borderColor: tone.border,
        background: tone.background,
      }}
    >
      {type}
    </Badge>
  );
}
