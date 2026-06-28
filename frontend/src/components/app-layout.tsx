import { Link, Outlet, useLocation, useNavigate, useRouterState } from '@tanstack/react-router';
import type { CSSProperties, ReactNode } from 'react';
import { createPortal } from 'react-dom';
import {
  Bell,
  ChevronDown,
  ChevronRight,
  DatabaseZap,
  Globe,
  LayoutDashboard,
  LogOut,
  Moon,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  KeyRound,
  RefreshCw,
  ScrollText,
  Server,
  Settings,
  Sun,
  User,
  type LucideIcon,
} from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { toast } from 'sonner';
import {
  clearSession,
  AUTH_SESSION_CHANGED_EVENT,
  fetchCurrentUser,
  getStoredUser,
  getAuthToken,
  logout,
  userHasAnyPermission,
} from '@/lib/auth';
import { useBaseConfig } from '@/lib/branding';
import { createRefresh, useDB } from '@/lib/dns-dhcp-store';
import { taskToastDoneOptionsFor, taskToastOptionsFor } from '@/lib/task-toast';
import { emitZoneLeaseRefresh, runtimeEventsUrl } from '@/lib/refresh';
import {
  applyZlTheme,
  cn,
  getInitialZlTheme,
  persistZlTheme,
  toggleZlTheme,
  type ZlTheme,
} from '@/lib/utils';
import { AppTooltip } from '@/components/app-tooltip';
import { NotificationPanel, type NotificationPanelPosition } from '@/components/notification-panel';
import { PasswordDialog } from '@/components/password-dialog';
import {
  clearNotifications,
  fetchNotifications,
  fetchUnreadNotificationCount,
  markAllNotificationsRead,
  markNotificationRead,
  onNotificationRefresh,
  type NotificationItem,
} from '@/lib/notifications';

interface NavItem {
  to: string;
  label: string;
  description: string;
  permissions: string[];
  icon: LucideIcon;
}

const items: NavItem[] = [
  {
    to: '/',
    label: '仪表板',
    description: '运行态总览',
    permissions: ['dashboard.read'],
    icon: LayoutDashboard,
  },
  {
    to: '/dns',
    label: 'DNS 管理',
    description: '区域与记录',
    permissions: ['dns.read'],
    icon: Globe,
  },
  {
    to: '/dhcp',
    label: 'DHCP 管理',
    description: '作用域与租约',
    permissions: ['dhcp.read'],
    icon: Network,
  },
  {
    to: '/audit',
    label: '任务 / 审计',
    description: '后台任务与操作审计',
    permissions: ['audit.read'],
    icon: ScrollText,
  },
  {
    to: '/settings',
    label: 'Agent 管理',
    description: 'Windows 服务器接入',
    permissions: ['servers.read'],
    icon: Server,
  },
  {
    to: '/system',
    label: '系统配置',
    description: '管理平台用户、认证与通知媒介',
    permissions: [
      'settings.base.read',
      'settings.users.read',
      'settings.auth.read',
      'settings.notifications.read',
    ],
    icon: Settings,
  },
];

const sidebarItemHeight = 46;
const sidebarItemGap = 4;
const sidebarTransitionUnitMs = 1000;
const sidebarTransitionMaxMs = 3000;

interface RuntimeRefreshEvent {
  taskId?: string;
  status?: string;
  message?: string;
  payload?: {
    totalAgents?: number;
    startedAgents?: number;
    syncedAgents?: number;
    failedAgents?: number;
    skippedAgents?: number;
    currentAgent?: string;
    agentEvent?: RefreshAgentEvent;
  };
}

interface RefreshAgentEvent {
  id?: string;
  name?: string;
  status?: 'running' | 'completed' | 'failed' | 'skipped' | string;
  error?: string;
  warn?: string;
}

interface ServiceHealth {
  status: 'online' | 'offline' | 'unknown';
  error?: string;
}

interface PlatformHealth {
  status: 'ok' | 'degraded' | 'unknown';
  services: {
    postgresql: ServiceHealth;
    redis: ServiceHealth;
  };
}

function matchesNavPath(to: string, path: string) {
  return to === '/' ? path === '/' : path === to || path.startsWith(`${to}/`);
}

function parseRuntimeRefreshEvent(event: MessageEvent): RuntimeRefreshEvent {
  try {
    return JSON.parse(event.data) as RuntimeRefreshEvent;
  } catch {
    return {};
  }
}

function formatRefreshToast(event: RuntimeRefreshEvent): string {
  const status = event.status ?? '';
  if (status === 'queued') return '全量刷新任务已排队';
  if (status === 'running') return 'Agent 同步已开始';
  return event.message?.trim() || 'Agent 正在同步';
}

function formatRefreshDoneToast(event: RuntimeRefreshEvent): string {
  const total = event.payload?.totalAgents ?? 0;
  if (event.status === 'failed' && total <= 0) return event.message?.trim() || '刷新任务失败';
  const synced = event.payload?.syncedAgents ?? total;
  const failed = event.payload?.failedAgents ?? 0;
  const skipped = event.payload?.skippedAgents ?? 0;
  const done = total > 0 ? Math.min(total, synced + failed + skipped) : synced;
  if (failed > 0 && total > 0) return `[${done}/${total}] 全量同步完成，异常 ${failed}`;
  if (skipped > 0 && total > 0) return `[${done}/${total}] 全量同步完成，跳过 ${skipped}`;
  return total > 0 ? `[${done}/${total}] 所有 Agent 已同步完成` : '所有 Agent 已同步完成';
}

function refreshAgentKey(agent: RefreshAgentEvent) {
  return agent.id?.trim() || agent.name?.trim() || '';
}

function formatAgentRefreshToast(agent: RefreshAgentEvent) {
  const name = agent.name?.trim() || 'Agent';
  if (agent.status === 'completed') return `${name} 同步完成`;
  if (agent.status === 'skipped')
    return agent.warn?.trim() ? agent.warn.trim() : `${name} 已处于正在同步中，跳过同步`;
  if (agent.status === 'failed')
    return agent.error?.trim() ? `${name} 同步失败\n${agent.error}` : `${name} 同步失败`;
  return `${name} 正在同步`;
}

const unknownHealth: PlatformHealth = {
  status: 'unknown',
  services: {
    postgresql: { status: 'unknown' },
    redis: { status: 'unknown' },
  },
};

let pendingPlatformHealth: Promise<PlatformHealth> | null = null;

async function fetchPlatformHealth(): Promise<PlatformHealth> {
  if (pendingPlatformHealth) return pendingPlatformHealth;
  pendingPlatformHealth = fetch('/api/health')
    .then(response => {
      if (!response.ok) throw new Error('读取平台健康状态失败');
      return response.json() as Promise<Partial<PlatformHealth>>;
    })
    .then(body => ({
      status: body.status ?? 'unknown',
      services: {
        postgresql: body.services?.postgresql ?? { status: 'unknown' },
        redis: body.services?.redis ?? { status: 'unknown' },
      },
    }))
    .finally(() => {
      pendingPlatformHealth = null;
    });
  return pendingPlatformHealth;
}

function serviceHealthText(status: ServiceHealth['status']) {
  if (status === 'online') return '在线';
  if (status === 'offline') return '离线';
  return '未知';
}

function serviceHealthColor(status: ServiceHealth['status']) {
  if (status === 'online') return '#22c55e';
  if (status === 'offline') return '#ef4444';
  return 'var(--zl-text-muted)';
}

export function RequireAuth({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const [token, setToken] = useState(() => getAuthToken());
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    const syncToken = () => setToken(getAuthToken());
    window.addEventListener(AUTH_SESSION_CHANGED_EVENT, syncToken);
    window.addEventListener('storage', syncToken);
    return () => {
      window.removeEventListener(AUTH_SESSION_CHANGED_EVENT, syncToken);
      window.removeEventListener('storage', syncToken);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    if (!token) {
      void navigate({ to: '/login', replace: true });
      setChecking(false);
      return;
    }
    void fetchCurrentUser()
      .then(() => {
        if (cancelled) return;
        setChecking(false);
      })
      .catch(() => {
        if (cancelled) return;
        void navigate({ to: '/login', replace: true });
        setChecking(false);
      });
    return () => {
      cancelled = true;
    };
  }, [navigate, token]);

  if (checking || !token) return null;
  return <>{children}</>;
}

export function AppLayout({ children }: { children?: ReactNode }) {
  const location = useLocation();
  const navigate = useNavigate();
  const db = useDB();
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [theme, setTheme] = useState<ZlTheme>(getInitialZlTheme);
  const [user, setUser] = useState(() => getStoredUser());
  const [loggingOut, setLoggingOut] = useState(false);
  const baseConfig = useBaseConfig();
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false);
  const [transitioningTo, setTransitioningTo] = useState('');
  const [notificationOpen, setNotificationOpen] = useState(false);
  const [notifications, setNotifications] = useState<NotificationItem[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [platformHealth, setPlatformHealth] = useState<PlatformHealth>(unknownHealth);
  const [notificationPanelPosition, setNotificationPanelPosition] =
    useState<NotificationPanelPosition>({
      top: 0,
      right: 16,
    });
  const routeStatus = useRouterState({
    select: state => ({
      pending: state.status === 'pending' || state.isLoading || state.isTransitioning,
      matchPath: state.matches.at(-1)?.pathname ?? state.location.pathname,
    }),
  });
  const userMenuRef = useRef<HTMLDivElement | null>(null);
  const notificationRef = useRef<HTMLDivElement | null>(null);
  const notificationPanelRef = useRef<HTMLDivElement | null>(null);
  const manualRefreshTaskRef = useRef('');
  const manualRefreshToastRef = useRef<string | number | null>(null);
  const manualRefreshAgentToastsRef = useRef(new Map<string, string | number>());
  const finishedRefreshTaskRef = useRef<RuntimeRefreshEvent | null>(null);
  const path = location.pathname;
  const visibleItems = useMemo(
    () => items.filter(item => userHasAnyPermission(user, item.permissions)),
    [user]
  );
  const canReadNotifications = userHasAnyPermission(user, ['notifications.read']);
  const canManageNotifications = userHasAnyPermission(user, ['notifications.manage']);
  const current = visibleItems.find(item => matchesNavPath(item.to, path)) ?? visibleItems[0];
  const hideRouteHeading = path === '/audit' || path === '/dns' || path === '/dhcp';
  const routeDescription =
    current?.to === '/settings'
      ? `Windows 服务器管理，共 ${db.servers.length} 台服务器`
      : current?.description;
  const routeAllowed = visibleItems.some(item => matchesNavPath(item.to, path));
  const redirectingUnauthorizedRoute = visibleItems.length > 0 && !routeAllowed;
  const previousNavIndexRef = useRef(0);
  const [sidebarIndicatorDuration, setSidebarIndicatorDuration] = useState(0);

  useEffect(() => {
    applyZlTheme(theme);
    persistZlTheme(theme);
  }, [theme]);

  useEffect(() => {
    const syncUser = () => setUser(getStoredUser());
    window.addEventListener(AUTH_SESSION_CHANGED_EVENT, syncUser);
    window.addEventListener('storage', syncUser);
    return () => {
      window.removeEventListener(AUTH_SESSION_CHANGED_EVENT, syncUser);
      window.removeEventListener('storage', syncUser);
    };
  }, []);

  useEffect(() => {
    if (!redirectingUnauthorizedRoute || !current) return;
    void navigate({ to: current.to as never, replace: true });
  }, [current, navigate, redirectingUnauthorizedRoute]);

  useEffect(() => {
    if (!loggingOut && visibleItems.length === 0) {
      toast.error('当前用户无权执行任何操作', { id: 'no-user-permissions' });
    }
  }, [loggingOut, visibleItems.length]);

  useEffect(() => {
    const source = new EventSource(runtimeEventsUrl());
    const handleFullRefreshEvent = (event: MessageEvent) => {
      const refreshEvent = parseRuntimeRefreshEvent(event);
      if (['queued', 'running', 'progress'].includes(refreshEvent.status ?? '')) {
        if (manualRefreshTaskRef.current === refreshEvent.taskId) {
          const agentEvent = refreshEvent.payload?.agentEvent;
          const agentKey = agentEvent ? refreshAgentKey(agentEvent) : '';
          if (refreshEvent.status === 'progress' && agentEvent && agentKey) {
            const toastId = manualRefreshAgentToastsRef.current.get(agentKey);
            const agentFinished = ['completed', 'failed', 'skipped'].includes(agentEvent.status ?? '');
            const nextOptions =
              agentFinished ? taskToastDoneOptionsFor(toastId) : taskToastOptionsFor(toastId);
            const nextToastId =
              agentEvent.status === 'failed'
                ? toast.error(formatAgentRefreshToast(agentEvent), nextOptions)
                : agentEvent.status === 'completed'
                  ? toast.success(formatAgentRefreshToast(agentEvent), nextOptions)
                  : agentEvent.status === 'skipped'
                    ? toast.warning(formatAgentRefreshToast(agentEvent), nextOptions)
                    : toast.loading(formatAgentRefreshToast(agentEvent), nextOptions);
            manualRefreshAgentToastsRef.current.set(agentKey, nextToastId);
            if (agentFinished) {
              window.setTimeout(() => {
                manualRefreshAgentToastsRef.current.delete(agentKey);
              }, 3000);
            }
          } else {
            toast.loading(
              formatRefreshToast(refreshEvent),
              taskToastOptionsFor(manualRefreshToastRef.current ?? undefined)
            );
          }
        }
        return;
      }
      if (['success', 'completed', 'failed'].includes(refreshEvent.status ?? '')) {
        if (refreshEvent.taskId) {
          finishedRefreshTaskRef.current = refreshEvent;
        }
        const manualTaskID = manualRefreshTaskRef.current;
        if (manualTaskID === refreshEvent.taskId) {
          setRefreshing(false);
          manualRefreshTaskRef.current = '';
          if (manualRefreshToastRef.current !== null) {
            toast.dismiss(manualRefreshToastRef.current);
          }
          if (refreshEvent.status === 'failed') {
            toast.error(formatRefreshDoneToast(refreshEvent), taskToastDoneOptionsFor());
          } else {
            toast.success(formatRefreshDoneToast(refreshEvent), taskToastDoneOptionsFor());
          }
          manualRefreshToastRef.current = null;
        }
        emitZoneLeaseRefresh();
        void loadNotifications().catch(() => undefined);
      }
    };
    source.addEventListener('runtime.refresh.all', handleFullRefreshEvent);
    source.addEventListener('runtime.refresh.dns.all', handleFullRefreshEvent);
    source.addEventListener('runtime.refresh.dhcp.all', handleFullRefreshEvent);
    source.addEventListener('runtime.refresh.dns.zone', () => {
      emitZoneLeaseRefresh();
    });
    source.addEventListener('runtime.refresh.dhcp.scope', () => {
      emitZoneLeaseRefresh();
    });
    source.addEventListener('runtime.refresh.server', () => {
      emitZoneLeaseRefresh();
    });
    source.addEventListener('runtime.updated', () => {
      setRefreshing(false);
      emitZoneLeaseRefresh();
      void loadNotifications().catch(() => undefined);
    });
    source.onerror = () => {
      source.close();
    };
    return () => {
      source.close();
    };
  }, []);

  useEffect(() => {
    const close = (event: MouseEvent) => {
      const target = event.target as Node;
      if (!userMenuRef.current?.contains(target)) setUserMenuOpen(false);
      if (
        notificationOpen &&
        !notificationRef.current?.contains(target) &&
        !notificationPanelRef.current?.contains(target)
      ) {
        setNotificationOpen(false);
      }
    };
    window.addEventListener('mousedown', close);
    return () => window.removeEventListener('mousedown', close);
  }, [notificationOpen]);

  useEffect(() => {
    void loadNotifications().catch(() => undefined);
    const offNotificationRefresh = onNotificationRefresh(() => {
      void loadNotifications().catch(() => undefined);
    });
    return offNotificationRefresh;
  }, []);

  useEffect(() => {
    let cancelled = false;
    const loadHealth = () => {
      void fetchPlatformHealth()
        .then(next => {
          if (!cancelled) {
            setPlatformHealth(next);
            void loadNotifications().catch(() => undefined);
          }
        })
        .catch(() => {
          if (!cancelled) setPlatformHealth(unknownHealth);
        });
    };
    loadHealth();
    const timer = window.setInterval(loadHealth, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    if (!notificationOpen) return;
    updateNotificationPanelPosition();
    window.addEventListener('resize', updateNotificationPanelPosition);
    window.addEventListener('scroll', updateNotificationPanelPosition, true);
    return () => {
      window.removeEventListener('resize', updateNotificationPanelPosition);
      window.removeEventListener('scroll', updateNotificationPanelPosition, true);
    };
  }, [notificationOpen]);

  async function loadNotifications() {
    if (!getAuthToken() || !canReadNotifications) {
      setNotifications([]);
      setUnreadCount(0);
      return;
    }
    const [items, count] = await Promise.all([
      fetchNotifications(20),
      fetchUnreadNotificationCount(),
    ]);
    setNotifications(items.items);
    setUnreadCount(count.count);
  }

  function updateNotificationPanelPosition() {
    const rect = notificationRef.current?.getBoundingClientRect();
    if (!rect) return;
    setNotificationPanelPosition({
      top: Math.round(rect.bottom + 8),
      right: Math.max(16, Math.round(window.innerWidth - rect.right)),
    });
  }

  function renderHealthTooltip() {
    const rows: Array<[string, ServiceHealth]> = [
      ['PostgreSQL', platformHealth.services.postgresql],
      ['Redis', platformHealth.services.redis],
    ];
    return (
      <div className="w-[220px] rounded-xl p-1">
        <div className="mb-2 flex items-center justify-between gap-3">
          <span className="text-xs font-semibold" style={{ color: 'var(--zl-text)' }}>
            平台服务状态
          </span>
          <span
            className="rounded-full px-2 py-0.5 text-[10px] font-semibold"
            style={{
              color: platformHealth.status === 'ok' ? '#16a34a' : 'var(--zl-text-muted)',
              background:
                platformHealth.status === 'ok' ? 'rgba(34,197,94,0.12)' : 'rgba(148,163,184,0.12)',
            }}
          >
            {platformHealth.status === 'ok'
              ? '运行正常'
              : platformHealth.status === 'degraded'
                ? '部分异常'
                : '状态未知'}
          </span>
        </div>
        <div className="space-y-2">
          {rows.map(([name, item]) => (
            <div
              key={name}
              className="flex items-center justify-between gap-3 rounded-lg border px-2.5 py-2"
              style={{
                borderColor: 'var(--zl-popover-border)',
                background: 'rgba(148,163,184,0.08)',
              }}
            >
              <div className="flex min-w-0 items-center gap-2">
                <span
                  className="h-2 w-2 shrink-0 rounded-full"
                  style={{ background: serviceHealthColor(item.status) }}
                />
                <span className="truncate text-xs" style={{ color: 'var(--zl-text)' }}>
                  {name}
                </span>
              </div>
              <span
                className="shrink-0 text-xs font-semibold"
                style={{ color: serviceHealthColor(item.status) }}
              >
                {serviceHealthText(item.status)}
              </span>
            </div>
          ))}
        </div>
      </div>
    );
  }

  function handleNotificationToggle() {
    setNotificationOpen(current => {
      const next = !current;
      if (next) {
        updateNotificationPanelPosition();
        void loadNotifications().catch(() => undefined);
      }
      return next;
    });
  }

  async function handleReadAllNotifications() {
    try {
      await markAllNotificationsRead();
      await loadNotifications();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '标记通知已读失败');
    }
  }

  async function handleClearNotifications() {
    try {
      await clearNotifications();
      await loadNotifications();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '清空通知失败');
    }
  }

  async function handleNotificationOpen(item: NotificationItem) {
    if (canManageNotifications && !item.readAt) {
      await markNotificationRead(item.id).catch(() => undefined);
      await loadNotifications().catch(() => undefined);
    }
    setNotificationOpen(false);
    void navigate({ to: '/audit' });
  }

  async function handleRefresh() {
    if (!userHasAnyPermission(user, ['refresh.manage'])) {
      toast.error('当前用户无权执行此操作');
      return;
    }
    setRefreshing(true);
    const toastId = toast.loading('全量刷新任务已排队', taskToastOptionsFor());
    manualRefreshToastRef.current = toastId;
    try {
      const task = await createRefresh();
      manualRefreshTaskRef.current = task.id;
      const finishedEvent = finishedRefreshTaskRef.current;
      if (finishedEvent?.taskId === task.id) {
        setRefreshing(false);
        if (finishedEvent.status === 'failed') {
          toast.dismiss(toastId);
          toast.error(formatRefreshDoneToast(finishedEvent), taskToastDoneOptionsFor());
        } else {
          toast.dismiss(toastId);
          toast.success(formatRefreshDoneToast(finishedEvent), taskToastDoneOptionsFor());
        }
        manualRefreshToastRef.current = null;
      } else {
        toast.loading('全量刷新任务已排队', taskToastOptionsFor(toastId));
      }
    } catch (error) {
      setRefreshing(false);
      toast.error(
        error instanceof Error ? error.message : '刷新任务提交失败',
        taskToastDoneOptionsFor(toastId)
      );
      manualRefreshToastRef.current = null;
    }
  }

  async function handleLogout() {
    setLoggingOut(true);
    setUserMenuOpen(false);
    setUser(null);
    toast.dismiss('no-user-permissions');
    void logout({ waitForRemote: false });
    toast.success('已退出登录');
    void navigate({ to: '/login', replace: true });
  }

  function handlePasswordChanged() {
    clearSession();
    void navigate({ to: '/login', replace: true });
  }

  const activeIndex = useMemo(
    () =>
      Math.max(
        0,
        visibleItems.findIndex(item => item.to === current?.to)
      ),
    [current?.to, visibleItems]
  );

  useEffect(() => {
    const previousIndex = previousNavIndexRef.current;
    const distance = Math.abs(activeIndex - previousIndex);
    setSidebarIndicatorDuration(
      Math.min(sidebarTransitionMaxMs, distance * sidebarTransitionUnitMs)
    );
    previousNavIndexRef.current = activeIndex;
  }, [activeIndex]);

  useEffect(() => {
    if (!transitioningTo) return;
    if (routeStatus.matchPath !== transitioningTo) return;
    const frame = window.requestAnimationFrame(() => {
      setTransitioningTo('');
    });
    return () => window.cancelAnimationFrame(frame);
  }, [routeStatus.matchPath, transitioningTo]);

  return (
    <div
      data-cmp="ZlLayout"
      className="flex h-screen w-full overflow-hidden"
      style={{ background: 'var(--zl-bg)', color: 'var(--zl-text)' }}
    >
      <aside
        className={cn(
          'zl-hidden-scrollbar z-30 flex flex-col overflow-y-auto border-r transition-all duration-300',
          sidebarOpen ? 'w-[240px] min-w-[240px]' : 'w-16 min-w-16'
        )}
        style={{
          background: 'var(--zl-sidebar)',
          borderColor: 'var(--zl-border)',
          boxShadow: 'var(--zl-shell-shadow)',
        }}
      >
        <div
          className="flex min-h-16 items-center gap-3 border-b px-3 py-4"
          style={{ borderColor: 'var(--zl-border)' }}
        >
          <img
            src={baseConfig.iconData}
            alt={baseConfig.appName}
            className="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          {sidebarOpen ? (
            <div className="min-w-0 flex-1 overflow-hidden transition-all duration-200">
              <div className="zl-gradient-text truncate text-base font-bold">
                {baseConfig.appName}
              </div>
              <div
                className="truncate whitespace-nowrap text-xs tracking-widest"
                style={{ color: 'var(--zl-text-muted)' }}
              >
                {baseConfig.appSubtitle}
              </div>
            </div>
          ) : null}
        </div>

        <nav className="relative flex-1 px-2 py-4">
          {visibleItems.length > 0 ? (
            <span
              className="zl-sidebar-active-indicator"
              aria-hidden="true"
              style={
                {
                  '--zl-sidebar-active-top': `${activeIndex * (sidebarItemHeight + sidebarItemGap)}px`,
                  '--zl-sidebar-active-duration': `${sidebarIndicatorDuration}ms`,
                } as CSSProperties
              }
            />
          ) : null}
          <div>
            {visibleItems.map(item => {
              const active = matchesNavPath(item.to, path);
              const Icon = item.icon;
              return (
                <Link
                  key={item.to}
                  to={item.to}
                  aria-current={active ? 'page' : undefined}
                  className={cn(
                    'zl-sidebar-button relative z-10 mb-1 flex h-[46px] items-center rounded-lg text-sm transition-all duration-150',
                    sidebarOpen ? 'justify-start px-3' : 'justify-center px-2',
                    active && 'zl-sidebar-item-active'
                  )}
                  style={{ color: active ? 'var(--zl-accent)' : 'var(--zl-text-muted)' }}
                  preload={false}
                  onClick={() => {
                    if (!active) setTransitioningTo(item.to);
                  }}
                >
                  <Icon size={17} />
                  {sidebarOpen ? (
                    <span className="ml-3 min-w-0 flex-1 truncate font-medium">{item.label}</span>
                  ) : null}
                  {active && sidebarOpen ? (
                    <span className="ml-auto h-1.5 w-1.5 rounded-full bg-[var(--zl-accent)]" />
                  ) : null}
                </Link>
              );
            })}
          </div>
        </nav>

        <div
          className="flex items-center justify-center border-t py-4"
          style={{ borderColor: 'var(--zl-border)' }}
        >
          <button
            type="button"
            onClick={() => setSidebarOpen(value => !value)}
            className="zl-action-button flex h-9 w-9 items-center justify-center rounded-lg border"
            style={{ color: 'var(--zl-accent-text)' }}
            aria-label={sidebarOpen ? '收起侧边栏' : '展开侧边栏'}
          >
            <AppTooltip label={sidebarOpen ? '收起侧边栏' : '展开侧边栏'} placement="top">
              {sidebarOpen ? <PanelLeftClose size={16} /> : <PanelLeftOpen size={16} />}
            </AppTooltip>
          </button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header
          className="relative z-20 flex h-16 shrink-0 items-center justify-between border-b px-6 backdrop-blur"
          style={{
            background: 'var(--zl-header)',
            borderColor: 'var(--zl-border)',
            boxShadow: 'var(--zl-header-shadow)',
          }}
        >
          <div className="flex min-w-0 items-center gap-2">
            <span className="text-sm" style={{ color: 'var(--zl-text-muted)' }}>
              {baseConfig.appName} 控制台
            </span>
            <ChevronRight size={14} style={{ color: 'var(--zl-text-muted)' }} />
            <span className="truncate text-sm font-medium" style={{ color: 'var(--zl-text)' }}>
              {current?.label ?? '无可用页面'}
            </span>
          </div>
          <div className="flex items-center gap-3">
            <AppTooltip label={renderHealthTooltip()} placement="bottom">
              <div
                className="zl-health-status-button hidden h-[42px] w-[42px] items-center justify-center rounded-lg border md:flex"
                style={
                  {
                    '--zl-health-status-color':
                      platformHealth.status === 'ok'
                        ? 'var(--zl-accent-text)'
                        : 'var(--zl-text-muted)',
                    '--zl-health-status-bg':
                      platformHealth.status === 'ok'
                        ? 'rgba(59,130,246,0.1)'
                        : 'rgba(148,163,184,0.1)',
                    '--zl-health-status-border':
                      platformHealth.status === 'ok'
                        ? 'rgba(59,130,246,0.24)'
                        : 'rgba(148,163,184,0.28)',
                  } as CSSProperties
                }
                aria-label="平台服务状态"
              >
                <DatabaseZap size={17} />
              </div>
            </AppTooltip>
            {userHasAnyPermission(user, ['refresh.manage']) ? (
              <AppTooltip label="全量刷新所有信息" placement="bottom">
                <button
                  type="button"
                  onClick={handleRefresh}
                  disabled={refreshing}
                  className="zl-action-button flex h-[42px] w-[42px] items-center justify-center rounded-lg border disabled:cursor-not-allowed disabled:opacity-60"
                  style={{
                    background: refreshing ? 'rgba(59,130,246,0.1)' : 'var(--zl-control-bg)',
                    borderColor: refreshing ? 'rgba(59,130,246,0.42)' : 'var(--zl-border)',
                    color: refreshing ? 'var(--zl-accent-text)' : 'var(--zl-text-muted)',
                  }}
                  aria-label="全量刷新所有信息"
                >
                  <RefreshCw size={17} className={refreshing ? 'animate-spin' : ''} />
                </button>
              </AppTooltip>
            ) : null}
            {canReadNotifications ? (
              <div ref={notificationRef} className="relative">
                <AppTooltip label="通知消息" placement="bottom">
                  <button
                    type="button"
                    onClick={handleNotificationToggle}
                    className="zl-action-button relative flex h-[42px] w-[42px] items-center justify-center rounded-lg border"
                    style={{
                      background: 'var(--zl-control-bg)',
                      borderColor: notificationOpen
                        ? 'rgba(59,130,246,0.48)'
                        : 'var(--zl-border)',
                      color: 'var(--zl-text-muted)',
                    }}
                    aria-label="通知消息"
                    aria-haspopup="dialog"
                    aria-expanded={notificationOpen}
                  >
                    <Bell size={17} />
                    {unreadCount > 0 ? (
                      <span
                        className="absolute -right-1 -top-1 flex h-5 min-w-5 items-center justify-center rounded-full px-1 text-[11px] font-semibold"
                        style={{
                          background: '#ef4444',
                          color: '#fff',
                          border: '2px solid var(--zl-header)',
                        }}
                      >
                        {unreadCount > 99 ? '99+' : unreadCount}
                      </span>
                    ) : null}
                  </button>
                </AppTooltip>
                {notificationOpen && typeof document !== 'undefined'
                  ? createPortal(
                      <NotificationPanel
                        ref={notificationPanelRef}
                        items={notifications}
                        unreadCount={unreadCount}
                        canManage={canManageNotifications}
                        position={notificationPanelPosition}
                        onReadAll={() => void handleReadAllNotifications()}
                        onClear={() => void handleClearNotifications()}
                        onOpen={item => void handleNotificationOpen(item)}
                        onClose={() => setNotificationOpen(false)}
                      />,
                      document.body
                    )
                  : null}
              </div>
            ) : null}
            <AppTooltip
              label={theme === 'dark' ? '切换浅色背景' : '切换深色背景'}
              placement="bottom"
            >
              <button
                type="button"
                onClick={() => setTheme(toggleZlTheme)}
                className="zl-action-button flex h-[42px] w-[42px] items-center justify-center rounded-lg border"
                style={{
                  background: 'var(--zl-control-bg)',
                  borderColor: 'var(--zl-border)',
                  color: 'var(--zl-text-muted)',
                }}
                aria-label={theme === 'dark' ? '切换浅色背景' : '切换深色背景'}
              >
                {theme === 'dark' ? <Sun size={17} /> : <Moon size={17} />}
              </button>
            </AppTooltip>
            <div ref={userMenuRef} className="relative">
              <AppTooltip
                label={user?.displayName || user?.username || 'admin'}
                placement="bottom"
                align="end"
              >
                <button
                  type="button"
                  onClick={() => setUserMenuOpen(value => !value)}
                  className="zl-action-button grid h-[42px] w-[150px] grid-cols-[26px_minmax(0,1fr)_14px] items-center gap-2 rounded-lg border px-3 py-1.5 text-sm"
                  aria-haspopup="menu"
                  aria-expanded={userMenuOpen}
                >
                  <div className="grid h-[26px] w-[26px] place-items-center rounded-full bg-[linear-gradient(135deg,var(--zl-accent),var(--zl-accent2))]">
                    <User size={14} color="#fff" />
                  </div>
                  <span className="min-w-0 truncate">
                    {user?.displayName || user?.username || 'admin'}
                  </span>
                  <ChevronDown
                    size={14}
                    className={
                      userMenuOpen ? 'rotate-180 transition-transform' : 'transition-transform'
                    }
                    style={{ color: 'var(--zl-text-muted)' }}
                  />
                </button>
              </AppTooltip>
              {userMenuOpen ? (
                <div
                  className="absolute right-0 top-[calc(100%+8px)] z-[1300] w-full rounded-lg p-2 shadow-2xl"
                  role="menu"
                  style={{
                    background: 'var(--zl-menu-bg)',
                    border: '1px solid var(--zl-border)',
                    boxShadow: 'var(--zl-menu-shadow)',
                  }}
                >
                  <button
                    type="button"
                    className="zl-action-button zl-menu-action-item flex h-10 w-full items-center gap-2 rounded-lg px-3 text-left text-sm"
                    role="menuitem"
                    onClick={() => {
                      setUserMenuOpen(false);
                      setPasswordDialogOpen(true);
                    }}
                  >
                    <KeyRound size={16} />
                    修改密码
                  </button>
                  <button
                    type="button"
                    className="zl-action-button zl-menu-action-item zl-danger-button flex h-10 w-full items-center gap-2 rounded-lg px-3 text-left text-sm"
                    role="menuitem"
                    onClick={() => void handleLogout()}
                  >
                    <LogOut size={16} />
                    退出系统
                  </button>
                </div>
              ) : null}
            </div>
          </div>
        </header>
        <main
          className="zl-hidden-scrollbar min-h-0 flex-1 overflow-y-auto"
          style={{ background: 'var(--zl-bg)' }}
        >
          <div
            key={location.pathname}
            className="zl-page-shell flex h-full min-h-full flex-col p-6"
          >
            {visibleItems.length === 0 ? (
              <div
                className="flex h-full items-center justify-center text-sm"
                style={{ color: 'var(--zl-text-muted)' }}
              >
                暂无可访问的功能
              </div>
            ) : (
              <>
                {current && !hideRouteHeading ? (
                  <div className="mb-5 flex shrink-0 items-start justify-between gap-4">
                    <div className="min-w-0">
                      <h1
                        className="truncate text-lg font-semibold"
                        style={{ color: 'var(--zl-text)' }}
                      >
                        {current.label}
                      </h1>
                      <p className="mt-1 text-sm" style={{ color: 'var(--zl-text-muted)' }}>
                        {routeDescription}
                      </p>
                    </div>
                  </div>
                ) : null}
                <div className="min-h-0 flex-1">
                  {transitioningTo || routeStatus.pending || redirectingUnauthorizedRoute ? (
                    <div
                      className="flex h-full items-center justify-center text-sm"
                      style={{ color: 'var(--zl-text-muted)' }}
                    >
                      正在加载页面...
                    </div>
                  ) : (
                    (children ?? <Outlet />)
                  )}
                </div>
              </>
            )}
          </div>
        </main>
      </div>
      <PasswordDialog
        open={passwordDialogOpen}
        onClose={() => setPasswordDialogOpen(false)}
        onSuccess={handlePasswordChanged}
      />
    </div>
  );
}
