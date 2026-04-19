// 配额消耗率图表 - 业务流量 vs 防御流量
import { useMemo } from 'react'
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts'

interface TrafficData {
  timestamp: number
  businessBytes: number
  defenseBytes: number
}

interface Props {
  data: TrafficData[]
  timeRange: '1h' | '24h' | '7d'
}

export default function QuotaBurnRateChart({ data, timeRange: _timeRange }: Props) {
  const chartData = useMemo(() => {
    return data.map(d => ({
      time: new Date(d.timestamp).toLocaleTimeString(),
      business: (d.businessBytes / (1024 * 1024)).toFixed(2), // MB
      defense: (d.defenseBytes / (1024 * 1024)).toFixed(2),   // MB
      total: ((d.businessBytes + d.defenseBytes) / (1024 * 1024)).toFixed(2),
    }))
  }, [data])
  
  const stats = useMemo(() => {
    const totalBusiness = data.reduce((sum, d) => sum + d.businessBytes, 0)
    const totalDefense = data.reduce((sum, d) => sum + d.defenseBytes, 0)
    const total = totalBusiness + totalDefense
    
    return {
      totalBusiness: (totalBusiness / (1024 * 1024 * 1024)).toFixed(2), // GB
      totalDefense: (totalDefense / (1024 * 1024 * 1024)).toFixed(2),
      total: (total / (1024 * 1024 * 1024)).toFixed(2),
      defenseRatio: total > 0 ? ((totalDefense / total) * 100).toFixed(1) : '0',
    }
  }, [data])
  
  return (
    <div className="w-full h-full bg-slate-900 rounded-lg p-6">
      <div className="mb-6">
        <h3 className="text-xl font-bold text-white mb-4">流量消耗分析</h3>
        
        {/* 统计卡片 */}
        <div className="grid grid-cols-4 gap-4 mb-6">
          <div className="bg-slate-800 rounded-lg p-4">
            <div className="text-slate-400 text-sm mb-1">业务流量</div>
            <div className="text-2xl font-bold text-blue-400">{stats.totalBusiness} GB</div>
          </div>
          <div className="bg-slate-800 rounded-lg p-4">
            <div className="text-slate-400 text-sm mb-1">防御流量</div>
            <div className="text-2xl font-bold text-orange-400">{stats.totalDefense} GB</div>
          </div>
          <div className="bg-slate-800 rounded-lg p-4">
            <div className="text-slate-400 text-sm mb-1">总流量</div>
            <div className="text-2xl font-bold text-white">{stats.total} GB</div>
          </div>
          <div className="bg-slate-800 rounded-lg p-4">
            <div className="text-slate-400 text-sm mb-1">防御占比</div>
            <div className="text-2xl font-bold text-purple-400">{stats.defenseRatio}%</div>
          </div>
        </div>
      </div>
      
      {/* 图表 */}
      <ResponsiveContainer width="100%" height={300}>
        <AreaChart data={chartData}>
          <defs>
            <linearGradient id="colorBusiness" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.8}/>
              <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
            </linearGradient>
            <linearGradient id="colorDefense" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#f97316" stopOpacity={0.8}/>
              <stop offset="95%" stopColor="#f97316" stopOpacity={0}/>
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" stroke="#334155" />
          <XAxis dataKey="time" stroke="#94a3b8" />
          <YAxis stroke="#94a3b8" label={{ value: 'MB', angle: -90, position: 'insideLeft', fill: '#94a3b8' }} />
          <Tooltip
            contentStyle={{
              backgroundColor: '#1e293b',
              border: '1px solid #334155',
              borderRadius: '8px',
              color: '#fff',
            }}
          />
          <Legend />
          <Area
            type="monotone"
            dataKey="business"
            name="业务流量"
            stroke="#3b82f6"
            fillOpacity={1}
            fill="url(#colorBusiness)"
          />
          <Area
            type="monotone"
            dataKey="defense"
            name="防御流量"
            stroke="#f97316"
            fillOpacity={1}
            fill="url(#colorDefense)"
          />
        </AreaChart>
      </ResponsiveContainer>
      
      {/* 成本透明化说明 */}
      <div className="mt-6 p-4 bg-slate-800/50 rounded-lg border border-slate-700">
        <div className="flex items-start gap-3">
          <div className="text-yellow-400 mt-1">💡</div>
          <div className="text-sm text-slate-300">
            <div className="font-semibold mb-1">拟态成本透明化</div>
            <div>业务流量 $0.10/GB · 防御流量 $0.05/GB · 实时计费 · 无隐藏费用</div>
          </div>
        </div>
      </div>
    </div>
  )
}
