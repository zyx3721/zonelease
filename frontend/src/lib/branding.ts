import { useEffect, useState } from 'react';
import { useMatches } from '@tanstack/react-router';
import { fetchPublicSystemBaseConfig, type SystemBaseConfig } from './system-settings';

export const defaultBaseConfig: SystemBaseConfig = {
  siteName: 'ZoneLease',
  loginName: 'ZoneLease',
  appName: 'ZoneLease',
  appSubtitle: 'DNS / DHCP Control',
  iconData: '/favicon.svg',
  resetCodeTtlMinutes: 10,
  resetCaptchaTtlMinutes: 1,
  passwordResetSendCooldownMinutes: 0.5,
  passwordResetRateLimitMinutes: 5,
  runtimeSyncConcurrency: 3,
  dnsRecordConcurrency: 3,
  dhcpScopeConcurrency: 5,
  operationRefreshDelaySeconds: 10,
  agentOfflineFailureCount: 3,
  agentConnectionTimeoutSeconds: 5,
  agentOperationTimeoutSeconds: 20,
  agentFullSyncTimeoutSeconds: 300,
  agentHealthCheckIntervalMinutes: 1,
  agentHealthCheckConcurrency: 1,
  wecomStateTtlMinutes: 5,
  loginMaxFailures: 5,
  loginLockoutMinutes: 2,
};

let cachedBaseConfig = defaultBaseConfig;
let baseConfigLoadedAt = 0;
let pendingBaseConfig: Promise<SystemBaseConfig> | null = null;
const listeners = new Set<(config: SystemBaseConfig) => void>();
const BASE_CONFIG_CACHE_TTL_MS = 30_000;

export type SsrBaseBranding = {
  siteName: string;
  loginName: string;
  appName: string;
  appSubtitle: string;
  iconData: string;
};

export const defaultSsrBaseBranding: SsrBaseBranding = {
  siteName: defaultBaseConfig.siteName,
  loginName: defaultBaseConfig.loginName,
  appName: defaultBaseConfig.appName,
  appSubtitle: defaultBaseConfig.appSubtitle,
  iconData: defaultBaseConfig.iconData,
};

export function getBaseConfigSnapshot() {
  return cachedBaseConfig;
}

// getHeadBranding 返回 head 渲染应使用的品牌字段；客户端快照尚未加载时返回 null，由调用方回退 SSR 数据
export function getHeadBranding(): { siteName: string; iconData: string } | null {
  if (typeof window === 'undefined' || baseConfigLoadedAt === 0) return null;
  return { siteName: cachedBaseConfig.siteName, iconData: cachedBaseConfig.iconData };
}

export function setBaseConfigSnapshot(config: Partial<SystemBaseConfig>) {
  cachedBaseConfig = normalizeBaseConfig(config);
  baseConfigLoadedAt = Date.now();
  applyDocumentBranding(cachedBaseConfig);
  listeners.forEach(listener => listener(cachedBaseConfig));
}

function loadBaseConfig() {
  if (baseConfigLoadedAt > 0 && Date.now() - baseConfigLoadedAt < BASE_CONFIG_CACHE_TTL_MS) {
    return Promise.resolve(cachedBaseConfig);
  }
  if (pendingBaseConfig) return pendingBaseConfig;
  pendingBaseConfig = fetchPublicSystemBaseConfig()
    .then(next => {
      setBaseConfigSnapshot(next);
      return cachedBaseConfig;
    })
    .catch(() => {
      setBaseConfigSnapshot(defaultBaseConfig);
      return cachedBaseConfig;
    })
    .finally(() => {
      pendingBaseConfig = null;
    });
  return pendingBaseConfig;
}

// getRootLoaderBranding 从首个 match（恒为 root）的 loader 读取 SSR 直出的品牌数据，供服务端渲染与客户端首帧保持一致
function getRootLoaderBranding(): SsrBaseBranding | null {
  const data = useMatches()[0]?.loaderData as Partial<SsrBaseBranding> | undefined;
  if (
    !data?.siteName ||
    !data?.loginName ||
    !data?.appName ||
    !data?.appSubtitle ||
    !data?.iconData
  ) {
    return null;
  }
  return {
    siteName: data.siteName,
    loginName: data.loginName,
    appName: data.appName,
    appSubtitle: data.appSubtitle,
    iconData: data.iconData,
  };
}

export function useBaseConfig() {
  const rootBranding = getRootLoaderBranding();
  const [config, setConfig] = useState<SystemBaseConfig>(() => {
    if (baseConfigLoadedAt > 0) return cachedBaseConfig;
    if (rootBranding) return { ...defaultBaseConfig, ...rootBranding };
    return cachedBaseConfig;
  });

  useEffect(() => {
    listeners.add(setConfig);
    return () => {
      listeners.delete(setConfig);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    void loadBaseConfig().then(next => {
      if (!cancelled) setConfig(next);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return config;
}

export function normalizeBaseConfig(config: Partial<SystemBaseConfig>): SystemBaseConfig {
  return {
    ...defaultBaseConfig,
    ...config,
    siteName: text(config.siteName, defaultBaseConfig.siteName),
    loginName: text(config.loginName, defaultBaseConfig.loginName),
    appName: text(config.appName, defaultBaseConfig.appName),
    appSubtitle: text(config.appSubtitle, defaultBaseConfig.appSubtitle),
    iconData: text(config.iconData, defaultBaseConfig.iconData),
    resetCodeTtlMinutes: numberValue(
      config.resetCodeTtlMinutes,
      defaultBaseConfig.resetCodeTtlMinutes
    ),
    resetCaptchaTtlMinutes: numberValue(
      config.resetCaptchaTtlMinutes,
      defaultBaseConfig.resetCaptchaTtlMinutes
    ),
    passwordResetSendCooldownMinutes: numberValue(
      config.passwordResetSendCooldownMinutes,
      defaultBaseConfig.passwordResetSendCooldownMinutes
    ),
    passwordResetRateLimitMinutes: numberValue(
      config.passwordResetRateLimitMinutes,
      defaultBaseConfig.passwordResetRateLimitMinutes
    ),
    runtimeSyncConcurrency: numberValue(
      config.runtimeSyncConcurrency,
      defaultBaseConfig.runtimeSyncConcurrency
    ),
    dnsRecordConcurrency: numberValue(
      config.dnsRecordConcurrency,
      defaultBaseConfig.dnsRecordConcurrency
    ),
    dhcpScopeConcurrency: numberValue(
      config.dhcpScopeConcurrency,
      defaultBaseConfig.dhcpScopeConcurrency
    ),
    operationRefreshDelaySeconds: numberValue(
      config.operationRefreshDelaySeconds,
      defaultBaseConfig.operationRefreshDelaySeconds
    ),
    agentOfflineFailureCount: numberValue(
      config.agentOfflineFailureCount,
      defaultBaseConfig.agentOfflineFailureCount
    ),
    agentConnectionTimeoutSeconds: numberValue(
      config.agentConnectionTimeoutSeconds,
      defaultBaseConfig.agentConnectionTimeoutSeconds
    ),
    agentOperationTimeoutSeconds: numberValue(
      config.agentOperationTimeoutSeconds,
      defaultBaseConfig.agentOperationTimeoutSeconds
    ),
    agentFullSyncTimeoutSeconds: numberValue(
      config.agentFullSyncTimeoutSeconds,
      defaultBaseConfig.agentFullSyncTimeoutSeconds
    ),
    agentHealthCheckIntervalMinutes: numberValue(
      config.agentHealthCheckIntervalMinutes,
      defaultBaseConfig.agentHealthCheckIntervalMinutes
    ),
    agentHealthCheckConcurrency: numberValue(
      config.agentHealthCheckConcurrency,
      defaultBaseConfig.agentHealthCheckConcurrency
    ),
    wecomStateTtlMinutes: numberValue(
      config.wecomStateTtlMinutes,
      defaultBaseConfig.wecomStateTtlMinutes
    ),
    loginMaxFailures: numberValue(config.loginMaxFailures, defaultBaseConfig.loginMaxFailures),
    loginLockoutMinutes: numberValue(
      config.loginLockoutMinutes,
      defaultBaseConfig.loginLockoutMinutes
    ),
  };
}

export function applyDocumentBranding(config: SystemBaseConfig) {
  if (typeof document === 'undefined') return;
  document.title = config.siteName;
  const icon = document.querySelector<HTMLLinkElement>("link[rel='icon']");
  if (icon) icon.href = config.iconData || defaultBaseConfig.iconData;
}

function text(...values: Array<string | undefined>) {
  for (const value of values) {
    const trimmed = value?.trim();
    if (trimmed) return trimmed;
  }
  return '';
}

function numberValue(value: unknown, fallback: number) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}
