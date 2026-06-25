import { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Check, Download, FileDown, Loader2, Search, X } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  exportRows,
  localTimestamp,
  sanitizeExportFileName,
  type ExportColumn,
  type ExportFormat,
} from '@/lib/export-data';
import type { DnsRecord, DnsZone } from '@/lib/dns-dhcp-store';
import { sortRecords, sortZones } from '@/features/dns/sort';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

type ExportScope = 'all' | 'forward' | 'reverse' | 'custom';

type DnsZoneExportRow = {
  zone: string;
  zoneKind: string;
  zoneType: string;
  dynamicUpdate: string;
  recordName: string;
  recordType: string;
  value: string;
  ttl: number | string;
  updatedAt: string;
  syncedAt: string;
};

const exportScopes: Array<{ value: ExportScope; label: string }> = [
  { value: 'all', label: '全部' },
  { value: 'forward', label: '正向' },
  { value: 'reverse', label: '反向' },
  { value: 'custom', label: '自定义' },
];

const formatOptions: Array<{ value: ExportFormat; label: string }> = [
  { value: 'xlsx', label: 'XLSX' },
  { value: 'xls', label: 'XLS' },
  { value: 'csv', label: 'CSV' },
  { value: 'txt', label: 'TXT' },
];

const defaultFormat: ExportFormat = 'xlsx';

const dnsZoneExportColumns: ExportColumn<DnsZoneExportRow>[] = [
  { id: 'zone', header: '区域', value: item => item.zone },
  { id: 'recordName', header: '记录名称', value: item => item.recordName },
  { id: 'recordType', header: '记录类型', value: item => item.recordType },
  { id: 'value', header: '记录值', value: item => item.value },
  { id: 'ttl', header: 'TTL', value: item => item.ttl },
  { id: 'zoneKind', header: '区域方向', value: item => item.zoneKind },
  { id: 'zoneType', header: '区域类型', value: item => item.zoneType },
  { id: 'dynamicUpdate', header: '动态更新', value: item => item.dynamicUpdate },
  { id: 'updatedAt', header: '更新时间', value: item => item.updatedAt },
  { id: 'syncedAt', header: '同步时间', value: item => item.syncedAt },
];

export function DnsZoneExportDialog({
  open,
  loading,
  zones,
  records,
  onClose,
}: {
  open: boolean;
  loading: boolean;
  zones: DnsZone[];
  records: DnsRecord[];
  onClose: () => void;
}) {
  const [portalNode, setPortalNode] = useState<HTMLElement | null>(null);
  const [scope, setScope] = useState<ExportScope>('all');
  const [format, setFormat] = useState<ExportFormat>(defaultFormat);
  const [fileName, setFileName] = useState(`DNS区域记录-${localTimestamp()}`);
  const [zoneSearch, setZoneSearch] = useState('');
  const [selectedZoneIds, setSelectedZoneIds] = useState<string[]>([]);
  const zoneSearchInputRef = useRef<HTMLInputElement | null>(null);
  const [zoneSearchRect, setZoneSearchRect] = useState<DOMRect | null>(null);

  useEffect(() => {
    setPortalNode(document.body);
  }, []);

  useEffect(() => {
    if (!open) return;
    setScope('all');
    setFormat(defaultFormat);
    setFileName(`DNS区域记录-${localTimestamp()}`);
    setZoneSearch('');
    setSelectedZoneIds([]);
  }, [open]);

  const selectableZones = useMemo(
    () =>
      zones.filter(
        zone =>
          !selectedZoneIds.includes(zone.id) &&
          zone.name.toLowerCase().includes(zoneSearch.trim().toLowerCase())
      ),
    [selectedZoneIds, zoneSearch, zones]
  );

  const selectedZones = useMemo(
    () => zones.filter(zone => selectedZoneIds.includes(zone.id)),
    [selectedZoneIds, zones]
  );

  const exportZones = useMemo(() => {
    if (scope === 'forward') return zones.filter(zone => !zone.reverse);
    if (scope === 'reverse') return zones.filter(zone => zone.reverse);
    if (scope === 'custom') return selectedZones;
    return zones;
  }, [scope, selectedZones, zones]);

  const exportRowsForScope = useMemo(
    () => buildExportRows(exportZones, records),
    [exportZones, records]
  );

  const previewName = `${sanitizeExportFileName(fileName)}.${format}`;
  const showZoneOptions = scope === 'custom' && zoneSearch.trim() !== '';

  if (!open || !portalNode) return null;

  const addZone = (zoneId: string) => {
    setSelectedZoneIds(current => (current.includes(zoneId) ? current : [...current, zoneId]));
    setZoneSearch('');
  };

  const removeZone = (zoneId: string) => {
    setSelectedZoneIds(current => current.filter(item => item !== zoneId));
  };

  const updateZoneSearchRect = () => {
    setZoneSearchRect(zoneSearchInputRef.current?.getBoundingClientRect() ?? null);
  };

  const handleExport = () => {
    if (loading) {
      toast.warning('导出数据仍在读取中');
      return;
    }
    if (scope === 'custom' && selectedZoneIds.length === 0) {
      toast.warning('请选择至少一个区域');
      return;
    }
    if (exportRowsForScope.length === 0) {
      toast.warning('没有可导出的数据');
      return;
    }
    exportRows(exportRowsForScope, dnsZoneExportColumns, format, fileName);
    toast.success('导出文件已生成');
    onClose();
  };

  return createPortal(
    <div
      className="zl-dialog-backdrop fixed inset-0 z-[1500] flex items-center justify-center px-4 py-6"
      role="presentation"
    >
      <section
        className="zl-dialog-panel flex max-h-[88vh] w-[min(94vw,760px)] flex-col overflow-hidden rounded-2xl shadow-2xl"
        role="dialog"
        aria-modal="true"
        aria-label="导出 DNS 区域记录"
      >
        <div
          className="flex items-start justify-between gap-4 border-b p-5"
          style={{ borderColor: 'var(--zl-border)' }}
        >
          <div className="min-w-0">
            <h3 className="text-base font-semibold" style={{ color: 'var(--zl-text)' }}>
              导出 DNS 区域记录
            </h3>
            <p className="mt-1 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              {loading
                ? '正在读取当前 DNS 区域与记录'
                : `将 ${exportRowsForScope.length} 行记录导出为文件`}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="zl-action-button flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border"
            style={{
              borderColor: 'var(--zl-border)',
              color: 'var(--zl-text-muted)',
              background: 'rgba(255,255,255,0.04)',
            }}
            aria-label="关闭"
          >
            <X size={15} />
          </button>
        </div>

        <div className="zl-hidden-scrollbar flex-1 space-y-5 overflow-y-auto p-5">
          <div className="grid gap-4 md:grid-cols-[1fr_150px_150px]">
            <label className="block space-y-1.5 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              <div>导出名称</div>
              <input
                value={fileName}
                onChange={event => setFileName(event.target.value)}
                className="h-10 w-full rounded-lg px-3 text-sm outline-none"
                style={{
                  background: 'var(--zl-control-bg)',
                  border: '1px solid var(--zl-border)',
                  color: 'var(--zl-text)',
                }}
              />
            </label>
            <div className="space-y-1.5 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              <div>导出范围</div>
              <Select value={scope} onValueChange={value => setScope(value as ExportScope)}>
                <SelectTrigger className="h-10">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="z-[1600]">
                  {exportScopes.map(item => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              <div>扩展名</div>
              <Select value={format} onValueChange={value => setFormat(value as ExportFormat)}>
                <SelectTrigger className="h-10">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="z-[1600]">
                  {formatOptions.map(item => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          {scope === 'custom' ? (
            <div
              className="space-y-3 rounded-xl border p-4"
              style={{ borderColor: 'var(--zl-border)', background: 'rgba(255,255,255,0.022)' }}
            >
              <div>
                <div className="text-xs font-semibold" style={{ color: 'var(--zl-text)' }}>
                  自定义区域
                </div>
                <div className="mt-1 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
                  输入区域名称后从下拉结果中选择，可选择多个区域
                </div>
              </div>
              <div className="relative">
                <Search
                  size={14}
                  className="absolute left-3 top-1/2 -translate-y-1/2"
                  style={{ color: 'var(--zl-text-muted)' }}
                />
                <input
                  ref={zoneSearchInputRef}
                  value={zoneSearch}
                  onChange={event => {
                    setZoneSearch(event.target.value);
                    window.requestAnimationFrame(updateZoneSearchRect);
                  }}
                  onFocus={updateZoneSearchRect}
                  placeholder="搜索区域"
                  className="h-10 w-full rounded-lg py-2 pl-9 pr-3 text-sm outline-none"
                  style={{
                    background: 'var(--zl-control-bg)',
                    border: '1px solid var(--zl-border)',
                    color: 'var(--zl-text)',
                  }}
                />
              </div>
              <div className="flex min-h-10 flex-wrap gap-2">
                {selectedZones.length > 0 ? (
                  selectedZones.map(zone => (
                    <span
                      key={zone.id}
                      className="inline-flex max-w-full items-center gap-2 rounded-lg border px-2.5 py-1.5 text-xs"
                      style={{
                        borderColor: zone.reverse
                          ? 'rgba(168,85,247,0.36)'
                          : 'rgba(59,130,246,0.36)',
                        background: zone.reverse ? 'rgba(168,85,247,0.1)' : 'rgba(59,130,246,0.1)',
                        color: 'var(--zl-text)',
                      }}
                    >
                      <span className="max-w-[220px] truncate">{zone.name}</span>
                      <button
                        type="button"
                        onClick={() => removeZone(zone.id)}
                        className="grid h-4 w-4 place-items-center rounded hover:bg-background/60"
                        aria-label={`移除 ${zone.name}`}
                      >
                        <X size={12} />
                      </button>
                    </span>
                  ))
                ) : !showZoneOptions ? (
                  <span className="text-xs text-muted-foreground">尚未选择区域</span>
                ) : null}
              </div>
            </div>
          ) : null}

          <div
            className="flex items-center gap-3 rounded-lg border p-3 text-sm"
            style={{
              borderColor: 'var(--zl-border)',
              color: 'var(--zl-text-muted)',
              background: 'rgba(255,255,255,0.028)',
            }}
          >
            {loading ? <Loader2 size={16} className="animate-spin" /> : <FileDown size={16} />}
            <span className="min-w-0 truncate">{previewName}</span>
            <span className="ml-auto shrink-0 text-xs">
              {loading ? '读取中' : `${exportZones.length} 区域 / ${exportRowsForScope.length} 行`}
            </span>
          </div>
        </div>

        <div
          className="flex justify-end gap-2 border-t p-4"
          style={{ borderColor: 'var(--zl-border)', background: 'rgba(255,255,255,0.018)' }}
        >
          <Button type="button" variant="outline" onClick={onClose}>
            取消
          </Button>
          <Button type="button" disabled={loading} onClick={handleExport}>
            {loading ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
            {loading ? '读取中' : '导出'}
          </Button>
        </div>
        {showZoneOptions && zoneSearchRect
          ? createPortal(
              <ZoneSearchOptions
                rect={zoneSearchRect}
                zones={selectableZones}
                onSelect={addZone}
              />,
              portalNode
            )
          : null}
      </section>
    </div>,
    portalNode
  );
}

function ZoneSearchOptions({
  rect,
  zones,
  onSelect,
}: {
  rect: DOMRect;
  zones: DnsZone[];
  onSelect: (zoneId: string) => void;
}) {
  const maxHeight = Math.max(96, Math.min(160, window.innerHeight - rect.bottom - 24));

  return (
    <div
      className="zl-hidden-scrollbar fixed z-[1800] overflow-y-auto rounded-lg border p-1 shadow-2xl"
      role="listbox"
      style={{
        left: rect.left,
        top: rect.bottom + 6,
        width: rect.width,
        maxHeight,
        background: 'var(--zl-card)',
        borderColor: 'rgba(96,165,250,0.34)',
        color: 'var(--zl-text)',
        boxShadow: '0 18px 42px rgba(15,23,42,0.28)',
      }}
    >
      {zones.length > 0 ? (
        zones.map(zone => (
          <button
            key={zone.id}
            type="button"
            className="group flex h-10 w-full items-center justify-between rounded-md px-3 text-left text-sm transition-colors hover:bg-[rgba(59,130,246,0.16)] focus:bg-[rgba(59,130,246,0.2)] focus:outline-none"
            role="option"
            aria-selected={false}
            onMouseDown={event => {
              event.preventDefault();
              onSelect(zone.id);
            }}
          >
            <span className="min-w-0 truncate font-medium group-hover:text-[color:var(--zl-accent-text)]">
              {zone.name}
            </span>
            <span className="ml-3 shrink-0 rounded-md px-1.5 py-0.5 text-xs text-muted-foreground group-hover:bg-[rgba(59,130,246,0.14)] group-hover:text-[color:var(--zl-accent-text)]">
              {zone.reverse ? '反向' : '正向'}
            </span>
          </button>
        ))
      ) : (
        <div className="px-3 py-2 text-sm text-muted-foreground">未找到匹配区域</div>
      )}
    </div>
  );
}

function buildExportRows(zones: DnsZone[], records: DnsRecord[]): DnsZoneExportRow[] {
  return sortZones(zones).flatMap(zone => {
    const zoneRecords = sortRecords(
      records.filter(record => record.zoneId === zone.id),
      { key: 'name', direction: null }
    );

    return zoneRecords.map(record => {
      return {
        zone: zone.name,
        zoneKind: zone.reverse ? '反向' : '正向',
        zoneType: zone.type,
        dynamicUpdate: zone.dynamicUpdate,
        recordName: record.name,
        recordType: record.type,
        value: record.value,
        ttl: record.ttl,
        updatedAt: formatDateTime(record.updatedAt),
        syncedAt: formatDateTime(record.lastSyncedAt),
      };
    });
  });
}

function formatDateTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}
