import { Link, useNavigate } from '@tanstack/react-router';
import { KeyRound, Loader2, Lock, Mail, Moon, RefreshCw, Send, Sun, User } from 'lucide-react';
import { type FormEvent, useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import {
  confirmPasswordReset,
  fetchPasswordResetCaptcha,
  sendPasswordResetCode,
  verifyPasswordResetIdentity,
  type PasswordResetCaptcha,
  type PasswordResetChannel,
} from '@/lib/password-reset';
import {
  applyZlTheme,
  getInitialZlTheme,
  persistZlTheme,
  toggleZlTheme,
  type ZlTheme,
} from '@/lib/utils';

type Step = 'identity' | 'delivery';

const primaryButtonStyle = {
  background: 'linear-gradient(135deg, #0f766e, #14b8a6)',
  color: '#fff',
  boxShadow: '0 18px 42px rgba(20,184,166,0.26)',
};

export function ForgotPasswordPage() {
  const navigate = useNavigate();
  const verifyEmailRef = useRef<HTMLInputElement | null>(null);
  const [theme, setTheme] = useState<ZlTheme>(getInitialZlTheme);
  const [step, setStep] = useState<Step>('identity');
  const [username, setUsername] = useState('');
  const [captcha, setCaptcha] = useState<PasswordResetCaptcha | null>(null);
  const [captchaAnswer, setCaptchaAnswer] = useState('');
  const [verificationToken, setVerificationToken] = useState('');
  const [channels, setChannels] = useState<PasswordResetChannel[]>([]);
  const [channel, setChannel] = useState('email');
  const [verifyEmail, setVerifyEmail] = useState('');
  const [code, setCode] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const [cooldown, setCooldown] = useState(0);

  useEffect(() => {
    applyZlTheme(theme);
    persistZlTheme(theme);
  }, [theme]);

  useEffect(() => {
    void loadCaptcha();
  }, []);

  useEffect(() => {
    if (!captcha?.expiresAt || step !== 'identity') return;
    const expiresAt = new Date(captcha.expiresAt).getTime();
    const delay = Number.isNaN(expiresAt) ? 60_000 : Math.max(1000, expiresAt - Date.now());
    const timer = window.setTimeout(() => {
      void loadCaptcha();
    }, delay);
    return () => window.clearTimeout(timer);
  }, [captcha?.expiresAt, step]);

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = window.setTimeout(() => setCooldown(value => Math.max(0, value - 1)), 1000);
    return () => window.clearTimeout(timer);
  }, [cooldown]);

  useEffect(() => {
    if (step !== 'delivery') return;
    window.requestAnimationFrame(() => verifyEmailRef.current?.focus());
  }, [step]);

  async function loadCaptcha() {
    try {
      const next = await fetchPasswordResetCaptcha();
      setCaptcha(next);
      setCaptchaAnswer('');
    } catch (err) {
      setError(err instanceof Error ? err.message : '生成验证码失败');
    }
  }

  async function handleVerify(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalizedUsername = username.trim();
    if (!normalizedUsername) {
      setError('请输入用户名');
      return;
    }
    if (!captcha || !captchaAnswer.trim()) {
      setError('请输入验证码');
      return;
    }
    setBusy('verify');
    setError('');
    try {
      const response = await verifyPasswordResetIdentity({
        username: normalizedUsername,
        captchaToken: captcha.token,
        captchaAnswer: captchaAnswer.trim(),
      });
      if (response.channels.length === 0) {
        setError('当前没有可用的找回密码媒介');
        return;
      }
      setUsername(normalizedUsername);
      setVerificationToken(response.verificationToken);
      setChannels(response.channels);
      setChannel(response.channels[0]?.id ?? 'email');
      setStep('delivery');
    } catch (err) {
      setError(err instanceof Error ? err.message : '校验失败，请稍后重试');
      void loadCaptcha();
    } finally {
      setBusy('');
    }
  }

  async function handleSendCode() {
    if (cooldown > 0) {
      setError(`验证码已发送，请于 ${cooldown} 秒后再试`);
      return;
    }
    if (!verifyEmail.trim()) {
      setError('请输入验证邮箱');
      return;
    }
    setBusy('send');
    setError('');
    try {
      const response = await sendPasswordResetCode({
        username,
        verificationToken,
        channel,
        verifyEmail: verifyEmail.trim(),
      });
      setCooldown(response.cooldownSeconds || 60);
      toast.success(response.devCode ? `验证码 ${response.devCode}` : '找回密码验证码已发送');
    } catch (err) {
      setError(err instanceof Error ? err.message : '发送验证码失败');
    } finally {
      setBusy('');
    }
  }

  async function handleReset(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!code.trim()) {
      setError('请输入找回密码验证码');
      return;
    }
    if (newPassword.length < 6 || confirmPassword.length < 6) {
      setError('密码至少 6 个字符');
      return;
    }
    if (newPassword !== confirmPassword) {
      setError('新密码与确认密码不一致');
      return;
    }
    setBusy('reset');
    setError('');
    try {
      await confirmPasswordReset({
        username,
        verificationToken,
        code: code.trim(),
        newPassword,
        confirmPassword,
      });
      toast.success('密码已重置，请重新登录');
      void navigate({ to: '/login', replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : '密码重置失败');
    } finally {
      setBusy('');
    }
  }

  const captchaText = captcha?.question.replace(/\s*=\s*\?\s*$/, '') ?? '--';

  return (
    <main
      data-cmp="ForgotPasswordPage"
      className="relative flex min-h-dvh items-center justify-center overflow-hidden px-4 py-8 sm:px-6"
      style={{
        background:
          'radial-gradient(circle at 50% 0%, rgba(20,184,166,0.22), transparent 30%), radial-gradient(circle at 10% 26%, rgba(59,130,246,0.16), transparent 28%), radial-gradient(circle at 86% 78%, rgba(16,185,129,0.15), transparent 30%), var(--zl-login-bg)',
        color: 'var(--zl-text)',
      }}
    >
      <button
        type="button"
        onClick={() => setTheme(toggleZlTheme)}
        className="zl-action-button absolute right-5 top-5 z-20 grid h-11 w-11 place-items-center rounded-lg border sm:right-6 sm:top-6"
        style={{
          background: 'var(--zl-control-bg)',
          borderColor: 'var(--zl-border)',
          color: 'var(--zl-text-muted)',
          boxShadow: 'var(--zl-menu-shadow)',
        }}
        aria-label="切换主题"
      >
        {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
      </button>
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
            src="/favicon.svg"
            alt="ZoneLease"
          />
          <p className="zl-gradient-text text-2xl font-bold tracking-wide">ZoneLease</p>
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
              <h1
                className="text-2xl font-bold"
                style={{ color: 'var(--zl-text)', textShadow: '0 0 22px rgba(6,182,212,0.16)' }}
              >
                忘记密码
              </h1>
              <p className="mt-2 text-sm" style={{ color: 'var(--zl-text-muted)' }}>
                {step === 'identity'
                  ? '请输入您需要找回密码的用户名'
                  : '验证账号邮箱并接收验证码，然后设置新密码'}
              </p>
            </div>

            {step === 'identity' ? (
              <form className="space-y-5" onSubmit={handleVerify}>
                <Field
                  icon={<User size={17} />}
                  label="用户名"
                  required
                  action={
                    <Link
                      to="/login"
                      className="text-xs font-semibold transition-colors"
                      style={{ color: 'var(--zl-accent-text)' }}
                    >
                      返回登录
                    </Link>
                  }
                >
                  <input
                    value={username}
                    onChange={event => setUsername(event.target.value)}
                    className="zonelease-auth-input"
                    placeholder="请输入用户名"
                  />
                </Field>
                <Field icon={<KeyRound size={17} />} label="验证码" required>
                  <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_132px]">
                    <input
                      value={captchaAnswer}
                      onChange={event => setCaptchaAnswer(event.target.value)}
                      className="zonelease-auth-input"
                      placeholder="请输入验证码"
                    />
                    <button
                      type="button"
                      onClick={() => void loadCaptcha()}
                      className="zl-action-button flex h-12 items-center justify-center gap-2 rounded-2xl border px-4 text-sm font-semibold"
                      style={{ borderColor: 'var(--zl-border)' }}
                    >
                      <span className="text-base tracking-[0.18em]">{captchaText}</span>
                      <RefreshCw size={15} style={{ color: 'var(--zl-text-muted)' }} />
                    </button>
                  </div>
                </Field>
                <ErrorText error={error} />
                <button
                  type="submit"
                  disabled={busy === 'verify'}
                  className="zl-login-submit flex h-12 w-full items-center justify-center gap-2 rounded-2xl text-sm font-semibold transition-all disabled:cursor-not-allowed disabled:opacity-60"
                  style={primaryButtonStyle}
                >
                  {busy === 'verify' ? <Loader2 size={17} className="zl-spinner" /> : null}
                  {busy === 'verify' ? '处理中' : '提交'}
                </button>
              </form>
            ) : (
              <form className="space-y-5" onSubmit={handleReset}>
                <Field
                  icon={<Mail size={17} />}
                  label="验证邮箱"
                  required
                  action={
                    <span
                      className="text-xs font-semibold"
                      style={{ color: 'var(--zl-text-muted)' }}
                    >
                      当前账号：<span style={{ color: 'var(--zl-text)' }}>{username}</span>
                    </span>
                  }
                >
                  <input
                    ref={verifyEmailRef}
                    value={verifyEmail}
                    onChange={event => setVerifyEmail(event.target.value)}
                    className="zonelease-auth-input"
                    placeholder="请输入当前账号配置的邮箱"
                  />
                </Field>
                <Field icon={<KeyRound size={17} />} label="重置验证码" required>
                  <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_96px]">
                    <input
                      value={code}
                      onChange={event => setCode(event.target.value)}
                      onFocus={() => {
                        if (cooldown > 0) setError(`验证码已发送，请于 ${cooldown} 秒后再试`);
                      }}
                      className="zonelease-auth-input"
                      placeholder="请输入收到的验证码"
                    />
                    <button
                      type="button"
                      disabled={Boolean(busy)}
                      onClick={() => void handleSendCode()}
                      className="zl-login-submit flex h-12 items-center justify-center gap-2 rounded-2xl text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-60"
                      style={{
                        background: 'linear-gradient(135deg, #0f766e, #14b8a6)',
                        color: '#fff',
                        boxShadow: '0 18px 42px rgba(20,184,166,0.26)',
                      }}
                    >
                      {busy === 'send' ? <Loader2 size={16} className="zl-spinner" /> : null}
                      {busy === 'send' ? '发送中' : cooldown > 0 ? `${cooldown}s` : '发送'}
                    </button>
                  </div>
                </Field>
                <Field icon={<Lock size={17} />} label="新密码" required>
                  <input
                    type="password"
                    value={newPassword}
                    onChange={event => setNewPassword(event.target.value)}
                    className="zonelease-auth-input"
                    placeholder="至少 6 个字符"
                  />
                </Field>
                <Field icon={<Lock size={17} />} label="确认密码" required>
                  <input
                    type="password"
                    value={confirmPassword}
                    onChange={event => setConfirmPassword(event.target.value)}
                    className="zonelease-auth-input"
                    placeholder="请再次输入新密码"
                  />
                </Field>
                <ErrorText error={error} />
                <button
                  type="submit"
                  disabled={busy === 'reset'}
                  className="zl-login-submit flex h-12 w-full items-center justify-center gap-2 rounded-2xl text-sm font-semibold transition-all disabled:cursor-not-allowed disabled:opacity-60"
                  style={primaryButtonStyle}
                >
                  {busy === 'reset' ? <Loader2 size={17} className="zl-spinner" /> : null}
                  {busy === 'reset' ? '处理中' : '重置密码'}
                </button>
              </form>
            )}
          </div>
        </section>
        <p className="text-xs" style={{ color: 'var(--zl-text-muted)' }}>
          (c) 2026 ZoneLease. Secure DNS and DHCP operations console.
        </p>
      </section>
    </main>
  );
}

function Field({
  icon,
  label,
  required,
  action,
  children,
}: {
  icon: React.ReactNode;
  label: string;
  required?: boolean;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-2 flex items-center justify-between gap-3 text-sm font-medium">
        <span>
          {label}
          {required ? <span className="ml-1 text-red-400">*</span> : null}
        </span>
        {action}
      </span>
      <span className="relative block">
        <span
          className="absolute left-4 top-1/2 -translate-y-1/2"
          style={{ color: 'var(--zl-text-muted)' }}
        >
          {icon}
        </span>
        {children}
      </span>
    </label>
  );
}

function ErrorText({ error }: { error: string }) {
  return (
    <div aria-live="polite" className="min-h-6">
      {error ? (
        <p
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
  );
}
