// 蜂窝编排器 - 意图驱动的 Gateway 生命周期管理
// 状态机：潜伏(Phase 0) → 校准(Phase 1) → 服役(Phase 2)
import { useState } from 'react';
import {
  useGateways,
  promoteToCalibration,
  activateGateway,
  retireGateway,
  type GatewayItem,
} from '../hooks/useConsoleApi';

const phaseLabel = (phase: number) => {
  switch (phase) {
    case 0: return { text: '潜伏', color: 'bg-yellow-500/20 text-yellow-400' };
    case 1: return { text: '校准', color: 'bg-blue-500/20 text-blue-400' };
    case 2: return { text: '服役', color: 'bg-green-500/20 text-green-400' };
    default: return { text: '未知', color: 'bg-slate-500/20 text-slate-400' };
  }
};

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
};

export default function CellOrchestrator() {
  const { data, loading, refetch } = useGateways();
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleAction = async (gatewayId: string, action: 'calibrate' | 'activate' | 'retire') => {
    setActionLoading(gatewayId);
    setError(null);
    try {
      switch (action) {
        case 'calibrate':
          await promoteToCalibration(gatewayId);
          break;
        case 'activate':
          await activateGateway(gatewayId);
          break;
        case 'retire':
          await retireGateway(gatewayId);
          break;
      }
      refetch();
    } catch (e) {
      setError(e instanceof Error ? e.message : '操作失败');
    } finally {
      setActionLoading(null);
    }
  };

  const gateways = data?.gateways || [];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">蜂窝编排器</h1>
          <p className="text-slate-400 text-sm mt-1">
            Gateway 生命周期管理 · 潜伏 → 校准 → 服役
          </p>
        </div>
        <div className="flex gap-2 text-xs">
          <span className="px-2 py-1 rounded bg-yellow-500/20 text-yellow-400">
            潜伏: {gateways.filter(g => g.phase === 0).length}
          </span>
          <span className="px-2 py-1 rounded bg-blue-500/20 text-blue-400">
            校准: {gateways.filter(g => g.phase === 1).length}
          </span>
          <span className="px-2 py-1 rounded bg-green-500/20 text-green-400">
            服役: {gateways.filter(g => g.phase === 2).length}
          </span>
        </div>
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 text-red-400 text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-slate-400">加载中...</div>
      ) : (
        <div className="grid gap-4">
          {gateways.map((gw) => (
            <GatewayCard
              key={gw.gateway_id}
              gateway={gw}
              onAction={handleAction}
              loading={actionLoading === gw.gateway_id}
            />
          ))}
          {gateways.length === 0 && (
            <div className="text-slate-500 text-center py-12">暂无 Gateway 节点</div>
          )}
        </div>
      )}
    </div>
  );
}

// Gateway 卡片组件
function GatewayCard({
  gateway,
  onAction,
  loading,
}: {
  gateway: GatewayItem;
  onAction: (id: string, action: 'calibrate' | 'activate' | 'retire') => void;
  loading: boolean;
}) {
  const phase = phaseLabel(gateway.phase);

  return (
    <div className="bg-slate-800/50 border border-slate-700 rounded-lg p-4 flex items-center justify-between">
      <div className="flex items-center gap-4">
        {/* 状态指示 */}
        <div className={`w-3 h-3 rounded-full ${gateway.is_online ? 'bg-green-400' : 'bg-slate-500'}`} />

        <div>
          <div className="flex items-center gap-2">
            <span className="text-white font-mono text-sm">{gateway.gateway_id}</span>
            <span className={`px-2 py-0.5 rounded text-xs ${phase.color}`}>{phase.text}</span>
          </div>
          <div className="text-slate-400 text-xs mt-1 flex gap-4">
            <span>{gateway.ip_address}</span>
            <span>Cell: {gateway.cell_id}</span>
            <span>连接: {gateway.active_connections}</span>
            <span>质量: {gateway.network_quality.toFixed(1)}</span>
            <span>CPU: {gateway.cpu_percent.toFixed(1)}%</span>
            <span>内存: {formatBytes(gateway.memory_bytes)}</span>
          </div>
        </div>
      </div>

      {/* 意图驱动操作按钮 */}
      <div className="flex gap-2">
        {gateway.phase === 0 && (
          <button
            onClick={() => onAction(gateway.gateway_id, 'calibrate')}
            disabled={loading}
            className="px-3 py-1.5 text-xs rounded bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50"
          >
            {loading ? '...' : '晋升校准'}
          </button>
        )}
        {gateway.phase === 1 && (
          <button
            onClick={() => onAction(gateway.gateway_id, 'activate')}
            disabled={loading}
            className="px-3 py-1.5 text-xs rounded bg-green-600 hover:bg-green-500 text-white disabled:opacity-50"
          >
            {loading ? '...' : '正式服役'}
          </button>
        )}
        {gateway.phase === 2 && (
          <button
            onClick={() => onAction(gateway.gateway_id, 'retire')}
            disabled={loading}
            className="px-3 py-1.5 text-xs rounded bg-red-600/80 hover:bg-red-500 text-white disabled:opacity-50"
          >
            {loading ? '...' : '退役'}
          </button>
        )}
      </div>
    </div>
  );
}
