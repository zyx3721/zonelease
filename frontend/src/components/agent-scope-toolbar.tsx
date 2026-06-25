import { RefreshCw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { ServerConfig } from '@/lib/dns-dhcp-store';

export function AgentScopeToolbar({
  agents,
  value,
  refreshing,
  onChange,
  onRefresh,
}: {
  agents: ServerConfig[];
  value: string;
  refreshing: boolean;
  onChange: (value: string) => void;
  onRefresh: () => void;
}) {
  return (
    <div className="flex min-w-[260px] items-center justify-end gap-2">
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger className="h-10 min-w-[180px] font-normal">
          <SelectValue placeholder="选择 Agent" />
        </SelectTrigger>
        <SelectContent>
          {agents.map(agent => (
            <SelectItem key={agent.id} value={agent.id} className="font-normal">
              {agent.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        type="button"
        variant="outline"
        className="h-10 gap-2"
        disabled={!value || refreshing}
        onClick={onRefresh}
      >
        <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
        刷新
      </Button>
    </div>
  );
}
