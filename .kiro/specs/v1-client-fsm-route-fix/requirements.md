# 需求文档：Client 连接安全与状态机修复

## 简介

本 Spec 对应 `Client 鲁棒性整改清单.md` 第一阶段（"先把会不会乱修掉"），目标是消灭 phantom-client 中最高风险的连接错位、路由错位和配置丢失问题。

当前 Client 的核心问题：
1. **并发重连竞态**：`forwardTUNToTunnel` 和 `forwardTunnelToTUN` 两个 goroutine 都可能同时调用 `client.Reconnect(ctx)`，导致并发重连和状态覆盖
2. **探测覆盖连接**：`ProbeAndConnect` 中多个探测 goroutine 同时成功时，后到的会覆盖 `c.quic`，导致 `currentGW` 与实际 QUIC 连接错位
3. **路由不跟随切换**：Level 2（Bootstrap Pool）和 Level 3（信令共振）重连成功后不触发 `switchFn`，Kill Switch 路由仍指向旧网关
4. **路由切换非原子**：连接切换和路由切换分散执行，中间失败可能留下半切换状态
5. **URI 兑换丢字段**：`redeemFromURI` 重新组装 BootstrapConfig 时可能丢失 PreSharedKey、CertFingerprint、UserID

## 术语表

- **GTunnelClient**：G-Tunnel 客户端主结构体，管理连接、FEC、重组器、路由表、当前网关
- **QUICEngine**：QUIC Datagram 连接引擎，管理物理 NIC 绑定和数据报收发
- **KillSwitch**：路由劫持管理器，实现 fail-closed 路由策略
- **RouteTable**：内存网关节点列表，支持排除当前 IP 后选择下一个可用节点
- **Resonance Resolver**：信令共振发现器，三通道（DoH/Gist/Mastodon）并发拉取加密信令
- **switchFn**：网关切换回调函数，由 main.go 注册，负责调用 `ks.UpdateGatewayRoute(newIP)`
- **ProbeAndConnect**：并发探测 bootstrap 节点，First-Win 模式连接第一个响应者
- **BootstrapConfig**：解密后的 token 载荷，包含 BootstrapPool、AuthKey、PSK、CertFingerprint、UserID、ExpiresAt

## 需求

### 需求 1：建立统一连接状态机

**用户故事：** 作为开发者，我需要 Client 在任意时刻都能明确知道当前处于什么连接状态，以便消除多 goroutine 同时修改连接对象的混乱。

#### 验收标准

1. THE GTunnelClient SHALL 定义正式连接状态枚举：`StateInit / StateBootstrapping / StateConnected / StateDegraded / StateReconnecting / StateExhausted / StateStopped`
2. THE GTunnelClient SHALL 维护单一 `state` 字段（atomic 或 mutex 保护），所有状态切换都通过统一的 `transition(newState)` 方法执行
3. WHEN `transition` 被调用时，THE GTunnelClient SHALL 记录状态变化日志（旧状态 → 新状态 + 时间戳 + 原因）
4. THE GTunnelClient SHALL 提供 `State() ConnState` 方法，返回当前连接状态
5. WHEN 状态为 `StateReconnecting` 时，THE GTunnelClient SHALL 拒绝新的 Reconnect 调用（单飞保护）
6. WHEN 状态为 `StateStopped` 时，THE GTunnelClient SHALL 拒绝所有 Send/Receive/Reconnect 调用

### 需求 2：重连收敛为单飞执行

**用户故事：** 作为开发者，我需要同一时刻只有一个重连流程在执行，以便网络抖动时不会触发多条并发重连链路。

#### 验收标准

1. THE GTunnelClient SHALL 使用 singleflight 模式保护 Reconnect 方法 — 同一时刻只允许一个 Reconnect goroutine 执行，其他调用者等待结果或直接返回
2. WHEN 多个 goroutine 同时调用 Reconnect 时，THE GTunnelClient SHALL 只执行一次实际重连逻辑，所有调用者共享同一个结果
3. THE Reconnect SHALL 在进入时将状态切换为 `StateReconnecting`，在成功时切换为 `StateConnected`，在失败时切换为 `StateExhausted`
4. IF Reconnect 正在执行中，THEN 新的 Reconnect 调用 SHALL 等待当前执行完成并返回相同结果（不启动新的重连）

### 需求 3：探测成功与正式接管连接拆开

**用户故事：** 作为开发者，我需要并发探测阶段只返回候选结果，由状态机选出唯一胜者后再正式接管，以便 `currentGW` 与实际 QUIC 连接始终一致。

#### 验收标准

1. THE `ProbeAndConnect` SHALL 改为两阶段执行：Phase 1 并发探测返回候选 `(gateway, engine)` 对；Phase 2 由调用者原子接管
2. WHEN 第一个探测成功时，THE ProbeAndConnect SHALL 立即取消其余探测 goroutine（通过 context cancel）
3. THE 原子接管 SHALL 在单个 mutex 临界区内完成：关闭旧 QUICEngine → 设置新 QUICEngine → 更新 currentGW → 更新 connected 状态
4. AFTER 原子接管完成，THE GTunnelClient SHALL 确保 `c.quic`、`c.currentGW`、`c.connected` 三者始终一致
5. IF 探测成功但接管时发现已有更新的连接（另一个 Reconnect 已完成），THEN THE 接管 SHALL 放弃并关闭候选 engine

### 需求 4：所有切换路径都触发 Kill Switch 路由更新

**用户故事：** 作为用户，我需要不论通过哪一级恢复成功，路由都会跟随切到当前 Gateway，以便不再出现"逻辑已切换、路由仍停在旧网关"的错位。

#### 验收标准

1. WHEN Level 1（RouteTable）重连成功且网关 IP 变化时，THE Reconnect SHALL 调用 `switchFn(newIP)`（当前已实现）
2. WHEN Level 2（Bootstrap Pool）重连成功且网关 IP 变化时，THE Reconnect SHALL 调用 `switchFn(newIP)`（当前未实现）
3. WHEN Level 3（信令共振）重连成功且网关 IP 变化时，THE Reconnect SHALL 调用 `switchFn(newIP)`（当前未实现）
4. THE `switchFn` 调用 SHALL 在原子接管完成后立即执行，不允许在接管和路由更新之间存在其他操作
5. IF `switchFn` 执行失败（路由更新失败），THEN THE Reconnect SHALL 回滚连接接管（恢复旧 engine 和 currentGW），并返回错误

### 需求 5：连接切换和路由切换做成原子事务

**用户故事：** 作为用户，我需要切换过程中即使失败，也不会把设备留在错误路由状态，以便连接与系统路由始终同向变化。

#### 验收标准

1. THE 网关切换 SHALL 按以下原子序列执行：① 探测候选成功 → ② 预加新 /32 路由 → ③ 接管新连接 → ④ 删除旧 /32 路由 → ⑤ 提交状态
2. IF 步骤 ② 失败（预加路由失败），THEN THE 切换 SHALL 中止并关闭候选 engine，保持当前连接不变
3. IF 步骤 ③ 失败（接管失败），THEN THE 切换 SHALL 删除步骤 ② 预加的路由，保持当前连接不变
4. THE KillSwitch SHALL 提供 `PreAddHostRoute(newIP)` 方法（添加新路由但不删除旧路由）和 `CommitSwitch(oldIP)` 方法（删除旧路由）
5. THE KillSwitch SHALL 提供 `RollbackPreAdd(newIP)` 方法（删除预加的路由，用于回滚）

### 需求 6：修复 URI 兑换链路对关键配置的丢失

**用户故事：** 作为用户，我需要通过 URI 兑换获得的配置与直接 token 模式在核心能力上保持一致，以便一次兑换即可长期使用。

#### 验收标准

1. THE `redeemFromURI` SHALL 从服务端响应中完整提取以下字段：BootstrapPool、AuthKey、PreSharedKey、CertFingerprint、UserID、ExpiresAt
2. IF 服务端响应中缺少 PreSharedKey 或 UserID，THEN THE `redeemFromURI` SHALL 返回明确错误而不是使用零值
3. THE `redeemFromURI` SHALL 不再伪造 ExpiresAt（当前硬编码 30 天），而是使用服务端返回的真实值
4. THE 生成的 BootstrapConfig SHALL 通过 `token.ParseToken` 往返后保持所有字段不变（往返一致性）
5. THE `redeemFromURI` SHALL 在兑换成功后验证 BootstrapPool 非空，否则返回错误
