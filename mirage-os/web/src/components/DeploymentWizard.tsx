/**
 * DeploymentWizard.tsx - 一键部署向导
 * 对应: setup.sh, production_ready_manifest.yaml
 * 功能: 节点引导进度、配置验证、部署状态监控
 */

import React, { useState, useEffect, useCallback } from 'react';

// 部署阶段
type DeployPhase = 
  | 'idle'
  | 'pulling'      // 镜像拉取
  | 'kernel'       // 内核检查
  | 'compiling'    // eBPF 编译
  | 'configuring'  // 配置应用
  | 'networking'   // 组网
  | 'complete'     // 完成
  | 'error';       // 错误

// 节点配置
interface NodeConfig {
  nodeId: string;
  region: string;
  ip: string;
  phase: DeployPhase;
  progress: number;
  logs: string[];
  error?: string;
}

// 配置验证结果
interface ValidationResult {
  field: string;
  valid: boolean;
  message: string;
}

// YAML 配置
interface ManifestConfig {
  nodeId: string;
  region: string;
  listenAddr: string;
  xdpInterface: string;
  shadowPoolSize: number;
  xmrWalletAddress: string;
  torHiddenService: boolean;
}

const DeploymentWizard: React.FC = () => {
  // 状态
  const [step, setStep] = useState<number>(1);
  const [config, setConfig] = useState<ManifestConfig>({
    nodeId: '',
    region: 'sg',
    listenAddr: '0.0.0.0:443',
    xdpInterface: 'eth0',
    shadowPoolSize: 5,
    xmrWalletAddress: '',
    torHiddenService: true,
  });
  const [validations, setValidations] = useState<ValidationResult[]>([]);
  const [nodes, setNodes] = useState<NodeConfig[]>([]);
  const [isDeploying, setIsDeploying] = useState(false);
  const [yamlPreview, setYamlPreview] = useState('');

  // 区域选项
  const regions = [
    { value: 'sg', label: '🇸🇬 Singapore' },
    { value: 'de', label: '🇩🇪 Frankfurt' },
    { value: 'us', label: '🇺🇸 Virginia' },
    { value: 'jp', label: '🇯🇵 Tokyo' },
    { value: 'ch', label: '🇨🇭 Zurich' },
  ];

  // 验证配置
  const validateConfig = useCallback(() => {
    const results: ValidationResult[] = [];

    // Node ID
    if (!config.nodeId) {
      results.push({ field: 'nodeId', valid: false, message: 'Node ID 不能为空' });
    } else if (!/^[a-z0-9-]+$/.test(config.nodeId)) {
      results.push({ field: 'nodeId', valid: false, message: 'Node ID 只能包含小写字母、数字和连字符' });
    } else {
      results.push({ field: 'nodeId', valid: true, message: 'Node ID 格式正确' });
    }

    // Listen Address
    if (!/^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}:\d+$/.test(config.listenAddr)) {
      results.push({ field: 'listenAddr', valid: false, message: '监听地址格式错误 (IP:PORT)' });
    } else {
      results.push({ field: 'listenAddr', valid: true, message: '监听地址格式正确' });
    }

    // XDP Interface
    if (!config.xdpInterface) {
      results.push({ field: 'xdpInterface', valid: false, message: 'XDP 接口不能为空' });
    } else {
      results.push({ field: 'xdpInterface', valid: true, message: 'XDP 接口已配置' });
    }

    // Shadow Pool Size
    if (config.shadowPoolSize < 3 || config.shadowPoolSize > 20) {
      results.push({ field: 'shadowPoolSize', valid: false, message: '影子池大小应在 3-20 之间' });
    } else {
      results.push({ field: 'shadowPoolSize', valid: true, message: '影子池大小合理' });
    }

    // XMR Wallet
    if (config.xmrWalletAddress && !/^4[0-9AB][1-9A-HJ-NP-Za-km-z]{93}$/.test(config.xmrWalletAddress)) {
      results.push({ field: 'xmrWalletAddress', valid: false, message: 'XMR 钱包地址格式错误' });
    } else if (config.xmrWalletAddress) {
      results.push({ field: 'xmrWalletAddress', valid: true, message: 'XMR 钱包地址格式正确' });
    }

    setValidations(results);
    return results.every(r => r.valid);
  }, [config]);

  // 生成 YAML 预览
  useEffect(() => {
    const yaml = `# Mirage Gateway Configuration
# Generated: ${new Date().toISOString()}

gateway:
  node_id: "${config.nodeId || 'node-xxx'}"
  region: "${config.region}"
  listen_addr: "${config.listenAddr}"

ebpf:
  xdp_interface: "${config.xdpInterface}"
  tc_interface: "${config.xdpInterface}"
  programs:
    - npm.o
    - bdna.o
    - jitter.o
    - sockmap.o

gswitch:
  shadow_pool_size: ${config.shadowPoolSize}
  warmup_interval_sec: 300
  reputation_threshold: 40

mcc:
  tor_hidden_service: ${config.torHiddenService}
  quorum_size: 3

billing:
  xmr_wallet: "${config.xmrWalletAddress || 'NOT_CONFIGURED'}"
`;
    setYamlPreview(yaml);
  }, [config]);

  // 开始部署
  const startDeploy = useCallback(async () => {
    if (!validateConfig()) {
      alert('配置验证失败，请检查错误');
      return;
    }

    setIsDeploying(true);
    setStep(3);

    // 创建节点
    const newNode: NodeConfig = {
      nodeId: config.nodeId,
      region: config.region,
      ip: '10.0.0.' + Math.floor(Math.random() * 255),
      phase: 'pulling',
      progress: 0,
      logs: [],
    };
    setNodes([newNode]);

    // 模拟部署过程
    await simulateDeployment(newNode);
  }, [config, validateConfig]);

  // 模拟部署过程
  const simulateDeployment = async (node: NodeConfig) => {
    const phases: { phase: DeployPhase; duration: number; logs: string[] }[] = [
      {
        phase: 'pulling',
        duration: 2000,
        logs: [
          '📦 Pulling mirage/gateway:latest...',
          '📦 Pulling mirage/ebpf-builder:latest...',
          '📦 Pulling mirage/tor:latest...',
          '✓ Images pulled successfully',
        ],
      },
      {
        phase: 'kernel',
        duration: 1500,
        logs: [
          '🔍 Checking kernel version...',
          '✓ Kernel 5.15.0-generic detected',
          '🔍 Checking kernel headers...',
          '✓ Headers available at /usr/src/linux-headers-5.15.0',
          '🔍 Checking BPF support...',
          '✓ BPF JIT enabled',
        ],
      },
      {
        phase: 'compiling',
        duration: 3000,
        logs: [
          '🔨 Compiling npm.c -> npm.o',
          '🔨 Compiling bdna.c -> bdna.o',
          '🔨 Compiling jitter.c -> jitter.o',
          '🔨 Compiling sockmap.c -> sockmap.o',
          '✓ eBPF programs compiled successfully',
        ],
      },
      {
        phase: 'configuring',
        duration: 1500,
        logs: [
          '⚙️ Applying gateway.yaml...',
          '⚙️ Configuring tmpfs mounts...',
          '⚙️ Setting up Tor hidden service...',
          '✓ Configuration applied',
        ],
      },
      {
        phase: 'networking',
        duration: 2000,
        logs: [
          '🌐 Discovering M.C.C. peers...',
          '🌐 Connecting to node-sg-1...',
          '🌐 Connecting to node-de-1...',
          '🌐 Joining Raft cluster...',
          '✓ Network established',
        ],
      },
      {
        phase: 'complete',
        duration: 500,
        logs: [
          '🎉 Deployment complete!',
          `🔗 Node ${node.nodeId} is now online`,
        ],
      },
    ];

    let totalProgress = 0;
    const progressPerPhase = 100 / phases.length;

    for (const { phase, duration, logs } of phases) {
      // 更新阶段
      setNodes(prev => prev.map(n => 
        n.nodeId === node.nodeId ? { ...n, phase, logs: [...n.logs, ...logs] } : n
      ));

      // 模拟进度
      const steps = 10;
      const stepDuration = duration / steps;
      for (let i = 0; i < steps; i++) {
        await new Promise(resolve => setTimeout(resolve, stepDuration));
        totalProgress += progressPerPhase / steps;
        setNodes(prev => prev.map(n => 
          n.nodeId === node.nodeId ? { ...n, progress: Math.min(totalProgress, 100) } : n
        ));
      }
    }

    setIsDeploying(false);
  };

  // 获取阶段颜色
  const getPhaseColor = (phase: DeployPhase) => {
    switch (phase) {
      case 'complete': return '#22c55e';
      case 'error': return '#ef4444';
      case 'idle': return '#6b7280';
      default: return '#3b82f6';
    }
  };

  // 获取阶段图标
  const getPhaseIcon = (phase: DeployPhase) => {
    switch (phase) {
      case 'pulling': return '📦';
      case 'kernel': return '🔍';
      case 'compiling': return '🔨';
      case 'configuring': return '⚙️';
      case 'networking': return '🌐';
      case 'complete': return '✅';
      case 'error': return '❌';
      default: return '⏳';
    }
  };

  return (
    <div className="p-6 bg-gray-900 min-h-screen">
      {/* 标题 */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-white flex items-center gap-3">
          <span className="text-3xl">🚀</span>
          一键部署向导
        </h1>
        <div className="text-gray-400 text-sm">
          Mirage-One-Click Deployment
        </div>
      </div>

      {/* 步骤指示器 */}
      <div className="flex items-center justify-center mb-8">
        {['配置', '验证', '部署'].map((label, idx) => (
          <React.Fragment key={label}>
            <div className="flex items-center">
              <div
                className={`w-10 h-10 rounded-full flex items-center justify-center font-bold ${
                  step > idx + 1
                    ? 'bg-green-500 text-white'
                    : step === idx + 1
                    ? 'bg-blue-500 text-white'
                    : 'bg-gray-700 text-gray-400'
                }`}
              >
                {step > idx + 1 ? '✓' : idx + 1}
              </div>
              <span className={`ml-2 ${step >= idx + 1 ? 'text-white' : 'text-gray-500'}`}>
                {label}
              </span>
            </div>
            {idx < 2 && (
              <div className={`w-20 h-1 mx-4 ${step > idx + 1 ? 'bg-green-500' : 'bg-gray-700'}`} />
            )}
          </React.Fragment>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* 左侧: 配置表单 */}
        <div className="bg-gray-800 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-white mb-4">📝 节点配置</h2>

          <div className="space-y-4">
            {/* Node ID */}
            <div>
              <label className="block text-gray-400 text-sm mb-1">Node ID *</label>
              <input
                type="text"
                value={config.nodeId}
                onChange={(e) => setConfig({ ...config, nodeId: e.target.value })}
                placeholder="node-sg-1"
                className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white"
              />
            </div>

            {/* Region */}
            <div>
              <label className="block text-gray-400 text-sm mb-1">Region *</label>
              <select
                value={config.region}
                onChange={(e) => setConfig({ ...config, region: e.target.value })}
                className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white"
              >
                {regions.map((r) => (
                  <option key={r.value} value={r.value}>{r.label}</option>
                ))}
              </select>
            </div>

            {/* Listen Address */}
            <div>
              <label className="block text-gray-400 text-sm mb-1">Listen Address</label>
              <input
                type="text"
                value={config.listenAddr}
                onChange={(e) => setConfig({ ...config, listenAddr: e.target.value })}
                className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white"
              />
            </div>

            {/* XDP Interface */}
            <div>
              <label className="block text-gray-400 text-sm mb-1">XDP Interface</label>
              <input
                type="text"
                value={config.xdpInterface}
                onChange={(e) => setConfig({ ...config, xdpInterface: e.target.value })}
                className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white"
              />
            </div>

            {/* Shadow Pool Size */}
            <div>
              <label className="block text-gray-400 text-sm mb-1">
                Shadow Pool Size: {config.shadowPoolSize}
              </label>
              <input
                type="range"
                min="3"
                max="20"
                value={config.shadowPoolSize}
                onChange={(e) => setConfig({ ...config, shadowPoolSize: parseInt(e.target.value) })}
                className="w-full"
              />
            </div>

            {/* XMR Wallet */}
            <div>
              <label className="block text-gray-400 text-sm mb-1">XMR Wallet Address</label>
              <input
                type="text"
                value={config.xmrWalletAddress}
                onChange={(e) => setConfig({ ...config, xmrWalletAddress: e.target.value })}
                placeholder="4..."
                className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white font-mono text-xs"
              />
            </div>

            {/* Tor Hidden Service */}
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={config.torHiddenService}
                onChange={(e) => setConfig({ ...config, torHiddenService: e.target.checked })}
                className="w-4 h-4"
              />
              <label className="text-gray-400 text-sm">启用 Tor 隐藏服务</label>
            </div>
          </div>

          {/* 验证按钮 */}
          <div className="mt-6 flex gap-3">
            <button
              onClick={() => { validateConfig(); setStep(2); }}
              className="flex-1 py-2 bg-blue-600 hover:bg-blue-500 rounded text-white font-medium"
            >
              验证配置
            </button>
            <button
              onClick={startDeploy}
              disabled={isDeploying}
              className="flex-1 py-2 bg-green-600 hover:bg-green-500 disabled:bg-gray-600 rounded text-white font-medium"
            >
              {isDeploying ? '部署中...' : '开始部署'}
            </button>
          </div>

          {/* 验证结果 */}
          {validations.length > 0 && (
            <div className="mt-4 space-y-2">
              {validations.map((v, idx) => (
                <div
                  key={idx}
                  className={`flex items-center gap-2 text-sm ${
                    v.valid ? 'text-green-400' : 'text-red-400'
                  }`}
                >
                  <span>{v.valid ? '✓' : '✗'}</span>
                  <span>{v.message}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* 右侧: YAML 预览 & 部署状态 */}
        <div className="space-y-6">
          {/* YAML 预览 */}
          <div className="bg-gray-800 rounded-lg p-6">
            <h2 className="text-lg font-semibold text-white mb-4">📄 YAML 预览</h2>
            <pre className="bg-gray-900 rounded p-4 text-xs text-gray-300 font-mono overflow-auto max-h-64">
              {yamlPreview}
            </pre>
          </div>

          {/* 部署状态 */}
          {nodes.length > 0 && (
            <div className="bg-gray-800 rounded-lg p-6">
              <h2 className="text-lg font-semibold text-white mb-4">📊 部署进度</h2>

              {nodes.map((node) => (
                <div key={node.nodeId} className="space-y-4">
                  {/* 节点信息 */}
                  <div className="flex items-center justify-between">
                    <div>
                      <span className="text-white font-medium">{node.nodeId}</span>
                      <span className="text-gray-400 text-sm ml-2">({node.region})</span>
                    </div>
                    <span
                      className="px-2 py-1 rounded text-xs font-medium"
                      style={{ backgroundColor: getPhaseColor(node.phase), color: '#fff' }}
                    >
                      {getPhaseIcon(node.phase)} {node.phase.toUpperCase()}
                    </span>
                  </div>

                  {/* 进度条 */}
                  <div className="w-full h-3 bg-gray-700 rounded-full overflow-hidden">
                    <div
                      className="h-full transition-all duration-300"
                      style={{
                        width: `${node.progress}%`,
                        backgroundColor: getPhaseColor(node.phase),
                      }}
                    />
                  </div>

                  {/* 阶段指示器 */}
                  <div className="flex justify-between text-xs">
                    {['pulling', 'kernel', 'compiling', 'configuring', 'networking', 'complete'].map((p) => (
                      <div
                        key={p}
                        className={`flex flex-col items-center ${
                          node.phase === p ? 'text-blue-400' : 
                          ['pulling', 'kernel', 'compiling', 'configuring', 'networking', 'complete']
                            .indexOf(node.phase) > ['pulling', 'kernel', 'compiling', 'configuring', 'networking', 'complete'].indexOf(p)
                            ? 'text-green-400' : 'text-gray-500'
                        }`}
                      >
                        <span>{getPhaseIcon(p as DeployPhase)}</span>
                        <span className="mt-1">{p}</span>
                      </div>
                    ))}
                  </div>

                  {/* 日志 */}
                  <div className="bg-gray-900 rounded p-3 max-h-40 overflow-auto">
                    {node.logs.map((log, idx) => (
                      <div key={idx} className="text-xs text-gray-400 font-mono">
                        {log}
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* 底部提示 */}
      <div className="mt-6 p-4 bg-blue-900/20 border border-blue-500/50 rounded-lg">
        <div className="flex items-center gap-3">
          <span className="text-2xl">💡</span>
          <div>
            <div className="text-blue-400 font-medium">部署提示</div>
            <div className="text-gray-400 text-sm">
              确保目标服务器内核版本 ≥ 5.15，已安装 clang/llvm，并具有 root 权限。
              部署完成后，节点将自动加入 M.C.C. 集群。
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default DeploymentWizard;
