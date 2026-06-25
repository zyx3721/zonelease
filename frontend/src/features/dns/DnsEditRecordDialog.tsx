import { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { updateRecordValue, type DnsRecord } from '@/lib/dns-dhcp-store';

function recordTypeLabel(type: string) {
  return type === 'A' || type === 'AAAA' ? 'A (或 AAAA)' : type;
}

export function DnsEditRecordDialog({
  record,
  onClose,
}: {
  record: DnsRecord | null;
  onClose: () => void;
}) {
  const [visibleRecord, setVisibleRecord] = useState(record);
  const [value, setValue] = useState('');
  const [createPtr, setCreatePtr] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!record) return;
    setVisibleRecord(record);
    setValue(record.value);
    setCreatePtr(Boolean(record.createPtr));
    setSaving(false);
  }, [record]);

  const displayRecord = record ?? visibleRecord;
  const canCreatePtr = displayRecord?.type === 'A' || displayRecord?.type === 'AAAA';
  const nextValue = value.trim();
  const disabled = !displayRecord || !nextValue || saving;

  async function handleSave() {
    if (!displayRecord || disabled) return;
    setSaving(true);
    try {
      const result = await updateRecordValue(
        displayRecord.id,
        nextValue,
        canCreatePtr ? { createPtr } : {}
      );
      const recordLabel = `${result.name} ${result.type} ${result.value}`;
      if (result.warnings?.length) {
        toast.warning(`${recordLabel} 记录已更新\n警告：${result.warnings.join('；')}`);
      } else {
        toast.success(`${recordLabel} 记录已更新`);
      }
      onClose();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'DNS 记录更新失败');
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={Boolean(record)} onOpenChange={open => !open && onClose()}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>编辑 DNS 记录</DialogTitle>
        </DialogHeader>
        {displayRecord ? (
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label>名称</Label>
                <Input value={displayRecord.name} disabled />
              </div>
              <div>
                <Label>类型</Label>
                <Input value={recordTypeLabel(displayRecord.type)} disabled />
              </div>
            </div>
            <div>
              <Label>值</Label>
              <Input
                value={value}
                onChange={event => setValue(event.target.value)}
                placeholder="记录值"
                autoFocus
              />
            </div>
            <div>
              <Label>TTL (秒)</Label>
              <Input value={displayRecord.ttl} disabled />
            </div>
            {canCreatePtr ? (
              <label className="group flex cursor-pointer items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 px-3 py-2 text-sm transition-colors hover:bg-muted/40">
                <span className="flex min-w-0 items-center gap-2">
                  <span
                    className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-md border shadow-[inset_0_1px_0_rgba(255,255,255,0.12)] transition-colors ${
                      createPtr
                        ? 'border-info bg-info text-info-foreground'
                        : 'border-[rgba(96,165,250,0.3)] bg-[rgba(96,165,250,0.1)] text-transparent group-hover:border-info/50 group-hover:bg-[rgba(96,165,250,0.16)]'
                    }`}
                  >
                    <span className="h-2 w-2 rounded-sm bg-current" />
                  </span>
                  <span className="font-medium">更新相关的指针 PTR 记录</span>
                </span>
                <span
                  className={`text-xs font-medium ${
                    createPtr ? 'text-info' : 'text-muted-foreground'
                  }`}
                >
                  {createPtr ? '已启用' : '未启用'}
                </span>
                <input
                  type="checkbox"
                  className="sr-only"
                  checked={createPtr}
                  onChange={event => setCreatePtr(event.target.checked)}
                />
              </label>
            ) : null}
          </div>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={saving}>
            取消
          </Button>
          <span className={disabled ? 'cursor-not-allowed' : undefined}>
            <Button disabled={disabled} onClick={() => void handleSave()}>
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              {saving ? '保存中' : '保存'}
            </Button>
          </span>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
