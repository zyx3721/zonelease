const TOKEN_KEY = 'zonelease.auth.token';
const USER_KEY = 'zonelease.auth.user';
const EXPIRES_AT_KEY = 'zonelease.auth.expires_at';
export const AUTH_SESSION_CHANGED_EVENT = 'zonelease:auth-session-changed';
export const WECOM_PROVIDERS_CHANGED_EVENT = 'zonelease:wecom-providers-changed';
export const WECOM_BIND_RESULT_EVENT = 'zonelease:wecom-bind-result';

export function emitWecomProvidersChanged() {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new Event(WECOM_PROVIDERS_CHANGED_EVENT));
}

export type AuthUser = {
  id: string;
  username: string;
  email: string;
  displayName: string;
  role: string;
  permissions: string[];
  wecomBound: boolean;
};

export type AuthSession = {
  token: string;
  expires_at: string;
  
  user: AuthUser;
};

export type PublicAuthProvider = {
  id: string;
  type: string;
  name: string;
  enabled: boolean;
};

type ApiErrorResponse = {
  error?: string;
  message?: string;
};

type ApiOptions = RequestInit & {
  auth?: boolean;
};

type LogoutOptions = {
  waitForRemote?: boolean;
};

const CURRENT_USER_CACHE_TTL_MS = 1500;
const PUBLIC_AUTH_PROVIDERS_CACHE_TTL_MS = 30_000;

let pendingCurrentUser: Promise<AuthUser> | null = null;
let cachedCurrentUser: { user: AuthUser; expiresAt: number } | null = null;
let pendingPublicAuthProviders: Promise<{ items: PublicAuthProvider[]; total: number }> | null =
  null;
let cachedPublicAuthProviders: {
  value: { items: PublicAuthProvider[]; total: number };
  expiresAt: number;
} | null = null;

// storage 登录态统一存取 localStorage，使同一浏览器内新建标签页与重启浏览器后仍保持登录，登录态仅随服务端会话过期失效
function storage() {
  if (typeof window === 'undefined') return null;
  return window.localStorage;
}

function emitSessionChanged() {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new Event(AUTH_SESSION_CHANGED_EVENT));
}

export function getAuthToken() {
  return storage()?.getItem(TOKEN_KEY) ?? '';
}

export function getStoredUser(): AuthUser | null {
  const raw = storage()?.getItem(USER_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as AuthUser;
  } catch {
    storage()?.removeItem(USER_KEY);
    return null;
  }
}

export function isAuthenticated() {
  return Boolean(getAuthToken() && getStoredUser());
}

export function persistSession(session: AuthSession) {
  const store = storage();
  if (!store) return;
  store.setItem(TOKEN_KEY, session.token);
  store.setItem(USER_KEY, JSON.stringify(session.user));
  store.setItem(EXPIRES_AT_KEY, session.expires_at);
  emitSessionChanged();
}

export function clearSession() {
  const store = storage();
  if (!store) return;
  store.removeItem(TOKEN_KEY);
  store.removeItem(USER_KEY);
  store.removeItem(EXPIRES_AT_KEY);
  emitSessionChanged();
}

const AUTH_EXPIRED_FLAG_KEY = 'zonelease.auth.expired';
let authRedirectingToLogin = false;

// markAuthExpired 标记本次进入登录页由会话失效引起：仅认证请求收到 401 时调用，
// 登录页挂载时消费该标记并提示重新登录；主动登出与未登录访问不产生标记
export function markAuthExpired() {
  if (typeof window === 'undefined') return;
  try {
    sessionStorage.setItem(AUTH_EXPIRED_FLAG_KEY, '1');
  } catch {
    return;
  }
}

// consumeAuthExpired 读取并清除会话失效标记，返回是否存在待提示的过期进入
export function consumeAuthExpired() {
  try {
    const expired = sessionStorage.getItem(AUTH_EXPIRED_FLAG_KEY) === '1';
    if (expired) sessionStorage.removeItem(AUTH_EXPIRED_FLAG_KEY);
    return expired;
  } catch {
    return false;
  }
}

// markAuthRedirecting 标记正以整页刷新跳转登录页，RequireAuth 据此跳过客户端跳转，
// 避免"先无动画进入登录页、再整页刷新带加载动画"的重复进入
export function markAuthRedirecting() {
  authRedirectingToLogin = true;
}

// isAuthRedirecting 返回当前文档是否已触发会话失效的整页跳转
export function isAuthRedirecting() {
  return authRedirectingToLogin;
}

export function userHasPermission(user: AuthUser | null, permission: string) {
  if (!user) return false;
  if (user.role === 'admin') return true;
  return user.permissions?.includes(permission) ?? false;
}

export function userHasAnyPermission(user: AuthUser | null, permissions: string[]) {
  return permissions.some(permission => userHasPermission(user, permission));
}

async function readApiError(response: Response) {
  try {
    const body = (await response.json()) as ApiErrorResponse;
    return normalizeApiErrorMessage(body.message || body.error || `请求失败：${response.status}`);
  } catch {
    return `请求失败：${response.status}`;
  }
}

function normalizeApiErrorMessage(message: string) {
  if (
    message.includes('JSON request body parsing requires .NET System.Web.Extensions') ||
    message.includes('Install/enable .NET Framework 3.5/4.x')
  ) {
    return 'DHCP Legacy Agent 缺少 .NET Framework JSON 组件，请安装或启用 .NET Framework 3.x 以上';
  }
  return message;
}

export function persistUser(user: AuthUser) {
  storage()?.setItem(USER_KEY, JSON.stringify(user));
}

export function setCurrentUserSnapshot(user: AuthUser) {
  cachedCurrentUser = { user, expiresAt: Date.now() + CURRENT_USER_CACHE_TTL_MS };
  persistUser(user);
}

export async function api<T>(path: string, options: ApiOptions = {}): Promise<T> {
  const headers = new Headers(options.headers);
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json; charset=utf-8');
  }
  const token = getAuthToken();
  if (options.auth !== false && token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  const response = await fetch(path, { ...options, headers });
  if (!response.ok) {
    const message = await readApiError(response);
    if (options.auth !== false && response.status === 401) {
      markAuthExpired();
      markAuthRedirecting();
      clearSession();
      if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
        window.location.assign('/login');
      }
    }
    throw new Error(message);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export async function login(username: string, password: string, provider = 'local') {
  const session = await api<AuthSession>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password, provider }),
  });
  persistSession(session);
  setCurrentUserSnapshot(session.user);
  return session;
}

export async function exchangeWecomTicket(ticket: string) {
  const session = await api<AuthSession>('/api/auth/wecom/exchange', {
    method: 'POST',
    auth: false,
    body: JSON.stringify({ ticket }),
  });
  persistSession(session);
  setCurrentUserSnapshot(session.user);
  return session;
}

export async function loginByWecomCenterTicket(ticket: string) {
  const session = await api<AuthSession>('/api/auth/wecom/login', {
    method: 'POST',
    auth: false,
    body: JSON.stringify({ ticket }),
  });
  persistSession(session);
  setCurrentUserSnapshot(session.user);
  return session;
}

export async function loginByWecomDirectCallback(code: string, state: string) {
  const session = await api<AuthSession>('/api/auth/wecom/login', {
    method: 'POST',
    auth: false,
    body: JSON.stringify({ code, state }),
  });
  persistSession(session);
  setCurrentUserSnapshot(session.user);
  return session;
}

export type WecomAuthorizeEmbed = {
  auth_mode: 'direct' | 'sso';
  iframe_url: string;
  state?: string;
  callback_path: string;
};

// fetchWecomAuthorize 获取企业微信扫码登录跳转地址与内嵌二维码渲染参数（公开接口）
export async function fetchWecomAuthorize() {
  return api<{ url: string; embed?: WecomAuthorizeEmbed }>('/api/auth/wecom/authorize', {
    auth: false,
  });
}

export function fetchWecomBindUrl() {
  return api<{ url: string }>('/api/auth/wecom/bind-url');
}

export type WecomBindResult = {
  bound: boolean;
  wecomUserid?: string;
};

export function wecomBindByCode(body: { code: string; state: string }) {
  return api<WecomBindResult>('/api/auth/wecom/bind', {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export function wecomBindByTicket(ticket: string) {
  return api<WecomBindResult>('/api/auth/wecom/bind', {
    method: 'POST',
    body: JSON.stringify({ ticket }),
  });
}

export function unbindWecom() {
  return api<WecomBindResult>('/api/auth/wecom/unbind', { method: 'POST' });
}

export const WECOM_ERROR_MESSAGES: Record<string, string> = {
  state_invalid: '登录状态校验失败，请重新发起企业微信登录',
  state_expired: '登录已超时，请重新发起企业微信登录',
  wecom_error: '企业微信身份获取失败，请稍后重试',
  center_error: '统一认证中心连接失败，请稍后重试',
  user_not_bound: '该企业微信账号尚未绑定平台用户，请先使用账号密码登录后在用户菜单绑定企业微信',
  user_not_provisioned: '该用户已被禁用，请联系管理员',
  login_failed: '企业微信登录失败，请稍后重试',
  wecom_not_enabled: '企业微信登录未启用，请联系管理员',
  wecom_config_invalid: '企业微信登录配置不完整，请联系管理员',
};

export function wecomErrorMessage(code: string | null) {
  if (!code) return '';
  return WECOM_ERROR_MESSAGES[code] ?? '企业微信登录未完成，请重新发起登录';
}

export function fetchPublicAuthProviders(options: { force?: boolean } = {}) {
  if (
    !options.force &&
    cachedPublicAuthProviders &&
    cachedPublicAuthProviders.expiresAt > Date.now()
  ) {
    return Promise.resolve(cachedPublicAuthProviders.value);
  }
  if (pendingPublicAuthProviders && !options.force) return pendingPublicAuthProviders;
  pendingPublicAuthProviders = api<{ items: PublicAuthProvider[]; total: number }>(
    '/api/auth/providers',
    { auth: false }
  )
    .then(value => {
      cachedPublicAuthProviders = {
        value,
        expiresAt: Date.now() + PUBLIC_AUTH_PROVIDERS_CACHE_TTL_MS,
      };
      return value;
    })
    .finally(() => {
      pendingPublicAuthProviders = null;
    });
  return pendingPublicAuthProviders;
}

export function fetchCurrentUser(options: { force?: boolean } = {}) {
  if (!options.force && cachedCurrentUser && cachedCurrentUser.expiresAt > Date.now()) {
    return Promise.resolve(cachedCurrentUser.user);
  }
  if (pendingCurrentUser && !options.force) return pendingCurrentUser;
  pendingCurrentUser = api<AuthUser>('/api/auth/me')
    .then(user => {
      cachedCurrentUser = { user, expiresAt: Date.now() + CURRENT_USER_CACHE_TTL_MS };
      persistUser(user);
      return user;
    })
    .finally(() => {
      pendingCurrentUser = null;
    });
  return pendingCurrentUser;
}

export async function logout(options: LogoutOptions = {}) {
  const token = getAuthToken();
  pendingCurrentUser = null;
  cachedCurrentUser = null;
  clearSession();
  if (!token) return;

  const request = fetch('/api/auth/logout', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    keepalive: true,
  }).catch(() => undefined);

  if (options.waitForRemote === false) {
    void request;
    return;
  }

  await request;
}

export function changePassword(body: {
  old_password: string;
  new_password: string;
  confirm_password: string;
}) {
  return api<{ status: string }>('/api/auth/change-password', {
    method: 'POST',
    body: JSON.stringify(body),
  });
}
