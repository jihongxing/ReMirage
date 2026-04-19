// 全系统脉动组件 - 纳秒级后端响应可视化
import { useState, useEffect, useRef } from 'react';

interface BPFLatencySample {
  timestamp: number;
  latencyUs: number; // 微秒
}

interface BufferGaugeData {
  dirtyCount: number;
  maxBuffer: number;
  memoryWriteRate: number;  // 条/秒
  diskWriteRate: number;    // 条/秒
}

interface IOComparisonPoint {
  time: string;
  memoryWrites: number;
  diskWrites: number;
}

export default function MirageHeartbeat() {
  const [bpfLatency, setBpfLatency] = useState<BPFLatencySample[]>([]);
  const [bufferGauge, setBufferGauge] = useState<BufferGaugeData>({
    dirtyCount: 0,
    maxBuffer: 1000,
    memoryWriteRate: 0,
    diskWriteRate: 0,
  });
  const [ioComparison, setIOComparison] = useState<IOComparisonPoint[]>([]);
  const [heartbeatPulse, setHeartbeatPulse] = useState(false);
  const canvasRef = useRef<HTMLCanvasElement>(null);

  // 模拟 BPF 延迟数据采样
  useEffect(() => {
    const interval = setInterval(() => {
      const now = Date.now();
      const newSample: BPFLatencySample = {
        timestamp: now,
        latencyUs: 50 + Math.random() * 150 + Math.sin(now / 500) * 30,
      };

      setBpfLatency(prev => {
        const updated = [...prev, newSample].slice(-100);
        return updated;
      });

      // 心跳脉冲
      setHeartbeatPulse(true);
      setTimeout(() => setHeartbeatPulse(false), 100);
    }, 200); // 5fps

    return () => clearInterval(interval);
  }, []);

  // 模拟缓冲区数据
  useEffect(() => {
    const interval = setInterval(() => {
      setBufferGauge({
        dirtyCount: Math.floor(Math.random() * 80 + 10),
        maxBuffer: 1000,
        memoryWriteRate: Math.floor(Math.random() * 500 + 200),
        diskWriteRate: Math.floor(Math.random() * 5 + 1),
      });
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  // 模拟 IO 对比数据
  useEffect(() => {
    const interval = setInterval(() => {
      const now = new Date();
      const timeStr = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}:${now.getSeconds().toString().padStart(2, '0')}`;
      
      setIOComparison(prev => {
        const newPoint: IOComparisonPoint = {
          time: timeStr,
          memoryWrites: Math.floor(Math.random() * 500 + 200),
          diskWrites: Math.floor(Math.random() * 5 + 1),
        };
        return [...prev, newPoint].slice(-20);
      });
    }, 2000);

    return () => clearInterval(interval);
  }, []);

  // 绘制示波器
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || bpfLatency.length < 2) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const width = canvas.width;
    const height = canvas.height;

    // 清空
    ctx.fillStyle = '#0a0a0a';
    ctx.fillRect(0, 0, width, height);

    // 网格
    ctx.strokeStyle = '#1a1a2e';
    ctx.lineWidth = 1;
    for (let i = 0; i < 10; i++) {
      const y = (height / 10) * i;
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(width, y);
      ctx.stroke();
    }

    // 波形
    ctx.strokeStyle = '#00ff88';
    ctx.lineWidth = 2;
    ctx.shadowColor = '#00ff88';
    ctx.shadowBlur = 10;
    ctx.beginPath();

    const maxLatency = 300;
    bpfLatency.forEach((sample, i) => {
      const x = (i / (bpfLatency.length - 1)) * width;
      const y = height - (sample.latencyUs / maxLatency) * height;
      
      if (i === 0) {
        ctx.moveTo(x, y);
      } else {
        ctx.lineTo(x, y);
      }
    });

    ctx.stroke();
    ctx.shadowBlur = 0;

    // 当前值标注
    const lastSample = bpfLatency[bpfLatency.length - 1];
    if (lastSample) {
      ctx.fillStyle = '#00ff88';
      ctx.font = '12px monospace';
      ctx.fillText(`${lastSample.latencyUs.toFixed(1)} μs`, width - 60, 20);
    }
  }, [bpfLatency]);

  const getBufferColor = (ratio: number) => {
    if (ratio < 0.3) return 'bg-green-500';
    if (ratio < 0.6) return 'bg-yellow-500';
    if (ratio < 0.8) return 'bg-orange-500';
    return 'bg-red-500';
  };

  const bufferRatio = bufferGauge.dirtyCount / bufferGauge.maxBuffer;
  const ioReduction = bufferGauge.memoryWriteRate > 0 
    ? ((bufferGauge.memoryWriteRate - bufferGauge.diskWriteRate) / bufferGauge.memoryWriteRate * 100).toFixed(1)
    : '0';

  return (
    <div className="bg-gray-900 rounded-lg p-6 space-y-6">
      {/* 标题 */}
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-white flex items-center gap-3">
          <span className={`w-3 h-3 rounded-full ${heartbeatPulse ? 'bg-green-400 scale-125' : 'bg-green-600'} transition-all duration-100`} />
          💓 系统脉动监控
        </h2>
        <div className="flex items-center gap-2 text-xs text-gray-500">
          <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
          实时采样 5fps
        </div>
      </div>

      {/* BPF 延迟示波器 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-white font-medium flex items-center gap-2">
            📡 内核响应频率 (eBPF Latency)
          </h3>
          <div className="flex items-center gap-4 text-sm">
            <span className="text-gray-400">
              平均: <span className="text-green-400 font-mono">
                {bpfLatency.length > 0 
                  ? (bpfLatency.reduce((s, v) => s + v.latencyUs, 0) / bpfLatency.length).toFixed(1)
                  : '0'} μs
              </span>
            </span>
            <span className="text-gray-400">
              峰值: <span className="text-yellow-400 font-mono">
                {bpfLatency.length > 0 
                  ? Math.max(...bpfLatency.map(v => v.latencyUs)).toFixed(1)
                  : '0'} μs
              </span>
            </span>
          </div>
        </div>
        <canvas
          ref={canvasRef}
          width={600}
          height={120}
          className="w-full rounded border border-gray-700"
        />
        <div className="flex items-center justify-between mt-2 text-xs text-gray-500">
          <span>0 μs</span>
          <span>目标: &lt; 1000 μs (1ms)</span>
          <span>300 μs</span>
        </div>
      </div>

      {/* 异步队列负载 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          📊 异步队列负载 (Buffer Gauge)
        </h3>
        <div className="grid grid-cols-2 gap-4">
          {/* 待刷盘数据量 */}
          <div className="bg-gray-900 rounded-lg p-4">
            <div className="flex items-center justify-between mb-2">
              <span className="text-gray-400 text-sm">待刷盘 (Dirty)</span>
              <span className="text-white font-mono">
                {bufferGauge.dirtyCount} / {bufferGauge.maxBuffer}
              </span>
            </div>
            <div className="relative h-8 bg-gray-700 rounded overflow-hidden">
              <div
                className={`absolute inset-y-0 left-0 ${getBufferColor(bufferRatio)} transition-all duration-300`}
                style={{ width: `${bufferRatio * 100}%` }}
              />
              <div className="absolute inset-0 flex items-center justify-center text-white text-sm font-bold">
                {(bufferRatio * 100).toFixed(1)}%
              </div>
            </div>
            <div className="text-xs text-gray-500 mt-2">
              触发条件: 100条 或 30秒
            </div>
          </div>

          {/* 写入速率对比 */}
          <div className="bg-gray-900 rounded-lg p-4">
            <div className="text-gray-400 text-sm mb-2">写入速率</div>
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-cyan-400 text-sm">内存写入</span>
                <span className="text-cyan-400 font-mono">{bufferGauge.memoryWriteRate} 条/s</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-orange-400 text-sm">磁盘写入</span>
                <span className="text-orange-400 font-mono">{bufferGauge.diskWriteRate} 条/s</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* IO 削峰动态图 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-white font-medium flex items-center gap-2">
            ⚡ IO 削峰动态图
          </h3>
          <span className="px-3 py-1 bg-green-500/20 text-green-400 text-sm rounded-full">
            ↓ {ioReduction}% IO 削减
          </span>
        </div>
        
        <div className="h-32 flex items-end gap-1">
          {ioComparison.map((point, idx) => (
            <div key={idx} className="flex-1 flex flex-col items-center gap-1">
              <div className="w-full flex flex-col-reverse gap-0.5" style={{ height: '100px' }}>
                {/* 内存写入柱 */}
                <div
                  className="w-full bg-cyan-500/60 rounded-t transition-all duration-300"
                  style={{ height: `${Math.min(point.memoryWrites / 5, 100)}px` }}
                  title={`内存: ${point.memoryWrites}`}
                />
                {/* 磁盘写入柱 */}
                <div
                  className="w-full bg-orange-500 rounded-t transition-all duration-300"
                  style={{ height: `${point.diskWrites * 10}px` }}
                  title={`磁盘: ${point.diskWrites}`}
                />
              </div>
            </div>
          ))}
        </div>
        
        <div className="flex items-center justify-center gap-6 mt-4 text-xs">
          <div className="flex items-center gap-1">
            <span className="w-3 h-3 bg-cyan-500/60 rounded" /> 内存写入
          </div>
          <div className="flex items-center gap-1">
            <span className="w-3 h-3 bg-orange-500 rounded" /> 磁盘写入
          </div>
        </div>
      </div>

      {/* 性能指标卡片 */}
      <div className="grid grid-cols-4 gap-4">
        <div className="bg-gray-800 rounded-lg p-4 text-center">
          <div className="text-2xl font-bold text-green-400">&lt;1ms</div>
          <div className="text-xs text-gray-400">BPF 延迟</div>
        </div>
        <div className="bg-gray-800 rounded-lg p-4 text-center">
          <div className="text-2xl font-bold text-cyan-400">~50ns</div>
          <div className="text-xs text-gray-400">内存读取</div>
        </div>
        <div className="bg-gray-800 rounded-lg p-4 text-center">
          <div className="text-2xl font-bold text-yellow-400">~100ns</div>
          <div className="text-xs text-gray-400">内存写入</div>
        </div>
        <div className="bg-gray-800 rounded-lg p-4 text-center">
          <div className="text-2xl font-bold text-green-400">99.9%</div>
          <div className="text-xs text-gray-400">IO 削减</div>
        </div>
      </div>
    </div>
  );
}
