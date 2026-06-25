import type { ServerConfig } from '@/lib/dns-dhcp-store';

type AgentRoleBadgeProps = {
  role: ServerConfig['role'] | string;
};

const roleStyles = {
  DNS: {
    label: 'DNS',
    color: 'var(--zl-accent-text)',
    borderColor: 'rgba(59,130,246,0.38)',
    background: 'rgba(59,130,246,0.11)',
  },
  DHCP: {
    label: 'DHCP',
    color: '#34d399',
    borderColor: 'rgba(16,185,129,0.38)',
    background: 'rgba(16,185,129,0.12)',
  },
  unknown: {
    label: '未知',
    color: 'var(--zl-text-muted)',
    borderColor: 'var(--zl-border)',
    background: 'var(--zl-control-bg-soft)',
  },
};

export function AgentRoleBadge({ role }: AgentRoleBadgeProps) {
  const tone =
    role === 'DNS' ? roleStyles.DNS : role === 'DHCP' ? roleStyles.DHCP : roleStyles.unknown;

  return (
    <span
      className="inline-flex items-center rounded-md border px-2 py-0.5 text-[11px] font-medium"
      style={{
        borderColor: tone.borderColor,
        color: tone.color,
        background: tone.background,
      }}
    >
      {tone.label}
    </span>
  );
}
