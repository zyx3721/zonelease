import type { SettingsField } from './settings-primitives';

export type WecomMode = 'direct' | 'center';

export const WECOM_MODE_LABELS: Record<WecomMode, string> = {
  direct: '直连企业微信',
  center: '统一认证中心',
};

export const WECOM_MODE_OPTIONS: Array<{ value: WecomMode; label: string; description: string }> = [
  { value: 'direct', label: '直连企业微信', description: '本系统直接持有企微应用凭据并完成扫码' },
  {
    value: 'center',
    label: '统一认证中心',
    description: '经由 wecom-auth-center 完成企微扫码后回跳',
  },
];

export function normalizeWecomMode(value: unknown): WecomMode {
  return value === 'center' ? 'center' : 'direct';
}

export function wecomRequiredFields(mode: WecomMode): SettingsField[] {
  if (mode === 'center') {
    return [
      {
        key: 'authCenterUrl',
        label: '认证中心地址',
        placeholder: 'https://auth.example.com',
        required: true,
        labelHint: '统一认证中心（wecom-auth-center）的外部访问地址',
      },
      {
        key: 'appId',
        label: '应用标识',
        placeholder: 'zonelease',
        required: true,
        labelHint: '认证中心 config.yaml 中 apps 下的条目名',
      },
      {
        key: 'appSecret',
        label: '应用密钥',
        placeholder: '与认证中心 apps 配置的 app_secret 一致',
        required: true,
        type: 'password',
      },
    ];
  }
  return [
    { key: 'corpId', label: '企业 ID（corpid）', placeholder: 'ww********', required: true },
    {
      key: 'agentId',
      label: '应用 AgentID',
      placeholder: '1000002',
      required: true,
      inputMode: 'numeric',
    },
    {
      key: 'secret',
      label: '应用 Secret',
      placeholder: '请输入自建应用的应用密钥',
      required: true,
      type: 'password',
    },
    {
      key: 'redirectPrefix',
      label: '回调地址前缀',
      placeholder: '留空则按当前访问地址推断',
      labelHint: '企业微信服务器需能访问，如 https://dns.example.com',
    },
  ];
}

export function prepareWecomConfig(
  form: Record<string, unknown>,
  enabled: boolean
): { config: Record<string, unknown>; error: string } {
  const next = { ...form };
  const mode = normalizeWecomMode(next.mode);
  next.mode = mode;
  if (!enabled) {
    return { config: removeEmpty(next), error: '' };
  }
  if (mode === 'direct') {
    if (!String(next.corpId ?? '').trim())
      return { config: {}, error: '企业 ID（corpid）不能为空' };
    if (!Number(next.agentId)) return { config: {}, error: '应用 AgentID 不能为空' };
    if (!secretReady(next, 'secret', 'hasSecret')) {
      return { config: {}, error: '应用 Secret 不能为空' };
    }
    next.agentId = Number(next.agentId);
    return { config: removeEmpty(next), error: '' };
  }
  const baseURL = String(next.authCenterUrl ?? '').trim();
  if (!baseURL) {
    return { config: {}, error: '认证中心地址不能为空' };
  }
  if (!/^https?:\/\//i.test(baseURL)) {
    return { config: {}, error: '认证中心地址需以 http:// 或 https:// 开头' };
  }
  next.authCenterUrl = baseURL.replace(/\/+$/, '');
  if (!String(next.appId ?? '').trim()) {
    return { config: {}, error: '应用标识不能为空' };
  }
  if (!secretReady(next, 'appSecret', 'hasAppSecret')) {
    return { config: {}, error: '应用密钥不能为空' };
  }
  return { config: removeEmpty(next), error: '' };
}

function secretReady(form: Record<string, unknown>, key: string, markerKey: string) {
  return String(form[key] ?? '').trim() !== '' || Boolean(form[markerKey]);
}

function removeEmpty(config: Record<string, unknown>) {
  return Object.fromEntries(
    Object.entries(config).filter(([, value]) => {
      if (typeof value === 'string') return value.trim() !== '';
      return value !== undefined && value !== null && value !== '';
    })
  );
}

export function wecomTestSuccessMessage(detail?: string) {
  return detail ? `企业微信认证测试通过：${detail}` : '企业微信认证测试通过';
}
