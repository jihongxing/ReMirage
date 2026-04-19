import { useState, useEffect } from 'react';

interface AttackerProfile {
  fingerprintId: string;
  hash: string;
  firstSeen: string;
  lastSeen: string;
  seenCount: number;
  associatedIPs: string[];
  browser: string;
  os: string;
  language: string;
  timezone: number;
  screenRes: string;
  riskScore: number;
  isSuspicious: boolean;
  country?: string;
  realLocation?: { lat: number; lng: number };
}

interface ProfilerStats {
  totalFingerprints: number;
  suspiciousCount: number;
  avgDwellTime: number;
  entrapmentRate: number;
}

interface AttackerProfilerProps {
  wsUrl?: string;
}

export function AttackerProfiler({ wsUrl = 'ws://localhost:8080/ws' }: AttackerProfilerProps) {
  const [profiles, setProfiles] = useState<AttackerProfile[]>([]);
  const [stats, setStats] = useState<ProfilerStats>({
    totalFingerprints: 0,
    suspiciousCount: 0,
    avgDwellTime: 0,
    entrapmentRate: 0,
  });
  const [selectedProfile, setSelectedProfile] = useState<AttackerProfile | null>(null);

  useEffect(() => {
    const ws = new WebSocket(wsUrl);
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'fingerprint_update') {
          handleFingerprintUpdate(data.payload);
        } else if (data.type === 'profiler_stats') {
          setStats(data.payload);
        }
      } catch (e) {
        console.error('Failed to parse profiler event:', e);
      }
    };
    return () => ws.close();
  }, [wsUrl]);

  const handleFingerprintUpdate = (fp: any) => {
    setProfiles((prev) => {
      const existing = prev.findIndex((p) => p.hash === fp.hash);
      const profile: AttackerProfile = {
        fingerprintId: fp.id,
        hash: fp.hash,
        firstSeen: fp.firstSeen,
        lastSeen: fp.lastSeen || new Date().toISOString(),
        seenCount: fp.seenCount || 1,
        associatedIPs: fp.associatedIPs || [],
        browser: parseBrowser(fp.userAgent),
        os: parseOS(fp.platform),
        language: fp.language || 'Unknown',
        timezone: fp.timezone || 0,
        screenRes: fp.screenRes || 'Unknown',
        riskScore: fp.riskScore || 0,
        isSuspicious: fp.isSuspicious || false,
        country: fp.country,
        realLocation: fp.realLocation,
      };
      if (existing >= 0) {
        const updated = [...prev];
        updated[existing] = profile;
        return updated;
      }
      return [profile, ...prev].slice(0, 100);
    });
  };

  const parseBrowser = (ua: string): string => {
    if (!ua) return 'Unknown';
    if (ua.includes('Chrome')) return 'Chrome';
    if (ua.includes('Firefox')) return 'Firefox';
    if (ua.includes('Safari')) return 'Safari';
    if (ua.includes('Edge')) return 'Edge';
    return 'Other';
  };

  const parseOS = (platform: string): string => {
    if (!platform) return 'Unknown';
    if (platform.includes('Win')) return 'Windows';
    if (platform.includes('Mac')) return 'macOS';
    if (platform.includes('Linux')) return 'Linux';
    return platform;
  };

  return (
    <div className="bg-gray-900 rounded-lg p-4 text-white">
      <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
        <span className="w-2 h-2 bg-red-500 rounded-full animate-pulse" />
        攻击者画像 (Attacker Profiling)
      </h2>

      {/* 统计卡片 */}
      <div className="grid grid-cols-4 gap-3 mb-4">
        <StatCard label="指纹总数" value={stats.totalFingerprints} color="blue" />
        <StatCard label="可疑设备" value={stats.suspiciousCount} color="red" />
        <StatCard label="平均停留(s)" value={stats.avgDwellTime} color="yellow" />
        <StatCard label="诱导成功率" value={`${stats.entrapmentRate}%`} color="green" />
      </div>

      {/* 攻击者列表 */}
      <div className="bg-gray-800 rounded overflow-hidden max-h-64 overflow-y-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-700 sticky top-0">
            <tr>
              <th className="px-3 py-2 text-left">指纹 ID</th>
              <th className="px-3 py-2 text-left">浏览器/OS</th>
              <th className="px-3 py-2 text-center">关联 IP</th>
              <th className="px-3 py-2 text-center">风险分</th>
              <th className="px-3 py-2 text-center">状态</th>
            </tr>
          </thead>
          <tbody>
            {profiles.map((profile, idx) => (
              <tr
                key={profile.hash}
                className={`cursor-pointer hover:bg-gray-700 ${idx % 2 === 0 ? 'bg-gray-800' : 'bg-gray-750'}`}
                onClick={() => setSelectedProfile(profile)}
              >
                <td className="px-3 py-2 font-mono text-xs text-blue-400">
                  {profile.fingerprintId}
                </td>
                <td className="px-3 py-2">
                  <span className="text-gray-300">{profile.browser}</span>
                  <span className="text-gray-500 mx-1">/</span>
                  <span className="text-gray-400">{profile.os}</span>
                </td>
                <td className="px-3 py-2 text-center text-yellow-400">
                  {profile.associatedIPs.length}
                </td>
                <td className="px-3 py-2 text-center">
                  <RiskBadge score={profile.riskScore} />
                </td>
                <td className="px-3 py-2 text-center">
                  {profile.isSuspicious ? (
                    <span className="px-2 py-0.5 rounded text-xs bg-red-500/20 text-red-400">可疑</span>
                  ) : (
                    <span className="px-2 py-0.5 rounded text-xs bg-gray-500/20 text-gray-400">正常</span>
                  )}
                </td>
              </tr>
            ))}
            {profiles.length === 0 && (
              <tr>
                <td colSpan={5} className="px-3 py-4 text-center text-gray-500">
                  暂无攻击者画像
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* 详情面板 */}
      {selectedProfile && (
        <div className="mt-4 bg-gray-800 rounded p-4">
          <div className="flex justify-between items-center mb-3">
            <h3 className="text-sm font-semibold text-purple-400">设备详情</h3>
            <button
              onClick={() => setSelectedProfile(null)}
              className="text-gray-500 hover:text-white"
            >
              ✕
            </button>
          </div>
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <div className="text-gray-500">指纹哈希</div>
              <div className="font-mono text-xs text-gray-300 truncate">{selectedProfile.hash}</div>
            </div>
            <div>
              <div className="text-gray-500">语言/时区</div>
              <div className="text-gray-300">{selectedProfile.language} / UTC{selectedProfile.timezone >= 0 ? '+' : ''}{-selectedProfile.timezone / 60}</div>
            </div>
            <div>
              <div className="text-gray-500">屏幕分辨率</div>
              <div className="text-gray-300">{selectedProfile.screenRes}</div>
            </div>
            <div>
              <div className="text-gray-500">访问次数</div>
              <div className="text-gray-300">{selectedProfile.seenCount}</div>
            </div>
            <div className="col-span-2">
              <div className="text-gray-500 mb-1">关联 IP 列表</div>
              <div className="flex flex-wrap gap-1">
                {selectedProfile.associatedIPs.map((ip) => (
                  <span key={ip} className="px-2 py-0.5 bg-gray-700 rounded text-xs font-mono text-yellow-400">
                    {ip}
                  </span>
                ))}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function StatCard({ label, value, color }: { label: string; value: number | string; color: string }) {
  const colorMap: Record<string, string> = {
    blue: 'from-blue-600 to-blue-800',
    red: 'from-red-600 to-red-800',
    yellow: 'from-yellow-600 to-yellow-800',
    green: 'from-green-600 to-green-800',
  };
  return (
    <div className={`bg-gradient-to-br ${colorMap[color]} rounded-lg p-3`}>
      <div className="text-2xl font-bold">{typeof value === 'number' ? value.toLocaleString() : value}</div>
      <div className="text-xs text-gray-300">{label}</div>
    </div>
  );
}

function RiskBadge({ score }: { score: number }) {
  let color = 'bg-green-500/20 text-green-400';
  if (score >= 60) color = 'bg-red-500/20 text-red-400';
  else if (score >= 30) color = 'bg-yellow-500/20 text-yellow-400';
  return <span className={`px-2 py-0.5 rounded text-xs ${color}`}>{score}</span>;
}

export default AttackerProfiler;
