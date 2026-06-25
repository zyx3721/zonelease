const TOKEN_KEY = 'zonelease.auth.token';
const USER_KEY = 'zonelease.auth.user';
const EXPIRES_AT_KEY = 'zonelease.auth.expires_at';
export const AUTH_SESSION_CHANGED_EVENT = 'zonelease:auth-session-changed';

export type AuthUser = {
  id: string;
  username: string;
  email: string;
  displayName: string;
  role: string;
  permissions: string[];
};

export type AuthSession = {
  token: string;
  expires_at: string;
  last_seen_at: string;
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
    return body.message || body.error || `请求失败：${response.status}`;
  } catch {
    return `请求失败：${response.status}`;
  }
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
    headers.set('Content-Type', 'application/json');
  }
  const token = getAuthToken();
  if (options.auth !== false && token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  const response = await fetch(path, { ...options, headers });
  if (!response.ok) {
    const message = await readApiError(response);
    if (options.auth !== false && response.status === 401) {
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

export function fetchPublicAuthProviders() {
  if (cachedPublicAuthProviders && cachedPublicAuthProviders.expiresAt > Date.now()) {
    return Promise.resolve(cachedPublicAuthProviders.value);
  }
  if (pendingPublicAuthProviders) return pendingPublicAuthProviders;
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

export function fetchCurrentUser() {
  if (cachedCurrentUser && cachedCurrentUser.expiresAt > Date.now()) {
    return Promise.resolve(cachedCurrentUser.user);
  }
  if (pendingCurrentUser) return pendingCurrentUser;
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
