// 资产保险箱管理界面 - Vault 可视化
import { useState, useEffect } from 'react';
import { useMirageSocket } from '../hooks/useMirageSocket';

interface VaultStats {
  fileSize: number;
  dnaCount: number;
  ipCount: number;
  sysCount: number;
  locked: boolean;
  state: 'RUNNING' | 'SHUTDOWN' | 'DEAD';
  lastBackup: number;
  hardwareKeyLoaded: boolean;
}

interface DNARecord {
  profileId: string;
  browser: string;
  version: string;
  os: string;
  ja4: string;
  checksum: string;
  updatedAt: number;
}

interface IPRecord {
  ip: string;
  latency: number;
  reputationScore: number;
  region: string;
  lastSeen: number;
}

const VaultManager = () => {
  const { sendCommand } = useMirageSocket();
  const [stats, setStats] = useState<VaultStats | null>(null);
  const [dnaRecords, setDnaRecords] = useState<DNARecord[]>([]);
  const [ipRecords, setIpRecords] = useState<IPRecord[]>([]);
  const [activeTab, setActiveTab] = useState<'overview' | 'dna' | 'ip' | 'system'>('overview');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadVaultData();
    const interval = setInterval(loadVaultData, 30000);
    return () => clearInterval(interval);
  }, []);

  const loadVaultData = () => {
    // 模拟数据
    setStats({
      fileSize: 2.4 * 1024 * 1024,
      dnaCount: 12,
      ipCount: 847,
      sysCount: 5,
      locked: false,
      state: 'RUNNING',
      lastBackup: Date.now() - 3600000,
      hardwareKeyLoaded: true,
    });

    setDnaRecords([
      { profileId: 'chrome-130-win11', browser: 'Chrome', version: '130.0', os: 'Windows 11', ja4: 't13d1516h2_8daaf6152771', checksum: 'a1b2c3d4', updatedAt: Date.now() - 86400000 },
      { profileId: 'firefox-125-linux', browser: 'Firefox', version: '125.0', os: 'Linux', ja4: 't13d1715h2_5b57614c22b0', checksum: 'e5f6g7h8', updatedAt: Date.now() - 172800000 },
      { profileId: 'safari-17-macos', browser: 'Safari', version: '17.4', os: 'macOS', ja4: 't13d1517h2_8daaf6152771', checksum: 'i9j0k1l2', updatedAt: Date.now() - 259200000 },
    ]);

    setIpRecords([
      { ip: '104.16.xxx.xxx', latency: 12.5, reputationScore: 98, region: 'US-West', lastSeen: Date.now() - 60000 },
      { ip: '172.67.xxx.xxx', latency: 45.2, reputationScore: 92, region: 'EU-Frankfurt', lastSeen: Date.now() - 120000 },
      { ip: '198.41.xxx.xxx', latency: 8.3, reputationScore: 99, region: 'SG', lastSeen: Date.now() - 30000 },
    ]);

    setLoading(false);
  };

  const formatBytes = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  };

  const formatTime = (ts: number): string => {
    const diff = Date.now() - ts;
    if (diff < 60000) return '刚刚';
    if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`;
    if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`;
    return `${Math.floor(diff / 86400000)} 天前`;
  };

  const handleRollback = (profileId: string) => {
    if (confirm(`确定回滚到 ${profileId}？`)) {
      sendCommand('vault:rollback_dna', { profileId });
    }
  };

  const handleLockToggle = () => {
    if (stats?.locked) {
      sendCommand('vault:unlock', {});
    } else {
      sendCommand('vault:lock', {});
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* 硬件密钥状态 */}
      <div className={`p-4 rounded-lg border ${stats?.hardwareKeyLoaded ? 'bg-green-900/20 border-green-600/30' : 'bg-red-900/20 border-red-600/30'}`}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="text-2xl">{stats?.hardwareKeyLoaded ? '🔓' : '🔒'}</span>
            <div>
              <div className="font-medium text-white">硬件密钥状态</div>
              <div className="text-sm text-slate-400">
                {stats?.hardwareKeyLoaded ? '密钥已从硬件指纹派生并加载到内存' : '密钥未加载，Vault 已锁定'}
              </div>
            </div>
          </div>
          <button
            onClick={handleLockToggle}
            className={`px-4 py-2 rounded text-sm transition-colors ${
              stats?.locked
                ? 'bg-green-600 hover:bg-green-500 text-white'
                : 'bg-red-600 hover:bg-red-500 text-white'
            }`}
          >
            {stats?.locked ? '解锁 Vault' : '锁定 Vault'}
          </button>
        </div>
      </div>

      {/* 资产占比统计 */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <h3 className="text-lg font-medium text-white mb-4 flex items-center gap-2">
          <span>📊</span> 资产占比统计
        </h3>

        <div className="grid grid-cols-4 gap-4 mb-6">
          <div className="bg-slate-800/50 rounded-lg p-4 text-center">
            <div className="text-3xl font-bold text-cyan-400">{stats?.dnaCount}</div>
            <div className="text-sm text-slate-400 mt-1">DNA 指纹</div>
            <div className="text-xs text-slate-500">~{formatBytes((stats?.fileSize || 0) * 0.3)}</div>
          </div>
          <div className="bg-slate-800/50 rounded-lg p-4 text-center">
            <div className="text-3xl font-bold text-orange-400">{stats?.ipCount}</div>
            <div className="text-sm text-slate-400 mt-1">IP 信誉</div>
            <div className="text-xs text-slate-500">~{formatBytes((stats?.fileSize || 0) * 0.6)}</div>
          </div>
          <div className="bg-slate-800/50 rounded-lg p-4 text-center">
            <div className="text-3xl font-bold text-purple-400">{stats?.sysCount}</div>
            <div className="text-sm text-slate-400 mt-1">系统配置</div>
            <div className="text-xs text-slate-500">~{formatBytes((stats?.fileSize || 0) * 0.1)}</div>
          </div>
          <div className="bg-slate-800/50 rounded-lg p-4 text-center">
            <div className="text-3xl font-bold text-white">{formatBytes(stats?.fileSize || 0)}</div>
            <div className="text-sm text-slate-400 mt-1">总大小</div>
            <div className="text-xs text-slate-500">AES-256-GCM 加密</div>
          </div>
        </div>

        {/* 占比条 */}
        <div className="h-4 bg-slate-800 rounded-full overflow-hidden flex">
          <div className="bg-cyan-500 h-full" style={{ width: '30%' }} title="DNA 指纹" />
          <div className="bg-orange-500 h-full" style={{ width: '60%' }} title="IP 信誉" />
          <div className="bg-purple-500 h-full" style={{ width: '10%' }} title="系统配置" />
        </div>
        <div className="flex justify-between text-xs text-slate-500 mt-2">
          <span>DNA 30%</span>
          <span>IP 60%</span>
          <span>SYS 10%</span>
        </div>
      </div>

      {/* 标签页 */}
      <div className="flex gap-2 border-b border-slate-800 pb-2">
        {(['overview', 'dna', 'ip', 'system'] as const).map(tab => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 rounded-t text-sm transition-colors ${
              activeTab === tab
                ? 'bg-slate-800 text-white'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            {tab === 'overview' && '📋 概览'}
            {tab === 'dna' && '🧬 DNA 库'}
            {tab === 'ip' && '🌐 IP 信誉'}
            {tab === 'system' && '⚙️ 系统'}
          </button>
        ))}
      </div>

      {/* DNA 库管理 */}
      {activeTab === 'dna' && (
        <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-medium text-white">指纹库版本管理</h3>
            <span className="text-xs text-slate-500">支持回滚到上一个稳定版本</span>
          </div>

          <div className="space-y-3">
            {dnaRecords.map((record, idx) => (
              <div key={record.profileId} className="bg-slate-800/50 rounded-lg p-4 flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div className={`w-3 h-3 rounded-full ${idx === 0 ? 'bg-green-500' : 'bg-slate-600'}`} />
                  <div>
                    <div className="font-medium text-white">{record.browser} {record.version}</div>
                    <div className="text-sm text-slate-400">{record.os}</div>
                  </div>
                </div>
                <div className="text-right">
                  <div className="text-xs text-slate-500 font-mono">{record.ja4.slice(0, 20)}...</div>
                  <div className="text-xs text-slate-600">更新于 {formatTime(record.updatedAt)}</div>
                </div>
                <button
                  onClick={() => handleRollback(record.profileId)}
                  disabled={idx === 0}
                  className={`px-3 py-1 rounded text-xs transition-colors ${
                    idx === 0
                      ? 'bg-slate-700 text-slate-500 cursor-not-allowed'
                      : 'bg-yellow-600/20 text-yellow-400 hover:bg-yellow-600/30'
                  }`}
                >
                  {idx === 0 ? '当前版本' : '回滚'}
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* IP 信誉库 */}
      {activeTab === 'ip' && (
        <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
          <h3 className="text-lg font-medium text-white mb-4">地理分布性能矩阵</h3>

          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-slate-400 border-b border-slate-700">
                  <th className="text-left py-2">IP</th>
                  <th className="text-left py-2">区域</th>
                  <th className="text-right py-2">延迟</th>
                  <th className="text-right py-2">信誉分</th>
                  <th className="text-right py-2">最后探测</th>
                </tr>
              </thead>
              <tbody>
                {ipRecords.map(record => (
                  <tr key={record.ip} className="border-b border-slate-800">
                    <td className="py-3 font-mono text-slate-300">{record.ip}</td>
                    <td className="py-3 text-slate-400">{record.region}</td>
                    <td className="py-3 text-right">
                      <span className={record.latency < 20 ? 'text-green-400' : record.latency < 50 ? 'text-yellow-400' : 'text-red-400'}>
                        {record.latency.toFixed(1)} ms
                      </span>
                    </td>
                    <td className="py-3 text-right">
                      <span className={record.reputationScore >= 90 ? 'text-green-400' : record.reputationScore >= 70 ? 'text-yellow-400' : 'text-red-400'}>
                        {record.reputationScore}
                      </span>
                    </td>
                    <td className="py-3 text-right text-slate-500">{formatTime(record.lastSeen)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* 系统状态 */}
      {activeTab === 'system' && (
        <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
          <h3 className="text-lg font-medium text-white mb-4">系统状态</h3>

          <div className="space-y-4">
            <div className="flex items-center justify-between p-3 bg-slate-800/50 rounded">
              <span className="text-slate-400">Kill Switch 状态</span>
              <span className={`font-medium ${stats?.state === 'RUNNING' ? 'text-green-400' : 'text-red-400'}`}>
                {stats?.state}
              </span>
            </div>
            <div className="flex items-center justify-between p-3 bg-slate-800/50 rounded">
              <span className="text-slate-400">Vault 锁定状态</span>
              <span className={`font-medium ${stats?.locked ? 'text-red-400' : 'text-green-400'}`}>
                {stats?.locked ? '已锁定' : '已解锁'}
              </span>
            </div>
            <div className="flex items-center justify-between p-3 bg-slate-800/50 rounded">
              <span className="text-slate-400">上次加密备份</span>
              <span className="text-slate-300">{formatTime(stats?.lastBackup || 0)}</span>
            </div>
            <div className="flex items-center justify-between p-3 bg-slate-800/50 rounded">
              <span className="text-slate-400">硬件密钥</span>
              <span className={`font-medium ${stats?.hardwareKeyLoaded ? 'text-green-400' : 'text-red-400'}`}>
                {stats?.hardwareKeyLoaded ? '已加载（仅内存）' : '未加载'}
              </span>
            </div>
          </div>
        </div>
      )}

      {/* 概览 */}
      {activeTab === 'overview' && (
        <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
          <h3 className="text-lg font-medium text-white mb-4">DB 健康度</h3>

          <div className="grid grid-cols-3 gap-4">
            <div className="bg-slate-800/50 rounded-lg p-4">
              <div className="text-sm text-slate-400 mb-1">文件大小</div>
              <div className="text-xl font-bold text-white">{formatBytes(stats?.fileSize || 0)}</div>
              <div className="text-xs text-green-400 mt-1">✓ 正常</div>
            </div>
            <div className="bg-slate-800/50 rounded-lg p-4">
              <div className="text-sm text-slate-400 mb-1">页溢出</div>
              <div className="text-xl font-bold text-white">0</div>
              <div className="text-xs text-green-400 mt-1">✓ 无溢出</div>
            </div>
            <div className="bg-slate-800/50 rounded-lg p-4">
              <div className="text-sm text-slate-400 mb-1">加密备份</div>
              <div className="text-xl font-bold text-white">{formatTime(stats?.lastBackup || 0)}</div>
              <div className="text-xs text-yellow-400 mt-1">⚠ 建议每日备份</div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default VaultManager;
