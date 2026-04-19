// 网关自检详情 - 可视化 monitor.go 的底层判定逻辑
import { useState, useEffect } from 'react';

interface RTTSample {
  timestamp: number;
  value: number;
  isAnomaly: boolean;
}

interface EntropyComparison {
  baseline: number;
  current: number;
  delta: number;
  isContaminated: boolean;
}

interface JA4Status {
  templateName: string;
  templateId: number;
  lastReset: string;
  cooldownRemaining: number;
  fingerprint: string;
}

interface AlertEvent {
  level: 'yellow' | 'red';
  message: string;
  timestamp: string;
  metric: string;
}

export default function HealthDiagnostics() {
  const [rttSamples, setRttSamples] = useState<RTTSample[]>([]);
  const [jitterEntropy, setJitterEntropy] = useState(0);
  const [entropyComparison, setEntropyComparison] = useState<EntropyComparison | null>(null);
  const [ja4Status, setJa4Status] = useState<JA4Status | null>(null);
  const [alerts, setAlerts] = useState<AlertEvent[]>([]);
  const [lossRate, setLossRate] = useState(0);
  const [alertLevel, setAlertLevel] = useState<'none' | 'yellow' | 'red'>('none');

  useEffect(() => {
    // 模拟 RTT 数据流
    const interval = setInterval(() => {
      setRttSamples(prev => {
        const newSample: RTTSample = {
          timestamp: Date.now(),
          value: 50 + Math.random() * 30 + (Math.random() > 0.9 ? 100 : 0),
          isAnomaly: Math.random() > 0.9,
        };
        const updated = [...prev, newSample].slice(-60);
        
        // 计算抖动熵
        if (updated.length > 10) {
          const diffs = updated.slice(1).map((s, i) => s.value - updated[i].value);
          const bins: Record<number, number> = {};
          diffs.forEach(d => {
            const bin = Math.floor(d / 5);
            bins[bin] = (bins[bin] || 0) + 1;
          });
          let entropy = 0;
          const total = diffs.length;
          Object.values(bins).forEach(count => {
            const p = count / total;
            if (p > 0) entropy -= p * Math.log2(p);
          });
          setJitterEntropy(entropy);
        }
        
        return updated;
      });
    }, 500);

    // 模拟其他数据
    setEntropyComparison({
      baseline: 4.2,
      current: 5.8,
      delta: 1.6,
      isContaminated: true,
    });

    setJa4Status({
      templateName: 'Chrome 120 Windows',
      templateId: 3,
      lastReset: '2 小时前',
      cooldownRemaining: 0,
      fingerprint: 't13d1516h2_8daaf6152771_e5627efa2ab1',
    });

    setAlerts([
      { level: 'yellow', message: '丢包率超过 10%', timestamp: '5 分钟前', metric: 'loss_rate' },
      { level: 'red', message: '检测到精准干扰', timestamp: '12 分钟前', metric: 'jitter_entropy' },
    ]);

    setLossRate(8.5);
    setAlertLevel('yellow');

    return () => clearInterval(interval);
  }, []);

  const getEntropyColor = (entropy: number) => {
    if (entropy < 3) return 'text-green-400';
    if (entropy < 5) return 'text-yellow-400';
    return 'text-red-400';
  };

  const getEntropyLabel = (entropy: number) => {
    if (entropy < 3) return '正常';
    if (entropy < 5) return '轻微波动';
    return '异常波动';
  };

  const maxRtt = Math.max(...rttSamples.map(s => s.value), 100);

  return (
    <div className="bg-gray-900 rounded-lg p-6 space-y-6">
      {/* 标题和告警状态 */}
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          🧠 网关自检详情
        </h2>
        <div className={`px-4 py-2 rounded-lg font-medium ${
          alertLevel === 'none' ? 'bg-green-500/20 text-green-400' :
          alertLevel === 'yellow' ? 'bg-yellow-500/20 text-yellow-400' :
          'bg-red-500/20 text-red-400'
        }`}>
          {alertLevel === 'none' ? '✓ 正常' :
           alertLevel === 'yellow' ? '⚠️ Yellow Alert' :
           '🚨 Red Alert'}
        </div>
      </div>

      {/* RTT 方差熵曲线 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-white font-medium flex items-center gap-2">
            📈 RTT 方差熵曲线
          </h3>
          <div className="flex items-center gap-4">
            <div className="text-sm">
              <span className="text-gray-400">抖动熵: </span>
              <span className={`font-bold ${getEntropyColor(jitterEntropy)}`}>
                {jitterEntropy.toFixed(2)} ({getEntropyLabel(jitterEntropy)})
              </span>
            </div>
            <div className="text-sm">
              <span className="text-gray-400">丢包率: </span>
              <span className={`font-bold ${lossRate > 10 ? 'text-red-400' : 'text-green-400'}`}>
                {lossRate.toFixed(1)}%
              </span>
            </div>
          </div>
        </div>

        {/* RTT 图表 */}
        <div className="h-40 flex items-end gap-0.5 bg-gray-900 rounded p-2">
          {rttSamples.map((sample, idx) => (
            <div
              key={idx}
              className={`flex-1 rounded-t transition-all ${
                sample.isAnomaly ? 'bg-red-500' : 'bg-cyan-500'
              }`}
              style={{ height: `${(sample.value / maxRtt) * 100}%` }}
              title={`RTT: ${sample.value.toFixed(1)}ms`}
            />
          ))}
        </div>

        <div className="flex items-center justify-between mt-2 text-xs text-gray-500">
          <span>60 秒前</span>
          <span>现在</span>
        </div>

        {/* 图例 */}
        <div className="flex items-center justify-center gap-6 mt-3 text-xs">
          <div className="flex items-center gap-1">
            <span className="w-3 h-3 bg-cyan-500 rounded" /> 正常拥塞
          </div>
          <div className="flex items-center gap-1">
            <span className="w-3 h-3 bg-red-500 rounded" /> 人为干扰
          </div>
        </div>
      </div>

      {/* 内容熵偏移对比 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          🔬 内容熵偏移对比
        </h3>
        {entropyComparison && (
          <div className="grid grid-cols-3 gap-4">
            <div className="bg-gray-900 rounded-lg p-4 text-center">
              <div className="text-sm text-gray-400 mb-1">基准熵值</div>
              <div className="text-2xl font-bold text-green-400">
                {entropyComparison.baseline.toFixed(2)}
              </div>
            </div>
            <div className="bg-gray-900 rounded-lg p-4 text-center">
              <div className="text-sm text-gray-400 mb-1">当前熵值</div>
              <div className={`text-2xl font-bold ${
                entropyComparison.isContaminated ? 'text-red-400' : 'text-green-400'
              }`}>
                {entropyComparison.current.toFixed(2)}
              </div>
            </div>
            <div className="bg-gray-900 rounded-lg p-4 text-center">
              <div className="text-sm text-gray-400 mb-1">偏移量</div>
              <div className={`text-2xl font-bold ${
                entropyComparison.delta > 1 ? 'text-red-400' : 'text-yellow-400'
              }`}>
                +{entropyComparison.delta.toFixed(2)}
              </div>
            </div>
          </div>
        )}
        
        {entropyComparison?.isContaminated && (
          <div className="mt-4 bg-red-500/20 border border-red-500 rounded-lg p-3 text-sm">
            <span className="text-red-400 font-medium">⚠️ 环境已被污染</span>
            <p className="text-gray-400 mt-1">
              内容熵偏移超过阈值，响应内容可能被注入或篡改
            </p>
          </div>
        )}
      </div>

      {/* JA4/B-DNA 状态 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          🎭 JA4/B-DNA 拟态状态
        </h3>
        {ja4Status && (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="bg-gray-900 rounded-lg p-3">
                <div className="text-sm text-gray-400 mb-1">当前模板</div>
                <div className="text-white font-medium">{ja4Status.templateName}</div>
                <div className="text-xs text-gray-500">ID: {ja4Status.templateId}</div>
              </div>
              <div className="bg-gray-900 rounded-lg p-3">
                <div className="text-sm text-gray-400 mb-1">上次重塑</div>
                <div className="text-white font-medium">{ja4Status.lastReset}</div>
                <div className="text-xs text-gray-500">
                  冷却: {ja4Status.cooldownRemaining > 0 ? `${ja4Status.cooldownRemaining}s` : '就绪'}
                </div>
              </div>
            </div>
            
            <div className="bg-gray-900 rounded-lg p-3">
              <div className="text-sm text-gray-400 mb-1">JA4 指纹</div>
              <code className="text-xs text-cyan-400 font-mono break-all">
                {ja4Status.fingerprint}
              </code>
            </div>

            <button
              disabled={ja4Status.cooldownRemaining > 0}
              className={`w-full py-2 rounded-lg font-medium transition-all ${
                ja4Status.cooldownRemaining > 0
                  ? 'bg-gray-700 text-gray-500 cursor-not-allowed'
                  : 'bg-purple-600 hover:bg-purple-500 text-white'
              }`}
            >
              {ja4Status.cooldownRemaining > 0 
                ? `冷却中 (${ja4Status.cooldownRemaining}s)` 
                : '🔄 强制重塑 B-DNA'}
            </button>
          </div>
        )}
      </div>

      {/* 告警历史 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          🚨 告警历史
        </h3>
        <div className="space-y-2">
          {alerts.map((alert, idx) => (
            <div
              key={idx}
              className={`flex items-center justify-between p-3 rounded-lg ${
                alert.level === 'yellow' 
                  ? 'bg-yellow-500/10 border-l-4 border-yellow-500' 
                  : 'bg-red-500/10 border-l-4 border-red-500'
              }`}
            >
              <div className="flex items-center gap-3">
                <span>{alert.level === 'yellow' ? '⚠️' : '🚨'}</span>
                <div>
                  <div className={`font-medium ${
                    alert.level === 'yellow' ? 'text-yellow-400' : 'text-red-400'
                  }`}>
                    {alert.message}
                  </div>
                  <div className="text-xs text-gray-500">
                    指标: {alert.metric}
                  </div>
                </div>
              </div>
              <span className="text-xs text-gray-500">{alert.timestamp}</span>
            </div>
          ))}
        </div>
      </div>

      {/* 判定逻辑说明 */}
      <div className="bg-gray-800/50 rounded-lg p-4 text-xs text-gray-500">
        <div className="font-medium text-gray-400 mb-2">判定逻辑</div>
        <ul className="space-y-1">
          <li>• Yellow Alert: 丢包率 &gt; 10% 或 RTT 方差熵 &gt; 3.0</li>
          <li>• Red Alert: 丢包率 &gt; 25% 或 RST 注入 &gt; 10 次 或 精准干扰检测</li>
          <li>• 精准干扰: 抖动熵 &gt; 5.0 且 丢包率 &gt; 5% 且 带宽未饱和</li>
        </ul>
      </div>
    </div>
  );
}
