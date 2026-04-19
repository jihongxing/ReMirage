// 威胁分析 - 个人/全局威胁态势
import { useState, useEffect } from 'react';
import { useMirageSocket } from '../hooks/useMirageSocket';

interface ThreatEvent {
  id: string;
  timestamp: number;
  type: string;
  severity: number;
  source: { ip: string; country: string; city: string };
  target: string;
  action: string;
  details: string;
}

interface LabyrinthStats {
  totalDeceived: number;
  avgTimeWasted: number;
  topDecoys: { name: string; hits: number }[];
}

const ThreatAnalysis = () => {
  const { lastThreat } = useMirageSocket();
  const [events, setEvents] = useState<ThreatEvent[]>([]);
  const [labyrinthStats, setLabyrinthStats] = useState<LabyrinthStats | null>(null);
  const [filter, setFilter] = useState<'all' | 'high' | 'medium' | 'low'>('all');

  useEffect(() => {
    // 模拟数据
    setEvents([
      { id: '1', timestamp: Date.now() - 300000, type: 'ACTIVE_PROBING', severity: 8, source: { ip: '45.33.32.156', country: 'US', city: 'Fremont' }, target: 'sg-node-01', action: 'LABYRINTH_REDIRECT', details: '检测到 CID 探测，已重定向至幻影迷宫' },
      { id: '2', timestamp: Date.now() - 600000, type: 'FINGERPRINT_SCAN', severity: 6, source: { ip: '185.220.101.1', country: 'DE', city: 'Frankfurt' }, target: 'ch-node-02', action: 'MIMICRY_SHIFT', details: 'JA4 指纹扫描，已切换至 Teams 拟态' },
      { id: '3', timestamp: Date.now() - 900000, type: 'TIMING_ANALYSIS', severity: 7, source: { ip: '103.152.220.44', country: 'CN', city: 'Beijing' }, target: 'is-node-01', action: 'JITTER_INJECT', details: '检测到 IAT 分析，已注入时域扰动' },
    ]);
    setLabyrinthStats({ totalDeceived: 847, avgTimeWasted: 42.5, topDecoys: [{ name: 'fake_db_01', hits: 234 }, { name: 'honeypot_ssh', hits: 189 }, { name: 'ghost_api', hits: 156 }] });
  }, []);

  useEffect(() => {
    if (lastThreat) {
      const newEvent: ThreatEvent = {
        id: Date.now().toString(),
        timestamp: Date.now(),
        type: lastThreat.threatType,
        severity: lastThreat.intensity,
        source: { ip: lastThreat.srcIp, country: 'Unknown', city: 'Unknown' },
        target: lastThreat.label,
        action: 'DETECTED',
        details: lastThreat.label,
      };
      setEvents(prev => [newEvent, ...prev].slice(0, 50));
    }
  }, [lastThreat]);

  const filteredEvents = events.filter(e => {
    if (filter === 'high') return e.severity >= 7;
    if (filter === 'medium') return e.severity >= 4 && e.severity < 7;
    if (filter === 'low') return e.severity < 4;
    return true;
  });

  const getSeverityColor = (s: number) => s >= 7 ? 'text-red-400' : s >= 4 ? 'text-yellow-400' : 'text-green-400';
  const getSeverityBg = (s: number) => s >= 7 ? 'bg-red-600/20 border-red-600/30' : s >= 4 ? 'bg-yellow-600/20 border-yellow-600/30' : 'bg-green-600/20 border-green-600/30';

  return (
    <div className="space-y-6">
      {/* 幻影迷宫统计 */}
      {labyrinthStats && (
        <div className="bg-gradient-to-r from-purple-600/20 to-pink-600/20 rounded-lg border border-purple-600/30 p-6">
          <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
            <span>🌀</span> 幻影迷宫战果
          </h2>
          <div className="grid grid-cols-3 gap-4">
            <div className="text-center">
              <p className="text-3xl font-bold text-purple-400">{labyrinthStats.totalDeceived}</p>
              <p className="text-xs text-slate-400">成功欺骗</p>
            </div>
            <div className="text-center">
              <p className="text-3xl font-bold text-pink-400">{labyrinthStats.avgTimeWasted}s</p>
              <p className="text-xs text-slate-400">平均浪费时间</p>
            </div>
            <div>
              <p className="text-xs text-slate-400 mb-2">热门诱饵</p>
              {labyrinthStats.topDecoys.slice(0, 3).map((d, i) => (
                <div key={i} className="flex justify-between text-xs">
                  <span className="text-slate-300">{d.name}</span>
                  <span className="text-purple-400">{d.hits}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* 过滤器 */}
      <div className="flex gap-2">
        {(['all', 'high', 'medium', 'low'] as const).map(f => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={`px-4 py-2 rounded text-sm transition-colors ${
              filter === f ? 'bg-cyan-600 text-white' : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
            }`}
          >
            {f === 'all' ? '全部' : f === 'high' ? '高危' : f === 'medium' ? '中危' : '低危'}
          </button>
        ))}
      </div>

      {/* 威胁事件列表 */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
          <span>🛡️</span> 威胁事件
        </h2>

        <div className="space-y-3">
          {filteredEvents.map(event => (
            <div key={event.id} className={`rounded-lg border p-4 ${getSeverityBg(event.severity)}`}>
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-3">
                  <span className={`text-lg font-bold ${getSeverityColor(event.severity)}`}>
                    {event.severity}
                  </span>
                  <span className="text-sm font-medium text-white">{event.type}</span>
                </div>
                <span className="text-xs text-slate-500">
                  {new Date(event.timestamp).toLocaleTimeString()}
                </span>
              </div>
              
              <div className="grid grid-cols-2 gap-4 text-xs mb-2">
                <div>
                  <span className="text-slate-500">来源: </span>
                  <span className="text-slate-300">{event.source.ip} ({event.source.country})</span>
                </div>
                <div>
                  <span className="text-slate-500">目标: </span>
                  <span className="text-slate-300">{event.target}</span>
                </div>
              </div>
              
              <div className="flex items-center gap-2">
                <span className="px-2 py-0.5 bg-cyan-600/30 text-cyan-400 rounded text-xs">
                  {event.action}
                </span>
                <span className="text-xs text-slate-400">{event.details}</span>
              </div>
            </div>
          ))}

          {filteredEvents.length === 0 && (
            <div className="text-center py-8 text-slate-500">
              暂无威胁事件
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default ThreatAnalysis;
