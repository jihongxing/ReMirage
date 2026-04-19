import React, { useState, useEffect } from 'react';

// 节点同步状态
interface NodeSyncStatus {
  nodeId: string;
  region: string;
  currentDomain: string;
  lastSyncTime: number;
  syncOffsetMs: number;
  isReady: boolean;
  isCommitted: boolean;
  needResync: boolean;
  failCount: number;
  latencyMs: number;
}

// 同步事件
interface SyncEvent {
  id: string;
  type: 'gswitch' | 'bdna_reset' | 'resync';
  timestamp: number;
  phase: 'precommit' | 'commit' | 'rollback' | 'complete';
  affectedNodes: string[];
  successCount: number;
  failCount: number;
}

// 模拟数据
const generateMockNodes = (): NodeSyncStatus[] => [
  { nodeId: 'node-sg-0', region: 'sg', currentDomain: 'shadow-1.cdn.example.com', lastSyncTime: Date.now() - 1000, syncOffsetMs: 12, isReady: true, isCommitted: true, needResync: false, failCount: 0, latencyMs: 30 },
  { nodeId: 'node-de-1', region: 'de', currentDomain: 'shadow-1.cdn.example.com', lastSyncTime: Date.now() - 2000, syncOffsetMs: 85, isReady: true, isCommitted: true, needResync: false, failCount: 0, latencyMs: 150 },
  { nodeId: 'node-us-2', region: 'us', currentDomain: 'shadow-1.cdn.example.com', lastSyncTime: Date.now() - 3000, syncOffsetMs: 142, isReady: true, isCommitted: true, needResync: false, failCount: 0, latencyMs: 200 },
  { nodeId: 'node-jp-3', region: 'jp', currentDomain: 'cdn-test.example.com', lastSyncTime: Date.now() - 60000, syncOffsetMs: -1, isReady: false, isCommitted: false, needResync: true, failCount: 2, latencyMs: 50 },
  { nodeId: 'node-ch-4', region: 'ch', currentDomain: 'shadow-1.cdn.example.com', lastSyncTime: Date.now() - 1500, syncOffsetMs: 98, isReady: true, isCommitted: true, needResync: false, failCount: 0, latencyMs: 180 },
];

const generateMockEvents = (): SyncEvent[] => [
  { id: 'SE-1', type: 'gswitch', timestamp: Date.now() - 5000, phase: 'complete', affectedNodes: ['node-sg-0', 'node-de-1', 'node-us-2', 'node-ch-4'], successCount: 4, failCount: 1 },
  { id: 'SE-2', type: 'resync', timestamp: Date.now() - 2000, phase: 'complete', affectedNodes: ['node-jp-3'], successCount: 0, failCount: 1 },
];

const GlobalAuditTrail: React.FC = () => {
  const [nodes, setNodes] = useState<NodeSyncStatus[]>([]);
  const [events, setEvents] = useState<SyncEvent[]>([]);
  const [selectedNode, setSelectedNode] = useState<string | null>(null);

  useEffect(() => {
    setNodes(generateMockNodes());
    setEvents(generateMockEvents());

    // 模拟实时更新
    const interval = setInterval(() => {
      setNodes(prev => prev.map(n => ({
        ...n,
        syncOffsetMs: n.needResync ? -1 : n.syncOffsetMs + Math.random() * 5 - 2.5,
        latencyMs: n.latencyMs + Math.random() * 10 - 5,
      })));
    }, 2000);

    return () => clearInterval(interval);
  }, []);

  const getStatusColor = (node: NodeSyncStatus) => {
    if (node.needResync) return 'bg-yellow-500';
    if (!node.isCommitted) return 'bg-red-500';
    if (node.syncOffsetMs > 100) return 'bg-orange-500';
    return 'bg-green-500';
  };

  const getStatusText = (node: NodeSyncStatus) => {
    if (node.needResync) return '🔄 重同步中...';
    if (!node.isCommitted) return '❌ 失步';
    if (node.syncOffsetMs > 100) return '⚠️ 延迟';
    return '✅ 同步';
  };

  const triggerResync = (nodeId: string) => {
    setNodes(prev => prev.map(n => 
      n.nodeId === nodeId ? { ...n, needResync: true } : n
    ));
    // 模拟重同步
    setTimeout(() => {
      setNodes(prev => prev.map(n => 
        n.nodeId === nodeId ? { 
          ...n, 
          needResync: false, 
          isCommitted: true, 
          currentDomain: 'shadow-1.cdn.example.com',
          syncOffsetMs: 50,
          lastSyncTime: Date.now(),
        } : n
      ));
    }, 3000);
  };

  return (
    <div className="bg-gray-900 rounded-lg p-6">
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          <span className="text-2xl">🌐</span> 分布式同步监控
        </h2>
        <div className="flex gap-2 text-sm">
          <span className="px-2 py-1 bg-green-900 text-green-300 rounded">同步: {nodes.filter(n => n.isCommitted && !n.needResync).length}</span>
          <span className="px-2 py-1 bg-yellow-900 text-yellow-300 rounded">重同步: {nodes.filter(n => n.needResync).length}</span>
          <span className="px-2 py-1 bg-red-900 text-red-300 rounded">失步: {nodes.filter(n => !n.isCommitted && !n.needResync).length}</span>
        </div>
      </div>

      {/* 节点状态网格 */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
        {nodes.map(node => (
          <div
            key={node.nodeId}
            className={`bg-gray-800 p-4 rounded-lg border-l-4 cursor-pointer transition-all ${
              node.needResync ? 'border-yellow-500 animate-pulse' :
              !node.isCommitted ? 'border-red-500' :
              node.syncOffsetMs > 100 ? 'border-orange-500' :
              'border-green-500'
            } ${selectedNode === node.nodeId ? 'ring-2 ring-blue-500' : ''}`}
            onClick={() => setSelectedNode(node.nodeId === selectedNode ? null : node.nodeId)}
          >
            <div className="flex justify-between items-start mb-2">
              <div>
                <div className="text-white font-medium">{node.nodeId}</div>
                <div className="text-gray-400 text-sm">{node.region.toUpperCase()}</div>
              </div>
              <div className={`w-3 h-3 rounded-full ${getStatusColor(node)}`} />
            </div>

            {/* 同步偏移量进度条 */}
            <div className="mb-2">
              <div className="flex justify-between text-xs text-gray-400 mb-1">
                <span>同步偏移</span>
                <span>{node.syncOffsetMs >= 0 ? `${node.syncOffsetMs.toFixed(0)}ms` : 'N/A'}</span>
              </div>
              <div className="h-2 bg-gray-700 rounded-full overflow-hidden">
                <div
                  className={`h-full transition-all ${
                    node.syncOffsetMs < 0 ? 'bg-red-500' :
                    node.syncOffsetMs < 50 ? 'bg-green-500' :
                    node.syncOffsetMs < 100 ? 'bg-yellow-500' :
                    'bg-orange-500'
                  }`}
                  style={{ width: `${Math.min(100, Math.max(0, node.syncOffsetMs))}%` }}
                />
              </div>
            </div>

            <div className="flex justify-between items-center">
              <span className="text-sm">{getStatusText(node)}</span>
              {(node.needResync || !node.isCommitted) && (
                <button
                  onClick={(e) => { e.stopPropagation(); triggerResync(node.nodeId); }}
                  className="text-xs bg-blue-600 hover:bg-blue-700 text-white px-2 py-1 rounded"
                >
                  强制同步
                </button>
              )}
            </div>
          </div>
        ))}
      </div>

      {/* 选中节点详情 */}
      {selectedNode && (
        <div className="bg-gray-800 p-4 rounded-lg mb-6">
          <h3 className="text-white font-medium mb-3">节点详情: {selectedNode}</h3>
          {(() => {
            const node = nodes.find(n => n.nodeId === selectedNode);
            if (!node) return null;
            return (
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                <div><span className="text-gray-400">当前域名:</span> <span className="text-white">{node.currentDomain}</span></div>
                <div><span className="text-gray-400">网络延迟:</span> <span className="text-white">{node.latencyMs.toFixed(0)}ms</span></div>
                <div><span className="text-gray-400">失败次数:</span> <span className={node.failCount > 0 ? 'text-red-400' : 'text-white'}>{node.failCount}</span></div>
                <div><span className="text-gray-400">最后同步:</span> <span className="text-white">{new Date(node.lastSyncTime).toLocaleTimeString()}</span></div>
              </div>
            );
          })()}
        </div>
      )}

      {/* 同步事件日志 */}
      <div className="bg-gray-800 p-4 rounded-lg">
        <h3 className="text-white font-medium mb-3">同步事件日志</h3>
        <div className="space-y-2 max-h-48 overflow-y-auto">
          {events.map(event => (
            <div key={event.id} className="flex items-center gap-3 text-sm p-2 bg-gray-900 rounded">
              <span className={`px-2 py-1 rounded text-xs ${
                event.type === 'gswitch' ? 'bg-blue-900 text-blue-300' :
                event.type === 'resync' ? 'bg-yellow-900 text-yellow-300' :
                'bg-purple-900 text-purple-300'
              }`}>
                {event.type.toUpperCase()}
              </span>
              <span className="text-gray-400">{new Date(event.timestamp).toLocaleTimeString()}</span>
              <span className="text-white flex-1">{event.affectedNodes.length} 节点</span>
              <span className="text-green-400">{event.successCount} ✓</span>
              <span className="text-red-400">{event.failCount} ✗</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

export default GlobalAuditTrail;
