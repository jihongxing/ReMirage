// ScorchedEarthButton - 焦土核按钮（强交互阻断）
// 安全机制：必须手动输入精确的 Gateway ID 才能执行
import { useState, useCallback } from 'react';

interface Props {
  gatewayId: string;
  onConfirm: (gatewayId: string) => void;
}

export const ScorchedEarthButton = ({ gatewayId, onConfirm }: Props) => {
  const [showModal, setShowModal] = useState(false);
  const [inputValue, setInputValue] = useState('');
  const [executing, setExecuting] = useState(false);

  const isMatch = inputValue === gatewayId;

  const handleExecute = useCallback(() => {
    if (!isMatch) return;
    setExecuting(true);
    onConfirm(gatewayId);
    // 3s 后重置
    setTimeout(() => {
      setExecuting(false);
      setShowModal(false);
      setInputValue('');
    }, 3000);
  }, [isMatch, gatewayId, onConfirm]);

  return (
    <>
      {/* 触发按钮 */}
      <button
        onClick={() => setShowModal(true)}
        className="px-3 py-1.5 text-xs font-mono bg-red-950/50 border border-red-800/50 
                   text-red-400 rounded hover:bg-red-900/50 hover:border-red-700 
                   transition-all active:scale-95"
      >
        ☢ SCORCHED EARTH
      </button>

      {/* 确认模态框 */}
      {showModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm">
          <div className="bg-slate-900 border border-red-800/50 rounded-xl p-6 w-[420px] shadow-2xl shadow-red-900/20">
            {/* 警告头部 */}
            <div className="text-center mb-6">
              <div className="text-4xl mb-3">☢️</div>
              <h2 className="text-lg font-bold text-red-400">焦土协议确认</h2>
              <p className="text-sm text-slate-400 mt-2">
                此操作将永久销毁目标节点的所有数据，包括 eBPF Map、TLS 证书和会话密钥。
                <span className="text-red-400 font-bold"> 不可逆。</span>
              </p>
            </div>

            {/* 目标信息 */}
            <div className="bg-slate-950 rounded-lg p-3 mb-4 border border-slate-800">
              <span className="text-xs text-slate-500">目标节点</span>
              <p className="text-sm font-mono text-red-300 mt-1">{gatewayId}</p>
            </div>

            {/* 输入确认 */}
            <div className="mb-6">
              <label className="text-xs text-slate-400 block mb-2">
                请输入完整的 Gateway ID 以确认销毁：
              </label>
              <input
                type="text"
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                placeholder={gatewayId}
                className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 
                           text-sm font-mono text-slate-200 placeholder-slate-700
                           focus:outline-none focus:border-red-600 transition-colors"
                autoFocus
                disabled={executing}
              />
              {inputValue.length > 0 && !isMatch && (
                <p className="text-xs text-red-500 mt-1">ID 不匹配</p>
              )}
            </div>

            {/* 操作按钮 */}
            <div className="flex gap-3">
              <button
                onClick={() => { setShowModal(false); setInputValue(''); }}
                className="flex-1 px-4 py-2 text-sm bg-slate-800 text-slate-300 rounded-lg
                           hover:bg-slate-700 transition-colors"
                disabled={executing}
              >
                取消
              </button>
              <button
                onClick={handleExecute}
                disabled={!isMatch || executing}
                className={`flex-1 px-4 py-2 text-sm rounded-lg font-bold transition-all ${
                  isMatch && !executing
                    ? 'bg-red-600 text-white hover:bg-red-500 active:scale-95'
                    : 'bg-slate-800 text-slate-600 cursor-not-allowed'
                }`}
              >
                {executing ? '🔥 执行中...' : '执行销毁'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
};

export default ScorchedEarthButton;
