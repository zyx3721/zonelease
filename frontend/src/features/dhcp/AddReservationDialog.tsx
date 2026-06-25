import { useEffect, useState } from 'react';
import { Plus } from 'lucide-react';
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
import { addReservation } from '@/lib/dns-dhcp-store';

export function AddReservationDialog({ scopeId }: { scopeId: string }) {
  const [open, setOpen] = useState(false);
  const [ip, setIp] = useState('');
  const [mac, setMac] = useState('');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');

  useEffect(() => {
    if (!open) return;
    setIp('');
    setMac('');
    setName('');
    setDescription('');
  }, [open, scopeId]);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          <Plus className="h-3.5 w-3.5 mr-1" /> 新增保留
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新增地址保留</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>IP 地址</Label>
            <Input value={ip} onChange={e => setIp(e.target.value)} placeholder="10.0.1.150" />
          </div>
          <div>
            <Label>MAC 地址</Label>
            <Input
              value={mac}
              onChange={e => setMac(e.target.value)}
              placeholder="AA-BB-CC-DD-EE-FF"
            />
          </div>
          <div>
            <Label>名称</Label>
            <Input value={name} onChange={e => setName(e.target.value)} />
          </div>
          <div>
            <Label>描述</Label>
            <Input value={description} onChange={e => setDescription(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            取消
          </Button>
          <Button
            disabled={!ip || !mac || !name}
            onClick={() => {
              void addReservation({ scopeId, ip, mac, name, description }).then(() => {
                setIp('');
                setMac('');
                setName('');
                setDescription('');
                setOpen(false);
              });
            }}
          >
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
