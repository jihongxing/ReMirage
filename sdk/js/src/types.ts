// Gateway Types
export interface HeartbeatRequest {
  gatewayId: string;
  version?: string;
  threatLevel?: number;
  status?: GatewayStatus;
  resource?: ResourceUsage;
}

export interface GatewayStatus {
  online: boolean;
  activeConnections: number;
  uptimeSeconds: number;
  cellId: string;
  region: string;
}

export interface ResourceUsage {
  cpuPercent: number;
  memoryBytes: number;
  bandwidthBps: number;
}

export interface HeartbeatResponse {
  success: boolean;
  message: string;
  remainingQuota: number;
  defenseLevel: number;
  nextHeartbeatInterval: number;
}

export interface TrafficReport {
  gatewayId: string;
  baseTrafficBytes: number;
  defenseTrafficBytes: number;
  cellLevel?: string;
}

export interface TrafficResponse {
  success: boolean;
  remainingQuota: number;
  currentCostUsd: number;
  quotaWarning: boolean;
}

export interface ThreatReport {
  gatewayId: string;
  threatType: ThreatType;
  sourceIp: string;
  severity: number;
}

export enum ThreatType {
  UNKNOWN = 0,
  ACTIVE_PROBING = 1,
  JA4_SCAN = 2,
  SNI_PROBE = 3,
  DPI_INSPECTION = 4,
  TIMING_ATTACK = 5,
  REPLAY_ATTACK = 6,
}

export interface ThreatResponse {
  success: boolean;
  action: ThreatAction;
  newDefenseLevel: number;
}

export enum ThreatAction {
  NONE = 0,
  INCREASE_DEFENSE = 1,
  BLOCK_IP = 2,
  SWITCH_CELL = 3,
  EMERGENCY_SHUTDOWN = 4,
}

export interface QuotaResponse {
  success: boolean;
  remainingBytes: number;
  totalBytes: number;
  expiresAt: number;
}

// Billing Types
export interface CreateAccountResponse {
  success: boolean;
  accountId: string;
  createdAt: number;
}

export interface DepositRequest {
  accountId: string;
  txHash: string;
  amountXmr: number;
}

export interface DepositResponse {
  success: boolean;
  balanceUsd: number;
  exchangeRate: number;
  confirmedAt: number;
}

export interface BalanceResponse {
  success: boolean;
  balanceUsd: number;
  totalBytes: number;
  usedBytes: number;
  remainingBytes: number;
}

export interface PurchaseRequest {
  accountId: string;
  packageType: PackageType;
  cellLevel?: string;
  quantity?: number;
}

export enum PackageType {
  PACKAGE_10GB = 1,
  PACKAGE_50GB = 2,
  PACKAGE_100GB = 3,
  PACKAGE_500GB = 4,
  PACKAGE_1TB = 5,
}

export interface PurchaseResponse {
  success: boolean;
  costUsd: number;
  remainingBalance: number;
  quotaAdded: number;
}

// Cell Types
export interface ListCellsRequest {
  level?: CellLevel;
  country?: string;
  onlineOnly?: boolean;
}

export enum CellLevel {
  STANDARD = 1,
  PLATINUM = 2,
  DIAMOND = 3,
}

export interface CellInfo {
  cellId: string;
  cellName: string;
  level: CellLevel;
  country: string;
  region: string;
  loadPercent: number;
  gatewayCount: number;
  maxGateways: number;
}

export interface ListCellsResponse {
  success: boolean;
  cells: CellInfo[];
}

export interface AllocateRequest {
  userId: string;
  gatewayId: string;
  preferredLevel?: CellLevel;
  preferredCountry?: string;
}

export interface AllocateResponse {
  success: boolean;
  cellId: string;
  connectionToken: string;
}

export interface SwitchCellRequest {
  userId: string;
  gatewayId: string;
  currentCellId: string;
  targetCellId?: string;
  reason?: SwitchReason;
}

export enum SwitchReason {
  USER_REQUEST = 1,
  THREAT_DETECTED = 2,
  CELL_OVERLOAD = 3,
  CELL_OFFLINE = 4,
}

export interface SwitchCellResponse {
  success: boolean;
  newCellId: string;
  connectionToken: string;
}
