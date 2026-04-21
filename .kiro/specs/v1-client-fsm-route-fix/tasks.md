# 任务清单：Client 连接安全与状态机修复

## 需求 1：建立统一连接状态机

- [x] 1. 连接状态机
  - [x] 1.1 创建 `phantom-client/pkg/gtclient/state.go`：定义 `ConnState` 类型（int32）和枚举常量（StateInit=0 / StateBootstrapping=1 / StateConnected=2 / StateDegraded=3 / StateReconnecting=4 / StateExhausted=5 / StateStopped=6），实现 `String()` 方法返回状态名称
  - [x] 1.2 修改 `phantom-client/pkg/gtclient/client.go` GTunnelClient 结构体：增加 `state atomic.Int32` 字段，移除 `connected bool` 字段
  - [x] 1.3 创建 `phantom-client/pkg/gtclient/client.go` `transition(newState ConnState, reason string)` 方法：使用 `c.state.Swap` 原子切换状态，当旧状态 != 新状态时 `log.Printf("[GTunnel] %s → %s (%s)", old, newState, reason)`
  - [x] 1.4 创建 `phantom-client/pkg/gtclient/client.go` `State() ConnState` 方法：返回 `ConnState(c.state.Load())`
  - [x] 1.5 修改 `phantom-client/pkg/gtclient/client.go` `IsConnected()` 方法：从 `return c.connected` 改为 `return c.State() == StateConnected`
  - [x] 1.6 修改 `phantom-client/pkg/gtclient/client.go` `NewGTunnelClient`：初始化 state 为 `StateInit`
  - [x] 1.7 修改 `phantom-client/pkg/gtclient/client.go` `ProbeAndConnect` 成功时：调用 `c.transition(StateConnected, "bootstrap success")` 替代 `c.connected = true`

## 需求 2：重连收敛为单飞执行

- [x] 2. 单飞重连
  - [x] 2.1 修改 `phantom-client/pkg/gtclient/client.go` GTunnelClient 结构体：增加 `reconnMu sync.Mutex` 字段
  - [x] 2.2 重构 `phantom-client/pkg/gtclient/client.go` `Reconnect` 方法：开头检查 `c.State() == StateStopped` 返回错误；检查 `c.State() == StateReconnecting` 时调用 `c.waitReconnComplete(ctx)` 等待；否则加锁 `reconnMu` 后再次检查状态（double-check），设置 `StateReconnecting`，解锁后调用 `c.doReconnect(ctx)`
  - [x] 2.3 创建 `phantom-client/pkg/gtclient/client.go` `waitReconnComplete(ctx context.Context) error` 方法：100ms 轮询 `c.State()`，当状态不再是 `StateReconnecting` 时返回（StateConnected 返回 nil，其他返回 error），ctx 超时返回 timeout error
  - [x] 2.4 创建 `phantom-client/pkg/gtclient/client.go` `doReconnect(ctx context.Context) error` 方法：包含原有三级降级逻辑（从现有 Reconnect 方法体迁移），成功时调用 `c.transition(StateConnected, ...)`，失败时调用 `c.transition(StateExhausted, ...)`

## 需求 3：探测成功与正式接管连接拆开

- [x] 3. 探测与接管分离
  - [x] 3.1 创建 `phantom-client/pkg/gtclient/client.go` `probeResult` 结构体：`gw token.GatewayEndpoint` + `engine *QUICEngine`
  - [x] 3.2 创建 `phantom-client/pkg/gtclient/client.go` `probeFirst(ctx context.Context, pool []token.GatewayEndpoint) (*probeResult, error)` 方法：为每个 gw 启动 goroutine 创建 QUICEngine 并 Connect，第一个成功的通过 buffered channel(cap=1) 发送结果并 cancel context，后续成功的 engine 直接 Close，超时返回错误
  - [x] 3.3 创建 `phantom-client/pkg/gtclient/client.go` `adoptConnection(result *probeResult)` 方法：加 `c.mu.Lock()`，关闭旧 `c.quic`（if != nil），设置 `c.quic = result.engine`，设置 `c.currentGW = result.gw`，调用 `c.transition(StateConnected, ...)`，解锁
  - [x] 3.4 重构 `phantom-client/pkg/gtclient/client.go` `ProbeAndConnect` 方法：调用 `probeFirst` 获取候选，然后调用 `adoptConnection` 接管；移除原有的在 probe goroutine 内直接设置 `c.quic` 和 `c.currentGW` 的逻辑
  - [x] 3.5 重构 `phantom-client/pkg/gtclient/client.go` `probe` 方法：不再直接修改 `c.quic`，改为返回 `*QUICEngine` 或 error（仅用于内部 probeFirst 调用）

## 需求 4：所有切换路径都触发路由更新

- [x] 4. 路由跟随切换
  - [x] 4.1 修改 `phantom-client/pkg/gtclient/client.go` `doReconnect` 方法：在三级降级逻辑的统一出口处（成功返回前），比较 `oldIP` 与 `c.CurrentGateway().IP`，若不同且 `c.switchFn != nil` 则调用 `c.switchFn(newIP)`
  - [x] 4.2 修改 `phantom-client/pkg/gtclient/client.go` `doReconnect` Level 2 分支：`ProbeAndConnect` 成功后记录新 IP，不再直接 return nil，而是 fall through 到统一路由更新出口
  - [x] 4.3 修改 `phantom-client/pkg/gtclient/client.go` `doReconnect` Level 3 分支：信令共振发现 + ProbeAndConnect 成功后同样 fall through 到统一路由更新出口

## 需求 5：连接切换和路由切换做成原子事务

- [x] 5. 原子事务切换
  - [x] 5.1 修改 `phantom-client/pkg/killswitch/killswitch.go`：增加 `PreAddHostRoute(newGatewayIP string) error` 方法 — 加锁，校验 activated，调用 `ks.platform.AddHostRoute(newGatewayIP, ks.originalGW, ks.originalIface)`
  - [x] 5.2 修改 `phantom-client/pkg/killswitch/killswitch.go`：增加 `CommitSwitch(oldGatewayIP, newGatewayIP string) error` 方法 — 加锁，调用 `ks.platform.DeleteHostRoute(oldGatewayIP)`，更新 `ks.gatewayIP = newGatewayIP`
  - [x] 5.3 修改 `phantom-client/pkg/killswitch/killswitch.go`：增加 `RollbackPreAdd(newGatewayIP string) error` 方法 — 加锁，调用 `ks.platform.DeleteHostRoute(newGatewayIP)`
  - [x] 5.4 修改 `phantom-client/pkg/gtclient/client.go` GTunnelClient 结构体：增加 `switchPreAddFn func(string) error`、`switchCommitFn func(string, string)`、`switchRollbackFn func(string)` 三个回调字段，增加对应 Set 方法
  - [x] 5.5 创建 `phantom-client/pkg/gtclient/client.go` `switchWithTransaction(result *probeResult, oldIP string) error` 方法：实现 ① PreAdd → ② adoptConnection → ③ CommitSwitch 的事务序列，任一步骤失败执行回滚
  - [x] 5.6 修改 `phantom-client/pkg/gtclient/client.go` `doReconnect` 方法：将所有成功路径的 `adoptConnection` + `switchFn` 替换为 `switchWithTransaction` 调用
  - [x] 5.7 修改 `phantom-client/cmd/phantom/main.go`：在 `client.OnGatewaySwitch(...)` 之后增加事务回调注册 — `client.SetSwitchPreAdd(func(newIP string) error { return ks.PreAddHostRoute(newIP) })`、`client.SetSwitchCommit(func(oldIP, newIP string) { ks.CommitSwitch(oldIP, newIP) })`、`client.SetSwitchRollback(func(newIP string) { ks.RollbackPreAdd(newIP) })`
  - [x] 5.8 修改 `phantom-client/cmd/phantom/main.go`：移除原有的 `client.OnGatewaySwitch(func(newIP string) { ks.UpdateGatewayRoute(newIP) })` 回调（已被事务模式替代）

## 需求 6：修复 URI 兑换链路对关键配置的丢失

- [x] 6. URI 兑换修复
  - [x] 6.1 修改 `phantom-client/cmd/phantom/main.go` `redeemFromURI` 函数：确保从服务端 JSON 响应中提取 `psk`（base64 解码为 []byte）、`cert_fingerprint`（string）、`user_id`（string）、`auth_key`（base64 解码为 []byte）字段
  - [x] 6.2 修改 `phantom-client/cmd/phantom/main.go` `redeemFromURI` 函数：增加必填字段校验 — `user_id` 为空返回 `fmt.Errorf("服务端响应缺少 user_id")`，`psk` 为空返回错误，`endpoints` 为空返回错误
  - [x] 6.3 修改 `phantom-client/cmd/phantom/main.go` `redeemFromURI` 函数：使用服务端返回的 `expires_at`（RFC3339 格式）解析为 `time.Time`，不再硬编码 `time.Now().Add(30*24*time.Hour)`
  - [x] 6.4 修改 `phantom-client/cmd/phantom/main.go` `redeemFromURI` 函数：组装 BootstrapConfig 时完整填充所有字段（BootstrapPool / AuthKey / PreSharedKey / CertFingerprint / UserID / ExpiresAt）
