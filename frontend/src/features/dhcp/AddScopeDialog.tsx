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
import { addScope } from '@/lib/dns-dhcp-store';

export function AddScopeDialog({ serverId }: { serverId: string }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [subnet, setSubnet] = useState('');
  const [startRange, setStart] = useState('');
  const [endRange, setEnd] = useState('');
  const [lease, setLease] = useState(24);
  const [creating, setCreating] = useState(false);

  const disabled = !name || !subnet || !startRange || !endRange || !serverId || creating;

  useEffect(() => {
    if (!open) return;
    setName('');
    setSubnet('');
    setStart('');
    setEnd('');
    setLease(24);
    setCreating(false);
  }, [open]);

  async function handleCreate() {
    if (disabled) return;
    setCreating(true);
    try {
      await addScope({
        name,
        description: '',
        subnet,
        startRange,
        endRange,
        leaseDurationHours: lease,
        state: 'Active',
        serverId,
      });
      setName('');
      setSubnet('');
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
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>名称</Label>
            <Input value={name} onChange={e => setName(e.target.value)} placeholder="Office-LAN" />
          </div>
          <div>
            <Label>子网</Label>
            <Input
              value={subnet}
              onChange={e => setSubnet(e.target.value)}
              placeholder="10.0.1.0/24"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label>起始 IP</Label>
              <Input
                value={startRange}
                onChange={e => setStart(e.target.value)}
                placeholder="10.0.1.100"
              />
            </div>
            <div>
              <Label>结束 IP</Label>
              <Input
                value={endRange}
                onChange={e => setEnd(e.target.value)}
                placeholder="10.0.1.250"
              />
            </div>
          </div>
          <div>
            <Label>租期 (小时)</Label>
            <Input type="number" value={lease} onChange={e => setLease(Number(e.target.value))} />
          </div>
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
