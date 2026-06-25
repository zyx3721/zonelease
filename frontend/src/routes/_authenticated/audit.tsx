import { createFileRoute } from '@tanstack/react-router';
import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  ClipboardList,
  Download,
  Eye,
  FileClock,
  Filter,
  Search,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import {
  fetchRefreshTasks,
  getDB,
  useDB,
  type AuditEntry,
  type RefreshTask,
} from '@/lib/dns-dhcp-store';
import { onZoneLeaseRefresh } from '@/lib/refresh';
import { localTimestamp, type ExportColumn } from '@/lib/export-data';
import { AppTooltip } from '@/components/app-tooltip';
import { ExportDialog } from '@/components/export-dialog';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

export const Route = createFileRoute('/_authenticated/audit')({
  component: AuditPage,
});

type TabKey = 'tasks' | 'audit';
type PageSize = 30 | 50 | 100 | 200 | 'all';
type JsonFilter = { key: string; value: string };
type DetailItem = { type: 'tasks'; item: RefreshTask } | { type: 'audit'; item: AuditEntry } | null;
type Tone = 'green' | 'red' | 'yellow' | 'blue' | 'gray';

const tabs: Array<{ key: TabKey; label: string; icon: typeof ClipboardList }> = [
  { key: 'tasks', label: '任务', icon: ClipboardList },
  { key: 'audit', label: '审计', icon: FileClock },
];

const pageSizes: Array<{ value: PageSize; label: string }> = [
  { value: 30, label: '30' },
  { value: 50, label: '50' },
  { value: 100, label: '100' },
  { value: 200, label: '200' },
  { value: 'all', label: '全部' },
];

const taskStatuses = [
  { value: 'all', label: '全部任务' },
  { value: 'queued', label: '排队' },
  { value: 'running', label: '运行' },
  { value: 'completed', label: '完成' },
  { value: 'failed', label: '失败' },
];

const searchPlaceholders: Record<TabKey, string> = {
  tasks: '搜索任务类型、状态、目标、进度或错误信息',
  audit: '搜索审计动作、用户、资源、IP 或元数据',
};

const jsonFilterLabels: Record<
  TabKey,
  { title: string; keyPlaceholder: string; valuePlaceholder: string; sourceName: string }
> = {
  tasks: {
    title: '任务载荷筛选',
    keyPlaceholder: '字段，如 currentAgent',
    valuePlaceholder: '值，如 DNS-01',
    sourceName: '任务载荷',
  },
  audit: {
    title: '审计元数据筛选',
    keyPlaceholder: '字段，如 username',
    valuePlaceholder: '值，如 admin',
    sourceName: '审计元数据',
  },
};

function AuditPage() {
  const db = useDB();
  const [tab, setTab] = useState<TabKey>('tasks');
  const [tasks, setTasks] = useState<RefreshTask[]>([]);
  const [loadingTasks, setLoadingTasks] = useState(true);
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('all');
  const [pageSize, setPageSize] = useState<PageSize>(50);
  const [page, setPage] = useState(1);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [exportLoading, setExportLoading] = useState(false);
  const [exportTasks, setExportTasks] = useState<RefreshTask[]>([]);
  const [exportAudit, setExportAudit] = useState<AuditEntry[]>([]);
  const [jsonFilters, setJsonFilters] = useState<Record<TabKey, JsonFilter>>({
    tasks: { key: '', value: '' },
    audit: { key: '', value: '' },
  });
  const [detail, setDetail] = useState<DetailItem>(null);

  useEffect(() => {
    let cancelled = false;
    const loadTasks = () => {
      setLoadingTasks(true);
      void fetchRefreshTasks('all')
        .then(body => {
          if (!cancelled) setTasks(body.items);
        })
        .catch(error => {
          if (!cancelled) toast.error(error instanceof Error ? error.message : '读取任务记录失败');
        })
        .finally(() => {
          if (!cancelled) setLoadingTasks(false);
        });
    };
    loadTasks();
    const off = onZoneLeaseRefresh(loadTasks);
    return () => {
      cancelled = true;
      off();
    };
  }, []);

  useEffect(() => {
    setPage(1);
    setStatus('all');
  }, [tab, pageSize]);

  useEffect(() => {
    setPage(1);
  }, [query, status, jsonFilters]);

  const activeJsonFilter = useMemo(
    () => ({
      key: jsonFilters[tab].key.trim(),
      value: jsonFilters[tab].value.trim(),
    }),
    [jsonFilters, tab]
  );
  const hasAdvancedFilter = activeJsonFilter.key !== '' || activeJsonFilter.value !== '';

  const exportTaskRows = useMemo(
    () => filterTaskRows(exportTasks, query, status, activeJsonFilter),
    [activeJsonFilter, exportTasks, query, status]
  );
  const exportAuditRows = useMemo(
    () => filterAuditRows(exportAudit, query, activeJsonFilter),
    [activeJsonFilter, exportAudit, query]
  );

  async function openExportDialog() {
    setExportTasks([]);
    setExportAudit([]);
    setExportLoading(true);
    setExportOpen(true);
    try {
      if (tab === 'tasks') {
        const body = await fetchRefreshTasks('all');
        setExportTasks(body.items);
      } else {
        const state = await getDB();
        setExportAudit(state.audit);
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '读取导出数据失败');
    } finally {
      setExportLoading(false);
    }
  }

  const filteredTasks = useMemo(
    () => filterTaskRows(tasks, query, status, activeJsonFilter),
    [activeJsonFilter, query, status, tasks]
  );

  const filteredAudit = useMemo(
    () => filterAuditRows(db.audit, query, activeJsonFilter),
    [activeJsonFilter, db.audit, query]
  );

  const total = tab === 'tasks' ? filteredTasks.length : filteredAudit.length;
  const pageCount = pageSize === 'all' ? 1 : Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(page, pageCount);
  const visibleTasks = paginate(filteredTasks, pageSize, currentPage);
  const visibleAudit = paginate(filteredAudit, pageSize, currentPage);
  const visibleCount = tab === 'tasks' ? visibleTasks.length : visibleAudit.length;
  const start = total === 0 ? 0 : pageSize === 'all' ? 1 : (currentPage - 1) * pageSize + 1;
  const end = pageSize === 'all' ? total : Math.min(total, start + visibleCount - 1);

  useEffect(() => {
    if (page > pageCount) setPage(pageCount);
  }, [page, pageCount]);

  function updateJsonFilter(field: keyof JsonFilter, value: string) {
    setJsonFilters(current => ({ ...current, [tab]: { ...current[tab], [field]: value } }));
  }

  function clearJsonFilter() {
    setJsonFilters(current => ({ ...current, [tab]: { key: '', value: '' } }));
  }

  return (
    <div data-cmp="OperationsPage" className="space-y-5 pb-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold" style={{ color: 'var(--zl-text)' }}>
            任务 / 审计
          </h2>
          <p className="mt-1 text-sm" style={{ color: 'var(--zl-text-muted)' }}>
            只读查看后台刷新任务和平台操作审计
          </p>
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div
          className="zl-card-hover flex w-fit flex-wrap gap-2 rounded-xl p-2"
          style={{ background: 'var(--zl-card)', border: '1px solid var(--zl-border)' }}
        >
          {tabs.map(item => {
            const Icon = item.icon;
            const active = tab === item.key;
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => setTab(item.key)}
                className="zl-config-nav-card flex items-center gap-2 rounded-lg px-3 py-2 text-sm"
                data-active={active}
                style={{
                  color: active ? 'var(--zl-accent-text)' : 'var(--zl-text-muted)',
                }}
              >
                <Icon size={15} />
                {item.label}
              </button>
            );
          })}
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <PageSizePicker value={pageSize} onChange={setPageSize} />
          <ExportButton onClick={() => void openExportDialog()} />
        </div>
      </div>

      <div
        className="zl-card-hover rounded-xl p-3"
        style={{ background: 'var(--zl-card)', border: '1px solid var(--zl-border)' }}
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="relative min-w-[260px] flex-1">
            <Search
              size={14}
              className="absolute left-3 top-1/2 -translate-y-1/2"
              style={{ color: 'var(--zl-text-muted)' }}
            />
            <input
              value={query}
              onChange={event => setQuery(event.target.value)}
              placeholder={searchPlaceholders[tab]}
              className="w-full rounded-lg py-2 pl-9 pr-3 text-sm outline-none"
              style={{
                background: 'var(--zl-control-bg)',
                border: '1px solid var(--zl-border)',
                color: 'var(--zl-text)',
              }}
            />
          </div>
          <button
            type="button"
            onClick={() => setAdvancedOpen(open => !open)}
            className="zl-action-button flex h-10 shrink-0 items-center gap-2 rounded-lg border px-3 text-sm"
            style={{
              background: hasAdvancedFilter ? 'rgba(59,130,246,0.15)' : 'rgba(255,255,255,0.03)',
              borderColor:
                hasAdvancedFilter || advancedOpen ? 'rgba(59,130,246,0.38)' : 'var(--zl-border)',
              color:
                hasAdvancedFilter || advancedOpen
                  ? 'var(--zl-accent-text)'
                  : 'var(--zl-text-muted)',
            }}
            aria-expanded={advancedOpen}
          >
            <Filter size={14} />
            高级筛选
            {hasAdvancedFilter ? (
              <span
                className="rounded-md px-1.5 py-0.5 text-[11px]"
                style={{ background: 'rgba(59,130,246,0.16)', color: 'var(--zl-accent-text)' }}
              >
                已启用
              </span>
            ) : null}
          </button>
          {tab === 'tasks' ? (
            <Segmented value={status} onChange={setStatus} items={taskStatuses} />
          ) : null}
        </div>
        {advancedOpen ? (
          <AdvancedJsonFilter
            filter={jsonFilters[tab]}
            labels={jsonFilterLabels[tab]}
            onChange={updateJsonFilter}
            onClear={clearJsonFilter}
          />
        ) : null}
      </div>

      <div>
        {loadingTasks && tab === 'tasks' ? (
          <div className="text-sm" style={{ color: 'var(--zl-text-muted)' }}>
            正在加载记录...
          </div>
        ) : tab === 'tasks' ? (
          <TaskTable items={visibleTasks} onDetail={item => setDetail({ type: 'tasks', item })} />
        ) : (
          <AuditTable items={visibleAudit} onDetail={item => setDetail({ type: 'audit', item })} />
        )}
      </div>

      <Pagination
        start={start}
        end={end}
        total={total}
        pageSize={pageSize}
        currentPage={currentPage}
        pageCount={pageCount}
        setPage={setPage}
      />
      {tab === 'tasks' ? (
        <ExportDialog
          open={exportOpen}
          title="导出任务"
          defaultName={`任务-${localTimestamp()}`}
          rows={exportTaskRows}
          columns={taskExportColumns}
          loading={exportLoading}
          onClose={() => setExportOpen(false)}
        />
      ) : (
        <ExportDialog
          open={exportOpen}
          title="导出审计"
          defaultName={`审计-${localTimestamp()}`}
          rows={exportAuditRows}
          columns={auditExportColumns}
          loading={exportLoading}
          onClose={() => setExportOpen(false)}
        />
      )}
      <DetailDialog detail={detail} onClose={() => setDetail(null)} />
    </div>
  );
}

function TaskTable({
  items,
  onDetail,
}: {
  items: RefreshTask[];
  onDetail: (item: RefreshTask) => void;
}) {
  return (
    <RecordShell fixed>
      <colgroup>
        <col className="w-[15%]" />
        <col className="w-[10%]" />
        <col className="w-[50%]" />
        <col className="w-[10%]" />
        <col className="w-[10%]" />
        <col className="w-[5%]" />
      </colgroup>
      <thead>
        <tr>
          {['类型', '状态', '目标', '进度', '时间', '查看'].map(head => (
            <Head key={head}>{head}</Head>
          ))}
        </tr>
      </thead>
      <tbody>
        {items.length === 0 ? <Empty colSpan={6} text="暂无任务记录" /> : null}
        {items.map(item => (
          <tr key={item.id} style={{ borderBottom: '1px solid rgba(56,78,120,0.16)' }}>
            <Cell main>{labelTaskType(item.type)}</Cell>
            <Cell>
              <StatusPill value={labelStatus(item.status)} tone={statusTone(item.status)} />
            </Cell>
            <Cell compact>
              <span className="break-words">{taskTarget(item)}</span>
            </Cell>
            <Cell>{taskProgress(item)}</Cell>
            <Cell>{formatTimeAgo(item.createdAt)}</Cell>
            <Cell>
              <TooltipIconButton label="查看详情" onClick={() => onDetail(item)}>
                <Eye size={14} />
              </TooltipIconButton>
            </Cell>
          </tr>
        ))}
      </tbody>
    </RecordShell>
  );
}

function AuditTable({
  items,
  onDetail,
}: {
  items: AuditEntry[];
  onDetail: (item: AuditEntry) => void;
}) {
  return (
    <RecordShell>
      <colgroup>
        <col className="w-[20%]" />
        <col className="w-[14%]" />
        <col className="w-[38%]" />
        <col className="w-[14%]" />
        <col className="w-[9%]" />
        <col className="w-[5%]" />
      </colgroup>
      <thead>
        <tr>
          {['动作', '用户', '资源', 'IP', '时间', '查看'].map(head => (
            <Head key={head}>{head}</Head>
          ))}
        </tr>
      </thead>
      <tbody>
        {items.length === 0 ? <Empty colSpan={6} text="暂无审计日志" /> : null}
        {items.map(item => (
          <tr key={item.id} style={{ borderBottom: '1px solid rgba(56,78,120,0.16)' }}>
            <Cell main>{labelAction(item.action)}</Cell>
            <Cell>{item.user || '-'}</Cell>
            <Cell compact>
              <span className="break-words">{auditResource(item)}</span>
            </Cell>
            <Cell>{item.ipAddress || '-'}</Cell>
            <Cell>{formatTimeAgo(item.ts)}</Cell>
            <Cell>
              <TooltipIconButton label="查看详情" onClick={() => onDetail(item)}>
                <Eye size={14} />
              </TooltipIconButton>
            </Cell>
          </tr>
        ))}
      </tbody>
    </RecordShell>
  );
}

function RecordShell({ children, fixed }: { children: ReactNode; fixed?: boolean }) {
  return (
    <div
      className="zl-card-hover overflow-hidden rounded-xl"
      style={{ background: 'var(--zl-card)', border: '1px solid var(--zl-border)' }}
    >
      <div className="zl-hidden-scrollbar overflow-x-auto">
        <table className={`${fixed ? 'table-fixed' : ''} w-full min-w-[780px] text-sm`}>
          {children}
        </table>
      </div>
    </div>
  );
}

function filterTaskRows(
  rows: RefreshTask[],
  query: string,
  status: string,
  activeJsonFilter: JsonFilter
) {
  return rows.filter(item => {
    if (status !== 'all' && item.status !== status) return false;
    if (!matchesKeyword(taskSearchText(item), query)) return false;
    return matchesJsonFilter(parsePayload(item.payload), activeJsonFilter);
  });
}

function filterAuditRows(rows: AuditEntry[], query: string, activeJsonFilter: JsonFilter) {
  return rows.filter(item => {
    if (!matchesKeyword(auditSearchText(item), query)) return false;
    return matchesJsonFilter(auditFilterPayload(item), activeJsonFilter);
  });
}

function Head({ children }: { children: ReactNode }) {
  return (
    <th
      className="sticky top-0 z-10 px-4 py-3 text-center text-xs font-semibold"
      style={{
        color: 'var(--zl-text-muted)',
        borderBottom: '1px solid var(--zl-border)',
        background: 'var(--zl-table-head-bg)',
      }}
    >
      {children}
    </th>
  );
}

function Cell({
  children,
  main,
  compact,
}: {
  children: ReactNode;
  main?: boolean;
  compact?: boolean;
}) {
  return (
    <td
      className={`${compact ? 'px-2' : 'px-4'} py-3 text-center align-middle`}
      style={{ color: main ? 'var(--zl-text)' : 'var(--zl-text-muted)' }}
    >
      {children}
    </td>
  );
}

function Empty({ colSpan, text }: { colSpan: number; text: string }) {
  return (
    <tr>
      <td
        colSpan={colSpan}
        className="px-4 py-10 text-center"
        style={{ color: 'var(--zl-text-muted)' }}
      >
        {text}
      </td>
    </tr>
  );
}

function AdvancedJsonFilter({
  filter,
  labels,
  onChange,
  onClear,
}: {
  filter: JsonFilter;
  labels: { title: string; keyPlaceholder: string; valuePlaceholder: string; sourceName: string };
  onChange: (field: keyof JsonFilter, value: string) => void;
  onClear: () => void;
}) {
  const active = filter.key.trim() !== '' || filter.value.trim() !== '';
  return (
    <div
      className="mt-3 rounded-lg border p-3"
      style={{ background: 'rgba(255,255,255,0.025)', borderColor: 'var(--zl-border)' }}
    >
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div
          className="flex items-center gap-2 text-xs font-semibold"
          style={{ color: 'var(--zl-text)' }}
        >
          <Filter size={13} />
          {labels.title}
        </div>
        {active ? (
          <button
            type="button"
            onClick={onClear}
            className="zl-action-button rounded-md border px-2 py-1 text-xs"
            style={{
              background: 'transparent',
              borderColor: 'var(--zl-border)',
              color: 'var(--zl-text-muted)',
            }}
          >
            清空
          </button>
        ) : null}
      </div>
      <div className="grid grid-cols-1 gap-2 md:grid-cols-[minmax(160px,220px)_1fr]">
        <input
          value={filter.key}
          onChange={event => onChange('key', event.target.value)}
          placeholder={labels.keyPlaceholder}
          className="h-9 rounded-lg px-3 text-sm outline-none"
          style={{
            background: 'var(--zl-control-bg)',
            border: '1px solid var(--zl-border)',
            color: 'var(--zl-text)',
          }}
        />
        <input
          value={filter.value}
          onChange={event => onChange('value', event.target.value)}
          placeholder={labels.valuePlaceholder}
          className="h-9 rounded-lg px-3 text-sm outline-none"
          style={{
            background: 'var(--zl-control-bg)',
            border: '1px solid var(--zl-border)',
            color: 'var(--zl-text)',
          }}
        />
      </div>
      <div className="mt-2 text-[11px]" style={{ color: 'var(--zl-text-muted)' }}>
        {filter.key.trim()
          ? `仅匹配 ${labels.sourceName} 顶层字段 ${filter.key.trim()}`
          : `字段为空时在整段 ${labels.sourceName} 中搜索值`}
      </div>
    </div>
  );
}

function Segmented({
  value,
  onChange,
  items,
}: {
  value: string;
  onChange: (value: string) => void;
  items: Array<{ value: string; label: string }>;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {items.map(item => (
        <button
          key={item.value}
          type="button"
          onClick={() => onChange(item.value)}
          className="zl-action-button rounded-lg px-3 py-2 text-xs"
          style={{
            background: value === item.value ? 'rgba(59,130,246,0.15)' : 'transparent',
            border:
              value === item.value
                ? '1px solid rgba(59,130,246,0.32)'
                : '1px solid var(--zl-border)',
            color: value === item.value ? 'var(--zl-accent-text)' : 'var(--zl-text-muted)',
          }}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}

function PageSizePicker({
  value,
  onChange,
}: {
  value: PageSize;
  onChange: (value: PageSize) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm" style={{ color: 'var(--zl-text-muted)' }}>
        显示数量
      </span>
      <Select
        value={String(value)}
        onValueChange={next => onChange(next === 'all' ? 'all' : (Number(next) as PageSize))}
      >
        <SelectTrigger className="h-10 w-28">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {pageSizes.map(item => (
            <SelectItem key={String(item.value)} value={String(item.value)}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function ExportButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="zl-action-button flex h-10 items-center gap-2 rounded-lg border px-3 text-sm"
      style={{
        borderColor: 'rgba(59,130,246,0.38)',
        color: 'var(--zl-accent-text)',
        background: 'rgba(59,130,246,0.1)',
      }}
    >
      <Download size={14} />
      导出
    </button>
  );
}

function Pagination({
  start,
  end,
  total,
  pageSize,
  currentPage,
  pageCount,
  setPage,
}: {
  start: number;
  end: number;
  total: number;
  pageSize: PageSize;
  currentPage: number;
  pageCount: number;
  setPage: React.Dispatch<React.SetStateAction<number>>;
}) {
  return (
    <div
      className="flex shrink-0 flex-wrap items-center justify-between gap-3 text-sm"
      style={{ color: 'var(--zl-text-muted)' }}
    >
      <span>
        显示 {start}-{end} 条，共 {total} 条
      </span>
      {pageSize !== 'all' ? (
        <div className="flex items-center gap-2">
          <PageButton label="首页" disabled={currentPage <= 1} onClick={() => setPage(1)}>
            <ChevronsLeft size={16} />
          </PageButton>
          <PageButton
            label="上一页"
            disabled={currentPage <= 1}
            onClick={() => setPage(value => Math.max(1, value - 1))}
          >
            <ChevronLeft size={16} />
          </PageButton>
          <span className="min-w-[76px] text-center">
            {currentPage} / {pageCount}
          </span>
          <PageButton
            label="下一页"
            disabled={currentPage >= pageCount}
            onClick={() => setPage(value => Math.min(pageCount, value + 1))}
          >
            <ChevronRight size={16} />
          </PageButton>
          <PageButton
            label="尾页"
            disabled={currentPage >= pageCount}
            onClick={() => setPage(pageCount)}
          >
            <ChevronsRight size={16} />
          </PageButton>
        </div>
      ) : null}
    </div>
  );
}

function PageButton({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string;
  disabled: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="zl-action-button flex h-9 w-9 items-center justify-center rounded-lg border disabled:opacity-40"
      style={{
        background: 'var(--zl-card)',
        borderColor: 'var(--zl-border)',
        color: 'var(--zl-text-muted)',
      }}
      aria-label={label}
    >
      {children}
    </button>
  );
}

function TooltipIconButton({
  label,
  onClick,
  children,
}: {
  label: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <AppTooltip label={label} placement="top">
      <button
        type="button"
        onClick={onClick}
        className="zl-action-button inline-flex h-8 w-8 items-center justify-center rounded-lg border"
        style={{
          borderColor: 'var(--zl-border)',
          color: 'var(--zl-text-muted)',
          background: 'rgba(255,255,255,0.03)',
        }}
        aria-label={label}
      >
        {children}
      </button>
    </AppTooltip>
  );
}

function DetailDialog({ detail, onClose }: { detail: DetailItem; onClose: () => void }) {
  if (!detail) return null;
  const title = detail.type === 'tasks' ? '任务详情' : '审计详情';
  const rows = detailRows(detail);
  const extra = extraPayload(detail);
  return (
    <Dialog open={Boolean(detail)} onOpenChange={open => !open && onClose()}>
      <DialogContent className="max-h-[86vh] max-w-4xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className="zl-hidden-scrollbar max-h-[72vh] overflow-y-auto">
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            {rows.map(row => (
              <ReadOnlyField key={row.label} label={row.label} value={row.value} />
            ))}
          </div>
          {extra ? (
            <div className="mt-4">
              <div className="mb-2 text-xs font-semibold" style={{ color: 'var(--zl-text-muted)' }}>
                {extra.title}
              </div>
              <pre className="zl-payload-pre zl-hidden-scrollbar max-h-[32vh] overflow-y-auto overflow-x-hidden whitespace-pre-wrap break-all rounded-lg p-4 text-xs">
                {extra.value}
              </pre>
            </div>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ReadOnlyField({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="zl-dialog-card zl-operations-detail-card rounded-lg p-3">
      <div className="mb-1 text-[11px]" style={{ color: 'var(--zl-text-muted)' }}>
        {label}
      </div>
      <div className="break-words text-sm font-medium" style={{ color: 'var(--zl-text)' }}>
        {value || '-'}
      </div>
    </div>
  );
}

function StatusPill({ value, tone }: { value: string; tone: Tone }) {
  const colors: Record<Tone, [string, string, string]> = {
    green: ['var(--zl-status-green-text)', 'rgba(16,185,129,0.12)', 'rgba(16,185,129,0.24)'],
    red: ['var(--zl-status-red-text)', 'rgba(239,68,68,0.12)', 'rgba(239,68,68,0.24)'],
    yellow: ['var(--zl-status-yellow-text)', 'rgba(245,158,11,0.12)', 'rgba(245,158,11,0.24)'],
    blue: ['var(--zl-status-blue-text)', 'rgba(59,130,246,0.12)', 'rgba(59,130,246,0.24)'],
    gray: ['var(--zl-status-gray-text)', 'rgba(148,163,184,0.1)', 'rgba(148,163,184,0.18)'],
  };
  return (
    <span
      className="inline-flex rounded-md px-2 py-1 text-[11px] font-semibold"
      style={{
        color: colors[tone][0],
        background: colors[tone][1],
        border: `1px solid ${colors[tone][2]}`,
      }}
    >
      {value}
    </span>
  );
}

function paginate<T>(items: T[], pageSize: PageSize, page: number) {
  if (pageSize === 'all') return items;
  return items.slice((page - 1) * pageSize, page * pageSize);
}

function matchesKeyword(text: string, query: string) {
  const keyword = query.trim().toLowerCase();
  return keyword === '' || text.toLowerCase().includes(keyword);
}

function matchesJsonFilter(payload: unknown, filter: JsonFilter) {
  const key = filter.key.trim();
  const value = filter.value.trim().toLowerCase();
  if (!key && !value) return true;
  const parsed = parsePayload(payload);
  if (key && isPlainObject(parsed)) {
    const target = parsed[key];
    if (value)
      return String(target ?? '')
        .toLowerCase()
        .includes(value);
    return target !== undefined;
  }
  if (!value) return true;
  return JSON.stringify(parsed ?? '')
    .toLowerCase()
    .includes(value);
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function parsePayload(value: unknown) {
  if (!value) return null;
  if (typeof value === 'string') {
    try {
      return JSON.parse(value);
    } catch {
      return value;
    }
  }
  return value;
}

function auditFilterPayload(item: AuditEntry) {
  const detail = parsePayload(item.detail);
  return {
    ...(isPlainObject(detail) ? detail : {}),
    id: item.id,
    action: normalizedAuditAction(item.action),
    user: item.user,
    target: item.target,
    resource: auditResource(item),
    resourceType: auditResourceType(item),
    module: item.module,
    result: item.result,
    ipAddress: item.ipAddress || '',
    detail,
  };
}

function taskSearchText(item: RefreshTask) {
  return [
    item.id,
    item.type,
    item.status,
    labelStatus(item.status),
    taskTarget(item),
    taskProgress(item),
    taskError(item),
    JSON.stringify(parsePayload(item.payload) ?? ''),
  ].join(' ');
}

function auditSearchText(item: AuditEntry) {
  return [
    item.id,
    item.action,
    normalizedAuditAction(item.action),
    item.user,
    item.target,
    item.module,
    item.result,
    item.ipAddress ?? '',
    item.detail ?? '',
    labelAction(item.action),
    labelModule(item.module),
    auditResource(item),
  ].join(' ');
}

function taskTarget(item: RefreshTask) {
  const payload = parsePayload(item.payload);
  const parts = taskTargetParts(item.type, isPlainObject(payload) ? payload : undefined);
  return `${parts.type}/${parts.id}`;
}

function taskTargetParts(type: string, payload?: Record<string, unknown>) {
  if (type === 'runtime.refresh.all' || type === 'runtime.refresh.fast') {
    return { type: 'runtime', id: '-' };
  }
  const targetType = payload ? stringField(payload, 'targetType') : '';
  const targetId = payload ? stringField(payload, 'targetId') : '';
  if (targetType) {
    return { type: targetType, id: targetId || '-' };
  }
  if (type === 'runtime.refresh.server') {
    return {
      type: 'server',
      id: payload
        ? stringField(payload, 'serverName') || stringField(payload, 'serverId') || '-'
        : '-',
    };
  }
  if (type === 'runtime.refresh.dns.zone') {
    return {
      type: 'dns.zone',
      id: payload ? stringField(payload, 'zoneName') || stringField(payload, 'zoneId') || '-' : '-',
    };
  }
  if (type === 'runtime.refresh.dhcp.scope') {
    return {
      type: 'dhcp.scope',
      id: payload
        ? stringField(payload, 'scopeName') || stringField(payload, 'scopeExternalId') || '-'
        : '-',
    };
  }
  return {
    type: payload ? stringField(payload, 'targetType') || type || '-' : type || '-',
    id: payload ? stringField(payload, 'targetId') || '-' : '-',
  };
}

function auditResource(item: AuditEntry) {
  return `${auditResourceType(item)}/${item.target || '-'}`;
}

function auditResourceType(item: AuditEntry) {
  const action = normalizedAuditAction(item.action);
  if (action.startsWith('auth.') || action.startsWith('settings.user.')) return 'user';
  if (action.startsWith('settings.role.')) return 'role';
  if (action.startsWith('settings.user_group.')) return 'user_group';
  if (action.startsWith('settings.auth_provider.')) return 'auth_provider';
  if (action.startsWith('settings.notification.')) return 'notification_channel';
  if (action.startsWith('notifications.')) return 'notification';
  if (action.startsWith('settings.base.')) return 'system_setting';
  if (action.startsWith('runtime.')) return 'runtime';
  if (action.startsWith('server.')) return 'server';
  if (action.startsWith('dns.')) return action.includes('.record') ? 'dns.record' : 'dns.zone';
  if (action.startsWith('dhcp.')) {
    if (action.includes('.lease')) return 'dhcp.lease';
    if (action.includes('.reservation')) return 'dhcp.reservation';
    return 'dhcp.scope';
  }
  const module = (item.module || '').toLowerCase();
  if (module === 'system') return 'system';
  if (module === 'server') return 'server';
  if (module === 'dns') return 'dns';
  if (module === 'dhcp') return 'dhcp';
  return item.module || '-';
}

function stringField(payload: Record<string, unknown>, key: string) {
  const value = payload[key];
  return typeof value === 'string' && value.trim() ? value.trim() : '';
}

function taskProgress(task: RefreshTask) {
  const payload = parsePayload(task.payload) as
    | { totalAgents?: number; syncedAgents?: number; failedAgents?: number; message?: string }
    | undefined;
  if (!payload || typeof payload !== 'object') return '-';
  if (typeof payload.totalAgents !== 'number') return payload.message || '-';
  return `${payload.syncedAgents ?? 0}/${payload.totalAgents}，异常 ${payload.failedAgents ?? 0}`;
}

function taskError(task: RefreshTask) {
  const payload = parsePayload(task.payload);
  if (!isPlainObject(payload)) return '-';
  return stringField(payload, 'error') || '-';
}

function detailRows(detail: NonNullable<DetailItem>) {
  if (detail.type === 'tasks') {
    const item = detail.item;
    return [
      { label: '任务 ID', value: item.id },
      { label: '任务类型', value: labelTaskType(item.type) },
      { label: '状态', value: labelStatus(item.status) },
      { label: '目标', value: taskTarget(item) },
      { label: '进度', value: taskProgress(item) },
      { label: '错误信息', value: taskError(item) },
      { label: '创建时间', value: fullTime(item.createdAt) },
      { label: '完成时间', value: item.finishedAt ? fullTime(item.finishedAt) : '-' },
    ];
  }
  const item = detail.item;
  return [
    { label: '审计 ID', value: item.id },
    { label: '动作', value: labelAction(item.action) },
    { label: '用户', value: item.user || '-' },
    { label: '资源', value: auditResource(item) },
    { label: 'IP 地址', value: item.ipAddress || '-' },
    { label: '发生时间', value: fullTime(item.ts) },
  ];
}

function extraPayload(detail: NonNullable<DetailItem>) {
  const value = detail.type === 'tasks' ? detail.item.payload : detail.item.detail;
  const parsed = parsePayload(value);
  if (!parsed) return null;
  return {
    title: detail.type === 'tasks' ? '任务载荷' : '审计元数据',
    value: typeof parsed === 'object' ? JSON.stringify(parsed, null, 2) : String(parsed),
  };
}

function exportPayload(value: unknown) {
  const parsed = parsePayload(value);
  if (!parsed) return '-';
  return typeof parsed === 'object' ? JSON.stringify(parsed) : String(parsed);
}

function fullTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function formatTimeAgo(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const diff = Date.now() - date.getTime();
  if (diff < 60_000) return '刚刚';
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`;
  if (diff < 604_800_000) return `${Math.floor(diff / 86_400_000)} 天前`;
  return date.toLocaleString();
}

function labelStatus(status: string) {
  return (
    {
      queued: '排队中',
      running: '运行中',
      completed: '已完成',
      failed: '失败',
    }[status] ?? status
  );
}

function statusTone(status: string): Tone {
  if (status === 'completed') return 'green';
  if (status === 'failed') return 'red';
  if (status === 'queued') return 'yellow';
  if (status === 'running') return 'blue';
  return 'gray';
}

function labelTaskType(type: string) {
  return type;
}

function labelModule(module: string) {
  return (
    {
      DNS: 'DNS',
      DHCP: 'DHCP',
      Server: '服务器',
      System: '系统',
    }[module] ?? module
  );
}

function labelAction(action: string) {
  return normalizedAuditAction(action);
}

function normalizedAuditAction(action: string) {
  return (
    {
      'User login': 'auth.login',
      'Changed password': 'auth.password.change',
      'Queued refresh': 'runtime.refresh',
      'Created server': 'server.create',
      'Deleted server': 'server.delete',
      'Queued server sync': 'server.sync',
      'Checked server health': 'server.health.check',
      'Created zone': 'dns.zone.create',
      'Queued DNS zone refresh': 'dns.zone.refresh',
      'Deleted zone': 'dns.zone.delete',
      'Created DNS record': 'dns.record.create',
      'Deleted DNS record': 'dns.record.delete',
      'Created DHCP scope': 'dhcp.scope.create',
      'Toggled DHCP scope': 'dhcp.scope.toggle',
      'Deleted DHCP scope': 'dhcp.scope.delete',
      'Released DHCP lease': 'dhcp.lease.release',
      'Created DHCP reservation': 'dhcp.reservation.create',
      'Deleted DHCP reservation': 'dhcp.reservation.delete',
      'Updated system base config': 'settings.base.update',
    }[action] ?? action
  );
}

const taskExportColumns: ExportColumn<RefreshTask>[] = [
  { id: 'id', header: '任务 ID', value: item => item.id },
  { id: 'type', header: '类型', value: item => labelTaskType(item.type) },
  { id: 'status', header: '状态', value: item => labelStatus(item.status) },
  { id: 'target', header: '目标', value: item => taskTarget(item) },
  { id: 'progress', header: '进度', value: item => taskProgress(item) },
  { id: 'error', header: '错误信息', value: item => taskError(item) },
  { id: 'createdAt', header: '创建时间', value: item => fullTime(item.createdAt) },
  {
    id: 'finishedAt',
    header: '完成时间',
    value: item => (item.finishedAt ? fullTime(item.finishedAt) : '-'),
  },
  { id: 'payload', header: '载荷', value: item => exportPayload(item.payload) },
];

const auditExportColumns: ExportColumn<AuditEntry>[] = [
  { id: 'id', header: '审计 ID', value: item => item.id },
  { id: 'action', header: '动作', value: item => labelAction(item.action) },
  { id: 'user', header: '用户', value: item => item.user || '-' },
  { id: 'target', header: '资源', value: item => auditResource(item) },
  { id: 'ip', header: 'IP', value: item => item.ipAddress || '-' },
  { id: 'time', header: '时间', value: item => fullTime(item.ts) },
  { id: 'detail', header: '元数据', value: item => exportPayload(item.detail) },
];
