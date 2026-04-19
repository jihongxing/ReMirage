// Mirage Command Center - 指挥中心入口页面
// 整合所有功能模块的统一入口
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';

interface ModuleCard {
  id: string;
  icon: string;
  title: string;
  description: string;
  path: string;
  status: 'online' | 'warning' | 'offline';
  stats?: { label: string; value: string }[];
}

export default function CommandCenter() {
  const navigate = useNavigate();
  const [hoveredModule, setHoveredModule] = useState<string | null>(null);

  const modules: ModuleCard[] = [
    {
      id: 'tactical',
      icon: '🎮',
      title: '战术指挥中心',
      description: '全球态势感知、性能监控、资产管理、DNA 进化',
      path: '/terminal/tactical',
      status: 'online',
      stats: [
        { label: '节点', value: '29' },
        { label: '威胁', value: '3' },
        { label: '信誉', value: '92%' },
      ],
    },
    {
      id: 'monitor',
      icon: '📊',
      title: '态势监控',
      description: '实时流量、威胁情报、攻击者画像',
      path: '/terminal/monitor',
      status: 'online',
      stats: [
        { label: '流量', value: '1.2TB' },
        { label: '拦截', value: '847' },
      ],
    },
    {
      id: 'gateways',
      icon: '🌐',
      title: '网关管理',
      description: '分布式节点部署、健康检查、负载均衡',
      path: '/terminal/gateways',
      status: 'online',
      stats: [
        { label: '在线', value: '29' },
        { label: '离线', value: '0' },
      ],
    },
    {
      id: 'protocols',
      icon: '🔧',
      title: '协议配置',
      description: 'NPM/B-DNA/VPC/Jitter/G-Tunnel/G-Switch',
      path: '/terminal/protocols',
      status: 'online',
      stats: [
        { label: '协议', value: '6' },
        { label: '规则', value: '156' },
      ],
    },
    {
      id: 'threats',
      icon: '🛡️',
      title: '威胁分析',
      description: 'DPI 检测、指纹扫描、时序攻击分析',
      path: '/terminal/threats',
      status: 'warning',
      stats: [
        { label: '活跃', value: '3' },
        { label: '已处理', value: '1.2K' },
      ],
    },
    {
      id: 'billing',
      icon: '💰',
      title: '计费中心',
      description: 'XMR 充值、配额管理、消费记录',
      path: '/terminal/billing',
      status: 'online',
      stats: [
        { label: '余额', value: '12.5 XMR' },
        { label: '配额', value: '500GB' },
      ],
    },
    {
      id: 'users',
      icon: '👥',
      title: '用户管理',
      description: '影子账户、权限控制、邀请码',
      path: '/terminal/users',
      status: 'online',
      stats: [
        { label: '用户', value: '156' },
        { label: '活跃', value: '89' },
      ],
    },
    {
      id: 'forensic',
      icon: '🔍',
      title: '取证归档',
      description: '攻击日志、行为分析、证据链',
      path: '/terminal/forensic',
      status: 'online',
      stats: [
        { label: '记录', value: '12.3K' },
        { label: '归档', value: '2.1GB' },
      ],
    },
  ];

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'online': return 'bg-green-500';
      case 'warning': return 'bg-yellow-500';
      case 'offline': return 'bg-red-500';
      default: return 'bg-gray-500';
    }
  };

  return (
    <div className="space-y-6">
      {/* 标题区 */}
      <div className="text-center mb-8">
        <h1 className="text-3xl font-bold text-white mb-2 flex items-center justify-center gap-3">
          <span className="text-4xl">🌀</span>
          Mirage Command Center
        </h1>
        <p className="text-slate-400">网络黑洞 · 绝对隐匿 · 全球态势感知平台</p>
        <div className="flex items-center justify-center gap-6 mt-4 text-sm">
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
            <span className="text-slate-400">系统运行正常</span>
          </div>
          <div className="text-slate-500">|</div>
          <div className="text-slate-400">
            节点: <span className="text-green-400 font-bold">29</span> 在线
          </div>
          <div className="text-slate-500">|</div>
          <div className="text-slate-400">
            威胁: <span className="text-yellow-400 font-bold">3</span> 活跃
          </div>
        </div>
      </div>

      {/* 模块网格 */}
      <div className="grid grid-cols-4 gap-4">
        {modules.map(module => (
          <div
            key={module.id}
            onClick={() => navigate(module.path)}
            onMouseEnter={() => setHoveredModule(module.id)}
            onMouseLeave={() => setHoveredModule(null)}
            className={`
              bg-slate-800 rounded-xl p-5 cursor-pointer transition-all duration-300
              border-2 ${hoveredModule === module.id ? 'border-cyan-500 scale-[1.02]' : 'border-slate-700'}
              hover:shadow-lg hover:shadow-cyan-500/10
            `}
          >
            {/* 状态指示 */}
            <div className="flex items-center justify-between mb-3">
              <span className="text-3xl">{module.icon}</span>
              <span className={`w-2.5 h-2.5 rounded-full ${getStatusColor(module.status)}`} />
            </div>

            {/* 标题 */}
            <h3 className="text-white font-bold mb-1">{module.title}</h3>
            <p className="text-slate-500 text-xs mb-3 line-clamp-2">{module.description}</p>

            {/* 统计 */}
            {module.stats && (
              <div className="flex items-center gap-3 pt-3 border-t border-slate-700">
                {module.stats.map((stat, idx) => (
                  <div key={idx} className="text-center">
                    <div className="text-cyan-400 font-bold text-sm">{stat.value}</div>
                    <div className="text-xs text-slate-500">{stat.label}</div>
                  </div>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>

      {/* 快捷操作 */}
      <div className="bg-slate-800 rounded-xl p-5 border border-slate-700">
        <h3 className="text-white font-bold mb-4 flex items-center gap-2">
          ⚡ 快捷操作
        </h3>
        <div className="grid grid-cols-6 gap-3">
          <button
            onClick={() => navigate('/terminal/tactical')}
            className="bg-cyan-600 hover:bg-cyan-500 text-white py-2.5 px-4 rounded-lg text-sm font-medium transition-colors"
          >
            🎮 战术 HUD
          </button>
          <button className="bg-slate-700 hover:bg-slate-600 text-white py-2.5 px-4 rounded-lg text-sm transition-colors">
            🔄 刷新节点
          </button>
          <button className="bg-slate-700 hover:bg-slate-600 text-white py-2.5 px-4 rounded-lg text-sm transition-colors">
            📡 同步 DNA
          </button>
          <button className="bg-slate-700 hover:bg-slate-600 text-white py-2.5 px-4 rounded-lg text-sm transition-colors">
            🌐 域名转生
          </button>
          <button className="bg-slate-700 hover:bg-slate-600 text-white py-2.5 px-4 rounded-lg text-sm transition-colors">
            📊 导出报告
          </button>
          <button className="bg-red-600/20 hover:bg-red-600/30 text-red-400 py-2.5 px-4 rounded-lg text-sm transition-colors">
            💀 Kill Switch
          </button>
        </div>
      </div>

      {/* 底部 */}
      <div className="text-center text-slate-600 text-xs pt-4">
        <p>Mirage Project © 2026 · V1.0 Final · 绝对隐匿 · 网络黑洞</p>
      </div>
    </div>
  );
}
