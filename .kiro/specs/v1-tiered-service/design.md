# 设计文档：阶梯服务策略落地

## 概述

本设计将 Mirage 的 `cell_level` 从"余额副作用"正式升级为"付费等级"，建立等级订阅购买 → cell_level 更新 → TierRouter 资源池分配 → 服务差异的完整链路。改动集中在 mirage-os 的 BillingService、CellService、CellScheduler，以及 DB schema 扩展。

## 设计原则

1. **最小改动**：复用现有 `cell_level`、`QuotaPurchase`、`TierRouter` 骨架，不新建复杂订单系统
2. **向后兼容**：新增字段均有默认值，未购买订阅的用户默认 Standard
3. **事务安全**：等级购买在单一 DB 事务中完成余额扣减 + 等级更新 + 记录创建
4. **依赖 Spec 2-2**：配额熔断按用户隔离依赖 `QuotaBucketManager`（Spec 2-2 已实现）

---

## 模块 1：DB Schema 扩展（需求 1、2）

### 改动范围

- `mirage-os/pkg/models/db.go`：User 模型新增字段、QuotaPurchase 扩展

### 设计细节

#### User 模型扩展

```go
type User struct {
    // ... 现有字段保持不变

    // 新增：等级订阅
    SubscriptionExpiresAt  *time.Time `json:"subscription_expires_at"`
    SubscriptionPackageType string    `gorm:"size:32;default:''" json:"subscription_package_type"`
}
```

#### 月费产品类型常量

```go
const (
    PlanStandardMonthly  = "plan_standard_monthly"
    PlanPlatinumMonthly  = "plan_platinum_monthly"
    PlanDiamondMonthly   = "plan_diamond_monthly"
)
```

#### BillingLog.LogType 扩展

现有值：`traffic` / `deposit` / `purchase` / `refund`
新增值：`subscription` / `fuse` / `downgrade`

---

## 模块 2：等级订阅价格表与购买逻辑（需求 2、3）

### 改动范围

- `mirage-os/services/billing/billing_service.go`：新增 `PurchaseTierSubscription` 方法、月费价格表
- `mirage-os/api/proto/mirage.proto`：新增 `PurchaseTierSubscription` RPC

### 设计细节

#### 月费价格表

```go
// 等级订阅价格表（美分/月）
var tierPrices = map[string]uint64{
    PlanStandardMonthly: 29900,  // $299/月
    PlanPlatinumMonthly: 99900,  // $999/月
    PlanDiamondMonthly:  299900, // $2,999/月
}

// 等级映射
var tierLevelMap = map[string]int{
    PlanStandardMonthly: 1,
    PlanPlatinumMonthly: 2,
    PlanDiamondMonthly:  3,
}
```

#### Proto 扩展

```protobuf
service BillingService {
    // ... 现有 RPC
    rpc PurchaseTierSubscription(TierSubscriptionRequest) returns (TierSubscriptionResponse);
}

message TierSubscriptionRequest {
    string account_id = 1;
    string plan_type = 2;  // plan_standard_monthly / plan_platinum_monthly / plan_diamond_monthly
}

message TierSubscriptionResponse {
    bool success = 1;
    string message = 2;
    uint64 cost_usd = 3;           // 美分
    uint64 remaining_balance = 4;  // 美分
    int32 new_cell_level = 5;
    int64 expires_at = 6;
}
```

#### PurchaseTierSubscription 实现

```go
func (s *BillingServiceImpl) PurchaseTierSubscription(ctx context.Context, req *pb.TierSubscriptionRequest) (*pb.TierSubscriptionResponse, error) {
    planType := req.GetPlanType()
    priceUSD, ok := tierPrices[planType]
    if !ok {
        return nil, status.Error(codes.InvalidArgument, "invalid plan_type")
    }
    newLevel, ok := tierLevelMap[planType]
    if !ok {
        return nil, status.Error(codes.InvalidArgument, "invalid plan_type")
    }

    totalPrice := float64(priceUSD) / 100.0
    var resp *pb.TierSubscriptionResponse

    err := s.db.Transaction(func(tx *gorm.DB) error {
        var user models.User
        if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
            Where("user_id = ?", req.GetAccountId()).
            First(&user).Error; err != nil {
            return status.Error(codes.NotFound, "account not found")
        }

        if user.BalanceUSD < totalPrice {
            return status.Error(codes.FailedPrecondition, "insufficient balance")
        }

        // 降级检查：如果目标等级 < 当前等级，标记为到期后生效
        if newLevel < user.CellLevel {
            // 记录降级意向，到期后由定时任务处理
            // 第一期简化：直接拒绝降级，用户等到期自然降级
            return status.Error(codes.FailedPrecondition, "downgrade takes effect after current subscription expires")
        }

        // 扣减余额
        if err := tx.Model(&user).Update("balance_usd", gorm.Expr("balance_usd - ?", totalPrice)).Error; err != nil {
            return status.Error(codes.Internal, "failed to deduct balance")
        }

        // 更新等级和订阅信息
        expiresAt := time.Now().Add(30 * 24 * time.Hour)
        if err := tx.Model(&user).Updates(map[string]any{
            "cell_level":                newLevel,
            "subscription_expires_at":   expiresAt,
            "subscription_package_type": planType,
        }).Error; err != nil {
            return status.Error(codes.Internal, "failed to update tier")
        }

        // 创建购买记录
        purchase := models.QuotaPurchase{
            UserID:      req.GetAccountId(),
            PackageType: planType,
            QuotaBytes:  0, // 月费不含流量
            CostUSD:     totalPrice,
            CellLevel:   newLevel,
            ExpiresAt:   &expiresAt,
        }
        if err := tx.Create(&purchase).Error; err != nil {
            return status.Error(codes.Internal, "failed to create purchase record")
        }

        // 计费流水
        billingLog := models.BillingLog{
            UserID:  req.GetAccountId(),
            CostUSD: totalPrice,
            LogType: "subscription",
        }
        if err := tx.Create(&billingLog).Error; err != nil {
            return status.Error(codes.Internal, "failed to create billing log")
        }

        newBalance := user.BalanceUSD - totalPrice
        resp = &pb.TierSubscriptionResponse{
            Success:          true,
            Message:          "subscription activated",
            CostUsd:          priceUSD,
            RemainingBalance: uint64(newBalance * 100),
            NewCellLevel:     int32(newLevel),
            ExpiresAt:        expiresAt.Unix(),
        }
        return nil
    })

    if err != nil {
        return nil, err
    }
    return resp, nil
}
```

#### 移除余额自动推导等级

在 `billing_service.go` 中搜索所有修改 `cell_level` 的代码路径，确保只有 `PurchaseTierSubscription` 和到期降级任务能修改该字段。具体：
- `Deposit` 确认回调中移除等级推导
- `PurchaseQuota`（流量包购买）中移除等级推导
- 任何余额变更回调中移除等级推导

---

## 模块 3：TierRouter 按等级分配资源池（需求 4）

### 改动范围

- `mirage-os/services/cellular/cell_service.go`：`AllocateGateway` 增加用户等级感知
- `mirage-os/pkg/strategy/cell_manager.go`：恢复调度增加等级优先级

### 设计细节

#### AllocateGateway 改造

```go
func (s *CellServiceImpl) AllocateGateway(ctx context.Context, req *pb.AllocateRequest) (*pb.AllocateResponse, error) {
    // 查询用户等级
    userLevel := int(req.GetPreferredLevel())
    if req.GetUserId() != "" {
        var user models.User
        if err := s.db.Where("user_id = ?", req.GetUserId()).First(&user).Error; err == nil {
            userLevel = user.CellLevel
        }
    }
    if userLevel == 0 {
        userLevel = 1
    }

    // 按等级筛选资源池，支持降级
    var cells []models.Cell
    for level := userLevel; level >= 1; level-- {
        s.db.Where("cell_level = ? AND status = ?", level, "active").Find(&cells)
        if len(cells) > 0 {
            if level < userLevel {
                log.Printf("[TierRouter] 用户等级 %d 资源池无可用 Cell，降级到等级 %d", userLevel, level)
            }
            break
        }
    }
    if len(cells) == 0 {
        return nil, status.Errorf(codes.NotFound, "无满足条件的可用蜂窝")
    }

    // 按等级应用不同负载阈值
    threshold := getTierLoadThreshold(userLevel)
    // ... 选择负载最低且低于阈值的 Cell
}
```

#### 等级服务差异配置表

```go
// TierConfig 等级服务差异配置
type TierConfig struct {
    MaxLoadPercent    float32 // 分配时的负载阈值
    ConnectionRatio   float32 // 连接上限比例（相对默认值）
    RecoveryPriority  int     // 恢复优先级（数字越大越优先）
}

var tierConfigs = map[int]TierConfig{
    1: {MaxLoadPercent: 80, ConnectionRatio: 1.0, RecoveryPriority: 1},  // Standard
    2: {MaxLoadPercent: 60, ConnectionRatio: 0.7, RecoveryPriority: 2},  // Platinum
    3: {MaxLoadPercent: 40, ConnectionRatio: 0.4, RecoveryPriority: 3},  // Diamond
}

func getTierLoadThreshold(level int) float32 {
    if cfg, ok := tierConfigs[level]; ok {
        return cfg.MaxLoadPercent
    }
    return 80
}

func getTierConnectionRatio(level int) float32 {
    if cfg, ok := tierConfigs[level]; ok {
        return cfg.ConnectionRatio
    }
    return 1.0
}

func getTierRecoveryPriority(level int) int {
    if cfg, ok := tierConfigs[level]; ok {
        return cfg.RecoveryPriority
    }
    return 1
}
```

---

## 模块 4：恢复优先级差异（需求 5）

### 改动范围

- `mirage-os/pkg/strategy/cell_manager.go`：恢复调度增加等级排序

### 设计细节

#### 恢复调度按等级排序

```go
// RecoverUsers 故障恢复时按等级优先级排序迁移
func (s *CellScheduler) RecoverUsers(failedGatewayID string) error {
    // 查询该 Gateway 上所有活跃会话
    var sessions []GatewaySessionWithLevel // join users 获取 cell_level

    // 按等级降序排序：Diamond(3) > Platinum(2) > Standard(1)
    sort.Slice(sessions, func(i, j int) bool {
        return sessions[i].CellLevel > sessions[j].CellLevel
    })

    for _, session := range sessions {
        // Diamond 用户优先选择网络质量最高的备选 Gateway
        // Platinum 次之
        // Standard 最后
        target := s.selectRecoveryTarget(session.CellLevel)
        if target != nil {
            s.migrateSession(session.SessionID, target.GatewayID)
        }
    }
    return nil
}
```

---

## 模块 5：配额熔断按用户隔离（需求 6）

### 改动范围

- 依赖 Spec 2-2 的 `QuotaBucketManager`（已实现按用户隔离桶）
- `mirage-os/gateway-bridge/pkg/grpc/handlers.go`：增加熔断事件处理

### 设计细节

本模块主要是确保 Spec 2-2 的用户隔离桶与等级体系正确集成：

1. Gateway 侧 `QuotaBucketManager.Consume` 返回 false 时，仅断开该用户连接
2. Gateway 上报熔断事件到 OS（复用 Spec 2-2 的 `ReportSessionEvent`，增加 `FUSE_TRIGGERED` 事件类型）
3. OS 收到熔断事件后写入 `BillingLog`（log_type = `fuse`）
4. 用户购买新流量包后，OS 通过 `QuotaPush` 下发新配额，Gateway 自动解除熔断

---

## 模块 6：等级订阅到期处理（需求 7）

### 改动范围

- `mirage-os/services/billing/subscription_manager.go`（新建）：定期任务处理到期

### 设计细节

```go
// SubscriptionManager 订阅到期管理器
type SubscriptionManager struct {
    db          *gorm.DB
    billing     *BillingServiceImpl
    interval    time.Duration
}

func NewSubscriptionManager(db *gorm.DB, billing *BillingServiceImpl) *SubscriptionManager {
    return &SubscriptionManager{
        db:       db,
        billing:  billing,
        interval: 1 * time.Hour,
    }
}

// Start 启动定期检查
func (m *SubscriptionManager) Start(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(m.interval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                m.processExpiredSubscriptions()
            }
        }
    }()
}

// processExpiredSubscriptions 处理到期订阅
func (m *SubscriptionManager) processExpiredSubscriptions() {
    var users []models.User
    m.db.Where("subscription_expires_at IS NOT NULL AND subscription_expires_at < ? AND cell_level > 1", time.Now()).
        Find(&users)

    for _, user := range users {
        if user.AutoRenew {
            // 尝试自动续费
            if err := m.tryAutoRenew(user); err == nil {
                continue
            }
        }
        // 降级到 Standard
        m.downgradeToStandard(user)
    }
}

// tryAutoRenew 尝试自动续费
func (m *SubscriptionManager) tryAutoRenew(user models.User) error {
    planType := user.SubscriptionPackageType
    if planType == "" {
        return fmt.Errorf("no subscription package type")
    }
    priceUSD, ok := tierPrices[planType]
    if !ok {
        return fmt.Errorf("unknown plan type: %s", planType)
    }
    totalPrice := float64(priceUSD) / 100.0
    if user.BalanceUSD < totalPrice {
        return fmt.Errorf("insufficient balance for auto-renew")
    }

    return m.db.Transaction(func(tx *gorm.DB) error {
        // 扣减余额 + 延长订阅
        expiresAt := time.Now().Add(30 * 24 * time.Hour)
        tx.Model(&user).Updates(map[string]any{
            "balance_usd":              gorm.Expr("balance_usd - ?", totalPrice),
            "subscription_expires_at":  expiresAt,
        })
        // 创建购买记录和流水
        tx.Create(&models.QuotaPurchase{
            UserID: user.UserID, PackageType: planType,
            CostUSD: totalPrice, CellLevel: user.CellLevel, ExpiresAt: &expiresAt,
        })
        tx.Create(&models.BillingLog{
            UserID: user.UserID, CostUSD: totalPrice, LogType: "subscription",
        })
        return nil
    })
}

// downgradeToStandard 降级到 Standard
func (m *SubscriptionManager) downgradeToStandard(user models.User) {
    m.db.Model(&user).Updates(map[string]any{
        "cell_level":                1,
        "subscription_package_type": "",
        "subscription_expires_at":   nil,
    })
    m.db.Create(&models.BillingLog{
        UserID: user.UserID, LogType: "downgrade",
    })
    log.Printf("[SubscriptionManager] 用户 %s 等级订阅到期，降级为 Standard", user.UserID)
}
```

---

## 模块 7：纯函数与属性测试

### PurchaseTierSubscription 纯函数

```go
// PurchaseTierPure 纯函数版本，用于属性测试
func PurchaseTierPure(balanceUSD float64, currentLevel int, planType string) (newBalance float64, newLevel int, err error) {
    priceUSD, ok := tierPrices[planType]
    if !ok {
        return balanceUSD, currentLevel, fmt.Errorf("invalid plan_type")
    }
    newLvl, ok := tierLevelMap[planType]
    if !ok {
        return balanceUSD, currentLevel, fmt.Errorf("invalid plan_type")
    }
    totalPrice := float64(priceUSD) / 100.0
    if balanceUSD < totalPrice {
        return balanceUSD, currentLevel, fmt.Errorf("insufficient balance")
    }
    if newLvl < currentLevel {
        return balanceUSD, currentLevel, fmt.Errorf("downgrade not immediate")
    }
    return balanceUSD - totalPrice, newLvl, nil
}
```

### 属性测试设计

1. **等级购买不变量**：购买成功后 `newLevel == tierLevelMap[planType]`，且 `newBalance == balanceUSD - price`
2. **余额不足拒绝**：余额 < 价格时，购买失败，余额和等级不变
3. **降级拒绝**：目标等级 < 当前等级时，购买失败
4. **资源池分配不变量**：分配结果的 cell_level <= 用户 cell_level（允许降级分配）
5. **恢复优先级排序**：排序后 Diamond 在前，Standard 在后
6. **熔断隔离**：一个用户耗尽不影响其他用户（复用 Spec 2-2 测试）

---

## 配置变更

新增等级服务差异配置（可通过环境变量或配置文件覆盖）：

```go
var tierConfigs = map[int]TierConfig{
    1: {MaxLoadPercent: 80, ConnectionRatio: 1.0, RecoveryPriority: 1},
    2: {MaxLoadPercent: 60, ConnectionRatio: 0.7, RecoveryPriority: 2},
    3: {MaxLoadPercent: 40, ConnectionRatio: 0.4, RecoveryPriority: 3},
}
```

## 不在本次范围内

- 增值服务（独享 Gateway、固定区域）→ 第二期
- 按天计费、动态竞价 → 不做
- 区域组合定价 → 不做
- 前端 UI（等级购买页面）→ 独立 Spec
- Client 侧等级感知（SUB-3）→ Spec 3-1
