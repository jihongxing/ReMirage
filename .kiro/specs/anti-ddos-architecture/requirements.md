# 需求文档：Mirage 抗 DDoS 架构整改

## 简介

本 Spec 对应 `docs/Mirage 抗DDoS架构整改清单.md` 的完整实现，覆盖文档中"立刻修"（第七章）、"两周内修"（第八章）、"版本级改造"（第九章）三个阶段的所有整改项。

核心定位：Mirage 的抗 DDoS 目标不是让每个公网节点成为永远打不死的堡垒，而是让受击节点可以被快速识别、快速放弃、快速替换，并让整套系统在 OS 编排与 Client 自愈能力支撑下持续存活。

参考文档：#[[file:docs/Mirage 抗DDoS架构整改清单.md]]

## 术语表

- **XDP**：eXpress Data Path，Linux 内核最早的包处理钩子点，在网卡驱动层执行
- **TC**：Traffic Control，Linux 流量控制层，在 XDP 之后执行
- **blacklist_lpm**：eBPF LPM Trie Map，用户级 IP 黑名单
- **asn_blocklist_lpm**：eBPF LPM Trie Map，ASN 级 IP 段黑名单
- **ingress_profile_map**：eBPF Hash Map，入口端口 → 协议画像映射
- **SecurityFSM**：Gateway 本地安全状态机（Normal/Alert/HighPressure/Isolated/Silent）
- **Registry**：OS 侧拓扑索引管理器，维护 Gateway 状态
- **RuntimeTopology**：Client 运行时拓扑池，可动态更新
- **Resonance**：信令共振发现器，Client 绝境复活机制
- **软防**：资源耗尽型攻击下的响应模式，节点继续存活但停止接纳新用户
- **硬断**：体积型攻击下的响应模式，节点按死亡处理，快速替补

## 当前代码状态

### 已具备的基础（上一轮已实现）
- XDP 层 `l1_defense.c` 已集成 blacklist_lpm 查询、ASN 过滤、非法画像检查、入口协议准入、速率限制
- OS 侧 Registry 已支持 UNDER_ATTACK / DRAINING / DEAD 状态
- OS 侧 `UpdateHeartbeatWithMetrics` 已接通真实指标做 DDoS 裁决
- `checkTimeouts` 已区分 1x 超时（OFFLINE）和 3x 超时（DEAD）
- Gateway 心跳上报已携带安全状态和威胁等级
- Client 三级降级重连已完整（RuntimeTopo → BootstrapPool → Resonance）

### 本轮需要补齐的能力
1. Gateway 多协议感知入口守卫（从单纯黑名单升级为多协议感知）
2. 多维准入控制（从粗粒度 IP 限流升级为行为治理）
3. OS 软防响应链路闭环（UNDER_ATTACK → 停止分配 → 攻击结束恢复）
4. OS 硬断响应链路闭环（DEAD → 备用节点接替 → 拓扑发布）
5. Client 运行时拓扑学习持续化
6. Client 失联后原子恢复状态机
7. V2 Survival Orchestrator 集成
8. V2 State Commit Engine 节点替补事务化
9. V2 抗 DDoS 预算模型
10. V2 独立恢复发布平面
11. V2 Gateway 预热池与容量分层

---

## 需求（立刻修 — 第七章）

### 需求 1：多协议感知入口守卫完善（Gateway-Immediate-1/3 补齐）

**用户故事：** 作为 Gateway 运维，我需要 XDP 入口守卫能识别不同协议入口的流量画像，以便非法流量在最早阶段被拒绝。

#### 验收标准

1. WHEN Go 控制面启动时，THE DefenseApplier SHALL 自动将默认入口画像（QUIC/443、WSS/8443、TURN/3478）同步到 `ingress_profile_map` eBPF Map
2. WHEN 收到目标端口为 443 的 UDP 包且载荷 < 1200 字节时，THE XDP guard SHALL 丢弃该包（QUIC Initial 最小 1200 字节）
3. WHEN 收到目标端口为 8443 的 UDP 包时，THE XDP guard SHALL 丢弃该包（WSS 入口仅允许 TCP）
4. THE Go 控制面 SHALL 支持通过 OS 下发指令动态更新入口画像配置
5. THE `l1_stats` SHALL 记录各类丢弃统计（blacklist_drops / sanity_drops / profile_drops）

### 需求 2：无状态准入能力（Gateway-Immediate-4）

**用户故事：** 作为安全工程师，我需要 Gateway 在握手前就能拒绝伪造源的洪泛流量，以便握手洪泛不会拖垮连接状态表。

#### 验收标准

1. THE XDP guard SHALL 对 TCP SYN 包实施 SYN Cookie 风格的无状态验证 — 在 SYN+ACK 中编码验证信息，只有正确回复 ACK 的源才允许进入完整握手
2. WHEN 单 IP 在 1 秒内发送超过阈值的 SYN 包且未完成任何有效握手时，THE XDP guard SHALL 将该 IP 临时加入速率限制黑名单
3. THE HandshakeGuard SHALL 在用户态层面保持 300ms 握手超时作为第二道防线
4. THE 无状态准入 SHALL 通过 eBPF Map 接收配置（启用/禁用、阈值），Go 控制面可动态调整

---

## 需求（两周内修 — 第八章）

### 需求 3：多维准入控制（Gateway-TwoWeeks-2）

**用户故事：** 作为安全工程师，我需要准入控制不再只绑定物理源 IP，以便减少 CGNAT/企业出口场景下的误伤。

#### 验收标准

1. THE Gateway SHALL 维护多维准入评分器，综合以下维度评估入站连接：单位时间新建请求量、有效验证通过率、会话令牌有效性、入口画像匹配率
2. WHEN 某 IP 的多维评分低于阈值时，THE Gateway SHALL 对该 IP 执行限速而非直接封禁
3. THE 多维评分器 SHALL 支持 CGNAT 感知 — 同一 IP 下多个有效会话不应触发封禁
4. THE 评分器状态 SHALL 通过 Prometheus metrics 暴露，便于监控

### 需求 4：OS 软防响应链路（OS-TwoWeeks-1）

**用户故事：** 作为 OS 控制面，我需要在 Gateway 受资源耗尽型攻击时自动停止向该节点分配新用户，攻击结束后自动恢复。

#### 验收标准

1. WHEN Gateway 心跳上报 ThreatLevel >= 3 或 CPU > 90% 时，THE OS SHALL 将该节点标记为 UNDER_ATTACK
2. WHEN 节点处于 UNDER_ATTACK 状态时，THE TierRouter SHALL 跳过该节点不分配新用户
3. WHEN UNDER_ATTACK 节点连续 3 次心跳 ThreatLevel <= 1 且 CPU < 70% 时，THE OS SHALL 自动恢复该节点为 ONLINE
4. THE OS SHALL 通过 Downlink 通知 UNDER_ATTACK 节点停止接受新连接
5. THE 软防链路 SHALL 记录完整的状态变迁日志（时间、原因、持续时长）

### 需求 5：OS 硬断响应链路（OS-TwoWeeks-2）

**用户故事：** 作为 OS 控制面，我需要在 Gateway 被体积型攻击打死后快速裁决、快速替补，不再等待节点自己恢复。

#### 验收标准

1. WHEN Gateway 心跳超时 > 3x timeout 时，THE OS SHALL 将该节点标记为 DEAD
2. WHEN 节点被标记为 DEAD 时，THE OS SHALL 自动从可分配池移除，并触发备用节点接替流程
3. THE CellScheduler SHALL 在检测到 DEAD 节点后，从同 Cell 的影子池中选择替补节点并激活
4. THE OS SHALL 在替补节点激活后发布新的路由表，通知 Client 更新拓扑
5. WHEN DEAD 节点重新发送心跳时，THE OS SHALL 允许其重新注册但不立即分配用户（需经过校准期）

### 需求 6：Client 运行时拓扑持续学习（Client-TwoWeeks-1）

**用户故事：** 作为 Client，我需要持续学习最新的 Gateway 池和优先级，以便在节点替换时能快速切换。

#### 验收标准

1. THE TopoRefresher SHALL 在连接正常时每 5 分钟拉取一次最新路由表
2. WHEN 拉取失败时，THE TopoRefresher SHALL 使用指数退避重试（最大间隔 30 分钟）
3. WHEN 连续 3 次拉取失败时，THE Client SHALL 记录告警日志
4. THE RuntimeTopology SHALL 支持增量更新 — 只有版本号更高的路由表才会被接受
5. THE Client SHALL 在每次成功拉取后持久化路由表到本地存储，作为下次启动的缓存

### 需求 7：Client 失联后原子恢复状态机（Client-TwoWeeks-2）

**用户故事：** 作为 Client，我需要在公网节点硬断时不会停在无意义的长时间重试中，而是按明确的恢复顺序快速找到新节点。

#### 验收标准

1. THE Client SHALL 区分三种失联场景：主链路抖动（< 5s）、节点受压（5s-30s）、节点死亡（> 30s）
2. WHEN 主链路抖动时，THE Client SHALL 在当前连接上重试（最多 3 次，间隔 1s）
3. WHEN 节点受压时，THE Client SHALL 触发拓扑刷新并尝试切换到同 Cell 的其他节点
4. WHEN 节点死亡时，THE Client SHALL 按 L1→L2→L3 降级顺序执行恢复
5. THE 恢复状态机 SHALL 有明确的超时控制 — 每级最多 15 秒，总恢复时间不超过 60 秒
6. THE Client SHALL 在恢复成功后记录恢复耗时和使用的恢复级别

---

## 需求（版本级改造 — 第九章）

### 需求 8：Survival Orchestrator 集成（V2-Architecture-1）

**用户故事：** 作为系统架构师，我需要抗 DDoS 成为 Survival Mode 编排内核的一部分，而不是外围脚本。

#### 验收标准

1. THE SurvivalOrchestrator SHALL 将 UNDER_ATTACK / DEAD / RECOVERING 纳入统一状态机
2. THE 状态机 SHALL 与 Link State、Session State、Control State 统一编排
3. WHEN 多个 Gateway 同时受攻击时，THE Orchestrator SHALL 按优先级（Diamond > Platinum > Standard）决定恢复顺序
4. THE Orchestrator SHALL 提供 API 查询当前系统生存状态

### 需求 9：State Commit Engine 节点替补事务化（V2-Architecture-2）

**用户故事：** 作为系统架构师，我需要节点替换过程可解释、可追踪、可恢复。

#### 验收标准

1. THE StateCommitEngine SHALL 对节点摘除、热备接替、拓扑发布、路由更新引入事务化控制
2. IF 替换过程中任一步骤失败，THEN THE Engine SHALL 回滚已执行的步骤
3. THE Engine SHALL 为每次替换生成唯一事务 ID，记录完整的操作日志
4. THE Engine SHALL 支持手动重试失败的替换事务

### 需求 10：抗 DDoS 预算模型（V2-Architecture-3）

**用户故事：** 作为运营，我需要抗 DDoS 动作受预算约束，不同服务等级绑定不同生存预算。

#### 验收标准

1. THE BudgetModel SHALL 为以下行为定义成本：高强度入口守卫、高频切换、热备节点常驻、高等级恢复路径
2. THE BudgetModel SHALL 根据用户等级（Diamond/Platinum/Standard）分配不同的生存预算
3. WHEN 预算耗尽时，THE System SHALL 降级到基础防护而非完全停止服务
4. THE BudgetModel SHALL 通过 API 暴露当前预算使用情况

### 需求 11：独立恢复发布平面（V2-Architecture-4）

**用户故事：** 作为系统架构师，我需要受击节点即使完全假死，Client 仍能获得新的有效入口。

#### 验收标准

1. THE OS SHALL 建立独立于单 Gateway 业务面的恢复发布平面
2. THE 恢复平面 SHALL 支持通过 DNS TXT / Gist / Mastodon 等多通道发布替补拓扑
3. THE Client SHALL 将恢复平面作为 L3 绝境恢复的数据源
4. THE 恢复平面 SHALL 在节点 DEAD 后 30 秒内发布新的替补拓扑

### 需求 12：Gateway 预热池与容量分层（V2-Architecture-5）

**用户故事：** 作为运维，我需要攻击发生后可以切到预热节点，而不是等待云资源慢启动。

#### 验收标准

1. THE CellScheduler SHALL 维护三级节点池：活跃池（Phase=2）、温备池（Phase=1）、冷备池（Phase=0）
2. WHEN 活跃节点被标记为 DEAD 时，THE Scheduler SHALL 优先从温备池拉起替补（< 10 秒）
3. WHEN 温备池耗尽时，THE Scheduler SHALL 从冷备池拉起（< 60 秒）
4. THE Scheduler SHALL 在温备池节点数低于阈值时自动补充
5. THE 预热池状态 SHALL 通过 Prometheus metrics 暴露
