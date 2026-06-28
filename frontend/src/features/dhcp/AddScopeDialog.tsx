import { useEffect, useState } from 'react';
import { Loader2, Plus } from 'lucide-react';
import { toast } from 'sonner';
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
import { addScope, type DhcpScope } from '@/lib/dns-dhcp-store';
import {
  validateDefaultGateway,
  validateScopeSubnetConflict,
  validateSubnetRange,
} from './dhcpValidation';
import { IPv4Input } from './IPv4Input';
import { ScopeRangeFields } from './ScopeRangeFields';

export function AddScopeDialog({
  serverId,
  existingScopes,
}: {
  serverId: string;
  existingScopes: DhcpScope[];
}) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [subnet, setSubnet] = useState('');
  const [defaultGateway, setDefaultGateway] = useState('');
  const [startRange, setStart] = useState('');
  const [endRange, setEnd] = useState('');
  const [leaseUnlimited, setLeaseUnlimited] = useState(false);
  const [leaseDays, setLeaseDays] = useState(1);
  const [leaseHours, setLeaseHours] = useState(0);
  const [leaseMinutes, setLeaseMinutes] = useState(0);
  const [creating, setCreating] = useState(false);

  const leaseDurationSeconds = leaseUnlimited
    ? -1
    : leaseDays * 86400 + leaseHours * 3600 + leaseMinutes * 60;
  const disabled =
    !name.trim() ||
    !subnet.trim() ||
    !defaultGateway.trim() ||
    !startRange.trim() ||
    !endRange.trim() ||
    (!leaseUnlimited && leaseDurationSeconds <= 0) ||
    !serverId ||
    creating;

  useEffect(() => {
    if (!open) return;
    setName('');
    setDescription('');
    setSubnet('');
    setDefaultGateway('');
    setStart('');
    setEnd('');
    setLeaseUnlimited(false);
    setLeaseDays(1);
    setLeaseHours(0);
    setLeaseMinutes(0);
    setCreating(false);
  }, [open]);

  async function handleCreate() {
    if (disabled) return;
    const validationError = validateSubnetRange(subnet.trim(), startRange.trim(), endRange.trim());
    if (validationError) {
      toast.error(validationError);
      return;
    }
    const gatewayError = validateDefaultGateway(subnet.trim(), defaultGateway.trim());
    if (gatewayError) {
      toast.error(gatewayError);
      return;
    }
    const subnetConflict = validateScopeSubnetConflict(subnet.trim(), existingScopes);
    if (subnetConflict) {
      toast.error(subnetConflict);
      return;
    }
    setCreating(true);
    try {
      await addScope({
        name: name.trim(),
        description: description.trim(),
        subnet: subnet.trim(),
        defaultGateway: defaultGateway.trim(),
        startRange: startRange.trim(),
        endRange: endRange.trim(),
        leaseDurationHours:
          leaseDurationSeconds > 0 ? Math.max(1, Math.ceil(leaseDurationSeconds / 3600)) : 0,
        leaseDurationSeconds,
        state: 'Active',
        serverId,
      });
      toast.success(`${subnet.trim()} 作用域创建成功`);
      setName('');
      setDescription('');
      setSubnet('');
      setDefaultGateway('');
      setStart('');
      setEnd('');
      setOpen(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'DHCP 作用域创建失败');
    } finally {
      setCreating(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus className="h-3.5 w-3.5 mr-1" /> 新建
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新建 DHCP 作用域</DialogTitle>
          <DialogDescription className="sr-only">
            填写 DHCP 作用域名称、子网、地址范围和租期
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>名称</Label>
            <Input value={name} onChange={e => setName(e.target.value)} placeholder="Office-LAN" />
          </div>
          <div>
            <Label>描述</Label>
            <Input
              value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="可选：默认为空"
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Label>子网</Label>
              <Input
                value={subnet}
                onChange={e => setSubnet(e.target.value)}
                placeholder="10.0.1.0/24"
              />
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
                name="new-scope-lease-mode"
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
                name="new-scope-lease-mode"
                checked={leaseUnlimited}
                onChange={() => setLeaseUnlimited(true)}
              />
              无限制
            </label>
          </div>
          <ScopeRangeFields
            startValue={startRange}
            endValue={endRange}
            onStartChange={setStart}
            onEndChange={setEnd}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            取消
          </Button>
          <Button disabled={disabled} onClick={() => void handleCreate()}>
            {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            {creating ? '创建中' : '创建'}
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
