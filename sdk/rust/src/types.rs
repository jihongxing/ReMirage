use serde::{Deserialize, Serialize};

// Gateway Types
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HeartbeatRequest {
    pub gateway_id: String,
    pub version: String,
    pub threat_level: u32,
    pub status: Option<GatewayStatus>,
    pub resource: Option<ResourceUsage>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GatewayStatus {
    pub online: bool,
    pub active_connections: u32,
    pub uptime_seconds: u64,
    pub cell_id: String,
    pub region: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResourceUsage {
    pub cpu_percent: f32,
    pub memory_bytes: u64,
    pub bandwidth_bps: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HeartbeatResponse {
    pub success: bool,
    pub message: String,
    pub remaining_quota: u64,
    pub defense_level: u32,
    pub next_heartbeat_interval: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TrafficReport {
    pub gateway_id: String,
    pub base_traffic_bytes: u64,
    pub defense_traffic_bytes: u64,
    pub cell_level: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TrafficResponse {
    pub success: bool,
    pub remaining_quota: u64,
    pub current_cost_usd: f32,
    pub quota_warning: bool,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[repr(u8)]
pub enum ThreatType {
    Unknown = 0,
    ActiveProbing = 1,
    Ja4Scan = 2,
    SniProbe = 3,
    DpiInspection = 4,
    TimingAttack = 5,
    ReplayAttack = 6,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ThreatReport {
    pub gateway_id: String,
    pub threat_type: ThreatType,
    pub source_ip: String,
    pub severity: u32,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[repr(u8)]
pub enum ThreatAction {
    None = 0,
    IncreaseDefense = 1,
    BlockIp = 2,
    SwitchCell = 3,
    EmergencyShutdown = 4,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ThreatResponse {
    pub success: bool,
    pub action: ThreatAction,
    pub new_defense_level: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct QuotaResponse {
    pub success: bool,
    pub remaining_bytes: u64,
    pub total_bytes: u64,
    pub expires_at: i64,
}

// Billing Types
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CreateAccountResponse {
    pub success: bool,
    pub account_id: String,
    pub created_at: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DepositRequest {
    pub account_id: String,
    pub tx_hash: String,
    pub amount_xmr: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DepositResponse {
    pub success: bool,
    pub balance_usd: u64,
    pub exchange_rate: f32,
    pub confirmed_at: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BalanceResponse {
    pub success: bool,
    pub balance_usd: u64,
    pub total_bytes: u64,
    pub used_bytes: u64,
    pub remaining_bytes: u64,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[repr(u8)]
pub enum PackageType {
    Package10Gb = 1,
    Package50Gb = 2,
    Package100Gb = 3,
    Package500Gb = 4,
    Package1Tb = 5,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PurchaseRequest {
    pub account_id: String,
    pub package_type: PackageType,
    pub cell_level: String,
    pub quantity: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PurchaseResponse {
    pub success: bool,
    pub cost_usd: u64,
    pub remaining_balance: u64,
    pub quota_added: u64,
}

// Cell Types
#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[repr(u8)]
pub enum CellLevel {
    Standard = 1,
    Platinum = 2,
    Diamond = 3,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ListCellsRequest {
    pub level: Option<CellLevel>,
    pub country: Option<String>,
    pub online_only: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CellInfo {
    pub cell_id: String,
    pub cell_name: String,
    pub level: CellLevel,
    pub country: String,
    pub region: String,
    pub load_percent: f32,
    pub gateway_count: u32,
    pub max_gateways: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ListCellsResponse {
    pub success: bool,
    pub cells: Vec<CellInfo>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AllocateRequest {
    pub user_id: String,
    pub gateway_id: String,
    pub preferred_level: Option<CellLevel>,
    pub preferred_country: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AllocateResponse {
    pub success: bool,
    pub cell_id: String,
    pub connection_token: String,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[repr(u8)]
pub enum SwitchReason {
    UserRequest = 1,
    ThreatDetected = 2,
    CellOverload = 3,
    CellOffline = 4,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SwitchCellRequest {
    pub user_id: String,
    pub gateway_id: String,
    pub current_cell_id: String,
    pub target_cell_id: Option<String>,
    pub reason: SwitchReason,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SwitchCellResponse {
    pub success: bool,
    pub new_cell_id: String,
    pub connection_token: String,
}
