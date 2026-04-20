// Mirage 路由器 - 隐藏路由与认证保护层
import { useState, useEffect, lazy, Suspense } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { useMirageAuth } from './hooks/useMirageAuth';
import ShadowCover from './components/ShadowCover';
import BreachPortal from './components/BreachPortal';
import AdminLayout from './components/AdminLayout';
import UserLayout from './components/UserLayout';
import UserPortal from './pages/UserPortal';

// 懒加载管理页面（只有认证后才加载代码包）
const MonitorDashboard = lazy(() => import('./pages/MonitorDashboard'));
const GatewayManager = lazy(() => import('./pages/GatewayManager'));
const UserManager = lazy(() => import('./pages/UserManager'));
const BillingCenter = lazy(() => import('./pages/BillingCenter'));
const ProtocolConfig = lazy(() => import('./pages/ProtocolConfig'));
const ThreatAnalysis = lazy(() => import('./pages/ThreatAnalysis'));
const TacticalHUD = lazy(() => import('./pages/TacticalHUD'));
const CommandCenter = lazy(() => import('./pages/CommandCenter'));
const ForensicCenter = lazy(() => import('./pages/ForensicCenter'));
const CellOrchestrator = lazy(() => import('./pages/CellOrchestrator'));

// 懒加载用户页面
const UserDashboard = lazy(() => import('./pages/UserDashboard'));
const XMRDeposit = lazy(() => import('./pages/XMRDeposit'));
const SubscriptionStore = lazy(() => import('./pages/SubscriptionStore'));

// 加载占位
const LoadingFallback = () => (
  <div className="min-h-screen bg-slate-950 flex items-center justify-center">
    <div className="text-center">
      <div className="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin mx-auto mb-4" />
      <p className="text-slate-400 text-sm">Loading module...</p>
    </div>
  </div>
);

const MirageRouter = () => {
  const { isAuthenticated, breachTriggered, accessTriggered } = useMirageAuth();
  const [showAdminPortal, setShowAdminPortal] = useState(false);
  const [showUserPortal, setShowUserPortal] = useState(false);
  const [userAuthenticated, setUserAuthenticated] = useState(false);

  // 检查用户 vault
  useEffect(() => {
    const vault = localStorage.getItem('mirage_vault');
    if (vault) {
      setUserAuthenticated(true);
    }
  }, []);

  // 监听 breach 触发（管理员）
  useEffect(() => {
    if (breachTriggered && !isAuthenticated) {
      setShowAdminPortal(true);
      setShowUserPortal(false);
    }
  }, [breachTriggered, isAuthenticated]);

  // 监听 access 触发（用户）
  useEffect(() => {
    if (accessTriggered && !userAuthenticated) {
      setShowUserPortal(true);
      setShowAdminPortal(false);
    }
  }, [accessTriggered, userAuthenticated]);

  // 开发模式：自动进入管理界面（仅开发环境）
  useEffect(() => {
    if (import.meta.env.DEV && !isAuthenticated) {
      // 开发环境自动触发认证
      setShowAdminPortal(true);
    }
  }, [isAuthenticated]);

  // 管理员已认证：显示管理界面
  if (isAuthenticated) {
    return (
      <Suspense fallback={<LoadingFallback />}>
        <Routes>
          <Route path="/terminal" element={<AdminLayout />}>
            <Route index element={<Navigate to="command" replace />} />
            <Route path="command" element={<CommandCenter />} />
            <Route path="tactical" element={<TacticalHUD />} />
            <Route path="monitor" element={<MonitorDashboard />} />
            <Route path="gateways" element={<GatewayManager />} />
            <Route path="users" element={<UserManager />} />
            <Route path="billing" element={<BillingCenter />} />
            <Route path="protocols" element={<ProtocolConfig />} />
            <Route path="threats" element={<ThreatAnalysis />} />
            <Route path="forensic" element={<ForensicCenter />} />
            <Route path="cells" element={<CellOrchestrator />} />
          </Route>
          <Route path="*" element={<Navigate to="/terminal/command" replace />} />
        </Routes>
      </Suspense>
    );
  }

  // 用户已认证：显示用户界面
  if (userAuthenticated) {
    return (
      <Suspense fallback={<LoadingFallback />}>
        <Routes>
          <Route path="/portal" element={<UserLayout />}>
            <Route index element={<Navigate to="dashboard" replace />} />
            <Route path="dashboard" element={<UserDashboard />} />
            <Route path="deposit" element={<XMRDeposit />} />
            <Route path="plans" element={<SubscriptionStore />} />
          </Route>
          <Route path="*" element={<Navigate to="/portal/dashboard" replace />} />
        </Routes>
      </Suspense>
    );
  }

  // 显示管理员认证门户
  if (showAdminPortal) {
    return <BreachPortal onSuccess={() => {}} />;
  }

  // 显示用户认证门户
  if (showUserPortal) {
    return (
      <UserPortal
        onAuthenticated={(_uid) => {
          setUserAuthenticated(true);
        }}
      />
    );
  }

  // 未认证：显示影子页面
  return (
    <Routes>
      <Route path="*" element={<ShadowCover />} />
    </Routes>
  );
};

export default MirageRouter;
