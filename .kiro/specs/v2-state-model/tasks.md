# 实施计划：V2 三层状态模型

## 概述

将设计文档中的三层状态模型（Link State、Session State、Control State）转化为可执行的编码任务。实现顺序：错误类型 → 状态模型 → 状态管理器 → 关系约束 → 并发控制 → DB Schema → HTTP API → 集成联调。代码分布在 `mirage-gateway/pkg/orchestrator/`（状态逻辑）和 `mirage-os/pkg/models/`（DB 模型）及 `mirage-os` API 层。

## Tasks

- [x] 1. 定义错误类型和状态模型基础
  - [x] 1.1 创建 `mirage-gateway/pkg/orchestrator/errors.go`，定义所有错误类型
    - 实现 `ErrInvalidTransition{From, To}`、`ErrLinkNotFound{LinkID}`、`ErrLinkUnavailable{LinkID}`、`ErrSessionNotFound{SessionID}`、`ErrOptimisticLockConflict`
    - 每个错误类型实现 `error` 接口，`Error()` 方法返回包含上下文信息的消息
    - _Requirements: 1.4, 2.4, 4.5, 7.4_

  - [x] 1.2 创建 `mirage-gateway/pkg/orchestrator/state_models.go`，定义枚举和结构体
    - 定义 `LinkPhase`（5 值）、`SessionPhase`（7 值）、`ControlHealth`（3 值）、`ServiceClass`（3 值）、`SurvivalMode`（6 值）枚举常量
    - 定义 `LinkState`、`SessionState`、`ControlState` 结构体，包含设计文档中所有字段
    - 添加 GORM 标签：主键、索引、CHECK 约束、默认值
    - 定义 `TableName()` 方法：`link_states`、`session_states`、`control_states`
    - 定义 `linkTransitions` 和 `sessionTransitions` 合法转换表（`map[[2]string]bool`）
    - _Requirements: 1.1, 1.2, 2.1, 2.2, 3.1, 3.2, 5.1, 5.2, 5.3, 5.6, 5.7, 5.8_

  - [x]* 1.3 为状态模型编写单元测试
    - 测试每个枚举值的字符串表示
    - 测试合法转换表的完整性（覆盖所有合法路径和非法路径样本）
    - 测试结构体默认值初始化
    - _Requirements: 1.2, 2.2, 3.2_

- [x] 2. 实现 LinkStateManager
  - [x] 2.1 创建 `mirage-gateway/pkg/orchestrator/link_state.go`，实现 `LinkStateManager` 接口
    - 实现 `Create`：创建新 LinkState，初始 phase 为 Probing
    - 实现 `Get`：按 link_id 查询，不存在返回 `ErrLinkNotFound`
    - 实现 `ListByGateway`：按 gateway_id 查询列表
    - 实现 `TransitionPhase`：校验转换合法性，执行副作用（Unavailable→available=false,health_score=0；Active→available=true,degraded=false；Degrading→degraded=true），记录 last_switch_reason
    - 实现 `UpdateHealth`：更新 health_score、rtt_ms、loss_rate、jitter_ms、last_probe_at
    - 实现 `Delete`：删除指定 link
    - 所有写操作在 GORM 事务中完成，同步更新 updated_at
    - _Requirements: 1.1, 1.3, 1.4, 1.5, 1.6, 1.7, 5.5_

  - [x]* 2.2 编写属性测试：Link 状态机转换合法性
    - **Property 1: Link 状态机转换合法性**
    - 使用 `pgregory.net/rapid` 随机生成 LinkPhase 对 (from, to)，验证 TransitionPhase 结果与合法转换表一致
    - **Validates: Requirements 1.3, 1.4**

  - [x]* 2.3 编写属性测试：Link 状态转换副作用一致性
    - **Property 2: Link 状态转换副作用一致性**
    - 随机生成合法转换，验证转换后 available、degraded、health_score 字段符合预期
    - **Validates: Requirements 1.5, 1.6, 1.7**

- [x] 3. 实现 SessionStateManager
  - [x] 3.1 创建 `mirage-gateway/pkg/orchestrator/session_state.go`，实现 `SessionStateManager` 接口
    - 实现 `Create`：创建新 SessionState，初始 state 为 Bootstrapping
    - 实现 `Get`：按 session_id 查询，不存在返回 `ErrSessionNotFound`
    - 实现 `ListByGateway`、`ListByUser`、`ListByFilter`：按条件过滤查询
    - 实现 `TransitionState`：校验转换合法性，执行副作用（Migrating→migration_pending=true；离开 Migrating→migration_pending=false）
    - 实现 `UpdateLink`：仅更新 current_link_id，保持 session_id、current_persona_id、service_class、priority 不变
    - 实现 `Delete`：删除指定 session
    - 所有写操作在 GORM 事务中完成
    - _Requirements: 2.1, 2.3, 2.4, 2.5, 2.6, 2.7, 5.5_

  - [x]* 3.2 编写属性测试：Session 状态机转换合法性
    - **Property 3: Session 状态机转换合法性**
    - 随机生成 SessionPhase 对 (from, to)，验证 TransitionState 结果与合法转换表一致
    - **Validates: Requirements 2.3, 2.4**

  - [x]* 3.3 编写属性测试：Session 迁移标记一致性
    - **Property 4: Session 迁移标记一致性**
    - 随机生成合法转换，验证 migration_pending 字段在进入/离开 Migrating 时正确设置
    - **Validates: Requirements 2.5, 2.6**

  - [x]* 3.4 编写属性测试：Session 链路变更不变量
    - **Property 5: Session 链路变更不变量**
    - 随机生成 SessionState 和新 link_id，执行 UpdateLink 后验证 session_id、current_persona_id、service_class、priority 不变
    - **Validates: Requirements 2.7**

- [x] 4. Checkpoint - 确保状态管理器基础功能正确
  - 确保所有测试通过，ask the user if questions arise.

- [x] 5. 实现 ControlStateManager
  - [x] 5.1 创建 `mirage-gateway/pkg/orchestrator/control_state.go`，实现 `ControlStateManager` 接口
    - 实现 `GetOrCreate`：按 gateway_id 查询，不存在则创建默认实例（epoch=0, control_health=Healthy）
    - 实现 `IncrementEpoch`：确保新 epoch 严格大于当前值，返回新 epoch
    - 实现 `BeginTransaction`：设置 active_tx_id，保持 control_health 不变
    - 实现 `CommitTransaction`：更新 last_successful_epoch=epoch，rollback_marker=epoch，清空 active_tx_id，记录 last_switch_reason
    - 实现 `RecoverOnStartup`：检测 active_tx_id 非空时，设 control_health=Recovering，epoch 回退到 rollback_marker，清空 active_tx_id
    - 所有写操作在 GORM 事务中完成
    - _Requirements: 3.1, 3.3, 3.4, 3.5, 3.6, 3.7, 4.3_

  - [x]* 5.2 编写属性测试：Epoch 严格递增
    - **Property 6: Epoch 严格递增**
    - 连续 N 次 IncrementEpoch，验证产生的 epoch 序列严格单调递增
    - **Validates: Requirements 3.3**

  - [x]* 5.3 编写属性测试：事务提交后状态一致性
    - **Property 7: 事务提交后状态一致性**
    - 执行 BeginTransaction + CommitTransaction，验证 last_successful_epoch、rollback_marker、active_tx_id 的值
    - **Validates: Requirements 3.5**

  - [x]* 5.4 编写属性测试：崩溃恢复正确性
    - **Property 8: 崩溃恢复正确性**
    - 模拟未完成事务状态，执行 RecoverOnStartup，验证 control_health=Recovering、epoch 回退、active_tx_id 清空
    - **Validates: Requirements 3.6, 3.7**

- [x] 6. 实现关系约束执行器
  - [x] 6.1 创建 `mirage-gateway/pkg/orchestrator/constraints.go`，实现 `ConstraintChecker` 接口
    - 实现 `ValidateLinkRef`：校验 link_id 对应的 LinkState 存在且 available=true，否则返回 `ErrLinkNotFound` 或 `ErrLinkUnavailable`
    - 实现 `OnLinkUnavailable`：查询所有 current_link_id 引用该 link 的 SessionState，将可转换到 Degraded 的 session 转为 Degraded
    - 实现 `ValidateControlStateSingleton`：确保每个 gateway_id 只有一个 ControlState
    - 在 SessionStateManager.Create 中集成 ValidateLinkRef 校验
    - 在 LinkStateManager.TransitionPhase 转为 Unavailable 时调用 OnLinkUnavailable
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5_

  - [x]* 6.2 编写属性测试：Session 创建引用完整性
    - **Property 9: Session 创建引用完整性**
    - 随机生成 LinkState（available=true/false）和 Session 创建请求，验证仅当 link 存在且可用时创建成功
    - **Validates: Requirements 4.1, 4.4, 4.5**

  - [x]* 6.3 编写属性测试：Link 不可用级联降级
    - **Property 10: Link 不可用级联降级**
    - 创建关联 session，将 link 转为 Unavailable，验证可降级的 session 转为 Degraded 而非 Closed
    - **Validates: Requirements 4.2**

- [x] 7. 实现并发控制
  - [x] 7.1 创建 `mirage-gateway/pkg/orchestrator/concurrency.go`，实现并发安全机制
    - 实现 `LockManager`：使用 `sync.Map` 管理每个 link_id / session_id / gateway_id 的 `sync.Mutex` 实例
    - 提供 `Lock(key string)` 和 `Unlock(key string)` 方法，支持 `context.Context` 超时控制
    - 在 LinkStateManager、SessionStateManager、ControlStateManager 的写操作中集成细粒度锁
    - 实现数据库乐观锁：GORM `Where("updated_at = ?", oldUpdatedAt)` 条件更新，影响行数为 0 时返回 `ErrOptimisticLockConflict`
    - 确保数据库事务失败时内存状态不变（先 DB 后内存）
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_

  - [x]* 7.2 编写属性测试：并发状态变更序列化
    - **Property 11: 并发状态变更序列化**
    - 对同一 link_id 发起 N 个并发 TransitionPhase，验证最终状态等价于某个串行执行结果，无数据竞争
    - **Validates: Requirements 7.1, 7.2, 7.3**

  - [x]* 7.3 编写属性测试：乐观锁冲突检测
    - **Property 12: 乐观锁冲突检测**
    - 两个并发更新基于相同 updated_at，验证恰好一个成功一个返回冲突错误
    - **Validates: Requirements 7.4**

- [x] 8. Checkpoint - 确保 orchestrator 包完整且测试通过
  - 确保所有测试通过，ask the user if questions arise.

- [x] 9. DB Schema 集成（mirage-os 侧）
  - [x] 9.1 在 `mirage-os/pkg/models/` 中添加三层状态的 GORM 模型定义
    - 创建 `mirage-os/pkg/models/state_models.go`，定义 `LinkState`、`SessionState`、`ControlState` 结构体（与 orchestrator 侧字段一致）
    - 添加完整的 GORM 标签：CHECK 约束（phase 枚举、state 枚举、health_score 0-100、priority 0-100、loss_rate 0-1）、索引、默认值
    - _Requirements: 5.1, 5.2, 5.3, 5.6, 5.7, 5.8_

  - [x] 9.2 将三张新表纳入 `AutoMigrate`
    - 修改 `mirage-os/pkg/models/db.go` 的 `AutoMigrate` 函数，添加 `&LinkState{}`、`&SessionState{}`、`&ControlState{}`
    - _Requirements: 5.4_

- [x] 10. 实现 State Query API（mirage-os 侧）
  - [x] 10.1 实现 Link 查询端点
    - `GET /api/v2/links?gateway_id=` 返回指定 gateway 下所有 LinkState 列表
    - `GET /api/v2/links/{link_id}` 返回单个 LinkState 详情，不存在返回 404
    - JSON 响应，时间戳 RFC 3339 格式
    - _Requirements: 6.1, 6.2, 6.6, 6.7_

  - [x] 10.2 实现 Session 查询端点
    - `GET /api/v2/sessions?gateway_id=&user_id=&state=` 支持多条件过滤
    - `GET /api/v2/sessions/{session_id}` 返回单个 SessionState 详情，不存在返回 404
    - `GET /api/v2/sessions/{session_id}/topology` 返回关联的 LinkState 和 ControlState 聚合视图
    - _Requirements: 6.3, 6.4, 6.6, 6.7, 6.8_

  - [x] 10.3 实现 Control 查询端点
    - `GET /api/v2/control/{gateway_id}` 返回 ControlState 详情，不存在返回 404
    - _Requirements: 6.5, 6.6, 6.7_

  - [x]* 10.4 编写 API 端点集成测试
    - 测试各端点的正常响应和 404 场景
    - 测试过滤参数组合
    - 测试拓扑聚合视图数据正确性
    - _Requirements: 6.1-6.8_

- [x] 11. Final Checkpoint - 全部测试通过，端到端验证
  - 确保所有测试通过，ask the user if questions arise.

## Notes

- 标记 `*` 的子任务为可选，可跳过以加速 MVP
- 每个任务引用具体需求编号，确保可追溯
- 属性测试使用 `pgregory.net/rapid`，每个属性对应设计文档中的 Property 编号
- orchestrator 包位于 mirage-gateway，DB 模型和 API 位于 mirage-os
- 并发测试（Property 11, 12）需要数据库环境，建议使用 SQLite 内存模式或 testcontainers
