import { NavLink, Outlet } from 'react-router-dom';

const navItems = [
  { to: '/',          label: 'Dashboard',  icon: '📊' },
  { to: '/gateways',  label: 'Gateways',   icon: '🌐' },
  { to: '/cells',     label: 'Cells',      icon: '🔷' },
  { to: '/billing',   label: 'Billing',    icon: '💰' },
  { to: '/threats',   label: 'Threats',    icon: '🛡️' },
  { to: '/strategy',  label: 'Strategy',   icon: '⚙️' },
];

export function Layout() {
  return (
    <div className="flex h-screen bg-slate-950 text-white">
      <aside className="w-56 bg-slate-900 border-r border-slate-800 flex flex-col">
        <div className="px-4 py-5 border-b border-slate-800">
          <h1 className="text-lg font-bold">Mirage-OS</h1>
          <p className="text-xs text-slate-500 mt-1">控制台</p>
        </div>
        <nav className="flex-1 py-4" aria-label="主导航">
          {navItems.map(item => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                `flex items-center gap-3 px-4 py-2.5 text-sm transition-colors ${
                  isActive
                    ? 'bg-slate-800 text-cyan-400 border-r-2 border-cyan-400'
                    : 'text-slate-400 hover:text-white hover:bg-slate-800/50'
                }`
              }
            >
              <span>{item.icon}</span>
              <span>{item.label}</span>
            </NavLink>
          ))}
        </nav>
      </aside>
      <main className="flex-1 overflow-auto p-6">
        <Outlet />
      </main>
    </div>
  );
}
