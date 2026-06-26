import { useEffect, useState, type ReactNode } from 'react';
import { ArrowLeftRight, Globe2, Loader2, Plus } from 'lucide-react';
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
import { addZone } from '@/lib/dns-dhcp-store';

export function DnsAddZoneDialog({ serverId }: { serverId: string }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [type, setType] = useState<'Primary' | 'Secondary' | 'Stub'>('Primary');
  const [zoneMode, setZoneMode] = useState<'forward' | 'reverse'>('forward');
  const [dynamicUpdate, setDynamicUpdate] = useState<'None' | 'Secure' | 'Nonsecure'>('None');
  const [creating, setCreating] = useState(false);

  const disabled = !name || !serverId || creating;
  const reverse = zoneMode === 'reverse';

  useEffect(() => {
    if (!open) return;
    setName('');
    setType('Primary');
    setZoneMode('forward');
    setDynamicUpdate('None');
    setCreating(false);
  }, [open]);

  async function handleCreate() {
    const zoneName = name.trim();
    if (!zoneName || !serverId || creating) return;
    setCreating(true);
    try {
      const result = await addZone({
        name: zoneName,
        type,
        reverse,
        dynamicUpdate: reverse ? 'None' : dynamicUpdate,
        serverId,
      });
      toast.success(`${zoneName} 创建成功`);
      if (result.warning) {
        toast.warning(result.warning);
      }
      setName('');
      setZoneMode('forward');
      setType('Primary');
      setDynamicUpdate('None');
      setOpen(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : `${zoneName} 创建失败`);
    } finally {
      setCreating(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="default">
          <Plus className="h-3.5 w-3.5 mr-1" /> 新建
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[620px]">
        <DialogHeader>
          <DialogTitle>新建 DNS 区域</DialogTitle>
          <DialogDescription className="sr-only">选择 DNS 区域模式并填写区域名称、类型和动态更新策略</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <ZoneModeCard
              active={zoneMode === 'forward'}
              icon={<Globe2 className="h-4 w-4" />}
              title="正向查找区域"
              description="用于域名解析到 IP 地址"
              onClick={() => setZoneMode('forward')}
            />
            <ZoneModeCard
              active={zoneMode === 'reverse'}
              icon={<ArrowLeftRight className="h-4 w-4" />}
              title="反向查找区域"
              description="用于 IP 地址反查域名"
              onClick={() => {
                setZoneMode('reverse');
                setDynamicUpdate('None');
              }}
            />
          </div>
          <div>
            <Label>区域名称</Label>
            <Input
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder={reverse ? '1.168.192' : 'example.com'}
            />
          </div>
          <div className={reverse ? 'grid gap-3 sm:grid-cols-1' : 'grid gap-3 sm:grid-cols-2'}>
            <div>
              <Label>类型</Label>
              <Select value={type} onValueChange={v => setType(v as typeof type)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="Primary">Primary</SelectItem>
                  <SelectItem value="Secondary">Secondary</SelectItem>
                  <SelectItem value="Stub">Stub</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {!reverse ? (
              <div>
                <Label>动态更新</Label>
                <Select
                  value={dynamicUpdate}
                  onValueChange={v => setDynamicUpdate(v as typeof dynamicUpdate)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="None">None</SelectItem>
                    <SelectItem value="Secure">Secure</SelectItem>
                    <SelectItem value="Nonsecure">Nonsecure</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            ) : null}
          </div>
          <div className="rounded-lg border border-border bg-muted/20 px-3 py-2 text-xs leading-5 text-muted-foreground">
            {reverse
              ? '反向区域只需填写网络 ID，例如 1.168.192，后端会自动补充 in-addr.arpa 后缀'
              : '正向区域可配置动态更新策略，默认使用 None'}
          </div>
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

function ZoneModeCard({
  active,
  icon,
  title,
  description,
  onClick,
}: {
  active: boolean;
  icon: ReactNode;
  title: string;
  description: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex cursor-pointer items-start gap-3 rounded-lg border px-3 py-3 text-left transition-all ${
        active
          ? 'border-info/60 bg-info/10 text-foreground shadow-[inset_0_1px_0_rgba(255,255,255,0.08)]'
          : 'border-border bg-muted/20 text-muted-foreground hover:border-info/40 hover:bg-muted/35'
      }`}
    >
      <span
        className={`mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md border ${
          active ? 'border-info/50 bg-info/15 text-info' : 'border-border bg-background/60'
        }`}
      >
        {icon}
      </span>
      <span className="min-w-0">
        <span className="block text-sm font-medium">{title}</span>
        <span className="mt-1 block text-xs leading-4">{description}</span>
      </span>
    </button>
  );
}
