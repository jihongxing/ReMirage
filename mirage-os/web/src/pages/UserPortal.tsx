// 用户登录/接入门户 - 匿名身份系统
import { useState, useEffect } from 'react';

interface VaultStatus {
  exists: boolean;
  uid: string | null;
  publicKey: string | null;
}

interface UserPortalProps {
  onAuthenticated: (uid: string) => void;
}

const UserPortal = ({ onAuthenticated }: UserPortalProps) => {
  const [, setVaultStatus] = useState<VaultStatus>({ exists: false, uid: null, publicKey: null });
  const [mode, setMode] = useState<'check' | 'import' | 'generate'>('check');
  const [importKey, setImportKey] = useState('');
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState('');

  // 检查本地 Vault
  useEffect(() => {
    checkLocalVault();
  }, []);

  const checkLocalVault = () => {
    const vault = localStorage.getItem('mirage_vault');
    if (vault) {
      try {
        const parsed = JSON.parse(vault);
        setVaultStatus({ exists: true, uid: parsed.uid, publicKey: parsed.publicKey });
        // 自动认证
        onAuthenticated(parsed.uid);
      } catch {
        setVaultStatus({ exists: false, uid: null, publicKey: null });
        setMode('import');
      }
    } else {
      setMode('import');
    }
  };

  // 生成新身份
  const handleGenerate = async () => {
    setGenerating(true);
    setError('');

    try {
      // 模拟 Ed25519 密钥生成
      const keyPair = await generateKeyPair();
      const uid = deriveUID(keyPair.publicKey);

      const vault = {
        uid,
        publicKey: keyPair.publicKey,
        privateKey: keyPair.privateKey,
        createdAt: Date.now(),
      };

      localStorage.setItem('mirage_vault', JSON.stringify(vault));
      setVaultStatus({ exists: true, uid, publicKey: keyPair.publicKey });
      onAuthenticated(uid);
    } catch (err) {
      setError('密钥生成失败');
    } finally {
      setGenerating(false);
    }
  };

  // 导入密钥
  const handleImport = () => {
    setError('');

    if (!importKey.trim()) {
      setError('请输入密钥');
      return;
    }

    try {
      // 验证密钥格式
      const keyData = JSON.parse(atob(importKey.trim()));
      if (!keyData.publicKey || !keyData.privateKey) {
        throw new Error('Invalid key format');
      }

      const uid = deriveUID(keyData.publicKey);
      const vault = {
        uid,
        publicKey: keyData.publicKey,
        privateKey: keyData.privateKey,
        createdAt: Date.now(),
      };

      localStorage.setItem('mirage_vault', JSON.stringify(vault));
      setVaultStatus({ exists: true, uid, publicKey: keyData.publicKey });
      onAuthenticated(uid);
    } catch {
      setError('密钥格式无效');
    }
  };

  // 模拟密钥生成
  const generateKeyPair = async (): Promise<{ publicKey: string; privateKey: string }> => {
    await new Promise(r => setTimeout(r, 1500)); // 模拟延迟
    const bytes = new Uint8Array(32);
    crypto.getRandomValues(bytes);
    const publicKey = Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('');
    const privateBytes = new Uint8Array(64);
    crypto.getRandomValues(privateBytes);
    const privateKey = Array.from(privateBytes).map(b => b.toString(16).padStart(2, '0')).join('');
    return { publicKey, privateKey };
  };

  // 从公钥派生 UID
  const deriveUID = (publicKey: string): string => {
    const hash = publicKey.slice(0, 12);
    return `u-${hash}`;
  };

  return (
    <div className="min-h-screen bg-black flex items-center justify-center p-4">
      <div className="w-full max-w-md">
        {/* Logo */}
        <div className="text-center mb-8">
          <div className="text-4xl mb-2">🔐</div>
          <h1 className="text-xl font-bold text-white">Sovereign Access</h1>
          <p className="text-slate-500 text-sm mt-1">匿名身份 · 无痕接入</p>
        </div>

        {/* 主面板 */}
        <div className="bg-slate-900/80 backdrop-blur-sm rounded-lg border border-slate-800 p-6">
          {mode === 'check' && (
            <div className="text-center py-8">
              <div className="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin mx-auto mb-4" />
              <p className="text-slate-400 text-sm">检查本地身份...</p>
            </div>
          )}

          {mode === 'import' && (
            <>
              {/* 选项卡 */}
              <div className="flex gap-2 mb-6">
                <button
                  onClick={() => setMode('import')}
                  className="flex-1 py-2 text-sm rounded bg-cyan-600/20 text-cyan-400 border border-cyan-600/30"
                >
                  导入密钥
                </button>
                <button
                  onClick={() => setMode('generate')}
                  className="flex-1 py-2 text-sm rounded bg-slate-800 text-slate-400 hover:bg-slate-700 transition-colors"
                >
                  生成新身份
                </button>
              </div>

              {/* 导入表单 */}
              <div className="space-y-4">
                <div>
                  <label className="block text-xs text-slate-500 mb-2">密钥文件 (Base64)</label>
                  <textarea
                    value={importKey}
                    onChange={(e) => setImportKey(e.target.value)}
                    placeholder="粘贴您的 mirage_vault 密钥..."
                    className="w-full h-32 bg-black/50 border border-slate-700 rounded p-3 text-sm font-mono text-green-400 placeholder-slate-600 focus:border-cyan-500 focus:outline-none resize-none"
                  />
                </div>

                {error && (
                  <p className="text-red-400 text-xs">{error}</p>
                )}

                <button
                  onClick={handleImport}
                  className="w-full py-3 bg-cyan-600 hover:bg-cyan-500 text-white rounded transition-colors"
                >
                  验证并接入
                </button>
              </div>
            </>
          )}

          {mode === 'generate' && (
            <>
              {/* 选项卡 */}
              <div className="flex gap-2 mb-6">
                <button
                  onClick={() => setMode('import')}
                  className="flex-1 py-2 text-sm rounded bg-slate-800 text-slate-400 hover:bg-slate-700 transition-colors"
                >
                  导入密钥
                </button>
                <button
                  onClick={() => setMode('generate')}
                  className="flex-1 py-2 text-sm rounded bg-cyan-600/20 text-cyan-400 border border-cyan-600/30"
                >
                  生成新身份
                </button>
              </div>

              {/* 生成说明 */}
              <div className="space-y-4">
                <div className="bg-black/50 rounded p-4 border border-slate-700">
                  <h3 className="text-sm font-medium text-white mb-2">⚠️ 重要提示</h3>
                  <ul className="text-xs text-slate-400 space-y-1">
                    <li>• 系统将生成 Ed25519 密钥对</li>
                    <li>• 您的 UID 由公钥派生，无法更改</li>
                    <li>• 私钥仅存储在本地，丢失无法恢复</li>
                    <li>• 请务必备份您的密钥文件</li>
                  </ul>
                </div>

                {error && (
                  <p className="text-red-400 text-xs">{error}</p>
                )}

                <button
                  onClick={handleGenerate}
                  disabled={generating}
                  className={`w-full py-3 rounded transition-colors ${
                    generating
                      ? 'bg-slate-700 text-slate-500 cursor-not-allowed'
                      : 'bg-green-600 hover:bg-green-500 text-white'
                  }`}
                >
                  {generating ? '生成中...' : '生成新身份'}
                </button>
              </div>
            </>
          )}
        </div>

        {/* 底部提示 */}
        <p className="text-center text-slate-600 text-xs mt-6">
          无账号 · 无邮箱 · 无追踪
        </p>
      </div>
    </div>
  );
};

export default UserPortal;
