import type { DnsRecordType, DnsZone } from '@/lib/dns-dhcp-store';

export function zoneTone(zone: DnsZone) {
  return zone.reverse
    ? {
        text: '#38bdf8',
        muted: '#7dd3fc',
        border: 'rgba(56,189,248,0.34)',
        background: 'rgba(56,189,248,0.1)',
      }
    : {
        text: '#818cf8',
        muted: '#a5b4fc',
        border: 'rgba(129,140,248,0.34)',
        background: 'rgba(129,140,248,0.1)',
      };
}

const recordTypeTones: Record<
  DnsRecordType,
  { color: string; border: string; background: string }
> = {
  A: { color: '#60a5fa', border: 'rgba(96,165,250,0.38)', background: 'rgba(96,165,250,0.1)' },
  AAAA: { color: '#38bdf8', border: 'rgba(56,189,248,0.38)', background: 'rgba(56,189,248,0.1)' },
  CNAME: {
    color: '#a78bfa',
    border: 'rgba(167,139,250,0.38)',
    background: 'rgba(167,139,250,0.1)',
  },
  MX: { color: '#f59e0b', border: 'rgba(245,158,11,0.4)', background: 'rgba(245,158,11,0.1)' },
  TXT: { color: '#f472b6', border: 'rgba(244,114,182,0.38)', background: 'rgba(244,114,182,0.1)' },
  PTR: { color: '#22d3ee', border: 'rgba(34,211,238,0.38)', background: 'rgba(34,211,238,0.1)' },
  NS: { color: '#818cf8', border: 'rgba(129,140,248,0.38)', background: 'rgba(129,140,248,0.1)' },
  SRV: { color: '#fb7185', border: 'rgba(251,113,133,0.38)', background: 'rgba(251,113,133,0.1)' },
};

export function recordTypeTone(type: DnsRecordType) {
  return (
    recordTypeTones[type] ?? {
      color: 'var(--zl-text-muted)',
      border: 'var(--zl-border)',
      background: 'var(--zl-control-bg-soft)',
    }
  );
}
