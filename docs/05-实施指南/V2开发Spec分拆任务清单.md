# Mirage V2 开发 Spec 分拆任务清单

## 文档目标

本文档将所有新增文档中的待实现功能和审计整改项，按依赖关系和优先级拆分成可独立创建的 Spec 任务。每个 Spec 对应一个可交付的工程单元，你可以按顺序逐个让我创建 Spec。

---

## 阶段总览

| 阶段 | 目标 | Spec 数量 |
|------|------|-----------|
| 阶段1 | V1 安全封洞（立刻修） | 3 个 |
| 阶段2 | V1 架构补齐（两周内修） | 4 个 |
| 阶段3 | V1 产品化收尾 + 商业化 | 3 个 |
| 阶段4 | V2 编排内核 P0 | 3 个 |
| 阶段5 | V2 编排内核 P1 | 3 个 |
| 阶段6 | V2 编排内核 P2 + 控制面 | 2 个 |
| **合计** | | **18 个 Spec** |

---

## 阶段1：V1 安全封洞（立刻修）

对应文档：`OS-Gateway 安全整改清单.md` 立刻修部分

### Spec 1-1：OS 安全紧急封洞

**Spec 目录名**：`v1-os-security-immediate`

**来源**：OS-Gateway 安全整改清单 → 五、OS 立刻修

**范围**：
- OS-Immediate-1：关闭普通用户到管理面的权限升级路径
- OS-Immediate-2：为所有管理接口补上角色校验
- OS-Immediate-3：WebSocket 指挥面改成只读
- OS-Immediate-4：收口所有内部接口（Provisioner HTTP、gateway-bridge REST、gRPC 绑定 127.0.0.1）
- OS-Immediate-5：登录/挑战/认证接口补速率限制

**涉及模块**：mirage-os（API Server、Gateway Bridge、WebSocket）

**前置依赖**：无

---

### Spec 1-2：Gateway 安全紧急封洞

**Spec 目录名**：`v1-gateway-security-immediate`

**来源**：OS-Gateway 安全整改清单 → 六、Gateway 立刻修

**范围**：
- Gateway-Immediate-1：生产模式强制 mTLS，禁止明文回退
- Gateway-Immediate-2：证书钉扎真正生效
- Gateway-Immediate-3：入口感知改成入口可拒绝（强规则丢弃扫描流量）
- Gateway-Immediate-4：修复黑名单下发到数据面的断点（eBPF map 加载 + 命中生效）
- Gateway-Immediate-5：对抗模式切换与 kill/焦土指令收敛到唯一可信来源

**涉及模块**：mirage-gateway（pkg/api、pkg/ebpf、pkg/threat、bpf/）

**前置依赖**：无（可与 Spec 1-1 并行）

---

### Spec 1-3：Client 连接安全与状态机修复

**Spec 目录名**：`v1-client-fsm-route-fix`

**来源**：Client 鲁棒性整改清单 → 第一阶段（先把"会不会乱"修掉）

**范围**：
- FSM-1：建立统一连接状态机（Init/Bootstrapping/Connected/Degraded/Reconnecting/Exhausted/Stopped）
- FSM-2：重连收敛成单飞执行
- FSM-3：探测成功与正式接管连接拆开
- ROUTE-1：所有切换路径都触发 Kill Switch 路由更新
- ROUTE-2：连接切换和路由切换做成原子事务
- SUB-1：修复 URI 兑换链路对关键配置的丢失

**涉及模块**：phantom-client

**前置依赖**：无（可与 Spec 1-1、1-2 并行）

---

## 阶段2：V1 架构补齐（两周内修）

对应文档：`OS-Gateway 安全整改清单.md` 两周内修 + `多节点架构整改清单.md` P0 + `Phantom-蜜罐策略审计与收敛建议.md` 第一阶段

### Spec 2-1：OS-Gateway 安全闭环

**Spec 目录名**：`v1-security-closure`

**来源**：OS-Gateway 安全整改清单 → 七~九（两周内修）

**范围**：
- OS-2W-1：统一 RBAC 与资源归属模型
- OS-2W-2：内部服务统一鉴权标准（shared secret / mTLS）
- OS-2W-3：控制面审计日志
- OS-2W-4：异常来源到 Gateway 封禁的统一回路
- OS-2W-5：安全基线（输入校验、限流、安全 Header、默认密钥清理）
- Gateway-2W-1：标准入口处置策略（拒绝/静默丢弃/告警/隔离/引流蜜罐）
- Gateway-2W-2：蜜罐、指纹、黑名单三者联动
- Gateway-2W-3：Gateway 本地安全状态机（正常/警戒/高压/隔离/静默）
- Gateway-2W-4：关键安全动作观测指标
- Gateway-2W-5：安全回归测试

**涉及模块**：mirage-os + mirage-gateway

**前置依赖**：Spec 1-1、Spec 1-2

---

### Spec 2-2：多节点架构 P0 — 归属与计费隔离

**Spec 目录名**：`v1-multinode-ownership`

**来源**：多节点架构整改清单 → P0 项

**范围**：
- Proto-1：TrafficRequest 支持精确归属（user_id / session_id）
- DB-1：从 Gateway 绑定 User 升级为 Gateway 承载多 Session（新增 gateway_sessions / client_sessions 表）
- DB-2：计费日志可回溯到用户与会话
- Gateway-1：配额从 Gateway 全局桶升级为用户/会话隔离桶
- Gateway-2：流量上报携带精确归属信息
- OS-3：建立 Client/User/Gateway 归属映射服务

**涉及模块**：mirage-os + mirage-gateway + proto

**前置依赖**：Spec 1-1

---

### Spec 2-3：多节点架构 P0 — OS 控制面补齐

**Spec 目录名**：`v1-os-control-plane`

**来源**：多节点架构整改清单 → OS-1、OS-2 + Proto-2

**范围**：
- OS-1：建立真实的 Gateway 注册与拓扑索引
- OS-2：按节点下推与按 Cell 下推做成正式能力
- Proto-2：Heartbeat 与状态同步消息补齐拓扑语义
- OS-4：多网关控制面一致性验收

**涉及模块**：mirage-os + proto

**前置依赖**：Spec 2-2

---

### Spec 2-4：Phantom 蜜罐收敛（第一阶段）

**Spec 目录名**：`v1-phantom-convergence`

**来源**：Phantom-蜜罐策略审计与收敛建议 → 第一阶段 + 第二阶段

**范围**：
- 修复数据面统计桶错误（phantom.c STAT_PASSED 计数）
- 名单增加 TTL 和过期机制
- 单一 honeypot_ip 升级为分层目标池
- 去掉显眼的 _tracking 字段和明显回调路径
- 停止依赖不可信的 Header 顺序判断
- 减少模板种类，每个 Gateway/Cell 绑定稳定外部画像
- 统一官网、错误页、API、默认页的世界观
- 无限迷宫改成有限深度自然死路

**涉及模块**：mirage-gateway（bpf/phantom.c + pkg/phantom/）

**前置依赖**：Spec 1-2

---

## 阶段3：V1 产品化收尾 + 商业化

对应文档：`Client 鲁棒性整改清单.md` 第二/三阶段 + `阶梯服务策略建议.md` + `零信任抗审与防守侧归因架构.md`

### Spec 3-1：Client 产品化（拓扑学习 + 订阅托管 + 后台服务化）

**Spec 目录名**：`v1-client-productization`

**来源**：Client 鲁棒性整改清单 → 第二阶段 + 第三阶段

**范围**：
- 第二阶段：
  - TOPO-1：实现真正可用的 Route Table 拉取
  - TOPO-2：拓扑学习改成持续刷新
  - TOPO-3：绝境发现链路真正接入主流程
  - SUB-2：订阅校验从启动时一次性升级为运行时托管
  - SUB-3：服务等级策略做成 Client 运行时能力
  - SVC-1：前台 CLI 改成正式后台服务
  - SVC-2：首次开通与长期运行拆成两个阶段
- 第三阶段（精选高价值项）：
  - FSM-4：分级退化策略显式行为
  - TOPO-4：区分启动种子池和运行时拓扑池
  - TOPO-5：拓扑最小可信约束（签名/版本校验）
  - SUB-4：离线宽限与受控退化策略
  - SVC-3：后台健康守护与自恢复
  - ROUTE-3：去掉 8.8.8.8:53 硬编码物理网卡探测
  - ROUTE-4：启动/异常退出/切换失败时路由回滚保护

**涉及模块**：phantom-client

**前置依赖**：Spec 1-3、Spec 2-3（依赖 OS 控制面提供拓扑同步协议）

---

### Spec 3-2：阶梯服务策略落地

**Spec 目录名**：`v1-tiered-service`

**来源**：阶梯服务策略建议.md → 最小改动方案

**范围**：
- 停止用余额自动推导等级，cell_level 正式定义为付费等级
- QuotaPurchase.package_type 承载月费产品（plan_standard_monthly / plan_platinum_monthly / plan_diamond_monthly / traffic_*）
- 购买等级时直接更新 users.cell_level
- TierRouter 按等级分配资源池（Standard→标准池 / Platinum→高优先级池 / Diamond→高隔离池）
- 第一期服务差异：资源池差异、连接负载差异、恢复优先级差异
- 配额熔断只影响对应用户，不影响整机

**涉及模块**：mirage-os（API Server、Gateway Bridge、DB）

**前置依赖**：Spec 2-2（依赖用户/会话隔离模型）

---

### Spec 3-3：零信任抗审计三层纵深防御

**Spec 目录名**：`v1-zero-trust-defense`

**来源**：零信任抗审与防守侧归因架构.md

**范围**：
- L1 物理与链路层清洗（eBPF XDP/TC）：
  - 静态 ASN/网段清洗（云厂商数据中心 IP → XDP_DROP）
  - 速率限制（SYN Flood / 高频连接）
  - 静默响应（禁止 ICMP Unreachable）
- L2 密码学与协议防线（User-Space）：
  - 严格 mTLS 校验
  - 抗重放归因（Nonce + Timestamp 记录，重放检测 → 封锁）
  - 半开连接/慢速嗅探熔断（300ms 内未完成握手 → 静默 RST）
- L3 统计与行为基线监控：
  - 非预期协议突发归因（443 收到 SSH/HTTP 明文 → 标记协议扫描器）
  - 时序与熵值校验（偏离马尔可夫链模型 → 踢下线）
- 威胁情报富化：
  - 云厂商公开网段离线库
  - ASN/WHOIS 离线库
  - 数据本地化闭环（禁止数据面向外查询）

**涉及模块**：mirage-gateway（bpf/ + pkg/threat/ + pkg/cortex/）

**前置依赖**：Spec 2-1（依赖安全闭环基础）

---

## 阶段4：V2 编排内核 P0

对应文档：`Mirage V2 编排内核设计草案.md` Phase 1-3

### Spec 4-1：V2 三层状态模型

**Spec 目录名**：`v2-state-model`

**来源**：编排内核设计草案 → Phase 1（状态建模）

**范围**：
- Link State 定义与实现（link_id / transport_type / health_score / rtt / loss / jitter / phase / available / degraded）
- Session State 定义与实现（session_id / user_id / client_id / service_class / priority / current_persona_id / current_link_id / current_survival_mode / state / migration_pending）
- Control State 定义与实现（epoch / persona_version / route_generation / active_tx_id / rollback_marker / last_successful_epoch / control_health）
- 三层状态关系图与生命周期
- 状态持久化（DB schema）
- 状态查询 API

**涉及模块**：mirage-gateway（新建 pkg/orchestrator/）+ mirage-os（DB）

**前置依赖**：阶段3 全部完成

---

### Spec 4-2：V2 Persona Manifest 与原子切换

**Spec 目录名**：`v2-persona-engine`

**来源**：编排内核设计草案 → Phase 2（Persona Manifest）

**范围**：
- Persona Manifest 结构定义（persona_id / version / epoch / checksum / handshake_profile_id / packet_shape_profile_id / timing_profile_id / background_profile_id）
- 版本与 epoch 规则
- Shadow / Active 双区模型
- Persona 生命周期（Prepared → ShadowLoaded → Active → Cooling → Retired）
- 原子切换语义（先 shadow write → checksum 校验 → 原子 flip）
- 将 B-DNA / NPM / Jitter-Lite / VPC 参数收敛到 Persona 快照
- Persona 选择约束（Session 服务等级 × Link 健康 × Survival Mode）
- 回滚能力（保留上一个稳定版本）

**涉及模块**：mirage-gateway（pkg/orchestrator/ + pkg/ebpf/）

**前置依赖**：Spec 4-1

---

### Spec 4-3：V2 State Commit Engine

**Spec 目录名**：`v2-commit-engine`

**来源**：编排内核设计草案 → Phase 3（State Commit Engine）

**范围**：
- CommitTransaction 对象（tx_id / tx_type / target_session_id / prepare_state / validate_state / flip_state / ack_state / commit_state / rollback_marker）
- 标准提交流程：Prepare → Validate Constraint → Shadow Write → Flip → Acknowledge → Commit/Rollback
- 受管变更类型：persona switch / link migration / gateway reassignment / survival mode switch
- 事务冲突规则（同一时刻只允许一个 Session 级事务 + 一个 Link 级事务 + 一个全局 Survival 事务）
- 恢复规则（重启后检查未完成事务 / rollback marker / 回到上一个稳定 epoch）
- rollback marker 机制

**涉及模块**：mirage-gateway（pkg/orchestrator/）

**前置依赖**：Spec 4-1、Spec 4-2

---

## 阶段5：V2 编排内核 P1

对应文档：`Mirage V2 编排内核设计草案.md` Phase 4-5

### Spec 5-1：V2 Budget Engine

**Spec 目录名**：`v2-budget-engine`

**来源**：编排内核设计草案 → Phase 4（Budget Engine）

**范围**：
- BudgetProfile 定义（latency_budget_ms / bandwidth_budget_ratio / switch_budget_per_hour / entry_burn_budget_per_day / gateway_load_budget / hardened_allowed / escape_allowed）
- Internal Cost Model（带宽/延迟/切换/入口消耗/Gateway 负载成本）
- External SLA / Service Policy（Standard / Priority / Hardened Access / Dedicated Survival Window）
- 预算判定流程（编排器提出动作 → 估算成本 → 映射服务等级 → allow / allow_degraded / allow_with_charge / deny_and_hold / deny_and_suspend）
- 与 Commit Engine 集成（Validate Constraint 阶段调用 Budget Engine）

**涉及模块**：mirage-gateway（pkg/orchestrator/）+ mirage-os

**前置依赖**：Spec 4-3

---

### Spec 5-2：V2 Survival Orchestrator

**Spec 目录名**：`v2-survival-orchestrator`

**来源**：编排内核设计草案 → Phase 5（Survival Orchestrator）

**范围**：
- Survival Mode 状态机（Normal / LowNoise / Hardened / Degraded / Escape / LastResort）
- 每种模式绑定的编排策略（transport policy / persona policy / budget policy / switch aggressiveness / session admission policy）
- 状态迁移触发因素（Link Health / Entry Burn / Budget / Policy）
- 状态迁移约束（cooldown / hysteresis / minimum dwell time）
- 将 G-Switch 升级为 Survival Orchestrator 的一部分
- Transport Fabric 设计（路径选择 / 切换 / 多通道调度 / 退化路径）
- Transport Policy 按 Survival Mode 分级（Normal / Hardened / Degraded / Escape）

**涉及模块**：mirage-gateway（pkg/orchestrator/ + pkg/gswitch/）

**前置依赖**：Spec 5-1

---

### Spec 5-3：V2 控制语义层

**Spec 目录名**：`v2-control-semantics`

**来源**：编排内核设计草案 → 十二（控制语义层设计）

**范围**：
- ControlEvent 对象（event_id / event_type / source / target_scope / priority / epoch / payload_ref / requires_ack）
- 事件类型定义：
  - EventSessionMigrateRequest / EventSessionMigrateAck
  - EventPersonaPrepare / EventPersonaFlip
  - EventSurvivalModeChange
  - EventRollbackRequest / EventRollbackDone
  - EventBudgetReject
- 事件语义要求（作用域 / 优先级 / 是否要求 ack / 是否幂等 / 是否可重放 / 是否携带 epoch）

**涉及模块**：mirage-gateway（pkg/orchestrator/）

**前置依赖**：Spec 5-2

---

## 阶段6：V2 编排内核 P2 + 控制面承载

对应文档：`Mirage V2 编排内核设计草案.md` Phase 6 + `MirageV2隐蔽控制面与DPI对抗承载架构.md`

### Spec 6-1：V2 观测与审计

**Spec 目录名**：`v2-observability`

**来源**：编排内核设计草案 → Phase 6（观测与审计）

**范围**：
- 事务审计（提交发起时间 / 发起原因 / 目标状态 / 预算判定结果 / flip 是否成功 / rollback 是否触发）
- 状态时间线（Session timeline / Link health timeline / Persona version timeline / Survival mode timeline / Transaction timeline）
- 最小诊断视图（当前 Session 挂在哪个 Link / 使用哪个 Persona / 系统处于哪个 Survival Mode / 最近一次切换原因 / 最近一次回滚原因 / 某次事务卡在哪个阶段）

**涉及模块**：mirage-gateway + mirage-os

**前置依赖**：Spec 5-3

---

### Spec 6-2：V2 隐蔽控制面承载

**Spec 目录名**：`v2-stealth-control-plane`

**来源**：MirageV2隐蔽控制面与DPI对抗承载架构.md

**范围**：
- 方案 A：QUIC 隐蔽流多路复用（Shadow Stream Multiplexing）
  - Stream 0 作为特权控制神经
  - Stream 1~N 承载用户代理数据
  - Protobuf 控制指令写入 Stream 0
- 方案 B：废包隐写术（Steganographic Dummy Payloads）
  - 劫持 NPM 废包替换为 HMAC + 密文控制指令
  - 接收端 HMAC 校验提取
  - 长度严格对齐 NPM 原始废包长度
- 防御状态切换流量潮汐（Transition_Duration 平滑过渡，马尔可夫链概率矩阵插值）
- 恒定时间锁与响应混淆（Spin-loop 对齐处理耗时，消除 RTT 侧信道）

**涉及模块**：mirage-gateway（pkg/gtunnel/ + bpf/npm.c）+ phantom-client

**前置依赖**：Spec 5-3、Spec 6-1

---

## 使用方式

你可以按顺序告诉我：

> "创建 Spec 1-1"

我会为你生成对应的 `requirements.md`、`design.md`、`tasks.md` 三件套，放到 `.kiro/specs/<目录名>/` 下。

同一阶段内没有依赖关系的 Spec 可以并行创建和执行。

---

## 依赖关系图

```
阶段1（并行）
├── Spec 1-1: OS 安全紧急封洞
├── Spec 1-2: Gateway 安全紧急封洞
└── Spec 1-3: Client 连接安全与状态机修复
        │
阶段2
├── Spec 2-1: OS-Gateway 安全闭环 ← 1-1, 1-2
├── Spec 2-2: 多节点归属与计费隔离 ← 1-1
├── Spec 2-3: OS 控制面补齐 ← 2-2
└── Spec 2-4: Phantom 蜜罐收敛 ← 1-2
        │
阶段3
├── Spec 3-1: Client 产品化 ← 1-3, 2-3
├── Spec 3-2: 阶梯服务策略落地 ← 2-2
└── Spec 3-3: 零信任三层纵深防御 ← 2-1
        │
阶段4（V2 P0）
├── Spec 4-1: 三层状态模型 ← 阶段3全部
├── Spec 4-2: Persona Engine ← 4-1
└── Spec 4-3: Commit Engine ← 4-1, 4-2
        │
阶段5（V2 P1）
├── Spec 5-1: Budget Engine ← 4-3
├── Spec 5-2: Survival Orchestrator ← 5-1
└── Spec 5-3: 控制语义层 ← 5-2
        │
阶段6（V2 P2）
├── Spec 6-1: 观测与审计 ← 5-3
└── Spec 6-2: 隐蔽控制面承载 ← 5-3, 6-1
```
