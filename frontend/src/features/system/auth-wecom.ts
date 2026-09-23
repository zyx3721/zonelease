import type { SettingsField } from './settings-primitives';

export type WecomMode = 'direct' | 'center';

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

export const WECOM_DIRECT_GUIDANCE =
  '配置步骤：企业微信管理后台 →「应用管理」→ 自建应用（记录 AgentID 与 Secret）→' +
  '在「网页授权及 JS-SDK」中把回调域名加入可信域名 → 在「企业可信 IP」中加入本服务出口 IP。' +
  '扫码确认后由企微官方登录面板回调授权码，本系统自动完成登录或绑定。';

export const WECOM_CENTER_GUIDANCE =
  '配置步骤：部署企业微信统一认证中心（wecom-auth-center）→ 在认证中心 config.yaml 的 apps 下为本系统新增条目：' +
  'domain 填本系统外部访问地址、callback_path 填 /login、app_secret 填 32 位以上随机密钥 →' +
  '在上方填写认证中心地址、应用标识与应用密钥（与认证中心保持一致）→ 重启认证中心使配置生效。' +
  '登录时本系统跳转认证中心完成企微扫码，认证中心携带一次性 ticket 回跳本系统 /login 完成登录或绑定。';

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
