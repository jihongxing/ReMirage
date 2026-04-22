# 任务清单：阶梯服务策略落地

## 任务

- [x] 1. DB Schema 扩展：等级订阅字段
  - [x] 1.1 在 `User` 模型中新增 `subscription_expires_at`（*time.Time）和 `subscription_package_type`（string）字段
  - [x] 1.2 定义月费产品类型常量：`PlanStandardMonthly`、`PlanPlatinumMonthly`、`PlanDiamondMonthly`
  - [x] 1.3 扩展 `BillingLog.LogType` 支持新值：`subscription`、`fuse`、`downgrade`
  - [x] 1.4 运行 AutoMigrate 验证 schema 变更生效

- [x] 2. 移除余额自动推导等级逻辑
  - [x] 2.1 审计 `billing_service.go` 中所有修改 `cell_level` 的代码路径，移除基于余额变化的等级推导
  - [x] 2.2 审计 `Deposit` 确认回调、`PurchaseQuota` 流量包购买中是否存在等级推导，如有则移除
  - [x] 2.3 编写单元测试：充值/购买流量包后 `cell_level` 保持不变

- [x] 3. Proto 扩展：等级订阅 RPC
  - [x] 3.1 在 `mirage.proto` 中新增 `TierSubscriptionRequest` 和 `TierSubscriptionResponse` 消息
  - [x] 3.2 在 `BillingService` service 中新增 `PurchaseTierSubscription` RPC
  - [x] 3.3 重新生成 Go proto 代码

- [x] 4. 等级订阅购买逻辑
  - [x] 4.1 在 `billing_service.go` 中新增月费价格表 `tierPrices` 和等级映射表 `tierLevelMap`
  - [x] 4.2 实现 `PurchaseTierSubscription` 方法：事务内完成余额扣减、cell_level 更新、subscription_expires_at 设置、QuotaPurchase 记录创建、BillingLog 流水写入
  - [x] 4.3 实现降级拒绝逻辑：目标等级 < 当前等级时返回错误，降级在到期后由定时任务处理
  - [x] 4.4 实现纯函数 `PurchaseTierPure` 用于属性测试
  - [x] 4.5 编写属性测试：购买成功后 newLevel == tierLevelMap[planType] 且 newBalance == balance - price
  - [x] 4.6 编写属性测试：余额不足时购买失败，余额和等级不变

- [x] 5. TierRouter 按等级分配资源池
  - [x] 5.1 新建等级服务差异配置表 `tierConfigs`（MaxLoadPercent、ConnectionRatio、RecoveryPriority）
  - [x] 5.2 改造 `AllocateGateway`：增加 `user_id` 参数，查询用户 cell_level，按等级筛选 Cell
  - [x] 5.3 实现资源池降级分配：对应等级池无可用 Cell 时降级到低一级资源池，记录降级日志
  - [x] 5.4 改造 Gateway 选择逻辑：按等级应用不同负载阈值（Standard < 80%、Platinum < 60%、Diamond < 40%）
  - [x] 5.5 提取纯函数 `SelectBestCellForTier`，用于属性测试
  - [x] 5.6 编写属性测试：分配结果的 cell_level <= 用户 cell_level

- [x] 6. 恢复优先级差异
  - [x] 6.1 改造 `CellScheduler`：故障恢复时查询受影响用户的 cell_level
  - [x] 6.2 实现按等级优先级排序迁移：Diamond(3) 优先 → Platinum(2) → Standard(1)
  - [x] 6.3 Diamond 用户优先选择网络质量最高的备选 Gateway
  - [x] 6.4 编写属性测试：恢复排序后 Diamond 在前，Standard 在后

- [x] 7. 配额熔断按用户隔离
  - [x] 7.1 确认 Spec 2-2 的 `QuotaBucketManager` 按用户隔离桶已就绪
  - [x] 7.2 在 Gateway 熔断回调中仅断开对应用户连接，上报熔断事件到 OS
  - [x] 7.3 在 OS gateway-bridge 中处理熔断事件，写入 `BillingLog`（log_type = `fuse`）
  - [x] 7.4 验证用户购买新流量包后 `QuotaPush` 下发新配额，Gateway 自动解除熔断

- [x] 8. 等级订阅到期处理
  - [x] 8.1 新建 `services/billing/subscription_manager.go`，实现 `SubscriptionManager`
  - [x] 8.2 实现 `processExpiredSubscriptions`：扫描到期用户，尝试自动续费或降级
  - [x] 8.3 实现 `tryAutoRenew`：auto_renew=true 且余额充足时自动续费
  - [x] 8.4 实现 `downgradeToStandard`：降级到 Standard，清空订阅信息，记录 downgrade 日志
  - [x] 8.5 在 OS 启动时注册 SubscriptionManager 定期任务（每小时执行）
  - [x] 8.6 编写属性测试：到期且不续费的用户处理后 cell_level = 1

- [x] 9. 集成测试
  - [x] 9.1 编写等级购买端到端测试：购买 Platinum → cell_level 变为 2 → 分配到 cell_level=2 的资源池
  - [x] 9.2 编写升级测试：Standard → Diamond → cell_level 变为 3
  - [x] 9.3 编写到期降级测试：订阅到期 + auto_renew=false → cell_level 降为 1
  - [x] 9.4 编写自动续费测试：订阅到期 + auto_renew=true + 余额充足 → 订阅延长
  - [x] 9.5 编写配额熔断隔离测试：同一 Gateway 上两个用户，一个耗尽不影响另一个
