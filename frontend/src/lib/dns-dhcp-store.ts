import { useCallback, useEffect, useState } from 'react';
import { api } from './auth';
import { getBaseConfigSnapshot } from './branding';
import { emitNotificationRefresh } from './notifications';
import { onZoneLeaseRefresh } from './refresh';

export type DnsRecordType = 'A' | 'AAAA' | 'CNAME' | 'MX' | 'TXT' | 'PTR' | 'NS' | 'SRV';

export interface DnsRecord {
  id: string;
  zoneId: string;
  name: string;
  type: DnsRecordType;
  value: string;
  ttl: number;
  createPtr?: boolean;
  updatedAt: string;
  lastSyncedAt?: string;
}

export interface DnsRecordCreateResponse extends DnsRecord {
  relatedRecords?: DnsRecord[];
  warnings?: string[];
}

export interface DnsZone {
  id: string;
  name: string;
  type: 'Primary' | 'Secondary' | 'Stub';
  reverse: boolean;
  dynamicUpdate: 'None' | 'Secure' | 'Nonsecure';
  serverId: string;
  lastSyncedAt?: string;
  syncStatus?: string;
  lastError?: string;
}

export interface DnsZoneCreateResponse extends DnsZone {
  records?: DnsRecord[];
  warning?: string;
}

export interface DhcpScope {
  id: string;
  name: string;
  description: string;
  subnet: string;
  defaultGateway?: string;
  startRange: string;
  endRange: string;
  leaseDurationHours: number;
  leaseDurationSeconds?: number;
  state: 'Active' | 'Inactive';
  serverId: string;
  externalId?: string;
  lastSyncedAt?: string;
  syncStatus?: string;
  lastError?: string;
}

export interface DhcpExclusion {
  id: string;
  scopeId: string;
  startIp: string;
  endIp: string;
  externalId?: string;
  lastSyncedAt?: string;
}

export interface DhcpLease {
  id: string;
  scopeId: string;
  ip: string;
  mac: string;
  hostname: string;
  state: 'Active' | 'Expired' | 'Reserved' | 'ReservedActive' | 'ReservedInactive' | string;
  expiresAt: string;
}

export interface DhcpReservation {
  id: string;
  scopeId: string;
  ip: string;
  mac: string;
  name: string;
  description: string;
}

export interface ServerConfig {
  id: string;
  name: string;
  host: string;
  role: 'DNS' | 'DHCP';
  agentUrl: string;
  apiKey: string;
  tlsInsecure: boolean;
  status: 'Online' | 'Offline' | 'Unknown';
  failureCount?: number;
  lastChecked: string;
}

export interface AuditEntry {
  id: string;
  ts: string;
  user: string;
  action: string;
  target: string;
  module: 'DNS' | 'DHCP' | 'Server' | 'System';
  result: 'success' | 'failed';
  ipAddress?: string;
  detail?: string;
}

export interface RefreshTask {
  id: string;
  type: string;
  status: 'queued' | 'running' | 'completed' | 'failed' | string;
  payload?: Record<string, unknown> | string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  finishedAt?: string;
}

interface DB {
  servers: ServerConfig[];
  zones: DnsZone[];
  records: DnsRecord[];
  scopes: DhcpScope[];
  exclusions: DhcpExclusion[];
  leases: DhcpLease[];
  reservations: DhcpReservation[];
  audit: AuditEntry[];
}

const EVENT = 'zonelease-db-change';
const CACHE_TTL_MS = 1200;

let cachedFastDB: { value: DB; expiresAt: number } | null = null;
let cachedDNSDB: { value: DB; expiresAt: number } | null = null;
let pendingFastDB: Promise<DB> | null = null;
let pendingDNSDB: Promise<DB> | null = null;

function emptyDB(): DB {
  return {
    servers: [],
    zones: [],
    records: [],
    scopes: [],
    exclusions: [],
    leases: [],
    reservations: [],
    audit: [],
  };
}

function notifyChange() {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent(EVENT));
  }
}

function updateCachedDB(mutator: (db: DB) => DB) {
  if (cachedFastDB) {
    cachedFastDB = { ...cachedFastDB, value: mutator(cachedFastDB.value) };
  }
  if (cachedDNSDB) {
    cachedDNSDB = { ...cachedDNSDB, value: mutator(cachedDNSDB.value) };
  }
}

function recordFromResponse(response: DnsRecordCreateResponse): DnsRecord {
  const { relatedRecords: _relatedRecords, warnings: _warnings, ...record } = response;
  return record;
}

function ipv4ReverseZone(value: string): string | null {
  const parts = value.trim().split('.');
  if (parts.length !== 4) return null;
  const octets = parts.map(part => Number(part));
  if (octets.some(octet => !Number.isInteger(octet) || octet < 0 || octet > 255)) return null;
  return `${octets[2]}.${octets[1]}.${octets[0]}.in-addr.arpa`;
}

function ptrValueForRecord(zone: DnsZone, record: DnsRecord): string {
  const zoneName = zone.name.replace(/\.+$/, '');
  const recordName = record.name.trim().replace(/\.+$/, '');
  const value = !recordName || recordName === '@' ? zoneName : `${recordName}.${zoneName}`;
  return value.endsWith('.') ? value : `${value}.`;
}

function relatedPtrRecordIds(db: DB, record: DnsRecord): Set<string> {
  if (record.type !== 'A') return new Set();
  const zone = db.zones.find(item => item.id === record.zoneId);
  const reverseZoneName = ipv4ReverseZone(record.value);
  if (!zone || !reverseZoneName) return new Set();
  const reverseZone = db.zones.find(
    item =>
      item.serverId === zone.serverId && item.reverse && item.name.toLowerCase() === reverseZoneName
  );
  if (!reverseZone) return new Set();
  const ptrName = `${record.value.trim()}.`;
  const ptrValue = ptrValueForRecord(zone, record);
  return new Set(
    db.records
      .filter(
        item =>
          item.zoneId === reverseZone.id &&
          item.type === 'PTR' &&
          item.name === ptrName &&
          item.value.toLowerCase() === ptrValue.toLowerCase()
      )
      .map(item => item.id)
  );
}

async function mutate<T>(request: Promise<T>): Promise<T> {
  const result = await request;
  cachedFastDB = null;
  cachedDNSDB = null;
  notifyChange();
  return result;
}

function normalizeDB(db: Partial<DB>): DB {
  return {
    servers: db.servers ?? [],
    zones: db.zones ?? [],
    records: db.records ?? [],
    scopes: db.scopes ?? [],
    exclusions: db.exclusions ?? [],
    leases: db.leases ?? [],
    reservations: db.reservations ?? [],
    audit: db.audit ?? [],
  };
}

export function getDB(options: { includeDns?: boolean } = {}): Promise<DB> {
  const includeDns = options.includeDns === true;
  const cached = includeDns ? cachedDNSDB : cachedFastDB;
  if (cached && cached.expiresAt > Date.now()) {
    return Promise.resolve(cached.value);
  }
  const pending = includeDns ? pendingDNSDB : pendingFastDB;
  if (pending) {
    return pending;
  }
  const request = api<Partial<DB>>(includeDns ? '/api/state?includeDns=true' : '/api/state')
    .then(normalizeDB)
    .then(value => {
      const next = { value, expiresAt: Date.now() + CACHE_TTL_MS };
      if (includeDns) {
        cachedDNSDB = next;
      } else {
        cachedFastDB = next;
      }
      return value;
    })
    .finally(() => {
      if (includeDns) {
        pendingDNSDB = null;
      } else {
        pendingFastDB = null;
      }
    });
  if (includeDns) {
    pendingDNSDB = request;
  } else {
    pendingFastDB = request;
  }
  return request;
}

export async function reloadDB(options: { includeDns?: boolean } = {}) {
  if (options.includeDns === true) {
    cachedDNSDB = null;
    pendingDNSDB = null;
  } else {
    cachedFastDB = null;
    pendingFastDB = null;
  }
  const db = await getDB(options);
  notifyChange();
  return db;
}

export function addZone(z: Omit<DnsZone, 'id'>) {
  return api<DnsZoneCreateResponse>('/api/dns/zones', {
    method: 'POST',
    body: JSON.stringify(z),
  }).then(result => {
    const { records = [], warning: _warning, ...zone } = result;
    const recordIds = new Set(records.map(record => record.id));
    updateCachedDB(db => ({
      ...db,
      zones: [...db.zones.filter(item => item.id !== zone.id), zone],
      records: [
        ...db.records.filter(record => record.zoneId !== zone.id && !recordIds.has(record.id)),
        ...records,
      ],
    }));
    notifyChange();
    return result;
  });
}

export function removeZone(id: string) {
  return mutate(
    api<{ status: string }>(`/api/dns/zones/${encodeURIComponent(id)}`, { method: 'DELETE' })
  );
}

export function addRecord(r: Omit<DnsRecord, 'id' | 'updatedAt'>) {
  return api<DnsRecordCreateResponse>('/api/dns/records', {
    method: 'POST',
    body: JSON.stringify(r),
  }).then(result => {
    const records = [recordFromResponse(result), ...(result.relatedRecords ?? [])];
    const recordIds = new Set(records.map(record => record.id));
    updateCachedDB(db => ({
      ...db,
      records: [...db.records.filter(record => !recordIds.has(record.id)), ...records],
    }));
    notifyChange();
    return result;
  });
}

export function updateRecordValue(
  id: string,
  value: string,
  options: { createPtr?: boolean } = {}
) {
  return api<DnsRecordCreateResponse>(`/api/dns/records/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify({ value, ...options }),
  }).then(result => {
    const nextRecord = recordFromResponse(result);
    updateCachedDB(db => {
      const oldRecord = db.records.find(record => record.id === id);
      const oldRelatedIds = oldRecord ? relatedPtrRecordIds(db, oldRecord) : new Set<string>();
      const records = [nextRecord, ...(result.relatedRecords ?? [])];
      const recordIds = new Set(records.map(record => record.id));
      return {
        ...db,
        records: [
          ...db.records.filter(
            record => record.id !== id && !oldRelatedIds.has(record.id) && !recordIds.has(record.id)
          ),
          ...records,
        ],
      };
    });
    notifyChange();
    return result;
  });
}

export function removeRecord(id: string) {
  return api<{ status: string }>(`/api/dns/records/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  }).then(result => {
    updateCachedDB(db => {
      const deletedRecord = db.records.find(item => item.id === id);
      const relatedIds = deletedRecord ? relatedPtrRecordIds(db, deletedRecord) : new Set<string>();
      return {
        ...db,
        records: db.records.filter(record => record.id !== id && !relatedIds.has(record.id)),
      };
    });
    notifyChange();
    return result;
  });
}

export function addScope(s: Omit<DhcpScope, 'id'>) {
  return mutate(api<DhcpScope>('/api/dhcp/scopes', { method: 'POST', body: JSON.stringify(s) }));
}

export function updateScope(id: string, s: DhcpScope) {
  return mutate(
    api<DhcpScope>(`/api/dhcp/scopes/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(s),
    })
  );
}

export function toggleScope(id: string) {
  return mutate(
    api<{ status: string }>(`/api/dhcp/scopes/${encodeURIComponent(id)}/toggle`, { method: 'POST' })
  );
}

export function removeScope(id: string) {
  return mutate(
    api<{ status: string }>(`/api/dhcp/scopes/${encodeURIComponent(id)}`, { method: 'DELETE' })
  );
}

export function addExclusion(e: Omit<DhcpExclusion, 'id'>) {
  return mutate(
    api<DhcpExclusion>('/api/dhcp/exclusions', { method: 'POST', body: JSON.stringify(e) })
  );
}

export function removeExclusion(id: string) {
  return mutate(
    api<{ status: string }>(`/api/dhcp/exclusions/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
  );
}

export function addReservation(r: Omit<DhcpReservation, 'id'>) {
  return api<DhcpReservation>('/api/dhcp/reservations', {
    method: 'POST',
    body: JSON.stringify(r),
  }).then(result => {
    pendingFastDB = null;
    pendingDNSDB = null;
    updateCachedDB(db => ({
      ...db,
      reservations: [...db.reservations.filter(item => item.id !== result.id), result],
      leases: db.leases.map(lease =>
        lease.scopeId === result.scopeId && lease.ip === result.ip
          ? { ...lease, hostname: result.name, state: 'ReservedInactive', expiresAt: 'never' }
          : lease
      ),
    }));
    notifyChange();
    return result;
  });
}

export function updateReservation(id: string, r: DhcpReservation) {
  return api<DhcpReservation>(`/api/dhcp/reservations/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(r),
  }).then(result => {
    pendingFastDB = null;
    pendingDNSDB = null;
    updateCachedDB(db => ({
      ...db,
      reservations: db.reservations.map(item => (item.id === id ? result : item)),
      leases: db.leases.map(lease =>
        lease.scopeId === result.scopeId && lease.ip === result.ip
          ? { ...lease, hostname: result.name }
          : lease
      ),
    }));
    notifyChange();
    return result;
  });
}

export function removeReservation(id: string) {
  return api<{ status: string }>(`/api/dhcp/reservations/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  }).then(result => {
    pendingFastDB = null;
    pendingDNSDB = null;
    updateCachedDB(db => {
      const reservation = db.reservations.find(item => item.id === id);
      return {
        ...db,
        reservations: db.reservations.filter(item => item.id !== id),
        leases: reservation
          ? db.leases.filter(
              lease => !(lease.scopeId === reservation.scopeId && lease.ip === reservation.ip)
            )
          : db.leases,
      };
    });
    notifyChange();
    return result;
  });
}

export function removeLease(id: string) {
  return mutate(
    api<{ status: string }>(`/api/dhcp/leases/${encodeURIComponent(id)}`, { method: 'DELETE' })
  );
}

export function addServer(
  s: Omit<ServerConfig, 'id' | 'host' | 'status' | 'failureCount' | 'lastChecked'>
) {
  return mutate(
    api<ServerConfig>('/api/servers', {
      method: 'POST',
      body: JSON.stringify({ ...s, host: s.name }),
    })
  );
}

export function probeServer(
  s: Omit<ServerConfig, 'id' | 'host' | 'status' | 'failureCount' | 'lastChecked' | 'role'> & {
    role: ServerConfig['role'] | '';
  }
) {
  return api<{ status: string; detail: string }>('/api/servers/probe', {
    method: 'POST',
    body: JSON.stringify({ ...s, host: s.name }),
  });
}

export function removeServer(id: string) {
  return mutate(
    api<{ status: string }>(`/api/servers/${encodeURIComponent(id)}`, { method: 'DELETE' })
  );
}

export function pingServer(id: string, options: { mode?: 'auto' } = {}) {
  const query = options.mode ? `?mode=${encodeURIComponent(options.mode)}` : '';
  return mutate(
    api<{ status: string; detail: string }>(`/api/servers/${encodeURIComponent(id)}/ping${query}`, {
      method: 'POST',
    })
  ).finally(emitNotificationRefresh);
}

export function syncServer(id: string, options: { skipHealthCheck?: boolean } = {}) {
  const query = options.skipHealthCheck ? '?skipHealthCheck=1' : '';
  return api<{ id: string; status: string }>(`/api/servers/${encodeURIComponent(id)}/sync${query}`, {
    method: 'POST',
  });
}

export function createRefresh(type = 'runtime.refresh.all') {
  return mutate(
    api<{ id: string; status: string }>('/api/refresh', {
      method: 'POST',
      body: JSON.stringify({ type }),
    })
  );
}

export function fetchRefreshTasks(limit: number | 'all' = 'all') {
  return api<{ items: RefreshTask[] }>(`/api/refresh/tasks?limit=${limit}`);
}

function wait(ms: number) {
  return new Promise(resolve => window.setTimeout(resolve, ms));
}

function refreshTaskMessage(task: RefreshTask) {
  const payload = task.payload;
  if (payload && typeof payload === 'object') {
    const message = payload.message;
    const error = payload.error;
    if (typeof error === 'string' && error.trim()) return error.trim();
    if (typeof message === 'string' && message.trim()) return message.trim();
  }
  if (typeof payload === 'string' && payload.trim()) return payload.trim();
  return '';
}

export async function waitRefreshTask(
  taskId: string,
  options: { intervalMs?: number; timeoutMs?: number } = {}
) {
  const intervalMs = options.intervalMs ?? 1200;
  const configuredTimeoutMs = getBaseConfigSnapshot().agentFullSyncTimeoutSeconds * 1000;
  const expiresAt = Date.now() + (options.timeoutMs ?? configuredTimeoutMs + 10_000);
  let missingCount = 0;
  while (Date.now() <= expiresAt) {
    const result = await fetchRefreshTasks('all');
    const task = result.items.find(item => item.id === taskId);
    if (!task) {
      missingCount++;
      if (missingCount >= 3) {
        throw new Error('刷新任务已跳过，请稍后查看同步结果');
      }
      await wait(intervalMs);
      continue;
    }
    missingCount = 0;
    if (task?.status === 'completed') {
      return task;
    }
    if (task?.status === 'failed') {
      throw new Error(refreshTaskMessage(task) || 'Agent 同步任务失败');
    }
    await wait(intervalMs);
  }
  throw new Error('Agent 同步任务等待超时');
}

export function refreshZone(id: string) {
  return api<{ id: string; status: string }>(`/api/dns/zones/${encodeURIComponent(id)}/refresh`, {
    method: 'POST',
  });
}

export function refreshScope(id: string) {
  return api<{ id: string; status: string }>(`/api/dhcp/scopes/${encodeURIComponent(id)}/refresh`, {
    method: 'POST',
  });
}

export function useDB(options: { includeDns?: boolean } = {}): DB {
  const [db, setDb] = useState<DB>(() => emptyDB());
  const includeDns = options.includeDns === true;

  const reload = useCallback(() => {
    void getDB({ includeDns })
      .then(setDb)
      .catch(error => {
        console.error(error);
        setDb(emptyDB());
      });
  }, [includeDns]);

  useEffect(() => {
    reload();
    window.addEventListener(EVENT, reload);
    const offRefresh = onZoneLeaseRefresh(reload);
    return () => {
      window.removeEventListener(EVENT, reload);
      offRefresh();
    };
  }, [reload]);

  return db;
}
