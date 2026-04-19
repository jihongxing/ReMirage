// 域名耗材视图 - 用户直观看到域名消耗状态和自动转生进度
import { useState, useEffect } from 'react';

interface AppHealthStatus {
  name: string;
  icon: string;
  status: 'healthy' | 'degraded' | 'blocked';
  score: number;
  lastCheck: string;
}

interface DomainStatus {
  domain: string;
  reputation: number;
  isActive: boolean;
  burnedAt?: string;
  appHealth: AppHealthStatus[];
}

interface ReincarnationEvent {
  oldDomain: string;
  newDomain: string;
  reason: string;
  timestamp: string;
  progress: number;
}

export default function DomainConsumables() {
  const [domains, setDomains] = useState<DomainStatus[]>([]);
  const [reincarnation, setReincarnation] = useState<ReincarnationEvent | null>(null);
  const [isRotating, setIsRotating] = useState(false);

  // 百大 App 健康度矩阵
  const appMatrix: AppHealthStatus[] = [
    { name: 'WhatsApp', icon: '💬', status: 'healthy', score: 98, lastCheck: '10s ago' },
    { name: 'Telegram', icon: '✈️', status: 'healthy', score: 95, lastCheck: '15s ago' },
    { name: 'TikTok', icon: '🎵', status: 'degraded', score: 72, lastCheck: '8s ago' },
    { name: 'YouTube', icon: '▶️', status: 'healthy', score: 99, lastCheck: '12s ago' },
    { name: 'Instagram', icon: '📷', status: 'healthy', score: 94, lastCheck: '20s ago' },
    { name: 'Discord', icon: '🎮', status: 'healthy', score: 97, lastCheck: '5s ago' },
    { name: 'Twitter/X', icon: '🐦', status: 'degraded', score: 68, lastCheck: '18s ago' },
    { name: 'Netflix', icon: '🎬', status: 'blocked', score: 15, lastCheck: '3s ago' },
    { name: 'Zoom', icon: '📹', status: 'healthy', score: 91, lastCheck: '25s ago' },
    { name: 'Signal', icon: '🔒', status: 'healthy', score: 96, lastCheck: '30s ago' },
  ];

  useEffect(() => {
    // 模拟 WebSocket 数据
    const mockDomains: DomainStatus[] = [
      {
        domain: 'cdn-a1b2c3.example.com',
        reputation: 85,
        isActive: true,
        appHealth: appMatrix,
      },
      {
        domain: 'cdn-d4e5f6.example.com',
        reputation: 100,
        isActive: false,
        appHealth: [],
      },
      {
        domain: 'cdn-g7h8i9.example.com',
        reputation: 100,
        isActive: false,
        appHealth: [],
      },
    ];
    setDomains(mockDomains);
  }, []);

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'healthy': return 'bg-green-500';
      case 'degraded': return 'bg-yellow-500';
      case 'blocked': return 'bg-red-500';
      default: return 'bg-gray-500';
    }
  };

  const getReputationColor = (score: number) => {
    if (score >= 80) return 'text-green-400';
    if (score >= 50) return 'text-yellow-400';
    return 'text-red-400';
  };

  const handleEmergencyRotate = async () => {
    setIsRotating(true);
    setReincarnation({
      oldDomain: domains[0]?.domain || '',
      newDomain: 'cdn-new123.example.com',
      reason: '用户手动触发',
      timestamp: new Date().toISOString(),
      progress: 0,
    });

    // 模拟转生进度
    for (let i = 0; i <= 100; i += 10) {
      await new Promise(r => setTimeout(r, 200));
      setReincarnation(prev => prev ? { ...prev, progress: i } : null);
    }

    setIsRotating(false);
    setReincarnation(null);
  };

  return (
    <div className="bg-gray-900 rounded-lg p-6 space-y-6">
      {/* 标题 */}
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          🦎 域名耗材状态
        </h2>
        <button
          onClick={handleEmergencyRotate}
          disabled={isRotating}
          className={`px-4 py-2 rounded-lg font-medium transition-all ${
            isRotating
              ? 'bg-gray-700 text-gray-400 cursor-not-allowed'
              : 'bg-red-600 hover:bg-red-500 text-white'
          }`}
        >
          {isRotating ? '转生中...' : '🚨 紧急热切'}
        </button>
      </div>

      {/* 转生进度条 */}
      {reincarnation && (
        <div className="bg-gray-800 rounded-lg p-4 border border-yellow-500/50 animate-pulse">
          <div className="flex items-center gap-3 mb-3">
            <div className="w-3 h-3 bg-yellow-500 rounded-full animate-ping" />
            <span className="text-yellow-400 font-medium">节点正在同步转生...</span>
          </div>
          <div className="text-sm text-gray-400 mb-2">
            {reincarnation.oldDomain} → {reincarnation.newDomain}
          </div>
          <div className="w-full bg-gray-700 rounded-full h-2">
            <div
              className="bg-yellow-500 h-2 rounded-full transition-all duration-200"
              style={{ width: `${reincarnation.progress}%` }}
            />
          </div>
          <div className="text-xs text-gray-500 mt-1">
            原因: {reincarnation.reason}
          </div>
        </div>
      )}

      {/* 当前活跃域名 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <div className="flex items-center justify-between mb-4">
          <span className="text-gray-400">当前活跃域名</span>
          <span className="text-green-400 text-sm flex items-center gap-1">
            <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
            在线
          </span>
        </div>
        {domains.filter(d => d.isActive).map(domain => (
          <div key={domain.domain} className="space-y-3">
            <div className="flex items-center justify-between">
              <code className="text-white font-mono">{domain.domain}</code>
              <span className={`font-bold ${getReputationColor(domain.reputation)}`}>
                信誉分: {domain.reputation}
              </span>
            </div>
            
            {/* 信誉度进度条 */}
            <div className="w-full bg-gray-700 rounded-full h-3">
              <div
                className={`h-3 rounded-full transition-all ${
                  domain.reputation >= 80 ? 'bg-green-500' :
                  domain.reputation >= 50 ? 'bg-yellow-500' : 'bg-red-500'
                }`}
                style={{ width: `${domain.reputation}%` }}
              />
            </div>
          </div>
        ))}
      </div>

      {/* 环境存活矩阵 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          🌐 环境存活矩阵
          <span className="text-xs text-gray-500">(百大 App 健康度)</span>
        </h3>
        <div className="grid grid-cols-5 gap-3">
          {appMatrix.map(app => (
            <div
              key={app.name}
              className="bg-gray-900 rounded-lg p-3 text-center relative group"
            >
              {/* 呼吸灯 */}
              <div className={`absolute top-2 right-2 w-2 h-2 rounded-full ${getStatusColor(app.status)} ${
                app.status === 'healthy' ? 'animate-pulse' : ''
              }`} />
              
              <div className="text-2xl mb-1">{app.icon}</div>
              <div className="text-xs text-gray-400 truncate">{app.name}</div>
              <div className={`text-sm font-bold ${getReputationColor(app.score)}`}>
                {app.score}%
              </div>
              
              {/* Tooltip */}
              <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-black rounded text-xs text-white opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap z-10">
                最后检测: {app.lastCheck}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* 影子域名池 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-3 flex items-center gap-2">
          👻 影子域名池
          <span className="text-xs text-green-400">({domains.filter(d => !d.isActive).length} 个预热完成)</span>
        </h3>
        <div className="space-y-2">
          {domains.filter(d => !d.isActive).map(domain => (
            <div
              key={domain.domain}
              className="flex items-center justify-between bg-gray-900 rounded px-3 py-2"
            >
              <code className="text-gray-400 text-sm font-mono">{domain.domain}</code>
              <span className="text-xs text-green-400">✓ 预热完成</span>
            </div>
          ))}
        </div>
      </div>

      {/* 状态图例 */}
      <div className="flex items-center justify-center gap-6 text-xs text-gray-500">
        <div className="flex items-center gap-1">
          <span className="w-2 h-2 bg-green-500 rounded-full" /> 正常
        </div>
        <div className="flex items-center gap-1">
          <span className="w-2 h-2 bg-yellow-500 rounded-full" /> 信誉下降
        </div>
        <div className="flex items-center gap-1">
          <span className="w-2 h-2 bg-red-500 rounded-full" /> 已封禁
        </div>
      </div>
    </div>
  );
}
