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
import { Label } from '@/components/ui/label';
import { addExclusion, type DhcpExclusion } from '@/lib/dns-dhcp-store';
import { validateExclusionRange } from './dhcpValidation';
import { IPv4Input } from './IPv4Input';

export function AddExclusionDialog({
  scopeId,
  scopeStartRange,
  scopeEndRange,
  exclusions,
}: {
  scopeId: string;
  scopeStartRange: string;
  scopeEndRange: string;
  exclusions: DhcpExclusion[];
}) {
  const [open, setOpen] = useState(false);
  const [startIp, setStartIp] = useState('');
  const [endIp, setEndIp] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setStartIp('');
    setEndIp('');
    setSaving(false);
  }, [open, scopeId]);

  const disabled = !startIp.trim() || !endIp.trim() || saving;

  async function handleSave() {
    if (disabled) return;
    const validationError = validateExclusionRange({
      startIp: startIp.trim(),
      endIp: endIp.trim(),
      scopeStartRange,
      scopeEndRange,
      existingExclusions: exclusions,
    });
    if (validationError) {
      toast.error(validationError);
      return;
    }
    setSaving(true);
    try {
      await addExclusion({ scopeId, startIp: startIp.trim(), endIp: endIp.trim() });
      toast.success('新增排除范围成功');
      setOpen(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'DHCP 排除范围创建失败');
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          <Plus className="h-3.5 w-3.5" />
          新增
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新增排除范围</DialogTitle>
          <DialogDescription className="sr-only">填写要从 DHCP 作用域中排除的起始和结束 IP 地址</DialogDescription>
        </DialogHeader>
        <div className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-3">
          <div>
            <Label>起始 IP 地址</Label>
            <IPv4Input value={startIp} onChange={setStartIp} aria-label="排除范围起始 IP 地址" />
          </div>
          <span className="pt-6 text-sm text-muted-foreground">-</span>
          <div>
            <Label>结束 IP 地址</Label>
            <IPv4Input value={endIp} onChange={setEndIp} aria-label="排除范围结束 IP 地址" />
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
