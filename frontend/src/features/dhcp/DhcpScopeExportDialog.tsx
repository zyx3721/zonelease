import { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Download, FileDown, Loader2, Search, X } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  exportRows,
  localTimestamp,
  sanitizeExportFileName,
  type ExportFormat,
} from '@/lib/export-data';
import type { DhcpExclusion, DhcpLease, DhcpReservation, DhcpScope } from '@/lib/dns-dhcp-store';
import {
  buildDhcpExportRowsAsync,
  dhcpExportColumns,
  dhcpExportScopes,
  dhcpExportTargetOption,
  dhcpExportTargetOptions,
  type DhcpExportScope,
  type DhcpExportRow,
  type DhcpExportTarget,
} from '@/features/dhcp/dhcpExportData';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

const formatOptions: Array<{ value: ExportFormat; label: string }> = [
  { value: 'xlsx', label: 'XLSX' },
  { value: 'xls', label: 'XLS' },
  { value: 'csv', label: 'CSV' },
  { value: 'txt', label: 'TXT' },
];

const defaultFormat: ExportFormat = 'xlsx';

export function DhcpScopeExportDialog({
  open,
  loading,
  scopes,
  exclusions,
  leases,
  reservations,
  onClose,
}: {
  open: boolean;
  loading: boolean;
  scopes: DhcpScope[];
  exclusions: DhcpExclusion[];
  leases: DhcpLease[];
  reservations: DhcpReservation[];
  onClose: () => void;
}) {
  const [portalNode, setPortalNode] = useState<HTMLElement | null>(null);
  const [target, setTarget] = useState<DhcpExportTarget>('scopes');
  const [scope, setScope] = useState<DhcpExportScope>('all');
  const [format, setFormat] = useState<ExportFormat>(defaultFormat);
  const [fileName, setFileName] = useState(`DHCP作用域-${localTimestamp()}`);
  const [scopeSearch, setScopeSearch] = useState('');
  const [selectedScopeIds, setSelectedScopeIds] = useState<string[]>([]);
  const [exportRowsForScope, setExportRowsForScope] = useState<DhcpExportRow[]>([]);
  const [preparingRows, setPreparingRows] = useState(false);
  const scopeSearchInputRef = useRef<HTMLInputElement | null>(null);
  const prepareTaskRef = useRef(0);
  const abortPrepareRef = useRef<AbortController | null>(null);
  const [scopeSearchRect, setScopeSearchRect] = useState<DOMRect | null>(null);

  useEffect(() => {
    setPortalNode(document.body);
  }, []);

  useEffect(() => {
    if (!open) return;
    setTarget('scopes');
    setScope('all');
    setFormat(defaultFormat);
    setFileName(`DHCP作用域-${localTimestamp()}`);
    setScopeSearch('');
    setSelectedScopeIds([]);
    setExportRowsForScope([]);
    setPreparingRows(false);
    abortPrepareRef.current?.abort();
    abortPrepareRef.current = null;
  }, [open]);

  useEffect(() => {
    if (!open) return;
    setFileName(`${dhcpExportTargetOption(target).fileLabel}-${localTimestamp()}`);
  }, [open, target]);

  const selectableScopes = useMemo(() => {
    const normalizedSearch = scopeSearch.trim().toLowerCase();
    return scopes.filter(
      item =>
        !selectedScopeIds.includes(item.id) &&
        [item.name, item.subnet, item.description, item.startRange, item.endRange]
          .join(' ')
          .toLowerCase()
          .includes(normalizedSearch)
    );
  }, [scopeSearch, scopes, selectedScopeIds]);

  const selectedScopes = useMemo(
    () => scopes.filter(item => selectedScopeIds.includes(item.id)),
    [scopes, selectedScopeIds]
  );

  const exportScopesForRange = useMemo(() => {
    if (scope === 'active') return scopes.filter(item => item.state === 'Active');
    if (scope === 'inactive') return scopes.filter(item => item.state !== 'Active');
    if (scope === 'custom') return selectedScopes;
    return scopes;
  }, [scope, scopes, selectedScopes]);

  const exportColumnsForTarget = useMemo(() => dhcpExportColumns(target), [target]);
  const targetOption = dhcpExportTargetOption(target);
  const busy = loading || preparingRows;
  const previewName = `${sanitizeExportFileName(fileName)}.${format}`;
  const showScopeOptions = scope === 'custom' && scopeSearch.trim() !== '';

  useEffect(() => {
    if (!open) return;
    if (loading) {
      prepareTaskRef.current += 1;
      abortPrepareRef.current?.abort();
      abortPrepareRef.current = null;
      setExportRowsForScope([]);
      setPreparingRows(false);
      return;
    }
    const taskId = prepareTaskRef.current + 1;
    prepareTaskRef.current = taskId;
    abortPrepareRef.current?.abort();
    const controller = new AbortController();
    abortPrepareRef.current = controller;
    setPreparingRows(true);
    const timer = window.setTimeout(() => {
      void buildDhcpExportRowsAsync(
        target,
        exportScopesForRange,
        exclusions,
        leases,
        reservations,
        { signal: controller.signal }
      )
        .then(rows => {
          if (prepareTaskRef.current !== taskId || controller.signal.aborted) return;
          setExportRowsForScope(rows);
          setPreparingRows(false);
          abortPrepareRef.current = null;
        })
        .catch(error => {
          if (controller.signal.aborted) return;
          setPreparingRows(false);
          abortPrepareRef.current = null;
          toast.error(error instanceof Error ? error.message : '准备导出数据失败');
        });
    }, 0);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [open, loading, target, exportScopesForRange, exclusions, leases, reservations]);

  if (!open || !portalNode) return null;

  const addScope = (scopeId: string) => {
    setSelectedScopeIds(current => (current.includes(scopeId) ? current : [...current, scopeId]));
    setScopeSearch('');
  };

  const removeScope = (scopeId: string) => {
    setSelectedScopeIds(current => current.filter(item => item !== scopeId));
  };

  const updateScopeSearchRect = () => {
    setScopeSearchRect(scopeSearchInputRef.current?.getBoundingClientRect() ?? null);
  };

  const handleExport = () => {
    if (loading) {
      toast.warning('导出数据仍在读取中');
      return;
    }
    if (preparingRows) {
      toast.warning('导出数据仍在准备中');
      return;
    }
    if (scope === 'custom' && selectedScopeIds.length === 0) {
      toast.warning('请选择至少一个作用域');
      return;
    }
    if (exportRowsForScope.length === 0) {
      toast.warning('没有可导出的数据');
      return;
    }
    exportRows(exportRowsForScope, exportColumnsForTarget, format, fileName);
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
        aria-label="导出 DHCP 作用域"
      >
        <div
          className="flex items-start justify-between gap-4 border-b p-5"
          style={{ borderColor: 'var(--zl-border)' }}
        >
          <div className="min-w-0">
            <h3 className="text-base font-semibold" style={{ color: 'var(--zl-text)' }}>
              导出 DHCP 数据
            </h3>
            <p className="mt-1 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              {loading
                ? '正在读取当前 DHCP 数据'
                : preparingRows
                  ? `正在准备${targetOption.label}导出数据`
                  : `将 ${exportRowsForScope.length} ${targetOption.countLabel}导出为文件`}
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
          <div className="grid gap-4 md:grid-cols-[1fr_140px_140px_130px]">
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
              <div>导出对象</div>
              <Select value={target} onValueChange={value => setTarget(value as DhcpExportTarget)}>
                <SelectTrigger className="h-10">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="z-[1600]">
                  {dhcpExportTargetOptions.map(item => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              <div>导出范围</div>
              <Select value={scope} onValueChange={value => setScope(value as DhcpExportScope)}>
                <SelectTrigger className="h-10">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="z-[1600]">
                  {dhcpExportScopes.map(item => (
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
                  自定义作用域
                </div>
                <div className="mt-1 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
                  输入作用域名称、子网或地址范围后从下拉结果中选择，可选择多个作用域
                </div>
              </div>
              <div className="relative">
                <Search
                  size={14}
                  className="absolute left-3 top-1/2 -translate-y-1/2"
                  style={{ color: 'var(--zl-text-muted)' }}
                />
                <input
                  ref={scopeSearchInputRef}
                  value={scopeSearch}
                  onChange={event => {
                    setScopeSearch(event.target.value);
                    window.requestAnimationFrame(updateScopeSearchRect);
                  }}
                  onFocus={updateScopeSearchRect}
                  placeholder="搜索作用域"
                  className="h-10 w-full rounded-lg py-2 pl-9 pr-3 text-sm outline-none"
                  style={{
                    background: 'var(--zl-control-bg)',
                    border: '1px solid var(--zl-border)',
                    color: 'var(--zl-text)',
                  }}
                />
              </div>
              <div className="flex min-h-10 flex-wrap gap-2">
                {selectedScopes.length > 0 ? (
                  selectedScopes.map(item => (
                    <span
                      key={item.id}
                      className="inline-flex max-w-full items-center gap-2 rounded-lg border px-2.5 py-1.5 text-xs"
                      style={{
                        borderColor:
                          item.state === 'Active'
                            ? 'rgba(45,212,191,0.42)'
                            : 'rgba(148,163,184,0.36)',
                        background:
                          item.state === 'Active'
                            ? 'rgba(45,212,191,0.1)'
                            : 'rgba(148,163,184,0.1)',
                        color: 'var(--zl-text)',
                      }}
                    >
                      <span className="max-w-[220px] truncate">
                        {item.name} · {item.subnet}
                      </span>
                      <button
                        type="button"
                        onClick={() => removeScope(item.id)}
                        className="grid h-4 w-4 place-items-center rounded hover:bg-background/60"
                        aria-label={`移除 ${item.name}`}
                      >
                        <X size={12} />
                      </button>
                    </span>
                  ))
                ) : !showScopeOptions ? (
                  <span className="text-xs text-muted-foreground">尚未选择作用域</span>
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
            {busy ? <Loader2 size={16} className="animate-spin" /> : <FileDown size={16} />}
            <span className="min-w-0 truncate">{previewName}</span>
            <span className="ml-auto shrink-0 text-xs">
              {loading
                ? '读取中'
                : preparingRows
                  ? '准备中'
                  : `${exportRowsForScope.length} ${targetOption.countLabel}`}
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
          <Button type="button" disabled={busy} onClick={handleExport}>
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
            {loading ? '读取中' : preparingRows ? '准备中' : '导出'}
          </Button>
        </div>
        {showScopeOptions && scopeSearchRect
          ? createPortal(
              <ScopeSearchOptions
                rect={scopeSearchRect}
                scopes={selectableScopes}
                onSelect={addScope}
              />,
              portalNode
            )
          : null}
      </section>
    </div>,
    portalNode
  );
}

function ScopeSearchOptions({
  rect,
  scopes,
  onSelect,
}: {
  rect: DOMRect;
  scopes: DhcpScope[];
  onSelect: (scopeId: string) => void;
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
      {scopes.length > 0 ? (
        scopes.map(item => (
          <button
            key={item.id}
            type="button"
            className="group flex h-10 w-full items-center justify-between rounded-md px-3 text-left text-sm transition-colors hover:bg-[rgba(59,130,246,0.16)] focus:bg-[rgba(59,130,246,0.2)] focus:outline-none"
            role="option"
            aria-selected={false}
            onMouseDown={event => {
              event.preventDefault();
              onSelect(item.id);
            }}
          >
            <span className="min-w-0 truncate font-medium group-hover:text-[color:var(--zl-accent-text)]">
              {item.name} · {item.subnet}
            </span>
            <span className="ml-3 shrink-0 rounded-md px-1.5 py-0.5 text-xs text-muted-foreground group-hover:bg-[rgba(59,130,246,0.14)] group-hover:text-[color:var(--zl-accent-text)]">
              {item.state === 'Active' ? '启用' : '停用'}
            </span>
          </button>
        ))
      ) : (
        <div className="px-3 py-2 text-sm text-muted-foreground">未找到匹配作用域</div>
      )}
    </div>
  );
}
