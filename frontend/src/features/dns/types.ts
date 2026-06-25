import type { DnsRecord, DnsZone } from '@/lib/dns-dhcp-store';

export type DnsSortKey = 'name' | 'type' | 'value';
export type SortDirection = 'asc' | 'desc' | null;

export type RecordSortState = {
  key: DnsSortKey;
  direction: SortDirection;
};

export type PendingDnsDelete =
  | { kind: 'zone'; id: string; name: string; detail: string }
  | { kind: 'record'; id: string; name: string; detail: string };

export type { DnsRecord, DnsZone };
