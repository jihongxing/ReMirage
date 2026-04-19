// 套餐与续费 - 匿名订阅系统
import { useState } from 'react';
import { useMirageSocket } from '../hooks/useMirageSocket';

interface Plan {
  id: string;
  name: string;
  quota: number; // GB
  price: number; // XMR
  features: string[];
  popular?: boolean;
}

const PLANS: Plan[] = [
  {
    id: 'starter',
    name: 'Starter',
    quota: 50,
    price: 0.05,
    features: ['50 GB 流量', '标准拟态', '3 节点可选'],
  },
  {
    id: 'pro',
    name: 'Pro',
    quota: 200,
    price: 0.15,
    features: ['200 GB 流量', '高级拟态', '全球节点', 'FEC 冗余'],
    popular: true,
  },
  {
    id: 'unlimited',
    name: 'Unlimited',
    quota: -1,
    price: 0.5,
    features: ['无限流量', '最高拟态', '专属节点', 'FEC + CID 轮换', '优先支持'],
  },
];

const SubscriptionStore = () => {
  const { sendCommand } = useMirageSocket();
  const [selectedPlan, setSelectedPlan] = useState<string | null>(null);
  const [billingMode, setBillingMode] = useState<'quota' | 'monthly'>('quota');
  const [customQuota, setCustomQuota] = useState(100);
  const [processing, setProcessing] = useState(false);
  const [balance] = useState(0.0847);

  const handlePurchase = (planId: string) => {
    const plan = PLANS.find(p => p.id === planId);
    if (!plan) return;

    if (balance < plan.price) {
      alert('余额不足，请先充值');
      return;
    }

    setProcessing(true);
    sendCommand('subscription:purchase', { planId, billingMode });

    // 模拟处理
    setTimeout(() => {
      setProcessing(false);
      setSelectedPlan(null);
      alert('订阅成功！');
    }, 2000);
  };

  const handleCustomPurchase = () => {
    const price = customQuota * 0.001; // 0.001 XMR/GB
    if (balance < price) {
      alert('余额不足，请先充值');
      return;
    }

    setProcessing(true);
    sendCommand('subscription:custom', { quota: customQuota, price });

    setTimeout(() => {
      setProcessing(false);
      alert('购买成功！');
    }, 2000);
  };

  return (
    <div className="space-y-6">
      {/* 计费模式切换 */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-4">
        <div className="flex items-center gap-4">
          <span className="text-sm text-slate-400">计费模式:</span>
          <div className="flex gap-2">
            <button
              onClick={() => setBillingMode('quota')}
              className={`px-4 py-2 rounded text-sm transition-colors ${
                billingMode === 'quota'
                  ? 'bg-cyan-600 text-white'
                  : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
              }`}
            >
              按量计费
            </button>
            <button
              onClick={() => setBillingMode('monthly')}
              className={`px-4 py-2 rounded text-sm transition-colors ${
                billingMode === 'monthly'
                  ? 'bg-cyan-600 text-white'
                  : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
              }`}
            >
              包月套餐
            </button>
          </div>
        </div>
      </div>

      {/* 套餐卡片 */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {PLANS.map(plan => (
          <div
            key={plan.id}
            className={`relative bg-slate-900/80 rounded-lg border p-6 transition-all ${
              plan.popular
                ? 'border-cyan-500 shadow-lg shadow-cyan-500/20'
                : 'border-slate-800 hover:border-slate-700'
            }`}
          >
            {plan.popular && (
              <div className="absolute -top-3 left-1/2 -translate-x-1/2 px-3 py-1 bg-cyan-600 text-white text-xs rounded-full">
                推荐
              </div>
            )}

            <h3 className="text-xl font-bold text-white mb-2">{plan.name}</h3>
            <div className="mb-4">
              <span className="text-3xl font-bold text-orange-400">{plan.price}</span>
              <span className="text-slate-400 ml-1">XMR</span>
              {billingMode === 'monthly' && (
                <span className="text-xs text-slate-500 ml-2">/月</span>
              )}
            </div>

            <ul className="space-y-2 mb-6">
              {plan.features.map((feature, idx) => (
                <li key={idx} className="flex items-center gap-2 text-sm text-slate-400">
                  <span className="text-green-400">✓</span>
                  {feature}
                </li>
              ))}
            </ul>

            <button
              onClick={() => handlePurchase(plan.id)}
              disabled={processing}
              className={`w-full py-3 rounded-lg transition-colors ${
                plan.popular
                  ? 'bg-cyan-600 hover:bg-cyan-500 text-white'
                  : 'bg-slate-800 hover:bg-slate-700 text-slate-300'
              } ${processing ? 'opacity-50 cursor-not-allowed' : ''}`}
            >
              {processing && selectedPlan === plan.id ? '处理中...' : '立即订阅'}
            </button>
          </div>
        ))}
      </div>

      {/* 自定义配额 */}
      {billingMode === 'quota' && (
        <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
          <h3 className="text-lg font-medium text-white mb-4">自定义配额</h3>
          
          <div className="space-y-4">
            <div>
              <div className="flex justify-between text-sm mb-2">
                <span className="text-slate-400">配额大小</span>
                <span className="text-white">{customQuota} GB</span>
              </div>
              <input
                type="range"
                min="10"
                max="500"
                step="10"
                value={customQuota}
                onChange={(e) => setCustomQuota(Number(e.target.value))}
                className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
              />
              <div className="flex justify-between text-xs text-slate-500 mt-1">
                <span>10 GB</span>
                <span>500 GB</span>
              </div>
            </div>

            <div className="flex items-center justify-between p-4 bg-black/30 rounded-lg">
              <span className="text-slate-400">费用</span>
              <div>
                <span className="text-2xl font-bold text-orange-400">
                  {(customQuota * 0.001).toFixed(3)}
                </span>
                <span className="text-slate-400 ml-1">XMR</span>
              </div>
            </div>

            <button
              onClick={handleCustomPurchase}
              disabled={processing}
              className={`w-full py-3 bg-orange-600 hover:bg-orange-500 text-white rounded-lg transition-colors ${
                processing ? 'opacity-50 cursor-not-allowed' : ''
              }`}
            >
              {processing ? '处理中...' : '购买配额'}
            </button>
          </div>
        </div>
      )}

      {/* 当前余额提示 */}
      <div className="bg-slate-900/50 rounded-lg border border-slate-800 p-4">
        <div className="flex items-center justify-between">
          <span className="text-sm text-slate-400">当前余额</span>
          <span className="text-lg font-medium text-orange-400">{balance.toFixed(4)} XMR</span>
        </div>
      </div>
    </div>
  );
};

export default SubscriptionStore;
