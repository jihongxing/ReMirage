// 态势监控仪表盘 - 整合现有组件
import { useState, useEffect, useCallback, Suspense } from 'react';
import { ErrorBoundary } from 'react-error-boundary';
import RealtimeIntelStream from '../components/RealtimeIntelStream';
import QuotaBurnRateChart from '../components/QuotaBurnRateChart';
import DeceptionInsights from '../components/DeceptionInsights';
import { useMirageSocket } from '../hooks/useMirageSocket';
import { useGhostMode } from '../components/AdminLayout';

// 懒加载 3D 组件（可能导致问题）
import { lazy } from 'react';
const GlobalTrafficGlobe = lazy(() => import('../components/GlobalTrafficGlobe'));

// 3D 组件加载失败时的降级组件
const GlobeFallback = () => (
  <div className="w-full h-full bg-slate-900 rounded-lg border border-slate-800 flex items-center justify-center">
    <div className="text-center">
      <div className="text-4xl mb-4">🌐</div>
      <p className="text-slate-400">3D 地球加载中...</p>
    </div>
  </div>
);

const GlobeError = () => (
  <div className="w-full h-full bg-slate-900 rounded-lg border border-slate-800 flex items-center justify-center">
    <div className="text-center">
      <div className="text-4xl mb-4">⚠️</div>
      <p className="text-slate-400">3D 组件加载失败</p>
      <p className="text-slate-500 text-sm mt-2">WebGL 可能不受支持</p>
    </div>
  </div>
);

// 高危坐标类型
interface HighlightCoord {
  lat: number;
  lng: number;
  timestamp: number;
}

const mockGateways = [
  { id: 'gw-us-west-01', lat: 34.05, lng: -118.24, status: 'online' as const, threatLevel: 0 },
  { id: 'gw-hk-01', lat: 22.32, lng: 114.17, status: 'online' as const, threatLevel: 2 },
  { id: 'gw-sg-01', lat: 1.35, lng: 103.82, status: 'threat' as const, threatLevel: 5 },
  { id: 'gw-eu-west-01', lat: 51.51, lng: -0.13, status: 'online' as const, threatLevel: 1 },
  { id: 'gw-is-01', lat: 64.13, lng: -21.89, status: 'online' as const, threatLevel: 0 },
];

// IP 转模拟坐标
const ipToCoords = (ip: string): { lat: number; lng: number } => {
  const parts = ip.split('.').map(Number);
  return {
    lat: ((parts[0] + parts[1]) % 180) - 90,
    lng: ((parts[2] + parts[3]) % 360) - 180,
  };
};

const MonitorDashboard = () => {
  const { connected, lastThreat, lastTraffic } = useMirageSocket();
  const { ghostMode } = useGhostMode();
  const [threats, setThreats] = useState<any[]>([]);
  const [trafficData, setTrafficData] = useState<any[]>([]);
  const [intelStream, setIntelStream] = useState<any[]>([]);
  const [highlightCoords, setHighlightCoords] = useState<HighlightCoord[]>([]);

  // Ghost Mode: 5分钟自动清理
  useEffect(() => {
    if (!ghostMode) return;
    
    const cleanupInterval = setInterval(() => {
      const fiveMinAgo = Date.now() - 5 * 60 * 1000;
      setIntelStream(prev => prev.filter(i => i.timestamp > fiveMinAgo));
      setThreats(prev => prev.filter(t => (t.timestamp || Date.now()) > fiveMinAgo));
      setTrafficData(prev => prev.filter(t => t.timestamp > fiveMinAgo));
      setHighlightCoords(prev => prev.filter(c => c.timestamp > fiveMinAgo));
    }, 30000); // 每30秒清理
    
    return () => clearInterval(cleanupInterval);
  }, [ghostMode]);

  // 高危情报触发地图高亮
  const triggerMapHighlight = useCallback((intel: any) => {
    if (intel.severity >= 7) {
      const coords = ipToCoords(intel.srcIp);
      setHighlightCoords(prev => [...prev, { ...coords, timestamp: Date.now() }]);
      // 5秒后移除高亮
      setTimeout(() => {
        setHighlightCoords(prev => prev.filter(c => c.timestamp !== intel.timestamp));
      }, 5000);
    }
  }, []);

  // 处理实时威胁
  useEffect(() => {
    if (lastThreat) {
      setThreats(prev => [...prev, lastThreat]);
      const newIntel = {
        id: `threat-${Date.now()}`,
        timestamp: Date.now(),
        srcIp: lastThreat.srcIp,
        ja4Fingerprint: Math.random().toString(36).substring(2, 15),
        threatType: lastThreat.threatType,
        severity: lastThreat.intensity,
        blocked: lastThreat.intensity >= 7,
      };
      setIntelStream(prev => [newIntel, ...prev.slice(0, 99)]);
      triggerMapHighlight(newIntel);
    }
  }, [lastThreat, triggerMapHighlight]);

  // 处理实时流量
  useEffect(() => {
    if (lastTraffic) {
      const newTraffic = {
        timestamp: Date.now(),
        businessBytes: lastTraffic.businessBytes,
        defenseBytes: lastTraffic.defenseBytes,
      };
      setTrafficData(prev => [...prev.slice(-20), newTraffic]);
    }
  }, [lastTraffic]);

  // 模拟数据
  useEffect(() => {
    if (connected) return;
    
    const interval = setInterval(() => {
      if (Math.random() > 0.7) {
        const newIntel = {
          id: `threat-${Date.now()}`,
          timestamp: Date.now(),
          srcIp: `${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}`,
          ja4Fingerprint: Math.random().toString(36).substring(2, 15),
          threatType: ['ACTIVE_PROBING', 'JA4_SCAN', 'DPI_INSPECTION', 'TIMING_ATTACK'][Math.floor(Math.random() * 4)],
          severity: Math.floor(Math.random() * 10) + 1,
          blocked: Math.random() > 0.3,
        };
        setIntelStream(prev => [newIntel, ...prev.slice(0, 99)]);
        setThreats(prev => [...prev, { severity: newIntel.severity, timestamp: Date.now() }]);
        triggerMapHighlight(newIntel);
      }
      
      const newTraffic = {
        timestamp: Date.now(),
        businessBytes: Math.random() * 100 * 1024 * 1024,
        defenseBytes: Math.random() * 50 * 1024 * 1024,
      };
      setTrafficData(prev => [...prev.slice(-20), newTraffic]);
    }, 3000);
    
    return () => clearInterval(interval);
  }, [connected, triggerMapHighlight]);

  return (
    <div className="space-y-6">
      {/* 顶部统计 */}
      <div className="grid grid-cols-5 gap-4">
        <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
          <p className="text-xs text-slate-500">在线节点</p>
          <p className="text-2xl font-bold text-green-400">
            {mockGateways.filter(g => g.status === 'online').length}
          </p>
        </div>
        <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
          <p className="text-xs text-slate-500">威胁检测</p>
          <p className="text-2xl font-bold text-red-400">{threats.length}</p>
        </div>
        <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
          <p className="text-xs text-slate-500">拦截率</p>
          <p className="text-2xl font-bold text-cyan-400">
            {intelStream.length > 0 
              ? ((intelStream.filter(i => i.blocked).length / intelStream.length) * 100).toFixed(1)
              : 0}%
          </p>
        </div>
        <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
          <p className="text-xs text-slate-500">WebSocket</p>
          <p className={`text-2xl font-bold ${connected ? 'text-green-400' : 'text-red-400'}`}>
            {connected ? 'LIVE' : 'OFFLINE'}
          </p>
        </div>
        <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
          <p className="text-xs text-slate-500">系统状态</p>
          <p className="text-2xl font-bold text-green-400">NOMINAL</p>
        </div>
      </div>

      {/* 主视图 */}
      <div className="grid grid-cols-12 gap-6">
        <div className="col-span-7 h-[500px]">
          <ErrorBoundary fallback={<GlobeError />}>
            <Suspense fallback={<GlobeFallback />}>
              <GlobalTrafficGlobe gateways={mockGateways} threats={threats} highlightCoords={highlightCoords} />
            </Suspense>
          </ErrorBoundary>
        </div>
        <div className="col-span-5 h-[500px]">
          <RealtimeIntelStream threats={intelStream} />
        </div>
      </div>

      {/* 底部图表 */}
      <div className="grid grid-cols-12 gap-6">
        <div className="col-span-7 h-[400px]">
          <QuotaBurnRateChart data={trafficData} timeRange="1h" />
        </div>
        <div className="col-span-5 h-[400px]">
          <DeceptionInsights />
        </div>
      </div>
    </div>
  );
};

export default MonitorDashboard;
