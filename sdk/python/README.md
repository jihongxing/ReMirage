# Mirage Python SDK

## 安装

```bash
pip install mirage-sdk
# 或从源码
pip install -e sdk/python/
```

## 快速开始

```python
from mirage import MirageClient

# 初始化
client = MirageClient(
    endpoint="grpc.mirage.example:50847",
    token="your_jwt_token"
)

# 心跳同步
response = client.gateway.sync_heartbeat(
    gateway_id="gw-001",
    version="1.0.0",
    threat_level=0
)
print(f"剩余配额: {response.remaining_quota} bytes")

# 查询余额
balance = client.billing.get_balance(account_id="acc-001")
print(f"余额: ${balance.balance_usd / 100:.2f}")

# 查询蜂窝
cells = client.cell.list_cells(online_only=True)
for cell in cells.cells:
    print(f"{cell.cell_name}: {cell.load_percent}%")
```

## WebSocket 实时推送

```python
from mirage import MirageWebSocket

ws = MirageWebSocket("wss://ws.mirage.example:18443", token="your_token")

@ws.on("threat")
def on_threat(data):
    print(f"威胁告警: {data}")

@ws.on("quota_warning")
def on_quota(data):
    print(f"配额告警: {data}")

ws.connect()
```

## API 参考

### GatewayService

| 方法 | 说明 |
|------|------|
| `sync_heartbeat()` | 心跳同步 |
| `report_traffic()` | 流量上报 |
| `report_threat()` | 威胁上报 |
| `get_quota()` | 配额查询 |

### BillingService

| 方法 | 说明 |
|------|------|
| `create_account()` | 创建账户 |
| `deposit()` | 充值 |
| `get_balance()` | 查询余额 |
| `purchase_quota()` | 购买流量包 |

### CellService

| 方法 | 说明 |
|------|------|
| `list_cells()` | 查询蜂窝 |
| `allocate_gateway()` | 分配 Gateway |
| `switch_cell()` | 切换蜂窝 |
