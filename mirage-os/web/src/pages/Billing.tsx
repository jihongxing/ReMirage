import { useState } from 'react';
import { useApi, apiPost } from '../hooks/useApi';
import { DataTable } from '../components/DataTable';

interface BillingLog {
  created_at: string; user_id: string; business_bytes: number;
  defense_bytes: number; business_cost: number; defense_cost: number; total_cost: number;
}
interface Quota { remaining_quota: number; total_recharged: number; total_consumed: number }

export function Billing() {
  const { data: logs, loading } = useApi<BillingLog[]>('/billing/logs');
  const { data: quota, refetch } = useApi<Quota>('/billing/quota');
  const [amount, setAmount] = useState('');
  const [showForm, setShowForm] = useState(false);
  const [msg, setMsg] = useState('');

  const columns = [
    { key: 'created_at', title: '时间', render: (v: unknown) => new Date(v as string).toLocaleString() },
    { key: 'user_id', title: '用户 ID' },
    { key: 'business_bytes', title: '业务流量', render: (v: unknown) => `${((v as number) / 1e9).toFixed(2)} GB` },
    { key: 'defense_bytes', title: '防御流量', render: (v: unknown) => `${((v as number) / 1e9).toFixed(2)} GB` },
    { key: 'business_cost', title: '业务费用', render: (v: unknown) => `$${(v as number).toFixed(4)}` },
    { key: 'defense_cost', title: '防御费用', render: (v: unknown) => `$${(v as number).toFixed(4)}` },
    { key: 'total_cost', title: '总费用', render: (v: unknown) => `$${(v as number).toFixed(4)}` },
  ];

  const handleRecharge = async () => {
    try {
      await apiPost('/billing/recharge', { amount: parseFloat(amount) });
      setMsg('充值成功');
      setShowForm(false);
      setAmount('');
      refetch();
    } catch {
      setMsg('充值失败');
    }
  };

  return (
    <div>
      <h2 className="text-xl font-semibold mb-4">Billing</h2>
      {quota && (
        <div className="grid grid-cols-3 gap-4 mb-6">
          <div className="bg-slate-900 border border-slate-800 rounded-lg p-4">
            <div className="text-sm text-slate-400">余额</div>
            <div className="text-2xl font-bold text-green-400">${quota.remaining_quota.toFixed(2)}</div>
          </div>
          <div className="bg-slate-900 border border-slate-800 rounded-lg p-4">
            <div className="text-sm text-slate-400">总充值</div>
            <div className="text-2xl font-bold">${quota.total_recharged.toFixed(2)}</div>
          </div>
          <div className="bg-slate-900 border border-slate-800 rounded-lg p-4">
            <div className="text-sm text-slate-400">总消费</div>
            <div className="text-2xl font-bold text-red-400">${quota.total_consumed.toFixed(2)}</div>
          </div>
        </div>
      )}
      <div className="mb-4 flex items-center gap-4">
        <button onClick={() => setShowForm(!showForm)} className="bg-cyan-600 hover:bg-cyan-700 text-white px-4 py-1.5 rounded text-sm">
          充值
        </button>
        {showForm && (
          <div className="flex items-center gap-2">
            <input
              type="number" value={amount} onChange={e => setAmount(e.target.value)}
              placeholder="金额" className="bg-slate-700 border border-slate-600 rounded px-3 py-1.5 text-white text-sm w-32"
            />
            <button onClick={handleRecharge} className="bg-green-600 hover:bg-green-700 text-white px-3 py-1.5 rounded text-sm">确认</button>
          </div>
        )}
        {msg && <span className={`text-sm ${msg.includes('成功') ? 'text-green-400' : 'text-red-400'}`}>{msg}</span>}
      </div>
      <DataTable columns={columns} data={logs ?? []} loading={loading} />
    </div>
  );
}
