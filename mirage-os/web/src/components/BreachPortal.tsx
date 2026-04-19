// 裂缝登录门户 - 隐匿认证界面
import { useState, useEffect } from 'react';
import { useMirageAuth } from '../hooks/useMirageAuth';

interface BreachPortalProps {
  onSuccess: () => void;
}

const BreachPortal = ({ onSuccess }: BreachPortalProps) => {
  const { breachProgress, authenticate, devAuthenticate, isAuthenticated } = useMirageAuth();
  const [challenge, setChallenge] = useState('');
  const [signature, setSignature] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (isAuthenticated) {
      onSuccess();
    }
  }, [isAuthenticated, onSuccess]);

  // 生成挑战码
  useEffect(() => {
    const newChallenge = Array.from(crypto.getRandomValues(new Uint8Array(16)))
      .map(b => b.toString(16).padStart(2, '0'))
      .join('');
    setChallenge(newChallenge);
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');

    const success = await authenticate(signature, challenge);
    if (success) {
      onSuccess();
    } else {
      setError('签名验证失败');
    }
    setLoading(false);
  };

  return (
    <div className="min-h-screen bg-black flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        {/* 进度指示器 */}
        {breachProgress > 0 && breachProgress < 1 && (
          <div className="mb-8">
            <div className="h-1 bg-slate-800 rounded-full overflow-hidden">
              <div 
                className="h-full bg-cyan-500 transition-all duration-300"
                style={{ width: `${breachProgress * 100}%` }}
              />
            </div>
          </div>
        )}

        {/* 认证表单 */}
        <div className="bg-slate-900/80 backdrop-blur-sm rounded-lg border border-slate-800 p-8">
          <div className="text-center mb-8">
            <div className="text-4xl mb-4">🔐</div>
            <h2 className="text-xl font-mono text-cyan-400">BREACH PROTOCOL</h2>
            <p className="text-slate-500 text-sm mt-2">Ed25519 Signature Required</p>
          </div>

          {/* 挑战码 */}
          <div className="mb-6">
            <label className="block text-slate-400 text-xs mb-2 font-mono">CHALLENGE</label>
            <div className="bg-black/50 rounded p-3 font-mono text-sm text-green-400 break-all border border-slate-700">
              {challenge}
            </div>
          </div>

          <form onSubmit={handleSubmit}>
            {/* 签名输入 */}
            <div className="mb-6">
              <label className="block text-slate-400 text-xs mb-2 font-mono">SIGNATURE</label>
              <textarea
                value={signature}
                onChange={(e) => setSignature(e.target.value)}
                className="w-full bg-black/50 rounded p-3 font-mono text-sm text-white border border-slate-700 focus:border-cyan-500 focus:outline-none resize-none h-24"
                placeholder="Paste Ed25519 signature..."
              />
            </div>

            {error && (
              <div className="mb-4 text-red-400 text-sm text-center">{error}</div>
            )}

            <button
              type="submit"
              disabled={loading || !signature}
              className="w-full bg-cyan-600 hover:bg-cyan-500 disabled:bg-slate-700 disabled:cursor-not-allowed text-white font-mono py-3 rounded transition-colors"
            >
              {loading ? 'VERIFYING...' : 'AUTHENTICATE'}
            </button>
          </form>

          {/* 开发模式快捷入口 */}
          {import.meta.env.DEV && (
            <button
              onClick={() => {
                devAuthenticate();
              }}
              className="w-full mt-4 bg-slate-800 hover:bg-slate-700 text-slate-400 font-mono py-2 rounded text-sm transition-colors"
            >
              [DEV] Skip Auth
            </button>
          )}
        </div>

        {/* 提示 */}
        <p className="text-center text-slate-600 text-xs mt-6 font-mono">
          Type 'mirage' to activate breach protocol
        </p>
      </div>
    </div>
  );
};

export default BreachPortal;
