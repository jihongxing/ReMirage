import React, { useState, useEffect } from 'react';

// 劫持样本
interface HijackSample {
  id: string;
  timestamp: number;
  type: 'html_injection' | 'redirect' | 'rst_flood' | 'content_modify';
  sourceIP: string;
  targetDomain: string;
  entropyDelta: number;
  rawSignature: string;
  region: string;
}

// RST 行为记录
interface RSTRecord {
  timestamp: number;
  count: number;
  sourcePort: number;
  pattern: 'burst' | 'periodic' | 'random';
  fingerprint: string;
}

// 战损报告
interface ForensicReport {
  id: string;
  generatedAt: number;
  testDuration: number;
  jammingLevel: number;
  summary: {
    totalHijacks: number;
    totalRST: number;
    totalSwitches: number;
    escapeLatencyMs: number;
    finalReputation: number;
  };
  hijackSamples: HijackSample[];
  rstRecords: RSTRecord[];
  switchHistory: Array<{
    from: string;
    to: string;
    reason: string;
    timestamp: number;
  }>;
  threatIntel: {
    injectionPatterns: string[];
    attackerFingerprints: string[];
    recommendedActions: string[];
  };
}

// 模拟数据生成
const generateMockReport = (): ForensicReport => {
  const now = Date.now();
  return {
    id: `FR-${now}`,
    generatedAt: now,
    testDuration: 15000,
    jammingLevel: 3,
    summary: {
      totalHijacks: 37,
      totalRST: 260,
      totalSwitches: 3,
      escapeLatencyMs: 0,
      finalReputation: 20.4,
    },
    hijackSamples: Array.from({ length: 5 }, (_, i) => ({
      id: `HJ-${i}`,
      timestamp: now - Math.random() * 15000,
      type: ['html_injection', 'redirect', 'content_modify'][i % 3] as any,
      sourceIP: `10.0.${Math.floor(Math.random() * 255)}.${Math.floor(Math.random() * 255)}`,
      targetDomain: `shadow-${(i % 3) + 1}.cdn.example.com`,
      entropyDelta: 2.1 + Math.random() * 1.5,
      rawSignature: `<html><body>BLOCKED-${i}</body></html>`.slice(0, 32),
      region: ['CN-BJ', 'CN-SH', 'RU-MOW'][i % 3],
    })),
    rstRecords: Array.from({ length: 3 }, (_, i) => ({
      timestamp: now - Math.random() * 15000,
      count: 80 + Math.floor(Math.random() * 40),
      sourcePort: 443,
      pattern: ['burst', 'periodic', 'random'][i % 3] as any,
      fingerprint: `RST-FP-${Math.random().toString(36).slice(2, 10)}`,
    })),
    switchHistory: [
      { from: 'cdn-test.example.com', to: 'shadow-1.cdn.example.com', reason: 'JA4_FINGERPRINT', timestamp: now - 12000 },
      { from: 'shadow-1.cdn.example.com', to: 'shadow-2.cdn.example.com', reason: 'HTTP_ERROR', timestamp: now - 8000 },
      { from: 'shadow-2.cdn.example.com', to: 'shadow-3.cdn.example.com', reason: 'HTTP_ERROR', timestamp: now - 4000 },
    ],
    threatIntel: {
      injectionPatterns: ['HTML Block Page (GFW-v3)', 'HTTP 302 Redirect', 'TCP RST Burst'],
      attackerFingerprints: ['DPI-CN-2024', 'RST-Injector-v2'],
      recommendedActions: ['启用 B-DNA Reset', '增加影子域名池深度', '切换至备用 IP 段'],
    },
  };
};

const ForensicArchive: React.FC = () => {
  const [reports, setReports] = useState<ForensicReport[]>([]);
  const [selectedReport, setSelectedReport] = useState<ForensicReport | null>(null);
  const [activeTab, setActiveTab] = useState<'summary' | 'hijacks' | 'rst' | 'intel'>('summary');

  useEffect(() => {
    // 加载历史报告
    const mockReports = [generateMockReport()];
    setReports(mockReports);
    if (mockReports.length > 0) {
      setSelectedReport(mockReports[0]);
    }
  }, []);

  const formatTime = (ts: number) => new Date(ts).toLocaleString();

  const exportReport = () => {
    if (!selectedReport) return;
    const blob = new Blob([JSON.stringify(selectedReport, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `forensic-${selectedReport.id}.json`;
    a.click();
  };

  if (!selectedReport) {
    return <div className="p-6 text-gray-400">暂无战损报告</div>;
  }

  return (
    <div className="bg-gray-900 rounded-lg p-6">
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          <span className="text-2xl">🔬</span> 战损取证归档
        </h2>
        <div className="flex gap-2">
          <select
            className="bg-gray-800 text-white px-3 py-1 rounded"
            value={selectedReport.id}
            onChange={(e) => setSelectedReport(reports.find(r => r.id === e.target.value) || null)}
          >
            {reports.map(r => (
              <option key={r.id} value={r.id}>{r.id} - {formatTime(r.generatedAt)}</option>
            ))}
          </select>
          <button
            onClick={exportReport}
            className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-1 rounded"
          >
            导出 JSON
          </button>
        </div>
      </div>

      {/* Tab 导航 */}
      <div className="flex gap-1 mb-4 border-b border-gray-700">
        {[
          { key: 'summary', label: '📊 概览' },
          { key: 'hijacks', label: '🎭 劫持样本' },
          { key: 'rst', label: '⚡ RST 分析' },
          { key: 'intel', label: '🧠 威胁情报' },
        ].map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key as any)}
            className={`px-4 py-2 ${activeTab === tab.key ? 'bg-gray-800 text-white border-b-2 border-blue-500' : 'text-gray-400 hover:text-white'}`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* 概览 */}
      {activeTab === 'summary' && (
        <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
          <StatCard label="测试时长" value={`${selectedReport.testDuration / 1000}s`} />
          <StatCard label="干扰等级" value={`Level ${selectedReport.jammingLevel}`} color="red" />
          <StatCard label="逃逸延迟" value={`${selectedReport.summary.escapeLatencyMs}ms`} color="green" />
          <StatCard label="劫持次数" value={selectedReport.summary.totalHijacks} color="yellow" />
          <StatCard label="RST 注入" value={selectedReport.summary.totalRST} color="red" />
          <StatCard label="域名切换" value={selectedReport.summary.totalSwitches} color="blue" />
          <div className="col-span-full">
            <h3 className="text-gray-400 mb-2">切换历史</h3>
            <div className="space-y-2">
              {selectedReport.switchHistory.map((sw, i) => (
                <div key={i} className="bg-gray-800 p-3 rounded flex justify-between items-center">
                  <span className="text-gray-300">{sw.from} → <span className="text-green-400">{sw.to}</span></span>
                  <span className={`px-2 py-1 rounded text-xs ${sw.reason === 'JA4_FINGERPRINT' ? 'bg-red-900 text-red-300' : 'bg-yellow-900 text-yellow-300'}`}>
                    {sw.reason}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* 劫持样本 */}
      {activeTab === 'hijacks' && (
        <div className="space-y-3">
          {selectedReport.hijackSamples.map(sample => (
            <div key={sample.id} className="bg-gray-800 p-4 rounded">
              <div className="flex justify-between items-start mb-2">
                <span className={`px-2 py-1 rounded text-xs ${
                  sample.type === 'html_injection' ? 'bg-red-900 text-red-300' :
                  sample.type === 'redirect' ? 'bg-yellow-900 text-yellow-300' :
                  'bg-purple-900 text-purple-300'
                }`}>
                  {sample.type}
                </span>
                <span className="text-gray-500 text-sm">{formatTime(sample.timestamp)}</span>
              </div>
              <div className="grid grid-cols-2 gap-2 text-sm">
                <div><span className="text-gray-500">来源 IP:</span> <span className="text-white">{sample.sourceIP}</span></div>
                <div><span className="text-gray-500">目标域名:</span> <span className="text-white">{sample.targetDomain}</span></div>
                <div><span className="text-gray-500">熵偏移:</span> <span className="text-red-400">+{sample.entropyDelta.toFixed(2)}</span></div>
                <div><span className="text-gray-500">区域:</span> <span className="text-white">{sample.region}</span></div>
              </div>
              <div className="mt-2 bg-gray-900 p-2 rounded font-mono text-xs text-gray-400 overflow-x-auto">
                {sample.rawSignature}...
              </div>
            </div>
          ))}
        </div>
      )}

      {/* RST 分析 */}
      {activeTab === 'rst' && (
        <div className="space-y-3">
          <div className="bg-gray-800 p-4 rounded">
            <h3 className="text-white mb-3">RST 注入模式分析</h3>
            <div className="h-32 bg-gray-900 rounded flex items-end justify-around p-2">
              {selectedReport.rstRecords.map((rec, i) => (
                <div key={i} className="flex flex-col items-center">
                  <div
                    className="w-16 bg-red-600 rounded-t"
                    style={{ height: `${rec.count / 3}px` }}
                  />
                  <span className="text-xs text-gray-400 mt-1">{rec.pattern}</span>
                  <span className="text-xs text-white">{rec.count}</span>
                </div>
              ))}
            </div>
          </div>
          {selectedReport.rstRecords.map((rec, i) => (
            <div key={i} className="bg-gray-800 p-3 rounded flex justify-between items-center">
              <div>
                <span className={`px-2 py-1 rounded text-xs mr-2 ${
                  rec.pattern === 'burst' ? 'bg-red-900 text-red-300' :
                  rec.pattern === 'periodic' ? 'bg-yellow-900 text-yellow-300' :
                  'bg-gray-700 text-gray-300'
                }`}>
                  {rec.pattern}
                </span>
                <span className="text-gray-400">Port {rec.sourcePort}</span>
              </div>
              <div className="text-right">
                <div className="text-white">{rec.count} RST</div>
                <div className="text-xs text-gray-500 font-mono">{rec.fingerprint}</div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* 威胁情报 */}
      {activeTab === 'intel' && (
        <div className="space-y-4">
          <IntelSection title="🎯 注入模式" items={selectedReport.threatIntel.injectionPatterns} color="red" />
          <IntelSection title="🔍 攻击者指纹" items={selectedReport.threatIntel.attackerFingerprints} color="yellow" />
          <IntelSection title="✅ 建议操作" items={selectedReport.threatIntel.recommendedActions} color="green" />
        </div>
      )}
    </div>
  );
};

// 统计卡片
const StatCard: React.FC<{ label: string; value: string | number; color?: string }> = ({ label, value, color = 'gray' }) => {
  const colorMap: Record<string, string> = {
    gray: 'text-white',
    red: 'text-red-400',
    green: 'text-green-400',
    yellow: 'text-yellow-400',
    blue: 'text-blue-400',
  };
  return (
    <div className="bg-gray-800 p-4 rounded">
      <div className="text-gray-400 text-sm">{label}</div>
      <div className={`text-2xl font-bold ${colorMap[color]}`}>{value}</div>
    </div>
  );
};

// 情报区块
const IntelSection: React.FC<{ title: string; items: string[]; color: string }> = ({ title, items, color }) => {
  const bgMap: Record<string, string> = {
    red: 'bg-red-900/30 border-red-800',
    yellow: 'bg-yellow-900/30 border-yellow-800',
    green: 'bg-green-900/30 border-green-800',
  };
  return (
    <div className={`p-4 rounded border ${bgMap[color]}`}>
      <h3 className="text-white mb-2">{title}</h3>
      <ul className="space-y-1">
        {items.map((item, i) => (
          <li key={i} className="text-gray-300 text-sm">• {item}</li>
        ))}
      </ul>
    </div>
  );
};

export default ForensicArchive;
