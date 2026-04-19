# WebSocket Real-time Push

## Connection

```
wss://ws.mirage.example:18443
```

## Authentication

Send auth message after connection:

```json
{ "type": "auth", "token": "your_jwt_token" }
```

Response:

```json
{ "type": "auth_success", "expires_at": 1704153600 }
```

---

## Event Types

### threat - Threat Alert

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

### quota_warning - Quota Warning

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

### cell_switch - Cell Switch

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

### defense_update - Defense Config Update

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

---

## Code Examples

### Python

```python
from mirage import MirageWebSocket

ws = MirageWebSocket("wss://ws.mirage.example:18443", token="your_token")

@ws.on("threat")
def on_threat(data):
    print(f"Threat: {data['threat_type']} from {data['source_ip']}")

@ws.on("quota_warning")
def on_quota(data):
    print(f"Quota remaining: {data['remaining_percent']}%")

ws.connect()
```

### JavaScript

```javascript
const ws = new MirageWebSocket('wss://ws.mirage.example:18443', { token });

ws.on('threat', (data) => {
  console.log(`Threat: ${data.threat_type} from ${data.source_ip}`);
});

await ws.connect();
```
