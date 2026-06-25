import { useDeferredValue, useMemo, useState } from 'react';
import { Search, Trash2 } from 'lucide-react';
import { AppTooltip } from '@/components/app-tooltip';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { removeExclusion, removeLease, removeReservation } from '@/lib/dns-dhcp-store';
import { AddExclusionDialog } from './AddExclusionDialog';
import { AddReservationDialog } from './AddReservationDialog';
import { EditReservationDialog } from './EditReservationDialog';
import type { DhcpExclusion, DhcpLease, DhcpReservation } from '@/lib/dns-dhcp-store';

interface DhcpScopeDetailsTabsProps {
  scopeId?: string;
  canManageDhcp: boolean;
  leases: DhcpLease[];
  reservations: DhcpReservation[];
  exclusions: DhcpExclusion[];
}

export function DhcpScopeDetailsTabs({
  scopeId,
  canManageDhcp,
  leases,
  reservations,
  exclusions,
}: DhcpScopeDetailsTabsProps) {
  const [activeTab, setActiveTab] = useState('leases');
  const [query, setQuery] = useState('');
  const normalizedQuery = useDeferredValue(query).trim().toLowerCase();

  const filteredLeases = useMemo(
    () =>
      normalizedQuery
        ? leases.filter(lease =>
            [lease.ip, lease.mac, leaseName(lease), leaseExpiryText(lease)]
              .join(' ')
              .toLowerCase()
              .includes(normalizedQuery)
          )
        : leases,
    [leases, normalizedQuery]
  );

  const filteredReservations = useMemo(
    () =>
      normalizedQuery
        ? reservations.filter(reservation =>
            [reservation.ip, reservation.mac, reservation.name, reservation.description]
              .join(' ')
              .toLowerCase()
              .includes(normalizedQuery)
          )
        : reservations,
    [reservations, normalizedQuery]
  );

  const filteredExclusions = useMemo(
    () =>
      normalizedQuery
        ? exclusions.filter(exclusion =>
            [exclusion.startIp, exclusion.endIp].join(' ').toLowerCase().includes(normalizedQuery)
          )
        : exclusions,
    [exclusions, normalizedQuery]
  );

  return (
    <Tabs value={activeTab} onValueChange={setActiveTab} className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-border px-4 py-3">
        <TabsList>
          <TabsTrigger value="leases">租约 ({leases.length})</TabsTrigger>
          <TabsTrigger value="reservations">保留 ({reservations.length})</TabsTrigger>
          <TabsTrigger value="exclusions">排除 ({exclusions.length})</TabsTrigger>
        </TabsList>
        <div className="ml-auto flex flex-wrap items-center justify-end gap-3">
          {activeTab === 'reservations' && scopeId && canManageDhcp ? (
            <AddReservationDialog scopeId={scopeId} />
          ) : null}
          {activeTab === 'exclusions' && scopeId && canManageDhcp ? (
            <AddExclusionDialog scopeId={scopeId} />
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
                <th className="px-5 py-2.5 text-center font-medium">IP 地址</th>
                <th className="px-5 py-2.5 text-center font-medium">名称</th>
                <th className="px-5 py-2.5 text-center font-medium">MAC</th>
                <th className="px-5 py-2.5 text-center font-medium">租用截止日期</th>
                <th className="px-5 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {filteredLeases.map(lease => (
                <tr key={lease.id} className="hover:bg-muted/40">
                  <td className="px-5 py-3 text-center font-mono">{lease.ip}</td>
                  <td className="px-5 py-3 text-center">{leaseName(lease)}</td>
                  <td className="px-5 py-3 text-center font-mono text-xs">{lease.mac}</td>
                  <td className="px-5 py-3 text-center font-mono text-xs">
                    {leaseExpiryText(lease)}
                  </td>
                  <td className="px-5 py-3 text-center">
                    {canManageDhcp ? (
                      <AppTooltip label="释放租约" placement="top">
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-7 w-7 text-muted-foreground hover:text-destructive"
                          aria-label="释放租约"
                          onClick={() => void removeLease(lease.id)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </AppTooltip>
                    ) : null}
                  </td>
                </tr>
              ))}
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
                <th className="px-5 py-2.5 text-center font-medium">IP</th>
                <th className="px-5 py-2.5 text-center font-medium">MAC</th>
                <th className="px-5 py-2.5 text-center font-medium">名称</th>
                <th className="px-5 py-2.5 text-center font-medium">描述</th>
                <th className="px-5 py-2.5" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {filteredReservations.map(reservation => (
                <tr key={reservation.id} className="hover:bg-muted/40">
                  <td className="px-5 py-3 text-center font-mono">{reservation.ip}</td>
                  <td className="px-5 py-3 text-center font-mono text-xs">{reservation.mac}</td>
                  <td className="px-5 py-3 text-center">{reservation.name}</td>
                  <td className="px-5 py-3 text-center text-muted-foreground">
                    {reservation.description}
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
                            onClick={() => void removeReservation(reservation.id)}
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
                          onClick={() => void removeExclusion(exclusion.id)}
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
    </Tabs>
  );
}

function leaseName(lease: DhcpLease) {
  if (looksLikeLeaseExpiry(lease.hostname)) {
    return displayText(lease.state === 'Active' ? '' : lease.state);
  }
  return displayText(lease.hostname);
}

function leaseExpiryText(lease: DhcpLease) {
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
  return state === 'reservedactive' || state === 'reserved' || state.includes('reservation');
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
