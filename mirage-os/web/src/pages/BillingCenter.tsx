// 计费中心 - Monero 支付与配额管理（取证对抗增强）
import { useState, useEffect } from 'react';
import { XAxis, YAxis, Tooltip, ResponsiveContainer, AreaChart, Area } from 'recharts';
import { fuzzyTime, maskHash, maskUID } from '../utils/GhostStorage';

interface User {
  uid: string;
  balance: number;  // XMR
  quotaUsed: number;
  quotaLimit: number;
  burnRate: number; // bytes/sec
  status: 'active' | 'suspended' | 'warning';
  lastActivity: number;
}

interface Transaction {
  txId: string;
  amount: number;
  confirmations: number;
  status: 'pending' | 'confirmed' | 'failed';
  timestamp: number;
}

const mockUsers: User[] = [
  { uid: 'u-7f3a9c', balance: 2.45, quotaUsed: 45.2, quotaLimit: 100, burnRate: 1250000, status: 'active', lastActivity: Date.now() - 60000 },
  { uid: 'u-8b2d1e', balance: 0.12, quotaUsed: 89.5, quotaLimit: 100, burnRate: 890000, status: 'warning', lastActivity: Date.now() - 120000 },
  { uid: 'u-4c5f2a', balance: 5.80, quotaUsed: 23.1, quotaLimit: 200, burnRate: 2100000, status: 'active', lastActivity: Date.now() - 30000 },
  { uid: 'u-9d6e3b', balance: 0.00, quotaUsed: 100, quotaLimit: 100, burnRate: 0, status: 'suspended', lastActivity: Date.now() - 3600000 },
];

const mockTransactions: Transaction[] = [
  { txId: 'tx-a1b2c3', amount: 1.5, confirmations: 12, status: 'confirmed', timestamp: Date.now() - 3600000 },
  { txId: 'tx-d4e5f6', amount: 0.5, confirmations: 3, status: 'pending', timestamp: Date.now() - 1800000 },
  { txId: 'tx-g7h8i9', amount: 2.0, confirmations: 0, status: 'pending', timestamp: Date.now() - 600000 },
];

const BillingCenter = () => {
  const [users, setUsers] = useState<User[]>(mockUsers);
  const [transactions] = useState<Transaction[]>(mockTransactions);
  const [burnHistory, setBurnHistory] = useState<any[]>([]);

  // 模拟烧录速度历史
  useEffect(() => {
    const history = Array.from({ length: 30 }, (_, i) => ({
      time: new Date(Date.now() - (29 - i) * 60000).toLocaleTimeString().slice(0, 5),
      rate: Math.random() * 3 + 1,
      cost: Math.random() * 0.001,
    }));
    setBurnHistory(history);
  }, []);

  // 模拟实时更新
  useEffect(() => {
    const interval = setInterval(() => {
      setUsers(prev => prev.map(u => ({
        ...u,
        quotaUsed: Math.min(u.quotaLimit, u.quotaUsed + (u.status === 'active' ? Math.random() * 0.1 : 0)),
        burnRate: u.status === 'active' ? Math.max(0, u.burnRate + (Math.random() - 0.5) * 200000) : 0,
      })));

      setBurnHistory(prev => [
        ...prev.slice(1),
        {
          time: new Date().toLocaleTimeString().slice(0, 5),
          rate: Math.random() * 3 + 1,
          cost: Math.random() * 0.001,
        }
      ]);
    }, 5000);
    return () => clearInterval(interval);
  }, []);

  const formatBytes = (bytes: number) => {
    if (bytes >= 1e9) return (bytes / 1e9).toFixed(2) + ' GB';
    if (bytes >= 1e6) return (bytes / 1e6).toFixed(2) + ' MB';
    if (bytes >= 1e3) return (bytes / 1e3).toFixed(2) + ' KB';
    return bytes + ' B';
  };

  const handleSuspend = (uid: string) => {
    setUsers(prev => prev.map(u => 
      u.uid === uid ? { ...u, status: 'suspended', burnRate: 0 } : u
    ));
  };

  const handleReactivate = (uid: string) => {
    setUsers(prev => prev.map(u => 
      u.uid === uid ? { ...u, status: 'active' } : u
    ));
  };

  const totalRevenue = users.reduce((sum, u) => sum + u.balance, 0);
  const activeUsers = users.filter(u => u.status === 'active').length;

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">计费中心</h1>
          <p className="text-slate-400 text-sm mt-1">Monero 支付与配额管理</p>
        </div>
        <div className="flex items-center gap-6">
          <div className="text-right">
            <p className="text-xs text-slate-500">总余额</p>
            <p className="text-xl font-bold text-orange-400">{totalRevenue.toFixed(4)} XMR</p>
          </div>
          <div className="text-right">
            <p className="text-xs text-slate-500">活跃用户</p>
            <p className="text-xl font-bold text-green-400">{activeUsers}</p>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-12 gap-6">
        {/* 烧录速度曲线 */}
        <div className="col-span-8 bg-slate-900 rounded-lg border border-slate-800 p-6">
          <h3 className="text-sm font-medium text-slate-400 mb-4">全局烧录速度 (MB/s → XMR/h)</h3>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={burnHistory}>
                <defs>
                  <linearGradient id="burnGradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#f97316" stopOpacity={0.3}/>
                    <stop offset="95%" stopColor="#f97316" stopOpacity={0}/>
                  </linearGradient>
                </defs>
                <XAxis dataKey="time" stroke="#475569" fontSize={10} />
                <YAxis stroke="#475569" fontSize={10} />
                <Tooltip 
                  contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #334155' }}
                  labelStyle={{ color: '#94a3b8' }}
                />
                <Area type="monotone" dataKey="rate" stroke="#f97316" fill="url(#burnGradient)" name="MB/s" />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* XMR 交易状态 */}
        <div className="col-span-4 bg-slate-900 rounded-lg border border-slate-800 p-6">
          <h3 className="text-sm font-medium text-slate-400 mb-4">XMR 交易确认</h3>
          <div className="space-y-3">
            {transactions.map(tx => (
              <div key={tx.txId} className="bg-slate-800/50 rounded-lg p-3">
                <div className="flex items-center justify-between mb-2">
                  {/* TxHash 遮掩：确认后仅显示状态 */}
                  <span className="text-xs font-mono text-slate-500">
                    {tx.status === 'confirmed' ? '••••••••' : maskHash(tx.txId)}
                  </span>
                  <span className={`text-xs px-2 py-0.5 rounded ${
                    tx.status === 'confirmed' ? 'bg-green-500/20 text-green-400' :
                    tx.status === 'pending' ? 'bg-yellow-500/20 text-yellow-400' :
                    'bg-red-500/20 text-red-400'
                  }`}>
                    {tx.status === 'confirmed' ? 'Completed' : tx.status}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-orange-400 font-medium">{tx.amount} XMR</span>
                  {tx.status !== 'confirmed' ? (
                    <div className="flex items-center gap-1">
                      {Array.from({ length: 10 }).map((_, i) => (
                        <div
                          key={i}
                          className={`w-1.5 h-3 rounded-sm ${
                            i < tx.confirmations ? 'bg-green-500' : 'bg-slate-700'
                          }`}
                        />
                      ))}
                      <span className="text-xs text-slate-500 ml-1">{tx.confirmations}/10</span>
                    </div>
                  ) : (
                    <span className="text-xs text-slate-500">{fuzzyTime(tx.timestamp)}</span>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* 用户列表 */}
        <div className="col-span-12 bg-slate-900 rounded-lg border border-slate-800">
          <div className="p-4 border-b border-slate-800">
            <h3 className="text-sm font-medium text-slate-400">用户配额审计</h3>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="text-xs text-slate-500 border-b border-slate-800">
                  <th className="text-left p-4">UID</th>
                  <th className="text-left p-4">余额 (XMR)</th>
                  <th className="text-left p-4">配额使用</th>
                  <th className="text-left p-4">烧录速度</th>
                  <th className="text-left p-4">状态</th>
                  <th className="text-left p-4">最后活动</th>
                  <th className="text-right p-4">操作</th>
                </tr>
              </thead>
              <tbody>
                {users.map(user => (
                  <tr key={user.uid} className="border-b border-slate-800/50 hover:bg-slate-800/30">
                    <td className="p-4">
                      <span className="font-mono text-sm text-white">{maskUID(user.uid)}</span>
                    </td>
                    <td className="p-4">
                      <span className={`font-medium ${user.balance < 0.5 ? 'text-red-400' : 'text-orange-400'}`}>
                        {user.balance.toFixed(4)}
                      </span>
                    </td>
                    <td className="p-4">
                      <div className="flex items-center gap-2">
                        <div className="w-24 h-2 bg-slate-800 rounded-full overflow-hidden">
                          <div 
                            className={`h-full ${
                              user.quotaUsed / user.quotaLimit > 0.9 ? 'bg-red-500' :
                              user.quotaUsed / user.quotaLimit > 0.7 ? 'bg-yellow-500' :
                              'bg-cyan-500'
                            }`}
                            style={{ width: `${(user.quotaUsed / user.quotaLimit) * 100}%` }}
                          />
                        </div>
                        <span className="text-xs text-slate-400">
                          {user.quotaUsed.toFixed(1)}/{user.quotaLimit} GB
                        </span>
                      </div>
                    </td>
                    <td className="p-4">
                      <span className="text-sm text-slate-300">{formatBytes(user.burnRate)}/s</span>
                    </td>
                    <td className="p-4">
                      <span className={`text-xs px-2 py-1 rounded ${
                        user.status === 'active' ? 'bg-green-500/20 text-green-400' :
                        user.status === 'warning' ? 'bg-yellow-500/20 text-yellow-400' :
                        'bg-red-500/20 text-red-400'
                      }`}>
                        {user.status}
                      </span>
                    </td>
                    <td className="p-4">
                      {/* 时间戳相对化 */}
                      <span className="text-xs text-slate-500">
                        {fuzzyTime(user.lastActivity)}
                      </span>
                    </td>
                    <td className="p-4 text-right">
                      {user.status === 'suspended' ? (
                        <button
                          onClick={() => handleReactivate(user.uid)}
                          className="text-xs px-3 py-1 bg-green-600/20 hover:bg-green-600/30 text-green-400 rounded transition-colors"
                        >
                          激活
                        </button>
                      ) : (
                        <button
                          onClick={() => handleSuspend(user.uid)}
                          className="text-xs px-3 py-1 bg-red-600/20 hover:bg-red-600/30 text-red-400 rounded transition-colors"
                        >
                          封禁
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
};

export default BillingCenter;
