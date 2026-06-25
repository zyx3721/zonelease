import { useEffect, useState } from 'react';
import { Loader2, Plus } from 'lucide-react';
import { toast } from 'sonner';
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
import { addExclusion } from '@/lib/dns-dhcp-store';

export function AddExclusionDialog({ scopeId }: { scopeId: string }) {
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
    setSaving(true);
    try {
      await addExclusion({ scopeId, startIp: startIp.trim(), endIp: endIp.trim() });
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
        </DialogHeader>
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <Label>起始 IP 地址</Label>
            <Input value={startIp} onChange={event => setStartIp(event.target.value)} />
          </div>
          <div>
            <Label>结束 IP 地址</Label>
            <Input value={endIp} onChange={event => setEndIp(event.target.value)} />
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
