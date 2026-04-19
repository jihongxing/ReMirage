// 用户仪表盘 - 匿名资产视角（取证对抗增强）
import { useState, useEffect } from 'react';
import { useMirageSocket } from '../hooks/useMirageSocket';
import { fuzzyTime, maskUID, ghostStore, handleSelfDestruct } from '../utils/GhostStorage';
import DomainConsumables from '../components/DomainConsumables';

interface UserStats {
  uid: string;
  creditLevel: number;
  quotaRemaining: number;
  quotaTotal: number;
  burnRate: number; // bytes/sec
  estimatedDepletion: number; // timestamp
  currentNode: {
    location: string;
    mimicryScore: number;
    latency: number;
  };
  activeConnections: number;
  totalTransferred: number;
  lastUpdate: number;
}

const UserDashboard = () => {
  const { connected, events } = useMirageSocket();
  const [stats, setStats] = useState<UserStats | null>(null);

  useEffect(() => {
    // 从 localStorage 获取 UID（仅鉴权数据）
    const vault = localStorage.getItem('mirage_vault');
    if (vault) {
      const { uid } = JSON.parse(vault);
      // 初始化数据存入内存态 ghostStore
      const initialStats: UserStats = {
        uid,
        creditLevel: 4,
        quotaRemaining: 85.2 * 1024 * 1024 * 1024,
        quotaTotal: 100 * 1024 * 1024 * 1024,
        burnRate: 2.4 * 1024 * 1024,
        estimatedDepletion: Date.now() + 36 * 3600 * 1000,
        currentNode: {
          location: '新加坡',
          mimicryScore: 94.7,
          latency: 42,
        },
        activeConnections: 3,
        totalTransferred: 14.8 * 1024 * 1024 * 1024,
        lastUpdate: Date.now(),
      };
      ghostStore.set('user_stats', initialStats);
      setStats(initialStats);
    }
  }, []);

  // 处理 WebSocket 事件（包括自毁指令）
  useEffect(() => {
    const latestEvent = events[events.length - 1];
    if (!latestEvent) return;
    
    // 监听自毁指令
    if ((latestEvent as any).type === 'self_destruct') {
      handleSelfDestruct();
      return;
    }
    
    // 更新统计数据到内存态存储
    if ((latestEvent as any).type === 'user:stats') {
      const payload = (latestEvent as any).payload;
      setStats(prev => {
        const updated = prev ? { ...prev, ...payload, lastUpdate: Date.now() } : null;
        if (updated) ghostStore.set('user_stats', updated);
        return updated;
      });
    }
  }, [events]);

  const formatBytes = (bytes: number): string => {
    if (bytes >= 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
    if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / 1024).toFixed(1)} KB`;
  };

  const formatTime = (timestamp: number): string => {
    const hours = Math.floor((timestamp - Date.now()) / 3600000);
    if (hours > 24) return `${Math.floor(hours / 24)}d ${hours % 24}h`;
    return `${hours}h`;
  };

  if (!stats) {
    return (
      <div className="min-h-screen bg-slate-950 flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  const quotaPercent = (stats.quotaRemaining / stats.quotaTotal) * 100;

  return (
    <div className="space-y-6">
      {/* Sovereign Identity HUD */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-medium text-white flex items-center gap-2">
            <span>🔐</span> Sovereign Identity
          </h2>
          <div className={`px-2 py-1 rounded text-xs ${connected ? 'bg-green-600/20 text-green-400' : 'bg-red-600/20 text-red-400'}`}>
            {connected ? '● 在线' : '○ 离线'}
          </div>
        </div>
        
        <div className="grid grid-cols-2 gap-6">
          <div>
            <p className="text-xs text-slate-500 mb-1">匿名 UID</p>
            <p className="font-mono text-cyan-400">{maskUID(stats.uid)}</p>
          </div>
          <div>
            <p className="text-xs text-slate-500 mb-1">信用等级</p>
            <div className="flex items-center gap-2">
              <div className="flex gap-0.5">
                {[1, 2, 3, 4, 5].map(i => (
                  <div
                    key={i}
                    className={`w-4 h-4 rounded ${i <= stats.creditLevel ? 'bg-cyan-500' : 'bg-slate-700'}`}
                  />
                ))}
              </div>
              <span className="text-sm text-slate-400">Lv.{stats.creditLevel}</span>
            </div>
          </div>
        </div>
      </div>

      {/* Vitals Monitor */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
          <span>📊</span> Vitals Monitor
        </h2>
        
        <div className="space-y-4">
          {/* 配额进度条 */}
          <div>
            <div className="flex justify-between text-sm mb-2">
              <span className="text-slate-400">配额剩余</span>
              <span className="text-white">{formatBytes(stats.quotaRemaining)} / {formatBytes(stats.quotaTotal)}</span>
            </div>
            <div className="h-3 bg-slate-800 rounded-full overflow-hidden">
              <div
                className={`h-full transition-all duration-500 ${
                  quotaPercent > 50 ? 'bg-cyan-500' : quotaPercent > 20 ? 'bg-yellow-500' : 'bg-red-500'
                }`}
                style={{ width: `${quotaPercent}%` }}
              />
            </div>
          </div>

          <div className="grid grid-cols-3 gap-4 pt-2">
            <div className="bg-black/30 rounded p-3">
              <p className="text-xs text-slate-500 mb-1">烧录速率</p>
              <p className="text-lg font-medium text-orange-400">{formatBytes(stats.burnRate)}/s</p>
            </div>
            <div className="bg-black/30 rounded p-3">
              <p className="text-xs text-slate-500 mb-1">预计耗尽</p>
              <p className="text-lg font-medium text-yellow-400">{formatTime(stats.estimatedDepletion)}</p>
            </div>
            <div className="bg-black/30 rounded p-3">
              <p className="text-xs text-slate-500 mb-1">活跃连接</p>
              <p className="text-lg font-medium text-green-400">{stats.activeConnections}</p>
            </div>
          </div>
        </div>
      </div>

      {/* Node Status */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
          <span>🌐</span> 当前节点
        </h2>
        
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 bg-cyan-600/20 rounded-lg flex items-center justify-center text-2xl">
              🇸🇬
            </div>
            <div>
              <p className="text-white font-medium">{stats.currentNode.location}</p>
              <p className="text-xs text-slate-500">出口节点</p>
            </div>
          </div>
          
          <div className="flex gap-6">
            <div className="text-center">
              <p className="text-2xl font-bold text-cyan-400">{stats.currentNode.mimicryScore}%</p>
              <p className="text-xs text-slate-500">拟态评分</p>
            </div>
            <div className="text-center">
              <p className="text-2xl font-bold text-green-400">{stats.currentNode.latency}ms</p>
              <p className="text-xs text-slate-500">延迟</p>
            </div>
          </div>
        </div>
      </div>

      {/* 累计流量 */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-4">
        <div className="flex items-center justify-between">
          <span className="text-sm text-slate-400">累计传输</span>
          <span className="text-lg font-medium text-white">{formatBytes(stats.totalTransferred)}</span>
        </div>
        <div className="flex items-center justify-between mt-2 pt-2 border-t border-slate-800">
          <span className="text-xs text-slate-500">数据更新</span>
          <span className="text-xs text-slate-500">{fuzzyTime(stats.lastUpdate)}</span>
        </div>
      </div>

      {/* 域名耗材状态 */}
      <DomainConsumables />

      {/* 客户端配置下载（阅后即焚） */}
      <ClientConfigCard />
    </div>
  );
};

/** 阅后即焚配置卡片 */
const ClientConfigCard = () => {
  const [configLink, setConfigLink] = useState<{ url: string; expiresAt: number } | null>(null);
  const [generating, setGenerating] = useState(false);
  const [redeemed, setRedeemed] = useState(false);
  const [copied, setCopied] = useState(false);

  const generateConfig = async () => {
    setGenerating(true);
    try {
      const API_BASE = import.meta.env.VITE_API_BASE || '/api';
      const token = localStorage.getItem('mirage_token');
      const resp = await fetch(`${API_BASE}/billing/generate-config`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
      });
      if (resp.ok) {
        const data = await resp.json();
        setConfigLink({
          url: data.deliveryUrl,
          expiresAt: data.expiresAt,
        });
        setRedeemed(false);
      }
    } catch {
      // 静默失败
    } finally {
      setGenerating(false);
    }
  };

  const copyLink = () => {
    if (configLink) {
      navigator.clipboard.writeText(configLink.url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
      <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
        <span>🔗</span> 客户端配置
      </h2>

      {!configLink ? (
        <div className="text-center py-6">
          <p className="text-slate-400 text-sm mb-3">生成一次性客户端配置链接</p>
          <p className="text-xs text-yellow-400/70 mb-4">⚠️ 链接仅可使用一次，打开后立即销毁</p>
          <button
            onClick={generateConfig}
            disabled={generating}
            className={`px-6 py-3 rounded-lg transition-colors ${
              generating
                ? 'bg-slate-700 text-slate-500 cursor-not-allowed'
                : 'bg-cyan-600 hover:bg-cyan-500 text-white'
            }`}
          >
            {generating ? '生成中...' : '生成配置链接'}
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          <div className="bg-black/50 rounded p-4 border border-cyan-600/30">
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-slate-500">阅后即焚链接</span>
              {redeemed ? (
                <span className="text-xs text-red-400">已销毁</span>
              ) : (
                <span className="text-xs text-green-400">有效</span>
              )}
            </div>
            <div className="flex items-center gap-2">
              <code className="flex-1 text-xs text-cyan-400 break-all font-mono">
                {redeemed ? '████████████████' : configLink.url}
              </code>
              {!redeemed && (
                <button
                  onClick={copyLink}
                  className={`px-3 py-1.5 rounded text-xs transition-colors ${
                    copied ? 'bg-green-600 text-white' : 'bg-slate-700 hover:bg-slate-600 text-slate-300'
                  }`}
                >
                  {copied ? '已复制' : '复制'}
                </button>
              )}
            </div>
          </div>
          <p className="text-xs text-slate-500">
            将此链接粘贴到客户端：<code className="text-cyan-400">./phantom -uri "链接"</code>
          </p>
          <button
            onClick={generateConfig}
            className="w-full py-2 bg-slate-800 hover:bg-slate-700 text-slate-400 rounded text-sm transition-colors"
          >
            重新生成
          </button>
        </div>
      )}
    </div>
  );
};

export default UserDashboard;
