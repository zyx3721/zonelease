import type { ExportColumn } from '@/lib/export-data';
import type { DhcpExclusion, DhcpLease, DhcpReservation, DhcpScope } from '@/lib/dns-dhcp-store';

export type DhcpExportTarget = 'scopes' | 'leases' | 'reservations' | 'exclusions';
export type DhcpExportScope = 'all' | 'active' | 'inactive' | 'custom';

export type DhcpExportTargetOption = {
  value: DhcpExportTarget;
  label: string;
  fileLabel: string;
  countLabel: string;
};

export type DhcpScopeExportRow = {
  name: string;
  subnet: string;
  state: string;
  startRange: string;
  endRange: string;
  range: string;
  defaultGateway: string;
  leaseDuration: string;
  description: string;
  exclusionCount: number;
  leaseCount: number;
  reservationCount: number;
  syncStatus: string;
  syncedAt: string;
  lastError: string;
};

export type DhcpLeaseExportRow = {
  scopeName: string;
  scopeSubnet: string;
  ip: string;
  mac: string;
  hostname: string;
  expiresAt: string;
};

export type DhcpReservationExportRow = {
  scopeName: string;
  scopeSubnet: string;
  ip: string;
  mac: string;
  name: string;
  description: string;
};

export type DhcpExclusionExportRow = {
  scopeName: string;
  scopeSubnet: string;
  startIp: string;
  endIp: string;
};

export type DhcpExportRow =
  | DhcpScopeExportRow
  | DhcpLeaseExportRow
  | DhcpReservationExportRow
  | DhcpExclusionExportRow;

type AsyncBuildOptions = {
  signal?: AbortSignal;
  batchSize?: number;
};

export const dhcpExportTargetOptions: DhcpExportTargetOption[] = [
  { value: 'scopes', label: '作用域', fileLabel: 'DHCP作用域', countLabel: '个作用域' },
  { value: 'leases', label: '租约', fileLabel: 'DHCP租约', countLabel: '条租约' },
  {
    value: 'reservations',
    label: '保留地址',
    fileLabel: 'DHCP保留地址',
    countLabel: '条保留地址',
  },
  {
    value: 'exclusions',
    label: '排除范围',
    fileLabel: 'DHCP排除范围',
    countLabel: '条排除范围',
  },
];

export const dhcpExportScopes: Array<{ value: DhcpExportScope; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'active', label: '启用' },
  { value: 'inactive', label: '停用' },
  { value: 'custom', label: '自定义' },
];

export const dhcpScopeExportColumns: ExportColumn<DhcpScopeExportRow>[] = [
  { id: 'name', header: '作用域名称', value: item => item.name },
  { id: 'subnet', header: '子网', value: item => item.subnet },
  { id: 'state', header: '状态', value: item => item.state },
  { id: 'startRange', header: '起始地址', value: item => item.startRange },
  { id: 'endRange', header: '结束地址', value: item => item.endRange },
  { id: 'range', header: '地址范围', value: item => item.range },
  { id: 'defaultGateway', header: '默认网关', value: item => item.defaultGateway },
  { id: 'leaseDuration', header: '租期', value: item => item.leaseDuration },
  { id: 'description', header: '描述', value: item => item.description },
  { id: 'exclusionCount', header: '排除范围数', value: item => item.exclusionCount },
  { id: 'leaseCount', header: '租约数', value: item => item.leaseCount },
  { id: 'reservationCount', header: '保留地址数', value: item => item.reservationCount },
  { id: 'syncStatus', header: '同步状态', value: item => item.syncStatus },
  { id: 'syncedAt', header: '同步时间', value: item => item.syncedAt },
  { id: 'lastError', header: '错误信息', value: item => item.lastError },
];

export const dhcpLeaseExportColumns: ExportColumn<DhcpLeaseExportRow>[] = [
  { id: 'scopeName', header: '作用域名称', value: item => item.scopeName },
  { id: 'scopeSubnet', header: '作用域子网', value: item => item.scopeSubnet },
  { id: 'ip', header: 'IP 地址', value: item => item.ip },
  { id: 'hostname', header: '名称', value: item => item.hostname },
  { id: 'mac', header: 'MAC', value: item => item.mac },
  { id: 'expiresAt', header: '租用截止日期', value: item => item.expiresAt },
];

export const dhcpReservationExportColumns: ExportColumn<DhcpReservationExportRow>[] = [
  { id: 'scopeName', header: '作用域名称', value: item => item.scopeName },
  { id: 'scopeSubnet', header: '作用域子网', value: item => item.scopeSubnet },
  { id: 'ip', header: 'IP 地址', value: item => item.ip },
  { id: 'mac', header: 'MAC', value: item => item.mac },
  { id: 'name', header: '名称', value: item => item.name },
  { id: 'description', header: '描述', value: item => item.description },
];

export const dhcpExclusionExportColumns: ExportColumn<DhcpExclusionExportRow>[] = [
  { id: 'scopeName', header: '作用域名称', value: item => item.scopeName },
  { id: 'scopeSubnet', header: '作用域子网', value: item => item.scopeSubnet },
  { id: 'startIp', header: '起始 IP 地址', value: item => item.startIp },
  { id: 'endIp', header: '结束 IP 地址', value: item => item.endIp },
];

export function dhcpExportTargetOption(target: DhcpExportTarget) {
  return dhcpExportTargetOptions.find(item => item.value === target) ?? dhcpExportTargetOptions[0];
}

export function dhcpExportColumns(target: DhcpExportTarget): ExportColumn<DhcpExportRow>[] {
  switch (target) {
    case 'leases':
      return dhcpLeaseExportColumns as ExportColumn<DhcpExportRow>[];
    case 'reservations':
      return dhcpReservationExportColumns as ExportColumn<DhcpExportRow>[];
    case 'exclusions':
      return dhcpExclusionExportColumns as ExportColumn<DhcpExportRow>[];
    default:
      return dhcpScopeExportColumns as ExportColumn<DhcpExportRow>[];
  }
}

export function buildDhcpExportRows(
  target: DhcpExportTarget,
  scopes: DhcpScope[],
  exclusions: DhcpExclusion[],
  leases: DhcpLease[],
  reservations: DhcpReservation[]
): DhcpExportRow[] {
  switch (target) {
    case 'leases':
      return buildLeaseRows(scopes, leases);
    case 'reservations':
      return buildReservationRows(scopes, reservations);
    case 'exclusions':
      return buildExclusionRows(scopes, exclusions);
    default:
      return buildScopeRows(scopes, exclusions, leases, reservations);
  }
}

export async function buildDhcpExportRowsAsync(
  target: DhcpExportTarget,
  scopes: DhcpScope[],
  exclusions: DhcpExclusion[],
  leases: DhcpLease[],
  reservations: DhcpReservation[],
  options: AsyncBuildOptions = {}
): Promise<DhcpExportRow[]> {
  if (target === 'scopes') {
    return buildScopeRows(scopes, exclusions, leases, reservations);
  }
  const batchSize = options.batchSize ?? 800;
  switch (target) {
    case 'leases':
      return buildScopedRowsAsync(scopes, leases, buildLeaseRow, batchSize, options.signal);
    case 'reservations':
      return buildScopedRowsAsync(
        scopes,
        reservations,
        buildReservationRow,
        batchSize,
        options.signal
      );
    case 'exclusions':
      return buildScopedRowsAsync(scopes, exclusions, buildExclusionRow, batchSize, options.signal);
    default:
      return [];
  }
}

function buildScopeRows(
  scopes: DhcpScope[],
  exclusions: DhcpExclusion[],
  leases: DhcpLease[],
  reservations: DhcpReservation[]
): DhcpScopeExportRow[] {
  const exclusionCountByScope = countByScope(exclusions);
  const leaseCountByScope = countByScope(leases);
  const reservationCountByScope = countByScope(reservations);

  return sortScopes(scopes).map(item => ({
    name: item.name,
    subnet: item.subnet,
    state: stateText(item.state),
    startRange: displayText(item.startRange),
    endRange: displayText(item.endRange),
    range: scopeRangeText(item.startRange, item.endRange),
    defaultGateway: displayText(item.defaultGateway ?? ''),
    leaseDuration: formatLeaseDuration(item.leaseDurationSeconds ?? item.leaseDurationHours * 3600),
    description: displayText(item.description),
    exclusionCount: exclusionCountByScope.get(item.id) ?? 0,
    leaseCount: leaseCountByScope.get(item.id) ?? 0,
    reservationCount: reservationCountByScope.get(item.id) ?? 0,
    syncStatus: displayText(item.syncStatus ?? ''),
    syncedAt: formatDateTime(item.lastSyncedAt),
    lastError: displayText(item.lastError ?? ''),
  }));
}

function buildLeaseRows(scopes: DhcpScope[], leases: DhcpLease[]): DhcpLeaseExportRow[] {
  const scopeById = scopeMap(scopes);
  return leases
    .filter(item => scopeById.has(item.scopeId))
    .sort((left, right) => compareScopedIP(left.scopeId, left.ip, right.scopeId, right.ip, scopes))
    .map(item => {
      const scope = scopeById.get(item.scopeId);
      return buildLeaseRow(item, scope);
    });
}

function buildReservationRows(
  scopes: DhcpScope[],
  reservations: DhcpReservation[]
): DhcpReservationExportRow[] {
  const scopeById = scopeMap(scopes);
  return reservations
    .filter(item => scopeById.has(item.scopeId))
    .sort((left, right) => compareScopedIP(left.scopeId, left.ip, right.scopeId, right.ip, scopes))
    .map(item => {
      const scope = scopeById.get(item.scopeId);
      return buildReservationRow(item, scope);
    });
}

function buildExclusionRows(
  scopes: DhcpScope[],
  exclusions: DhcpExclusion[]
): DhcpExclusionExportRow[] {
  const scopeById = scopeMap(scopes);
  return exclusions
    .filter(item => scopeById.has(item.scopeId))
    .sort((left, right) =>
      compareScopedIP(left.scopeId, left.startIp, right.scopeId, right.startIp, scopes)
    )
    .map(item => {
      const scope = scopeById.get(item.scopeId);
      return buildExclusionRow(item, scope);
    });
}

async function buildScopedRowsAsync<
  T extends { scopeId: string; ip?: string; startIp?: string },
  R extends DhcpExportRow,
>(
  scopes: DhcpScope[],
  items: T[],
  rowBuilder: (item: T, scope?: DhcpScope) => R,
  batchSize: number,
  signal?: AbortSignal
): Promise<R[]> {
  const sortedScopes = sortScopes(scopes);
  const scopeById = scopeMap(sortedScopes);
  const itemsByScope = groupItemsByScope(items, scopeById);
  const rows: R[] = [];
  let processed = 0;

  for (const scope of sortedScopes) {
    throwIfAborted(signal);
    const scopeItems = itemsByScope.get(scope.id) ?? [];
    scopeItems.sort((left, right) => compareItemIP(left, right));
    for (const item of scopeItems) {
      rows.push(rowBuilder(item, scope));
      processed += 1;
      if (processed % batchSize === 0) {
        await nextFrame();
        throwIfAborted(signal);
      }
    }
  }
  return rows;
}

function groupItemsByScope<T extends { scopeId: string }>(
  items: T[],
  scopeById: Map<string, DhcpScope>
) {
  const grouped = new Map<string, T[]>();
  for (const item of items) {
    if (!scopeById.has(item.scopeId)) continue;
    const group = grouped.get(item.scopeId);
    if (group) {
      group.push(item);
    } else {
      grouped.set(item.scopeId, [item]);
    }
  }
  return grouped;
}

function buildLeaseRow(item: DhcpLease, scope?: DhcpScope): DhcpLeaseExportRow {
  return {
    scopeName: scope?.name ?? '-',
    scopeSubnet: scope?.subnet ?? '-',
    ip: item.ip,
    mac: item.mac,
    hostname: leaseName(item),
    expiresAt: leaseExpiryText(item),
  };
}

function buildReservationRow(item: DhcpReservation, scope?: DhcpScope): DhcpReservationExportRow {
  return {
    scopeName: scope?.name ?? '-',
    scopeSubnet: scope?.subnet ?? '-',
    ip: item.ip,
    mac: item.mac,
    name: displayText(item.name),
    description: displayText(item.description),
  };
}

function buildExclusionRow(item: DhcpExclusion, scope?: DhcpScope): DhcpExclusionExportRow {
  return {
    scopeName: scope?.name ?? '-',
    scopeSubnet: scope?.subnet ?? '-',
    startIp: item.startIp,
    endIp: item.endIp,
  };
}

function compareItemIP(
  left: { ip?: string; startIp?: string },
  right: { ip?: string; startIp?: string }
) {
  return compareIPv4(left.ip ?? left.startIp ?? '', right.ip ?? right.startIp ?? '');
}

function nextFrame() {
  return new Promise<void>(resolve => {
    window.setTimeout(resolve, 0);
  });
}

function throwIfAborted(signal?: AbortSignal) {
  if (signal?.aborted) {
    throw new DOMException('Export preparation aborted', 'AbortError');
  }
}

function sortScopes(scopes: DhcpScope[]) {
  return [...scopes].sort(compareScopesBySubnet);
}

function scopeMap(scopes: DhcpScope[]) {
  return new Map(scopes.map(item => [item.id, item]));
}

function countByScope(items: Array<{ scopeId: string }>) {
  const counts = new Map<string, number>();
  for (const item of items) {
    counts.set(item.scopeId, (counts.get(item.scopeId) ?? 0) + 1);
  }
  return counts;
}

function compareScopedIP(
  leftScopeId: string,
  leftIP: string,
  rightScopeId: string,
  rightIP: string,
  scopes: DhcpScope[]
) {
  const order = new Map(sortScopes(scopes).map((item, index) => [item.id, index]));
  const scopeDiff = (order.get(leftScopeId) ?? 0) - (order.get(rightScopeId) ?? 0);
  if (scopeDiff !== 0) return scopeDiff;
  return compareIPv4(leftIP, rightIP);
}

function compareScopesBySubnet(a: DhcpScope, b: DhcpScope) {
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

function stateText(value: string) {
  return value === 'Active' ? '启用' : '停用';
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

function formatDateTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
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
  return state === 'reservedactive' || state === 'reserved' || state === 'activereservation';
}

function isReservedInactiveLease(lease: DhcpLease) {
  const state = lease.state.trim().toLowerCase();
  const expiresAt = lease.expiresAt.trim().toLowerCase();
  return state === 'reservedinactive' || state === 'inactivereservation' || expiresAt === '不活动';
}

function looksLikeLeaseExpiry(value: string) {
  return /^\d{4}\/\d{1,2}\/\d{1,2}\s+\d{1,2}:\d{2}:\d{2}$/.test(value.trim());
}

function formatLeaseExpiry(value: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  if (date.getUTCFullYear() <= 1) return '-';
  const pad = (item: number) => String(item).padStart(2, '0');
  return `${date.getFullYear()}/${date.getMonth() + 1}/${date.getDate()} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}
