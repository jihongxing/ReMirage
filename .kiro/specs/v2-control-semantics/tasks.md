# 任务清单：V2 控制语义层

## 1. 枚举与常量定义
- [x] 1.1 创建 `pkg/orchestrator/events/types.go`，定义 EventType（8 种）、EventScope（3 种）、AllEventTypes 切片
- [x] 1.2 单元测试：验证 8 种 EventType 字符串表示互不相同，EventScope 三种值正确

## 2. EventSemantics 语义属性矩阵
- [x] 2.1 创建 `pkg/orchestrator/events/semantics.go`，定义 EventSemantics 结构体、EventSemanticsMap 常量表、GetSemantics 函数
- [x] 2.2 PBT: Property 1 — EventSemantics 矩阵完整性与正确性（对所有 EventType 验证语义属性与矩阵一致，未定义类型返回 nil）

## 3. ControlEvent 结构体与校验
- [x] 3.1 创建 `pkg/orchestrator/events/event.go`，定义 ControlEvent 结构体（含 JSON tag snake_case）和 Validate 方法
- [x] 3.2 PBT: Property 3 — Validate 综合校验正确性（event_id/source 非空、event_type/target_scope 枚举、priority 范围、requires_ack 一致性、epoch 一致性）

## 4. 工厂函数
- [x] 4.1 创建 `pkg/orchestrator/events/factory.go`，实现 8 个具名工厂函数和 NewEvent 通用工厂函数
- [x] 4.2 PBT: Property 2 — 工厂函数默认值填充与自动字段生成（UUID 格式、created_at 非零、默认值与 EventSemantics 一致、carries_epoch 逻辑）

## 5. 错误类型
- [x] 5.1 创建 `pkg/orchestrator/events/errors.go`，定义 ErrValidation、ErrInvalidEventType、ErrInvalidScope、ErrHandlerNotRegistered、ErrDuplicateRegistration、ErrEpochStale、ErrDispatchFailed 及其 Error()/Unwrap() 方法
- [x] 5.2 单元测试：验证所有错误类型的 Error() 方法包含关键字段信息，ErrDispatchFailed.Unwrap() 返回 Cause

## 6. EventHandler 接口
- [x] 6.1 创建 `pkg/orchestrator/events/handler.go`，定义 EventHandler 接口（Handle + EventType 方法）

## 7. EventRegistry 注册表
- [x] 7.1 创建 `pkg/orchestrator/events/registry.go`，定义 EventRegistry 接口和基于 sync.RWMutex 的实现（Register/GetHandler/ListRegistered/IsRegistered）
- [x] 7.2 PBT: Property 4 — 注册表一对一映射与重复注册拒绝

## 8. DeduplicationStore 去重集合
- [x] 8.1 创建 `pkg/orchestrator/events/dedup.go`，定义 DeduplicationStore 接口和基于 sync.Map 的实现（Contains/Add/Cleanup，1 小时过期）
- [x] 8.2 PBT: Property 9 — 去重集合清理正确性

## 9. EventDispatcher 分发器
- [x] 9.1 创建 `pkg/orchestrator/events/dispatcher.go`，定义 EpochProvider 接口、EventDispatcher 接口和实现（校验→去重→epoch 校验→路由→同步/异步分发→优先级排序）
- [x] 9.2 PBT: Property 5 — 幂等去重正确性（idempotent=true 重复跳过，idempotent=false 每次执行）
- [x] 9.3 PBT: Property 6 — Epoch 校验正确性（carries_epoch=true 校验 epoch ≥ lastSuccessfulEpoch，carries_epoch=false 跳过）
- [x] 9.4 PBT: Property 7 — 同步/异步分发行为（requires_ack=true 同步等待，requires_ack=false 立即返回）
- [x] 9.5 PBT: Property 8 — Handler 错误包装（ErrDispatchFailed 包含 event_id、event_type、Cause）

## 10. JSON 序列化
- [x] 10.1 PBT: Property 10 — ControlEvent 和 EventSemantics 的 JSON round-trip（序列化→反序列化等价，created_at RFC 3339，snake_case key）

## 11. 并发安全集成测试
- [x] 11.1 集成测试：EventDispatcher 多 goroutine 并发 Dispatch（-race 检测）
- [x] 11.2 集成测试：EventRegistry 多 goroutine 并发读写（-race 检测）
- [x] 11.3 集成测试：DeduplicationStore 多 goroutine 并发读写（-race 检测）

## 12. 端到端集成测试
- [x] 12.1 集成测试：完整事件流程（工厂创建→校验→注册 Handler→分发→Handler 执行→幂等重放→epoch 拒绝）
