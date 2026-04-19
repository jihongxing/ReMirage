import { useState, useEffect } from 'react';

interface WarfareStats {
  cognitiveBias: CognitiveBiasData;
  resourceArbitrage: ResourceArbitrageData;
  fingerprintDrift: FingerprintDriftData;
  chaosStats: ChaosStatsData;
}

interface CognitiveBiasData {
  totalRepeatTests: number;
  uniqueFakeVulns: number;
  avgTestsPerVuln: number;
  mostTestedVuln: string;
  biasScore: number; // 0-100
}

interface ResourceArbitrageData {
  ourBandwidthKB: number;
  theirComputeHours: number;
  arbitrageRatio: number;
  costSavings: number;
}

interface FingerprintDriftData {
  totalUIDs: number;
  avgProxiesPerUID: number;
  maxProxiesPerUID: number;
  penetrationRate: number; // 穿透率
  driftHistory: DriftPoint[];
}

interface DriftPoint {
  uid: string;
  proxyCount: number;
  blocked: boolean;
}

interface ChaosStatsData {
  reincarnations: number;
  cthulhuActivations: number;
  payloadsMirrored: number;
  activeShadows: number;
}

interface CognitiveWarfareDashboardProps {
  wsUrl?: string;
}

export function CognitiveWarfareDashboard({ wsUrl = 'ws://localhost:8080/ws' }: CognitiveWarfareDashboardProps) {
  const [stats, setStats] = useState<WarfareStats>({
    cognitiveBias: {
      totalRepeatTests: 0,
      uniqueFakeVulns: 0,
      avgTestsPerVuln: 0,
      mostTestedVuln: '-',
      biasScore: 0,
    },
    resourceArbitrage: {
      ourBandwidthKB: 0,
      theirComputeHours: 0,
      arbitrageRatio: 0,
      costSavings: 0,
    },
    fingerprintDrift: {
      totalUIDs: 0,
      avgProxiesPerUID: 0,
      maxProxiesPerUID: 0,
      penetrationRate: 0,
      driftHistory: [],
    },
    chaosStats: {
      reincarnations: 0,
      cthulhuActivations: 0,
      payloadsMirrored: 0,
      activeShadows: 0,
    },
  });

  useEffect(() => {
    const ws = new WebSocket(wsUrl);
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'cognitive_warfare_stats') {
          setStats(data.payload);
        }
      } catch (e) {
        console.error('Failed to parse warfare event:', e);
      }
    };
    return () => ws.close();
  }, [wsUrl]);

  return (
    <div className="bg-gray-900 rounded-lg p-4 text-white">
      <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
        <span className="text-2xl">⚔️</span>
        战神视图 - Cognitive Warfare
      </h2>

      {/* 认知偏差度 */}
      <div className="mb-4">
        <CognitiveBiasPanel data={stats.cognitiveBias} />
      </div>

      {/* 资源置换率 */}
      <div className="mb-4">
        <ResourceArbitragePanel data={stats.resourceArbitrage} />
      </div>

      {/* 指纹漂移轨迹 */}
      <div className="mb-4">
        <FingerprintDriftPanel data={stats.fingerprintDrift} />
      </div>

      {/* 混沌统计 */}
      <div>
        <ChaosStatsPanel data={stats.chaosStats} />
      </div>
    </div>
  );
}

function CognitiveBiasPanel({ data }: { data: CognitiveBiasData }) {
  const biasLevel = data.biasScore >= 80 ? '极高' : data.biasScore >= 60 ? '高' : data.biasScore >= 40 ? '中' : '低';
  const biasColor = data.biasScore >= 80 ? 'text-red-400' : data.biasScore >= 60 ? 'text-orange-400' : data.biasScore >= 40 ? 'text-yellow-400' : 'text-green-400';

  return (
    <div className="bg-gray-800 rounded-lg p-4">
      <h3 className="text-sm text-gray-400 mb-3 flex items-center gap-2">
        <span>🧠</span> 认知偏差度
      </h3>
      <div className="grid grid-cols-2 gap-4">
        <div>
          <div className="text-3xl font-bold">{data.totalRepeatTests}</div>
          <div className="text-xs text-gray-500">重复测试次数</div>
        </div>
        <div>
          <div className="text-3xl font-bold">{data.uniqueFakeVulns}</div>
          <div className="text-xs text-gray-500">假漏洞数量</div>
        </div>
        <div>
          <div className="text-xl font-semibold">{data.avgTestsPerVuln.toFixed(1)}</div>
          <div className="text-xs text-gray-500">平均测试/漏洞</div>
        </div>
        <div>
          <div className={`text-xl font-semibold ${biasColor}`}>{biasLevel}</div>
          <div className="text-xs text-gray-500">误导程度 ({data.biasScore}%)</div>
        </div>
      </div>
      {data.mostTestedVuln !== '-' && (
        <div className="mt-3 p-2 bg-gray-700/50 rounded text-xs">
          <span className="text-gray-400">最受关注:</span> <span className="text-purple-400">{data.mostTestedVuln}</span>
        </div>
      )}
    </div>
  );
}

function ResourceArbitragePanel({ data }: { data: ResourceArbitrageData }) {
  return (
    <div className="bg-gray-800 rounded-lg p-4">
      <h3 className="text-sm text-gray-400 mb-3 flex items-center gap-2">
        <span>💰</span> 资源置换率
      </h3>
      <div className="flex items-center justify-between mb-4">
        <div className="text-center">
          <div className="text-2xl font-bold text-blue-400">{formatKB(data.ourBandwidthKB)}</div>
          <div className="text-xs text-gray-500">我方带宽</div>
        </div>
        <div className="text-2xl text-gray-500">→</div>
        <div className="text-center">
          <div className="text-2xl font-bold text-red-400">{data.theirComputeHours.toFixed(1)}h</div>
          <div className="text-xs text-gray-500">对方算力</div>
        </div>
      </div>
      <div className="flex justify-between items-center">
        <div>
          <span className="text-4xl font-bold text-green-400">{data.arbitrageRatio.toFixed(1)}x</span>
          <span className="text-sm text-gray-400 ml-2">置换比</span>
        </div>
        <div className="text-right">
          <div className="text-lg font-semibold text-yellow-400">${data.costSavings.toFixed(0)}</div>
          <div className="text-xs text-gray-500">节省成本</div>
        </div>
      </div>
    </div>
  );
}

function FingerprintDriftPanel({ data }: { data: FingerprintDriftData }) {
  const penetrationColor = data.penetrationRate >= 95 ? 'text-green-400' : data.penetrationRate >= 80 ? 'text-yellow-400' : 'text-red-400';

  return (
    <div className="bg-gray-800 rounded-lg p-4">
      <h3 className="text-sm text-gray-400 mb-3 flex items-center gap-2">
        <span>🔍</span> 指纹漂移轨迹
      </h3>
      <div className="grid grid-cols-4 gap-3 mb-4">
        <div className="text-center">
          <div className="text-2xl font-bold">{data.totalUIDs}</div>
          <div className="text-xs text-gray-500">追踪 UID</div>
        </div>
        <div className="text-center">
          <div className="text-2xl font-bold">{data.avgProxiesPerUID.toFixed(1)}</div>
          <div className="text-xs text-gray-500">平均代理数</div>
        </div>
        <div className="text-center">
          <div className="text-2xl font-bold text-orange-400">{data.maxProxiesPerUID}</div>
          <div className="text-xs text-gray-500">最大代理数</div>
        </div>
        <div className="text-center">
          <div className={`text-2xl font-bold ${penetrationColor}`}>{data.penetrationRate.toFixed(1)}%</div>
          <div className="text-xs text-gray-500">穿透率</div>
        </div>
      </div>
      
      {/* 漂移历史 */}
      <div className="space-y-1 max-h-32 overflow-y-auto">
        {data.driftHistory.slice(0, 5).map((point, idx) => (
          <div key={idx} className="flex items-center justify-between text-xs bg-gray-700/30 rounded px-2 py-1">
            <span className="text-gray-400 font-mono">{point.uid.slice(0, 12)}...</span>
            <span className="text-purple-400">{point.proxyCount} 代理</span>
            <span className={point.blocked ? 'text-green-400' : 'text-red-400'}>
              {point.blocked ? '✓ 已拦截' : '○ 活跃'}
            </span>
          </div>
        ))}
        {data.driftHistory.length === 0 && (
          <div className="text-gray-500 text-xs text-center py-2">暂无漂移数据</div>
        )}
      </div>
    </div>
  );
}

function ChaosStatsPanel({ data }: { data: ChaosStatsData }) {
  return (
    <div className="bg-gradient-to-br from-purple-900/50 to-red-900/50 rounded-lg p-4">
      <h3 className="text-sm text-gray-300 mb-3 flex items-center gap-2">
        <span>🐙</span> 混沌引擎状态
      </h3>
      <div className="grid grid-cols-4 gap-3">
        <div className="text-center">
          <div className="text-2xl font-bold text-cyan-400">{data.reincarnations}</div>
          <div className="text-xs text-gray-400">影子重生</div>
        </div>
        <div className="text-center">
          <div className="text-2xl font-bold text-red-400">{data.cthulhuActivations}</div>
          <div className="text-xs text-gray-400">克苏鲁激活</div>
        </div>
        <div className="text-center">
          <div className="text-2xl font-bold text-yellow-400">{data.payloadsMirrored}</div>
          <div className="text-xs text-gray-400">载荷回显</div>
        </div>
        <div className="text-center">
          <div className="text-2xl font-bold text-green-400">{data.activeShadows}</div>
          <div className="text-xs text-gray-400">活跃影子</div>
        </div>
      </div>
    </div>
  );
}

function formatKB(kb: number): string {
  if (kb < 1024) return `${kb} KB`;
  if (kb < 1024 * 1024) return `${(kb / 1024).toFixed(1)} MB`;
  return `${(kb / 1024 / 1024).toFixed(1)} GB`;
}

export default CognitiveWarfareDashboard;
