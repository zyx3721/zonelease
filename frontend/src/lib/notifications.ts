import { api } from './auth';

const NOTIFICATION_REFRESH_EVENT = 'zonelease-notifications-refresh';

export type NotificationLevel = 'info' | 'success' | 'warning' | 'critical' | string;

export type NotificationItem = {
  id: string;
  level: NotificationLevel;
  title: string;
  message: string;
  sourceType: string;
  sourceId: string;
  metadata: Record<string, unknown>;
  readAt?: string;
  dismissedAt?: string;
  createdAt: string;
  updatedAt: string;
};

let pendingNotifications: Promise<{ items: NotificationItem[]; total: number }> | null = null;
let pendingUnreadCount: Promise<{ count: number }> | null = null;

export function emitNotificationRefresh() {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new CustomEvent(NOTIFICATION_REFRESH_EVENT));
}

export function onNotificationRefresh(callback: () => void) {
  if (typeof window === 'undefined') return () => undefined;
  window.addEventListener(NOTIFICATION_REFRESH_EVENT, callback);
  return () => window.removeEventListener(NOTIFICATION_REFRESH_EVENT, callback);
}

export function fetchNotifications(limit = 20) {
  if (limit === 20 && pendingNotifications) return pendingNotifications;
  const request = api<{ items: NotificationItem[]; total: number }>(
    `/api/notifications?limit=${limit}`
  );
  if (limit !== 20) return request;
  pendingNotifications = request.finally(() => {
    pendingNotifications = null;
  });
  return pendingNotifications;
}

export function fetchUnreadNotificationCount() {
  if (pendingUnreadCount) return pendingUnreadCount;
  pendingUnreadCount = api<{ count: number }>('/api/notifications/unread-count').finally(() => {
    pendingUnreadCount = null;
  });
  return pendingUnreadCount;
}

export function markNotificationRead(id: string) {
  return api<{ status: string }>(`/api/notifications/${encodeURIComponent(id)}`, {
    method: 'POST',
  });
}

export function markAllNotificationsRead() {
  return api<{ status: string }>('/api/notifications/read-all', { method: 'POST' });
}

export function clearNotifications() {
  return api<{ status: string }>('/api/notifications/clear', { method: 'POST' });
}
