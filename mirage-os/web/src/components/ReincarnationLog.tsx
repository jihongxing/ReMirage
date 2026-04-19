// 转生记录表
import { useEffect, useState } from 'react'

interface ReincarnationEvent {
  id: string
  timestamp: number
  oldGatewayId: string
  newGatewayId: string
  oldLocation: string
  newLocation: string
  reason: string
  duration: number // 毫秒
  status: 'success' | 'failed'
}

export default function ReincarnationLog() {
  const [events, setEvents] = useState<ReincarnationEvent[]>([])
  
  useEffect(() => {
    // 模拟历史数据
    const mockEvents: ReincarnationEvent[] = [
      {
        id: '1',
        timestamp: Date.now() - 3600000,
        oldGatewayId: 'gw-iceland-01',
        newGatewayId: 'gw-swiss-02',
        oldLocation: '冰岛',
        newLocation: '瑞士',
        reason: '威胁等级过高',
        duration: 1250,
        status: 'success',
      },
      {
        id: '2',
        timestamp: Date.now() - 7200000,
        oldGatewayId: 'gw-panama-01',
        newGatewayId: 'gw-singapore-03',
        oldLocation: '巴拿马',
        newLocation: '新加坡',
        reason: '物理封锁检测',
        duration: 980,
        status: 'success',
      },
      {
        id: '3',
        timestamp: Date.now() - 10800000,
        oldGatewayId: 'gw-swiss-01',
        newGatewayId: 'gw-seychelles-01',
        oldLocation: '瑞士',
        newLocation: '塞舌尔',
        reason: 'DDoS 攻击',
        duration: 1580,
        status: 'success',
      },
    ]
    
    setEvents(mockEvents)
  }, [])
  
  const getReasonColor = (reason: string) => {
    if (reason.includes('物理封锁')) return 'text-red-400'
    if (reason.includes('威胁')) return 'text-orange-400'
    if (reason.includes('DDoS')) return 'text-yellow-400'
    return 'text-blue-400'
  }
  
  const getReasonIcon = (reason: string) => {
    if (reason.includes('物理封锁')) return '🚨'
    if (reason.includes('威胁')) return '⚠️'
    if (reason.includes('DDoS')) return '🛡️'
    return '🔄'
  }
  
  const formatTime = (timestamp: number) => {
    const date = new Date(timestamp)
    return date.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  }
  
  return (
    <div className="bg-slate-900/80 backdrop-blur-sm p-6 rounded-lg">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-xl font-bold text-white">转生记录</h3>
        <div className="text-sm text-gray-400">
          总计: <span className="text-white font-bold">{events.length}</span> 次无感逃逸
        </div>
      </div>
      
      {/* 时间线 */}
      <div className="space-y-4">
        {events.map((event, index) => (
          <div key={event.id} className="relative">
            {/* 连接线 */}
            {index < events.length - 1 && (
              <div className="absolute left-6 top-12 w-0.5 h-full bg-slate-700" />
            )}
            
            <div className="flex gap-4">
              {/* 时间轴点 */}
              <div className="flex flex-col items-center">
                <div className="w-12 h-12 rounded-full bg-slate-800 flex items-center justify-center text-2xl">
                  {getReasonIcon(event.reason)}
                </div>
              </div>
              
              {/* 事件内容 */}
              <div className="flex-1 bg-slate-800/50 p-4 rounded-lg">
                <div className="flex items-start justify-between mb-2">
                  <div>
                    <div className={`font-medium ${getReasonColor(event.reason)}`}>
                      {event.reason}
                    </div>
                    <div className="text-sm text-gray-400">{formatTime(event.timestamp)}</div>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="text-xs text-gray-400">耗时</div>
                    <div className="text-sm font-mono text-green-400">{event.duration}ms</div>
                  </div>
                </div>
                
                <div className="flex items-center gap-4 text-sm">
                  <div className="flex items-center gap-2">
                    <span className="text-gray-400">原节点:</span>
                    <span className="text-white font-mono">{event.oldGatewayId}</span>
                    <span className="text-gray-500">({event.oldLocation})</span>
                  </div>
                  <div className="text-gray-600">→</div>
                  <div className="flex items-center gap-2">
                    <span className="text-gray-400">新节点:</span>
                    <span className="text-white font-mono">{event.newGatewayId}</span>
                    <span className="text-gray-500">({event.newLocation})</span>
                  </div>
                </div>
                
                {event.status === 'success' && (
                  <div className="mt-2 inline-flex items-center gap-1 px-2 py-1 bg-green-500/20 text-green-400 text-xs rounded">
                    ✓ 转生成功
                  </div>
                )}
              </div>
            </div>
          </div>
        ))}
      </div>
      
      {/* 统计 */}
      <div className="mt-6 pt-4 border-t border-slate-700 grid grid-cols-3 gap-4">
        <div className="text-center">
          <div className="text-2xl font-bold text-green-400">100%</div>
          <div className="text-xs text-gray-400">成功率</div>
        </div>
        <div className="text-center">
          <div className="text-2xl font-bold text-blue-400">
            {(events.reduce((sum, e) => sum + e.duration, 0) / events.length).toFixed(0)}ms
          </div>
          <div className="text-xs text-gray-400">平均耗时</div>
        </div>
        <div className="text-center">
          <div className="text-2xl font-bold text-purple-400">
            {events.filter(e => e.reason.includes('物理封锁')).length}
          </div>
          <div className="text-xs text-gray-400">物理逃逸</div>
        </div>
      </div>
    </div>
  )
}
