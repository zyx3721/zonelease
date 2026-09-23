import { Loader2 } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchWecomAuthorize, type WecomAuthorizeEmbed } from '@/lib/auth';

// WecomEmbedSuccess 内嵌二维码扫码确认后交由父页面完成登录的授权参数
export type WecomEmbedSuccess = {
  authMode: 'direct' | 'sso';
  code?: string;
  state?: string;
  ticket?: string;
};

type WecomQrLoginProps = {
  onSuccess: (payload: WecomEmbedSuccess) => void;
  onFallback: () => void;
};

// WecomQrLogin 内嵌企业微信扫码二维码：iframe 加载扫码页，iframe 内回跳中转路由时同源读取授权参数
export function WecomQrLogin({ onSuccess, onFallback }: WecomQrLoginProps) {
  const [embed, setEmbed] = useState<WecomAuthorizeEmbed | null>(null);
  const iframeRef = useRef<HTMLIFrameElement | null>(null);
  const handledRef = useRef(false);
  const onSuccessRef = useRef(onSuccess);
  const onFallbackRef = useRef(onFallback);
  onSuccessRef.current = onSuccess;
  onFallbackRef.current = onFallback;

  useEffect(() => {
    let cancelled = false;
    fetchWecomAuthorize()
      .then(response => {
        if (cancelled) return;
        if (!response.embed) {
          onFallbackRef.current();
          return;
        }
        setEmbed(response.embed);
      })
      .catch(() => {
        if (!cancelled) onFallbackRef.current();
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const handleIframeLoad = useCallback(() => {
    if (handledRef.current || !embed) return;
    const frame = iframeRef.current;
    if (!frame) return;
    let query: string;
    try {
      const frameLocation = frame.contentWindow?.location;
      if (!frameLocation) return;
      if (frameLocation.pathname === '/login' && frameLocation.search) {
        onFallbackRef.current();
        return;
      }
      if (frameLocation.pathname !== embed.callback_path) return;
      query = frameLocation.search;
    } catch {
      return;
    }
    const params = new URLSearchParams(query);
    handledRef.current = true;
    if (embed.auth_mode === 'sso') {
      onSuccessRef.current({ authMode: 'sso', ticket: params.get('ticket') ?? '' });
      return;
    }
    onSuccessRef.current({
      authMode: 'direct',
      code: params.get('code') ?? '',
      state: params.get('state') ?? '',
    });
  }, [embed]);

  if (!embed) {
    return (
      <div
        className="flex h-[420px] w-full items-center justify-center rounded-2xl"
        style={{
          background: 'var(--zl-control-bg)',
          border: '1px solid var(--zl-border)',
          color: 'var(--zl-text-muted)',
        }}
      >
        <Loader2 size={24} className="zl-spinner" />
      </div>
    );
  }

  return (
    <iframe
      ref={iframeRef}
      src={embed.iframe_url}
      title="企业微信扫码登录"
      onLoad={handleIframeLoad}
      className="w-full rounded-2xl"
      style={{ height: 420, border: '1px solid var(--zl-border)', background: '#ffffff' }}
    />
  );
}
