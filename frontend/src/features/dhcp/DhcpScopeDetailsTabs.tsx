import { useDeferredValue, useEffect, useMemo, useState } from 'react';
import { ArrowDown, ArrowUp, ArrowUpDown, BookmarkPlus, Search, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { AppTooltip } from '@/components/app-tooltip';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { removeExclusion, removeLease, removeReservation } from '@/lib/dns-dhcp-store';
import { AddExclusionDialog } from './AddExclusionDialog';
import { AddReservationFromLeaseDialog } from './AddReservationFromLeaseDialog';
import { DhcpConfirmDialog } from './DhcpConfirmDialog';
import { EditReservationDialog } from './EditReservationDialog';
import type { DhcpExclusion, DhcpLease, DhcpReservation } from '@/lib/dns-dhcp-store';

const LEASE_RENDER_BATCH_SIZE = 200;

type SortDirection = 'asc' | 'desc' | null;
type DhcpLeaseSortKey = 'ip' | 'name' | 'mac' | 'expiresAt';
type DhcpReservationSortKey = 'ip' | 'mac' | 'name' | 'description';
type PendingDhcpDelete =
  | { kind: 'lease'; id: string; name: string; detail: string }
  | { kind: 'reservation'; id: string; name: string; detail: string }
  | { kind: 'exclusion'; id: string; name: string; detail: string };

interface DhcpScopeDetailsTabsProps {
  scopeId?: string;
  scopeStartRange?: string;
  scopeEndRange?: string;
  canManageDhcp: boolean;
  leases: DhcpLease[];
  reservations: DhcpReservation[];
  exclusions: DhcpExclusion[];
}

export function DhcpScopeDetailsTabs({
  scopeId,
  scopeStartRange = '',
  scopeEndRange = '',
  canManageDhcp,
  leases,
  reservations,
  exclusions,
}: DhcpScopeDetailsTabsProps) {
  const [activeTab, setActiveTab] = useState('leases');
  const [query, setQuery] = useState('');
  const [pendingReservationLease, setPendingReservationLease] = useState<DhcpLease | null>(null);
  const [pendingDelete, setPendingDelete] = useState<PendingDhcpDelete | null>(null);
  const [visibleDelete, setVisibleDelete] = useState<PendingDhcpDelete | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [visibleLeaseCount, setVisibleLeaseCount] = useState(LEASE_RENDER_BATCH_SIZE);
  const [leaseSort, setLeaseSort] = useState<{
    key: DhcpLeaseSortKey;
    direction: SortDirection;
  }>({ key: 'ip', direction: null });
  const [reservationSort, setReservationSort] = useState<{
    key: DhcpReservationSortKey;
    direction: SortDirection;
  }>({ key: 'ip', direction: null });
  const normalizedQuery = useDeferredValue(query).trim().toLowerCase();

  const sortedLeases = useMemo(() => sortDhcpLeases(leases, leaseSort), [leases, leaseSort]);

  const filteredLeases = useMemo(() => {
    if (!normalizedQuery) return sortedLeases;
    return sortedLeases.filter(lease =>
      [lease.ip, lease.mac, leaseName(lease), leaseExpiryText(lease)]
        .join(' ')
        .toLowerCase()
        .includes(normalizedQuery)
    );
  }, [sortedLeases, normalizedQuery]);

  const sortedReservations = useMemo(
    () => sortDhcpReservations(reservations, reservationSort),
    [reservations, reservationSort]
  );

  const filteredReservations = useMemo(() => {
    if (!normalizedQuery) return sortedReservations;
    return sortedReservations.filter(reservation =>
      [reservation.ip, reservation.mac, reservation.name, reservation.description]
        .join(' ')
        .toLowerCase()
        .includes(normalizedQuery)
    );
  }, [sortedReservations, normalizedQuery]);

  const sortedExclusions = useMemo(
    () => [...exclusions].sort((left, right) => compareIPv4(left.startIp, right.startIp)),
    [exclusions]
  );

  const filteredExclusions = useMemo(() => {
    if (!normalizedQuery) return sortedExclusions;
    return sortedExclusions.filter(exclusion =>
      [exclusion.startIp, exclusion.endIp].join(' ').toLowerCase().includes(normalizedQuery)
    );
  }, [sortedExclusions, normalizedQuery]);
  const reservedIps = useMemo(
    () => new Set(reservations.map(reservation => reservation.ip.trim())),
    [reservations]
  );
  const visibleLeases = useMemo(
    () => filteredLeases.slice(0, visibleLeaseCount),
    [filteredLeases, visibleLeaseCount]
  );
  const hasMoreLeases = visibleLeaseCount < filteredLeases.length;
  const deleteTarget = pendingDelete ?? visibleDelete;

  useEffect(() => {
    setVisibleLeaseCount(LEASE_RENDER_BATCH_SIZE);
  }, [scopeId, normalizedQuery, activeTab]);

  useEffect(() => {
    setQuery('');
  }, [scopeId]);

  function toggleLeaseSort(key: DhcpLeaseSortKey) {
    setLeaseSort(current => toggleSortState(current, key));
  }

  function toggleReservationSort(key: DhcpReservationSortKey) {
    setReservationSort(current => toggleSortState(current, key));
  }

  function confirmDelete() {
    if (!pendingDelete) return;
    setDeleting(true);
    const target = pendingDelete;
    const action =
      target.kind === 'lease'
        ? removeLease(target.id)
        : target.kind === 'reservation'
          ? removeReservation(target.id)
          : removeExclusion(target.id);
    void action
      .then(() => {
        const label =
          target.kind === 'lease'
            ? '租约'
            : target.kind === 'reservation'
              ? '保留地址'
              : '排除范围';
        toast.success(`${target.name} ${label}已删除`);
        setPendingDelete(null);
      })
      .catch(error => {
        toast.error(error instanceof Error ? error.message : '删除失败');
      })
      .finally(() => setDeleting(false));
  }

  function openDelete(target: PendingDhcpDelete) {
    setVisibleDelete(target);
    setPendingDelete(target);
  }

  function openReservationConfirm(lease: DhcpLease) {
    setPendingReservationLease(lease);
  }

  return (
    <Tabs value={activeTab} onValueChange={setActiveTab} className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-border px-4 py-3">
        <TabsList>
          <TabsTrigger value="leases">租约 ({leases.length})</TabsTrigger>
          <TabsTrigger value="reservations">保留 ({reservations.length})</TabsTrigger>
          <TabsTrigger value="exclusions">排除 ({exclusions.length})</TabsTrigger>
        </TabsList>
        <div className="ml-auto flex flex-wrap items-center justify-end gap-3">
          {activeTab === 'exclusions' && scopeId && canManageDhcp ? (
            <AddExclusionDialog
              scopeId={scopeId}
              scopeStartRange={scopeStartRange}
              scopeEndRange={scopeEndRange}
              exclusions={exclusions}
            />
          ) : null}
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="搜索租约、保留或排除..."
              className="w-56 pl-8"
              value={query}
              onChange={event => setQuery(event.target.value)}
            />
          </div>
        </div>
      </div>
      <TabsContent value="leases" className="m-0 min-h-0 flex-1">
        <div className="zl-hidden-scrollbar h-full overflow-auto">
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10 bg-muted/50 text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <SortableDhcpHeader
                  label="IP 地址"
                  sortKey="ip"
                  active={leaseSort.key === 'ip' ? leaseSort.direction : null}
                  onSort={toggleLeaseSort}
                />
                <SortableDhcpHeader
                  label="名称"
                  sortKey="name"
                  active={leaseSort.key === 'name' ? leaseSort.direction : null}
                  onSort={toggleLeaseSort}
                />
                <SortableDhcpHeader
                  label="MAC"
                  sortKey="mac"
                  active={leaseSort.key === 'mac' ? leaseSort.direction : null}
                  onSort={toggleLeaseSort}
                />
                <SortableDhcpHeader
                  label="租用截止日期"
                  sortKey="expiresAt"
                  active={leaseSort.key === 'expiresAt' ? leaseSort.direction : null}
                  onSort={toggleLeaseSort}
                />
                <th className="px-5 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {visibleLeases.map(lease => {
                const reserved = reservedIps.has(lease.ip.trim());
                return (
                  <tr key={lease.id} className="hover:bg-muted/40">
                    <td className="px-5 py-3 text-center font-mono">{lease.ip}</td>
                    <td className="px-5 py-3 text-center">{leaseName(lease)}</td>
                    <td className="px-5 py-3 text-center font-mono text-xs">{lease.mac}</td>
                    <td className="px-5 py-3 text-center font-mono text-xs">
                      {leaseExpiryText(lease)}
                    </td>
                    <td className="px-5 py-3 text-center">
                      {canManageDhcp ? (
                        <div className="flex items-center justify-center gap-1">
                          <AppTooltip
                            label={reserved ? '该 IP 已存在保留地址' : '添加到保留'}
                            placement="top"
                          >
                            <Button
                              size="icon"
                              variant="ghost"
                              className="h-7 w-7 text-muted-foreground hover:text-primary"
                              aria-label={reserved ? '该 IP 已存在保留地址' : '添加到保留'}
                              disabled={reserved}
                              onClick={() => openReservationConfirm(lease)}
                            >
                              <BookmarkPlus className="h-3.5 w-3.5" />
                            </Button>
                          </AppTooltip>
                          <AppTooltip
                            label={reserved ? '请从保留列表中删除该保留地址' : '释放租约'}
                            placement="top"
                            align={reserved ? 'end' : 'center'}
                          >
                            <Button
                              size="icon"
                              variant="ghost"
                              className="h-7 w-7 text-muted-foreground hover:text-destructive"
                              aria-label={reserved ? '请从保留列表中删除该保留地址' : '释放租约'}
                              disabled={reserved}
                              onClick={() =>
                                openDelete({
                                  kind: 'lease',
                                  id: lease.id,
                                  name: lease.ip,
                                  detail: `${leaseName(lease)} · ${lease.mac}`,
                                })
                              }
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                          </AppTooltip>
                        </div>
                      ) : null}
                    </td>
                  </tr>
                );
              })}
              {hasMoreLeases ? (
                <tr>
                  <td colSpan={5} className="px-5 py-4">
                    <div className="flex flex-wrap items-center justify-center gap-3 text-sm text-muted-foreground">
                      <span>
                        已显示 {visibleLeases.length} / {filteredLeases.length} 条
                      </span>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() =>
                          setVisibleLeaseCount(count => count + LEASE_RENDER_BATCH_SIZE)
                        }
                      >
                        加载更多
                      </Button>
                    </div>
                  </td>
                </tr>
              ) : null}
              {filteredLeases.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-5 py-10 text-center text-sm text-muted-foreground">
                    {leases.length === 0 ? '暂无租约' : '未找到匹配租约'}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </TabsContent>
      <TabsContent value="reservations" className="m-0 min-h-0 flex-1">
        <div className="zl-hidden-scrollbar h-full overflow-auto">
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10 bg-muted/50 text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <SortableDhcpHeader
                  label="IP"
                  sortKey="ip"
                  active={reservationSort.key === 'ip' ? reservationSort.direction : null}
                  onSort={toggleReservationSort}
                />
                <SortableDhcpHeader
                  label="MAC"
                  sortKey="mac"
                  active={reservationSort.key === 'mac' ? reservationSort.direction : null}
                  onSort={toggleReservationSort}
                />
                <SortableDhcpHeader
                  label="名称"
                  sortKey="name"
                  active={reservationSort.key === 'name' ? reservationSort.direction : null}
                  onSort={toggleReservationSort}
                />
                <SortableDhcpHeader
                  label="描述"
                  sortKey="description"
                  active={reservationSort.key === 'description' ? reservationSort.direction : null}
                  onSort={toggleReservationSort}
                />
                <th className="px-5 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {filteredReservations.map(reservation => (
                <tr key={reservation.id} className="hover:bg-muted/40">
                  <td className="px-5 py-3 text-center font-mono">{reservation.ip}</td>
                  <td className="px-5 py-3 text-center font-mono text-xs">{reservation.mac}</td>
                  <td className="px-5 py-3 text-center">{reservation.name || '-'}</td>
                  <td className="px-5 py-3 text-center text-muted-foreground">
                    {reservation.description || '-'}
                  </td>
                  <td className="px-5 py-3 text-center">
                    {canManageDhcp ? (
                      <div className="flex items-center justify-center gap-1">
                        <EditReservationDialog reservation={reservation} />
                        <AppTooltip label="删除保留地址" placement="top">
                          <Button
                            size="icon"
                            variant="ghost"
                            className="h-7 w-7 text-muted-foreground hover:text-destructive"
                            aria-label="删除保留地址"
                            onClick={() =>
                              openDelete({
                                kind: 'reservation',
                                id: reservation.id,
                                name: reservation.ip,
                                detail: `${reservation.name || '-'} · ${reservation.mac}`,
                              })
                            }
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </AppTooltip>
                      </div>
                    ) : null}
                  </td>
                </tr>
              ))}
              {filteredReservations.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-5 py-10 text-center text-sm text-muted-foreground">
                    {reservations.length === 0 ? '暂无保留' : '未找到匹配保留'}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </TabsContent>
      <TabsContent value="exclusions" className="m-0 min-h-0 flex-1">
        <div className="zl-hidden-scrollbar h-full overflow-auto">
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10 bg-muted/50 text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-5 py-2.5 text-center font-medium">起始 IP 地址</th>
                <th className="px-5 py-2.5 text-center font-medium">结束 IP 地址</th>
                <th className="px-5 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {filteredExclusions.map(exclusion => (
                <tr key={exclusion.id} className="hover:bg-muted/40">
                  <td className="px-5 py-3 text-center font-mono">{exclusion.startIp}</td>
                  <td className="px-5 py-3 text-center font-mono">{exclusion.endIp}</td>
                  <td className="px-5 py-3 text-center">
                    {canManageDhcp ? (
                      <AppTooltip label="删除排除范围" placement="top">
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-7 w-7 text-muted-foreground hover:text-destructive"
                          aria-label="删除排除范围"
                          onClick={() =>
                            openDelete({
                              kind: 'exclusion',
                              id: exclusion.id,
                              name: `${exclusion.startIp} - ${exclusion.endIp}`,
                              detail: 'DHCP 排除范围',
                            })
                          }
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </AppTooltip>
                    ) : null}
                  </td>
                </tr>
              ))}
              {filteredExclusions.length === 0 ? (
                <tr>
                  <td colSpan={3} className="px-5 py-10 text-center text-sm text-muted-foreground">
                    {exclusions.length === 0 ? '暂无排除范围' : '未找到匹配排除范围'}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </TabsContent>
      <AddReservationFromLeaseDialog
        scopeId={scopeId}
        lease={pendingReservationLease}
        initialName={pendingReservationLease ? leaseName(pendingReservationLease) : ''}
        onClose={() => setPendingReservationLease(null)}
      />
      <DhcpConfirmDialog
        open={pendingDelete !== null}
        title={
          deleteTarget?.kind === 'lease'
            ? '释放 DHCP 租约'
            : deleteTarget?.kind === 'reservation'
              ? '删除 DHCP 保留地址'
              : '删除 DHCP 排除范围'
        }
        description={deleteTarget ? `${deleteTarget.name} ${deleteTarget.detail}` : ''}
        confirmText={deleteTarget?.kind === 'lease' ? '释放' : '删除'}
        tone="destructive"
        destructive
        loading={deleting}
        onOpenChange={open => {
          if (deleting) return;
          if (!open) setPendingDelete(null);
          if (open && pendingDelete) setVisibleDelete(pendingDelete);
        }}
        onConfirm={confirmDelete}
      />
    </Tabs>
  );
}

function SortableDhcpHeader<T extends string>({
  label,
  sortKey,
  active,
  onSort,
}: {
  label: string;
  sortKey: T;
  active: SortDirection;
  onSort: (key: T) => void;
}) {
  const Icon = active === 'asc' ? ArrowUp : active === 'desc' ? ArrowDown : ArrowUpDown;
  const ariaSort = active === 'asc' ? 'ascending' : active === 'desc' ? 'descending' : 'none';
  return (
    <th className="px-5 py-2.5 text-center font-medium" aria-sort={ariaSort}>
      <button
        type="button"
        className="zl-action-button mx-auto inline-flex h-8 items-center justify-center gap-1.5 rounded-md border px-2.5 text-xs font-medium"
        style={{
          background: active ? 'rgba(59,130,246,0.12)' : 'transparent',
          borderColor: active ? 'rgba(59,130,246,0.35)' : 'transparent',
          color: active ? 'var(--zl-accent-text)' : 'var(--zl-text-muted)',
        }}
        onClick={() => onSort(sortKey)}
      >
        {label}
        <Icon className="h-3.5 w-3.5" />
      </button>
    </th>
  );
}

function toggleSortState<T extends string>(current: { key: T; direction: SortDirection }, key: T) {
  if (current.key !== key) return { key, direction: 'asc' as const };
  if (current.direction === null) return { key, direction: 'asc' as const };
  if (current.direction === 'asc') return { key, direction: 'desc' as const };
  return { key, direction: null };
}

function sortDhcpLeases(
  leases: DhcpLease[],
  sort: { key: DhcpLeaseSortKey; direction: SortDirection }
) {
  const items = [...leases];
  if (!sort.direction) return items.sort(compareLeaseByIP);
  const factor = sort.direction === 'asc' ? 1 : -1;
  return items.sort((left, right) => {
    const result =
      sort.key === 'ip'
        ? compareIPv4(left.ip, right.ip)
        : naturalCompare(leaseSortValue(left, sort.key), leaseSortValue(right, sort.key));
    return result === 0 ? compareLeaseByIP(left, right) : result * factor;
  });
}

function sortDhcpReservations(
  reservations: DhcpReservation[],
  sort: { key: DhcpReservationSortKey; direction: SortDirection }
) {
  const items = [...reservations];
  if (!sort.direction) return items.sort(compareReservationByIP);
  const factor = sort.direction === 'asc' ? 1 : -1;
  return items.sort((left, right) => {
    const result =
      sort.key === 'ip'
        ? compareIPv4(left.ip, right.ip)
        : naturalCompare(
            reservationSortValue(left, sort.key),
            reservationSortValue(right, sort.key)
          );
    return result === 0 ? compareReservationByIP(left, right) : result * factor;
  });
}

function compareLeaseByIP(left: DhcpLease, right: DhcpLease) {
  return compareIPv4(left.ip, right.ip);
}

function compareReservationByIP(left: DhcpReservation, right: DhcpReservation) {
  return compareIPv4(left.ip, right.ip);
}

function leaseSortValue(lease: DhcpLease, key: DhcpLeaseSortKey) {
  if (key === 'name') return leaseName(lease);
  if (key === 'mac') return lease.mac;
  if (key === 'expiresAt') return leaseExpiryText(lease);
  return lease.ip;
}

function reservationSortValue(reservation: DhcpReservation, key: DhcpReservationSortKey) {
  if (key === 'mac') return reservation.mac;
  if (key === 'name') return reservation.name;
  if (key === 'description') return reservation.description;
  return reservation.ip;
}

function compareIPv4(left: string, right: string) {
  const leftParts = ipv4Parts(left);
  const rightParts = ipv4Parts(right);
  if (!leftParts || !rightParts) return naturalCompare(left, right);
  for (let index = 0; index < 4; index += 1) {
    const diff = leftParts[index] - rightParts[index];
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

function naturalCompare(left: string, right: string) {
  return left.trim().localeCompare(right.trim(), 'zh-Hans-CN', {
    numeric: true,
    sensitivity: 'base',
  });
}

function leaseName(lease: DhcpLease) {
  if (looksLikeLeaseExpiry(lease.hostname)) {
    return displayText(lease.state === 'Active' ? '' : lease.state);
  }
  return displayText(lease.hostname);
}

function leaseExpiryText(lease: DhcpLease) {
  if (isReservedInactiveLease(lease)) {
    return '保留 (不活动的)';
  }
  if (isReservedActiveLease(lease)) {
    return '保留 (活动的)';
  }
  if (looksLikeLeaseExpiry(lease.hostname)) {
    return lease.hostname;
  }
  return formatLeaseExpiry(lease.expiresAt);
}

function isReservedActiveLease(lease: DhcpLease) {
  const state = lease.state.trim().toLowerCase();
  const expiresAt = lease.expiresAt.trim().toLowerCase();
  if (expiresAt === 'never' || expiresAt === '永不过期') return true;
  return state === 'reservedactive' || state === 'reserved' || state.includes('reservation');
}

function isReservedInactiveLease(lease: DhcpLease) {
  const state = lease.state.trim().toLowerCase();
  const expiresAt = lease.expiresAt.trim().toLowerCase();
  return state === 'reservedinactive' || expiresAt === '不活动';
}

function looksLikeLeaseExpiry(value: string) {
  return /^\d{4}\/\d{1,2}\/\d{1,2}\s+\d{1,2}:\d{2}:\d{2}$/.test(value.trim());
}

function displayText(value: string) {
  return value.trim() || '-';
}

function formatLeaseExpiry(value: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  if (date.getUTCFullYear() <= 1) return '-';
  const pad = (item: number) => String(item).padStart(2, '0');
  return `${date.getFullYear()}/${date.getMonth() + 1}/${date.getDate()} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}
