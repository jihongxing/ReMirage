import { useApi } from '../hooks/useApi';
import { StatusIndicator } from '../components/StatusIndicator';

interface Gateway { id: string; status: string }
interface Cell { id: string }
interface ThreatStats { banned_count: number; active_users: number }

export function Dashboard() {
  const { data: gateways } = useApi<Gateway[]>('/gateways');
  const { data: cells } = useApi<Cell[]>('/cells');
  const { data: stats } = useApi<ThreatStats>('/threats/stats');

  const onlineGw = gateways?.filter(g => g.status === 'ONLINE').length ?? 0;
  const totalGw = gateways?.length ?? 0;
  const totalCells = cells?.length ?? 0;
  const activeUsers = stats?.active_users ?? 0;
  const bannedIPs = stats?.banned_count ?? 0;

  type Status = 'online' | 'degraded' | 'offline';
  const gwStatus: Status = totalGw === 0 ? 'offline' : onlineGw === totalGw ? 'online' : 'degraded';
  const cellStatus: Status = totalCells > 0 ? 'online' : 'offline';
  const userStatus: Status = activeUsers > 0 ? 'online' : 'offline';
  const banStatus: Status = bannedIPs > 0 ? 'degraded' : 'online';

  const cards = [
    { label: '在线 Gateway', value: `${onlineGw}/${totalGw}`, status: gwStatus },
    { label: '蜂窝数量', value: totalCells, status: cellStatus },
    { label: '活跃用户', value: activeUsers, status: userStatus },
    { label: '封禁 IP', value: bannedIPs, status: banStatus },
  ];

  return (
    <div>
      <h2 className="text-xl font-semibold mb-6">Dashboard</h2>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {cards.map(card => (
          <div key={card.label} className="bg-slate-900 border border-slate-800 rounded-lg p-5">
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm text-slate-400">{card.label}</span>
              <StatusIndicator status={card.status} />
            </div>
            <div className="text-3xl font-bold">{card.value}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
