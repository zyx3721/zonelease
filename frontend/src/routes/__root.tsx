import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  HeadContent,
  Link,
  Outlet,
  Scripts,
  createRootRouteWithContext,
  useRouter,
} from '@tanstack/react-router';
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  Home,
  Info,
  Loader2,
  RefreshCcw,
  SearchX,
  ShieldAlert,
  XCircle,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { BootScreen } from '@/components/boot-screen';
import { Toaster } from '@/components/ui/sonner';

import appCss from '../styles.css?url';

const themeScript = `
try {
  var theme = localStorage.getItem('zonelease.zl-theme') === 'light' ? 'light' : 'dark';
  document.documentElement.dataset.zlTheme = theme;
  document.documentElement.style.colorScheme = theme;
} catch (_) {}
`;

function NotFoundComponent() {
  return (
    <main className="relative grid min-h-screen place-items-center overflow-hidden bg-[var(--zl-bg)] px-4 py-10 sm:px-6">
      <div className="pointer-events-none absolute inset-x-0 top-1/2 h-px -translate-y-1/2 bg-gradient-to-r from-transparent via-[rgba(96,165,250,0.28)] to-transparent" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(148,163,184,0.055)_1px,transparent_1px),linear-gradient(90deg,rgba(148,163,184,0.045)_1px,transparent_1px)] bg-[size:46px_46px] [mask-image:radial-gradient(circle_at_50%_50%,#000_0%,transparent_70%)]" />

      <section className="zl-surface-3d w-full max-w-4xl rounded-2xl px-6 py-8 text-center sm:px-10 sm:py-10 lg:px-12">
        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl border border-[rgba(96,165,250,0.34)] bg-[rgba(59,130,246,0.12)] text-[var(--zl-accent-hover)] shadow-[0_16px_36px_rgba(37,99,235,0.18)]">
          <SearchX size={28} aria-hidden="true" />
        </div>
        <div className="zl-gradient-text mt-7 text-[clamp(5.25rem,15vw,9rem)] font-black leading-none tracking-normal drop-shadow-[0_14px_32px_rgba(37,99,235,0.18)]">
          404
        </div>
        <h1 className="zl-gradient-text mt-4 text-2xl font-semibold tracking-normal sm:text-3xl">
          页面不存在
        </h1>
        <p className="mx-auto mt-3 max-w-xl text-sm leading-6 text-muted-foreground sm:text-base">
          当前访问的页面不存在，或已经被移动。请返回首页后重新选择需要访问的功能。
        </p>
        <div className="mt-8 flex flex-wrap justify-center gap-3">
          <Link
            to="/"
            className="zl-login-submit inline-flex h-10 items-center justify-center gap-2 rounded-lg px-5 text-sm font-medium text-white"
          >
            <Home size={16} aria-hidden="true" />
            返回首页
          </Link>
          <button
            type="button"
            onClick={() => history.back()}
            className="zl-action-button inline-flex h-10 items-center justify-center gap-2 rounded-lg border px-5 text-sm font-medium"
          >
            <ArrowLeft size={16} aria-hidden="true" />
            返回上一页
          </button>
        </div>
      </section>
    </main>
  );
}

function ErrorComponent({ error, reset }: { error: Error; reset: () => void }) {
  console.error(error);
  const router = useRouter();

  return (
    <main className="relative grid min-h-screen place-items-center overflow-hidden bg-[var(--zl-bg)] px-4 py-10 sm:px-6">
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(148,163,184,0.052)_1px,transparent_1px),linear-gradient(90deg,rgba(148,163,184,0.04)_1px,transparent_1px)] bg-[size:44px_44px] [mask-image:radial-gradient(circle_at_50%_46%,#000_0%,transparent_72%)]" />

      <section className="zl-surface-3d w-full max-w-4xl rounded-2xl px-6 py-8 text-center sm:px-10 sm:py-10">
        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl border border-[rgba(239,68,68,0.34)] bg-[rgba(239,68,68,0.11)] text-[var(--zl-status-red-text)] shadow-[0_16px_36px_rgba(239,68,68,0.14)]">
          <ShieldAlert size={28} aria-hidden="true" />
        </div>
        <h1 className="mt-6 text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
          页面加载失败
        </h1>
        <p className="mx-auto mt-3 max-w-xl text-sm leading-6 text-muted-foreground sm:text-base">
          页面加载过程中出现异常，可以重试或返回首页。
        </p>
        {import.meta.env.DEV ? (
          <pre className="zl-error-detail-pre zl-hidden-scrollbar mx-auto mt-6 max-h-80 w-full max-w-3xl overflow-auto rounded-xl border p-5 text-left text-xs leading-5 sm:text-[0.8125rem]">
            {error.stack ?? error.message}
          </pre>
        ) : null}
        <div className="mt-8 flex flex-wrap justify-center gap-3">
          <button
            type="button"
            onClick={() => {
              router.invalidate();
              reset();
            }}
            className="zl-login-submit inline-flex h-10 items-center justify-center gap-2 rounded-lg px-5 text-sm font-medium text-white"
          >
            <RefreshCcw size={16} aria-hidden="true" />
            重试
          </button>
          <a
            href="/"
            className="zl-action-button inline-flex h-10 items-center justify-center gap-2 rounded-lg border px-5 text-sm font-medium"
          >
            <Home size={16} aria-hidden="true" />
            返回首页
          </a>
        </div>
      </section>
    </main>
  );
}

export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  head: () => ({
    meta: [
      { charSet: 'utf-8' },
      { name: 'viewport', content: 'width=device-width, initial-scale=1' },
      { title: 'ZoneLease' },
      { name: 'description', content: 'Windows DNS 与 DHCP 统一管理控制台。' },
      { name: 'author', content: 'ZoneLease' },
      { property: 'og:title', content: 'ZoneLease 控制台' },
      { property: 'og:description', content: 'Windows DNS 与 DHCP 统一管理控制台。' },
      { property: 'og:type', content: 'website' },
      { name: 'twitter:card', content: 'summary' },
      { name: 'twitter:title', content: 'ZoneLease 控制台' },
      { name: 'twitter:description', content: 'Windows DNS 与 DHCP 统一管理控制台。' },
    ],
    links: [
      {
        rel: 'stylesheet',
        href: appCss,
      },
      {
        rel: 'icon',
        href: '/favicon.svg',
        type: 'image/svg+xml',
      },
    ],
  }),
  shellComponent: RootShell,
  component: RootComponent,
  notFoundComponent: NotFoundComponent,
  errorComponent: ErrorComponent,
});

function RootShell({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN" data-zl-theme="dark">
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
        <HeadContent />
      </head>
      <body>
        {children}
        <Scripts />
      </body>
    </html>
  );
}

function RootComponent() {
  const { queryClient } = Route.useRouteContext();
  const [booting, setBooting] = useState(true);

  useEffect(() => {
    const timer = window.setTimeout(() => setBooting(false), 520);
    return () => window.clearTimeout(timer);
  }, []);

  return (
    <QueryClientProvider client={queryClient}>
      {booting ? <BootScreen /> : <Outlet />}
      <Toaster
        position="top-right"
        theme="system"
        closeButton
        expand
        visibleToasts={8}
        gap={10}
        duration={3000}
        offset={{ top: 28, right: 20 }}
        mobileOffset={{ top: 16, right: 12, left: 12 }}
        icons={{
          success: <CheckCircle2 size={18} />,
          error: <XCircle size={18} />,
          warning: <AlertTriangle size={18} />,
          info: <Info size={18} />,
          loading: <Loader2 className="zl-toast-spin" size={18} />,
        }}
      />
    </QueryClientProvider>
  );
}
