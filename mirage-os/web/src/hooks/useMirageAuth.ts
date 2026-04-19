// Mirage 认证 Hook - 代理到全局 AuthContext
// 保持向后兼容，实际状态由 AuthContext 管理
export { useAuthContext as useMirageAuth } from '../contexts/AuthContext';
export type { UserRole, AuthState } from '../contexts/AuthContext';
