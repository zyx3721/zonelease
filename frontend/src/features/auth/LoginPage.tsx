import { Link, useNavigate } from '@tanstack/react-router';
import { Eye, EyeOff, Loader2, Lock, Moon, QrCode, Sun, User } from 'lucide-react';
import { type FormEvent, useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { AppTooltip } from '@/components/app-tooltip';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useBaseConfig } from '@/lib/branding';
import {
  WECOM_BIND_RESULT_EVENT,
  consumeAuthExpired,
  exchangeWecomTicket,
  fetchCurrentUser,
  fetchPublicAuthProviders,
  fetchWecomAuthorize,
  getAuthToken,
  login,
  loginByWecomCenterTicket,
  loginByWecomDirectCallback,
  persistUser,
  wecomBindByCode,
  wecomBindByTicket,
  wecomErrorMessage,
  type AuthSession,
  type PublicAuthProvider,
} from '@/lib/auth';
import { WecomQrLogin, type WecomEmbedSuccess } from './WecomQrLogin';
import {
  applyZlTheme,
  getInitialZlTheme,
  persistZlTheme,
  toggleZlTheme,
  type ZlTheme,
} from '@/lib/utils';

export function LoginPage() {
  const navigate = useNavigate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [provider, setProvider] = useState('local');
  const [providers, setProviders] = useState<PublicAuthProvider[]>([]);
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [wecomBusy, setWecomBusy] = useState(false);
  const [wecomEmbedFailed, setWecomEmbedFailed] = useState(false);
  const [wecomEmbedNonce, setWecomEmbedNonce] = useState(0);
  const [error, setError] = useState('');
  const [theme, setTheme] = useState<ZlTheme>(getInitialZlTheme);
  const [isCallbackView] = useState(() => hasWecomCallbackParams());
  const baseConfig = useBaseConfig();
  const isWecomProvider = provider === 'wecom';
  const toggleLabel = theme === 'dark' ? '切换浅色背景' : '切换深色背景';

  const finishWecomLogin = useCallback(
    (session: AuthSession) => {
      toast.success(`欢迎回来，${session.user.displayName || session.user.username}`);
      void navigate({ to: '/', replace: true });
    },
    [navigate]
  );

  useEffect(() => {
    applyZlTheme(theme);
    persistZlTheme(theme);
  }, [theme]);

  useEffect(() => {
    if (consumeAuthExpired()) {
      toast.error('登录会话已过期，请重新登录');
    }
  }, []);

  useEffect(() => {
    setWecomEmbedFailed(false);
  }, [provider]);

  useEffect(() => {
    let cancelled = false;
    if (getAuthToken() && !hasWecomCallbackParams()) {
      void fetchCurrentUser()
        .then(user => {
          if (cancelled) return;
          persistUser(user);
          void navigate({ to: '/', replace: true });
        })
        .catch(() => undefined);
    }
    return () => {
      cancelled = true;
    };
  }, [navigate]);

  useEffect(() => {
    fetchPublicAuthProviders()
      .then(response => setProviders(response.items))
      .catch(() => setProviders([]));
  }, []);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const loginTicket = params.get('wecomTicket');
    const centerTicket = params.get('ticket');
    const code = params.get('code');
    const state = params.get('state');
    const errorCode = params.get('wecomError');
    if (!loginTicket && !centerTicket && !errorCode && !(code && state)) return;
    for (const key of ['wecomTicket', 'ticket', 'code', 'state', 'wecomError']) {
      params.delete(key);
    }
    const rest = params.toString();
    window.history.replaceState(null, '', window.location.pathname + (rest ? `?${rest}` : ''));
    if (errorCode) {
      setError(wecomErrorMessage(errorCode));
      return;
    }
    if (getAuthToken() && ((code && state) || centerTicket)) {
      setWecomBusy(true);
      const bindRequest =
        code && state ? wecomBindByCode({ code, state }) : wecomBindByTicket(centerTicket ?? '');
      bindRequest
        .then(binding => {
          window.opener?.postMessage(
            { type: WECOM_BIND_RESULT_EVENT, ok: true, wecomUserid: binding.wecomUserid },
            window.location.origin
          );
          toast.success('企业微信绑定成功');
          void fetchCurrentUser({ force: true }).then(user => persistUser(user));
          window.setTimeout(() => window.close(), 300);
        })
        .catch((err: unknown) => {
          const message = err instanceof Error ? err.message : '企业微信绑定失败，请稍后重试';
          window.opener?.postMessage(
            { type: WECOM_BIND_RESULT_EVENT, ok: false, message },
            window.location.origin
          );
          setError(message);
          if (window.opener) {
            window.setTimeout(() => window.close(), 1500);
          }
        })
        .finally(() => setWecomBusy(false));
      return;
    }
    setWecomBusy(true);
    const loginRequest = loginTicket
      ? exchangeWecomTicket(loginTicket)
      : code && state
        ? loginByWecomDirectCallback(code, state)
        : loginByWecomCenterTicket(centerTicket ?? '');
    loginRequest
      .then(session => {
        finishWecomLogin(session);
      })
      .catch(err => {
        setError(err instanceof Error ? err.message : '企业微信登录失败，请稍后重试');
      })
      .finally(() => setWecomBusy(false));
  }, [finishWecomLogin, navigate]);

  useEffect(() => {
    const handlePageShow = (event: PageTransitionEvent) => {
      if (!event.persisted) return;
      if (hasWecomCallbackParams()) {
        window.location.replace('/login');
        return;
      }
      setWecomBusy(false);
      setLoading(false);
    };
    window.addEventListener('pageshow', handlePageShow);
    return () => window.removeEventListener('pageshow', handlePageShow);
  }, []);

  async function startWecomLogin() {
    setWecomBusy(true);
    setError('');
    try {
      const response = await fetchWecomAuthorize();
      window.location.assign(response.url);
      window.setTimeout(() => setWecomBusy(false), 4000);
    } catch (err) {
      setError(err instanceof Error ? err.message : '企业微信登录暂不可用，请稍后重试');
      setWecomBusy(false);
    }
  }

  async function handleWecomEmbedSuccess(payload: WecomEmbedSuccess) {
    setWecomBusy(true);
    setError('');
    try {
      const session =
        payload.authMode === 'sso'
          ? await loginByWecomCenterTicket(payload.ticket ?? '')
          : await loginByWecomDirectCallback(payload.code ?? '', payload.state ?? '');
      finishWecomLogin(session);
    } catch (err) {
      const message = err instanceof Error ? err.message : '企业微信登录失败，请稍后重试';
      setError(message);
      setWecomEmbedNonce(value => value + 1);
    } finally {
      setWecomBusy(false);
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isWecomProvider) {
      startWecomLogin();
      return;
    }
    const normalizedUsername = username.trim();
    if (!normalizedUsername || !password) {
      setError('用户名或密码不能为空');
      return;
    }
    setLoading(true);
    setError('');
    try {
      const session = await login(normalizedUsername, password, provider);
      toast.success(`欢迎回来，${session.user.displayName || session.user.username}`);
      void navigate({ to: '/', replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败，请稍后重试');
    } finally {
      setLoading(false);
    }
  }

  if (isCallbackView) {
    return (
      <main
        data-cmp="Login"
        className="relative flex min-h-dvh items-center justify-center overflow-hidden px-4 py-8 sm:px-6"
        style={{
          background:
            'radial-gradient(circle at 50% 0%, rgba(59,130,246,0.24), transparent 30%), var(--zl-login-bg)',
          color: 'var(--zl-text)',
        }}
      >
        <div className="zl-login-grid absolute inset-0" aria-hidden="true" />
        <section
          className="relative z-10 flex w-full max-w-[420px] flex-col items-center gap-4 rounded-[24px] p-8 text-center"
          style={{
            background: 'var(--zl-login-panel-bg)',
            border: '1px solid var(--zl-border)',
            backdropFilter: 'blur(18px)',
            boxShadow: 'var(--zl-login-panel-shadow)',
          }}
        >
          {error ? (
            <>
              <p className="text-sm leading-6" style={{ color: '#fca5a5' }} role="alert">
                {error}
              </p>
              <button
                type="button"
                onClick={() => window.location.replace('/login')}
                className="zl-action-button rounded-xl px-4 py-2 text-sm font-medium"
                style={{
                  borderColor: 'var(--zl-border)',
                  background: 'var(--zl-control-bg)',
                  color: 'var(--zl-accent-text)',
                }}
              >
                返回登录
              </button>
            </>
          ) : (
            <>
              <Loader2 size={28} className="zl-spinner" />
              <p className="text-sm font-medium" style={{ color: 'var(--zl-text)' }}>
                正在处理企业微信授权，请稍候…
              </p>
            </>
          )}
        </section>
      </main>
    );
  }

  return (
    <main
      data-cmp="Login"
      className="relative flex min-h-dvh items-center justify-center overflow-hidden px-4 py-8 sm:px-6"
      style={{
        background:
          'radial-gradient(circle at 50% 0%, rgba(59,130,246,0.24), transparent 30%), radial-gradient(circle at 12% 22%, rgba(6,182,212,0.18), transparent 28%), radial-gradient(circle at 88% 82%, rgba(16,185,129,0.14), transparent 30%), var(--zl-login-bg)',
        color: 'var(--zl-text)',
      }}
    >
      <AppTooltip
        label={toggleLabel}
        placement="bottom"
        className="absolute right-5 top-5 z-20 sm:right-6 sm:top-6"
      >
        <button
          type="button"
          onClick={() => setTheme(toggleZlTheme)}
          className="zl-action-button grid h-11 w-11 place-items-center rounded-lg border"
          style={{
            background: 'var(--zl-control-bg)',
            borderColor: 'var(--zl-border)',
            color: 'var(--zl-text-muted)',
            boxShadow: 'var(--zl-menu-shadow)',
          }}
          aria-label={toggleLabel}
        >
          {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
        </button>
      </AppTooltip>
      <div className="zl-login-grid absolute inset-0" aria-hidden="true" />
      <div
        className="zl-login-orb absolute left-[-6rem] top-20 h-64 w-64 rounded-full"
        aria-hidden="true"
      />
      <div
        className="zl-login-orb absolute bottom-[-4rem] right-[-5rem] h-80 w-80 rounded-full"
        aria-hidden="true"
      />

      <section className="relative z-10 flex w-full max-w-[460px] flex-col items-center gap-5">
        <div className="zl-login-reveal flex flex-col items-center text-center">
          <img
            className="mb-4 h-20 w-20 drop-shadow-[0_18px_42px_rgba(6,182,212,0.18)]"
            src={baseConfig.iconData}
            alt={baseConfig.loginName}
          />
          <p className="zl-gradient-text text-2xl font-bold tracking-wide">
            {baseConfig.loginName}
          </p>
          <p
            className="mt-1 text-xs uppercase tracking-[0.28em]"
            style={{ color: 'var(--zl-text-muted)' }}
          >
            DNS DHCP Control Plane
          </p>
        </div>

        <section
          className="zl-login-frame zl-login-reveal w-full rounded-[28px] p-1"
          style={{ animationDelay: '80ms' }}
        >
          <div
            className="rounded-[24px] p-6 sm:p-8"
            style={{
              background: 'var(--zl-login-panel-bg)',
              border: '1px solid var(--zl-border)',
              backdropFilter: 'blur(18px)',
              boxShadow: 'var(--zl-login-panel-shadow)',
            }}
          >
            <div className="mb-7 text-center">
              <h2
                className="text-2xl font-bold"
                style={{ color: 'var(--zl-text)', textShadow: '0 0 22px rgba(6,182,212,0.16)' }}
              >
                欢迎回来
              </h2>
              <p className="mt-2 text-sm" style={{ color: 'var(--zl-text-muted)' }}>
                登录您的账户以继续
              </p>
            </div>

            <form className="space-y-5" onSubmit={handleSubmit} noValidate>
              {providers.length > 0 ? (
                <LoginProviderSelect
                  value={provider}
                  providers={providers}
                  onChange={setProvider}
                />
              ) : null}
              {isWecomProvider ? (
                wecomEmbedFailed ? (
                  <div
                    className="flex flex-col items-center gap-2 rounded-2xl px-4 py-5 text-center"
                    style={{
                      background: 'var(--zl-control-bg)',
                      border: '1px solid var(--zl-border)',
                      color: 'var(--zl-text)',
                    }}
                  >
                    <span
                      className="grid h-14 w-14 place-items-center rounded-full"
                      style={{ background: 'rgba(59,130,246,0.14)' }}
                    >
                      <QrCode size={26} style={{ color: 'var(--zl-accent-text)' }} />
                    </span>
                    <p className="text-sm font-medium" style={{ color: 'var(--zl-text)' }}>
                      企业微信扫码登录
                    </p>
                    <p className="text-xs leading-5" style={{ color: 'var(--zl-text-muted)' }}>
                      点击下方按钮跳转至企业微信授权页，
                      <br />
                      使用企业微信 App 扫码确认后自动登录
                    </p>
                  </div>
                ) : (
                  <WecomQrLogin
                    key={wecomEmbedNonce}
                    onSuccess={handleWecomEmbedSuccess}
                    onFallback={() => setWecomEmbedFailed(true)}
                  />
                )
              ) : (
                <>
                  <div>
                    <label className="mb-2 block text-sm font-medium" htmlFor="username">
                      用户名
                    </label>
                    <div className="relative">
                      <User
                        className="absolute left-4 top-1/2 -translate-y-1/2"
                        size={17}
                        style={{ color: 'var(--zl-text-muted)' }}
                      />
                      <input
                        id="username"
                        autoComplete="username"
                        value={username}
                        onChange={event => setUsername(event.target.value)}
                        className="h-12 w-full rounded-2xl py-3 pl-12 pr-4 text-sm outline-none transition-all"
                        style={{
                          background: 'var(--zl-control-bg)',
                          border: '1px solid var(--zl-border)',
                          color: 'var(--zl-text)',
                        }}
                        placeholder="请输入用户名"
                      />
                    </div>
                  </div>

                  <div>
                    <div className="mb-2 flex items-center justify-between">
                      <label className="block text-sm font-medium" htmlFor="password">
                        密码
                      </label>
                      <Link
                        to="/forgot-password"
                        className="text-xs font-semibold"
                        style={{ color: 'var(--zl-accent-text)' }}
                      >
                        忘记密码?
                      </Link>
                    </div>
                    <div className="relative">
                      <Lock
                        className="absolute left-4 top-1/2 -translate-y-1/2"
                        size={17}
                        style={{ color: 'var(--zl-text-muted)' }}
                      />
                      <input
                        id="password"
                        autoComplete="current-password"
                        type={showPassword ? 'text' : 'password'}
                        value={password}
                        onChange={event => setPassword(event.target.value)}
                        className="h-12 w-full rounded-2xl py-3 pl-12 pr-12 text-sm outline-none transition-all"
                        style={{
                          background: 'var(--zl-control-bg)',
                          border: '1px solid var(--zl-border)',
                          color: 'var(--zl-text)',
                        }}
                        placeholder="请输入密码"
                      />
                      <button
                        type="button"
                        onClick={() => setShowPassword(value => !value)}
                        className="absolute right-2 top-1/2 grid h-9 w-9 -translate-y-1/2 place-items-center rounded-xl"
                        style={{ color: 'var(--zl-text-muted)' }}
                        aria-label={showPassword ? '隐藏密码' : '显示密码'}
                      >
                        {showPassword ? <EyeOff size={17} /> : <Eye size={17} />}
                      </button>
                    </div>
                  </div>
                </>
              )}

              <div aria-live="polite" className="min-h-6">
                {error ? (
                  <p
                    role="alert"
                    className="rounded-xl px-3 py-2 text-xs"
                    style={{
                      background: 'rgba(239,68,68,0.12)',
                      border: '1px solid rgba(239,68,68,0.28)',
                      color: '#fca5a5',
                    }}
                  >
                    {error}
                  </p>
                ) : null}
              </div>

              {isWecomProvider && !wecomEmbedFailed ? (
                <button
                  type="button"
                  onClick={startWecomLogin}
                  disabled={loading || wecomBusy}
                  className="w-full text-center text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60"
                  style={{ color: 'var(--zl-text-muted)' }}
                >
                  扫码异常？使用跳转方式登录
                </button>
              ) : (
                <button
                  type="submit"
                  disabled={loading || wecomBusy}
                  className="zl-login-submit flex h-12 w-full items-center justify-center gap-2 rounded-2xl text-sm font-semibold transition-all disabled:cursor-not-allowed disabled:opacity-60"
                  style={{
                    background: 'linear-gradient(135deg, #2563eb, #06b6d4)',
                    color: '#fff',
                    boxShadow: '0 18px 48px rgba(37,99,235,0.35)',
                  }}
                >
                  {loading || wecomBusy ? <Loader2 size={17} className="zl-spinner" /> : null}
                  {loading || wecomBusy
                    ? '处理中...'
                    : isWecomProvider
                      ? '企业微信扫码登录'
                      : '登录'}
                </button>
              )}
            </form>
          </div>
        </section>
        <p className="text-xs" style={{ color: 'var(--zl-text-muted)' }}>
          (c) 2026 {baseConfig.siteName}. Secure DNS and DHCP operations console.
        </p>
      </section>
    </main>
  );
}

function hasWecomCallbackParams() {
  if (typeof window === 'undefined') return false;
  const params = new URLSearchParams(window.location.search);
  return Boolean(
    params.get('wecomTicket') ||
    params.get('ticket') ||
    params.get('wecomError') ||
    (params.get('code') && params.get('state'))
  );
}

function LoginProviderSelect({
  value,
  providers,
  onChange,
}: {
  value: string;
  providers: PublicAuthProvider[];
  onChange: (value: string) => void;
}) {
  const options = [
    { id: 'local', name: '本地账号' },
    ...providers.map(item => ({
      id: item.id,
      name: item.name || (item.type === 'wecom' ? '企业微信' : 'AD/LDAP'),
    })),
  ];

  return (
    <div>
      <label className="mb-2 block text-sm font-medium" htmlFor="login-provider-select">
        登录方式
      </label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id="login-provider-select" className="h-12 rounded-2xl px-4 font-normal">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map(item => (
            <SelectItem key={item.id} value={item.id} className="font-normal">
              {item.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
