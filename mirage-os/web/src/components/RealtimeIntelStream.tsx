// 实时情报流 - 黑客帝国代码雨风格
import { useEffect, useRef, useState } from 'react'

interface ThreatIntel {
  id: string
  timestamp: number
  srcIp: string
  ja4Fingerprint: string
  threatType: string
  severity: number
  blocked: boolean
}

interface Props {
  threats: ThreatIntel[]
  maxItems?: number
}

export default function RealtimeIntelStream({ threats, maxItems = 20 }: Props) {
  const [displayThreats, setDisplayThreats] = useState<ThreatIntel[]>([])
  const containerRef = useRef<HTMLDivElement>(null)
  
  useEffect(() => {
    setDisplayThreats(prev => {
      const updated = [...threats, ...prev].slice(0, maxItems)
      return updated
    })
  }, [threats, maxItems])
  
  const getThreatColor = (severity: number) => {
    if (severity >= 8) return 'text-red-400'
    if (severity >= 5) return 'text-orange-400'
    return 'text-yellow-400'
  }
  
  const getThreatIcon = (threatType: string) => {
    switch (threatType) {
      case 'ACTIVE_PROBING': return '🔍'
      case 'JA4_SCAN': return '🔐'
      case 'DPI_INSPECTION': return '👁️'
      case 'TIMING_ATTACK': return '⏱️'
      default: return '⚠️'
    }
  }
  
  return (
    <div className="w-full h-full bg-slate-950 rounded-lg overflow-hidden relative">
      {/* 标题栏 */}
      <div className="bg-slate-900 border-b border-slate-800 px-6 py-4">
        <div className="flex items-center justify-between">
          <h3 className="text-xl font-bold text-white">全球威胁情报流</h3>
          <div className="flex items-center gap-2">
            <div className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
            <span className="text-sm text-slate-400">实时监控</span>
          </div>
        </div>
      </div>
      
      {/* 情报流 */}
      <div
        ref={containerRef}
        className="h-[calc(100%-80px)] overflow-y-auto scrollbar-thin scrollbar-thumb-slate-700 scrollbar-track-slate-900"
      >
        {displayThreats.map((threat) => (
          <div
            key={threat.id}
            className="border-b border-slate-800 hover:bg-slate-900/50 transition-colors animate-fadeIn"
          >
            <div className="px-6 py-4">
              <div className="flex items-start gap-4">
                <div className="text-2xl">{getThreatIcon(threat.threatType)}</div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-3 mb-2">
                    <span className={`font-mono text-sm ${getThreatColor(threat.severity)}`}>
                      {threat.srcIp}
                    </span>
                    <span className="text-xs text-slate-500">
                      {new Date(threat.timestamp).toLocaleTimeString()}
                    </span>
                    {threat.blocked && (
                      <span className="px-2 py-0.5 bg-red-500/20 text-red-400 text-xs rounded-full">
                        已封禁
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-4 text-sm">
                    <div className="text-slate-400">
                      类型: <span className="text-white">{threat.threatType}</span>
                    </div>
                    <div className="text-slate-400">
                      严重程度: <span className={getThreatColor(threat.severity)}>{threat.severity}/10</span>
                    </div>
                  </div>
                  {threat.ja4Fingerprint && (
                    <div className="mt-2 font-mono text-xs text-slate-500 truncate">
                      JA4: {threat.ja4Fingerprint}
                    </div>
                  )}
                </div>
                <div className="flex flex-col gap-1">
                  {[...Array(10)].map((_, i) => (
                    <div
                      key={i}
                      className={`w-1 h-1 rounded-full ${
                        i < threat.severity
                          ? threat.severity >= 8
                            ? 'bg-red-500'
                            : threat.severity >= 5
                            ? 'bg-orange-500'
                            : 'bg-yellow-500'
                          : 'bg-slate-700'
                      }`}
                    />
                  ))}
                </div>
              </div>
            </div>
          </div>
        ))}
        
        {displayThreats.length === 0 && (
          <div className="flex items-center justify-center h-full text-slate-500">
            <div className="text-center">
              <div className="text-4xl mb-4">🛡️</div>
              <div>暂无威胁检测</div>
              <div className="text-sm mt-2">系统正在监控全球流量...</div>
            </div>
          </div>
        )}
      </div>
      
      {/* 统计栏 */}
      <div className="absolute bottom-0 left-0 right-0 bg-slate-900/95 backdrop-blur-sm border-t border-slate-800 px-6 py-3">
        <div className="flex items-center justify-between text-sm">
          <div className="text-slate-400">
            总检测: <span className="text-white font-semibold">{displayThreats.length}</span>
          </div>
          <div className="text-slate-400">
            已封禁: <span className="text-red-400 font-semibold">
              {displayThreats.filter(t => t.blocked).length}
            </span>
          </div>
          <div className="text-slate-400">
            高危: <span className="text-orange-400 font-semibold">
              {displayThreats.filter(t => t.severity >= 8).length}
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
