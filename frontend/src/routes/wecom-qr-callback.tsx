import { createFileRoute } from '@tanstack/react-router';
import { Loader2 } from 'lucide-react';
import { useEffect } from 'react';

// WecomQrCallbackPage 企微扫码中转页：iframe 内静默占位由父页面接管，顶层整页回跳转发登录页现有回调逻辑
function WecomQrCallbackPage() {
  const embedded = typeof window !== 'undefined' && window.self !== window.top;
  useEffect(() => {
    if (!embedded) {
      window.location.replace('/login' + window.location.search);
    }
  }, [embedded]);
  return (
    <main
      data-cmp="WecomQrCallback"
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
        <Loader2 size={28} className="zl-spinner" />
        <p className="text-sm font-medium" style={{ color: 'var(--zl-text)' }}>
          {embedded ? '扫码成功，正在登录' : '企业微信授权回跳处理中'}
        </p>
        <p className="text-xs" style={{ color: 'var(--zl-text-muted)' }}>
          {embedded ? '请勿关闭当前页面' : '即将返回登录页继续'}
        </p>
      </section>
    </main>
  );
}

export const Route = createFileRoute('/wecom-qr-callback')({
  component: WecomQrCallbackPage,
});
