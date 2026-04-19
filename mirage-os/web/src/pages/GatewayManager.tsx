// 网关管理中枢 - 远程操控 eBPF 数据面
import { useState, useEffect } from 'react';
import { useMirageSocket } from '../hooks/useMirageSocket';

interface Gateway {
  id: string;
  name: string;
  region: string;
  lat: number;
  lng: number;
  status: 'online' | 'offline' | 'threat' | 'calibrating';
  cpu: number;
  memory: number;
  ebpfLoad: number;
  activeDNA: string[];
  cidRotationRate: number;
  fecRedundancy: number;
  socialJitter: number;
  lastHeartbeat: number;
}

// 模拟网关数据
const mockGateways: Gateway[] = [
  {
    id: 'gw-is-01', name: 'Iceland Node', region: 'EU-NORTH',
    lat: 64.13, lng: -21.89, status: 'online',
    cpu: 23, memory: 45, ebpfLoad: 12,
    activeDNA: ['YouTube', 'Netflix', 'Zoom'],
    cidRotationRate: 5, fecRedundancy: 20, socialJitter: 50,
    lastHeartbeat: Date.now() - 1000,
  },
  {
    id: 'gw-ch-01', name: 'Switzerland Node', region: 'EU-CENTRAL',
    lat: 46.95, lng: 7.45, status: 'online',
    cpu: 31, memory: 52, ebpfLoad: 18,
    activeDNA: ['Spotify', 'Teams', 'Discord'],
    cidRotationRate: 8, fecRedundancy: 25, socialJitter: 65,
    lastHeartbeat: Date.now() - 2000,
  },
  {
    id: 'gw-sg-01', name: 'Singapore Node', region: 'APAC',
    lat: 1.35, lng: 103.82, status: 'threat',
    cpu: 67, memory: 78, ebpfLoad: 45,
    activeDNA: ['WeChat', 'TikTok', 'LINE'],
    cidRotationRate: 15, fecRedundancy: 40, socialJitter: 80,
    lastHeartbeat: Date.now() - 500,
  },
  {
    id: 'gw-us-01', name: 'US West Node', region: 'NA-WEST',
    lat: 37.77, lng: -122.42, status: 'calibrating',
    cpu: 15, memory: 38, ebpfLoad: 8,
    activeDNA: ['YouTube', 'Twitch', 'Slack'],
    cidRotationRate: 3, fecRedundancy: 15, socialJitter: 40,
    lastHeartbeat: Date.now() - 3000,
  },
];

const REGIONS = ['GLOBAL', 'APAC', 'EU-NORTH', 'EU-CENTRAL', 'NA-WEST', 'NA-EAST', 'MIDDLE-EAST', 'CHINA'];
const DNA_TEMPLATES = ['YouTube', 'Netflix', 'Zoom', 'Spotify', 'Teams', 'Discord', 'WeChat', 'TikTok', 'LINE'];

// 全局战术模式
type TacticalMode = 'normal' | 'sleep' | 'aggressive' | 'stealth';

const TACTICAL_MODES: { id: TacticalMode; label: string; icon: string; desc: string }[] = [
  { id: 'normal', label: '常规', icon: '🟢', desc: '标准拟态配置' },
  { id: 'sleep', label: '休眠', icon: '🌙', desc: '最小化流量特征' },
  { id: 'aggressive', label: '激进', icon: '⚡', desc: '最大化抗检测' },
  { id: 'stealth', label: '隐匿', icon: '👻', desc: '全协议深度伪装' },
];

const GatewayManager = () => {
  const { connected, sendCommand } = useMirageSocket();
  const [gateways, setGateways] = useState<Gateway[]>(mockGateways);
  const [selectedGateway, setSelectedGateway] = useState<Gateway | null>(null);
  const [globalSyncMode, setGlobalSyncMode] = useState(false);
  const [tacticalMode, setTacticalMode] = useState<TacticalMode>('normal');
  const [syncingAll, setSyncingAll] = useState(false);
  const [syncProgress, setSyncProgress] = useState(0);

  // 模拟实时更新
  useEffect(() => {
    const interval = setInterval(() => {
      setGateways(prev => prev.map(gw => ({
        ...gw,
        cpu: Math.max(5, Math.min(95, gw.cpu + (Math.random() - 0.5) * 10)),
        memory: Math.max(20, Math.min(90, gw.memory + (Math.random() - 0.5) * 5)),
        ebpfLoad: Math.max(1, Math.min(50, gw.ebpfLoad + (Math.random() - 0.5) * 8)),
        lastHeartbeat: Date.now(),
      })));
    }, 3000);
    return () => clearInterval(interval);
  }, []);

  // 全局同步推送
  const handleGlobalSync = async () => {
    if (!globalSyncMode) return;
    setSyncingAll(true);
    setSyncProgress(0);
    
    const config = getTacticalConfig(tacticalMode);
    const totalNodes = gateways.length;
    
    for (let i = 0; i < totalNodes; i++) {
      const gw = gateways[i];
      // 推送配置到每个节点
      if (connected) {
        sendCommand('gateway:config', { gatewayId: gw.id, ...config });
      }
      // 更新本地状态
      setGateways(prev => prev.map(g => 
        g.id === gw.id ? { ...g, ...config } : g
      ));
      setSyncProgress(((i + 1) / totalNodes) * 100);
      await new Promise(r => setTimeout(r, 300)); // 模拟延迟
    }
    
    setSyncingAll(false);
  };

  // 获取战术模式配置
  const getTacticalConfig = (mode: TacticalMode) => {
    switch (mode) {
      case 'sleep':
        return { socialJitter: 10, cidRotationRate: 1, fecRedundancy: 10 };
      case 'aggressive':
        return { socialJitter: 90, cidRotationRate: 25, fecRedundancy: 45 };
      case 'stealth':
        return { socialJitter: 70, cidRotationRate: 20, fecRedundancy: 35 };
      default:
        return { socialJitter: 50, cidRotationRate: 5, fecRedundancy: 20 };
    }
  };

  const handleConfigUpdate = (gatewayId: string, field: string, value: number) => {
    setGateways(prev => prev.map(gw => 
      gw.id === gatewayId ? { ...gw, [field]: value } : gw
    ));
    if (selectedGateway?.id === gatewayId) {
      setSelectedGateway(prev => prev ? { ...prev, [field]: value } : null);
    }
  };

  const handleEmergencyFallback = (gatewayId: string) => {
    if (confirm('确认触发紧急熔断？所有 UDP/H3 流量将回退到标准 TLS。')) {
      console.log(`Emergency fallback triggered for ${gatewayId}`);
      // 实际调用 API
    }
  };

  const getStatusColor = (status: Gateway['status']) => {
    switch (status) {
      case 'online': return 'bg-green-500';
      case 'offline': return 'bg-slate-500';
      case 'threat': return 'bg-red-500 animate-pulse';
      case 'calibrating': return 'bg-yellow-500 animate-pulse';
    }
  };

  return (
    <div className="space-y-6">
      {/* Global Tactical Override */}
      <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <span className="text-sm text-slate-400">全局战术模式</span>
              <button
                onClick={() => setGlobalSyncMode(!globalSyncMode)}
                className={`relative w-12 h-6 rounded-full transition-colors ${
                  globalSyncMode ? 'bg-cyan-600' : 'bg-slate-700'
                }`}
              >
                <div className={`absolute top-1 w-4 h-4 rounded-full bg-white transition-transform ${
                  globalSyncMode ? 'left-7' : 'left-1'
                }`} />
              </button>
            </div>
            
            {globalSyncMode && (
              <div className="flex items-center gap-2">
                {TACTICAL_MODES.map(mode => (
                  <button
                    key={mode.id}
                    onClick={() => setTacticalMode(mode.id)}
                    className={`px-3 py-1.5 rounded text-sm transition-colors ${
                      tacticalMode === mode.id
                        ? 'bg-cyan-600 text-white'
                        : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                    }`}
                    title={mode.desc}
                  >
                    {mode.icon} {mode.label}
                  </button>
                ))}
              </div>
            )}
          </div>
          
          <div className="flex items-center gap-4">
            {globalSyncMode && (
              <button
                onClick={handleGlobalSync}
                disabled={syncingAll}
                className={`px-4 py-2 rounded-lg text-sm transition-colors ${
                  syncingAll
                    ? 'bg-slate-700 text-slate-500 cursor-not-allowed'
                    : 'bg-orange-600 hover:bg-orange-500 text-white'
                }`}
              >
                {syncingAll ? `同步中 ${syncProgress.toFixed(0)}%` : '🚀 全球推送'}
              </button>
            )}
            <span className="text-sm text-slate-400">
              在线: <span className="text-green-400 font-bold">{gateways.filter(g => g.status === 'online').length}</span>
            </span>
            <button className="px-4 py-2 bg-cyan-600 hover:bg-cyan-500 text-white rounded-lg text-sm transition-colors">
              + 添加节点
            </button>
          </div>
        </div>
        
        {/* 同步进度条 */}
        {syncingAll && (
          <div className="mt-3">
            <div className="h-1.5 bg-slate-800 rounded-full overflow-hidden">
              <div 
                className="h-full bg-orange-500 transition-all duration-300"
                style={{ width: `${syncProgress}%` }}
              />
            </div>
          </div>
        )}
      </div>

      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">网关管理中枢</h1>
          <p className="text-slate-400 text-sm mt-1">远程操控 eBPF 数据面行为</p>
        </div>
      </div>

      <div className="grid grid-cols-12 gap-6">
        {/* 网关列表 */}
        <div className="col-span-5 space-y-4">
          {gateways.map(gw => (
            <div
              key={gw.id}
              onClick={() => setSelectedGateway(gw)}
              className={`bg-slate-900 rounded-lg border p-4 cursor-pointer transition-all ${
                selectedGateway?.id === gw.id 
                  ? 'border-cyan-500 ring-1 ring-cyan-500/50' 
                  : 'border-slate-800 hover:border-slate-700'
              }`}
            >
              <div className="flex items-start justify-between mb-3">
                <div>
                  <div className="flex items-center gap-2">
                    <div className={`w-2 h-2 rounded-full ${getStatusColor(gw.status)}`} />
                    <h3 className="font-medium text-white">{gw.name}</h3>
                  </div>
                  <p className="text-xs text-slate-500 mt-1">{gw.id} · {gw.region}</p>
                </div>
                <span className={`text-xs px-2 py-1 rounded ${
                  gw.status === 'threat' ? 'bg-red-500/20 text-red-400' :
                  gw.status === 'calibrating' ? 'bg-yellow-500/20 text-yellow-400' :
                  'bg-green-500/20 text-green-400'
                }`}>
                  {gw.status.toUpperCase()}
                </span>
              </div>

              {/* 资源指标 */}
              <div className="grid grid-cols-3 gap-3 mb-3">
                <div>
                  <p className="text-xs text-slate-500">CPU</p>
                  <div className="flex items-center gap-2">
                    <div className="flex-1 h-1.5 bg-slate-800 rounded-full overflow-hidden">
                      <div className={`h-full ${gw.cpu > 80 ? 'bg-red-500' : gw.cpu > 50 ? 'bg-yellow-500' : 'bg-green-500'}`} style={{ width: `${gw.cpu}%` }} />
                    </div>
                    <span className="text-xs text-slate-400">{gw.cpu.toFixed(0)}%</span>
                  </div>
                </div>
                <div>
                  <p className="text-xs text-slate-500">Memory</p>
                  <div className="flex items-center gap-2">
                    <div className="flex-1 h-1.5 bg-slate-800 rounded-full overflow-hidden">
                      <div className="h-full bg-cyan-500" style={{ width: `${gw.memory}%` }} />
                    </div>
                    <span className="text-xs text-slate-400">{gw.memory.toFixed(0)}%</span>
                  </div>
                </div>
                <div>
                  <p className="text-xs text-slate-500">eBPF</p>
                  <div className="flex items-center gap-2">
                    <div className="flex-1 h-1.5 bg-slate-800 rounded-full overflow-hidden">
                      <div className="h-full bg-purple-500" style={{ width: `${gw.ebpfLoad * 2}%` }} />
                    </div>
                    <span className="text-xs text-slate-400">{gw.ebpfLoad.toFixed(0)}%</span>
                  </div>
                </div>
              </div>

              {/* 活跃 DNA */}
              <div className="flex flex-wrap gap-1">
                {gw.activeDNA.map(dna => (
                  <span key={dna} className="text-xs px-2 py-0.5 bg-slate-800 text-slate-400 rounded">
                    {dna}
                  </span>
                ))}
              </div>
            </div>
          ))}
        </div>

        {/* 配置面板 */}
        <div className="col-span-7">
          {selectedGateway ? (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-6">
              <div className="flex items-center justify-between mb-6">
                <div>
                  <h2 className="text-xl font-bold text-white">{selectedGateway.name}</h2>
                  <p className="text-sm text-slate-400">{selectedGateway.id}</p>
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={() => handleEmergencyFallback(selectedGateway.id)}
                    className="px-4 py-2 bg-red-600/20 hover:bg-red-600/30 text-red-400 rounded-lg text-sm transition-colors"
                  >
                    🚨 紧急熔断
                  </button>
                </div>
              </div>

              {/* 区域切换 */}
              <div className="mb-6">
                <label className="block text-sm text-slate-400 mb-2">区域配置 (Regional Profile)</label>
                <div className="flex flex-wrap gap-2">
                  {REGIONS.map(region => (
                    <button
                      key={region}
                      className={`px-3 py-1.5 rounded text-sm transition-colors ${
                        selectedGateway.region === region
                          ? 'bg-cyan-600 text-white'
                          : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                      }`}
                    >
                      {region}
                    </button>
                  ))}
                </div>
              </div>

              {/* B-DNA 模板权重 */}
              <div className="mb-6">
                <label className="block text-sm text-slate-400 mb-2">B-DNA 拟态模板</label>
                <div className="grid grid-cols-3 gap-2">
                  {DNA_TEMPLATES.map(dna => (
                    <button
                      key={dna}
                      className={`px-3 py-2 rounded text-sm transition-colors ${
                        selectedGateway.activeDNA.includes(dna)
                          ? 'bg-purple-600/30 text-purple-400 border border-purple-600/50'
                          : 'bg-slate-800 text-slate-500 hover:bg-slate-700'
                      }`}
                    >
                      {dna}
                    </button>
                  ))}
                </div>
              </div>

              {/* 滑动条控制 */}
              <div className="space-y-6">
                {/* CID 轮换频率 */}
                <div>
                  <div className="flex justify-between mb-2">
                    <label className="text-sm text-slate-400">CID 轮换频率</label>
                    <span className="text-sm text-cyan-400">{selectedGateway.cidRotationRate} 次/分钟</span>
                  </div>
                  <input
                    type="range"
                    min="1"
                    max="30"
                    value={selectedGateway.cidRotationRate}
                    onChange={(e) => handleConfigUpdate(selectedGateway.id, 'cidRotationRate', parseInt(e.target.value))}
                    className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
                  />
                  <div className="flex justify-between text-xs text-slate-600 mt-1">
                    <span>低频 (隐蔽)</span>
                    <span>高频 (抗追踪)</span>
                  </div>
                </div>

                {/* FEC 冗余度 */}
                <div>
                  <div className="flex justify-between mb-2">
                    <label className="text-sm text-slate-400">FEC 冗余度</label>
                    <span className="text-sm text-cyan-400">{selectedGateway.fecRedundancy}%</span>
                  </div>
                  <input
                    type="range"
                    min="5"
                    max="50"
                    value={selectedGateway.fecRedundancy}
                    onChange={(e) => handleConfigUpdate(selectedGateway.id, 'fecRedundancy', parseInt(e.target.value))}
                    className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
                  />
                  <div className="flex justify-between text-xs text-slate-600 mt-1">
                    <span>低冗余 (省流量)</span>
                    <span>高冗余 (抗丢包)</span>
                  </div>
                </div>

                {/* Social Jitter */}
                <div>
                  <div className="flex justify-between mb-2">
                    <label className="text-sm text-slate-400">Social Jitter 强度</label>
                    <span className="text-sm text-cyan-400">{selectedGateway.socialJitter}%</span>
                  </div>
                  <input
                    type="range"
                    min="0"
                    max="100"
                    value={selectedGateway.socialJitter}
                    onChange={(e) => handleConfigUpdate(selectedGateway.id, 'socialJitter', parseInt(e.target.value))}
                    className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
                  />
                  <div className="flex justify-between text-xs text-slate-600 mt-1">
                    <span>关闭</span>
                    <span>最大拟态</span>
                  </div>
                </div>
              </div>

              {/* 应用按钮 */}
              <div className="mt-8 flex gap-4">
                <button className="flex-1 px-4 py-3 bg-cyan-600 hover:bg-cyan-500 text-white rounded-lg transition-colors">
                  应用配置
                </button>
                <button className="px-4 py-3 bg-slate-800 hover:bg-slate-700 text-slate-400 rounded-lg transition-colors">
                  重置
                </button>
              </div>
            </div>
          ) : (
            <div className="bg-slate-900 rounded-lg border border-slate-800 p-12 text-center">
              <div className="text-4xl mb-4">🌐</div>
              <p className="text-slate-400">选择一个网关节点进行配置</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default GatewayManager;
