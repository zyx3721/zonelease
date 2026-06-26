import { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { addReservation, type DhcpLease } from '@/lib/dns-dhcp-store';

interface AddReservationFromLeaseDialogProps {
  scopeId?: string;
  lease: DhcpLease | null;
  initialName: string;
  onClose: () => void;
}

export function AddReservationFromLeaseDialog({
  scopeId,
  lease,
  initialName,
  onClose,
}: AddReservationFromLeaseDialogProps) {
  const [visibleLease, setVisibleLease] = useState(lease);
  const [ip, setIp] = useState('');
  const [mac, setMac] = useState('');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!lease) return;
    setVisibleLease(lease);
    setIp(lease.ip);
    setMac(lease.mac);
    setName(initialName === '-' ? '' : initialName);
    setDescription('');
    setSaving(false);
  }, [initialName, lease]);

  const displayLease = lease ?? visibleLease;
  const disabled = !scopeId || !displayLease || saving;

  async function handleAdd() {
    if (!scopeId || !displayLease || disabled) return;
    setSaving(true);
    try {
      const result = await addReservation({
        scopeId,
        ip: displayLease.ip,
        mac: displayLease.mac,
        name: name.trim(),
        description: description.trim(),
      });
      toast.success(`${result.ip} 已添加到保留`);
      onClose();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '添加保留地址失败');
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={Boolean(lease)} onOpenChange={open => !open && !saving && onClose()}>
      <DialogContent onOpenAutoFocus={event => event.preventDefault()}>
        <DialogHeader>
          <DialogTitle>添加到保留</DialogTitle>
          <DialogDescription className="sr-only">填写 DHCP 保留地址的名称和描述</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label>IP 地址</Label>
            <Input value={ip} disabled />
          </div>
          <div>
            <Label>MAC 地址</Label>
            <Input value={mac} disabled />
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
          <Button variant="outline" disabled={saving} onClick={onClose}>
            取消
          </Button>
          <Button disabled={disabled} onClick={() => void handleAdd()}>
            {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            {saving ? '添加中' : '添加'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
