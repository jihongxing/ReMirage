import { useState, useEffect, useCallback } from 'react';

interface CortexStats {
  totalFingerprints: number;
  highRiskCount: number;
  trustedCount: number;
  totalAssociatedIPs: number;
  autoBlockRate: number;
  predictiveBlocks: number;
}

interface FingerprintLineage {
  uid: string;
  hash: string;
  threatScore: number;
  ips: Array<{
    ip: string;
    region: string;
    country: string;
    firstSeen: string;
    requestCount: number;
  }>;
}

interface CortexDashboardProps {
  wsUrl?: string;
}

export function CortexDashboard({ wsUrl = 'ws://localhost:8080/ws' }: CortexDashboardProps) {
  const [stats, setStats] = useState<CortexStats>({
    totalFingerprints: 0,
    highRiskCount: 0,
    trustedCount: 0,
    totalAssociatedIPs: 0,
    autoBlockRate: 0,
    predictiveBlocks: 0,
  });
  const [lineages, setLineages] = useState<FingerprintLineage[]>([]);
  const [selectedLineage, setSelectedLineage] = useState<FingerprintLineage | null>(null);

  useEffect(() => {
    const ws = new WebSocket(wsUrl);
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'cortex_stats') {
          setStats(data.payload);
        } else if (data.type === 'fingerprint_lineage') {
          handleLineageUpdate(data.payload);
        }
      } catch (e) {
        console.error('Failed to parse cortex event:', e);
      }
    };
    return () => ws.close();
  }, [wsUrl]);

  const handleLineageUpdate = useCallback((lineage: FingerprintLineage) => {
    setLineages((prev) => {
      const idx = prev.findIndex((l) => l.hash === lineage.hash);
      if (idx >= 0) {
        const updated = [...prev];
        updated[idx] = lineage;
        return updated;
      }
      return [lineage, ...prev].slice(0, 50);
    });
  }, []);

  return (
    <div className="bg-gray-900 rounded-lg p-4 text-white">
      <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
        <span className="text-2xl">🧠</span>
        Cortex 威胁感知中枢
      </h2>

      {/* 核心指标 */}
      <div className="grid grid-cols-3 gap-3 mb-4">
        <div className="bg-gradient-to-br from-purple-600 to-purple-800 rounded-lg p-4">
          <div className="text-3xl font-bold">{stats.totalFingerprints.toLocaleString()}</div>
          <div className="text-sm text-purple-200">唯一指纹总数</div>
          <div className="text-xs text-purple-300 mt-1">大脑进化状态</div>
        </div>
        <div className="bg-gradient-to-br from-red-600 to-red-800 rounded-lg p-4">
          <div className="text-3xl font-bold">{stats.autoBlockRate.toFixed(1)}%</div>
          <div className="text-sm text-red-200">自动封禁比例</div>
          <div className="text-xs text-red-300 mt-1">{stats.highRiskCount} 高危指纹</div>
        </div>
        <div className="bg-gradient-to-br from-green-600 to-green-800 rounded-lg p-4">
          <div className="text-3xl font-bold">{stats.predictiveBlocks.toLocaleString()}</div>
          <div className="text-sm text-green-200">预判拦截数</div>
          <div className="text-xs text-green-300 mt-1">行为发生前拦截</div>
        </div>
      </div>

      {/* 指纹血缘图 */}
      <div className="mb-4">
        <h3 className="text-sm text-gray-400 mb-2">指纹血缘图 (Fingerprint Lineage)</h3>
        <div className="bg-gray-800 rounded overflow-hidden">
          <div className="max-h-64 overflow-y-auto">
            {lineages.map((lineage) => (
              <div
                key={lineage.hash}
                className="p-3 border-b border-gray-700 hover:bg-gray-750 cursor-pointer"
                onClick={() => setSelectedLineage(lineage)}
              >
                <div className="flex justify-between items-center">
                  <div>
                    <span className="font-mono text-xs text-purple-400">{lineage.uid}</span>
                    <span className="mx-2 text-gray-500">→</span>
                    <span className="text-yellow-400">{lineage.ips.length} IPs</span>
                  </div>
                  <ThreatBadge score={lineage.threatScore} />
                </div>
                {/* IP 迁移路径可视化 */}
                <div className="flex items-center gap-1 mt-2 overflow-x-auto">
                  {lineage.ips.slice(0, 6).map((ip, idx) => (
                    <div key={ip.ip} className="flex items-center">
                      {idx > 0 && <span className="text-gray-600 mx-1">→</span>}
                      <span className="px-2 py-0.5 bg-gray-700 rounded text-xs">
                        <span className="text-gray-400">{ip.region}</span>
                        <span className="text-gray-500 mx-1">|</span>
                        <span className="font-mono text-gray-300">{ip.ip.split('.').slice(0, 2).join('.')}.*</span>
                      </span>
                    </div>
                  ))}
                  {lineage.ips.length > 6 && (
                    <span className="text-gray-500 text-xs">+{lineage.ips.length - 6} more</span>
                  )}
                </div>
              </div>
            ))}
            {lineages.length === 0 && (
              <div className="p-4 text-center text-gray-500">暂无指纹血缘数据</div>
            )}
          </div>
        </div>
      </div>

      {/* 详情面板 */}
      {selectedLineage && (
        <div className="bg-gray-800 rounded p-4">
          <div className="flex justify-between items-center mb-3">
            <h3 className="text-sm font-semibold text-purple-400">
              攻击者迁移路径详情
            </h3>
            <button
              onClick={() => setSelectedLineage(null)}
              className="text-gray-500 hover:text-white"
            >
              ✕
            </button>
          </div>
          <div className="grid grid-cols-2 gap-4 text-sm mb-4">
            <div>
              <div className="text-gray-500">指纹 UID</div>
              <div className="font-mono text-purple-400">{selectedLineage.uid}</div>
            </div>
            <div>
              <div className="text-gray-500">威胁分值</div>
              <div className="text-red-400 font-bold">{selectedLineage.threatScore}</div>
            </div>
          </div>
          <div className="text-gray-500 text-xs mb-2">IP 迁移时间线</div>
          <div className="space-y-2 max-h-48 overflow-y-auto">
            {selectedLineage.ips.map((ip, idx) => (
              <div key={ip.ip} className="flex items-center gap-3 text-sm">
                <div className="w-6 text-gray-500">{idx + 1}.</div>
                <div className="font-mono text-yellow-400 w-32">{ip.ip}</div>
                <div className="text-gray-400 w-16">{ip.region}</div>
                <div className="text-gray-500 w-24">{ip.country}</div>
                <div className="text-gray-600 text-xs">{ip.requestCount} 次请求</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 底部统计 */}
      <div className="mt-4 flex justify-between text-xs text-gray-500">
        <div>白名单指纹: {stats.trustedCount}</div>
        <div>关联 IP 总数: {stats.totalAssociatedIPs}</div>
      </div>
    </div>
  );
}

function ThreatBadge({ score }: { score: number }) {
  let color = 'bg-green-500/20 text-green-400';
  let label = '低危';
  if (score >= 80) {
    color = 'bg-red-500/20 text-red-400';
    label = '极危';
  } else if (score >= 50) {
    color = 'bg-orange-500/20 text-orange-400';
    label = '高危';
  } else if (score >= 30) {
    color = 'bg-yellow-500/20 text-yellow-400';
    label = '中危';
  }
  return (
    <span className={`px-2 py-0.5 rounded text-xs ${color}`}>
      {label} ({score})
    </span>
  );
}

export default CortexDashboard;
