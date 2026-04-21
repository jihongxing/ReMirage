# 设计文档：Client 连接安全与状态机修复

## 概述

本设计覆盖 phantom-client 的 6 项紧急修复，核心目标是消除并发重连竞态、路由错位和配置丢失。所有改动集中在 `phantom-client/pkg/gtclient/` 和 `phantom-client/cmd/phantom/main.go`。

## 设计原则

1. **单一真相源**：任意时刻只有一个真实连接状态、一个真实当前 Gateway、一个真实路由状态
2. **先保证正确，再追求性能**：宁可重连慢一点，也不能出现状态错位
3. **原子切换**：连接 + 路由必须同向变化，中间失败必须回滚

---

## 模块 1：连接状态机（需求 1）

### 改动范围

- `phantom-client/pkg/gtclient/state.go`（新建）

### 设计细节

```go
// pkg/gtclient/state.go
package gtclient

import "sync/atomic"

type ConnState int32

const (
    StateInit          ConnState = iota // 初始化
    StateBootstrapping                  // 正在探测 bootstrap 节点
    StateConnected                      // 已连接
    StateDegraded                       // 连接质量下降
    StateReconnecting                   // 正在重连
    StateExhausted                      // 所有重连策略耗尽
    StateStopped                        // 已停止
)

func (s ConnState) String() string {
    // ...
}
```

在 `GTunnelClient` 中增加：

```go
type GTunnelClient struct {
    // ... 现有字段
    state atomic.Int32 // ConnState
}

func (c *GTunnelClient) transition(newState ConnState, reason string) {
    old := ConnState(c.state.Swap(int32(newState)))
    if old != newState {
        log.Printf("[GTunnel] 状态切换: %s → %s (%s)", old, newState, reason)
    }
}

func (c *GTunnelClient) State() ConnState {
    return ConnState(c.state.Load())
}
```

---

## 模块 2：单飞重连（需求 2）

### 改动范围

- `phantom-client/pkg/gtclient/client.go`：Reconnect 方法重构

### 设计细节

使用 `sync.Once` 风格的单飞控制器：

```go
type GTunnelClient struct {
    // ... 现有字段
    reconnMu   sync.Mutex
    reconnOnce *sync.Once // 每次重连周期重置
}

func (c *GTunnelClient) Reconnect(ctx context.Context) error {
    // 状态检查：已停止或已连接则跳过
    if c.State() == StateStopped {
        return fmt.Errorf("client stopped")
    }
    if c.State() == StateConnected {
        return nil
    }

    // 单飞：如果已在重连中，等待结果
    c.reconnMu.Lock()
    if c.State() == StateReconnecting {
        c.reconnMu.Unlock()
        // 等待当前重连完成（轮询状态）
        return c.waitReconnComplete(ctx)
    }
    c.transition(StateReconnecting, "reconnect triggered")
    c.reconnMu.Unlock()

    // 执行实际重连逻辑
    err := c.doReconnect(ctx)
    if err != nil {
        c.transition(StateExhausted, err.Error())
    }
    return err
}
```

`doReconnect` 包含原有的三级降级逻辑。

---

## 模块 3：探测与接管分离（需求 3）

### 改动范围

- `phantom-client/pkg/gtclient/client.go`：ProbeAndConnect 重构

### 设计细节

将 `ProbeAndConnect` 拆成两步：

```go
// probeResult 探测结果
type probeResult struct {
    gw     token.GatewayEndpoint
    engine *QUICEngine
}

// probeFirst 并发探测，返回第一个成功的候选
func (c *GTunnelClient) probeFirst(ctx context.Context, pool []token.GatewayEndpoint) (*probeResult, error) {
    probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    ch := make(chan *probeResult, 1) // 只接收第一个

    for _, gw := range pool {
        go func(gw token.GatewayEndpoint) {
            engine := NewQUICEngine(&QUICEngineConfig{GatewayAddr: fmt.Sprintf("%s:%d", gw.IP, gw.Port)})
            if err := engine.Connect(probeCtx); err != nil {
                return
            }
            select {
            case ch <- &probeResult{gw: gw, engine: engine}:
                cancel() // 取消其余探测
            default:
                engine.Close() // 已有胜者，关闭多余连接
            }
        }(gw)
    }

    select {
    case result := <-ch:
        return result, nil
    case <-probeCtx.Done():
        return nil, fmt.Errorf("all probes failed or timeout")
    }
}

// adoptConnection 原子接管连接
func (c *GTunnelClient) adoptConnection(result *probeResult) {
    c.mu.Lock()
    defer c.mu.Unlock()

    // 关闭旧连接
    if c.quic != nil {
        c.quic.Close()
    }

    // 原子设置新连接
    c.quic = result.engine
    c.currentGW = result.gw
    c.connected = true
    c.transition(StateConnected, fmt.Sprintf("adopted %s", result.gw.IP))
}
```

---

## 模块 4：路由跟随切换（需求 4）

### 改动范围

- `phantom-client/pkg/gtclient/client.go`：Reconnect 所有路径统一触发 switchFn

### 设计细节

在 `doReconnect` 中，所有三级降级成功后统一调用 switchFn：

```go
func (c *GTunnelClient) doReconnect(ctx context.Context) error {
    oldIP := c.CurrentGateway().IP

    // Level 1: RouteTable
    // Level 2: Bootstrap Pool
    // Level 3: Resonance

    // 统一路由更新（无论哪一级成功）
    newIP := c.CurrentGateway().IP
    if newIP != oldIP && c.switchFn != nil {
        c.switchFn(newIP)
    }
    return nil
}
```

关键改动：当前代码中 Level 2 的 `ProbeAndConnect` 成功后没有调用 `switchFn`，Level 3 也没有。统一在 `doReconnect` 末尾处理。

---

## 模块 5：原子事务切换（需求 5）

### 改动范围

- `phantom-client/pkg/killswitch/killswitch.go`：增加 PreAddHostRoute / CommitSwitch / RollbackPreAdd
- `phantom-client/pkg/gtclient/client.go`：切换序列改为事务模式

### 设计细节

#### KillSwitch 新增方法

```go
// PreAddHostRoute 预加新网关路由（不删除旧路由）
func (ks *KillSwitch) PreAddHostRoute(newGatewayIP string) error {
    ks.mu.Lock()
    defer ks.mu.Unlock()
    if !ks.activated {
        return fmt.Errorf("kill switch not activated")
    }
    return ks.platform.AddHostRoute(newGatewayIP, ks.originalGW, ks.originalIface)
}

// CommitSwitch 提交切换（删除旧路由，更新内部状态）
func (ks *KillSwitch) CommitSwitch(oldGatewayIP, newGatewayIP string) error {
    ks.mu.Lock()
    defer ks.mu.Unlock()
    _ = ks.platform.DeleteHostRoute(oldGatewayIP)
    ks.gatewayIP = newGatewayIP
    return nil
}

// RollbackPreAdd 回滚预加的路由
func (ks *KillSwitch) RollbackPreAdd(newGatewayIP string) error {
    ks.mu.Lock()
    defer ks.mu.Unlock()
    return ks.platform.DeleteHostRoute(newGatewayIP)
}
```

#### GTunnelClient 事务切换

```go
func (c *GTunnelClient) switchWithTransaction(result *probeResult, oldIP string) error {
    newIP := result.gw.IP
    if newIP == oldIP {
        c.adoptConnection(result)
        return nil
    }

    // Step 1: 预加新路由
    if c.switchPreAddFn != nil {
        if err := c.switchPreAddFn(newIP); err != nil {
            result.engine.Close()
            return fmt.Errorf("pre-add route failed: %w", err)
        }
    }

    // Step 2: 接管连接
    c.adoptConnection(result)

    // Step 3: 提交路由（删除旧路由）
    if c.switchCommitFn != nil {
        c.switchCommitFn(oldIP, newIP)
    }

    return nil
}
```

main.go 中注册事务回调：

```go
client.SetSwitchPreAdd(func(newIP string) error {
    return ks.PreAddHostRoute(newIP)
})
client.SetSwitchCommit(func(oldIP, newIP string) {
    ks.CommitSwitch(oldIP, newIP)
})
client.SetSwitchRollback(func(newIP string) {
    ks.RollbackPreAdd(newIP)
})
```

---

## 模块 6：URI 兑换修复（需求 6）

### 改动范围

- `phantom-client/cmd/phantom/main.go`：redeemFromURI 函数

### 设计细节

当前 `redeemFromURI` 从服务端 JSON 响应中提取字段后重新组装 BootstrapConfig。问题在于：
- 可能丢失 PreSharedKey、CertFingerprint、UserID
- ExpiresAt 可能被硬编码为 30 天

修复方案：

```go
func redeemFromURI(uri string) (tokenB64 string, decryptKey []byte, err error) {
    // ... 现有 HTTP 请求逻辑 ...

    // 解析服务端响应
    var resp struct {
        Endpoints []struct {
            IP   string `json:"ip"`
            Port int    `json:"port"`
            Region string `json:"region"`
        } `json:"endpoints"`
        PSK             string `json:"psk"`              // base64
        CertFingerprint string `json:"cert_fingerprint"`
        UserID          string `json:"user_id"`
        ExpiresAt       string `json:"expires_at"`       // RFC3339
        AuthKey         string `json:"auth_key"`         // base64
    }

    // 校验必填字段
    if resp.UserID == "" {
        return "", nil, fmt.Errorf("服务端响应缺少 user_id")
    }
    if resp.PSK == "" {
        return "", nil, fmt.Errorf("服务端响应缺少 psk")
    }
    if len(resp.Endpoints) == 0 {
        return "", nil, fmt.Errorf("服务端响应缺少 endpoints")
    }

    // 使用服务端真实 ExpiresAt
    expiresAt, err := time.Parse(time.RFC3339, resp.ExpiresAt)
    if err != nil {
        return "", nil, fmt.Errorf("无效的 expires_at: %w", err)
    }

    // 组装完整 BootstrapConfig
    config := &token.BootstrapConfig{
        BootstrapPool:   pool,
        PreSharedKey:    pskBytes,
        CertFingerprint: resp.CertFingerprint,
        UserID:          resp.UserID,
        AuthKey:         authKeyBytes,
        ExpiresAt:       expiresAt,
    }

    // 编码为 token
    // ...
}
```

---

## 不在本次范围内

- 拓扑学习（TOPO-1~5）→ Spec 3-1
- 订阅托管（SUB-2~5）→ Spec 3-1
- 后台服务化（SVC-1~5）→ Spec 3-1
- 去掉 8.8.8.8:53 硬编码（ROUTE-3）→ Spec 3-1
- 路由回滚保护（ROUTE-4）→ Spec 3-1
- 路由一致性自检（ROUTE-5）→ Spec 3-1
