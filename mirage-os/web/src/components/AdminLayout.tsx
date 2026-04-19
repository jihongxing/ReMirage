// 管理后台布局
import { useState, createContext, useContext } from 'react';
import { Outlet, NavLink, useNavigate } from 'react-router-dom';
import { useMirageAuth } from '../hooks/useMirageAuth';
import { useMirageSocket } from '../hooks/useMirageSocket';

// Ghost Mode Context
interface GhostModeContextType {
  ghostMode: boolean;
  setGhostMode: (v: boolean) => void;
}

const GhostModeContext = createContext<GhostModeContextType>({
  ghostMode: false,
  setGhostMode: () => {},
});

export const useGhostMode = () => useContext(GhostModeContext);

const AdminLayout = () => {
  const { userId, userRole, logout } = useMirageAuth();
  const { connected } = useMirageSocket();
  const navigate = useNavigate();
  const [collapsed, setCollapsed] = useState(false);
  const [ghostMode, setGhostMode] = useState(false);

  const handleLogout = () => {
    logout();
    navigate('/');
  };

  const navItems = [
    { path: '/terminal/command', icon: '🏠', label: '指挥中心', roles: ['admin', 'operator', 'viewer'] },
    { path: '/terminal/tactical', icon: '�', label: '战术 HUD', roles: ['admin', 'operator', 'viewer'] },
    { path: '/terminal/monitor', icon: '�', label: '态势监控', roles: ['admin', 'operator', 'viewer'] },
    { path: '/terminal/gateways', icon: '🌐', label: '网关管理', roles: ['admin', 'operator'] },
    { path: '/terminal/users', icon: '�', label: '用户管理', roles: ['admin'] },
    { path: '/terminal/billing', icon: '💰', label: '计费中心', roles: ['admin'] },
    { path: '/terminal/protocols', icon: '🔧', label: '协议配置', roles: ['admin', 'operator'] },
    { path: '/terminal/threats', icon: '🛡️', label: '威胁分析', roles: ['admin', 'operator', 'viewer'] },
    { path: '/terminal/forensic', icon: '🔍', label: '取证归档', roles: ['admin', 'operator'] },
  ];

  const filteredNavItems = navItems.filter(item => 
    item.roles.includes(userRole)
  );

  return (
    <div className="min-h-screen bg-slate-950 flex">
      {/* 侧边栏 */}
      <aside className={`${collapsed ? 'w-16' : 'w-64'} bg-slate-900 border-r border-slate-800 transition-all duration-300 flex flex-col`}>
        {/* Logo */}
        <div className="p-4 border-b border-slate-800">
          <div className="flex items-center gap-3">
            <div className="text-2xl">🌀</div>
            {!collapsed && (
              <div>
                <h1 className="text-lg font-bold text-white">Mirage-OS</h1>
                <p className="text-xs text-slate-500">Control Center</p>
              </div>
            )}
          </div>
        </div>

        {/* 导航 */}
        <nav className="flex-1 p-2">
          {filteredNavItems.map(item => (
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
                  {connected ? 'Connected' : 'Disconnected'}
                </span>
              </div>
              <div className="text-xs text-slate-500">
                <p>UID: {userId?.slice(0, 8)}...</p>
                <p>Role: {userRole}</p>
              </div>
            </div>
          )}
          
          <button
            onClick={handleLogout}
            className="w-full flex items-center justify-center gap-2 px-3 py-2 bg-red-600/20 hover:bg-red-600/30 text-red-400 rounded-lg transition-colors text-sm"
          >
            <span>🚪</span>
            {!collapsed && <span>Logout</span>}
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
            <div className="flex items-center gap-4">
              <div className="flex items-center gap-2">
                <div className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500 animate-pulse' : 'bg-red-500'}`} />
                <span className="text-sm text-slate-400">
                  {connected ? 'Real-time Sync' : 'Offline Mode'}
                </span>
              </div>
              
              {/* Ghost Mode 开关 */}
              <div className="flex items-center gap-2 ml-4 pl-4 border-l border-slate-700">
                <span className="text-sm text-slate-500">👻 Ghost Mode</span>
                <button
                  onClick={() => setGhostMode(!ghostMode)}
                  className={`relative w-10 h-5 rounded-full transition-colors ${
                    ghostMode ? 'bg-purple-600' : 'bg-slate-700'
                  }`}
                >
                  <div className={`absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform ${
                    ghostMode ? 'left-5' : 'left-0.5'
                  }`} />
                </button>
                {ghostMode && (
                  <span className="text-xs text-purple-400 animate-pulse">阅后即焚</span>
                )}
              </div>
            </div>
            <div className="flex items-center gap-4 text-sm text-slate-400">
              <span>🕐 {new Date().toLocaleTimeString()}</span>
            </div>
          </div>
        </header>

        {/* 页面内容 */}
        <div className="p-6">
          <GhostModeContext.Provider value={{ ghostMode, setGhostMode }}>
            <Outlet />
          </GhostModeContext.Provider>
        </div>
      </main>
    </div>
  );
};

export default AdminLayout;
