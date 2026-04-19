/**
 * DnaLab.tsx - DNA 进化实验室
 * 对应: dna_loader.go
 * 功能: 指纹版本对比、一键热部署、拟态成功率预测
 */

import React, { useState, useEffect, useCallback } from 'react';

// DNA 模板
interface DNATemplate {
  id: string;
  name: string;
  version: string;
  browser: string;
  os: string;
  ja4: string;
  windowSize: number;
  ttl: number;
  mss: number;
  createdAt: string;
  checksum: string;
}

// 同步状态
interface SyncStatus {
  nodeId: string;
  region: string;
  synced: boolean;
  latencyMs: number;
  version: string;
}

// 生存预测
interface SurvivalPrediction {
  app: string;
  probability: number;
  risk: 'low' | 'medium' | 'high';
  notes: string;
}

const DnaLab: React.FC = () => {
  // 状态
  const [templates, setTemplates] = useState<DNATemplate[]>([]);
  const [activeTemplate, setActiveTemplate] = useState<string>('');
  const [pendingTemplate, setPendingTemplate] = useState<DNATemplate | null>(null);
  const [syncStatus, setSyncStatus] = useState<SyncStatus[]>([]);
  const [predictions, setPredictions] = useState<SurvivalPrediction[]>([]);
  const [isDeploying, setIsDeploying] = useState(false);
  const [deployProgress, setDeployProgress] = useState(0);
  const [showDiff, setShowDiff] = useState(false);

  // 加载模板数据
  useEffect(() => {
    const mockTemplates: DNATemplate[] = [
      {
        id: 'chrome-130-win11',
        name: 'Chrome 130 on Windows 11',
        version: '130.0.0.0',
        browser: 'Chrome',
        os: 'Windows 11',
        ja4: 't13d1516h2_8daaf6152771_e5627efa2ab1',
        windowSize: 65535,
        ttl: 128,
        mss: 1460,
        createdAt: '2026-02-01',
        checksum: 'a1b2c3d4',
      },
      {
        id: 'firefox-125-linux',
        name: 'Firefox 125 on Linux',
        version: '125.0',
        browser: 'Firefox',
        os: 'Linux',
        ja4: 't13d1715h2_5b57614c22b0_3d5424432f57',
        windowSize: 32768,
        ttl: 64,
        mss: 1460,
        createdAt: '2026-01-15',
        checksum: 'e5f6g7h8',
      },
      {
        id: 'safari-17-macos',
        name: 'Safari 17 on macOS',
        version: '17.4',
        browser: 'Safari',
        os: 'macOS Sonoma',
        ja4: 't13d1517h2_8daaf6152771_02713d6af862',
        windowSize: 65535,
        ttl: 64,
        mss: 1460,
        createdAt: '2026-01-20',
        checksum: 'i9j0k1l2',
      },
    ];
    setTemplates(mockTemplates);
    setActiveTemplate('chrome-130-win11');

    // 模拟待更新模板
    setPendingTemplate({
      id: 'chrome-131-win11',
      name: 'Chrome 131 on Windows 11',
      version: '131.0.0.0',
      browser: 'Chrome',
      os: 'Windows 11',
      ja4: 't13d1516h2_9ebaf7263882_f6738gfb3bc2',
      windowSize: 65535,
      ttl: 128,
      mss: 1460,
      createdAt: '2026-02-05',
      checksum: 'm3n4o5p6',
    });

    // 模拟节点同步状态
    setSyncStatus([
      { nodeId: 'node-sg-1', region: 'Singapore', synced: true, latencyMs: 35, version: '130.0.0.0' },
      { nodeId: 'node-de-1', region: 'Frankfurt', synced: true, latencyMs: 145, version: '130.0.0.0' },
      { nodeId: 'node-us-1', region: 'Virginia', synced: true, latencyMs: 195, version: '130.0.0.0' },
      { nodeId: 'node-jp-1', region: 'Tokyo', synced: false, latencyMs: 55, version: '129.0.0.0' },
      { nodeId: 'node-ch-1', region: 'Zurich', synced: true, latencyMs: 175, version: '130.0.0.0' },
    ]);

    // 模拟生存预测
    setPredictions([
      { app: 'YouTube', probability: 98.5, risk: 'low', notes: 'JA4 完全匹配' },
      { app: 'Netflix', probability: 96.2, risk: 'low', notes: 'TLS 指纹正常' },
      { app: 'Twitter/X', probability: 94.8, risk: 'low', notes: 'API 行为模式匹配' },
      { app: 'Instagram', probability: 89.3, risk: 'medium', notes: 'Canvas 指纹需更新' },
      { app: 'TikTok', probability: 82.1, risk: 'medium', notes: 'WebGL 指纹偏差' },
      { app: 'Cloudflare', probability: 75.6, risk: 'high', notes: 'Bot 检测风险' },
    ]);
  }, []);

  // 执行部署
  const handleDeploy = useCallback(async () => {
    if (!pendingTemplate) return;

    setIsDeploying(true);
    setDeployProgress(0);

    // 模拟部署过程
    const totalNodes = syncStatus.length;
    for (let i = 0; i <= totalNodes; i++) {
      await new Promise(resolve => setTimeout(resolve, 800));
      setDeployProgress((i / totalNodes) * 100);

      if (i < totalNodes) {
        setSyncStatus(prev => prev.map((node, idx) => 
          idx === i ? { ...node, synced: true, version: pendingTemplate.version } : node
        ));
      }
    }

    // 更新活跃模板
    setTemplates(prev => [...prev, pendingTemplate]);
    setActiveTemplate(pendingTemplate.id);
    setPendingTemplate(null);
    setIsDeploying(false);
  }, [pendingTemplate, syncStatus]);

  // 获取风险颜色
  const getRiskColor = (risk: string) => {
    switch (risk) {
      case 'low': return '#22c55e';
      case 'medium': return '#eab308';
      case 'high': return '#ef4444';
      default: return '#6b7280';
    }
  };

  // 获取浏览器图标
  const getBrowserIcon = (browser: string) => {
    switch (browser) {
      case 'Chrome': return '🌐';
      case 'Firefox': return '🦊';
      case 'Safari': return '🧭';
      case 'Edge': return '📘';
      default: return '🔷';
    }
  };

  // 当前活跃模板
  const currentTemplate = templates.find(t => t.id === activeTemplate);

  return (
    <div className="p-6 bg-gray-900 min-h-screen">
      {/* 标题 */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-white flex items-center gap-3">
          <span className="text-3xl">🧬</span>
          DNA 进化实验室
        </h1>
        <div className="text-gray-400 text-sm">
          指纹库版本: v2026.02.05 | 模板数: {templates.length}
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* 左侧: 指纹库 */}
        <div className="bg-gray-800 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-white mb-4">📚 指纹模板库</h2>

          <div className="space-y-3">
            {templates.map((template) => (
              <div
                key={template.id}
                onClick={() => setActiveTemplate(template.id)}
                className={`p-3 rounded-lg cursor-pointer transition-all ${
                  activeTemplate === template.id
                    ? 'bg-blue-600/30 border border-blue-500'
                    : 'bg-gray-700/50 border border-transparent hover:border-gray-600'
                }`}
              >
                <div className="flex items-center gap-2 mb-1">
                  <span className="text-xl">{getBrowserIcon(template.browser)}</span>
                  <span className="text-white font-medium">{template.browser}</span>
                  <span className="text-gray-400 text-sm">v{template.version}</span>
                </div>
                <div className="text-gray-400 text-xs">{template.os}</div>
                <div className="text-gray-500 text-xs mt-1 font-mono truncate">
                  JA4: {template.ja4.substring(0, 20)}...
                </div>
              </div>
            ))}
          </div>

          {/* 待更新模板 */}
          {pendingTemplate && (
            <div className="mt-4 p-3 bg-green-900/30 border border-green-500 rounded-lg">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-green-400 text-xs font-medium">NEW</span>
                <span className="text-xl">{getBrowserIcon(pendingTemplate.browser)}</span>
                <span className="text-white font-medium">{pendingTemplate.browser}</span>
                <span className="text-green-400 text-sm">v{pendingTemplate.version}</span>
              </div>
              <div className="text-gray-400 text-xs">{pendingTemplate.os}</div>
              <div className="text-gray-500 text-xs mt-1">
                来自 M.C.C. 推送 | {pendingTemplate.createdAt}
              </div>
            </div>
          )}
        </div>

        {/* 中间: 版本对比 & 部署 */}
        <div className="bg-gray-800 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-white mb-4">🔄 版本对比 & 部署</h2>

          {/* Diff 视图 */}
          {currentTemplate && pendingTemplate && (
            <div className="mb-4">
              <button
                onClick={() => setShowDiff(!showDiff)}
                className="text-blue-400 text-sm hover:underline mb-2"
              >
                {showDiff ? '隐藏差异' : '显示差异'} ▼
              </button>

              {showDiff && (
                <div className="bg-gray-900 rounded p-3 text-xs font-mono space-y-1">
                  <div className="text-red-400">- version: {currentTemplate.version}</div>
                  <div className="text-green-400">+ version: {pendingTemplate.version}</div>
                  <div className="text-red-400">- ja4: {currentTemplate.ja4}</div>
                  <div className="text-green-400">+ ja4: {pendingTemplate.ja4}</div>
                  <div className="text-red-400">- checksum: {currentTemplate.checksum}</div>
                  <div className="text-green-400">+ checksum: {pendingTemplate.checksum}</div>
                </div>
              )}
            </div>
          )}

          {/* 部署按钮 */}
          {pendingTemplate && (
            <button
              onClick={handleDeploy}
              disabled={isDeploying}
              className={`w-full py-3 rounded-lg font-medium transition-all ${
                isDeploying
                  ? 'bg-gray-600 cursor-not-allowed'
                  : 'bg-green-600 hover:bg-green-500'
              } text-white`}
            >
              {isDeploying ? (
                <span>部署中... {deployProgress.toFixed(0)}%</span>
              ) : (
                <span>🚀 Deploy to Cluster</span>
              )}
            </button>
          )}

          {/* 部署进度 */}
          {isDeploying && (
            <div className="mt-4">
              <div className="w-full h-2 bg-gray-700 rounded-full overflow-hidden">
                <div
                  className="h-full bg-green-500 transition-all duration-300"
                  style={{ width: `${deployProgress}%` }}
                />
              </div>
              <div className="text-gray-400 text-xs mt-2 text-center">
                同步延迟: ~105ms | 预计完成: {Math.ceil((100 - deployProgress) / 20)}s
              </div>
            </div>
          )}

          {/* 节点同步状态 */}
          <div className="mt-6">
            <h3 className="text-sm font-medium text-gray-400 mb-3">节点同步状态</h3>
            <div className="space-y-2">
              {syncStatus.map((node) => (
                <div
                  key={node.nodeId}
                  className="flex items-center justify-between p-2 bg-gray-700/50 rounded"
                >
                  <div className="flex items-center gap-2">
                    <span className={node.synced ? 'text-green-400' : 'text-yellow-400'}>
                      {node.synced ? '✓' : '○'}
                    </span>
                    <span className="text-white text-sm">{node.nodeId}</span>
                  </div>
                  <div className="flex items-center gap-3 text-xs">
                    <span className="text-gray-400">{node.latencyMs}ms</span>
                    <span className={node.synced ? 'text-green-400' : 'text-yellow-400'}>
                      v{node.version}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* 右侧: 生存预测 */}
        <div className="bg-gray-800 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-white mb-4">📊 拟态生存预测</h2>

          <div className="text-gray-400 text-xs mb-4">
            基于当前 DNA 库 ({currentTemplate?.browser} v{currentTemplate?.version})
          </div>

          <div className="space-y-3">
            {predictions.map((pred) => (
              <div key={pred.app} className="p-3 bg-gray-700/50 rounded-lg">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-white font-medium">{pred.app}</span>
                  <span
                    className="px-2 py-0.5 rounded text-xs font-medium"
                    style={{ backgroundColor: getRiskColor(pred.risk), color: '#fff' }}
                  >
                    {pred.risk.toUpperCase()}
                  </span>
                </div>

                {/* 概率条 */}
                <div className="w-full h-2 bg-gray-600 rounded-full overflow-hidden mb-1">
                  <div
                    className="h-full transition-all"
                    style={{
                      width: `${pred.probability}%`,
                      backgroundColor: getRiskColor(pred.risk),
                    }}
                  />
                </div>

                <div className="flex justify-between text-xs">
                  <span className="text-gray-400">{pred.notes}</span>
                  <span style={{ color: getRiskColor(pred.risk) }}>
                    {pred.probability.toFixed(1)}%
                  </span>
                </div>
              </div>
            ))}
          </div>

          {/* 总体评估 */}
          <div className="mt-4 p-3 bg-gray-700/30 rounded-lg">
            <div className="text-sm text-gray-400 mb-2">总体生存评估</div>
            <div className="flex items-center gap-3">
              <div className="text-3xl font-bold text-green-400">
                {(predictions.reduce((sum, p) => sum + p.probability, 0) / predictions.length).toFixed(1)}%
              </div>
              <div className="text-gray-400 text-xs">
                平均生存率<br />
                基于 {predictions.length} 个主流平台
              </div>
            </div>
          </div>

          {/* 建议 */}
          <div className="mt-4 p-3 bg-yellow-900/20 border border-yellow-500/50 rounded-lg">
            <div className="text-yellow-400 text-sm font-medium mb-1">💡 优化建议</div>
            <div className="text-gray-400 text-xs">
              Cloudflare 检测风险较高，建议启用 VPC 噪声注入协议增加流量随机性。
            </div>
          </div>
        </div>
      </div>

      {/* 底部: 当前模板详情 */}
      {currentTemplate && (
        <div className="mt-6 bg-gray-800 rounded-lg p-6">
          <h2 className="text-lg font-semibold text-white mb-4">
            📋 当前活跃模板: {currentTemplate.name}
          </h2>

          <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-4">
            <div className="p-3 bg-gray-700/50 rounded">
              <div className="text-gray-400 text-xs mb-1">Browser</div>
              <div className="text-white font-medium">{currentTemplate.browser}</div>
            </div>
            <div className="p-3 bg-gray-700/50 rounded">
              <div className="text-gray-400 text-xs mb-1">Version</div>
              <div className="text-white font-medium">{currentTemplate.version}</div>
            </div>
            <div className="p-3 bg-gray-700/50 rounded">
              <div className="text-gray-400 text-xs mb-1">OS</div>
              <div className="text-white font-medium">{currentTemplate.os}</div>
            </div>
            <div className="p-3 bg-gray-700/50 rounded">
              <div className="text-gray-400 text-xs mb-1">Window Size</div>
              <div className="text-white font-medium">{currentTemplate.windowSize}</div>
            </div>
            <div className="p-3 bg-gray-700/50 rounded">
              <div className="text-gray-400 text-xs mb-1">TTL</div>
              <div className="text-white font-medium">{currentTemplate.ttl}</div>
            </div>
            <div className="p-3 bg-gray-700/50 rounded">
              <div className="text-gray-400 text-xs mb-1">MSS</div>
              <div className="text-white font-medium">{currentTemplate.mss}</div>
            </div>
          </div>

          <div className="mt-4 p-3 bg-gray-900 rounded font-mono text-xs text-gray-400">
            JA4 Fingerprint: {currentTemplate.ja4}
          </div>
        </div>
      )}
    </div>
  );
};

export default DnaLab;
