import { useApi } from '../hooks/useApi';
import { DataTable } from '../components/DataTable';
import { StatusIndicator } from '../components/StatusIndicator';

interface Cell {
  id: string; name: string; region: string; level: number;
  user_count: number; max_users: number; gateway_count: number; health: string;
}

const healthMap = (h: string) => h === 'HEALTHY' ? 'online' : h === 'DEGRADED' ? 'degraded' : 'offline';

export function Cells() {
  const { data, loading } = useApi<Cell[]>('/cells');

  const columns = [
    { key: 'name', title: '名称' },
    { key: 'region', title: '区域' },
    { key: 'level', title: '级别' },
    { key: 'user_count', title: '用户数', render: (_: unknown, r: Cell) => `${r.user_count}/${r.max_users}` },
    { key: 'gateway_count', title: 'Gateway 数' },
    { key: 'health', title: '健康状态', render: (_: unknown, r: Cell) => <StatusIndicator status={healthMap(r.health)} /> },
  ];

  return (
    <div>
      <h2 className="text-xl font-semibold mb-4">Cells</h2>
      <DataTable columns={columns} data={data ?? []} loading={loading} />
    </div>
  );
}
