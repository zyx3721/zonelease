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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { addRecord, type DnsRecordType } from '@/lib/dns-dhcp-store';

const TYPES: DnsRecordType[] = ['A', 'CNAME'];
const defaultRecordType: DnsRecordType = 'A';
const defaultTTL = 3600;
const defaultCreatePtr = true;

function recordTypeLabel(type: DnsRecordType) {
  return type === 'A' ? 'A (或 AAAA)' : type;
}

export function DnsAddRecordDialog({ zoneId }: { zoneId: string }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [type, setType] = useState<DnsRecordType>(defaultRecordType);
  const [value, setValue] = useState('');
  const [ttl, setTtl] = useState(defaultTTL);
  const [createPtr, setCreatePtr] = useState(defaultCreatePtr);
  const [creating, setCreating] = useState(false);

  const canCreatePtr = type === 'A';
  const disabled = !name || !value || creating;

  useEffect(() => {
    if (!open) return;
    setName('');
    setType(defaultRecordType);
    setValue('');
    setTtl(defaultTTL);
    setCreatePtr(defaultCreatePtr);
    setCreating(false);
  }, [open]);

  async function handleCreate() {
    if (disabled) return;
    const recordName = name.trim();
    const recordValue = value.trim();
    const recordLabel = `${recordName} ${type} ${recordValue}`;
    setCreating(true);
    try {
      const result = await addRecord({
        zoneId,
        name: recordName,
        type,
        value: recordValue,
        ttl,
        createPtr: canCreatePtr && createPtr,
      });
      if (result.warnings?.length) {
        toast.warning(`${recordLabel} 记录创建成功\n警告：${result.warnings.join('；')}`);
      } else {
        toast.success(`${recordLabel} 记录创建成功`);
      }
      setName('');
      setValue('');
      setCreatePtr(defaultCreatePtr);
      setOpen(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : `${recordName} 创建失败`);
    } finally {
      setCreating(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus className="h-3.5 w-3.5 mr-1" /> 新建记录
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>添加 DNS 记录</DialogTitle>
          <DialogDescription className="sr-only">填写 DNS 记录的名称、类型、值和 TTL</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label>名称</Label>
              <Input value={name} onChange={e => setName(e.target.value)} placeholder="www" />
            </div>
            <div>
              <Label>类型</Label>
              <Select
                value={type}
                onValueChange={v => {
                  const nextType = v as DnsRecordType;
                  setType(nextType);
                  setCreatePtr(nextType === 'A' ? defaultCreatePtr : false);
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TYPES.map(item => (
                    <SelectItem key={item} value={item}>
                      {recordTypeLabel(item)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div>
            <Label>值</Label>
            <Input value={value} onChange={e => setValue(e.target.value)} placeholder="ip地址" />
          </div>
          <div>
            <Label>TTL (秒)</Label>
            <Input type="number" value={ttl} onChange={e => setTtl(Number(e.target.value))} />
          </div>
          <label
            className={`group flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 px-3 py-2 text-sm transition-colors ${
              canCreatePtr ? 'cursor-pointer hover:bg-muted/40' : 'cursor-not-allowed opacity-60'
            }`}
          >
            <span className="flex min-w-0 items-center gap-2">
              <span
                className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-md border shadow-[inset_0_1px_0_rgba(255,255,255,0.12)] transition-colors ${
                  createPtr && canCreatePtr
                    ? 'border-info bg-info text-info-foreground'
                    : 'border-[rgba(96,165,250,0.3)] bg-[rgba(96,165,250,0.1)] text-transparent group-hover:border-info/50 group-hover:bg-[rgba(96,165,250,0.16)]'
                }`}
              >
                <span className="h-2 w-2 rounded-sm bg-current" />
              </span>
              <span className="font-medium">创建相关的指针 PTR 记录</span>
            </span>
            <span
              className={`text-xs font-medium ${
                createPtr && canCreatePtr ? 'text-info' : 'text-muted-foreground'
              }`}
            >
              {createPtr && canCreatePtr ? '已启用' : '未启用'}
            </span>
            <input
              type="checkbox"
              className="sr-only"
              disabled={!canCreatePtr}
              checked={createPtr && canCreatePtr}
              onChange={e => setCreatePtr(e.target.checked)}
            />
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            取消
          </Button>
          <span className={disabled ? 'cursor-not-allowed' : undefined}>
            <Button disabled={disabled} onClick={() => void handleCreate()}>
              {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              {creating ? '创建中' : '创建'}
            </Button>
          </span>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
