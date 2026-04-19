// 拦截热图
import { useEffect, useState } from 'react'

interface InterceptionData {
  gatewayId: string
  location: string
  lat: number
  lng: number
  successCount: number
  lastInterception: number
}

export default function InterceptionHeatmap() {
  const [data, setData] = useState<InterceptionData[]>([])
  
  useEffect(() => {
    // 模拟数据
    const mockData: InterceptionData[] = [
      { gatewayId: 'gw-iceland-01', location: '冰岛', lat: 64.1, lng: -21.9, successCount: 127, lastInterception: Date.now() - 30000 },
      { gatewayId: 'gw-swiss-01', location: '瑞士', lat: 46.9, lng: 7.4, successCount: 98, lastInterception: Date.now() - 60000 },
      { gatewayId: 'gw-singapore-01', location: '新加坡', lat: 1.3, lng: 103.8, successCount: 156, lastInterception: Date.now() - 15000 },
      { gatewayId: 'gw-panama-01', location: '巴拿马', lat: 9.0, lng: -79.5, successCount: 73, lastInterception: Date.now() - 120000 },
      { gatewayId: 'gw-seychelles-01', location: '塞舌尔', lat: -4.6, lng: 55.5, successCount: 45, lastInterception: Date.now() - 90000 },
    ]
    
    setData(mockData)
    
    // 定期更新
    const interval = setInterval(() => {
      setData(prev => prev.map(item => ({
        ...item,
        successCount: item.successCount + Math.floor(Math.random() * 3),
        lastInterception: Math.random() > 0.7 ? Date.now() : item.lastInterception,
      })))
    }, 5000)
    
    return () => clearInterval(interval)
  }, [])
  
  const getHeatColor = (count: number) => {
    if (count > 150) return 'bg-red-500'
    if (count > 100) return 'bg-orange-500'
    if (count > 50) return 'bg-yellow-500'
    return 'bg-green-500'
  }
  
  const getTimeSince = (timestamp: number) => {
    const seconds = Math.floor((Date.now() - timestamp) / 1000)
    if (seconds < 60) return `${seconds}秒前`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}分钟前`
    return `${Math.floor(seconds / 3600)}小时前`
  }
  
  return (
    <div className="bg-slate-900/80 backdrop-blur-sm p-6 rounded-lg">
      <h3 className="text-xl font-bold text-white mb-4">全球拦截热图</h3>
      
      {/* 热图表格 */}
      <div className="space-y-2">
        {data.map(item => (
          <div key={item.gatewayId} className="bg-slate-800/50 p-4 rounded flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className={`w-3 h-3 rounded-full ${getHeatColor(item.successCount)} animate-pulse`} />
              <div>
                <div className="text-white font-medium">{item.location}</div>
                <div className="text-sm text-gray-400">{item.gatewayId}</div>
              </div>
            </div>
            
            <div className="flex items-center gap-8">
              <div className="text-right">
                <div className="text-2xl font-bold text-white">{item.successCount}</div>
                <div className="text-xs text-gray-400">成功诱骗</div>
              </div>
              
              <div className="text-right">
                <div className="text-sm text-gray-300">{getTimeSince(item.lastInterception)}</div>
                <div className="text-xs text-gray-400">最后拦截</div>
              </div>
            </div>
          </div>
        ))}
      </div>
      
      {/* 总计 */}
      <div className="mt-6 pt-4 border-t border-slate-700">
        <div className="grid grid-cols-3 gap-4">
          <div className="text-center">
            <div className="text-3xl font-bold text-blue-400">
              {data.reduce((sum, item) => sum + item.successCount, 0)}
            </div>
            <div className="text-sm text-gray-400">总拦截次数</div>
          </div>
          <div className="text-center">
            <div className="text-3xl font-bold text-green-400">{data.length}</div>
            <div className="text-sm text-gray-400">活跃节点</div>
          </div>
          <div className="text-center">
            <div className="text-3xl font-bold text-purple-400">
              {(data.reduce((sum, item) => sum + item.successCount, 0) / data.length).toFixed(0)}
            </div>
            <div className="text-sm text-gray-400">平均拦截</div>
          </div>
        </div>
      </div>
    </div>
  )
}
