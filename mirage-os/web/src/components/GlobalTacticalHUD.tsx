// 全球态势看板 - 展示全球节点的信誉分布与战损统计
import { useState, useEffect } from 'react';

interface RegionReputation {
  region: string;
  code: string;
  lat: number;
  lng: number;
  reputation: number;
  activeNodes: number;
  burnedDomains: number;
  lastIncident?: string;
}

interface HijackingFingerprint {
  id: string;
  region: string;
  type: string;
  pattern: string;
  detectedAt: string;
  affectedDomains: number;
}

interface BurnRateStats {
  hour: string;
  burned: number;
  rotated: number;
}

// 内存指标（对应 Go 端 MemoryMetrics）
interface MemoryMetrics {
  ip_cache_count: number;
  ip_cache_max: number;
  dna_cache_count: number;
  dirty_count: number;
  cache_hit_rate: number;
  read_hits: number;
  read_total: number;
  evict_count: number;
  file_size_bytes: number;
}

export default function GlobalTacticalHUD() {
  const [regions, setRegions] = useState<RegionReputation[]>([]);
  const [fingerprints, setFingerprints] = useState<HijackingFingerprint[]>([]);
  const [burnRate, setBurnRate] = useState<BurnRateStats[]>([]);
  const [totalBurned24h, setTotalBurned24h] = useState(0);
  const [avgReputation, setAvgReputation] = useState(0);
  const [memoryMetrics, setMemoryMetrics] = useState<MemoryMetrics | null>(null);

  useEffect(() => {
    // 模拟数据
    const mockRegions: RegionReputation[] = [
      { region: '新加坡', code: 'SG', lat: 1.35, lng: 103.82, reputation: 92, activeNodes: 5, burnedDomains: 2 },
      { region: '法兰克福', code: 'DE', lat: 50.11, lng: 8.68, reputation: 95, activeNodes: 4, burnedDomains: 1 },
      { region: '美西', code: 'US-W', lat: 37.77, lng: -122.42, reputation: 88, activeNodes: 6, burnedDomains: 3 },
      { region: '东京', code: 'JP', lat: 35.68, lng: 139.69, reputation: 91, activeNodes: 3, burnedDomains: 1 },
      { region: '苏黎世', code: 'CH', lat: 47.37, lng: 8.54, reputation: 98, activeNodes: 2, burnedDomains: 0 },
      { region: '伦敦', code: 'UK', lat: 51.51, lng: -0.13, reputation: 85, activeNodes: 4, burnedDomains: 4 },
      { region: '悉尼', code: 'AU', lat: -33.87, lng: 151.21, reputation: 93, activeNodes: 2, burnedDomains: 1 },
      { region: '圣保罗', code: 'BR', lat: -23.55, lng: -46.63, reputation: 78, activeNodes: 3, burnedDomains: 5 },
    ];

    const mockFingerprints: HijackingFingerprint[] = [
      {
        id: 'fp-001',
        region: 'BR',
        type: 'HTTP 302 Redirect',
        pattern: 'Location: http://blocked.gov.br/warning',
        detectedAt: '5 分钟前',
        affectedDomains: 3,
      },
      {
        id: 'fp-002',
        region: 'UK',
        type: 'Content Injection',
        pattern: 'Content-Length: 2048 (expected: 0)',
        detectedAt: '12 分钟前',
        affectedDomains: 2,
      },
      {
        id: 'fp-003',
        region: 'US-W',
        type: 'TCP RST Injection',
        pattern: 'RST after TLS ClientHello',
        detectedAt: '28 分钟前',
        affectedDomains: 1,
      },
    ];

    const mockBurnRate: BurnRateStats[] = [
      { hour: '00:00', burned: 2, rotated: 3 },
      { hour: '04:00', burned: 1, rotated: 2 },
      { hour: '08:00', burned: 4, rotated: 5 },
      { hour: '12:00', burned: 3, rotated: 4 },
      { hour: '16:00', burned: 5, rotated: 6 },
      { hour: '20:00', burned: 2, rotated: 3 },
    ];

    setRegions(mockRegions);
    setFingerprints(mockFingerprints);
    setBurnRate(mockBurnRate);
    setTotalBurned24h(mockRegions.reduce((sum, r) => sum + r.burnedDomains, 0));
    setAvgReputation(Math.round(mockRegions.reduce((sum, r) => sum + r.reputation, 0) / mockRegions.length));

    // 模拟内存指标数据
    const mockMemoryMetrics: MemoryMetrics = {
      ip_cache_count: 45230,
      ip_cache_max: 100000,
      dna_cache_count: 156,
      dirty_count: 23,
      cache_hit_rate: 100.0,
      read_hits: 1523456,
      read_total: 1523456,
      evict_count: 1200,
      file_size_bytes: 52428800, // 50MB
    };
    setMemoryMetrics(mockMemoryMetrics);
  }, []);

  const getReputationColor = (score: number) => {
    if (score >= 90) return 'bg-green-500';
    if (score >= 70) return 'bg-yellow-500';
    if (score >= 50) return 'bg-orange-500';
    return 'bg-red-500';
  };

  const getReputationTextColor = (score: number) => {
    if (score >= 90) return 'text-green-400';
    if (score >= 70) return 'text-yellow-400';
    if (score >= 50) return 'text-orange-400';
    return 'text-red-400';
  };

  const maxBurned = Math.max(...burnRate.map(b => b.burned), 1);

  return (
    <div className="bg-gray-900 rounded-lg p-6 space-y-6">
      {/* 标题和统计 */}
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          🌍 全球态势看板
        </h2>
        <div className="flex items-center gap-6">
          <div className="text-center">
            <div className="text-2xl font-bold text-white">{avgReputation}%</div>
            <div className="text-xs text-gray-500">平均信誉</div>
          </div>
          <div className="text-center">
            <div className="text-2xl font-bold text-red-400">{totalBurned24h}</div>
            <div className="text-xs text-gray-500">24h 战损</div>
          </div>
          <div className="text-center">
            <div className="text-2xl font-bold text-green-400">{regions.reduce((s, r) => s + r.activeNodes, 0)}</div>
            <div className="text-xs text-gray-500">活跃节点</div>
          </div>
        </div>
      </div>

      {/* 信誉分热力图 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          🗺️ 信誉分热力图
        </h3>
        <div className="grid grid-cols-4 gap-3">
          {regions.map(region => (
            <div
              key={region.code}
              className="bg-gray-900 rounded-lg p-4 relative overflow-hidden"
            >
              {/* 背景热力 */}
              <div
                className={`absolute inset-0 opacity-20 ${getReputationColor(region.reputation)}`}
              />
              
              <div className="relative z-10">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-white font-medium">{region.region}</span>
                  <span className="text-xs text-gray-500">{region.code}</span>
                </div>
                
                <div className={`text-3xl font-bold ${getReputationTextColor(region.reputation)}`}>
                  {region.reputation}%
                </div>
                
                <div className="flex items-center justify-between mt-2 text-xs">
                  <span className="text-gray-400">
                    🟢 {region.activeNodes} 节点
                  </span>
                  <span className="text-red-400">
                    💀 {region.burnedDomains} 战损
                  </span>
                </div>

                {/* 信誉条 */}
                <div className="w-full bg-gray-700 rounded-full h-1 mt-2">
                  <div
                    className={`h-1 rounded-full ${getReputationColor(region.reputation)}`}
                    style={{ width: `${region.reputation}%` }}
                  />
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* 封锁特征捕获 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          🔍 封锁特征捕获
          <span className="text-xs text-gray-500">({fingerprints.length} 个活跃指纹)</span>
        </h3>
        <div className="space-y-3">
          {fingerprints.map(fp => (
            <div
              key={fp.id}
              className="bg-gray-900 rounded-lg p-3 border-l-4 border-red-500"
            >
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  <span className="px-2 py-0.5 bg-red-500/20 text-red-400 text-xs rounded">
                    {fp.type}
                  </span>
                  <span className="text-gray-400 text-sm">{fp.region}</span>
                </div>
                <span className="text-xs text-gray-500">{fp.detectedAt}</span>
              </div>
              <code className="text-xs text-yellow-400 font-mono block bg-black/30 p-2 rounded">
                {fp.pattern}
              </code>
              <div className="text-xs text-gray-500 mt-2">
                影响域名: {fp.affectedDomains} 个
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* 转生频率分析 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          📊 转生频率分析
          <span className="text-xs text-gray-500">(过去 24 小时)</span>
        </h3>
        <div className="flex items-end justify-between h-32 gap-2">
          {burnRate.map((stat, idx) => (
            <div key={idx} className="flex-1 flex flex-col items-center gap-1">
              <div className="w-full flex flex-col items-center gap-1" style={{ height: '100px' }}>
                {/* 转生柱 */}
                <div
                  className="w-full bg-green-500/50 rounded-t"
                  style={{ height: `${(stat.rotated / maxBurned) * 80}px` }}
                />
                {/* 战损柱 */}
                <div
                  className="w-full bg-red-500 rounded-t"
                  style={{ height: `${(stat.burned / maxBurned) * 80}px` }}
                />
              </div>
              <span className="text-xs text-gray-500">{stat.hour}</span>
            </div>
          ))}
        </div>
        <div className="flex items-center justify-center gap-6 mt-4 text-xs">
          <div className="flex items-center gap-1">
            <span className="w-3 h-3 bg-red-500 rounded" /> 战损
          </div>
          <div className="flex items-center gap-1">
            <span className="w-3 h-3 bg-green-500/50 rounded" /> 转生
          </div>
        </div>
      </div>

      {/* 域名池状态 */}
      <div className="grid grid-cols-3 gap-4">
        <div className="bg-gray-800 rounded-lg p-4 text-center">
          <div className="text-3xl font-bold text-green-400">12</div>
          <div className="text-sm text-gray-400">可用域名</div>
        </div>
        <div className="bg-gray-800 rounded-lg p-4 text-center">
          <div className="text-3xl font-bold text-yellow-400">5</div>
          <div className="text-sm text-gray-400">预热中</div>
        </div>
        <div className="bg-gray-800 rounded-lg p-4 text-center">
          <div className="text-3xl font-bold text-red-400">{totalBurned24h}</div>
          <div className="text-sm text-gray-400">已废弃</div>
        </div>
      </div>

      {/* 警告提示 */}
      {totalBurned24h > 10 && (
        <div className="bg-red-500/20 border border-red-500 rounded-lg p-4 flex items-center gap-3">
          <span className="text-2xl">⚠️</span>
          <div>
            <div className="text-red-400 font-medium">域名消耗速度过快</div>
            <div className="text-sm text-gray-400">
              过去 24 小时消耗 {totalBurned24h} 个域名，建议向域名池补充资源
            </div>
          </div>
        </div>
      )}

      {/* 内存 vs 磁盘状态 */}
      {memoryMetrics && (
        <div className="bg-gray-800 rounded-lg p-4">
          <h3 className="text-white font-medium mb-4 flex items-center gap-2">
            💾 内存 vs 磁盘状态
            <span className="px-2 py-0.5 bg-green-500/20 text-green-400 text-xs rounded">
              实时
            </span>
          </h3>
          
          <div className="grid grid-cols-2 gap-4 mb-4">
            {/* 缓存命中率 */}
            <div className="bg-gray-900 rounded-lg p-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-gray-400 text-sm">缓存命中率</span>
                <span className="text-green-400 font-bold text-2xl">
                  {memoryMetrics.cache_hit_rate.toFixed(1)}%
                </span>
              </div>
              <div className="w-full bg-gray-700 rounded-full h-2">
                <div
                  className="h-2 rounded-full bg-green-500"
                  style={{ width: `${memoryMetrics.cache_hit_rate}%` }}
                />
              </div>
              <div className="text-xs text-gray-500 mt-2">
                {memoryMetrics.read_hits.toLocaleString()} / {memoryMetrics.read_total.toLocaleString()} 次命中
              </div>
            </div>

            {/* 待刷盘队列 */}
            <div className="bg-gray-900 rounded-lg p-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-gray-400 text-sm">待刷盘队列</span>
                <span className={`font-bold text-2xl ${memoryMetrics.dirty_count > 50 ? 'text-yellow-400' : 'text-blue-400'}`}>
                  {memoryMetrics.dirty_count}
                </span>
              </div>
              <div className="w-full bg-gray-700 rounded-full h-2">
                <div
                  className={`h-2 rounded-full ${memoryMetrics.dirty_count > 50 ? 'bg-yellow-500' : 'bg-blue-500'}`}
                  style={{ width: `${Math.min(memoryMetrics.dirty_count, 100)}%` }}
                />
              </div>
              <div className="text-xs text-gray-500 mt-2">
                异步批量写入中（30s / 100条触发）
              </div>
            </div>
          </div>

          {/* IP 缓存使用率 */}
          <div className="bg-gray-900 rounded-lg p-4 mb-4">
            <div className="flex items-center justify-between mb-2">
              <span className="text-gray-400 text-sm">IP 信誉缓存</span>
              <span className="text-white">
                {memoryMetrics.ip_cache_count.toLocaleString()} / {memoryMetrics.ip_cache_max.toLocaleString()}
              </span>
            </div>
            <div className="w-full bg-gray-700 rounded-full h-3">
              <div
                className={`h-3 rounded-full ${
                  memoryMetrics.ip_cache_count / memoryMetrics.ip_cache_max > 0.9
                    ? 'bg-red-500'
                    : memoryMetrics.ip_cache_count / memoryMetrics.ip_cache_max > 0.7
                    ? 'bg-yellow-500'
                    : 'bg-cyan-500'
                }`}
                style={{ width: `${(memoryMetrics.ip_cache_count / memoryMetrics.ip_cache_max) * 100}%` }}
              />
            </div>
            <div className="flex items-center justify-between text-xs text-gray-500 mt-2">
              <span>LRU 淘汰: {memoryMetrics.evict_count.toLocaleString()} 条</span>
              <span>DNA 模板: {memoryMetrics.dna_cache_count} 个</span>
            </div>
          </div>

          {/* IO 削峰对比 */}
          <div className="bg-gray-900 rounded-lg p-4">
            <div className="text-gray-400 text-sm mb-3">IO 削峰效果</div>
            <div className="grid grid-cols-2 gap-4">
              <div className="text-center">
                <div className="text-red-400 text-xs mb-1">优化前</div>
                <div className="text-2xl font-bold text-red-400">~5000</div>
                <div className="text-xs text-gray-500">IOPS/s</div>
              </div>
              <div className="text-center">
                <div className="text-green-400 text-xs mb-1">优化后</div>
                <div className="text-2xl font-bold text-green-400">~3</div>
                <div className="text-xs text-gray-500">IOPS/s</div>
              </div>
            </div>
            <div className="text-center mt-3">
              <span className="px-3 py-1 bg-green-500/20 text-green-400 text-sm rounded-full">
                ↓ 99.9% IO 降低
              </span>
            </div>
          </div>

          {/* 磁盘文件大小 */}
          <div className="flex items-center justify-between mt-4 text-sm">
            <span className="text-gray-400">BoltDB 冷存储</span>
            <span className="text-gray-300">
              {(memoryMetrics.file_size_bytes / 1024 / 1024).toFixed(1)} MB
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
