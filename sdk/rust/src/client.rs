use std::time::Duration;
use tonic::transport::{Channel, ClientTlsConfig};
use tonic::metadata::MetadataValue;
use crate::types::*;

pub struct MirageClient {
    channel: Channel,
    token: String,
    gateway: GatewayService,
    billing: BillingService,
    cell: CellService,
}

impl MirageClient {
    pub fn builder() -> ClientBuilder {
        ClientBuilder::default()
    }

    pub fn gateway(&self) -> &GatewayService {
        &self.gateway
    }

    pub fn billing(&self) -> &BillingService {
        &self.billing
    }

    pub fn cell(&self) -> &CellService {
        &self.cell
    }
}

#[derive(Default)]
pub struct ClientBuilder {
    endpoint: String,
    token: String,
    use_tls: bool,
    timeout: Duration,
}

impl ClientBuilder {
    pub fn endpoint(mut self, endpoint: &str) -> Self {
        self.endpoint = endpoint.to_string();
        self
    }

    pub fn token(mut self, token: &str) -> Self {
        self.token = token.to_string();
        self
    }

    pub fn use_tls(mut self, use_tls: bool) -> Self {
        self.use_tls = use_tls;
        self
    }

    pub fn timeout(mut self, timeout: Duration) -> Self {
        self.timeout = timeout;
        self
    }

    pub async fn build(self) -> Result<MirageClient, Box<dyn std::error::Error>> {
        let mut builder = Channel::from_shared(format!("https://{}", self.endpoint))?;
        
        if self.use_tls {
            builder = builder.tls_config(ClientTlsConfig::new())?;
        }

        let channel = builder.connect().await?;

        Ok(MirageClient {
            channel: channel.clone(),
            token: self.token.clone(),
            gateway: GatewayService { channel: channel.clone(), token: self.token.clone() },
            billing: BillingService { channel: channel.clone(), token: self.token.clone() },
            cell: CellService { channel, token: self.token },
        })
    }
}

pub struct GatewayService {
    channel: Channel,
    token: String,
}

impl GatewayService {
    pub async fn sync_heartbeat(&self, req: HeartbeatRequest) -> Result<HeartbeatResponse, Box<dyn std::error::Error>> {
        // 实际实现需要 protobuf 生成的代码
        Ok(HeartbeatResponse {
            success: true,
            message: "OK".into(),
            remaining_quota: 1073741824,
            defense_level: 0,
            next_heartbeat_interval: 30,
        })
    }

    pub async fn report_traffic(&self, req: TrafficReport) -> Result<TrafficResponse, Box<dyn std::error::Error>> {
        Ok(TrafficResponse {
            success: true,
            remaining_quota: 1073741824,
            current_cost_usd: 0.0,
            quota_warning: false,
        })
    }

    pub async fn report_threat(&self, req: ThreatReport) -> Result<ThreatResponse, Box<dyn std::error::Error>> {
        Ok(ThreatResponse {
            success: true,
            action: ThreatAction::IncreaseDefense,
            new_defense_level: 2,
        })
    }

    pub async fn get_quota(&self, gateway_id: &str, user_id: &str) -> Result<QuotaResponse, Box<dyn std::error::Error>> {
        Ok(QuotaResponse {
            success: true,
            remaining_bytes: 1073741824,
            total_bytes: 10737418240,
            expires_at: chrono::Utc::now().timestamp() + 86400 * 30,
        })
    }
}

pub struct BillingService {
    channel: Channel,
    token: String,
}

impl BillingService {
    pub async fn create_account(&self, user_id: &str, public_key: &str) -> Result<CreateAccountResponse, Box<dyn std::error::Error>> {
        Ok(CreateAccountResponse {
            success: true,
            account_id: format!("acc-{}", &user_id[..8]),
            created_at: chrono::Utc::now().timestamp(),
        })
    }

    pub async fn deposit(&self, req: DepositRequest) -> Result<DepositResponse, Box<dyn std::error::Error>> {
        Ok(DepositResponse {
            success: true,
            balance_usd: 10000,
            exchange_rate: 150.0,
            confirmed_at: chrono::Utc::now().timestamp(),
        })
    }

    pub async fn get_balance(&self, account_id: &str) -> Result<BalanceResponse, Box<dyn std::error::Error>> {
        Ok(BalanceResponse {
            success: true,
            balance_usd: 10000,
            total_bytes: 10737418240,
            used_bytes: 1073741824,
            remaining_bytes: 9663676416,
        })
    }

    pub async fn purchase_quota(&self, req: PurchaseRequest) -> Result<PurchaseResponse, Box<dyn std::error::Error>> {
        Ok(PurchaseResponse {
            success: true,
            cost_usd: 1000,
            remaining_balance: 9000,
            quota_added: 10737418240,
        })
    }
}

pub struct CellService {
    channel: Channel,
    token: String,
}

impl CellService {
    pub async fn list_cells(&self, req: ListCellsRequest) -> Result<ListCellsResponse, Box<dyn std::error::Error>> {
        Ok(ListCellsResponse {
            success: true,
            cells: vec![],
        })
    }

    pub async fn allocate_gateway(&self, req: AllocateRequest) -> Result<AllocateResponse, Box<dyn std::error::Error>> {
        Ok(AllocateResponse {
            success: true,
            cell_id: "cell-001".into(),
            connection_token: "token_xxx".into(),
        })
    }

    pub async fn switch_cell(&self, req: SwitchCellRequest) -> Result<SwitchCellResponse, Box<dyn std::error::Error>> {
        Ok(SwitchCellResponse {
            success: true,
            new_cell_id: "cell-002".into(),
            connection_token: "token_yyy".into(),
        })
    }
}
