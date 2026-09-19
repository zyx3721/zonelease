import type { SettingsField } from './settings-primitives';

export type WecomMode = 'direct' | 'center';

export const WECOM_MODE_LABELS: Record<WecomMode, string> = {
  direct: '直连企业微信',
  center: '统一认证中心',
};

export function normalizeWecomMode(value: unknown): WecomMode {
  return value === 'center' ? 'center' : 'direct';
}

export function wecomDescription(mode: WecomMode) {
  if (mode === 'center') {
    return '通过 wecom-auth-center 统一认证中心完成企业微信扫码，企微凭据集中保管在认证中心，本系统仅持 verify 对接密钥';
  }
  return '本系统直连企业微信 OAuth 完成扫码登录，企业 ID 与应用 Secret 保存在本平台';
}

export function wecomRequiredFields(mode: WecomMode): SettingsField[] {
  if (mode === 'center') {
    return [
      {
        key: 'authCenterUrl',
        label: '认证中心地址',
        placeholder: 'https://auth.example.com',
        required: true,
        helper: 'wecom-auth-center 服务地址，写入前会自动去掉末尾斜杠',
      },
      {
        key: 'appId',
        label: '应用标识',
        placeholder: 'zonelease',
        required: true,
        helper: '认证中心 apps 下登记的本系统标识，需与认证中心配置一致',
      },
      {
        key: 'appSecret',
        label: '应用对接密钥',
        placeholder: '请输入应用对接密钥',
        required: true,
        type: 'password',
        helper: '认证中心为本系统分配的 app_secret，仅用于后端 verify 签名，不进入前端',
      },
    ];
  }
  return [
    { key: 'corpId', label: '企业 ID', placeholder: 'ww1234567890abcdef', required: true },
    {
      key: 'agentId',
      label: '应用 AgentID',
      placeholder: '1000002',
      required: true,
      type: 'number',
      inputMode: 'numeric',
    },
    {
      key: 'secret',
      label: '应用 Secret',
      placeholder: '请输入应用 Secret',
      required: true,
      type: 'password',
      helper: '仅服务端保存并缓存 access_token，接口不回显',
    },
  ];
}

export function wecomOptionalFields(mode: WecomMode): SettingsField[] {
  const redirectPrefix: SettingsField = {
    key: 'redirectPrefix',
    label: '回调地址前缀',
    placeholder: 'https://dns.example.com',
    helper: '留空时按当前访问地址自动推断回调地址；域名需在企微后台配置为可信回调域名',
  };
  if (mode === 'center') {
    return [redirectPrefix];
  }
  return [
    redirectPrefix,
    {
      key: 'fetchName',
      label: '获取成员姓名',
      type: 'checkbox',
      helper: '回调时额外调用企微通讯录接口补全姓名，需要在企微后台授予通讯录读取权限，取不到时不影响登录',
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
    if (!String(next.corpId ?? '').trim()) return { config: {}, error: '企业 ID 不能为空' };
    if (!Number(next.agentId)) return { config: {}, error: '应用 AgentID 不能为空' };
    if (!secretReady(next, 'secret', 'hasSecret')) {
      return { config: {}, error: '应用 Secret 不能为空' };
    }
    next.agentId = Number(next.agentId);
    const loginMode = String(next.loginMode ?? 'qr');
    if (loginMode !== 'qr' && loginMode !== 'inside') {
      return { config: {}, error: '授权方式不正确' };
    }
    next.loginMode = loginMode;
    return { config: removeEmpty(next), error: '' };
  }
  if (!String(next.authCenterUrl ?? '').trim()) {
    return { config: {}, error: '认证中心地址不能为空' };
  }
  next.authCenterUrl = String(next.authCenterUrl).trim().replace(/\/+$/, '');
  if (!String(next.appId ?? '').trim()) {
    return { config: {}, error: '应用标识不能为空' };
  }
  if (!secretReady(next, 'appSecret', 'hasAppSecret')) {
    return { config: {}, error: '应用对接密钥不能为空' };
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
