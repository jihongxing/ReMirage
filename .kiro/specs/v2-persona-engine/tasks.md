# 实施计划：V2 Persona Manifest 与原子切换

## 概述

将设计文档中的 Persona Engine 转化为可执行的编码任务。实现顺序：错误类型 → Manifest 结构与校验 → 生命周期状态机 → PersonaMapUpdater（eBPF 双 Slot） → PersonaEngine 核心（切换/回滚） → PersonaSelector 三重约束 → DB Schema → HTTP API → 集成联调。代码分布在 `mirage-gateway/pkg/orchestrator/persona/`（引擎逻辑）、`mirage-gateway/pkg/ebpf/`（Map 桥接）、`mirage-os/pkg/models/`（DB 模型）及 `mirage-os` API 层。

## Tasks

- [x] 1. 定义错误类型与 Manifest 基础结构
  - [x] 1.1 创建 `mirage-gateway/pkg/orchestrator/persona/errors.go`，定义所有错误类型
    - 实现 `ErrMissingProfile{FieldName}`、`ErrChecksumMismatch{Expected, Actual}`、`ErrVersionConflict{PersonaID, ExistingMax, Attempted}`、`ErrImmutableField{FieldName}`
    - 实现 `ErrInvalidLifecycleTransition{From, To}`、`ErrShadowVerifyFailed{MapName, Field}`、`ErrMapWriteFailed{MapName}`、`ErrFlipFailed`、`ErrSwitchInProgress`、`ErrNoCoolingTarget`、`ErrNoMatchingPersona{Constraints}`
    - 每个错误类型实现 `error` 接口，`Error()` 返回包含上下文信息的消息
    - _Requirements: 1.4, 3.3, 4.4, 5.3, 5.5, 6.6, 7.5, 8.5_

  - [x] 1.2 创建 `mirage-gateway/pkg/orchestrator/persona/manifest.go`，定义 Manifest 结构体与枚举
    - 定义 `PersonaLifecycle` 枚举（Prepared / ShadowLoaded / Active / Cooling / Retired）
    - 定义 `PersonaManifest` 结构体，包含设计文档中所有字段（persona_id、version、epoch、checksum、六个 profile_id、lifecycle_policy_id、lifecycle、created_at）
    - 添加 GORM 标签：联合唯一索引 `idx_persona_version`、CHECK 约束、默认值
    - 定义合法生命周期转换表 `validTransitions map[[2]PersonaLifecycle]bool`
    - 实现 `ValidateManifest`：校验四个必填 profile_id 非空
    - 实现 `ComputeChecksum`：基于六个 profile_id 拼接计算 SHA-256
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 3.1, 3.2_

  - [x]* 1.3 编写属性测试：Manifest 完整性校验
    - **Property 1: Manifest 完整性校验**
    - 使用 `pgregory.net/rapid` 随机生成四个必填 profile_id（含空字符串情况），验证 ValidateManifest 结果与字段是否全部非空一致
    - **Validates: Requirements 1.2, 1.4**

  - [x]* 1.4 编写属性测试：Checksum 确定性与唯一性
    - **Property 2: Checksum 确定性与唯一性**
    - 随机生成两组六个 profile_id，验证相同输入产生相同 checksum，不同输入产生不同 checksum
    - **Validates: Requirements 1.3**


- [x] 2. 实现生命周期状态机与版本管理
  - [x] 2.1 在 `manifest.go` 中实现生命周期转换逻辑
    - 实现 `TransitionLifecycle`：校验 (from, to) 是否在合法转换表中，非法转换返回 `ErrInvalidLifecycleTransition`
    - 仅允许五条路径：Prepared→ShadowLoaded、ShadowLoaded→Active、ShadowLoaded→Retired、Active→Cooling、Cooling→Retired
    - _Requirements: 3.2, 3.3, 3.5_

  - [x] 2.2 实现版本管理逻辑
    - 在 `CreateManifest` 中确保 version 严格大于同一 persona_id 下已存在的最大 version
    - 将 epoch 设置为当前 ControlState 的 Epoch 值
    - 创建后禁止修改 version、epoch、checksum 字段
    - 版本冲突时返回 `ErrVersionConflict`
    - _Requirements: 2.1, 2.2, 2.3, 2.4_

  - [x]* 2.3 编写属性测试：生命周期转换合法性
    - **Property 5: 生命周期转换合法性**
    - 随机生成 PersonaLifecycle 对 (from, to)，验证 TransitionLifecycle 结果与合法转换表完全一致
    - **Validates: Requirements 3.2, 3.3, 3.5**

  - [x]* 2.4 编写属性测试：版本严格递增与 Epoch 对齐
    - **Property 3: 版本严格递增与 Epoch 对齐**
    - 对同一 persona_id 连续 N 次 CreateManifest，验证 version 严格单调递增，epoch 等于创建时 ControlState.Epoch
    - **Validates: Requirements 2.1, 2.2, 2.4**

  - [x]* 2.5 编写属性测试：创建后不可变字段
    - **Property 4: 创建后不可变字段**
    - 创建 Manifest 后尝试修改 version、epoch、checksum，验证操作被拒绝
    - **Validates: Requirements 2.3**

- [x] 3. Checkpoint - 确保 Manifest 基础结构和状态机正确
  - 确保所有测试通过，ask the user if questions arise.

- [x] 4. 实现 PersonaMapUpdater（eBPF 双 Slot 桥接）
  - [x] 4.1 创建 `mirage-gateway/pkg/ebpf/persona_updater.go`，实现 `PersonaMapUpdater` 接口
    - 定义 `PersonaParams` 结构体（DNA、Jitter、VPC、NPM 四组参数）
    - 实现 `GetActiveSlot`：从 active_slot_map 读取当前活跃 Slot 编号
    - 实现 `WriteShadow`：计算 shadow = 1 - active，依次写入 dna_template_map[shadow]、jitter_config_map[shadow]、vpc_config_map[shadow]、npm_config_map[shadow]，任一写入失败立即停止并返回 `ErrMapWriteFailed`
    - 实现 `VerifyShadow`：从 Shadow Slot 回读全部参数，逐字段比对，不一致返回 `ErrShadowVerifyFailed`
    - 实现 `Flip`：单次 active_slot_map.Put(0, newActiveSlot) 完成原子切换
    - 使用现有 `ebpf.Loader.GetMap()` 获取 Map 引用，与 `DNATemplateEntry`、`JitterConfig`、`VPCConfig` 结构体兼容
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 6.1, 6.2, 6.3, 6.4, 6.5, 6.6_

  - [x] 4.2 创建 `PersonaMapUpdater` 的 Mock 实现用于测试
    - 使用内存 map 模拟双 Slot 存储和 active_slot_map
    - 支持注入写入/读取错误用于故障测试
    - _Requirements: 4.2, 4.3, 4.4_

  - [x]* 4.3 编写属性测试：Shadow Slot 写入 round-trip
    - **Property 7: Shadow Slot 写入 round-trip**
    - 随机生成 PersonaParams，WriteShadow 后 VerifyShadow 回读，验证每个字段值完全一致
    - **Validates: Requirements 4.2, 4.3, 6.1, 6.2, 6.3, 6.4, 6.5**

- [x] 5. 实现 PersonaEngine 核心（切换与回滚）
  - [x] 5.1 创建 `mirage-gateway/pkg/orchestrator/persona/engine.go`，实现 `PersonaEngine` 接口
    - 实现 `CreateManifest`：校验 → 计算 checksum → 版本检查 → epoch 对齐 → GORM 持久化
    - 实现 `SwitchPersona`：互斥锁保护 → ValidateManifest → WriteShadow → VerifyShadow → Flip → 更新 SessionState.current_persona_id → 更新 ControlState.persona_version → 旧 Active→Cooling → 新 Manifest→Active
    - 任一步骤失败时保持 active_slot_map 和 Session/Control 状态不变
    - 使用 `sync.Mutex` 确保同一时刻只有一个切换操作执行，并发调用返回 `ErrSwitchInProgress`
    - _Requirements: 5.1, 5.2, 5.3, 5.5_

  - [x] 5.2 实现 `Rollback` 方法
    - 查找当前 Session 下 lifecycle=Cooling 的 Manifest，不存在返回 `ErrNoCoolingTarget`
    - 执行 Flip 切换回 Cooling Persona 所在 Slot
    - 更新 SessionState.current_persona_id 为回滚目标
    - Cooling→Active，原 Active→Retired
    - _Requirements: 5.4, 5.6, 8.1, 8.2, 8.3, 8.4, 8.5_

  - [x] 5.3 实现查询方法
    - `GetLatest`：按 persona_id 查询最新版本
    - `ListVersions`：按 persona_id 查询全部版本，version 降序
    - `GetActiveBySession`：按 session_id 查询当前 Active Persona
    - _Requirements: 10.1, 10.2, 10.3_

  - [x]* 5.4 编写属性测试：原子切换后状态一致性
    - **Property 8: 原子切换后状态一致性**
    - 执行 SwitchPersona 成功后，验证 active_slot_map 指向新 Slot、Session.current_persona_id 等于新 persona_id、旧 Active→Cooling
    - **Validates: Requirements 5.1, 5.2**

  - [x]* 5.5 编写属性测试：切换失败保持不变
    - **Property 9: 切换失败保持不变**
    - 注入各步骤失败（Shadow 写入失败、校验失败、Flip 失败），验证 active_slot_map、Session、Control 状态与切换前完全相同
    - **Validates: Requirements 4.4, 5.3**

  - [x]* 5.6 编写属性测试：回滚恢复到 Cooling 版本
    - **Property 10: 回滚恢复到 Cooling 版本**
    - 执行切换产生 Cooling Persona 后执行 Rollback，验证 active_slot_map 指向 Cooling Slot、Cooling→Active、原 Active→Retired
    - **Validates: Requirements 5.4, 5.6, 8.3, 8.4**

  - [x]* 5.7 编写属性测试：Session 维度 Active/Cooling 唯一性
    - **Property 6: Session 维度 Active/Cooling 唯一性**
    - 对同一 Session 执行多次切换，验证任意时刻最多一个 Active 和最多一个 Cooling
    - **Validates: Requirements 3.4, 8.1, 8.2**

  - [x]* 5.8 编写属性测试：切换互斥
    - **Property 11: 切换互斥**
    - 对同一 Session 发起 N 个并发 SwitchPersona，验证恰好一个成功，其余返回 ErrSwitchInProgress
    - **Validates: Requirements 5.5**

- [x] 6. Checkpoint - 确保切换与回滚核心逻辑正确
  - 确保所有测试通过，ask the user if questions arise.

- [x] 7. 实现 PersonaSelector 三重约束选择器
  - [x] 7.1 创建 `mirage-gateway/pkg/orchestrator/persona/selector.go`，实现 `PersonaSelector` 接口
    - 定义 `SelectionConstraints` 结构体（ServiceClass、LinkHealth、SurvivalMode）
    - 实现 `Select`：按 ServiceClass 过滤兼容 Manifest → 按 SurvivalMode 排序防御强度 → 按 LinkHealth 过滤资源消耗
    - ServiceClass=Standard 时仅返回 Standard 兼容 Manifest
    - SurvivalMode 为 Hardened/Escape/LastResort 时优先高防御强度
    - LinkHealth < 50 时优先低资源消耗
    - 无匹配时返回 `ErrNoMatchingPersona`
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_

  - [x]* 7.2 编写属性测试：三重约束选择一致性
    - **Property 12: 三重约束选择一致性**
    - 随机生成 SelectionConstraints 组合，验证返回的 Manifest 满足 ServiceClass 兼容、SurvivalMode 防御强度、LinkHealth 资源消耗约束
    - **Validates: Requirements 7.2, 7.3, 7.4, 7.5**

- [x] 8. DB Schema 集成（mirage-os 侧）
  - [x] 8.1 在 `mirage-os/pkg/models/` 中添加 PersonaManifest 的 GORM 模型
    - 创建 `mirage-os/pkg/models/persona_manifest.go`，定义 `PersonaManifest` 结构体（与 orchestrator 侧字段一致）
    - 添加完整 GORM 标签：联合主键 (persona_id, version)、CHECK 约束（lifecycle 枚举）、epoch 索引、默认值
    - 定义 `TableName()` 返回 `persona_manifests`
    - _Requirements: 9.1, 9.2, 9.3_

  - [x] 8.2 将 PersonaManifest 纳入 `AutoMigrate`
    - 修改 `mirage-os/pkg/models/db.go` 的 `AutoMigrate` 函数，添加 `&PersonaManifest{}`
    - _Requirements: 9.5_

- [x] 9. 实现 Persona Query API（mirage-os 侧）
  - [x] 9.1 实现 `GET /api/v2/personas/{persona_id}` 端点
    - 返回指定 persona_id 的最新版本 Persona Manifest
    - 不存在返回 HTTP 404，包含资源类型与标识
    - JSON 响应，时间戳 RFC 3339
    - _Requirements: 10.1, 10.4, 10.5_

  - [x] 9.2 实现 `GET /api/v2/personas/{persona_id}/versions` 端点
    - 返回指定 persona_id 的全部版本列表，按 version 降序
    - 不存在返回 HTTP 404
    - _Requirements: 10.2, 10.4, 10.5_

  - [x] 9.3 实现 `GET /api/v2/sessions/{session_id}/persona` 端点
    - 返回指定 Session 当前 Active 状态的 Persona Manifest
    - 不存在返回 HTTP 404
    - _Requirements: 10.3, 10.4, 10.5_

  - [x]* 9.4 编写属性测试：Manifest JSON round-trip
    - **Property 13: Manifest JSON round-trip**
    - 随机生成合法 PersonaManifest，JSON 序列化后反序列化，验证等价且 created_at 符合 RFC 3339
    - **Validates: Requirements 10.5**

  - [x]* 9.5 编写 API 端点集成测试
    - 测试三个端点的正常响应和 404 场景
    - 测试版本列表降序排列
    - 测试 JSON 响应格式和时间戳格式
    - _Requirements: 10.1-10.5_

- [x] 10. Final Checkpoint - 全部测试通过，端到端验证
  - 确保所有测试通过，ask the user if questions arise.

## Notes

- 标记 `*` 的子任务为可选，可跳过以加速 MVP
- 每个任务引用具体需求编号，确保可追溯
- 属性测试使用 `pgregory.net/rapid`，每个属性对应设计文档中的 Property 编号
- PersonaEngine 依赖 Spec 4-1 的 SessionState、ControlState、LinkState（`pkg/orchestrator/`）
- PersonaMapUpdater 依赖现有 `pkg/ebpf/Loader` 和 Map 通信接口
- Go 控制面通过 eBPF Map 与 C 数据面通信，禁止直接函数调用
- 并发测试（Property 11）建议使用 Mock MapUpdater 避免真实 eBPF 依赖
