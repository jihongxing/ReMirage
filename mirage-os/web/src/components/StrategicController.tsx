/**
 * StrategicController.tsx - 战略控制面板
 * 对应: kill_switch.go
 * 功能: 两阶段解锁、静默状态监控、全网清除进度
 */

import React, { useState, useEffect, useRef, useCallback } from 'react';

// 开关状态
type KillSwitchState = 'active' | 'armed' | 'triggered' | 'silent';

// 节点清除状态
interface NodeClearStatus {
  nodeId: string;
  region: string;
  status: 'active' | 'clearing' | 'ghost';
  progress: number;
  ebpfUnloaded: boolean;
  memoryCleared: boolean;
  dnsSwapped: boolean;
}

// 静默进度
interface SilenceProgress {
  phase: string;
  totalNodes: number;
  clearedNodes: number;
  startTime: number;
  estimatedComplete: number;
}

const StrategicController: React.FC = () => {
  // 状态
  const [switchState, setSwitchState] = useState<KillSwitchState>('active');
  const [armCode, setArmCode] = useState('');
  const [triggerCode, setTriggerCode] = useState('');
  const [coverOpen, setCoverOpen] = useState(false);
  const [holdProgress, setHoldProgress] = useState(0);
  const [isHolding, setIsHolding] = useState(false);
  const [nodes, setNodes] = useState<NodeClearStatus[]>([]);
  const [silenceProgress, setSilenceProgress] = useState<SilenceProgress | null>(null);
  const [showWarning, setShowWarning] = useState(false);
  const [shakeScreen, setShakeScreen] = useState(false);

  const holdTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  // 模拟节点数据
  useEffect(() => {
    const mockNodes: NodeClearStatus[] = [
      { nodeId: 'node-sg-1', region: 'Singapore', status: 'active', progress: 0, ebpfUnloaded: false, memoryCleared: false, dnsSwapped: false },
      { nodeId: 'node-de-1', region: 'Frankfurt', status: 'active', progress: 0, ebpfUnloaded: false, memoryCleared: false, dnsSwapped: false },
      { nodeId: 'node-us-1', region: 'Virginia', status: 'active', progress: 0, ebpfUnloaded: false, memoryCleared: false, dnsSwapped: false },
      { nodeId: 'node-jp-1', region: 'Tokyo', status: 'active', progress: 0, ebpfUnloaded: false, memoryCleared: false, dnsSwapped: false },
      { nodeId: 'node-ch-1', region: 'Zurich', status: 'active', progress: 0, ebpfUnloaded: false, memoryCleared: false, dnsSwapped: false },
    ];
    setNodes(mockNodes);
  }, []);

  // 武装开关
  const handleArm = useCallback(() => {
    if (armCode.length < 8) {
      alert('武装码至少 8 位');
      return;
    }

    // 模拟 API 调用
    setSwitchState('armed');
    setShowWarning(true);
    
    // 5 分钟后自动解除
    setTimeout(() => {
      if (switchState === 'armed') {
        setSwitchState('active');
        setShowWarning(false);
        setCoverOpen(false);
      }
    }, 5 * 60 * 1000);
  }, [armCode, switchState]);

  // 开始按住触发
  const handleHoldStart = useCallback(() => {
    if (switchState !== 'armed') return;
    if (triggerCode.length < 8) {
      alert('触发码至少 8 位');
      return;
    }

    setIsHolding(true);
    let progress = 0;

    holdTimerRef.current = setInterval(() => {
      progress += 3.33; // 3 秒完成
      setHoldProgress(progress);

      if (progress >= 100) {
        if (holdTimerRef.current) clearInterval(holdTimerRef.current);
        executeTrigger();
      }
    }, 100);
  }, [switchState, triggerCode]);

  // 松开取消
  const handleHoldEnd = useCallback(() => {
    if (holdTimerRef.current) {
      clearInterval(holdTimerRef.current);
    }
    setIsHolding(false);
    setHoldProgress(0);
  }, []);

  // 执行触发
  const executeTrigger = useCallback(() => {
    setSwitchState('triggered');
    setShakeScreen(true);
    
    // 播放警告音
    if (audioRef.current) {
      audioRef.current.play();
    }

    // 停止震动
    setTimeout(() => setShakeScreen(false), 2000);

    // 开始静默进度
    setSilenceProgress({
      phase: '初始化',
      totalNodes: nodes.length,
      clearedNodes: 0,
      startTime: Date.now(),
      estimatedComplete: Date.now() + 30000,
    });

    // 模拟清除过程
    simulateClearProcess();
  }, [nodes]);

  // 模拟清除过程
  const simulateClearProcess = useCallback(() => {
    let clearedCount = 0;

    nodes.forEach((_, index) => {
      // 阶段 1: eBPF 卸载
      setTimeout(() => {
        setNodes(prev => prev.map((n, i) => 
          i === index ? { ...n, status: 'clearing', progress: 33, ebpfUnloaded: true } : n
        ));
        setSilenceProgress(prev => prev ? { ...prev, phase: 'eBPF 卸载中' } : null);
      }, index * 1000 + 500);

      // 阶段 2: 内存清除
      setTimeout(() => {
        setNodes(prev => prev.map((n, i) => 
          i === index ? { ...n, progress: 66, memoryCleared: true } : n
        ));
        setSilenceProgress(prev => prev ? { ...prev, phase: '内存擦除中' } : null);
      }, index * 1000 + 1500);

      // 阶段 3: DNS 切换
      setTimeout(() => {
        setNodes(prev => prev.map((n, i) => 
          i === index ? { ...n, status: 'ghost', progress: 100, dnsSwapped: true } : n
        ));
        clearedCount++;
        setSilenceProgress(prev => prev ? { 
          ...prev, 
          phase: clearedCount === nodes.length ? '静默完成' : 'DNS 切换中',
          clearedNodes: clearedCount 
        } : null);

        if (clearedCount === nodes.length) {
          setSwitchState('silent');
        }
      }, index * 1000 + 2500);
    });
  }, [nodes]);

  // 获取状态颜色
  const getStatusColor = (status: string) => {
    switch (status) {
      case 'active': return '#22c55e';
      case 'clearing': return '#eab308';
      case 'ghost': return '#6b7280';
      default: return '#6b7280';
    }
  };

  // 获取开关状态颜色
  const getSwitchStateColor = () => {
    switch (switchState) {
      case 'active': return '#22c55e';
      case 'armed': return '#eab308';
      case 'triggered': return '#ef4444';
      case 'silent': return '#6b7280';
    }
  };

  return (
    <div className={`p-6 bg-gray-900 min-h-screen ${shakeScreen ? 'animate-shake' : ''}`}>
      {/* 全屏警告遮罩 */}
      {showWarning && switchState === 'armed' && (
        <div className="fixed inset-0 bg-red-900/20 border-4 border-red-500 animate-pulse pointer-events-none z-40" />
      )}

      {/* 警告音 */}
      <audio ref={audioRef} src="/warning.mp3" />

      {/* 标题 */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-white flex items-center gap-3">
          <span className="text-3xl">☢️</span>
          战略控制面板
          <span className="text-sm px-2 py-1 rounded" style={{ backgroundColor: getSwitchStateColor() }}>
            {switchState.toUpperCase()}
          </span>
        </h1>
        <div className="text-gray-400 text-sm">
          Physical Kill Switch v1.0
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* 左侧: 开关控制 */}
        <div className="bg-gray-800 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-white mb-4">🔴 紧急物理开关</h2>

          {/* 两阶段解锁 */}
          <div className="space-y-4">
            {/* 阶段 1: 武装 */}
            <div className={`p-4 rounded-lg border-2 ${switchState === 'active' ? 'border-gray-600' : 'border-yellow-500'}`}>
              <div className="text-sm text-gray-400 mb-2">阶段 1: 武装系统</div>
              <div className="flex gap-2">
                <input
                  type="password"
                  placeholder="输入武装码 (8+ 位)"
                  value={armCode}
                  onChange={(e) => setArmCode(e.target.value)}
                  disabled={switchState !== 'active'}
                  className="flex-1 px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white"
                />
                <button
                  onClick={handleArm}
                  disabled={switchState !== 'active'}
                  className="px-4 py-2 bg-yellow-600 hover:bg-yellow-500 disabled:bg-gray-600 rounded text-white font-medium"
                >
                  武装
                </button>
              </div>
            </div>

            {/* 阶段 2: 触发 */}
            <div className={`p-4 rounded-lg border-2 ${switchState === 'armed' ? 'border-red-500' : 'border-gray-600'}`}>
              <div className="text-sm text-gray-400 mb-2">阶段 2: 触发静默</div>
              
              {/* 触发码输入 */}
              <div className="flex gap-2 mb-4">
                <input
                  type="password"
                  placeholder="输入触发码 (8+ 位)"
                  value={triggerCode}
                  onChange={(e) => setTriggerCode(e.target.value)}
                  disabled={switchState !== 'armed'}
                  className="flex-1 px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white"
                />
              </div>

              {/* 物理开关 (带保护盖) */}
              <div className="flex items-center justify-center">
                <div className="relative">
                  {/* 保护盖 */}
                  <div
                    onClick={() => switchState === 'armed' && setCoverOpen(!coverOpen)}
                    className={`absolute -top-2 left-1/2 -translate-x-1/2 w-24 h-12 rounded-t-lg cursor-pointer transition-transform duration-300 ${
                      coverOpen ? '-rotate-180 -translate-y-10' : ''
                    }`}
                    style={{ 
                      backgroundColor: '#dc2626',
                      transformOrigin: 'bottom center',
                      boxShadow: '0 2px 4px rgba(0,0,0,0.5)'
                    }}
                  >
                    <div className="text-center text-white text-xs pt-2 font-bold">
                      {coverOpen ? '' : '⚠️ LIFT'}
                    </div>
                  </div>

                  {/* 开关按钮 */}
                  <button
                    onMouseDown={handleHoldStart}
                    onMouseUp={handleHoldEnd}
                    onMouseLeave={handleHoldEnd}
                    onTouchStart={handleHoldStart}
                    onTouchEnd={handleHoldEnd}
                    disabled={!coverOpen || switchState !== 'armed'}
                    className={`w-24 h-24 rounded-full border-4 transition-all ${
                      coverOpen && switchState === 'armed'
                        ? 'border-red-500 bg-red-600 hover:bg-red-500 cursor-pointer'
                        : 'border-gray-600 bg-gray-700 cursor-not-allowed'
                    }`}
                    style={{
                      boxShadow: isHolding ? '0 0 30px rgba(239, 68, 68, 0.8)' : 'none'
                    }}
                  >
                    <div className="text-white text-xs font-bold">
                      {isHolding ? `${holdProgress.toFixed(0)}%` : 'HOLD 3s'}
                    </div>
                  </button>

                  {/* 进度环 */}
                  {isHolding && (
                    <svg className="absolute inset-0 w-24 h-24 -rotate-90">
                      <circle
                        cx="48"
                        cy="48"
                        r="44"
                        fill="none"
                        stroke="#ef4444"
                        strokeWidth="4"
                        strokeDasharray={`${holdProgress * 2.76} 276`}
                      />
                    </svg>
                  )}
                </div>
              </div>

              <div className="text-center text-gray-400 text-xs mt-4">
                打开保护盖 → 按住开关 3 秒 → 全网静默
              </div>
            </div>
          </div>

          {/* 状态说明 */}
          <div className="mt-4 p-3 bg-gray-700/50 rounded text-sm text-gray-300">
            <div className="font-medium mb-2">状态说明:</div>
            <div className="space-y-1 text-xs">
              <div><span className="text-green-400">●</span> ACTIVE - 正常运行</div>
              <div><span className="text-yellow-400">●</span> ARMED - 已武装 (5分钟超时)</div>
              <div><span className="text-red-400">●</span> TRIGGERED - 正在执行静默</div>
              <div><span className="text-gray-400">●</span> SILENT - 静默完成</div>
            </div>
          </div>
        </div>

        {/* 右侧: 节点状态 */}
        <div className="bg-gray-800 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-white mb-4">🌐 全球节点状态</h2>

          {/* 静默进度 */}
          {silenceProgress && (
            <div className="mb-4 p-4 bg-red-900/30 border border-red-500 rounded-lg">
              <div className="flex justify-between items-center mb-2">
                <span className="text-red-400 font-medium">{silenceProgress.phase}</span>
                <span className="text-white">
                  {silenceProgress.clearedNodes}/{silenceProgress.totalNodes}
                </span>
              </div>
              <div className="w-full h-2 bg-gray-700 rounded-full overflow-hidden">
                <div
                  className="h-full bg-red-500 transition-all duration-300"
                  style={{ width: `${(silenceProgress.clearedNodes / silenceProgress.totalNodes) * 100}%` }}
                />
              </div>
            </div>
          )}

          {/* 节点列表 */}
          <div className="space-y-3">
            {nodes.map((node) => (
              <div
                key={node.nodeId}
                className="p-3 bg-gray-700/50 rounded-lg border-l-4 transition-all"
                style={{ borderColor: getStatusColor(node.status) }}
              >
                <div className="flex justify-between items-center mb-2">
                  <div>
                    <span className="text-white font-medium">{node.nodeId}</span>
                    <span className="text-gray-400 text-sm ml-2">{node.region}</span>
                  </div>
                  <span
                    className="px-2 py-1 rounded text-xs font-medium"
                    style={{ backgroundColor: getStatusColor(node.status), color: '#fff' }}
                  >
                    {node.status.toUpperCase()}
                  </span>
                </div>

                {/* 清除进度 */}
                {node.status !== 'active' && (
                  <div className="space-y-1">
                    <div className="w-full h-1 bg-gray-600 rounded-full overflow-hidden">
                      <div
                        className="h-full bg-red-500 transition-all duration-500"
                        style={{ width: `${node.progress}%` }}
                      />
                    </div>
                    <div className="flex gap-4 text-xs">
                      <span className={node.ebpfUnloaded ? 'text-green-400' : 'text-gray-500'}>
                        {node.ebpfUnloaded ? '✓' : '○'} eBPF
                      </span>
                      <span className={node.memoryCleared ? 'text-green-400' : 'text-gray-500'}>
                        {node.memoryCleared ? '✓' : '○'} Memory
                      </span>
                      <span className={node.dnsSwapped ? 'text-green-400' : 'text-gray-500'}>
                        {node.dnsSwapped ? '✓' : '○'} DNS
                      </span>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* 底部警告 */}
      <div className="mt-6 p-4 bg-red-900/20 border border-red-500 rounded-lg">
        <div className="flex items-center gap-3">
          <span className="text-3xl">⚠️</span>
          <div>
            <div className="text-red-400 font-bold">警告: 此操作不可逆</div>
            <div className="text-gray-400 text-sm">
              触发后将卸载所有 eBPF 程序、清空内存数据、切换 DNS 至伪装服务器。
              所有节点将变为"幽灵"状态，无法恢复。
            </div>
          </div>
        </div>
      </div>

      {/* 震动动画样式 */}
      <style>{`
        @keyframes shake {
          0%, 100% { transform: translateX(0); }
          10%, 30%, 50%, 70%, 90% { transform: translateX(-10px); }
          20%, 40%, 60%, 80% { transform: translateX(10px); }
        }
        .animate-shake {
          animation: shake 0.5s ease-in-out infinite;
        }
      `}</style>
    </div>
  );
};

export default StrategicController;
