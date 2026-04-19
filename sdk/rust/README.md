# Mirage Rust SDK

## 安装

```toml
[dependencies]
mirage-sdk = "1.0"
```

## 快速开始

```rust
use mirage_sdk::{MirageClient, HeartbeatRequest};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // 初始化
    let client = MirageClient::builder()
        .endpoint("grpc.mirage.example:50847")
        .token("your_jwt_token")
        .build()
        .await?;

    // 心跳同步
    let resp = client.gateway().sync_heartbeat(HeartbeatRequest {
        gateway_id: "gw-001".into(),
        version: "1.0.0".into(),
        threat_level: 0,
        status: None,
        resource: None,
    }).await?;
    println!("剩余配额: {} bytes", resp.remaining_quota);

    // 查询余额
    let balance = client.billing().get_balance("acc-001").await?;
    println!("余额: ${:.2}", balance.balance_usd as f64 / 100.0);

    // 查询蜂窝
    let cells = client.cell().list_cells(ListCellsRequest {
        level: None,
        country: None,
        online_only: true,
    }).await?;
    for cell in cells.cells {
        println!("{}: {:.1}%", cell.cell_name, cell.load_percent);
    }

    Ok(())
}
```

## WebSocket 实时推送

```rust
use mirage_sdk::MirageWebSocket;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut ws = MirageWebSocket::builder()
        .url("wss://ws.mirage.example:18443")
        .token("your_token")
        .build()?;

    ws.on("threat", |data| {
        println!("威胁告警: {:?}", data);
    });

    ws.on("quota_warning", |data| {
        println!("配额告警: {:?}", data);
    });

    ws.connect().await?;

    Ok(())
}
```

## API 参考

### GatewayService

| 方法 | 说明 |
|------|------|
| `sync_heartbeat(req)` | 心跳同步 |
| `report_traffic(req)` | 流量上报 |
| `report_threat(req)` | 威胁上报 |
| `get_quota(gateway_id, user_id)` | 配额查询 |

### BillingService

| 方法 | 说明 |
|------|------|
| `create_account(user_id, public_key)` | 创建账户 |
| `deposit(req)` | 充值 |
| `get_balance(account_id)` | 查询余额 |
| `purchase_quota(req)` | 购买流量包 |

### CellService

| 方法 | 说明 |
|------|------|
| `list_cells(req)` | 查询蜂窝 |
| `allocate_gateway(req)` | 分配 Gateway |
| `switch_cell(req)` | 切换蜂窝 |
