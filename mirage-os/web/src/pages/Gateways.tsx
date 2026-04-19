import { useState } from 'react';
import { useApi } from '../hooks/useApi';
import { DataTable } from '../components/DataTable';
import { StatusIndicator } from '../components/StatusIndicator';
import { ControlPanel } from '../components/ControlPanel';

interface Gateway {
  id: string; status: string; last_heartbeat: string;
  active_connections: number; memory_usage_mb: number; threat_level: number;
}

const statusMap = (s: string) => s === 'ONLINE' ? 'online' : s === 'DEGRADED' ? 'degraded' : 'offline';

export function Gateways() {
  const [filter, setFilter] = useState('ALL');
  const { data, loading } = useApi<Gateway[]>('/gateways');

  const filtered = data?.filter(g => filter === 'ALL' || g.status === filter) ?? [];

  const columns = [
    { key: 'id', title: 'IP / ID' },
    { key: 'status', title: '状态', render: (_: unknown, r: Gateway) => <StatusIndicator status={statusMap(r.status)} /> },
    { key: 'last_heartbeat', title: '最后心跳', render: (v: unknown) => new Date(v as string).toLocaleString() },
    { key: 'active_connections', title: '连接数' },
    { key: 'memory_usage_mb', title: '内存 (MB)' },
    { key: 'threat_level', title: '威胁等级' },
  ];

  return (
    <div>
      <h2 className="text-xl font-semibold mb-4">Gateways</h2>
      <div className="mb-4">
        <ControlPanel controls={[{
          type: 'select', label: '状态', value: filter,
          options: [
            { value: 'ALL', label: '全部' },
            { value: 'ONLINE', label: '在线' },
            { value: 'DEGRADED', label: '降级' },
            { value: 'OFFLINE', label: '离线' },
          ],
          onChange: setFilter,
        }]} />
      </div>
      <DataTable columns={columns} data={filtered} loading={loading} />
    </div>
  );
}
