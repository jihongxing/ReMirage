// GatewayCard - 单个网关状态卡片
// 颜色编码：绿=在线, 黄=降级, 红=威胁, 灰=离线
import { useMemo } from 'react';

export interface GatewayStatus {
  gateway_id: string;
  status: 'online' | 'degraded' | 'threat' | 'offline';
  cpu_percent: number;
  memory_mb: number;
  connections: number;
  threat_level: number;
  tunnel_path: string;
  last_heartbeat: number;
  region?: string;
}

const statusColors = {
  online: 'border-green-500/50 bg-green-950/20',
  degraded: 'border-yellow-500/50 bg-yellow-950/20',
  threat: 'border-red-500/50 bg-red-950/20',
  offline: 'border-slate-700 bg-slate-950/50',
};

const statusDot = {
  online: 'bg-green-400 shadow-green-400/50',
  degraded: 'bg-yellow-400 shadow-yellow-400/50',
  threat: 'bg-red-400 animate-pulse shadow-red-400/50',
  offline: 'bg-slate-600',
};

const tunnelLabels: Record<string, string> = {
  quic: 'QUIC',
  wss: 'WSS',
  webrtc: 'WebRTC',
  icmp: 'ICMP',
  dns: 'DNS',
};

export const GatewayCard = ({ gw }: { gw: GatewayStatus }) => {
  const timeSinceHeartbeat = useMemo(() => {
    const diff = Math.floor((Date.now() / 1000) - gw.last_heartbeat);
    if (diff < 60) return `${diff}s ago`;
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    return `${Math.floor(diff / 3600)}h ago`;
  }, [gw.last_heartbeat]);

  return (
    <div className={`rounded-lg border p-4 transition-all duration-300 ${statusColors[gw.status]}`}>
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <div className={`w-2.5 h-2.5 rounded-full shadow-lg ${statusDot[gw.status]}`} />
          <span className="text-sm font-mono text-slate-200 truncate max-w-[120px]">
            {gw.gateway_id}
          </span>
        </div>
        {gw.region && (
          <span className="text-xs text-slate-500">{gw.region}</span>
        )}
      </div>

      {/* Metrics */}
      <div className="grid grid-cols-2 gap-2 text-xs">
        <div>
          <span className="text-slate-500">CPU</span>
          <div className="mt-1 h-1.5 bg-slate-800 rounded-full overflow-hidden">
            <div
              className={`h-full rounded-full transition-all ${
                gw.cpu_percent > 80 ? 'bg-red-500' : gw.cpu_percent > 50 ? 'bg-yellow-500' : 'bg-green-500'
              }`}
              style={{ width: `${Math.min(gw.cpu_percent, 100)}%` }}
            />
          </div>
        </div>
        <div>
          <span className="text-slate-500">MEM</span>
          <span className="ml-1 text-slate-300">{gw.memory_mb}MB</span>
        </div>
        <div>
          <span className="text-slate-500">CONN</span>
          <span className="ml-1 text-slate-300">{gw.connections}</span>
        </div>
        <div>
          <span className="text-slate-500">THREAT</span>
          <span className={`ml-1 ${gw.threat_level > 3 ? 'text-red-400' : 'text-slate-300'}`}>
            L{gw.threat_level}
          </span>
        </div>
      </div>

      {/* Footer */}
      <div className="flex items-center justify-between mt-3 pt-2 border-t border-slate-800">
        <span className={`text-xs px-1.5 py-0.5 rounded font-mono ${
          gw.tunnel_path === 'quic' ? 'bg-cyan-900/50 text-cyan-300' :
          gw.tunnel_path === 'wss' ? 'bg-blue-900/50 text-blue-300' :
          gw.tunnel_path === 'icmp' ? 'bg-orange-900/50 text-orange-300' :
          'bg-slate-800 text-slate-400'
        }`}>
          {tunnelLabels[gw.tunnel_path] || gw.tunnel_path?.toUpperCase() || 'N/A'}
        </span>
        <span className="text-xs text-slate-600">{timeSinceHeartbeat}</span>
      </div>
    </div>
  );
};

export default GatewayCard;
