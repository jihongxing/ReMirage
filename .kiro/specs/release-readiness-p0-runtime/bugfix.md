# Bugfix Requirements Document

## Introduction

Mirage V2 存在 5 个 P0 级别运行时阻断 Bug，直接导致 Gateway 和 Client 无法编译、Topology API 契约不兼容、Heartbeat SQL 列名冲突、以及 Provisioning 交付链路完全不通。这些问题必须在任何其他修复之前全部解决，否则系统无法进入联调阶段。

## Bug Analysis

### Current Behavior (Defect)

1.1 WHEN `mirage-gateway/cmd/gateway/main.go` 将 `events.EventDispatcher`（方法签名 `Dispatch(ctx, *ControlEvent)`）赋值给 `stealth.StealthControlPlaneOpts.Dispatcher`（期望 `Dispatch(ctx, interface{})`）THEN 编译器报错 `cannot use v2Dispatcher as stealth.EventDispatcher value`，Gateway 主程序无法构建

1.2 WHEN `phantom-client/cmd/phantom/main.go` 的 `OnBanned` 回调中引用变量 `kr` 执行 `kr.Delete(keyringService, keyringPSK)` 和 `kr.Delete(keyringService, keyringAuthKey)` THEN 编译器报错 `undefined: kr`，phantom-client 主程序无法构建

1.3 WHEN OS 端 topology handler（`mirage-os/services/topology/handler.go`）返回 `GatewayEntry{gateway_id, ip_address, cell_id, status}` 且 `Signature` 为 hex string、`PublishedAt` 为 RFC3339 string THEN Client 端（`phantom-client/pkg/gtclient/topo.go`）期望 `GatewayNode{ip, port, priority, region, cell_id}` 且 `Signature` 为 `[]byte`、`PublishedAt` 为 `time.Time`，JSON 反序列化失败或字段丢失，拓扑数据无法正确解析

1.4 WHEN OS 端 HMAC 签名使用 `fmt.Sprintf("%d%s%s", version, publishedAt, gwJSON)` 的 hex string 拼接方式计算 THEN Client 端使用 `json.Marshal(hmacBody{Gateways, Version, PublishedAt})` 的 JSON 序列化方式计算，两端 HMAC 永远不匹配，签名验证必然失败

1.5 WHEN `gateway-bridge/pkg/topology/registry.go` 写入 `last_heartbeat_at` 列，而 `gateway-bridge/pkg/grpc/server.go` 写入 `last_heartbeat` 列，`gateway-bridge/pkg/rest/handler.go` 查询 `last_heartbeat` 列 THEN 同一张 `gateways` 表存在两个不同的列名引用，运行时产生 SQL 列不存在错误

1.6 WHEN `gateway-bridge/pkg/grpc/server.go` 使用主键列名 `id` 而 `gateway-bridge/pkg/topology/registry.go` 使用主键列名 `gateway_id` THEN 同一张 `gateways` 表的主键列名不一致，UPSERT 冲突解析行为不可预测

1.7 WHEN `mirage-os/services/provisioning/http_handler.go` 定义了 `/internal/delivery/redeem`、`/internal/delivery/status/`、`/internal/provision` 路由 THEN `gateway-bridge/cmd/bridge/main.go` 中仅挂载了 `rest.Handler` 的路由，provisioning handler 未被注册到任何 HTTP server，交付链路完全不通

1.8 WHEN Client 集成测试（`phantom-client/pkg/gtclient/integration_test.go`）请求 `/api/v1/topology` THEN OS 端注册的路由为 `/api/v2/topology`，测试桩与真实端点路径不一致

### Expected Behavior (Correct)

2.1 WHEN `mirage-gateway/cmd/gateway/main.go` 创建 `StealthControlPlane` 时传入 dispatcher THEN 系统 SHALL 确保 `events.EventDispatcher` 接口与 `stealth.EventDispatcher` 接口兼容（统一方法签名或通过适配器桥接），`cd mirage-gateway && go build ./cmd/gateway/` 成功

2.2 WHEN `phantom-client/cmd/phantom/main.go` 的 `OnBanned` 回调需要清除 keyring 中的敏感材料 THEN 系统 SHALL 正确定义或引入 `kr` 变量（keyring 实例），`cd phantom-client && go build ./cmd/phantom/` 成功

2.3 WHEN OS 端 topology handler 返回路由表响应 THEN 系统 SHALL 使用与 Client 端 `RouteTableResponse` 一致的字段结构（包含 `ip`、`port`、`priority`、`region`、`cell_id`），`Signature` 类型和 `PublishedAt` 类型两端一致，Client 能正确反序列化

2.4 WHEN OS 端和 Client 端分别计算 HMAC 签名 THEN 系统 SHALL 使用相同的 canonical 序列化方式（统一为 JSON marshal 或统一为字符串拼接），确保两端对同一响应体计算出相同的 HMAC 值

2.5 WHEN gateway-bridge 各模块操作 `gateways` 表的 heartbeat 列 THEN 系统 SHALL 统一使用同一个列名（`last_heartbeat_at` 或 `last_heartbeat`），所有 INSERT/UPDATE/SELECT 语句引用一致

2.6 WHEN gateway-bridge 各模块操作 `gateways` 表的主键列 THEN 系统 SHALL 统一使用同一个主键列名（`gateway_id` 或 `id`），所有 UPSERT ON CONFLICT 语句引用一致

2.7 WHEN gateway-bridge 启动时 THEN 系统 SHALL 将 provisioning HTTP handler 的路由注册到 REST HTTP server 的 mux 上，使 `/internal/delivery/redeem`、`/internal/delivery/status/`、`/internal/provision` 端点可访问

2.8 WHEN Client 集成测试请求 topology API THEN 系统 SHALL 使用与 OS 端一致的路径 `/api/v2/topology`

### Unchanged Behavior (Regression Prevention)

3.1 WHEN Gateway 主程序中 V2 EventDispatcher 正常分发 `*ControlEvent` 事件 THEN 系统 SHALL CONTINUE TO 正确执行事件去重、epoch 校验、handler 路由等完整分发流程

3.2 WHEN phantom-client 在非 banned 状态下正常运行（bootstrap、TUN 转发、拓扑刷新、权限检查） THEN 系统 SHALL CONTINUE TO 保持所有现有功能不变

3.3 WHEN Client 端 `TopoVerifier` 对合法路由表执行 HMAC 验签、版本单调递增检查、PublishedAt 反回滚检查 THEN 系统 SHALL CONTINUE TO 正确执行所有验证逻辑

3.4 WHEN gateway-bridge 的 gRPC server 接收 heartbeat 并更新 gateway 状态、威胁等级、连接数等字段 THEN 系统 SHALL CONTINUE TO 正确写入除列名修复外的所有其他字段

3.5 WHEN gateway-bridge 的 REST handler 查询 gateway 列表并返回 JSON THEN 系统 SHALL CONTINUE TO 返回包含 id、cell_id、ip_address、status、ebpf_loaded、threat_level、active_connections、memory_usage_mb 的完整信息

3.6 WHEN provisioning handler 处理 redeem/status/provision 请求 THEN 系统 SHALL CONTINUE TO 保持现有的请求校验、错误处理、响应格式不变

3.7 WHEN gateway-bridge 的 REST API 受 InternalAuthMiddleware 和 AccessLogMiddleware 保护 THEN 系统 SHALL CONTINUE TO 对新挂载的 provisioning 路由同样施加相同的中间件保护
