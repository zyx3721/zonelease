import { AlertTriangle } from 'lucide-react';
import { useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type { PendingDnsDelete } from './types';

export function DnsDeleteConfirmDialog({
  target,
  busy,
  onClose,
  onConfirm,
}: {
  target: PendingDnsDelete | null;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const [visibleTarget, setVisibleTarget] = useState(target);

  useEffect(() => {
    if (target) setVisibleTarget(target);
  }, [target]);

  const displayTarget = target ?? visibleTarget;
  const title = displayTarget?.kind === 'zone' ? '删除 DNS 区域' : '删除 DNS 记录';
  const description =
    displayTarget?.kind === 'zone'
      ? '将删除该区域及其关联记录快照，Windows DNS 侧也会同步删除该区域'
      : '将删除该 DNS 记录，并在 Windows DNS 侧同步执行删除操作';
  return (
    <Dialog open={Boolean(target)} onOpenChange={open => !open && onClose()}>
      <DialogContent className="max-w-xl border-red-400/35 p-7 sm:p-8">
        <DialogHeader className="pr-10 text-left">
          <div className="flex items-start gap-4">
            <span
              className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl border"
              style={{
                background: 'rgba(239,68,68,0.12)',
                borderColor: 'rgba(239,68,68,0.3)',
                color: '#f87171',
              }}
            >
              <AlertTriangle size={24} />
            </span>
            <div className="min-w-0">
              <DialogTitle className="text-2xl font-semibold">{title}</DialogTitle>
              <p className="mt-2 text-base leading-6 text-muted-foreground">{description}</p>
            </div>
          </div>
        </DialogHeader>
        {displayTarget ? (
          <div
            className="mt-2 rounded-xl border p-4"
            style={{
              background: 'var(--zl-control-bg-soft)',
              borderColor: 'var(--zl-border)',
              color: 'var(--zl-text)',
            }}
          >
            <div className="truncate text-lg font-medium">{displayTarget.name}</div>
            <div
              className="mt-2 truncate font-mono text-sm"
              style={{ color: 'var(--zl-text-muted)' }}
            >
              {displayTarget.detail}
            </div>
          </div>
        ) : null}
        <DialogFooter className="mt-2 gap-3 sm:space-x-0">
          <button
            type="button"
            className="zl-action-button rounded-lg border px-6 py-3 text-base disabled:cursor-not-allowed disabled:opacity-60"
            onClick={onClose}
            disabled={busy}
          >
            取消
          </button>
          <button
            type="button"
            className="zl-action-button zl-danger-button rounded-lg border px-6 py-3 text-base font-semibold disabled:cursor-not-allowed disabled:opacity-60"
            onClick={onConfirm}
            disabled={busy}
            style={{
              background: 'rgba(239,68,68,0.12)',
              borderColor: 'rgba(239,68,68,0.35)',
              color: 'var(--zl-status-red-text)',
            }}
          >
            {busy ? '删除中' : '确认删除'}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
