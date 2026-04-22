# 需求文档：V2 控制语义层

## 简介

本文档定义 Mirage V2 编排内核的控制语义层（Control Semantics Layer）。控制语义层为编排内核内部的关键操作定义统一的语义事件对象（ControlEvent），使会话迁移、Persona 切换、Survival Mode 变更、回滚、预算拒绝等操作具备明确的作用域、优先级、幂等性、可重放性和 Epoch 关联语义。

控制语义层只定义"语义对象"，不定义外部承载协议。所有事件在 Go 控制面内生产和消费。

## 术语表

- **Control_Semantics_Layer**：控制语义层，负责定义和管理编排内核内部的语义事件对象
- **ControlEvent**：控制事件基础对象，携带 event_id、event_type、source、target_scope、priority、epoch、payload_ref、requires_ack 等字段
- **EventType**：事件类型枚举，标识控制事件的具体语义
- **EventScope**：事件作用域枚举，取值为 Session / Link / Global
- **Event_Dispatcher**：事件分发器，负责将 ControlEvent 路由到对应的处理器
- **Event_Handler**：事件处理器，负责消费特定类型的 ControlEvent
- **Event_Registry**：事件注册表，维护 EventType 到 Event_Handler 的映射
- **Epoch**：全局递增逻辑时钟，由 ControlStateManager（Spec 4-1）管理
- **Ack**：确认回执，表示事件已被目标处理器成功处理
- **Payload_Ref**：载荷引用，指向事件关联的详细数据（如 CommitTransaction tx_id、SessionState session_id）
- **Idempotent**：幂等，同一事件重复处理产生的效果与处理一次相同
- **Replayable**：可重放，事件可以在系统恢复后重新投递并正确处理
- **CommitEngine**：事务化状态提交引擎（Spec 4-3）
- **SurvivalOrchestrator**：生存编排器（Spec 5-2）
- **PersonaEngine**：统一画像引擎（Spec 4-2）
- **BudgetEngine**：预算引擎（Spec 5-1）

## 需求

### 需求 1：ControlEvent 基础对象定义

**用户故事：** 作为编排内核开发者，我需要一个统一的控制事件基础对象，以便所有编排操作使用一致的语义结构进行通信。

#### 验收标准

1. THE Control_Semantics_Layer SHALL 定义 ControlEvent 结构体，包含以下字段：event_id（string，UUID v4）、event_type（EventType 枚举）、source（string，事件来源标识）、target_scope（EventScope 枚举）、priority（int，0-10 范围，数值越大优先级越高）、epoch（uint64，关联的逻辑时钟值）、payload_ref（string，载荷引用标识）、requires_ack（bool，是否要求确认回执）、created_at（时间戳）
2. WHEN 创建 ControlEvent 时，THE Control_Semantics_Layer SHALL 自动生成唯一的 event_id（UUID v4 格式）并设置 created_at 为当前时间
3. THE Control_Semantics_Layer SHALL 将 priority 字段约束在 0 到 10 的整数范围内，0 表示最低优先级，10 表示最高优先级
4. IF priority 字段值超出 0-10 范围，THEN THE Control_Semantics_Layer SHALL 返回包含字段名和非法值的校验错误
5. IF event_type 字段值不属于已定义的 EventType 枚举，THEN THE Control_Semantics_Layer SHALL 返回包含非法值的校验错误
6. IF target_scope 字段值不属于 Session / Link / Global 三种枚举值，THEN THE Control_Semantics_Layer SHALL 返回包含非法值的校验错误

---

### 需求 2：EventType 枚举定义

**用户故事：** 作为编排内核开发者，我需要明确定义所有控制事件类型，以便每种编排操作都有对应的语义标识。

#### 验收标准

1. THE Control_Semantics_Layer SHALL 定义以下 8 种 EventType 枚举值：EventSessionMigrateRequest、EventSessionMigrateAck、EventPersonaPrepare、EventPersonaFlip、EventSurvivalModeChange、EventRollbackRequest、EventRollbackDone、EventBudgetReject
2. THE Control_Semantics_Layer SHALL 为每种 EventType 提供唯一的字符串表示，格式为 "session.migrate.request"、"session.migrate.ack"、"persona.prepare"、"persona.flip"、"survival.mode.change"、"rollback.request"、"rollback.done"、"budget.reject"
3. WHEN 使用未定义的 EventType 字符串创建 ControlEvent 时，THE Control_Semantics_Layer SHALL 返回包含该非法字符串的错误

---

### 需求 3：事件语义属性矩阵

**用户故事：** 作为编排内核开发者，我需要每种事件类型都有明确的语义属性定义，以便正确处理事件的作用域、优先级、确认、幂等和重放行为。

#### 验收标准

1. THE Control_Semantics_Layer SHALL 为每种 EventType 定义一组 EventSemantics 属性，包含：default_scope（默认作用域）、default_priority（默认优先级）、requires_ack（是否要求确认）、idempotent（是否幂等）、replayable（是否可重放）、carries_epoch（是否携带 epoch）
2. THE Control_Semantics_Layer SHALL 按以下规则定义 EventSessionMigrateRequest 的语义属性：default_scope 为 Session，default_priority 为 5，requires_ack 为 true，idempotent 为 false，replayable 为 false，carries_epoch 为 true
3. THE Control_Semantics_Layer SHALL 按以下规则定义 EventSessionMigrateAck 的语义属性：default_scope 为 Session，default_priority 为 5，requires_ack 为 false，idempotent 为 true，replayable 为 true，carries_epoch 为 true
4. THE Control_Semantics_Layer SHALL 按以下规则定义 EventPersonaPrepare 的语义属性：default_scope 为 Session，default_priority 为 4，requires_ack 为 true，idempotent 为 true，replayable 为 true，carries_epoch 为 true
5. THE Control_Semantics_Layer SHALL 按以下规则定义 EventPersonaFlip 的语义属性：default_scope 为 Session，default_priority 为 7，requires_ack 为 true，idempotent 为 false，replayable 为 false，carries_epoch 为 true
6. THE Control_Semantics_Layer SHALL 按以下规则定义 EventSurvivalModeChange 的语义属性：default_scope 为 Global，default_priority 为 9，requires_ack 为 true，idempotent 为 false，replayable 为 false，carries_epoch 为 true
7. THE Control_Semantics_Layer SHALL 按以下规则定义 EventRollbackRequest 的语义属性：default_scope 为 Session，default_priority 为 8，requires_ack 为 true，idempotent 为 true，replayable 为 true，carries_epoch 为 true
8. THE Control_Semantics_Layer SHALL 按以下规则定义 EventRollbackDone 的语义属性：default_scope 为 Session，default_priority 为 8，requires_ack 为 false，idempotent 为 true，replayable 为 true，carries_epoch 为 true
9. THE Control_Semantics_Layer SHALL 按以下规则定义 EventBudgetReject 的语义属性：default_scope 为 Session，default_priority 为 6，requires_ack 为 false，idempotent 为 true，replayable 为 true，carries_epoch 为 false
10. WHEN 查询任意已定义 EventType 的 EventSemantics 时，THE Control_Semantics_Layer SHALL 返回非 nil 的 EventSemantics 对象，且所有字段值与上述定义一致

---

### 需求 4：事件创建与默认值填充

**用户故事：** 作为编排内核开发者，我需要通过工厂函数创建特定类型的控制事件，以便自动填充该类型的默认语义属性，减少手动配置错误。

#### 验收标准

1. THE Control_Semantics_Layer SHALL 为每种 EventType 提供工厂函数，接受 source、payload_ref 和 epoch 参数，返回预填充默认语义属性的 ControlEvent
2. WHEN 通过工厂函数创建 ControlEvent 时，THE Control_Semantics_Layer SHALL 将 target_scope 设置为该 EventType 的 default_scope，将 priority 设置为该 EventType 的 default_priority，将 requires_ack 设置为该 EventType 的 requires_ack 默认值
3. WHEN 通过工厂函数创建 ControlEvent 时，THE Control_Semantics_Layer SHALL 生成唯一的 event_id 并设置 created_at 为当前时间
4. WHEN 通过工厂函数创建携带 epoch 的事件类型（carries_epoch 为 true）时，THE Control_Semantics_Layer SHALL 将传入的 epoch 参数写入 ControlEvent 的 epoch 字段
5. WHEN 通过工厂函数创建不携带 epoch 的事件类型（carries_epoch 为 false）时，THE Control_Semantics_Layer SHALL 将 ControlEvent 的 epoch 字段设置为 0

---

### 需求 5：事件校验

**用户故事：** 作为编排内核开发者，我需要对控制事件进行完整性校验，以便在分发前拦截非法事件。

#### 验收标准

1. THE Control_Semantics_Layer SHALL 提供 Validate 方法，对 ControlEvent 的所有字段执行完整性校验
2. WHEN event_id 为空字符串时，THE Control_Semantics_Layer SHALL 返回包含 "event_id" 字段名的校验错误
3. WHEN source 为空字符串时，THE Control_Semantics_Layer SHALL 返回包含 "source" 字段名的校验错误
4. WHEN requires_ack 为 true 的事件类型的 ControlEvent 实例的 requires_ack 字段被设置为 false 时，THE Control_Semantics_Layer SHALL 返回包含 "requires_ack" 字段名和事件类型的校验错误
5. WHEN carries_epoch 为 true 的事件类型的 ControlEvent 实例的 epoch 字段为 0 时，THE Control_Semantics_Layer SHALL 返回包含 "epoch" 字段名和事件类型的校验错误

---

### 需求 6：事件分发器

**用户故事：** 作为编排内核开发者，我需要一个事件分发器将控制事件路由到对应的处理器，以便实现事件驱动的编排逻辑。

#### 验收标准

1. THE Event_Dispatcher SHALL 提供 Register 方法，接受 EventType 和 Event_Handler 参数，将处理器注册到指定事件类型
2. THE Event_Dispatcher SHALL 提供 Dispatch 方法，接受 ControlEvent 参数，将事件路由到已注册的 Event_Handler
3. WHEN Dispatch 接收到未注册处理器的 EventType 时，THE Event_Dispatcher SHALL 返回包含该 EventType 的错误
4. WHEN Dispatch 接收到 requires_ack 为 true 的 ControlEvent 时，THE Event_Dispatcher SHALL 等待 Event_Handler 返回处理结果，并将结果作为 Dispatch 的返回值
5. WHEN Dispatch 接收到 requires_ack 为 false 的 ControlEvent 时，THE Event_Dispatcher SHALL 异步执行 Event_Handler，Dispatch 立即返回 nil
6. THE Event_Dispatcher SHALL 按 priority 字段值从高到低的顺序处理并发到达的事件，高优先级事件优先分发
7. IF Event_Handler 返回错误，THEN THE Event_Dispatcher SHALL 将该错误包装为包含 event_id 和 event_type 的分发错误返回

---

### 需求 7：事件处理器接口

**用户故事：** 作为编排内核开发者，我需要统一的事件处理器接口，以便各编排组件（CommitEngine、SurvivalOrchestrator、PersonaEngine）实现自己的事件处理逻辑。

#### 验收标准

1. THE Control_Semantics_Layer SHALL 定义 Event_Handler 接口，包含 Handle 方法，接受 context 和 ControlEvent 参数，返回 error
2. THE Control_Semantics_Layer SHALL 定义 Event_Handler 接口的 EventType 方法，返回该处理器负责的 EventType
3. WHEN Event_Handler 处理幂等事件（idempotent 为 true）时，THE Event_Handler SHALL 对同一 event_id 的重复调用产生与首次调用相同的结果且无副作用

---

### 需求 8：事件注册表

**用户故事：** 作为编排内核开发者，我需要一个事件注册表来维护所有已注册的事件处理器，以便在系统启动时完成事件路由配置。

#### 验收标准

1. THE Event_Registry SHALL 维护 EventType 到 Event_Handler 的一对一映射
2. WHEN 对同一 EventType 重复注册不同的 Event_Handler 时，THE Event_Registry SHALL 返回包含该 EventType 的重复注册错误
3. THE Event_Registry SHALL 提供 ListRegistered 方法，返回所有已注册的 EventType 列表
4. THE Event_Registry SHALL 提供 IsRegistered 方法，接受 EventType 参数，返回该类型是否已注册

---

### 需求 9：幂等性保证

**用户故事：** 作为编排内核开发者，我需要幂等事件在重复投递时不产生额外副作用，以便系统在恢复和重放场景下保持一致性。

#### 验收标准

1. THE Control_Semantics_Layer SHALL 维护已处理事件的 event_id 去重集合
2. WHEN 分发一个 idempotent 为 true 且 event_id 已存在于去重集合中的 ControlEvent 时，THE Event_Dispatcher SHALL 跳过 Event_Handler 执行并返回 nil
3. WHEN 分发一个 idempotent 为 false 的 ControlEvent 时，THE Event_Dispatcher SHALL 始终执行 Event_Handler，不进行去重检查
4. THE Control_Semantics_Layer SHALL 提供清理机制，移除超过 1 小时的已处理 event_id 记录

---

### 需求 10：Epoch 关联校验

**用户故事：** 作为编排内核开发者，我需要携带 epoch 的事件在分发时校验 epoch 有效性，以便拒绝过期事件防止状态回退。

#### 验收标准

1. WHEN 分发 carries_epoch 为 true 的 ControlEvent 时，THE Event_Dispatcher SHALL 校验事件的 epoch 字段值不小于 ControlStateManager 的 last_successful_epoch
2. IF 事件的 epoch 字段值小于 ControlStateManager 的 last_successful_epoch，THEN THE Event_Dispatcher SHALL 返回包含事件 epoch 和当前 epoch 的过期错误，且不执行 Event_Handler
3. WHEN 分发 carries_epoch 为 false 的 ControlEvent 时，THE Event_Dispatcher SHALL 跳过 epoch 校验

---

### 需求 11：JSON 序列化

**用户故事：** 作为编排内核开发者，我需要所有控制语义层的核心数据结构支持 JSON 序列化和反序列化，以便支持审计日志和诊断。

#### 验收标准

1. THE Control_Semantics_Layer SHALL 为 ControlEvent 结构体提供 JSON 序列化支持，所有字段使用 snake_case 命名
2. THE Control_Semantics_Layer SHALL 为 EventSemantics 结构体提供 JSON 序列化支持，所有字段使用 snake_case 命名
3. FOR ALL 合法的 ControlEvent 对象，JSON 序列化后再反序列化 SHALL 产生等价对象（所有字段值保持不变）
4. FOR ALL 合法的 EventSemantics 对象，JSON 序列化后再反序列化 SHALL 产生等价对象（所有字段值保持不变）
5. THE Control_Semantics_Layer SHALL 在 ControlEvent 的 JSON 输出中将 created_at 字段格式化为 RFC 3339 时间戳

---

### 需求 12：并发安全

**用户故事：** 作为编排内核开发者，我需要事件分发器和注册表在并发场景下安全运行，以便多个 goroutine 同时提交和处理事件。

#### 验收标准

1. THE Event_Dispatcher SHALL 支持多个 goroutine 并发调用 Dispatch 方法，不产生数据竞争
2. THE Event_Registry SHALL 支持多个 goroutine 并发调用 IsRegistered 和 ListRegistered 方法，不产生数据竞争
3. THE Control_Semantics_Layer 的 event_id 去重集合 SHALL 支持多个 goroutine 并发读写，不产生数据竞争
