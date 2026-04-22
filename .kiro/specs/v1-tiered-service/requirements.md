# 需求文档：阶梯服务策略落地

## 简介

本 Spec 对应 `阶梯服务策略建议.md` 中最小改动方案（第十节），目标是用最小代码改动实现三档服务等级（Standard / Platinum / Diamond）+ 流量包的商业化模型。

当前核心问题：
1. **等级推导不明确**：`cell_level` 可能被余额变化自动推导，不是明确购买的结果
2. **缺少月费产品类型**：`QuotaPurchase.package_type` 只有流量包类型（10GB/50GB/100GB/500GB/1TB），没有月费订阅类型
3. **缺少等级购买 API**：没有正式的等级购买/升降级接口
4. **TierRouter 不完整**：按等级分池的逻辑存在但不完整，缺少连接负载差异和恢复优先级差异
5. **配额熔断全局化**：配额熔断是全局的，不是按用户隔离的（依赖 Spec 2-2 的用户隔离桶）

本轮整改核心目标：**将 cell_level 正式定义为付费等级，建立等级购买 → 资源池分配 → 服务差异的完整链路。**

## 术语表

- **Service_Tier**：服务等级，对应 `users.cell_level`，取值 1（Standard）/ 2（Platinum）/ 3（Diamond）
- **Tier_Subscription**：等级订阅，用户购买的月费产品，决定 Service_Tier
- **Traffic_Package**：流量包，用户购买的配额产品，决定 `remaining_quota`
- **Resource_Pool**：资源池，按 `cells.cell_level` 划分的 Gateway 集合
- **TierRouter**：等级路由器，按用户 Service_Tier 分配到对应 Resource_Pool
- **Fuse_Policy**：熔断策略，配额耗尽时的处理策略，按用户隔离
- **Plan_Package_Type**：月费产品类型标识，如 `plan_standard_monthly`、`plan_platinum_monthly`、`plan_diamond_monthly`
- **Subscription_Expiry**：订阅到期时间，等级订阅的有效期截止时间

## 需求

### 需求 1：停止余额自动推导等级，cell_level 正式定义为付费等级

**用户故事：** 作为系统架构师，我需要 cell_level 只能通过明确购买行为变更，以便服务等级与付费行为严格绑定，不会因余额波动自动升降级。

#### 验收标准

1. THE BillingService SHALL 移除所有基于余额自动变更 `users.cell_level` 的逻辑
2. THE `users.cell_level` SHALL 仅在用户购买等级订阅时由 PurchaseTierSubscription 操作更新
3. WHEN 用户余额变化（充值、消费、退款），THE BillingService SHALL 保持 `users.cell_level` 不变
4. THE User 模型 SHALL 新增 `subscription_expires_at`（timestamp）字段，记录当前等级订阅的到期时间
5. THE User 模型 SHALL 新增 `subscription_package_type`（string）字段，记录当前生效的订阅产品类型
6. IF 等级订阅到期且未续费，THEN THE System SHALL 将 `users.cell_level` 降级为 1（Standard）

### 需求 2：QuotaPurchase.package_type 承载月费产品

**用户故事：** 作为计费系统，我需要 QuotaPurchase 能记录月费订阅购买，以便等级订阅和流量包使用统一的购买记录结构。

#### 验收标准

1. THE `QuotaPurchase.package_type` 字段 SHALL 扩展支持以下月费产品类型：`plan_standard_monthly`、`plan_platinum_monthly`、`plan_diamond_monthly`
2. THE `QuotaPurchase.package_type` 字段 SHALL 继续支持现有流量包类型：`traffic_10gb`、`traffic_50gb`、`traffic_100gb`、`traffic_500gb`、`traffic_1tb`
3. THE BillingService SHALL 维护月费产品价格表，包含每个等级的月费价格（美分）
4. WHEN 用户购买月费产品，THE BillingService SHALL 创建 `QuotaPurchase` 记录，`package_type` 设为对应的 plan 类型
5. THE `BillingLog` SHALL 新增 `log_type` 值 `subscription`，用于区分等级订阅流水与流量购买流水

### 需求 3：购买等级时直接更新 users.cell_level

**用户故事：** 作为用户，我需要购买等级订阅后立即生效，以便我能马上享受对应等级的服务能力。

#### 验收标准

1. THE BillingService SHALL 提供 `PurchaseTierSubscription` gRPC 方法，接受 user_id 和目标等级
2. WHEN 用户购买 Standard 月费，THE BillingService SHALL 将 `users.cell_level` 更新为 1
3. WHEN 用户购买 Platinum 月费，THE BillingService SHALL 将 `users.cell_level` 更新为 2
4. WHEN 用户购买 Diamond 月费，THE BillingService SHALL 将 `users.cell_level` 更新为 3
5. THE 购买操作 SHALL 在同一数据库事务中完成余额扣减、等级更新、购买记录创建、计费流水写入
6. WHEN 用户升级等级（如 Standard → Platinum），THE BillingService SHALL 按新等级全额收费，不做差价计算（第一期简化）
7. WHEN 用户降级等级（如 Diamond → Standard），THE BillingService SHALL 在当前订阅到期后生效，不立即变更
8. IF 用户余额不足，THEN THE BillingService SHALL 拒绝购买并返回 `FAILED_PRECONDITION` 错误

### 需求 4：TierRouter 按等级分配资源池

**用户故事：** 作为用户，我需要根据我的服务等级被分配到对应质量的资源池，以便高等级用户获得更好的网络体验。

#### 验收标准

1. THE CellService SHALL 在分配 Gateway 时按用户 `cell_level` 筛选对应等级的 Cell
2. WHEN Standard 用户请求分配，THE CellService SHALL 从 `cell_level = 1` 的标准资源池中选择
3. WHEN Platinum 用户请求分配，THE CellService SHALL 从 `cell_level = 2` 的高优先级资源池中选择
4. WHEN Diamond 用户请求分配，THE CellService SHALL 从 `cell_level = 3` 的高隔离资源池中选择
5. IF 对应等级资源池无可用 Cell，THEN THE CellService SHALL 降级到低一级资源池分配，并记录降级日志
6. THE CellService SHALL 在 `AllocateGateway` 请求中增加 `user_id` 参数，用于查询用户等级

### 需求 5：第一期服务差异 — 连接负载差异与恢复优先级差异

**用户故事：** 作为高等级用户，我需要在连接负载和故障恢复时获得优先待遇，以便我的服务体验明显优于低等级用户。

#### 验收标准

1. THE CellService SHALL 为不同等级设置不同的 Gateway 最大连接数上限：Standard 使用默认上限，Platinum 上限为默认值的 70%，Diamond 上限为默认值的 40%
2. THE CellService SHALL 在选择 Gateway 时，按等级应用不同的负载阈值：Standard 在负载 < 80% 时可分配，Platinum 在负载 < 60% 时可分配，Diamond 在负载 < 40% 时可分配
3. WHEN Gateway 故障需要迁移用户，THE CellScheduler SHALL 按等级优先级排序：Diamond 优先迁移，Platinum 次之，Standard 最后
4. THE CellScheduler SHALL 在恢复调度中为 Diamond 用户优先选择网络质量最高的备选 Gateway
5. THE 等级对应的负载阈值和连接上限 SHALL 通过配置表定义，支持运行时调整

### 需求 6：配额熔断按用户隔离

**用户故事：** 作为 Gateway 运维，我需要配额熔断只影响对应用户，以便一个用户配额耗尽不影响同一 Gateway 上的其他用户。

#### 验收标准

1. THE Gateway SHALL 在用户配额耗尽时仅熔断该用户的连接，其他用户的连接不受影响
2. THE Gateway SHALL 通过 Spec 2-2 建立的用户隔离配额桶（`QuotaBucketManager`）执行按用户熔断
3. WHEN 用户配额耗尽，THE Gateway SHALL 向 OS 上报该用户的熔断事件（user_id + 熔断原因）
4. THE OS SHALL 在收到熔断事件后记录到 `BillingLog`（log_type = `fuse`）
5. WHEN 用户购买新流量包或续费后，THE OS SHALL 通过 `QuotaPush` 下发新配额，Gateway 自动解除该用户的熔断状态

### 需求 7：等级订阅到期处理

**用户故事：** 作为系统，我需要在等级订阅到期时自动处理降级，以便未续费用户不会继续享受高等级服务。

#### 验收标准

1. THE System SHALL 运行定期任务（每小时检查一次），扫描 `subscription_expires_at` 已过期的用户
2. WHEN 用户等级订阅到期且 `auto_renew = true` 且余额充足，THE System SHALL 自动续费并延长订阅期
3. WHEN 用户等级订阅到期且（`auto_renew = false` 或余额不足），THE System SHALL 将 `cell_level` 降级为 1（Standard）
4. THE System SHALL 在降级时记录 `BillingLog`（log_type = `downgrade`）
5. THE System SHALL 在降级后触发 TierRouter 重新分配，将用户迁移到 Standard 资源池
