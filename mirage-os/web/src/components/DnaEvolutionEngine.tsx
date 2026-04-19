// DNA 进化控制台 - 指纹拟态与版本可视化
import { useState, useEffect } from 'react';

interface DNATemplate {
  profileId: string;
  browser: string;
  version: string;
  os: string;
  ja4: string;
  tcpWindow: number;
  ttl: number;
  checksum: string;
  checksumValid: boolean;
  similarity: number; // 与真实浏览器的相似度
  updatedAt: number;
  syncStatus: 'synced' | 'syncing' | 'pending' | 'error';
}

interface SyncProgress {
  nodeId: string;
  region: string;
  progress: number;
  status: 'completed' | 'syncing' | 'waiting' | 'failed';
}

interface RealBrowserFingerprint {
  name: string;
  ja4: string;
  tcpWindow: number;
  ttl: number;
}

const REAL_BROWSERS: RealBrowserFingerprint[] = [
  { name: 'Chrome 150 (Windows)', ja4: 't13d1516h2_8daaf6152771_e5627efa2ab1', tcpWindow: 65535, ttl: 128 },
  { name: 'Chrome 150 (macOS)', ja4: 't13d1516h2_8daaf6152771_b0da82dd1658', tcpWindow: 65535, ttl: 64 },
  { name: 'Firefox 130 (Linux)', ja4: 't13d1711h2_523f7e4a5c2d_cd8dafe26982', tcpWindow: 65535, ttl: 64 },
  { name: 'Safari 18 (macOS)', ja4: 't13d1412h2_a09f3c2e8b1d_f2e8c9a1b3d5', tcpWindow: 65535, ttl: 64 },
];

export default function DnaEvolutionEngine() {
  const [dnaTemplates, setDnaTemplates] = useState<DNATemplate[]>([]);
  const [syncProgress, setSyncProgress] = useState<SyncProgress[]>([]);
  const [selectedDna, setSelectedDna] = useState<DNATemplate | null>(null);
  const [isHotUpdating, setIsHotUpdating] = useState(false);

  useEffect(() => {
    // 模拟 DNA 模板数据
    const mockTemplates: DNATemplate[] = [
      {
        profileId: 'chrome_v150_win',
        browser: 'Chrome',
        version: '150.0.6422.112',
        os: 'Windows 11',
        ja4: 't13d1516h2_8daaf6152771_e5627efa2ab1',
        tcpWindow: 65535,
        ttl: 128,
        checksum: 'a1b2c3d4',
        checksumValid: true,
        similarity: 99.7,
        updatedAt: Date.now() - 3600000,
        syncStatus: 'synced',
      },
      {
        profileId: 'chrome_v150_mac',
        browser: 'Chrome',
        version: '150.0.6422.112',
        os: 'macOS 14',
        ja4: 't13d1516h2_8daaf6152771_b0da82dd1658',
        tcpWindow: 65535,
        ttl: 64,
        checksum: 'e5f6g7h8',
        checksumValid: true,
        similarity: 98.9,
        updatedAt: Date.now() - 7200000,
        syncStatus: 'synced',
      },
      {
        profileId: 'firefox_v130_linux',
        browser: 'Firefox',
        version: '130.0',
        os: 'Ubuntu 24.04',
        ja4: 't13d1711h2_523f7e4a5c2d_cd8dafe26982',
        tcpWindow: 65535,
        ttl: 64,
        checksum: 'i9j0k1l2',
        checksumValid: true,
        similarity: 97.2,
        updatedAt: Date.now() - 86400000,
        syncStatus: 'syncing',
      },
      {
        profileId: 'safari_v18_mac',
        browser: 'Safari',
        version: '18.0',
        os: 'macOS 15',
        ja4: 't13d1412h2_a09f3c2e8b1d_f2e8c9a1b3d5',
        tcpWindow: 65535,
        ttl: 64,
        checksum: 'm3n4o5p6',
        checksumValid: false, // 校验失败示例
        similarity: 95.1,
        updatedAt: Date.now() - 172800000,
        syncStatus: 'error',
      },
    ];

    const mockSyncProgress: SyncProgress[] = [
      { nodeId: 'gw-sg-01', region: '新加坡', progress: 100, status: 'completed' },
      { nodeId: 'gw-de-01', region: '法兰克福', progress: 100, status: 'completed' },
      { nodeId: 'gw-us-01', region: '美西', progress: 78, status: 'syncing' },
      { nodeId: 'gw-jp-01', region: '东京', progress: 45, status: 'syncing' },
      { nodeId: 'gw-uk-01', region: '伦敦', progress: 0, status: 'waiting' },
    ];

    setDnaTemplates(mockTemplates);
    setSyncProgress(mockSyncProgress);
  }, []);

  // 模拟热更新
  const triggerHotUpdate = () => {
    setIsHotUpdating(true);
    setSyncProgress(prev => prev.map(p => ({
      ...p,
      progress: p.status === 'completed' ? 100 : 0,
      status: p.status === 'completed' ? 'completed' : 'syncing',
    })));

    // 模拟进度更新
    const interval = setInterval(() => {
      setSyncProgress(prev => {
        const updated = prev.map(p => {
          if (p.status === 'syncing' && p.progress < 100) {
            const newProgress = Math.min(p.progress + Math.random() * 15, 100);
            return {
              ...p,
              progress: newProgress,
              status: newProgress >= 100 ? 'completed' as const : 'syncing' as const,
            };
          }
          if (p.status === 'waiting') {
            return { ...p, status: 'syncing' as const, progress: Math.random() * 10 };
          }
          return p;
        });

        if (updated.every(p => p.status === 'completed')) {
          clearInterval(interval);
          setIsHotUpdating(false);
        }

        return updated;
      });
    }, 500);
  };

  const getSimilarityColor = (similarity: number) => {
    if (similarity >= 98) return 'text-green-400';
    if (similarity >= 95) return 'text-yellow-400';
    if (similarity >= 90) return 'text-orange-400';
    return 'text-red-400';
  };

  const getSyncStatusBadge = (status: string) => {
    switch (status) {
      case 'synced':
        return <span className="px-2 py-0.5 bg-green-500/20 text-green-400 text-xs rounded">已同步</span>;
      case 'syncing':
        return <span className="px-2 py-0.5 bg-blue-500/20 text-blue-400 text-xs rounded animate-pulse">同步中</span>;
      case 'pending':
        return <span className="px-2 py-0.5 bg-yellow-500/20 text-yellow-400 text-xs rounded">待同步</span>;
      case 'error':
        return <span className="px-2 py-0.5 bg-red-500/20 text-red-400 text-xs rounded">异常</span>;
      default:
        return null;
    }
  };

  const formatTime = (ts: number) => {
    const diff = Date.now() - ts;
    if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`;
    if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`;
    return `${Math.floor(diff / 86400000)} 天前`;
  };

  return (
    <div className="bg-gray-900 rounded-lg p-6 space-y-6">
      {/* 标题 */}
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          🧬 DNA 进化控制台
        </h2>
        <button
          onClick={triggerHotUpdate}
          disabled={isHotUpdating}
          className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
            isHotUpdating
              ? 'bg-gray-700 text-gray-400 cursor-not-allowed'
              : 'bg-cyan-600 hover:bg-cyan-500 text-white'
          }`}
        >
          {isHotUpdating ? '同步中...' : '🔄 触发热更新'}
        </button>
      </div>

      {/* DNA 模板列表 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          📋 指纹模板库
          <span className="text-xs text-gray-500">({dnaTemplates.length} 个模板)</span>
        </h3>
        <div className="space-y-3">
          {dnaTemplates.map(dna => (
            <div
              key={dna.profileId}
              onClick={() => setSelectedDna(dna)}
              className={`bg-gray-900 rounded-lg p-4 cursor-pointer transition-all border-2 ${
                selectedDna?.profileId === dna.profileId
                  ? 'border-cyan-500'
                  : 'border-transparent hover:border-gray-700'
              } ${!dna.checksumValid ? 'border-l-4 border-l-red-500' : ''}`}
            >
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-3">
                  <span className="text-white font-medium">{dna.browser} {dna.version}</span>
                  <span className="text-gray-500 text-sm">{dna.os}</span>
                  {getSyncStatusBadge(dna.syncStatus)}
                </div>
                <div className="flex items-center gap-4">
                  {/* Checksum 状态 */}
                  <div className="flex items-center gap-1">
                    {dna.checksumValid ? (
                      <span className="text-green-400 text-xs">✓ {dna.checksum}</span>
                    ) : (
                      <span className="text-red-400 text-xs">✗ 校验失败</span>
                    )}
                  </div>
                  {/* 相似度 */}
                  <div className={`text-lg font-bold ${getSimilarityColor(dna.similarity)}`}>
                    {dna.similarity}%
                  </div>
                </div>
              </div>
              
              <div className="flex items-center justify-between text-xs text-gray-500">
                <code className="font-mono">{dna.profileId}</code>
                <span>更新于 {formatTime(dna.updatedAt)}</span>
              </div>

              {/* 相似度进度条 */}
              <div className="w-full bg-gray-700 rounded-full h-1 mt-2">
                <div
                  className={`h-1 rounded-full ${
                    dna.similarity >= 98 ? 'bg-green-500' :
                    dna.similarity >= 95 ? 'bg-yellow-500' :
                    dna.similarity >= 90 ? 'bg-orange-500' : 'bg-red-500'
                  }`}
                  style={{ width: `${dna.similarity}%` }}
                />
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* 指纹相似度比对 */}
      {selectedDna && (
        <div className="bg-gray-800 rounded-lg p-4">
          <h3 className="text-white font-medium mb-4 flex items-center gap-2">
            🔬 指纹相似度比对
            <span className="text-xs text-gray-500">{selectedDna.profileId}</span>
          </h3>
          <div className="grid grid-cols-2 gap-4">
            {/* 当前 DNA */}
            <div className="bg-gray-900 rounded-lg p-4">
              <div className="text-cyan-400 text-sm mb-3">当前 DNA</div>
              <div className="space-y-2 text-sm font-mono">
                <div className="flex justify-between">
                  <span className="text-gray-400">JA4:</span>
                  <span className="text-white">{selectedDna.ja4.slice(0, 20)}...</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-400">TCP Window:</span>
                  <span className="text-white">{selectedDna.tcpWindow}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-400">TTL:</span>
                  <span className="text-white">{selectedDna.ttl}</span>
                </div>
              </div>
            </div>

            {/* 真实浏览器 */}
            <div className="bg-gray-900 rounded-lg p-4">
              <div className="text-green-400 text-sm mb-3">真实 {selectedDna.browser}</div>
              <div className="space-y-2 text-sm font-mono">
                {REAL_BROWSERS.filter(b => b.name.includes(selectedDna.browser)).slice(0, 1).map(real => (
                  <div key={real.name}>
                    <div className="flex justify-between">
                      <span className="text-gray-400">JA4:</span>
                      <span className="text-white">{real.ja4.slice(0, 20)}...</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-400">TCP Window:</span>
                      <span className="text-white">{real.tcpWindow}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-400">TTL:</span>
                      <span className="text-white">{real.ttl}</span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>

          {/* 匹配度仪表盘 */}
          <div className="mt-4 text-center">
            <div className={`text-4xl font-bold ${getSimilarityColor(selectedDna.similarity)}`}>
              {selectedDna.similarity}%
            </div>
            <div className="text-gray-400 text-sm mt-1">指纹匹配度</div>
            <div className="text-xs text-gray-500 mt-2">
              {selectedDna.similarity >= 98 ? '✓ 高度拟态，难以区分' :
               selectedDna.similarity >= 95 ? '⚠ 轻微偏差，建议优化' :
               '✗ 偏差较大，需要更新'}
            </div>
          </div>
        </div>
      )}

      {/* 热更新同步进度 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          🌐 节点同步进度
          {isHotUpdating && <span className="text-xs text-cyan-400 animate-pulse">热更新中...</span>}
        </h3>
        <div className="space-y-3">
          {syncProgress.map(node => (
            <div key={node.nodeId} className="flex items-center gap-4">
              <div className="w-24 text-sm">
                <div className="text-white">{node.region}</div>
                <div className="text-xs text-gray-500">{node.nodeId}</div>
              </div>
              <div className="flex-1">
                <div className="w-full bg-gray-700 rounded-full h-2">
                  <div
                    className={`h-2 rounded-full transition-all duration-300 ${
                      node.status === 'completed' ? 'bg-green-500' :
                      node.status === 'syncing' ? 'bg-cyan-500' :
                      node.status === 'failed' ? 'bg-red-500' : 'bg-gray-600'
                    }`}
                    style={{ width: `${node.progress}%` }}
                  />
                </div>
              </div>
              <div className="w-16 text-right">
                <span className={`text-sm font-mono ${
                  node.status === 'completed' ? 'text-green-400' :
                  node.status === 'syncing' ? 'text-cyan-400' :
                  node.status === 'failed' ? 'text-red-400' : 'text-gray-500'
                }`}>
                  {node.progress.toFixed(0)}%
                </span>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Checksum 异常警告 */}
      {dnaTemplates.some(d => !d.checksumValid) && (
        <div className="bg-red-500/20 border border-red-500 rounded-lg p-4 flex items-center gap-3">
          <span className="text-2xl">⚠️</span>
          <div>
            <div className="text-red-400 font-medium">检测到 DNA 完整性异常</div>
            <div className="text-sm text-gray-400">
              {dnaTemplates.filter(d => !d.checksumValid).map(d => d.profileId).join(', ')} 校验失败，可能已被篡改
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
