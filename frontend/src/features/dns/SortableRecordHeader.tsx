import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react';
import type { DnsSortKey, SortDirection } from './types';

export function SortableRecordHeader({
  label,
  sortKey,
  active,
  onSort,
}: {
  label: string;
  sortKey: DnsSortKey;
  active: SortDirection;
  onSort: (key: DnsSortKey) => void;
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
