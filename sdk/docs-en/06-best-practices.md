# Best Practices

## Connection Management

### Reuse Connections

```python
# ✅ Correct: Reuse client
client = MirageClient(endpoint, token)
for i in range(100):
    client.gateway.sync_heartbeat(request)
client.close()

# ❌ Wrong: Create new connection each time
for i in range(100):
    client = MirageClient(endpoint, token)
    client.gateway.sync_heartbeat(request)
    client.close()
```

---

## Heartbeat Strategy

### Adaptive Interval

```python
class HeartbeatManager:
    def __init__(self, client):
        self.client = client
        self.interval = 30  # Default 30 seconds
    
    def run(self):
        while True:
            response = self.client.gateway.sync_heartbeat(request)
            # Use server-returned interval
            self.interval = response.next_heartbeat_interval
            time.sleep(self.interval)
```

---

## Quota Management

### Warning Thresholds

```python
def check_quota(response):
    remaining_percent = response.remaining_bytes / response.total_bytes * 100
    
    if remaining_percent < 5:
        # Critical: Pause non-essential traffic
        pause_non_essential()
    elif remaining_percent < 10:
        # Warning: Notify admin
        notify_admin()
    elif remaining_percent < 20:
        # Reminder: Consider purchasing
        log_warning("Quota running low")
```

---

## Threat Response

### Tiered Handling

```python
def handle_threat(threat_data):
    severity = threat_data['severity']
    
    if severity >= 9:
        # Critical: Switch cell immediately
        client.cell.switch_cell(SwitchCellRequest(
            reason=SwitchReason.THREAT_DETECTED
        ))
    elif severity >= 7:
        # High: Increase defense level
        pass
    elif severity >= 5:
        # Medium: Log and monitor
        log_threat(threat_data)
    else:
        # Low: Log only
        log_info(threat_data)
```

---

## Cell Selection

### Proximity Principle

```python
def select_best_cell(cells, user_country):
    # 1. Prefer same country
    same_country = [c for c in cells if c.country == user_country]
    if same_country:
        cells = same_country
    
    # 2. Select lowest load
    cells.sort(key=lambda c: c.load_percent)
    
    # 3. Exclude high load
    cells = [c for c in cells if c.load_percent < 80]
    
    return cells[0] if cells else None
```

---

## Logging

```python
import logging

logger = logging.getLogger("mirage")

# Request log
logger.info("request", extra={
    "method": "SyncHeartbeat",
    "gateway_id": gateway_id,
    "duration_ms": duration
})

# Error log
logger.error("request_failed", extra={
    "method": "SyncHeartbeat",
    "error_code": error.code(),
    "error_message": str(error)
})
```
