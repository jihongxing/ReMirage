// TunnelIndicator - 隧道路径状态灯
// 显示当前活跃的传输协议链路及降级状态
import { useMemo } from 'react';

export interface TunnelStatus {
  gateway_id: string;
  active_path: string;
  fallback_chain: string[];
  latency_ms: number;
}

const pathConfig: Record<string, { label: string; color: string; icon: string }> = {
  quic: { label: 'QUIC/H3', color: 'green', icon: '⚡' },
  wss: { label: 'WSS', color: 'blue', icon: '🔒' },
  webrtc: { label: 'WebRTC', color: 'yellow', icon: '📡' },
  icmp: { label: 'ICMP', color: 'orange', icon: '🏓' },
  dns: { label: 'DNS-TUN', color: 'red', icon: '🧬' },
};

const colorMap: Record<string, string> = {
  green: 'bg-green-400 shadow-green-400/60',
  blue: 'bg-blue-400 shadow-blue-400/60',
  yellow: 'bg-yellow-400 shadow-yellow-400/60',
  orange: 'bg-orange-400 shadow-orange-400/60',
  red: 'bg-red-400 shadow-red-400/60 animate-pulse',
};

const dimColorMap: Record<string, string> = {
  green: 'bg-green-900/30 border-green-800/50',
  blue: 'bg-blue-900/30 border-blue-800/50',
  yellow: 'bg-yellow-900/30 border-yellow-800/50',
  orange: 'bg-orange-900/30 border-orange-800/50',
  red: 'bg-red-900/30 border-red-800/50',
};

export const TunnelIndicator = ({ tunnels }: { tunnels: TunnelStatus[] }) => {
  // 聚合所有 gateway 的隧道状态
  const summary = useMemo(() => {
    const pathCounts: Record<string, number> = {};
    let totalLatency = 0;
    for (const t of tunnels) {
      pathCounts[t.active_path] = (pathCounts[t.active_path] || 0) + 1;
      totalLatency += t.latency_ms;
    }
    return {
      pathCounts,
      avgLatency: tunnels.length > 0 ? Math.round(totalLatency / tunnels.length) : 0,
      total: tunnels.length,
    };
  }, [tunnels]);

  const allPaths = ['quic', 'wss', 'webrtc', 'icmp', 'dns'];

  return (
    <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-sm font-medium text-slate-300">隧道状态</h3>
        <span className="text-xs text-slate-500">
          avg {summary.avgLatency}ms · {summary.total} nodes
        </span>
      </div>

      {/* 协议状态灯 */}
      <div className="flex gap-3">
        {allPaths.map((path) => {
          const cfg = pathConfig[path];
          const count = summary.pathCounts[path] || 0;
          const isActive = count > 0;

          return (
            <div
              key={path}
              className={`flex-1 rounded-lg border p-3 text-center transition-all ${
                isActive ? dimColorMap[cfg.color] : 'bg-slate-950 border-slate-800'
              }`}
            >
              {/* 状态灯 */}
              <div className="flex justify-center mb-2">
                <div className={`w-3 h-3 rounded-full shadow-lg ${
                  isActive ? colorMap[cfg.color] : 'bg-slate-700'
                }`} />
              </div>
              {/* 标签 */}
              <div className={`text-xs font-mono ${isActive ? 'text-slate-200' : 'text-slate-600'}`}>
                {cfg.label}
              </div>
              {/* 计数 */}
              {isActive && (
                <div className="text-xs text-slate-400 mt-1">{count} gw</div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
};

export default TunnelIndicator;
