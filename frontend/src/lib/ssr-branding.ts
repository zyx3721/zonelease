import { createServerFn } from '@tanstack/react-start';
import { defaultSsrBaseBranding, type SsrBaseBranding } from './branding';

// fetchSsrBaseBranding 在服务端直连后端读取公开品牌字段，用于 SSR 首屏直出站点名称与图标
export const fetchSsrBaseBranding = createServerFn({ method: 'GET' }).handler(
  async (): Promise<SsrBaseBranding> => {
    const origin =
      process.env.VITE_API_BASE_URL ||
      import.meta.env.VITE_API_BASE_URL ||
      'http://127.0.0.1:8080';
    try {
      const response = await fetch(`${origin}/api/public/base`, {
        headers: { accept: 'application/json' },
        signal: AbortSignal.timeout(2000),
      });
      if (!response.ok) {
        throw new Error(`backend responded with status ${response.status}`);
      }
      const data = (await response.json()) as Partial<
        Record<keyof SsrBaseBranding, unknown>
      >;
      return {
        siteName: brandingText(data.siteName, defaultSsrBaseBranding.siteName),
        loginName: brandingText(data.loginName, defaultSsrBaseBranding.loginName),
        appName: brandingText(data.appName, defaultSsrBaseBranding.appName),
        appSubtitle: brandingText(data.appSubtitle, defaultSsrBaseBranding.appSubtitle),
        iconData: brandingText(data.iconData, defaultSsrBaseBranding.iconData),
      };
    } catch (error) {
      console.error('Failed to load base branding for SSR, falling back to default', error);
      return defaultSsrBaseBranding;
    }
  }
);

function brandingText(value: unknown, fallback: string) {
  const trimmed = typeof value === 'string' ? value.trim() : '';
  return trimmed || fallback;
}
