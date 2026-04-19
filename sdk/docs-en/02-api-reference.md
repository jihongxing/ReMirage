# API Reference

## Service Overview

| Service | Port | Protocol | Purpose |
|---------|------|----------|---------|
| Gateway | 50847 | gRPC | Heartbeat/Traffic/Threat reporting |
| Cell | 50847 | gRPC | Cell management |
| Billing | 50847 | gRPC | Billing/Deposit |
| WebSocket | 18443 | WSS | Real-time push |

---

## GatewayService

### SyncHeartbeat

Gateway periodically reports status and receives latest configuration.

**Request Parameters**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| gateway_id | string | ✅ | Gateway unique identifier |
| version | string | ✅ | Gateway version |
| threat_level | uint32 | ❌ | Current threat level (0-5) |
| status | GatewayStatus | ❌ | Gateway status |
| resource | ResourceUsage | ❌ | Resource usage |

**Response Parameters**

| Field | Type | Description |
|-------|------|-------------|
| success | bool | Success status |
| remaining_quota | uint64 | Remaining quota (bytes) |
| defense_config | DefenseConfig | New defense configuration |
| next_heartbeat_interval | int64 | Next heartbeat interval (seconds) |

**Call Frequency**: 30 seconds

---

### ReportTraffic

Report traffic consumption for billing.

**Request Parameters**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| gateway_id | string | ✅ | Gateway ID |
| base_traffic_bytes | uint64 | ✅ | Business traffic (bytes) |
| defense_traffic_bytes | uint64 | ✅ | Defense traffic (bytes) |
| cell_level | string | ❌ | Cell level |

**Response Parameters**

| Field | Type | Description |
|-------|------|-------------|
| success | bool | Success status |
| remaining_quota | uint64 | Remaining quota |
| current_cost_usd | float | Current cost (USD) |
| quota_warning | bool | Quota warning (< 10%) |

**Call Frequency**: 10 seconds

---

### ReportThreat

Real-time threat reporting.

**Request Parameters**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| gateway_id | string | ✅ | Gateway ID |
| threat_type | ThreatType | ✅ | Threat type |
| source_ip | string | ✅ | Source IP |
| severity | uint32 | ✅ | Severity (0-10) |

**ThreatType**

| Value | Description |
|-------|-------------|
| ACTIVE_PROBING | Active probing |
| JA4_SCAN | JA4 fingerprint scan |
| SNI_PROBE | SNI probe |
| DPI_INSPECTION | DPI deep inspection |
| TIMING_ATTACK | Timing attack |
| REPLAY_ATTACK | Replay attack |

---

## BillingService

### CreateAccount

**Request Parameters**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| user_id | string | ✅ | User ID (anonymous hash) |
| public_key | string | ✅ | Public key (for verification) |

### GetBalance

**Request Parameters**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| account_id | string | ✅ | Account ID |

**Response Parameters**

| Field | Type | Description |
|-------|------|-------------|
| balance_usd | uint64 | Balance (cents) |
| total_bytes | uint64 | Total quota |
| used_bytes | uint64 | Used traffic |
| remaining_bytes | uint64 | Remaining traffic |

### PurchaseQuota

**PackageType**

| Value | Capacity |
|-------|----------|
| PACKAGE_10GB | 10 GB |
| PACKAGE_50GB | 50 GB |
| PACKAGE_100GB | 100 GB |
| PACKAGE_500GB | 500 GB |
| PACKAGE_1TB | 1 TB |

---

## CellService

### ListCells

**CellLevel**

| Value | Description | Cost Multiplier |
|-------|-------------|-----------------|
| STANDARD | Standard cell | 1.0x |
| PLATINUM | Platinum cell | 1.5x |
| DIAMOND | Diamond cell | 2.0x |

### SwitchCell

**SwitchReason**

| Value | Description |
|-------|-------------|
| USER_REQUEST | User initiated |
| THREAT_DETECTED | Threat detected |
| CELL_OVERLOAD | Cell overload |
| CELL_OFFLINE | Cell offline |
