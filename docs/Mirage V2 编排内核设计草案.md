# Mirage V2 编排内核设计草案

## 一、文档目标

本文档用于将 [MirageV2升级核心纪要-编排内核.md](D:/codeSpace/ReMirage/docs/MirageV2升级核心纪要-编排内核.md) 进一步细化为正式设计稿。

这份设计稿回答五个问题：

1. Mirage V2 到底要升级成什么系统
2. 这个系统由哪些核心对象和状态组成
3. 关键状态如何切换、提交、确认和回滚
4. 资源预算与服务等级如何进入编排逻辑
5. 这套设计如何分阶段落地

本文档只处理**编排内核**，不直接展开代码实现，不讨论具体 UI，不展开具体外部承载技巧，不扩写为新的市场定位文档。

---

## 二、设计边界

### 2.1 本文档覆盖范围

本文档覆盖：

- `Transport Fabric` 的编排接口与状态建模
- `Persona Engine` 的统一画像模型
- `Survival Orchestrator` 的状态机
- `Link / Session / Control` 三层状态模型
- `State Commit Engine` 的提交流程
- `Budget Engine` 的内部成本与外部 SLA 映射
- 审计、恢复、回滚和观测要求

### 2.2 本文档不覆盖范围

本文档不覆盖：

- 新协议发明
- 外部控制承载细节
- 前端控制台细节
- 数据库具体表结构实现
- 加密原语替换
- 具体内核技巧的代码写法

---

## 三、V2 总体设计原则

Mirage V2 的设计必须同时服从以下 6 条原则。

### 3.1 会话优先于参数

系统中的主语不再是“某几个协议参数”，而是“一个被运营、被计费、被约束的会话”。

### 3.2 画像优先于局部伪装

`B-DNA / NPM / Jitter-Lite / VPC` 不再分别作为独立参数对外暴露，而是统一收敛到 `Persona`。

### 3.3 生存姿态优先于单点切换

`G-Switch` 不再只承担域名切换，而是升级为 `Survival Orchestrator` 的一部分。

### 3.4 提交优先于即时生效

所有关键变更必须经过：

- 约束校验
- Shadow 写入
- 原子切换
- 确认
- 回滚

### 3.5 预算优先于无上限增强

所有强化动作都必须消耗预算，不能因为风险升高就无限抬高策略强度。

### 3.6 小团队可运营优先于理论完美

这套设计必须适配 Mirage 当前路线：

- 小团队
- 约 200 个高价值客户
- 低暴露运营
- 可回退
- 可诊断

---

## 四、V2 总体架构

Mirage V2 从“六协议协同系统”升级为“三层编排系统 + 两个横切引擎”。

### 4.1 三层编排系统

#### Layer A：Transport Fabric

负责：

- 底层承载路径
- 路径健康
- 迁移与降级
- 多传输调度

当前主要承载模块：

- `G-Tunnel`
- 多路径调度器
- 传输降级逻辑

#### Layer B：Persona Engine

负责：

- 握手画像
- 包长形态
- 时域特征
- 背景噪音

当前主要承载模块：

- `B-DNA`
- `NPM`
- `Jitter-Lite`
- `VPC`

#### Layer C：Survival Orchestrator

负责：

- 生存姿态切换
- 整体风险下的系统模式控制
- 入口迁移与资源升级

当前主要承载模块：

- `G-Switch`
- 策略引擎
- 多入口控制逻辑

### 4.2 两个横切引擎

#### Engine 1：State Commit Engine

负责所有关键状态变更的：

- prepare
- validate
- shadow write
- flip
- acknowledge
- commit / rollback

#### Engine 2：Budget Engine

负责所有强化动作的：

- 内部资源成本计算
- 服务等级约束判断
- 是否允许执行某次增强/切换

---

## 五、核心对象模型

V2 的编排内核围绕 7 个核心对象运行。

### 5.1 Session

`Session` 是 V2 中最重要的业务对象。

建议字段：

```text
Session
- session_id
- user_id
- client_id
- gateway_id
- service_class
- session_priority
- current_persona_id
- current_persona_version
- current_link_id
- current_survival_mode
- state
- created_at
- updated_at
```

它表达的是：

“某个用户的某个客户端，在某个时刻，正在通过哪种链路、以哪种画像、处于哪种生存姿态运行。”

### 5.2 Link

`Link` 表示承载路径。

建议字段：

```text
Link
- link_id
- gateway_id
- transport_type
- remote_entry
- local_binding
- health_score
- rtt_ms
- loss_rate
- jitter_ms
- available
- degraded
- last_probe_at
```

它描述的不是用户，而是底层可供编排器选择的物理/逻辑承载单元。

### 5.3 Persona Manifest

`Persona Manifest` 是 V2 的统一画像快照。

建议字段：

```text
PersonaManifest
- persona_id
- version
- epoch
- checksum
- handshake_profile_id
- packet_shape_profile_id
- timing_profile_id
- background_profile_id
- mtu_profile_id
- fec_profile_id
- lifecycle_policy_id
- created_at
```

原则：

- 一个 Persona 必须是完整快照
- 不能拆成四套独立临时参数
- 必须能原子切换
- 必须保留旧版本以支持回滚

### 5.4 Survival Mode

`Survival Mode` 是系统姿态，而不是某个单协议参数。

建议定义：

```text
SurvivalMode
- Normal
- LowNoise
- Hardened
- Degraded
- Escape
- LastResort
```

每种模式绑定一组编排策略，包括：

- transport policy
- persona policy
- budget policy
- switch aggressiveness
- session admission policy

### 5.5 Budget Profile

`Budget Profile` 用于表达系统和用户的约束边界。

建议字段：

```text
BudgetProfile
- latency_budget_ms
- bandwidth_budget_ratio
- switch_budget_per_hour
- entry_burn_budget_per_day
- gateway_load_budget
- hardened_allowed
- escape_allowed
- last_resort_allowed
```

### 5.6 Commit Transaction

`Commit Transaction` 用于记录一次正式状态变更。

建议字段：

```text
CommitTransaction
- tx_id
- tx_type
- target_session_id
- target_link_id
- target_persona_id
- target_survival_mode
- prepare_state
- validate_state
- flip_state
- ack_state
- commit_state
- rollback_marker
- created_at
- finished_at
```

### 5.7 Control Event

`Control Event` 是编排系统的语义事件，不是外部协议本身。

建议字段：

```text
ControlEvent
- event_id
- event_type
- source
- target_scope
- priority
- epoch
- payload_ref
- requires_ack
- created_at
```

---

## 六、三层状态模型

Mirage V2 的状态必须严格拆成三层。

### 6.1 Link State

#### 定义

`Link State` 描述链路本身的状态，不包含用户业务语义。

#### 建议字段

```text
LinkState
- link_id
- transport_type
- gateway_id
- health_score
- rtt_ms
- loss_rate
- jitter_ms
- phase
- available
- degraded
- last_switch_reason
```

#### 建议状态

- `Probing`
- `Active`
- `Degrading`
- `Standby`
- `Unavailable`

#### 关键原则

- 链路失效不等于会话失效
- 链路切换只能影响 Session 的承载映射，不能直接抹掉 Session

### 6.2 Session State

#### 定义

`Session State` 描述业务会话本身。

#### 建议字段

```text
SessionState
- session_id
- user_id
- service_class
- priority
- current_persona_id
- current_link_id
- current_survival_mode
- billing_mode
- state
- migration_pending
```

#### 建议状态

- `Bootstrapping`
- `Active`
- `Protected`
- `Migrating`
- `Degraded`
- `Suspended`
- `Closed`

#### 关键原则

- Session 是编排器的主要服务对象
- Session 必须能承受底层 Link 漂移
- Session 的优先级和服务等级必须参与编排

### 6.3 Control State

#### 定义

`Control State` 描述“系统正在做什么切换或提交”。

#### 建议字段

```text
ControlState
- epoch
- persona_version
- route_generation
- active_tx_id
- rollback_marker
- last_successful_epoch
- last_switch_reason
- control_health
```

#### 关键原则

- Control State 必须独立存在
- 这样系统才能解释“现在为什么是这个状态”
- 也才能在崩溃后恢复到上一个稳定点

---

## 七、Persona Engine 设计

### 7.1 Persona Engine 的职责

`Persona Engine` 的职责不是“调四个协议”，而是“为一个 Session 选定并维持一个统一画像”。

### 7.2 Persona 的最小结构

一个 Persona 至少包含四类配置：

- 握手画像
- 包长画像
- 时域画像
- 背景画像

禁止继续保留如下行为：

- 单独修改 TLS 指纹
- 单独修改 padding
- 单独提高 jitter
- 单独切背景噪音

这些动作必须被统一包装进 `Persona Manifest`。

### 7.3 Persona 生命周期

建议 Persona 生命周期分为：

- `Prepared`
- `ShadowLoaded`
- `Active`
- `Cooling`
- `Retired`

### 7.4 Persona 选择原则

Persona 选择应同时受三类因素约束：

1. 当前 Session 的服务等级
2. 当前 Link 的健康与类型
3. 当前 Survival Mode

### 7.5 Persona 切换原则

Persona 切换必须满足：

- 先校验预算
- 先写 shadow
- 再原子 flip
- 保留上一个稳定版本
- 一次切换只允许一个有效目标版本

---

## 八、Transport Fabric 设计

### 8.1 Transport Fabric 的职责

Transport Fabric 负责：

- 承载路径选择
- 路径切换
- 多通道调度
- 退化路径启用

它不负责定义对外画像。

### 8.2 Transport Policy

建议为不同 Survival Mode 绑定不同的传输策略。

例如：

#### Normal

- 优先高性能路径
- 限制无意义切换

#### Hardened

- 允许更高容错成本
- 提高切换敏感度

#### Degraded

- 降低激进切换
- 保守维持当前有效路径

#### Escape

- 允许使用高成本入口
- 优先存活而不是性能

### 8.3 Link 迁移原则

一次 Link 迁移必须满足：

- Session 层语义尽量不变
- Persona 尽量稳定，除非必须联动切换
- 迁移结果必须被 Control State 记录

---

## 九、Survival Orchestrator 设计

### 9.1 职责

`Survival Orchestrator` 负责把局部事件升级成全局生存姿态。

例如：

- 入口失效
- 高战死频率
- 链路健康连续下降
- 预算耗尽

### 9.2 状态机定义

建议状态机如下：

#### `Normal`

- 正常运行
- 优先效率

#### `LowNoise`

- 降低额外噪音和激进动作
- 保守维持日常画像

#### `Hardened`

- 提升资源消耗上限
- 允许更高成本生存动作

#### `Degraded`

- 系统处于弱稳态
- 保守维护现有会话

#### `Escape`

- 进入快速迁移姿态
- 允许入口和链路重组

#### `LastResort`

- 只保留高优先级会话
- 降低系统整体暴露面

### 9.3 状态迁移触发因素

迁移因素分四类：

- `Link Health Trigger`
- `Entry Burn Trigger`
- `Budget Trigger`
- `Policy Trigger`

### 9.4 状态迁移约束

状态机必须避免：

- 高频抖动
- 反复横跳
- 一次事件触发多次升级

建议引入：

- cooldown
- hysteresis
- minimum dwell time

---

## 十、Budget Engine 设计

### 10.1 目标

`Budget Engine` 的目标不是单纯记账，而是把 Mirage 从“无限强化”约束到“有限决策”。

### 10.2 双层模型

#### Internal Cost Model

记录内部成本：

- 带宽增量成本
- 延迟增加成本
- 入口消耗成本
- 切换次数成本
- Gateway 负载成本

#### External SLA / Service Policy

记录对外权限：

- 用户是否允许 Hardened
- 用户是否允许 Escape
- 用户是否允许高频切换
- 用户是否允许占用高成本资源池

### 10.3 预算判定流程

建议流程：

1. 编排器提出动作
2. Budget Engine 估算内部成本
3. 映射服务等级策略
4. 决定：
   - 允许
   - 允许但降级执行
   - 拒绝

### 10.4 预算动作结果

预算结果不只返回 `allow / deny`，建议返回：

- `allow`
- `allow_degraded`
- `allow_with_charge`
- `deny_and_hold`
- `deny_and_suspend`

---

## 十一、State Commit Engine 设计

### 11.1 职责

State Commit Engine 负责所有关键状态变化的事务化管理。

### 11.2 受管变更类型

建议至少纳入以下变更：

- persona switch
- link migration
- gateway reassignment
- survival mode switch
- billing / SLA mode change

### 11.3 标准提交流程

建议所有受管变更统一走以下流程：

#### Stage 1：Prepare

收集上下文：

- 当前 Session 状态
- 当前 Link 状态
- 当前 Persona 版本
- 当前 Survival Mode
- 当前预算余量

#### Stage 2：Validate Constraint

校验：

- 是否满足预算
- 是否满足服务等级
- 是否满足冷却时间
- 是否存在更高优先级事务冲突

#### Stage 3：Shadow Write

将目标状态写入 shadow 区：

- 新 persona manifest
- 新 route target
- 新 survival mode
- rollback marker

#### Stage 4：Flip

执行单点切换，使新状态正式生效。

#### Stage 5：Acknowledge

等待关键参与方确认：

- 本地控制面确认
- 数据面版本确认
- 关键链路状态确认

#### Stage 6：Commit / Rollback

如果确认成功：

- 更新稳定版本指针
- 清理 rollback marker

如果确认失败：

- 回切旧版本
- 恢复上一个稳定 epoch
- 标记本次事务失败原因

### 11.4 事务冲突规则

建议同一时刻只允许：

- 一个 Session 级事务
- 一个 Link 级事务
- 一个全局 Survival 事务

并通过优先级规则处理抢占关系。

### 11.5 恢复规则

系统重启后必须优先检查：

- 是否存在未完成事务
- 是否存在 rollback marker
- 当前 epoch 是否已提交
- 当前 shadow state 是否应丢弃

原则是：

**重启后的系统必须回到上一个稳定点，而不是停在中间态。**

---

## 十二、控制语义层设计

### 12.1 目标

V2 需要控制语义层，但控制语义层不等于新协议。

本文档只定义“语义对象”，不定义外部承载技巧。

### 12.2 建议事件类型

建议至少定义：

- `EventSessionMigrateRequest`
- `EventSessionMigrateAck`
- `EventPersonaPrepare`
- `EventPersonaFlip`
- `EventSurvivalModeChange`
- `EventRollbackRequest`
- `EventRollbackDone`
- `EventBudgetReject`

### 12.3 事件语义要求

每个控制事件都应明确：

- 作用域
- 优先级
- 是否要求 ack
- 是否幂等
- 是否可重放
- 是否携带 epoch

---

## 十三、观测与审计设计

### 13.1 必须可回答的问题

V2 上线后，系统必须能回答：

- 当前某个 Session 挂在哪个 Link 上
- 当前 Session 使用哪个 Persona
- 当前系统处于哪个 Survival Mode
- 最近一次切换为什么发生
- 最近一次回滚为什么发生
- 某次事务卡在了哪个阶段

### 13.2 最小观测对象

建议观测最少包含：

- Session timeline
- Link health timeline
- Persona version timeline
- Survival mode timeline
- Transaction timeline

### 13.3 审计要求

所有关键事务都应写审计记录：

- 提交发起时间
- 发起原因
- 目标状态
- 预算判定结果
- flip 是否成功
- rollback 是否触发

---

## 十四、实施步骤

### Phase 0：语义冻结

目标：

- 冻结术语
- 冻结状态模型
- 冻结事务阶段名称

交付：

- 统一术语表
- 核心对象表
- 状态定义表

### Phase 1：状态建模

目标：

- 落地 `Link / Session / Control` 三层状态

交付：

- 状态对象结构
- 状态关系图
- 生命周期定义

### Phase 2：Persona Manifest

目标：

- 把 `B-DNA / NPM / Jitter-Lite / VPC` 收敛到 Persona 快照

交付：

- Persona Manifest 结构
- 版本与 epoch 规则
- Shadow / Active 双区模型

### Phase 3：State Commit Engine

目标：

- 让关键变更进入事务流程

交付：

- Prepare / Validate / Shadow / Flip / Ack / Rollback 状态机
- rollback marker 机制
- 重启恢复规则

### Phase 4：Budget Engine

目标：

- 让强化动作受预算约束

交付：

- Internal Cost Model
- External SLA / Policy Model
- allow / degrade / deny 判定规则

### Phase 5：Survival Orchestrator

目标：

- 把 `G-Switch` 升级成姿态控制器

交付：

- Survival Mode 状态机
- 迁移触发条件
- 模式切换冷却与滞回规则

### Phase 6：观测与审计

目标：

- 让系统可解释、可复盘

交付：

- 事务审计
- 状态时间线
- 最小诊断视图

---

## 十五、V2 的验收标准

Mirage V2 的验收不应再只看：

- 某条链路能否切换
- 某个协议参数能否生效

而应至少满足以下 6 条：

1. Persona 可以作为完整快照原子切换
2. Link 切换不会直接导致 Session 消失
3. Survival Mode 切换有明确状态机和预算约束
4. 所有关键变更都通过 Commit Engine
5. 系统重启后可以恢复到上一个稳定 epoch
6. 每次切换和回滚都能被解释

---

## 十六、总结

Mirage V2 的本质升级，不是再增加单点能力，而是给现有能力加上：

- 统一状态模型
- 统一画像快照
- 统一预算约束
- 统一提交流程
- 统一恢复与审计逻辑

因此，这份设计稿的核心结论可以压缩成一句话：

**Mirage V2 的目标不是做更复杂的协议栈，而是做一套围绕 Session 运行、围绕 Persona 呈现、围绕 Survival Mode 调度、围绕 Commit Engine 保证一致性的编排内核。**
