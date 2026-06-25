import type { CSSProperties, ReactNode } from 'react';

interface StatCardProps {
  label: string;
  value: string | number;
  hint?: string;
  tone?: 'default' | 'success' | 'warning' | 'danger' | 'info';
  icon?: ReactNode;
}

const toneMap = {
  default: {
    background: 'rgba(59,130,246,0.12)',
    color: 'var(--zl-accent-text)',
    border: '1px solid rgba(59,130,246,0.16)',
  },
  success: {
    background: 'rgba(16,185,129,0.12)',
    color: 'var(--zl-success)',
    border: '1px solid rgba(16,185,129,0.16)',
  },
  warning: {
    background: 'rgba(245,158,11,0.14)',
    color: 'var(--zl-warning)',
    border: '1px solid rgba(245,158,11,0.18)',
  },
  danger: {
    background: 'rgba(239,68,68,0.12)',
    color: 'var(--zl-danger)',
    border: '1px solid rgba(239,68,68,0.16)',
  },
  info: {
    background: 'rgba(6,182,212,0.12)',
    color: 'var(--zl-accent2)',
    border: '1px solid rgba(6,182,212,0.16)',
  },
} satisfies Record<NonNullable<StatCardProps['tone']>, CSSProperties>;

export function StatCard({ label, value, hint, tone = 'default', icon }: StatCardProps) {
  return (
    <div
      className="zl-surface-3d zl-card-hover rounded-lg p-5"
      style={{ boxShadow: 'var(--shadow-card)' }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-xs uppercase tracking-wide text-muted-foreground font-medium">
            {label}
          </div>
          <div className="mt-2 text-2xl font-semibold tracking-tight">{value}</div>
          {hint && <div className="mt-1 text-xs text-muted-foreground">{hint}</div>}
        </div>
        {icon && (
          <div className="grid h-10 w-10 place-items-center rounded-lg" style={toneMap[tone]}>
            {icon}
          </div>
        )}
      </div>
    </div>
  );
}
