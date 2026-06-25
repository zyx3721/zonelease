import { useEffect, useState } from 'react';
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
  agentOperationTimeoutSeconds: 20,
  agentFullSyncTimeoutSeconds: 300,
  agentHealthCheckIntervalMinutes: 1,
  agentHealthCheckConcurrency: 1,
};

let cachedBaseConfig = defaultBaseConfig;
let baseConfigLoadedAt = 0;
let pendingBaseConfig: Promise<SystemBaseConfig> | null = null;
const listeners = new Set<(config: SystemBaseConfig) => void>();
const BASE_CONFIG_CACHE_TTL_MS = 30_000;

export function getBaseConfigSnapshot() {
  return cachedBaseConfig;
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

export function useBaseConfig() {
  const [config, setConfig] = useState<SystemBaseConfig>(cachedBaseConfig);

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
