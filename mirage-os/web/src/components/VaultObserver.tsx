// 资产保险箱监控 - 物理安全具象化
import { useState, useEffect, useCallback } from 'react';

interface IPReputationBucket {
  range: string;
  count: number;
  percentage: number;
}

interface WipeStep {
  id: string;
  name: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  progress: number;
}

interface HardwareKeyStatus {
  source: 'hardware' | 'tpm' | 'failsafe';
  entropy: number;
  mounted: boolean;
  locked: boolean;
  timestamp: number;
}

interface VaultStats {
  ipCacheCount: number;
  ipCacheMax: number;
  dnaCacheCount: number;
  fileSizeBytes: number;
  cacheHitRate: number;
  evictCount: number;
}

export default function VaultObserver() {
  const [ipDistribution, setIpDistribution] = useState<IPReputationBucket[]>([]);
  const [vaultStats, setVaultStats] = useState<VaultStats | null>(null);
  const [hardwareKey, setHardwareKey] = useState<HardwareKeyStatus | null>(null);
  const [wipeSteps, setWipeSteps] = useState<WipeStep[]>([]);
  const [isWiping, setIsWiping] = useState(false);
  const [wipeConfirmCount, setWipeConfirmCount] = useState(0);

  useEffect(() => {
    // 模拟 IP 信誉分布数据
    const mockDistribution: IPReputationBucket[] = [
      { range: '90-100', count: 35420, percentage: 35.4 },
      { range: '70-89', count: 28650, percentage: 28.7 },
      { range: '50-69', count: 18230, percentage: 18.2 },
      { range: '30-49', count: 12100, percentage: 12.1 },
      { range: '0-29', count: 5600, percentage: 5.6 },
    ];

    const mockStats: VaultStats = {
      ipCacheCount: 100000,
      ipCacheMax: 100000,
      dnaCacheCount: 156,
      fileSizeBytes: 52428800,
      cacheHitRate: 100.0,
      evictCount: 1200,
    };

    const mockHardwareKey: HardwareKeyStatus = {
      source: 'hardware',
      entropy: 128,
      mounted: true,
      locked: false,
      timestamp: Date.now() - 86400000,
    };

    setIpDistribution(mockDistribution);
    setVaultStats(mockStats);
    setHardwareKey(mockHardwareKey);
  }, []);

  // 触发自毁流程
  const triggerWipe = useCallback(() => {
    if (wipeConfirmCount < 2) {
      setWipeConfirmCount(prev => prev + 1);
      return;
    }

    setIsWiping(true);
    setWipeSteps([
      { id: 'memory', name: '内存清零', status: 'pending', progress: 0 },
      { id: 'gc', name: 'GC 回收', status: 'pending', progress: 0 },
      { id: 'db_state', name: '状态标记', status: 'pending', progress: 0 },
      { id: 'db_close', name: '关闭数据库', status: 'pending', progress: 0 },
      { id: 'overwrite1', name: '覆盖 (0x00)', status: 'pending', progress: 0 },
      { id: 'overwrite2', name: '覆盖 (0xFF)', status: 'pending', progress: 0 },
      { id: 'overwrite3', name: '覆盖 (0xAA)', status: 'pending', progress: 0 },
      { id: 'delete', name: '文件删除', status: 'pending', progress: 0 },
      { id: 'key_wipe', name: '密钥擦除', status: 'pending', progress: 0 },
    ]);

    // 模拟擦除进度
    let currentStep = 0;
    const interval = setInterval(() => {
      setWipeSteps(prev => {
        const updated = [...prev];
        if (currentStep < updated.length) {
          if (updated[currentStep].progress < 100) {
            updated[currentStep].status = 'running';
            updated[currentStep].progress = Math.min(updated[currentStep].progress + 20, 100);
          } else {
            updated[currentStep].status = 'completed';
            currentStep++;
          }
        }
        if (currentStep >= updated.length) {
          clearInterval(interval);
        }
        return updated;
      });
    }, 200);
  }, [wipeConfirmCount]);

  const getDistributionColor = (range: string) => {
    if (range.startsWith('90')) return 'bg-green-500';
    if (range.startsWith('70')) return 'bg-cyan-500';
    if (range.startsWith('50')) return 'bg-yellow-500';
    if (range.startsWith('30')) return 'bg-orange-500';
    return 'bg-red-500';
  };

  const getKeySourceLabel = (source: string) => {
    switch (source) {
      case 'hardware': return '硬件指纹';
      case 'tpm': return 'TPM 模块';
      case 'failsafe': return 'Fail-Safe';
      default: return '未知';
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  };

  return (
    <div className="bg-gray-900 rounded-lg p-6 space-y-6">
      {/* 标题 */}
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          🔐 资产保险箱监控
        </h2>
        <div className="flex items-center gap-2">
          {hardwareKey?.mounted && (
            <span className="px-2 py-1 bg-green-500/20 text-green-400 text-xs rounded flex items-center gap-1">
              <span className="w-2 h-2 bg-green-500 rounded-full" />
              已挂载
            </span>
          )}
          {hardwareKey?.locked && (
            <span className="px-2 py-1 bg-red-500/20 text-red-400 text-xs rounded">
              🔒 已锁定
            </span>
          )}
        </div>
      </div>

      {/* 硬件指纹锁定状态 */}
      {hardwareKey && (
        <div className="bg-gray-800 rounded-lg p-4">
          <h3 className="text-white font-medium mb-4 flex items-center gap-2">
            🔑 硬件密钥状态
          </h3>
          <div className="grid grid-cols-4 gap-4">
            <div className="bg-gray-900 rounded-lg p-3 text-center">
              <div className="text-cyan-400 text-lg font-bold">
                {getKeySourceLabel(hardwareKey.source)}
              </div>
              <div className="text-xs text-gray-500">密钥来源</div>
            </div>
            <div className="bg-gray-900 rounded-lg p-3 text-center">
              <div className="text-green-400 text-lg font-bold">
                {hardwareKey.entropy} bits
              </div>
              <div className="text-xs text-gray-500">熵值</div>
            </div>
            <div className="bg-gray-900 rounded-lg p-3 text-center">
              <div className={`text-lg font-bold ${hardwareKey.mounted ? 'text-green-400' : 'text-red-400'}`}>
                {hardwareKey.mounted ? '已挂载' : '未挂载'}
              </div>
              <div className="text-xs text-gray-500">挂载状态</div>
            </div>
            <div className="bg-gray-900 rounded-lg p-3 text-center">
              <div className={`text-lg font-bold ${hardwareKey.locked ? 'text-red-400' : 'text-green-400'}`}>
                {hardwareKey.locked ? '🔒 锁定' : '🔓 解锁'}
              </div>
              <div className="text-xs text-gray-500">锁定状态</div>
            </div>
          </div>
        </div>
      )}

      {/* LRU 资产热力图 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          🌡️ IP 信誉分布热力图
          <span className="text-xs text-gray-500">
            ({vaultStats?.ipCacheCount.toLocaleString()} / {vaultStats?.ipCacheMax.toLocaleString()})
          </span>
        </h3>
        
        {/* 热力条 */}
        <div className="flex h-8 rounded overflow-hidden mb-4">
          {ipDistribution.map((bucket, idx) => (
            <div
              key={idx}
              className={`${getDistributionColor(bucket.range)} transition-all duration-300`}
              style={{ width: `${bucket.percentage}%` }}
              title={`${bucket.range}: ${bucket.count.toLocaleString()} (${bucket.percentage}%)`}
            />
          ))}
        </div>

        {/* 图例 */}
        <div className="grid grid-cols-5 gap-2">
          {ipDistribution.map((bucket, idx) => (
            <div key={idx} className="bg-gray-900 rounded p-2 text-center">
              <div className={`w-full h-2 rounded mb-2 ${getDistributionColor(bucket.range)}`} />
              <div className="text-white text-sm font-bold">{bucket.count.toLocaleString()}</div>
              <div className="text-xs text-gray-500">{bucket.range} 分</div>
            </div>
          ))}
        </div>

        {/* LRU 淘汰统计 */}
        <div className="mt-4 flex items-center justify-between text-sm">
          <span className="text-gray-400">LRU 淘汰: {vaultStats?.evictCount.toLocaleString()} 条</span>
          <span className="text-gray-400">缓存命中率: {vaultStats?.cacheHitRate.toFixed(1)}%</span>
        </div>
      </div>

      {/* 存储统计 */}
      {vaultStats && (
        <div className="grid grid-cols-4 gap-4">
          <div className="bg-gray-800 rounded-lg p-4 text-center">
            <div className="text-2xl font-bold text-cyan-400">
              {vaultStats.ipCacheCount.toLocaleString()}
            </div>
            <div className="text-xs text-gray-400">IP 记录</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4 text-center">
            <div className="text-2xl font-bold text-green-400">
              {vaultStats.dnaCacheCount}
            </div>
            <div className="text-xs text-gray-400">DNA 模板</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4 text-center">
            <div className="text-2xl font-bold text-yellow-400">
              {formatBytes(vaultStats.fileSizeBytes)}
            </div>
            <div className="text-xs text-gray-400">磁盘占用</div>
          </div>
          <div className="bg-gray-800 rounded-lg p-4 text-center">
            <div className="text-2xl font-bold text-green-400">
              {vaultStats.cacheHitRate.toFixed(0)}%
            </div>
            <div className="text-xs text-gray-400">命中率</div>
          </div>
        </div>
      )}

      {/* 物理擦除控制 */}
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-white font-medium mb-4 flex items-center gap-2">
          💀 物理擦除控制
          <span className="text-xs text-red-400">(不可逆操作)</span>
        </h3>

        {!isWiping ? (
          <div className="space-y-4">
            <div className="text-sm text-gray-400">
              触发物理擦除将执行以下操作：
            </div>
            <ul className="text-sm text-gray-500 space-y-1 ml-4">
              <li>• 清空内存中所有 IP 信誉和 DNA 模板</li>
              <li>• 强制 GC 回收敏感数据</li>
              <li>• 三次覆盖 BoltDB 文件 (0x00, 0xFF, 0xAA)</li>
              <li>• 删除数据库文件</li>
              <li>• 擦除内存中的加密密钥</li>
            </ul>

            <button
              onClick={triggerWipe}
              className={`w-full py-3 rounded-lg font-medium transition-all ${
                wipeConfirmCount === 0
                  ? 'bg-red-600 hover:bg-red-500 text-white'
                  : wipeConfirmCount === 1
                  ? 'bg-red-700 hover:bg-red-600 text-white animate-pulse'
                  : 'bg-red-800 text-white animate-pulse'
              }`}
            >
              {wipeConfirmCount === 0
                ? '⚠️ 触发物理擦除'
                : wipeConfirmCount === 1
                ? '⚠️ 再次确认 (1/2)'
                : '💀 最终确认 - 执行擦除'}
            </button>

            {wipeConfirmCount > 0 && (
              <button
                onClick={() => setWipeConfirmCount(0)}
                className="w-full py-2 bg-gray-700 hover:bg-gray-600 text-gray-300 rounded-lg text-sm"
              >
                取消
              </button>
            )}
          </div>
        ) : (
          <div className="space-y-3">
            {wipeSteps.map(step => (
              <div key={step.id} className="flex items-center gap-4">
                <div className="w-32 text-sm">
                  <span className={`${
                    step.status === 'completed' ? 'text-green-400' :
                    step.status === 'running' ? 'text-cyan-400' :
                    step.status === 'failed' ? 'text-red-400' : 'text-gray-500'
                  }`}>
                    {step.status === 'completed' ? '✓' :
                     step.status === 'running' ? '⟳' :
                     step.status === 'failed' ? '✗' : '○'} {step.name}
                  </span>
                </div>
                <div className="flex-1">
                  <div className="w-full bg-gray-700 rounded-full h-2">
                    <div
                      className={`h-2 rounded-full transition-all duration-200 ${
                        step.status === 'completed' ? 'bg-green-500' :
                        step.status === 'running' ? 'bg-cyan-500' :
                        step.status === 'failed' ? 'bg-red-500' : 'bg-gray-600'
                      }`}
                      style={{ width: `${step.progress}%` }}
                    />
                  </div>
                </div>
                <div className="w-12 text-right text-sm text-gray-400">
                  {step.progress}%
                </div>
              </div>
            ))}

            {wipeSteps.every(s => s.status === 'completed') && (
              <div className="mt-4 p-4 bg-green-500/20 border border-green-500 rounded-lg text-center">
                <div className="text-green-400 font-bold">✓ 物理擦除完成</div>
                <div className="text-sm text-gray-400 mt-1">所有敏感数据已安全销毁</div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Fail-Safe 警告 */}
      {hardwareKey?.source === 'failsafe' && (
        <div className="bg-red-500/20 border border-red-500 rounded-lg p-4 flex items-center gap-3">
          <span className="text-2xl">🚨</span>
          <div>
            <div className="text-red-400 font-medium">Fail-Safe 模式激活</div>
            <div className="text-sm text-gray-400">
              硬件指纹不足，系统已锁定。无法读写加密数据。
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
