// 遥测数据收集器 - 学术研究项目（取证对抗伪装）
// 本系统用于网络流量模式研究，符合 IEEE 数据伦理规范
import { useState } from 'react';
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts';
import { fuzzyTime, maskUID, maskHash } from '../utils/GhostStorage';

interface TelemetrySubject {
  subjectId: string;        // 伪装：研究对象编号
  enrolledAt: number;       // 伪装：入组时间
  cohort: 'alpha' | 'beta' | 'control';  // 伪装：研究组别
  dataStatus: 'collecting' | 'paused';   // 伪装：数据采集状态
  lastDataPoint: number;    // 伪装：最后数据点
  dataPoints: number;       // 伪装：数据点数量
  deviceHash: string;       // 伪装：设备哈希（匿名化）
}

// 伪装数据
const mockSubjects: TelemetrySubject[] = [
  { subjectId: 'S-001-A7F3', enrolledAt: Date.now() - 86400000 * 30, cohort: 'alpha', dataStatus: 'collecting', lastDataPoint: Date.now() - 3600000, dataPoints: 15600, deviceHash: 'a7f3c9d2' },
  { subjectId: 'S-002-B8E4', enrolledAt: Date.now() - 86400000 * 15, cohort: 'beta', dataStatus: 'collecting', lastDataPoint: Date.now() - 7200000, dataPoints: 8900, deviceHash: 'b8e4d1a3' },
  { subjectId: 'S-003-C9F5', enrolledAt: Date.now() - 86400000 * 7, cohort: 'control', dataStatus: 'collecting', lastDataPoint: Date.now() - 86400000, dataPoints: 2300, deviceHash: 'c9f5e2b4' },
  { subjectId: 'S-004-D0G6', enrolledAt: Date.now() - 86400000 * 3, cohort: 'beta', dataStatus: 'paused', lastDataPoint: Date.now() - 86400000 * 2, dataPoints: 1200, deviceHash: 'd0g6f3c5' },
];

// 统计图表数据
const cohortDistribution = [
  { name: 'Alpha', value: 35, color: '#8b5cf6' },
  { name: 'Beta', value: 45, color: '#06b6d4' },
  { name: 'Control', value: 20, color: '#64748b' },
];

const dataCollectionTrend = [
  { week: 'W1', points: 12400 },
  { week: 'W2', points: 18900 },
  { week: 'W3', points: 24300 },
  { week: 'W4', points: 31200 },
];

const UserManager = () => {
  const [subjects, setSubjects] = useState<TelemetrySubject[]>(mockSubjects);
  const [showEnroll, setShowEnroll] = useState(false);
  const [enrollCohort, setEnrollCohort] = useState<'alpha' | 'beta' | 'control'>('control');
  const [enrollCode, setEnrollCode] = useState('');

  const generateEnrollCode = () => {
    const code = Array.from(crypto.getRandomValues(new Uint8Array(16)))
      .map(b => b.toString(16).padStart(2, '0'))
      .join('');
    setEnrollCode(code);
  };

  const handleStatusToggle = (subjectId: string) => {
    setSubjects(prev => prev.map(s => 
      s.subjectId === subjectId 
        ? { ...s, dataStatus: s.dataStatus === 'collecting' ? 'paused' : 'collecting' }
        : s
    ));
  };

  const handleWithdraw = (subjectId: string) => {
    if (confirm(`确认将研究对象 ${subjectId} 退出研究？数据将按规范匿名化处理。`)) {
      setSubjects(prev => prev.filter(s => s.subjectId !== subjectId));
    }
  };

  const getCohortColor = (cohort: string) => {
    switch (cohort) {
      case 'alpha': return 'bg-purple-500/20 text-purple-400';
      case 'beta': return 'bg-cyan-500/20 text-cyan-400';
      default: return 'bg-slate-500/20 text-slate-400';
    }
  };

  return (
    <div className="space-y-6">
      {/* 页面标题 - 学术研究伪装 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">遥测数据收集器</h1>
          <p className="text-slate-400 text-sm mt-1">
            网络流量模式研究 · IRB Protocol #2026-NTM-0042
          </p>
        </div>
        <button
          onClick={() => { setShowEnroll(true); generateEnrollCode(); }}
          className="px-4 py-2 bg-cyan-600 hover:bg-cyan-500 text-white rounded-lg text-sm transition-colors"
        >
          + 新增研究对象
        </button>
      </div>

      {/* 研究声明 */}
      <div className="bg-blue-900/20 border border-blue-800/50 rounded-lg p-4">
        <p className="text-xs text-blue-300">
          📋 本研究已获得机构审查委员会（IRB）批准，所有数据采集符合 IEEE 数据伦理规范。
          研究对象可随时退出，数据将按规范进行匿名化处理。
        </p>
      </div>

      {/* 统计图表优先展示 */}
      <div className="grid grid-cols-12 gap-6">
        {/* 研究组别分布 */}
        <div className="col-span-4 bg-slate-900 rounded-lg border border-slate-800 p-6">
          <h3 className="text-sm font-medium text-slate-400 mb-4">研究组别分布</h3>
          <div className="h-48">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={cohortDistribution}
                  cx="50%"
                  cy="50%"
                  innerRadius={40}
                  outerRadius={70}
                  dataKey="value"
                  label={({ name, percent }) => `${name} ${(percent * 100).toFixed(0)}%`}
                >
                  {cohortDistribution.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* 数据采集趋势 */}
        <div className="col-span-8 bg-slate-900 rounded-lg border border-slate-800 p-6">
          <h3 className="text-sm font-medium text-slate-400 mb-4">数据点采集趋势（周）</h3>
          <div className="h-48">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={dataCollectionTrend}>
                <XAxis dataKey="week" stroke="#475569" fontSize={10} />
                <YAxis stroke="#475569" fontSize={10} />
                <Tooltip 
                  contentStyle={{ backgroundColor: '#1e293b', border: '1px solid #334155' }}
                />
                <Bar dataKey="points" fill="#06b6d4" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      </div>

      {/* 入组弹窗 */}
      {showEnroll && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-slate-900 rounded-lg border border-slate-800 p-6 w-full max-w-md">
            <h3 className="text-lg font-bold text-white mb-4">新增研究对象</h3>
            
            <div className="mb-4">
              <label className="block text-sm text-slate-400 mb-2">分配组别</label>
              <div className="flex gap-2">
                {(['alpha', 'beta', 'control'] as const).map(cohort => (
                  <button
                    key={cohort}
                    onClick={() => setEnrollCohort(cohort)}
                    className={`flex-1 py-2 rounded text-sm transition-colors capitalize ${
                      enrollCohort === cohort 
                        ? 'bg-cyan-600 text-white' 
                        : 'bg-slate-800 text-slate-400'
                    }`}
                  >
                    {cohort}
                  </button>
                ))}
              </div>
            </div>

            <div className="mb-6">
              <label className="block text-sm text-slate-400 mb-2">入组凭证</label>
              <div className="bg-black/50 rounded p-3 font-mono text-sm text-green-400 break-all border border-slate-700">
                {enrollCode}
              </div>
              <p className="text-xs text-slate-500 mt-2">有效期：24 小时 · 单次使用</p>
            </div>

            <div className="flex gap-3">
              <button
                onClick={() => navigator.clipboard.writeText(enrollCode)}
                className="flex-1 py-2 bg-slate-800 hover:bg-slate-700 text-white rounded transition-colors"
              >
                复制
              </button>
              <button
                onClick={() => setShowEnroll(false)}
                className="flex-1 py-2 bg-slate-700 hover:bg-slate-600 text-slate-300 rounded transition-colors"
              >
                关闭
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 统计卡片 */}
      <div className="grid grid-cols-4 gap-4">
        <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
          <p className="text-xs text-slate-500">研究对象</p>
          <p className="text-2xl font-bold text-white">{subjects.length}</p>
        </div>
        <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
          <p className="text-xs text-slate-500">采集中</p>
          <p className="text-2xl font-bold text-green-400">{subjects.filter(s => s.dataStatus === 'collecting').length}</p>
        </div>
        <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
          <p className="text-xs text-slate-500">总数据点</p>
          <p className="text-2xl font-bold text-cyan-400">{(subjects.reduce((sum, s) => sum + s.dataPoints, 0) / 1000).toFixed(1)}K</p>
        </div>
        <div className="bg-slate-900 rounded-lg border border-slate-800 p-4">
          <p className="text-xs text-slate-500">研究周期</p>
          <p className="text-2xl font-bold text-purple-400">4 周</p>
        </div>
      </div>

      {/* 研究对象列表 */}
      <div className="bg-slate-900 rounded-lg border border-slate-800">
        <div className="p-4 border-b border-slate-800">
          <h3 className="text-sm font-medium text-slate-400">研究对象清单</h3>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="text-xs text-slate-500 border-b border-slate-800">
                <th className="text-left p-4">对象编号</th>
                <th className="text-left p-4">组别</th>
                <th className="text-left p-4">采集状态</th>
                <th className="text-left p-4">入组时间</th>
                <th className="text-left p-4">最后数据点</th>
                <th className="text-left p-4">数据点数</th>
                <th className="text-left p-4">设备哈希</th>
                <th className="text-right p-4">操作</th>
              </tr>
            </thead>
            <tbody>
              {subjects.map(subject => (
                <tr key={subject.subjectId} className="border-b border-slate-800/50 hover:bg-slate-800/30">
                  <td className="p-4">
                    <span className="font-mono text-sm text-white">{maskUID(subject.subjectId)}</span>
                  </td>
                  <td className="p-4">
                    <span className={`text-xs px-2 py-1 rounded capitalize ${getCohortColor(subject.cohort)}`}>
                      {subject.cohort}
                    </span>
                  </td>
                  <td className="p-4">
                    <span className={`text-xs px-2 py-1 rounded ${
                      subject.dataStatus === 'collecting' 
                        ? 'bg-green-500/20 text-green-400' 
                        : 'bg-yellow-500/20 text-yellow-400'
                    }`}>
                      {subject.dataStatus === 'collecting' ? '采集中' : '已暂停'}
                    </span>
                  </td>
                  <td className="p-4">
                    <span className="text-xs text-slate-400">{fuzzyTime(subject.enrolledAt)}</span>
                  </td>
                  <td className="p-4">
                    <span className="text-xs text-slate-400">{fuzzyTime(subject.lastDataPoint)}</span>
                  </td>
                  <td className="p-4">
                    <span className="text-sm text-slate-300">{subject.dataPoints.toLocaleString()}</span>
                  </td>
                  <td className="p-4">
                    <span className="font-mono text-xs text-slate-500">{maskHash(subject.deviceHash)}</span>
                  </td>
                  <td className="p-4 text-right">
                    <div className="flex items-center justify-end gap-2">
                      <button
                        onClick={() => handleStatusToggle(subject.subjectId)}
                        className={`text-xs px-3 py-1 rounded transition-colors ${
                          subject.dataStatus === 'collecting'
                            ? 'bg-yellow-600/20 hover:bg-yellow-600/30 text-yellow-400'
                            : 'bg-green-600/20 hover:bg-green-600/30 text-green-400'
                        }`}
                      >
                        {subject.dataStatus === 'collecting' ? '暂停' : '恢复'}
                      </button>
                      <button
                        onClick={() => handleWithdraw(subject.subjectId)}
                        className="text-xs px-3 py-1 bg-red-600/20 hover:bg-red-600/30 text-red-400 rounded transition-colors"
                      >
                        退出
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* 底部声明 */}
      <div className="text-center text-xs text-slate-600 py-4">
        Network Traffic Modeling Research · Data Ethics Compliant · IRB #2026-NTM-0042
      </div>
    </div>
  );
};

export default UserManager;
