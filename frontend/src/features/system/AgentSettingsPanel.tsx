import type { SystemBaseConfig } from '@/lib/system-settings';
import { NumberControl } from './settings-primitives';

export function AgentPanel({
  form,
  onUpdate,
  disabled,
}: {
  form: SystemBaseConfig;
  onUpdate: (patch: Partial<SystemBaseConfig>) => void;
  disabled?: boolean;
}) {
  const fields = [
    {
      label: 'Agent 离线失败次数',
      description: `自动健康检查连续失败 ${form.agentOfflineFailureCount} 次后才标记为离线并创建通知`,
      unit: '次',
      value: form.agentOfflineFailureCount,
      min: 1,
      max: 20,
      patch: (value: number) => ({ agentOfflineFailureCount: value }),
    },
    {
      label: 'Agent 连通性检查间隔',
      description: `后端每 ${form.agentHealthCheckIntervalMinutes} 分钟自动检查一次已登记 Agent`,
      unit: '分钟',
      value: form.agentHealthCheckIntervalMinutes,
      min: 1,
      max: 60,
      patch: (value: number) => ({ agentHealthCheckIntervalMinutes: value }),
    },
    {
      label: '自动检查并发',
      description: `自动健康检查时最多同时检查 ${form.agentHealthCheckConcurrency} 个 Agent，默认 1 个保持串行`,
      unit: '个',
      value: form.agentHealthCheckConcurrency,
      min: 1,
      max: 20,
      patch: (value: number) => ({ agentHealthCheckConcurrency: value }),
    },
    {
      label: 'Agent 连接超时',
      description: `连接测试和同步前检查超过 ${form.agentConnectionTimeoutSeconds} 秒无响应则失败`,
      unit: '秒',
      value: form.agentConnectionTimeoutSeconds,
      min: 1,
      max: 20,
      patch: (value: number) => ({ agentConnectionTimeoutSeconds: value }),
    },
    {
      label: 'Agent 操作超时',
      description: `DNS / DHCP 操作超过 ${form.agentOperationTimeoutSeconds} 秒无响应则失败`,
      unit: '秒',
      value: form.agentOperationTimeoutSeconds,
      min: 1,
      max: 60,
      patch: (value: number) => ({ agentOperationTimeoutSeconds: value }),
    },
    {
      label: 'Agent 全量同步超时',
      description: `全量同步、单 Agent 同步和局部同步超过 ${form.agentFullSyncTimeoutSeconds} 秒则失败`,
      unit: '秒',
      value: form.agentFullSyncTimeoutSeconds,
      min: 60,
      max: 600,
      patch: (value: number) => ({ agentFullSyncTimeoutSeconds: value }),
    },
  ] satisfies Array<{
    label: string;
    description?: string;
    unit: string;
    value: number;
    min: number;
    max: number;
    patch: (value: number) => Partial<SystemBaseConfig>;
  }>;

  return (
    <div className="grid gap-3 lg:grid-cols-2">
      {fields.map(field => (
        <NumberControl
          key={field.label}
          label={field.label}
          description={field.description}
          unit={field.unit}
          value={field.value}
          min={field.min}
          max={field.max}
          disabled={disabled}
          onChange={value => onUpdate(field.patch(value))}
        />
      ))}
    </div>
  );
}
