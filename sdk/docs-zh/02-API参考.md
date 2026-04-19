# API 参考

## 服务概览

| 服务 | 端口 | 协议 | 用途 |
|------|------|------|------|
| Gateway | 50847 | gRPC | 心跳/流量/威胁上报 |
| Cell | 50847 | gRPC | 蜂窝管理 |
| Billing | 50847 | gRPC | 计费/充值 |
| WebSocket | 18443 | WSS | 实时推送 |

---

## GatewayService

### SyncHeartbeat - 心跳同步

Gateway 定期上报状态，获取最新配置。

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| gateway_id | string | ✅ | Gateway 唯一标识 |
| version | string | ✅ | Gateway 版本 |
| threat_level | uint32 | ❌ | 当前威胁等级 (0-5) |
| status | GatewayStatus | ❌ | Gateway 状态 |
| resource | ResourceUsage | ❌ | 资源使用情况 |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| success | bool | 是否成功 |
| remaining_quota | uint64 | 剩余配额（字节） |
| defense_config | DefenseConfig | 新的防御配置 |
| next_heartbeat_interval | int64 | 下次心跳间隔（秒） |

**调用频率**: 30秒/次

---

### ReportTraffic - 流量上报

上报流量消耗，用于计费。

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| gateway_id | string | ✅ | Gateway ID |
| base_traffic_bytes | uint64 | ✅ | 业务流量（字节） |
| defense_traffic_bytes | uint64 | ✅ | 防御流量（字节） |
| cell_level | string | ❌ | 蜂窝等级 |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| success | bool | 是否成功 |
| remaining_quota | uint64 | 剩余配额 |
| current_cost_usd | float | 当前费用（美元） |
| quota_warning | bool | 配额告警（< 10%） |

**调用频率**: 10秒/次

---

### ReportThreat - 威胁上报

实时上报检测到的威胁。

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| gateway_id | string | ✅ | Gateway ID |
| threat_type | ThreatType | ✅ | 威胁类型 |
| source_ip | string | ✅ | 源 IP |
| severity | uint32 | ✅ | 严重程度 (0-10) |

**威胁类型 (ThreatType)**

| 值 | 说明 |
|------|------|
| ACTIVE_PROBING | 主动探测 |
| JA4_SCAN | JA4 指纹扫描 |
| SNI_PROBE | SNI 探测 |
| DPI_INSPECTION | DPI 深度检测 |
| TIMING_ATTACK | 时序攻击 |
| REPLAY_ATTACK | 重放攻击 |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| success | bool | 是否成功 |
| action | ThreatAction | 建议动作 |
| new_defense_level | uint32 | 新的防御等级 |

---

### GetQuota - 配额查询

查询当前配额状态。

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| gateway_id | string | ✅ | Gateway ID |
| user_id | string | ✅ | 用户 ID |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| remaining_bytes | uint64 | 剩余流量 |
| total_bytes | uint64 | 总配额 |
| expires_at | int64 | 过期时间（Unix 秒） |

---

## BillingService

### CreateAccount - 创建账户

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | string | ✅ | 用户 ID（匿名哈希） |
| public_key | string | ✅ | 公钥（用于验证） |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| account_id | string | 账户 ID |
| created_at | int64 | 创建时间 |

---

### Deposit - 充值

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| account_id | string | ✅ | 账户 ID |
| tx_hash | string | ✅ | Monero 交易哈希 |
| amount_xmr | uint64 | ✅ | 充值金额（皮摩尔） |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| balance_usd | uint64 | 余额（美分） |
| exchange_rate | float | 汇率（XMR/USD） |
| confirmed_at | int64 | 确认时间 |

---

### GetBalance - 查询余额

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| account_id | string | ✅ | 账户 ID |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| balance_usd | uint64 | 余额（美分） |
| total_bytes | uint64 | 总配额 |
| used_bytes | uint64 | 已用流量 |
| remaining_bytes | uint64 | 剩余流量 |

---

### PurchaseQuota - 购买流量包

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| account_id | string | ✅ | 账户 ID |
| package_type | PackageType | ✅ | 流量包类型 |
| cell_level | string | ❌ | 蜂窝等级 |
| quantity | uint32 | ❌ | 购买数量 |

**流量包类型 (PackageType)**

| 值 | 容量 |
|------|------|
| PACKAGE_10GB | 10 GB |
| PACKAGE_50GB | 50 GB |
| PACKAGE_100GB | 100 GB |
| PACKAGE_500GB | 500 GB |
| PACKAGE_1TB | 1 TB |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| cost_usd | uint64 | 费用（美分） |
| remaining_balance | uint64 | 剩余余额 |
| quota_added | uint64 | 增加的配额 |
| expires_at | int64 | 过期时间 |

---

## CellService

### ListCells - 查询蜂窝

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| level | CellLevel | ❌ | 筛选等级 |
| country | string | ❌ | 筛选国家 |
| online_only | bool | ❌ | 仅在线蜂窝 |

**蜂窝等级 (CellLevel)**

| 值 | 说明 | 成本倍率 |
|------|------|---------|
| STANDARD | 标准蜂窝 | 1.0x |
| PLATINUM | 白金蜂窝 | 1.5x |
| DIAMOND | 钻石蜂窝 | 2.0x |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| cells | []CellInfo | 蜂窝列表 |

---

### AllocateGateway - 分配 Gateway

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | string | ✅ | 用户 ID |
| gateway_id | string | ✅ | Gateway ID |
| preferred_level | CellLevel | ❌ | 偏好等级 |
| preferred_country | string | ❌ | 偏好国家 |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| cell_id | string | 分配的蜂窝 ID |
| connection_token | string | 连接令牌 |

---

### SwitchCell - 切换蜂窝

**请求参数**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | string | ✅ | 用户 ID |
| gateway_id | string | ✅ | Gateway ID |
| current_cell_id | string | ✅ | 当前蜂窝 ID |
| target_cell_id | string | ❌ | 目标蜂窝 ID |
| reason | SwitchReason | ✅ | 切换原因 |

**切换原因 (SwitchReason)**

| 值 | 说明 |
|------|------|
| USER_REQUEST | 用户主动切换 |
| THREAT_DETECTED | 威胁检测 |
| CELL_OVERLOAD | 蜂窝过载 |
| CELL_OFFLINE | 蜂窝离线 |

**响应参数**

| 字段 | 类型 | 说明 |
|------|------|------|
| new_cell_id | string | 新蜂窝 ID |
| connection_token | string | 新连接令牌 |
