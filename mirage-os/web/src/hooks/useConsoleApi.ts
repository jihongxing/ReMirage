// Console API 钩子 - 对接影子控制台后端
import { useApi, apiPost, apiGet } from './useApi';

// ═══ 类型定义 ═══

export interface UserSummary {
  user_id: string;
  cell_level: number;
  balance_usd: number;
  remaining_quota: number;
  total_quota: number;
  trust_score: number;
  status: string;
  created_at: string;
}

export interface FinanceOverview {
  total_users: number;
  active_users: number;
  total_balance_usd: number;
  total_deposits_usd: number;
  total_quota_bytes: number;
  used_quota_bytes: number;
  pending_deposits: number;
}

export interface DepositItem {
  user_id: string;
  tx_hash: string;
  amount_xmr: number;
  amount_usd: number;
  exchange_rate: number;
  status: string;
  confirmations: number;
  created_at: string;
}

export interface GatewayItem {
  gateway_id: string;
  cell_id: string;
  ip_address: string;
  phase: number;
  is_online: boolean;
  active_connections: number;
  network_quality: number;
  cpu_percent: number;
  memory_bytes: number;
}

export interface QuotaOverview {
  total_allocated_bytes: number;
  total_used_bytes: number;
  total_remaining_bytes: number;
  burn_rate_gb_per_day: number;
  users_near_limit: number;
}

// ═══ 用户管理 ═══

export function useUsers(status?: string) {
  const params = status ? `?status=${status}` : '';
  return useApi<{ users: UserSummary[]; total: number }>(`/v1/users${params}`, 10000);
}

export async function banUser(userId: string, action: 'ban' | 'unban') {
  return apiPost<{ status: string }>('/v1/users/ban', { user_id: userId, action });
}

// ═══ 财务 ═══

export function useFinanceOverview() {
  return useApi<FinanceOverview>('/v1/finance/overview', 15000);
}

export function useDepositHistory() {
  return useApi<{ deposits: DepositItem[] }>('/v1/finance/deposits', 10000);
}

// ═══ 邀请码 ═══

export async function generateInviteCode(creatorUid: string) {
  return apiPost<{ code: string }>('/v1/invitations/generate', { creator_uid: creatorUid });
}

export function useInvitations(userId?: string) {
  const params = userId ? `?user_id=${userId}` : '';
  return useApi<{ invitations: any[] }>(`/v1/invitations/list${params}`, 30000);
}

// ═══ 蜂窝生命周期 ═══

export function useGateways(cellId?: string) {
  const params = cellId ? `?cell_id=${cellId}` : '';
  return useApi<{ gateways: GatewayItem[] }>(`/v1/cells/gateways${params}`, 5000);
}

export async function promoteToCalibration(gatewayId: string) {
  return apiPost<{ status: string }>('/v1/cells/promote-to-calibration', { gateway_id: gatewayId });
}

export async function activateGateway(gatewayId: string, force = false) {
  return apiPost<{ status: string }>('/v1/cells/activate', { gateway_id: gatewayId, force });
}

export async function retireGateway(gatewayId: string) {
  return apiPost<{ status: string }>('/v1/cells/retire', { gateway_id: gatewayId });
}

// ═══ 配额 ═══

export function useQuotaOverview() {
  return useApi<QuotaOverview>('/v1/quota/overview', 10000);
}
