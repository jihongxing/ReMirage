# 需求文档：Release Readiness P0 Runtime Blockers

## 简介

本文档收敛本轮审计中所有会直接阻断 Mirage V2 上线的运行时问题。目标不是继续扩展能力，而是先恢复一条可构建、可启动、可交付、可切换的最小可信主链路。

本 Spec 处理 5 类 P0 阻断：

1. `mirage-gateway` 主程序不可编译
2. `phantom-client` 主程序不可编译
3. `mirage-os` 与 `phantom-client` 的 topology 契约不兼容
4. `gateway-bridge` 心跳仍写旧版 schema
5. 配置交付链路被错误地按内部接口保护，无法被客户端正常兑换

---

## 需求

### 需求 1：Gateway 主程序必须恢复可构建状态

**用户故事：** 作为发布负责人，我需要 `mirage-gateway/cmd/gateway` 可以稳定编译，以便 Gateway 二进制能进入正式发布流程。

#### 验收标准

1. WHEN `cmd/gateway/main.go` 初始化 `StealthControlPlane` 时，THE Gateway SHALL 传入与 `stealth.EventDispatcher` 签名兼容的 dispatcher 适配层
2. THE `mirage-gateway` 仓库 SHALL 通过 `go test ./... -run '^$'` 或等价构建验证，不再出现 dispatcher 接口不匹配错误
3. THE Gateway 启动入口 SHALL 保留 V2 组件接线意图，但不得以不可编译的接口拼接方式交付

---

### 需求 2：phantom-client 主程序必须恢复可构建状态

**用户故事：** 作为发布负责人，我需要 `phantom-client/cmd/phantom` 可以稳定编译，以便客户端二进制可进入打包与验收。

#### 验收标准

1. WHEN 账号进入 banned 状态时，THE Client SHALL 能通过实际存在的 keyring 句柄删除敏感材料
2. THE `phantom-client` 仓库 SHALL 通过 `go test ./...`，不再出现未定义变量 `kr` 的编译错误
3. THE banned 回调 SHALL 保持可取消运行、可清理密钥、可输出明确日志的行为

---

### 需求 3：Topology 契约必须在 OS 与 Client 之间统一

**用户故事：** 作为运行时系统，我需要 OS 发布的 topology 响应与 Client 的验签和拓扑池结构完全一致，以便故障切换与持续学习真实可用。

#### 验收标准

1. THE OS topology 响应 SHALL 与 `phantom-client/pkg/gtclient/topo.go` 中的 `RouteTableResponse` 结构一致
2. THE topology 响应 SHALL 至少包含 `version`、`published_at`、`gateways`、`signature`
3. THE `gateways` 条目 SHALL 至少对齐 `ip`、`port`、`priority`、`region`、`cell_id`
4. THE OS 与 Client SHALL 使用同一套 HMAC 规范生成和校验 `signature`
5. WHEN Client 拉取到 topology 响应后，THE `TopoVerifier` SHALL 成功验签并更新 `RuntimeTopology`
6. WHEN 当前 Gateway 失效时，THE Client SHALL 能从 runtime topology 中选出下一跳节点，而非停留在 bootstrap 状态

---

### 需求 4：Gateway 心跳与会话清理必须对齐运行时数据库真相

**用户故事：** 作为控制面，我需要 `gateway-bridge` 用统一 schema 记录 Gateway 存活状态，以便 OS 的节点编排、会话清理和拓扑判断可信。

#### 验收标准

1. WHEN `SyncHeartbeat` upsert `gateways` 表时，THE bridge SHALL 使用与 `mirage-os/pkg/models/db.go` 一致的列名
2. THE bridge SHALL 使用 `gateway_id` 作为业务主键，而不是旧版 `id` 字段承载 Gateway 标识
3. THE bridge SHALL 更新 `last_heartbeat_at`、`current_threat_level`、`memory_bytes` 等运行时真相字段
4. WHEN 检测 stale gateways 时，THE bridge SHALL 基于新列名查询超时节点并清理对应会话
5. THE `mirage-os` 相关测试或最小集成验证 SHALL 覆盖 heartbeat upsert 与 stale session cleanup 两条路径

---

### 需求 5：配置交付链路必须从“可保护”变为“可兑换”

**用户故事：** 作为新客户端，我需要用一次性 URI 成功兑换配置，而不是在第一跳就被内部鉴权拦截。

#### 验收标准

1. WHEN 客户端访问 `GET /delivery/:token?key=...` 时，THE 公网兑换入口 SHALL 不要求内部 HMAC 头
2. THE DeliveryController 对 Provisioner 的内部调用 SHALL 继续使用内部鉴权保护
3. THE 默认 `PROVISIONER_URL` SHALL 与实际 Provisioner 监听端口一致
4. THE NestJS 内部端口 SHALL 真正启动，或相关日志与注释不得再宣称已暴露内部监听器
5. WHEN XMR webhook 或 delivery 触发 Provisioner 时，THE 请求 SHALL 能打到真实存在的内部端点

