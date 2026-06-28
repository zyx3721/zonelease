import { AlertTriangle, CheckCircle2, Info, XCircle } from 'lucide-react';
import { forwardRef } from 'react';
import type { NotificationItem } from '@/lib/notifications';

export type NotificationPanelPosition = {
  top: number;
  right: number;
};

type NotificationPanelProps = {
  items: NotificationItem[];
  unreadCount: number;
  canManage: boolean;
  position: NotificationPanelPosition;
  onReadAll: () => void;
  onClear: () => void;
  onOpen: (item: NotificationItem) => void;
  onClose: () => void;
};

export const NotificationPanel = forwardRef<HTMLDivElement, NotificationPanelProps>(
  function NotificationPanel(
    { items, unreadCount, canManage, position, onReadAll, onClear, onOpen, onClose },
    ref
  ) {
    return (
      <div
        ref={ref}
        className="zl-notification-panel fixed z-[1300] flex w-[min(420px,calc(100vw-2rem))] flex-col overflow-hidden rounded-xl shadow-2xl"
        role="dialog"
        aria-label="通知消息"
        style={{ top: position.top, right: position.right }}
      >
        <div
          className="flex items-center justify-between gap-3 px-5 py-4"
          style={{ borderBottom: '1px solid var(--zl-border)' }}
        >
          <div>
            <div className="text-sm font-semibold" style={{ color: 'var(--zl-text)' }}>
              通知消息
            </div>
            <div className="mt-1 text-xs" style={{ color: 'var(--zl-text-muted)' }}>
              {unreadCount > 0 ? `${unreadCount} 条未读消息` : '暂无未读消息'}
            </div>
          </div>
          <div className="flex items-center gap-3 text-xs">
            {canManage ? (
              <>
                <button
                  type="button"
                  onClick={onReadAll}
                  className="zl-action-button"
                  style={{ color: 'var(--zl-accent-text)' }}
                >
                  全部已读
                </button>
                <button
                  type="button"
                  onClick={onClear}
                  className="zl-action-button"
                  style={{ color: '#f87171' }}
                >
                  清空
                </button>
              </>
            ) : null}
            <button
              type="button"
              onClick={onClose}
              className="zl-action-button md:hidden"
              style={{ color: 'var(--zl-text-muted)' }}
            >
              关闭
            </button>
          </div>
        </div>
        <div className="zl-hidden-scrollbar max-h-[28rem] overflow-y-auto">
          {items.length === 0 ? (
            <div
              className="px-5 py-10 text-center text-sm"
              style={{ color: 'var(--zl-text-muted)' }}
            >
              没有更多了
            </div>
          ) : (
            items.map(item => {
              const Icon = notificationIcon(item.level);
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => onOpen(item)}
                  className="zl-action-button flex w-full gap-3 px-5 py-4 text-left"
                  style={{ borderBottom: '1px solid rgba(148,163,184,0.14)' }}
                >
                  <div
                    className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
                    style={{
                      color: notificationTone(item.level),
                      background: `${notificationTone(item.level)}18`,
                      border: `1px solid ${notificationTone(item.level)}33`,
                    }}
                  >
                    <Icon size={17} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      {!item.readAt ? (
                        <span
                          className="h-1.5 w-1.5 shrink-0 rounded-full"
                          style={{ background: '#ef4444' }}
                        />
                      ) : null}
                      <span
                        className="truncate text-sm font-semibold"
                        style={{ color: 'var(--zl-text)' }}
                      >
                        {item.title}
                      </span>
                      <span
                        className="ml-auto shrink-0 text-xs"
                        style={{ color: 'var(--zl-text-muted)' }}
                      >
                        {formatTimeAgo(item.createdAt)}
                      </span>
                    </div>
                    <div
                      className="mt-1 line-clamp-2 text-sm leading-5"
                      style={{ color: 'var(--zl-text-muted)' }}
                    >
                      {item.message}
                    </div>
                  </div>
                </button>
              );
            })
          )}
        </div>
      </div>
    );
  }
);

function notificationIcon(level: string) {
  if (level === 'success') return CheckCircle2;
  if (level === 'critical') return XCircle;
  if (level === 'warning') return AlertTriangle;
  return Info;
}

function notificationTone(level: string) {
  if (level === 'success') return '#34d399';
  if (level === 'critical') return '#f87171';
  if (level === 'warning') return '#f59e0b';
  return '#60a5fa';
}

function formatTimeAgo(value: string) {
  const time = new Date(value).getTime();
  const diff = Date.now() - time;
  if (!Number.isFinite(time) || diff < 0) return '刚刚';
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diff < minute) return '刚刚';
  if (diff < hour) return `${Math.floor(diff / minute)}分钟前`;
  if (diff < day) return `${Math.floor(diff / hour)}小时前`;
  return `${Math.floor(diff / day)}天前`;
}
