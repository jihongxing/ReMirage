// ThreatFeed - 威胁事件滚动列表（环形缓冲区，最多 200 条）
import { useReducer, useEffect, useRef } from 'react';

export interface ThreatEntry {
  id: string;
  timestamp: number;
  gateway_id: string;
  threat_type: string;
  source_ip: string;
  severity: number;
  packet_count: number;
}

// 环形缓冲区 reducer：永远只保留最新 200 条
type State = { threats: ThreatEntry[] };
type Action = { type: 'push'; payload: ThreatEntry[] } | { type: 'clear' };

const MAX_ENTRIES = 200;

function threatReducer(state: State, action: Action): State {
  switch (action.type) {
    case 'push':
      return {
        threats: [...action.payload, ...state.threats].slice(0, MAX_ENTRIES),
      };
    case 'clear':
      return { threats: [] };
    default:
      return state;
  }
}

const severityColor = (s: number) => {
  if (s >= 8) return 'text-red-400 bg-red-950/50';
  if (s >= 5) return 'text-orange-400 bg-orange-950/50';
  if (s >= 3) return 'text-yellow-400 bg-yellow-950/50';
  return 'text-slate-400 bg-slate-800/50';
};

const threatTypeLabel: Record<string, string> = {
  ACTIVE_PROBING: 'PROBE',
  REPLAY_ATTACK: 'REPLAY',
  TIMING_ATTACK: 'TIMING',
  DPI_DETECTION: 'DPI',
  JA4_SCAN: 'JA4',
  SNI_PROBE: 'SNI',
};

export const ThreatFeed = ({ events }: { events: ThreatEntry[] }) => {
  const [state, dispatch] = useReducer(threatReducer, { threats: [] });
  const listRef = useRef<HTMLDivElement>(null);

  // 新事件进入时推入环形缓冲区
  useEffect(() => {
    if (events.length > 0) {
      dispatch({ type: 'push', payload: events });
    }
  }, [events]);

  return (
    <div className="bg-slate-900 rounded-lg border border-slate-800 flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-slate-800">
        <div className="flex items-center gap-2">
          <div className="w-2 h-2 rounded-full bg-red-400 animate-pulse" />
          <h3 className="text-sm font-medium text-slate-300">威胁雷达</h3>
        </div>
        <span className="text-xs text-slate-500">{state.threats.length} events</span>
      </div>

      {/* 滚动列表 */}
      <div ref={listRef} className="flex-1 overflow-y-auto px-2 py-2 space-y-1">
        {state.threats.length === 0 ? (
          <div className="flex items-center justify-center h-full text-slate-600 text-sm">
            暂无威胁事件
          </div>
        ) : (
          state.threats.map((t) => (
            <div
              key={t.id}
              className="flex items-center gap-2 px-2 py-1.5 rounded text-xs hover:bg-slate-800/50 transition-colors"
            >
              {/* 严重度 */}
              <span className={`px-1.5 py-0.5 rounded font-mono font-bold ${severityColor(t.severity)}`}>
                {t.severity}
              </span>
              {/* 类型 */}
              <span className="text-slate-400 font-mono w-14 truncate">
                {threatTypeLabel[t.threat_type] || t.threat_type}
              </span>
              {/* 源 IP */}
              <span className="text-slate-500 font-mono flex-1 truncate">
                {t.source_ip}
              </span>
              {/* 包数 */}
              {t.packet_count > 1 && (
                <span className="text-slate-600">×{t.packet_count}</span>
              )}
              {/* 时间 */}
              <span className="text-slate-700 w-12 text-right">
                {new Date(t.timestamp * 1000).toLocaleTimeString('en', { hour12: false }).slice(3)}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
};

export default ThreatFeed;
