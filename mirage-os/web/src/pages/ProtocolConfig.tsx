// 协议详细配置 - H3/QUIC 深度微调
import { useState } from 'react';
import { useMirageSocket } from '../hooks/useMirageSocket';

interface ProtocolSettings {
  bdna: {
    forcedMimicry: string | null;
    autoSwitch: boolean;
    switchInterval: number;
  };
  gtunnel: {
    fecRedundancy: number;
    cidRotationInterval: number;
    multipathEnabled: boolean;
    pathCount: number;
  };
  jitter: {
    enabled: boolean;
    profile: string;
    variance: number;
  };
  npm: {
    paddingEnabled: boolean;
    paddingSize: number;
    burstMode: boolean;
  };
}

const MIMICRY_OPTIONS = [
  { id: null, name: '自动切换', desc: '根据时区自动选择' },
  { id: 'zoom', name: 'Zoom', desc: '视频会议拟态' },
  { id: 'teams', name: 'Teams', desc: '企业协作拟态' },
  { id: 'netflix', name: 'Netflix', desc: '流媒体拟态' },
  { id: 'youtube', name: 'YouTube', desc: '视频流拟态' },
  { id: 'cloudflare', name: 'Cloudflare', desc: 'CDN 拟态' },
];

const JITTER_PROFILES = [
  { id: 'residential', name: '家庭宽带', desc: '模拟 DSL/光纤' },
  { id: 'mobile', name: '移动网络', desc: '模拟 4G/5G' },
  { id: 'corporate', name: '企业专线', desc: '低抖动稳定' },
  { id: 'satellite', name: '卫星链路', desc: '高延迟高抖动' },
];

const ProtocolConfig = () => {
  const { sendCommand } = useMirageSocket();
  const [settings, setSettings] = useState<ProtocolSettings>({
    bdna: {
      forcedMimicry: null,
      autoSwitch: true,
      switchInterval: 3600,
    },
    gtunnel: {
      fecRedundancy: 4,
      cidRotationInterval: 30,
      multipathEnabled: true,
      pathCount: 3,
    },
    jitter: {
      enabled: true,
      profile: 'residential',
      variance: 15,
    },
    npm: {
      paddingEnabled: true,
      paddingSize: 128,
      burstMode: false,
    },
  });
  const [saving, setSaving] = useState(false);

  const updateSetting = <K extends keyof ProtocolSettings>(
    category: K,
    key: keyof ProtocolSettings[K],
    value: ProtocolSettings[K][keyof ProtocolSettings[K]]
  ) => {
    setSettings(prev => ({
      ...prev,
      [category]: { ...prev[category], [key]: value },
    }));
  };

  const handleSave = () => {
    setSaving(true);
    sendCommand('protocol:config', settings);
    setTimeout(() => {
      setSaving(false);
    }, 1000);
  };

  return (
    <div className="space-y-6">
      {/* B-DNA 拟态配置 */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
          <span>🧬</span> B-DNA 拟态配置
        </h2>

        <div className="space-y-4">
          <div>
            <label className="block text-sm text-slate-400 mb-2">拟态模式</label>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
              {MIMICRY_OPTIONS.map(opt => (
                <button
                  key={opt.id ?? 'auto'}
                  onClick={() => {
                    updateSetting('bdna', 'forcedMimicry', opt.id);
                    updateSetting('bdna', 'autoSwitch', opt.id === null);
                  }}
                  className={`p-3 rounded-lg border text-left transition-colors ${
                    settings.bdna.forcedMimicry === opt.id
                      ? 'border-cyan-500 bg-cyan-600/20 text-cyan-400'
                      : 'border-slate-700 bg-slate-800/50 text-slate-400 hover:border-slate-600'
                  }`}
                >
                  <p className="text-sm font-medium">{opt.name}</p>
                  <p className="text-xs text-slate-500">{opt.desc}</p>
                </button>
              ))}
            </div>
          </div>

          {settings.bdna.autoSwitch && (
            <div>
              <label className="block text-sm text-slate-400 mb-2">
                切换间隔: {settings.bdna.switchInterval}s
              </label>
              <input
                type="range"
                min="600"
                max="7200"
                step="300"
                value={settings.bdna.switchInterval}
                onChange={(e) => updateSetting('bdna', 'switchInterval', Number(e.target.value))}
                className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
              />
            </div>
          )}
        </div>
      </div>

      {/* G-Tunnel 配置 */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
          <span>🚇</span> G-Tunnel 隧道配置
        </h2>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div>
            <label className="block text-sm text-slate-400 mb-2">
              FEC 冗余度: RS(10, {settings.gtunnel.fecRedundancy})
            </label>
            <input
              type="range"
              min="2"
              max="8"
              value={settings.gtunnel.fecRedundancy}
              onChange={(e) => updateSetting('gtunnel', 'fecRedundancy', Number(e.target.value))}
              className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
            />
            <p className="text-xs text-slate-500 mt-1">
              冗余度越高，抗丢包能力越强，但带宽消耗增加
            </p>
          </div>

          <div>
            <label className="block text-sm text-slate-400 mb-2">
              CID 轮换间隔: {settings.gtunnel.cidRotationInterval}s
            </label>
            <input
              type="range"
              min="10"
              max="120"
              step="5"
              value={settings.gtunnel.cidRotationInterval}
              onChange={(e) => updateSetting('gtunnel', 'cidRotationInterval', Number(e.target.value))}
              className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
            />
          </div>

          <div className="flex items-center justify-between p-3 bg-black/30 rounded-lg">
            <div>
              <p className="text-sm text-white">多路径传输</p>
              <p className="text-xs text-slate-500">同时使用多条路径</p>
            </div>
            <button
              onClick={() => updateSetting('gtunnel', 'multipathEnabled', !settings.gtunnel.multipathEnabled)}
              className={`relative w-10 h-5 rounded-full transition-colors ${
                settings.gtunnel.multipathEnabled ? 'bg-cyan-600' : 'bg-slate-700'
              }`}
            >
              <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform ${
                settings.gtunnel.multipathEnabled ? 'left-5' : 'left-0.5'
              }`} />
            </button>
          </div>

          {settings.gtunnel.multipathEnabled && (
            <div>
              <label className="block text-sm text-slate-400 mb-2">
                路径数量: {settings.gtunnel.pathCount}
              </label>
              <input
                type="range"
                min="2"
                max="5"
                value={settings.gtunnel.pathCount}
                onChange={(e) => updateSetting('gtunnel', 'pathCount', Number(e.target.value))}
                className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
              />
            </div>
          )}
        </div>
      </div>

      {/* Jitter-Lite 配置 */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
          <span>⏱️</span> Jitter-Lite 时域扰动
        </h2>

        <div className="space-y-4">
          <div className="flex items-center justify-between p-3 bg-black/30 rounded-lg">
            <div>
              <p className="text-sm text-white">启用时域扰动</p>
              <p className="text-xs text-slate-500">模拟真实网络抖动</p>
            </div>
            <button
              onClick={() => updateSetting('jitter', 'enabled', !settings.jitter.enabled)}
              className={`relative w-10 h-5 rounded-full transition-colors ${
                settings.jitter.enabled ? 'bg-cyan-600' : 'bg-slate-700'
              }`}
            >
              <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform ${
                settings.jitter.enabled ? 'left-5' : 'left-0.5'
              }`} />
            </button>
          </div>

          {settings.jitter.enabled && (
            <>
              <div>
                <label className="block text-sm text-slate-400 mb-2">抖动配置文件</label>
                <div className="grid grid-cols-2 gap-2">
                  {JITTER_PROFILES.map(profile => (
                    <button
                      key={profile.id}
                      onClick={() => updateSetting('jitter', 'profile', profile.id)}
                      className={`p-3 rounded-lg border text-left transition-colors ${
                        settings.jitter.profile === profile.id
                          ? 'border-cyan-500 bg-cyan-600/20 text-cyan-400'
                          : 'border-slate-700 bg-slate-800/50 text-slate-400 hover:border-slate-600'
                      }`}
                    >
                      <p className="text-sm font-medium">{profile.name}</p>
                      <p className="text-xs text-slate-500">{profile.desc}</p>
                    </button>
                  ))}
                </div>
              </div>

              <div>
                <label className="block text-sm text-slate-400 mb-2">
                  抖动方差: ±{settings.jitter.variance}ms
                </label>
                <input
                  type="range"
                  min="5"
                  max="50"
                  value={settings.jitter.variance}
                  onChange={(e) => updateSetting('jitter', 'variance', Number(e.target.value))}
                  className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
                />
              </div>
            </>
          )}
        </div>
      </div>

      {/* NPM 流量伪装 */}
      <div className="bg-slate-900/80 rounded-lg border border-slate-800 p-6">
        <h2 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
          <span>📦</span> NPM 流量伪装
        </h2>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="flex items-center justify-between p-3 bg-black/30 rounded-lg">
            <div>
              <p className="text-sm text-white">Padding 填充</p>
              <p className="text-xs text-slate-500">混淆包大小特征</p>
            </div>
            <button
              onClick={() => updateSetting('npm', 'paddingEnabled', !settings.npm.paddingEnabled)}
              className={`relative w-10 h-5 rounded-full transition-colors ${
                settings.npm.paddingEnabled ? 'bg-cyan-600' : 'bg-slate-700'
              }`}
            >
              <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform ${
                settings.npm.paddingEnabled ? 'left-5' : 'left-0.5'
              }`} />
            </button>
          </div>

          <div className="flex items-center justify-between p-3 bg-black/30 rounded-lg">
            <div>
              <p className="text-sm text-white">Burst 模式</p>
              <p className="text-xs text-slate-500">模拟突发流量</p>
            </div>
            <button
              onClick={() => updateSetting('npm', 'burstMode', !settings.npm.burstMode)}
              className={`relative w-10 h-5 rounded-full transition-colors ${
                settings.npm.burstMode ? 'bg-cyan-600' : 'bg-slate-700'
              }`}
            >
              <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform ${
                settings.npm.burstMode ? 'left-5' : 'left-0.5'
              }`} />
            </button>
          </div>

          {settings.npm.paddingEnabled && (
            <div className="md:col-span-2">
              <label className="block text-sm text-slate-400 mb-2">
                Padding 大小: {settings.npm.paddingSize} bytes
              </label>
              <input
                type="range"
                min="32"
                max="512"
                step="32"
                value={settings.npm.paddingSize}
                onChange={(e) => updateSetting('npm', 'paddingSize', Number(e.target.value))}
                className="w-full h-2 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-500"
              />
            </div>
          )}
        </div>
      </div>

      {/* 保存按钮 */}
      <button
        onClick={handleSave}
        disabled={saving}
        className={`w-full py-3 rounded-lg transition-colors ${
          saving
            ? 'bg-slate-700 text-slate-500 cursor-not-allowed'
            : 'bg-cyan-600 hover:bg-cyan-500 text-white'
        }`}
      >
        {saving ? '保存中...' : '保存配置'}
      </button>
    </div>
  );
};

export default ProtocolConfig;
