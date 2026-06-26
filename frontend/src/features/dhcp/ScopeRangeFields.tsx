import { IPv4Input } from './IPv4Input';

interface ScopeRangeFieldsProps {
  startValue: string;
  endValue: string;
  onStartChange: (value: string) => void;
  onEndChange: (value: string) => void;
}

export function ScopeRangeFields({
  startValue,
  endValue,
  onStartChange,
  onEndChange,
}: ScopeRangeFieldsProps) {
  return (
    <div className="space-y-3 rounded-lg border border-border p-3">
      <div className="text-sm font-medium">地址范围</div>
      <div className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-end gap-3">
        <div className="space-y-1.5">
          <div className="text-xs text-muted-foreground">起始 IP 地址</div>
          <IPv4Input value={startValue} onChange={onStartChange} aria-label="起始 IP 地址" />
        </div>
        <span className="pb-2 text-sm text-muted-foreground">-</span>
        <div className="space-y-1.5">
          <div className="text-xs text-muted-foreground">结束 IP 地址</div>
          <IPv4Input value={endValue} onChange={onEndChange} aria-label="结束 IP 地址" />
        </div>
      </div>
    </div>
  );
}
