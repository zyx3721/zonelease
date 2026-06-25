import { useEffect, useState } from 'react';
import { Loader2, SquarePen } from 'lucide-react';
import { toast } from 'sonner';
import { AppTooltip } from '@/components/app-tooltip';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { updateScope, type DhcpScope } from '@/lib/dns-dhcp-store';

export function EditScopeDialog({ scope }: { scope: DhcpScope }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(scope.name);
  const [startRange, setStartRange] = useState(scope.startRange);
  const [endRange, setEndRange] = useState(scope.endRange);
  const [lease, setLease] = useState(scope.leaseDurationHours);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setName(scope.name);
    setStartRange(scope.startRange);
    setEndRange(scope.endRange);
    setLease(scope.leaseDurationHours);
    setSaving(false);
  }, [open, scope]);

  const disabled = !name.trim() || !startRange.trim() || !endRange.trim() || lease <= 0 || saving;

  async function handleSave() {
    if (disabled) return;
    setSaving(true);
    try {
      await updateScope(scope.id, {
        ...scope,
        name: name.trim(),
        startRange: startRange.trim(),
        endRange: endRange.trim(),
        leaseDurationHours: lease,
      });
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
      <DialogContent>
        <DialogHeader>
          <DialogTitle>编辑 DHCP 作用域</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>名称</Label>
            <Input value={name} onChange={event => setName(event.target.value)} />
          </div>
          <div>
            <Label>租期 (小时)</Label>
            <Input
              type="number"
              min={1}
              value={lease}
              onChange={event => setLease(Number(event.target.value))}
            />
          </div>
          <div>
            <Label>子网</Label>
            <Input value={scope.subnet} disabled />
          </div>
          <div>
            <Label>地址范围</Label>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Input
                value={startRange}
                onChange={event => setStartRange(event.target.value)}
                placeholder="起始 IP 地址"
              />
              <Input
                value={endRange}
                onChange={event => setEndRange(event.target.value)}
                placeholder="结束 IP 地址"
              />
            </div>
          </div>
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
