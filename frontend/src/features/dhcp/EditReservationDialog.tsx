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
import { updateReservation, type DhcpReservation } from '@/lib/dns-dhcp-store';

export function EditReservationDialog({ reservation }: { reservation: DhcpReservation }) {
  const [open, setOpen] = useState(false);
  const [ip, setIp] = useState(reservation.ip);
  const [mac, setMac] = useState(reservation.mac);
  const [name, setName] = useState(reservation.name);
  const [description, setDescription] = useState(reservation.description);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setIp(reservation.ip);
    setMac(reservation.mac);
    setName(reservation.name);
    setDescription(reservation.description);
    setSaving(false);
  }, [open, reservation]);

  const disabled = !ip.trim() || !mac.trim() || !name.trim() || saving;

  async function handleSave() {
    if (disabled) return;
    setSaving(true);
    try {
      await updateReservation(reservation.id, {
        ...reservation,
        ip: ip.trim(),
        mac: mac.trim(),
        name: name.trim(),
        description: description.trim(),
      });
      setOpen(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'DHCP 保留地址更新失败');
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <AppTooltip label="编辑保留地址" placement="top">
        <DialogTrigger asChild>
          <Button
            size="icon"
            variant="ghost"
            className="h-7 w-7 text-muted-foreground hover:text-info"
            aria-label="编辑保留地址"
          >
            <SquarePen className="h-3.5 w-3.5" />
          </Button>
        </DialogTrigger>
      </AppTooltip>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>编辑地址保留</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>IP 地址</Label>
            <Input value={ip} onChange={event => setIp(event.target.value)} />
          </div>
          <div>
            <Label>MAC 地址</Label>
            <Input value={mac} onChange={event => setMac(event.target.value)} />
          </div>
          <div>
            <Label>名称</Label>
            <Input value={name} onChange={event => setName(event.target.value)} />
          </div>
          <div>
            <Label>描述</Label>
            <Input value={description} onChange={event => setDescription(event.target.value)} />
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
