// 用户端布局 - 匿名用户界面
import { useState } from 'react';
import { Outlet, NavLink, useNavigate } from 'react-router-dom';
import { useMirageSocket } from '../hooks/useMirageSocket';

const UserLayout = () => {
  const { connected } = useMirageSocket();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(false);

  // 获取 UID
  const vault = localStorage.getItem('mirage_vault');
  const uid = vault ? JSON.parse(vault).uid : 'unknown';

  const handleLogout = () => {
    localStorage.removeItem('mirage_vault');
    navigate('/');
    window.location.reload();
  };

  const navItems = [
    { path: '/portal/dashboard', icon: '📊', label: '仪表盘' },
    { path: '/portal/deposit', icon: '💰', label: '充值' },
    { path: '/portal/plans', icon: '📦', label: '套餐' },
  ];

  return (
    <div className="min-h-screen bg-slate-950 flex">
      {/* 侧边栏 */}
      <aside className={`${collapsed ? 'w-16' : 'w-56'} bg-slate-900 border-r border-slate-800 transition-all duration-300 flex flex-col`}>
        {/* Logo */}
        <div className="p-4 border-b border-slate-800">
          <div className="flex items-center gap-3">
            <div className="text-2xl">🔐</div>
            {!collapsed && (
              <div>
                <h1 className="text-lg font-bold text-white">Mirage</h1>
                <p className="text-xs text-slate-500">User Portal</p>
              </div>
            )}
          </div>
        </div>

        {/* 导航 */}
        <nav className="flex-1 p-2">
          {navItems.map(item => (
            <NavLink
              key={item.path}
              to={item.path}
              className={({ isActive }) => `
                flex items-center gap-3 px-3 py-2.5 rounded-lg mb-1 transition-colors
                ${isActive 
                  ? 'bg-cyan-600/20 text-cyan-400 border border-cyan-600/30' 
                  : 'text-slate-400 hover:bg-slate-800 hover:text-white'
                }
              `}
            >
              <span className="text-lg">{item.icon}</span>
              {!collapsed && <span className="text-sm">{item.label}</span>}
            </NavLink>
          ))}
        </nav>

        {/* 底部状态 */}
        <div className="p-4 border-t border-slate-800">
          {!collapsed && (
            <div className="mb-4">
              <div className="flex items-center gap-2 mb-2">
                <div className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
                <span className="text-xs text-slate-400">
                  {connected ? '已连接' : '离线'}
                </span>
              </div>
              <div className="text-xs text-slate-500">
                <p className="truncate">UID: {uid}</p>
              </div>
            </div>
          )}
          
          <button
            onClick={handleLogout}
            className="w-full flex items-center justify-center gap-2 px-3 py-2 bg-red-600/20 hover:bg-red-600/30 text-red-400 rounded-lg transition-colors text-sm"
          >
            <span>🚪</span>
            {!collapsed && <span>退出</span>}
          </button>
        </div>

        {/* 折叠按钮 */}
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="p-2 border-t border-slate-800 text-slate-500 hover:text-white transition-colors"
        >
          {collapsed ? '→' : '←'}
        </button>
      </aside>

      {/* 主内容区 */}
      <main className="flex-1 overflow-auto">
        {/* 顶部栏 */}
        <header className="bg-slate-900/50 backdrop-blur-sm border-b border-slate-800 px-6 py-4 sticky top-0 z-10">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500 animate-pulse' : 'bg-red-500'}`} />
              <span className="text-sm text-slate-400">
                {connected ? '实时同步' : '离线模式'}
              </span>
            </div>
            <div className="text-sm text-slate-400">
              🕐 {new Date().toLocaleTimeString()}
            </div>
          </div>
        </header>

        {/* 页面内容 */}
        <div className="p-6">
          <Outlet />
        </div>
      </main>
    </div>
  );
};

export default UserLayout;
