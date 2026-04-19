// XMR 充值系统 - 匿名加密货币充值
import { useState, useEffect } from 'react';
import { useMirageSocket } from '../hooks/useMirageSocket';

interface DepositAddress {
  address: string;
  paymentId: string;
  expiresAt: number;
  qrCode: string;
}

interface PendingDeposit {
  txHash: string;
  amount: number;
  confirmations: number;
  requiredConfirmations: number;
  status: 'pending' | 'confirming' | 'confirmed';
}

const XMRDeposit = () => {
  const { sendCommand } = useMirageSocket();
  const [depositAddress, setDepositAddress] = useState<DepositAddress | null>(null);
  const [pendingDeposits, setPendingDeposits] = useState<PendingDeposit[]>([]);
  const [generating, setGenerating] = useState(false);
  const [copied, setCopied] = useState(false);
  const [balance, setBalance] = useState(0);

  useEffect(() => {
    // 模拟余额
    setBalance(0.0847);
    
    // 模拟待确认交易
    setPendingDeposits([
      {
        txHash: 'a1b2c3d4e5f6...',
        amount: 0.05,
        confirmations: 7,
        requiredConfirmations: 10,
        status: 'confirming',
      },
    ]);
  }, []);

  const generateAddress = () => {
    setGenerating(true);
    sendCommand('deposit:generate', {});
    
    // 模拟生成
    setTimeout(() => {
      setDepositAddress({
        address: '4AdUndXHHZ6cfufTMvppY6JwXNouMBzSkbLYfpAV5Usx3skxNgYeYTRj5UzqtReoS44qo9mtmXCqY45DJ852K5Jv2684Rge',
        paymentId: 'f8a7b6c5d4e3f2a1',
        expiresAt: Date.now() + 3600000,
        qrCode: '',
      });
      setGenerating(false);
    }, 1500);
  };

  const copyAddress = () => {
    if (depositAddress) {
      navigator.clipboard.writeText(depositAddress.address);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const formatTime = (timestamp: number): string => {
    const mins = Math.floor((timestamp - Date.now()) / 60000);
    return `${mins} 分钟`;
  };

  return (
    <div className="space-y-6">
      {/* 当前余额 */}
      <div className="bg-gradient-to-r from-orange-600/20 to-yellow-600/20 rounded-lg border border-orange-600/30 p-6">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm text-orange-300/70 mb-1">XMR 余额</p>
            <p className="text-3xl font-bold text-orange-400">{balance.toFixed(4)} XMR</p>
            <p className="text-xs text-slate-500 mt-1">≈ ${(balance * 165).toFixed(2)} USD</p>
          </div>
          <div className="text-5xl">🪙</div>
        </div>
      </div>

      {/* 充值地址 */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
          <span>📥</span> 充值地址
        </h2>

        {!depositAddress ? (
          <div className="text-center py-8">
            <p className="text-slate-400 text-sm mb-4">生成一次性充值地址</p>
            <button
              onClick={generateAddress}
              disabled={generating}
              className={`px-6 py-3 rounded-lg transition-colors ${
                generating
                  ? 'bg-slate-700 text-slate-500 cursor-not-allowed'
                  : 'bg-orange-600 hover:bg-orange-500 text-white'
              }`}
            >
              {generating ? '生成中...' : '生成充值地址'}
            </button>
          </div>
        ) : (
          <div className="space-y-4">
            {/* 地址显示 */}
            <div className="bg-black/50 rounded p-4 border border-slate-700">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-slate-500">Monero 地址</span>
                <span className="text-xs text-yellow-400">
                  有效期: {formatTime(depositAddress.expiresAt)}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <code className="flex-1 text-xs text-orange-400 break-all font-mono">
                  {depositAddress.address}
                </code>
                <button
                  onClick={copyAddress}
                  className={`px-3 py-1.5 rounded text-xs transition-colors ${
                    copied ? 'bg-green-600 text-white' : 'bg-slate-700 hover:bg-slate-600 text-slate-300'
                  }`}
                >
                  {copied ? '已复制' : '复制'}
                </button>
              </div>
            </div>

            {/* Payment ID */}
            <div className="bg-black/30 rounded p-3">
              <p className="text-xs text-slate-500 mb-1">Payment ID (必填)</p>
              <code className="text-sm text-cyan-400 font-mono">{depositAddress.paymentId}</code>
            </div>

            {/* 提示 */}
            <div className="bg-yellow-600/10 border border-yellow-600/30 rounded p-3">
              <p className="text-xs text-yellow-400">
                ⚠️ 请务必填写 Payment ID，否则充值无法到账
              </p>
            </div>

            <button
              onClick={generateAddress}
              className="w-full py-2 bg-slate-800 hover:bg-slate-700 text-slate-400 rounded text-sm transition-colors"
            >
              重新生成地址
            </button>
          </div>
        )}
      </div>

      {/* 待确认交易 */}
      {pendingDeposits.length > 0 && (
        <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
          <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
            <span>⏳</span> 待确认交易
          </h2>

          <div className="space-y-3">
            {pendingDeposits.map((deposit, idx) => (
              <div key={idx} className="bg-black/30 rounded p-4 border border-slate-700">
                <div className="flex items-center justify-between mb-3">
                  <code className="text-xs text-slate-400 font-mono">{deposit.txHash}</code>
                  <span className="text-sm font-medium text-orange-400">+{deposit.amount} XMR</span>
                </div>
                
                {/* 确认进度 */}
                <div className="space-y-2">
                  <div className="flex justify-between text-xs">
                    <span className="text-slate-500">区块确认</span>
                    <span className="text-cyan-400">
                      {deposit.confirmations}/{deposit.requiredConfirmations}
                    </span>
                  </div>
                  <div className="h-2 bg-slate-800 rounded-full overflow-hidden">
                    <div
                      className="h-full bg-cyan-500 transition-all duration-500"
                      style={{ width: `${(deposit.confirmations / deposit.requiredConfirmations) * 100}%` }}
                    />
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 充值说明 */}
      <div className="bg-slate-900/50 rounded-lg border border-slate-800 p-4">
        <h3 className="text-sm font-medium text-white mb-2">充值说明</h3>
        <ul className="text-xs text-slate-400 space-y-1">
          <li>• 最低充值: 0.01 XMR</li>
          <li>• 需要 10 个区块确认后到账</li>
          <li>• 充值地址有效期 1 小时</li>
          <li>• 充值完全匿名，无需 KYC</li>
        </ul>
      </div>
    </div>
  );
};

export default XMRDeposit;
