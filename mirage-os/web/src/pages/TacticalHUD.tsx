// Mirage Tactical HUD V1.0 Final - 战术指挥中心
// 集成所有核心组件的沉浸式战术控制台
import { useState, useEffect, useCallback } from 'react';
import StrategicController from '../components/StrategicController';
import GlobalTacticalHUD from '../components/GlobalTacticalHUD';
import MirageHeartbeat from '../components/MirageHeartbeat';
import VaultObserver from '../components/VaultObserver';
import DnaEvolutionEngine from '../components/DnaEvolutionEngine';
import HealthDiagnostics from '../components/HealthDiagnostics';

type TabType = 'overview' | 'performance' | 'assets' | 'dna';

export default function TacticalHUD() {
  const [activeTab, setActiveTab] = useState<TabType>('overview');
  const [ghostUI, setGhostUI] = useState(false);
  const [systemTime, setSystemTime] = useState(new Date());

  // 系统时钟
  useEffect(() => {
    const timer = setInterval(() => setSystemTime(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  // Ghost UI 快捷键 (Ctrl+Shift+G)
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.ctrlKey && e.shiftKey && e.key === 'G') {
      e.preventDefault();
      setGhostUI(prev => !prev);
    }
  }, []);

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  // Ghost UI 模式 - 伪装为普通性能监控
  if (ghostUI) {
    return (
      <div className="min-h-screen bg-white p-8">
        <div className="max-w-4xl mx-auto">
          <h1 className="text-2xl font-bold text-gray-800 mb-6">System Performance Monitor</h1>
          <div className="grid grid-cols-3 gap-4 mb-6">
            <div className="bg-gray-100 rounded-lg p-4">
              <div className="text-sm text-gray-500">CPU Usage</div>
              <div className="text-2xl font-bold text-gray-800">23%</div>
            </div>
            <div className="bg-gray-100 rounded-lg p-4">
              <div className="text-sm text-gray-500">Memory</div>
              <div className="text-2xl font-bold text-gray-800">4.2 GB</div>
            </div>
            <div className="bg-gray-100 rounded-lg p-4">
              <div className="text-sm text-gray-500">Disk I/O</div>
              <div className="text-2xl font-bold text-gray-800">12 MB/s</div>
            </div>
          </div>
          <div className="bg-gray-100 rounded-lg p-4 h-64 flex items-center justify-center">
            <span className="text-gray-400">Performance Graph</span>
          </div>
          <p className="text-xs text-gray-400 mt-4 text-center">
            Press Ctrl+Shift+G to return · Standard Monitoring Interface
          </p>
        </div>
      </div>
    );
  }

  const tabs = [
    { id: 'overview' as const, label: '🎯 态势总览', desc: '全球节点与威胁态势' },
    { id: 'performance' as const, label: '⚡ 性能脉动', desc: '纳秒级响应监控' },
    { id: 'assets' as const, label: '🔐 资产保险箱', desc: 'IP/DNA 资产管理' },
    { id: 'dna' as const, label: '🧬 DNA 进化', desc: '指纹拟态控制' },
  ];

  return (
    <div className="min-h-screen bg-slate-950">
      {/* 顶部状态栏 */}
      <header className="bg-slate-900 border-b border-slate-800 px-6 py-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <h1 className="text-xl font-bold text-white flex items-center gap-2">
              🎮 Mirage Tactical HUD
              <span className="text-xs text-cyan-400 font-normal">V1.0</span>
            </h1>
            <div className="flex items-center gap-2 text-xs text-slate-500">
              <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
              实时同步
            </div>
          </div>
          
          <div className="flex items-center gap-6">
            {/* Ghost UI 提示 */}
            <div className="text-xs text-slate-600">
              Ctrl+Shift+G 切换伪装
            </div>
            
            {/* 系统时间 */}
            <div className="text-sm text-slate-400 font-mono">
              {systemTime.toLocaleTimeString()}
            </div>
          </div>
        </div>
      </header>

      {/* Tab 导航 */}
      <div className="bg-slate-900/50 border-b border-slate-800 px-6">
        <div className="flex gap-1">
          {tabs.map(tab => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-3 text-sm transition-all border-b-2 ${
                activeTab === tab.id
                  ? 'text-cyan-400 border-cyan-400 bg-cyan-400/5'
                  : 'text-slate-400 border-transparent hover:text-white hover:bg-slate-800/50'
              }`}
            >
              <div>{tab.label}</div>
              <div className="text-xs text-slate-500">{tab.desc}</div>
            </button>
          ))}
        </div>
      </div>

      {/* 主内容区 */}
      <main className="p-6">
        {activeTab === 'overview' && (
          <div className="grid grid-cols-12 gap-6">
            {/* 左侧：战略控制器 */}
            <div className="col-span-3">
              <StrategicController />
            </div>
            
            {/* 中间：全球态势 */}
            <div className="col-span-9">
              <GlobalTacticalHUD />
            </div>
          </div>
        )}

        {activeTab === 'performance' && (
          <div className="grid grid-cols-12 gap-6">
            {/* 系统脉动 */}
            <div className="col-span-7">
              <MirageHeartbeat />
            </div>
            
            {/* 健康诊断 */}
            <div className="col-span-5">
              <HealthDiagnostics />
            </div>
          </div>
        )}

        {activeTab === 'assets' && (
          <div className="grid grid-cols-12 gap-6">
            {/* 资产保险箱 */}
            <div className="col-span-12">
              <VaultObserver />
            </div>
          </div>
        )}

        {activeTab === 'dna' && (
          <div className="grid grid-cols-12 gap-6">
            {/* DNA 进化控制台 */}
            <div className="col-span-12">
              <DnaEvolutionEngine />
            </div>
          </div>
        )}
      </main>

      {/* 底部状态栏 */}
      <footer className="fixed bottom-0 left-0 right-0 bg-slate-900/95 backdrop-blur-sm border-t border-slate-800 px-6 py-2">
        <div className="flex items-center justify-between text-xs text-slate-500">
          <div className="flex items-center gap-6">
            <span>Mirage Project © 2026</span>
            <span className="text-slate-600">|</span>
            <span className="flex items-center gap-1">
              <span className="w-1.5 h-1.5 bg-green-500 rounded-full" />
              BPF: &lt;1ms
            </span>
            <span className="flex items-center gap-1">
              <span className="w-1.5 h-1.5 bg-cyan-500 rounded-full" />
              Cache: 100%
            </span>
            <span className="flex items-center gap-1">
              <span className="w-1.5 h-1.5 bg-yellow-500 rounded-full" />
              IO: ↓99.9%
            </span>
          </div>
          <div className="flex items-center gap-4">
            <span>绝对隐匿 · 网络黑洞</span>
          </div>
        </div>
      </footer>
    </div>
  );
}
