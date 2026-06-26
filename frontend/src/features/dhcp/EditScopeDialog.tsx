import { useEffect, useState } from 'react';
import { Loader2, SquarePen } from 'lucide-react';
import { toast } from 'sonner';
import { AppTooltip } from '@/components/app-tooltip';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { updateScope, type DhcpScope } from '@/lib/dns-dhcp-store';
import { validateDefaultGateway, validateScopeRange } from './dhcpValidation';
import { IPv4Input } from './IPv4Input';
import { ScopeRangeFields } from './ScopeRangeFields';

export function EditScopeDialog({ scope }: { scope: DhcpScope }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(scope.name);
  const [description, setDescription] = useState(scope.description);
  const [defaultGateway, setDefaultGateway] = useState(scope.defaultGateway ?? '');
  const [startRange, setStartRange] = useState(scope.startRange);
  const [endRange, setEndRange] = useState(scope.endRange);
  const [leaseUnlimited, setLeaseUnlimited] = useState(scope.leaseDurationSeconds === -1);
  const [leaseDays, setLeaseDays] = useState(0);
  const [leaseHours, setLeaseHours] = useState(0);
  const [leaseMinutes, setLeaseMinutes] = useState(0);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    const leaseSeconds = scope.leaseDurationSeconds ?? scope.leaseDurationHours * 3600;
    setName(scope.name);
    setDescription(scope.description);
    setDefaultGateway(scope.defaultGateway ?? '');
    setStartRange(scope.startRange);
    setEndRange(scope.endRange);
    setLeaseUnlimited(leaseSeconds === -1);
    setLeaseDays(leaseSeconds > 0 ? Math.floor(leaseSeconds / 86400) : 0);
    setLeaseHours(leaseSeconds > 0 ? Math.floor((leaseSeconds % 86400) / 3600) : 0);
    setLeaseMinutes(leaseSeconds > 0 ? Math.floor((leaseSeconds % 3600) / 60) : 0);
    setSaving(false);
  }, [open, scope]);

  const leaseDurationSeconds = leaseUnlimited
    ? -1
    : leaseDays * 86400 + leaseHours * 3600 + leaseMinutes * 60;
  const originalLeaseSeconds = scope.leaseDurationSeconds ?? scope.leaseDurationHours * 3600;
  const hasChanges =
    name.trim() !== scope.name ||
    description.trim() !== scope.description ||
    defaultGateway.trim() !== (scope.defaultGateway ?? '') ||
    startRange.trim() !== scope.startRange ||
    endRange.trim() !== scope.endRange ||
    leaseDurationSeconds !== originalLeaseSeconds;
  const disabled =
    !name.trim() ||
    !defaultGateway.trim() ||
    !startRange.trim() ||
    !endRange.trim() ||
    (!leaseUnlimited && leaseDurationSeconds <= 0) ||
    !hasChanges ||
    saving;

  async function handleSave() {
    if (disabled) return;
    const validationError = validateScopeRange(
      scope.subnet,
      startRange.trim(),
      endRange.trim(),
      scope.startRange,
      scope.endRange
    );
    if (validationError) {
      toast.error(validationError);
      return;
    }
    const gatewayError = validateDefaultGateway(scope.subnet, defaultGateway.trim());
    if (gatewayError) {
      toast.error(gatewayError);
      return;
    }
    setSaving(true);
    try {
      await updateScope(scope.id, {
        ...scope,
        name: name.trim(),
        description: description.trim(),
        defaultGateway: defaultGateway.trim(),
        startRange: startRange.trim(),
        endRange: endRange.trim(),
        leaseDurationHours:
          leaseDurationSeconds > 0 ? Math.max(1, Math.ceil(leaseDurationSeconds / 3600)) : 0,
        leaseDurationSeconds,
      });
      toast.success(`${scope.subnet} 作用域更新成功`);
      setOpen(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'DHCP 作用域更新失败');
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <AppTooltip label="编辑作用域" placement="top">
        <DialogTrigger asChild>
          <Button size="sm" variant="outline" aria-label="编辑作用域">
            <SquarePen className="h-3.5 w-3.5" />
            编辑
          </Button>
        </DialogTrigger>
      </AppTooltip>
      <DialogContent onOpenAutoFocus={event => event.preventDefault()}>
        <DialogHeader>
          <DialogTitle>编辑 DHCP 作用域</DialogTitle>
          <DialogDescription className="sr-only">修改 DHCP 作用域名称、租期和地址范围</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>名称</Label>
            <Input value={name} onChange={event => setName(event.target.value)} />
          </div>
          <div>
            <Label>描述</Label>
            <Input value={description} onChange={event => setDescription(event.target.value)} />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label>子网</Label>
              <Input value={scope.subnet} disabled />
            </div>
            <div>
              <Label>默认网关</Label>
              <IPv4Input
                value={defaultGateway}
                onChange={setDefaultGateway}
                aria-label="默认网关"
              />
            </div>
          </div>
          <div className="space-y-2 rounded-lg border border-border p-3">
            <Label>租期</Label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="radio"
                name="lease-mode"
                checked={!leaseUnlimited}
                onChange={() => setLeaseUnlimited(false)}
              />
              限制为
            </label>
            <div className="grid grid-cols-3 gap-3 pl-6">
              <NumberField
                label="天"
                value={leaseDays}
                disabled={leaseUnlimited}
                min={0}
                onChange={setLeaseDays}
              />
              <NumberField
                label="小时"
                value={leaseHours}
                disabled={leaseUnlimited}
                min={0}
                max={23}
                onChange={setLeaseHours}
              />
              <NumberField
                label="分钟"
                value={leaseMinutes}
                disabled={leaseUnlimited}
                min={0}
                max={59}
                onChange={setLeaseMinutes}
              />
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="radio"
                name="lease-mode"
                checked={leaseUnlimited}
                onChange={() => setLeaseUnlimited(true)}
              />
              无限制
            </label>
          </div>
          <ScopeRangeFields
            startValue={startRange}
            endValue={endRange}
            onStartChange={setStartRange}
            onEndChange={setEndRange}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            取消
          </Button>
          <Button disabled={disabled} onClick={() => void handleSave()}>
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            {saving ? '保存中' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function NumberField({
  label,
  value,
  disabled,
  min,
  max,
  onChange,
}: {
  label: string;
  value: number;
  disabled: boolean;
  min: number;
  max?: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className="space-y-1 text-xs text-muted-foreground">
      <span>{label}</span>
      <Input
        type="number"
        min={min}
        max={max}
        value={value}
        disabled={disabled}
        onChange={event => {
          const nextValue = Number(event.target.value);
          const normalized = Number.isFinite(nextValue) ? Math.max(min, nextValue) : min;
          onChange(typeof max === 'number' ? Math.min(max, normalized) : normalized);
        }}
      />
    </label>
  );
}
