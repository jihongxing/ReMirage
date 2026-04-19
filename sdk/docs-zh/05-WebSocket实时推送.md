# WebSocket 实时推送

## 连接

```
wss://ws.mirage.example:18443
```

## 认证

连接后发送认证消息：

```json
{ "type": "auth", "token": "your_jwt_token" }
```

响应：

```json
{ "type": "auth_success", "expires_at": 1704153600 }
```

---

## 事件类型

### threat - 威胁告警

```json
{
  "event": "threat",
  "data": {
    "gateway_id": "gw-001",
    "threat_type": "ACTIVE_PROBING",
    "source_ip": "1.2.3.4",
    "severity": 8,
    "action": "INCREASE_DEFENSE",
    "timestamp": 1704067200
  }
}
```

### quota_warning - 配额告警

```json
{
  "event": "quota_warning",
  "data": {
    "gateway_id": "gw-001",
    "remaining_bytes": 107374182,
    "remaining_percent": 10,
    "expires_at": 1704153600
  }
}
```

### cell_switch - 蜂窝切换

```json
{
  "event": "cell_switch",
  "data": {
    "gateway_id": "gw-001",
    "old_cell_id": "cell-001",
    "new_cell_id": "cell-002",
    "reason": "THREAT_DETECTED",
    "connection_token": "new_token"
  }
}
```

### defense_update - 防御配置更新

```json
{
  "event": "defense_update",
  "data": {
    "gateway_id": "gw-001",
    "defense_level": 3,
    "jitter_mean_us": 5000,
    "noise_intensity": 50
  }
}
```

### heartbeat - 心跳

```json
{
  "event": "heartbeat",
  "data": { "timestamp": 1704067200 }
}
```

服务端每 30 秒发送，客户端需回复：

```json
{ "event": "heartbeat_ack" }
```

---

## 代码示例

### Python

```python
from mirage import MirageWebSocket

ws = MirageWebSocket("wss://ws.mirage.example:18443", token="your_token")

@ws.on("threat")
def on_threat(data):
    print(f"威胁: {data['threat_type']} from {data['source_ip']}")
    if data['severity'] >= 8:
        # 紧急处理
        pass

@ws.on("quota_warning")
def on_quota(data):
    print(f"配额剩余: {data['remaining_percent']}%")

ws.connect()
```

### Go

```go
ws, _ := mirage.NewWebSocket("wss://ws.mirage.example:18443", mirage.WithToken(token))

ws.On("threat", func(data map[string]interface{}) {
    fmt.Printf("威胁: %v\n", data["threat_type"])
})

ws.On("quota_warning", func(data map[string]interface{}) {
    fmt.Printf("配额剩余: %.0f%%\n", data["remaining_percent"])
})

ws.Connect()
```

### JavaScript

```javascript
const ws = new MirageWebSocket('wss://ws.mirage.example:18443', { token });

ws.on('threat', (data) => {
  console.log(`威胁: ${data.threat_type} from ${data.source_ip}`);
});

ws.on('quota_warning', (data) => {
  console.log(`配额剩余: ${data.remaining_percent}%`);
});

await ws.connect();
```

---

## 订阅过滤

可选择订阅特定事件：

```json
{
  "type": "subscribe",
  "events": ["threat", "quota_warning"]
}
```

取消订阅：

```json
{
  "type": "unsubscribe",
  "events": ["heartbeat"]
}
```
