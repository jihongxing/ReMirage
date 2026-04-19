import { useState, useEffect, useCallback } from 'react';

interface TrapRecord {
  srcIP: string;
  country: string;
  firstSeen: string;
  lastSeen: string;
  requestCount: number;
  honeypotId: number;
  status: 'active' | 'exhausted' | 'escaped';
}

interface DeceptionStats {
  totalRedirected: number;
  activeTraps: number;
  requestsConsumed: number;
  canaryTriggered: number;
}

interface DeceptionInsightsProps {
  wsUrl?: string;
}

export function DeceptionInsights({ wsUrl = 'ws://localhost:8080/ws' }: DeceptionInsightsProps) {
  const [trapRecords, setTrapRecords] = useState<TrapRecord[]>([]);
  const [stats, setStats] = useState<DeceptionStats>({
    totalRedirected: 0,
    activeTraps: 0,
    requestsConsumed: 0,
    canaryTriggered: 0,
  });
  const [logs, setLogs] = useState<string[]>([]);

  // WebSocket 连接
  useEffect(() => {
    const ws = new WebSocket(wsUrl);

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'phantom_event') {
          handlePhantomEvent(data.payload);
        } else if (data.type === 'phantom_stats') {
          setStats(data.payload);
        }
      } catch (e) {
        console.error('Failed to parse phantom event:', e);
      }
    };

    return () => ws.close();
  }, [wsUrl]);

  const handlePhantomEvent = useCallback((event: any) => {
    // 更新陷阱记录
    setTrapRecords((prev) => {
      const existing = prev.find((r) => r.srcIP === event.srcIP);
      if (existing) {
        return prev.map((r) =>
          r.srcIP === event.srcIP
            ? { ...r, lastSeen: new Date().toISOString(), requestCount: r.requestCount + 1 }
            : r
        );
      }
      return [
        ...prev,
        {
          srcIP: event.srcIP,
          country: event.country || 'Unknown',
          firstSeen: new Date().toISOString(),
          lastSeen: new Date().toISOString(),
          requestCount: 1,
          honeypotId: event.honeypotId || 1,
          status: 'active' as const,
        },
      ].slice(-50); // 保留最近 50 条
    });

    // 添加日志
    const logMsg = `发现来自 ${event.country || '未知'} 的扫描器，已成功引导至影子服务器 #${String(event.honeypotId || 1).padStart(2, '0')}，当前已消耗其 ${event.requestCount || 1} 次请求尝试`;
    setLogs((prev) => [logMsg, ...prev].slice(0, 100));
  }, []);

  return (
    <div className="bg-gray-900 rounded-lg p-4 text-white">
      <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
        <span className="w-2 h-2 bg-purple-500 rounded-full animate-pulse" />
        欺骗态势 (Deception Insights)
      </h2>

      {/* 统计卡片 */}
      <div className="grid grid-cols-4 gap-3 mb-4">
        <StatCard label="重定向总数" value={stats.totalRedirected} color="purple" />
        <StatCard label="活跃陷阱" value={stats.activeTraps} color="pink" />
        <StatCard label="消耗请求" value={stats.requestsConsumed} color="indigo" />
        <StatCard label="金丝雀触发" value={stats.canaryTriggered} color="red" />
      </div>

      {/* 幽灵点位表格 */}
      <div className="mb-4">
        <h3 className="text-sm text-gray-400 mb-2">幽灵点位 (Ghost Targets)</h3>
        <div className="bg-gray-800 rounded overflow-hidden max-h-48 overflow-y-auto">
          <table className="w-full text-sm">
            <thead className="bg-gray-700 sticky top-0">
              <tr>
                <th className="px-3 py-2 text-left">来源 IP</th>
                <th className="px-3 py-2 text-left">国家</th>
                <th className="px-3 py-2 text-right">请求数</th>
                <th className="px-3 py-2 text-center">蜜罐</th>
                <th className="px-3 py-2 text-center">状态</th>
              </tr>
            </thead>
            <tbody>
              {trapRecords.map((record, idx) => (
                <tr key={record.srcIP} className={idx % 2 === 0 ? 'bg-gray-800' : 'bg-gray-750'}>
                  <td className="px-3 py-2 font-mono text-purple-400">{record.srcIP}</td>
                  <td className="px-3 py-2">{record.country}</td>
                  <td className="px-3 py-2 text-right text-yellow-400">{record.requestCount}</td>
                  <td className="px-3 py-2 text-center">#{String(record.honeypotId).padStart(2, '0')}</td>
                  <td className="px-3 py-2 text-center">
                    <StatusBadge status={record.status} />
                  </td>
                </tr>
              ))}
              {trapRecords.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-3 py-4 text-center text-gray-500">
                    暂无活跃陷阱
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* 捕获日志 */}
      <div>
        <h3 className="text-sm text-gray-400 mb-2">捕获日志 (Capture Log)</h3>
        <div className="bg-gray-800 rounded p-3 h-32 overflow-y-auto font-mono text-xs">
          {logs.map((log, idx) => (
            <div key={idx} className="text-green-400 mb-1">
              <span className="text-gray-500">[{new Date().toLocaleTimeString()}]</span> {log}
            </div>
          ))}
          {logs.length === 0 && <div className="text-gray-500">等待捕获事件...</div>}
        </div>
      </div>
    </div>
  );
}

function StatCard({ label, value, color }: { label: string; value: number; color: string }) {
  const colorMap: Record<string, string> = {
    purple: 'from-purple-600 to-purple-800',
    pink: 'from-pink-600 to-pink-800',
    indigo: 'from-indigo-600 to-indigo-800',
    red: 'from-red-600 to-red-800',
  };

  return (
    <div className={`bg-gradient-to-br ${colorMap[color]} rounded-lg p-3`}>
      <div className="text-2xl font-bold">{value.toLocaleString()}</div>
      <div className="text-xs text-gray-300">{label}</div>
    </div>
  );
}

function StatusBadge({ status }: { status: 'active' | 'exhausted' | 'escaped' }) {
  const styles: Record<string, string> = {
    active: 'bg-green-500/20 text-green-400',
    exhausted: 'bg-yellow-500/20 text-yellow-400',
    escaped: 'bg-red-500/20 text-red-400',
  };

  const labels: Record<string, string> = {
    active: '活跃',
    exhausted: '耗尽',
    escaped: '逃逸',
  };

  return (
    <span className={`px-2 py-0.5 rounded text-xs ${styles[status]}`}>{labels[status]}</span>
  );
}

export default DeceptionInsights;
