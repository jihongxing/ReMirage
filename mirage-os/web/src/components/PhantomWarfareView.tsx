import { useState, useEffect } from 'react';

interface PhantomStats {
  mazeDepth: number;
  maxMazeDepth: number;
  contextHits: number;
  contextMisses: number;
  resourceSinkRatio: number;
  totalRequests: number;
  totalResponseBytes: number;
  templateDistribution: Record<string, number>;
}

interface PhantomWarfareViewProps {
  wsUrl?: string;
}

export function PhantomWarfareView({ wsUrl = 'ws://localhost:8080/ws' }: PhantomWarfareViewProps) {
  const [stats, setStats] = useState<PhantomStats>({
    mazeDepth: 0,
    maxMazeDepth: 0,
    contextHits: 0,
    contextMisses: 0,
    resourceSinkRatio: 0,
    totalRequests: 0,
    totalResponseBytes: 0,
    templateDistribution: {},
  });

  useEffect(() => {
    const ws = new WebSocket(wsUrl);
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'phantom_warfare_stats') {
          setStats(data.payload);
        }
      } catch (e) {
        console.error('Failed to parse phantom warfare event:', e);
      }
    };
    return () => ws.close();
  }, [wsUrl]);

  const contextHitRate = stats.contextHits + stats.contextMisses > 0
    ? (stats.contextHits / (stats.contextHits + stats.contextMisses) * 100).toFixed(1)
    : '0';

  return (
    <div className="bg-gray-900 rounded-lg p-4 text-white">
      <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
        <span className="text-2xl">🎭</span>
        Phantom 心理战视图
      </h2>

      {/* 核心指标 */}
      <div className="grid grid-cols-3 gap-3 mb-4">
        <MetricCard
          label="迷宫深度"
          value={stats.mazeDepth}
          subValue={`最深: ${stats.maxMazeDepth}`}
          color="purple"
          icon="🌀"
        />
        <MetricCard
          label="一致性命中"
          value={`${contextHitRate}%`}
          subValue={`${stats.contextHits} / ${stats.contextHits + stats.contextMisses}`}
          color="blue"
          icon="🎯"
        />
        <MetricCard
          label="资源损耗比"
          value={`${stats.resourceSinkRatio.toFixed(2)}x`}
          subValue={`${formatBytes(stats.totalResponseBytes)} 投喂`}
          color="green"
          icon="💸"
        />
      </div>

      {/* 模板分布 */}
      <div className="bg-gray-800 rounded p-4 mb-4">
        <h3 className="text-sm text-gray-400 mb-3">影子模板分布</h3>
        <div className="space-y-2">
          {Object.entries(stats.templateDistribution).map(([template, count]) => (
            <TemplateBar key={template} template={template} count={count} total={stats.totalRequests} />
          ))}
          {Object.keys(stats.templateDistribution).length === 0 && (
            <div className="text-gray-500 text-sm">暂无数据</div>
          )}
        </div>
      </div>

      {/* 战术说明 */}
      <div className="bg-gray-800/50 rounded p-3 text-xs text-gray-400">
        <div className="flex items-center gap-2 mb-2">
          <span>📊</span>
          <span className="font-semibold">指标解读</span>
        </div>
        <ul className="space-y-1 ml-5">
          <li>迷宫深度越高 = 诱导越成功</li>
          <li>一致性命中 = 指纹库稳定性</li>
          <li>资源损耗比 = 消耗对方资源的效率</li>
        </ul>
      </div>
    </div>
  );
}

function MetricCard({ label, value, subValue, color, icon }: {
  label: string;
  value: string | number;
  subValue: string;
  color: string;
  icon: string;
}) {
  const colorMap: Record<string, string> = {
    purple: 'from-purple-600 to-purple-800',
    blue: 'from-blue-600 to-blue-800',
    green: 'from-green-600 to-green-800',
  };

  return (
    <div className={`bg-gradient-to-br ${colorMap[color]} rounded-lg p-4`}>
      <div className="flex items-center gap-2 mb-2">
        <span className="text-xl">{icon}</span>
        <span className="text-sm text-gray-200">{label}</span>
      </div>
      <div className="text-2xl font-bold">{value}</div>
      <div className="text-xs text-gray-300 mt-1">{subValue}</div>
    </div>
  );
}

function TemplateBar({ template, count, total }: { template: string; count: number; total: number }) {
  const percentage = total > 0 ? (count / total * 100) : 0;
  
  const templateLabels: Record<string, string> = {
    corporate_web: '🏢 公司官网',
    network_error: '⚠️ 网络错误',
    old_admin_portal: '🌀 迷宫后台',
    standard_https: '📄 标准 404',
  };

  const templateColors: Record<string, string> = {
    corporate_web: 'bg-blue-500',
    network_error: 'bg-yellow-500',
    old_admin_portal: 'bg-purple-500',
    standard_https: 'bg-gray-500',
  };

  return (
    <div>
      <div className="flex justify-between text-sm mb-1">
        <span>{templateLabels[template] || template}</span>
        <span className="text-gray-400">{count} ({percentage.toFixed(1)}%)</span>
      </div>
      <div className="h-2 bg-gray-700 rounded overflow-hidden">
        <div
          className={`h-full ${templateColors[template] || 'bg-gray-500'} transition-all duration-500`}
          style={{ width: `${percentage}%` }}
        />
      </div>
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

export default PhantomWarfareView;
