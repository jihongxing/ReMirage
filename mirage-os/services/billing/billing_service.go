package billing

import (
	"context"
	"fmt"
	pb "mirage-os/api/proto"
	"mirage-os/pkg/models"
	"regexp"
	"time"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PackagePriceInfo 流量包价格信息
type PackagePriceInfo struct {
	PriceUSD   uint64 // 美分
	QuotaBytes int64
}

// 流量包价格表：PackageType × CellLevel → (priceUSD cents, quotaBytes)
// CellLevel: "standard"=1, "platinum"=2, "diamond"=3
var packagePrices = map[pb.PackageType]map[string]PackagePriceInfo{
	pb.PackageType_PACKAGE_10GB: {
		"standard": {PriceUSD: 200, QuotaBytes: 10737418240},
		"platinum": {PriceUSD: 400, QuotaBytes: 10737418240},
		"diamond":  {PriceUSD: 800, QuotaBytes: 10737418240},
	},
	pb.PackageType_PACKAGE_50GB: {
		"standard": {PriceUSD: 800, QuotaBytes: 53687091200},
		"platinum": {PriceUSD: 1600, QuotaBytes: 53687091200},
		"diamond":  {PriceUSD: 3200, QuotaBytes: 53687091200},
	},
	pb.PackageType_PACKAGE_100GB: {
		"standard": {PriceUSD: 1400, QuotaBytes: 107374182400},
		"platinum": {PriceUSD: 2800, QuotaBytes: 107374182400},
		"diamond":  {PriceUSD: 5600, QuotaBytes: 107374182400},
	},
	pb.PackageType_PACKAGE_500GB: {
		"standard": {PriceUSD: 6000, QuotaBytes: 536870912000},
		"platinum": {PriceUSD: 12000, QuotaBytes: 536870912000},
		"diamond":  {PriceUSD: 24000, QuotaBytes: 536870912000},
	},
	pb.PackageType_PACKAGE_1TB: {
		"standard": {PriceUSD: 10000, QuotaBytes: 1099511627776},
		"platinum": {PriceUSD: 20000, QuotaBytes: 1099511627776},
		"diamond":  {PriceUSD: 40000, QuotaBytes: 1099511627776},
	},
}

// 等级订阅价格表（美分/月）
var tierPrices = map[string]uint64{
	models.PlanStandardMonthly: 29900,  // $299/月
	models.PlanPlatinumMonthly: 99900,  // $999/月
	models.PlanDiamondMonthly:  299900, // $2,999/月
}

// 等级映射
var tierLevelMap = map[string]int{
	models.PlanStandardMonthly: 1,
	models.PlanPlatinumMonthly: 2,
	models.PlanDiamondMonthly:  3,
}

// GetTierPrice 查询等级订阅价格（导出供测试使用）
func GetTierPrice(planType string) (uint64, bool) {
	price, ok := tierPrices[planType]
	return price, ok
}

// GetTierLevel 查询等级映射（导出供测试使用）
func GetTierLevel(planType string) (int, bool) {
	level, ok := tierLevelMap[planType]
	return level, ok
}

// GetPackagePrice 查询流量包价格（导出供测试使用）
func GetPackagePrice(pkg pb.PackageType, cellLevel string) (PackagePriceInfo, bool) {
	levels, ok := packagePrices[pkg]
	if !ok {
		return PackagePriceInfo{}, false
	}
	info, ok := levels[cellLevel]
	return info, ok
}

// BillingServiceImpl 计费 gRPC 服务实现
type BillingServiceImpl struct {
	pb.UnimplementedBillingServiceServer
	db          *gorm.DB
	rdb         *redis.Client
	moneroMgr   *MoneroManager
	quotaBridge *QuotaBridge
}

// NewBillingServiceImpl 创建计费服务
func NewBillingServiceImpl(db *gorm.DB, rdb *redis.Client, moneroMgr *MoneroManager) *BillingServiceImpl {
	return &BillingServiceImpl{
		db:        db,
		rdb:       rdb,
		moneroMgr: moneroMgr,
	}
}

// SetQuotaBridge 注入配额桥接器
func (s *BillingServiceImpl) SetQuotaBridge(bridge *QuotaBridge) {
	s.quotaBridge = bridge
}

var txHashRegex = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// CreateAccount 创建账户
func (s *BillingServiceImpl) CreateAccount(ctx context.Context, req *pb.CreateAccountRequest) (*pb.CreateAccountResponse, error) {
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.GetPublicKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "public_key is required")
	}

	// 检查唯一性
	var count int64
	if err := s.db.Model(&models.User{}).Where("user_id = ? OR hardware_public_key = ?", req.GetUserId(), req.GetPublicKey()).Count(&count).Error; err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if count > 0 {
		return nil, status.Error(codes.AlreadyExists, "account already exists")
	}

	user := models.User{
		UserID:            req.GetUserId(),
		HardwarePublicKey: req.GetPublicKey(),
		Status:            "active",
	}

	if err := s.db.Create(&user).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to create account")
	}

	// 生成 Monero 子地址
	if s.moneroMgr != nil && s.moneroMgr.RPCClient != nil {
		addr, err := s.moneroMgr.RPCClient.CreateAddress(ctx, 0)
		if err == nil {
			s.db.Model(&user).Update("xmr_address", addr)
		}
	}

	return &pb.CreateAccountResponse{
		Success:   true,
		Message:   "account created",
		AccountId: req.GetUserId(),
		CreatedAt: user.CreatedAt.Unix(),
	}, nil
}

// Deposit 充值（辅助接口，主流程由 MoneroManager.MonitorDeposits 自动检测）
func (s *BillingServiceImpl) Deposit(ctx context.Context, req *pb.DepositRequest) (*pb.DepositResponse, error) {
	if req.GetAccountId() == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if !txHashRegex.MatchString(req.GetTxHash()) {
		return nil, status.Error(codes.InvalidArgument, "invalid tx_hash format, must be 64 hex chars")
	}

	// 查找用户
	var user models.User
	if err := s.db.Where("user_id = ?", req.GetAccountId()).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "account not found")
	}

	// 创建 pending Deposit 记录
	deposit := models.Deposit{
		UserID: req.GetAccountId(),
		TxHash: req.GetTxHash(),
		Status: "pending",
	}

	if err := s.db.Create(&deposit).Error; err != nil {
		return nil, status.Error(codes.AlreadyExists, "deposit already exists")
	}

	return &pb.DepositResponse{
		Success:    true,
		Message:    "deposit record created, awaiting confirmation",
		BalanceUsd: uint64(user.BalanceUSD * 100),
	}, nil
}

// GetBalance 查询余额
func (s *BillingServiceImpl) GetBalance(ctx context.Context, req *pb.BalanceRequest) (*pb.BalanceResponse, error) {
	if req.GetAccountId() == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	var user models.User
	if err := s.db.Where("user_id = ?", req.GetAccountId()).First(&user).Error; err != nil {
		return nil, status.Error(codes.NotFound, "account not found")
	}

	quotaInfo := &pb.QuotaInfo{
		TotalBytes:     uint64(user.TotalQuota),
		RemainingBytes: uint64(user.RemainingQuota),
	}
	if user.TotalQuota > user.RemainingQuota {
		quotaInfo.UsedBytes = uint64(user.TotalQuota - user.RemainingQuota)
	}
	if user.QuotaExpiresAt != nil {
		quotaInfo.ExpiresAt = user.QuotaExpiresAt.Unix()
	}

	// 查询蜂窝分配
	var gateways []models.Gateway
	s.db.Where("user_id = ? AND is_online = ?", req.GetAccountId(), true).Find(&gateways)

	var cells []*pb.CellAllocation
	for _, gw := range gateways {
		var cell models.Cell
		if err := s.db.Where("cell_id = ?", gw.CellID).First(&cell).Error; err != nil {
			continue
		}
		levelStr := "standard"
		switch cell.CellLevel {
		case 2:
			levelStr = "platinum"
		case 3:
			levelStr = "diamond"
		}
		cells = append(cells, &pb.CellAllocation{
			CellId:         cell.CellID,
			CellLevel:      levelStr,
			CostMultiplier: float32(cell.CostMultiplier),
		})
	}

	return &pb.BalanceResponse{
		Success:    true,
		Message:    "ok",
		BalanceUsd: uint64(user.BalanceUSD * 100),
		Quota:      quotaInfo,
		Cells:      cells,
	}, nil
}

// PurchaseQuota 购买流量包
func (s *BillingServiceImpl) PurchaseQuota(ctx context.Context, req *pb.PurchaseRequest) (*pb.PurchaseResponse, error) {
	if req.GetAccountId() == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if req.GetPackageType() == pb.PackageType_PACKAGE_UNKNOWN {
		return nil, status.Error(codes.InvalidArgument, "invalid package_type")
	}

	cellLevel := req.GetCellLevel()
	if cellLevel == "" {
		cellLevel = "standard"
	}

	priceInfo, ok := GetPackagePrice(req.GetPackageType(), cellLevel)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid package_type or cell_level")
	}

	quantity := req.GetQuantity()
	if quantity == 0 {
		quantity = 1
	}

	totalPrice := float64(priceInfo.PriceUSD*uint64(quantity)) / 100.0
	totalQuota := priceInfo.QuotaBytes * int64(quantity)

	var resp *pb.PurchaseResponse

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		// FOR UPDATE 行级排他锁：防止并发购买导致余额双花
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", req.GetAccountId()).
			First(&user).Error; err != nil {
			return status.Error(codes.NotFound, "account not found")
		}

		if user.BalanceUSD < totalPrice {
			return status.Error(codes.FailedPrecondition, "insufficient balance")
		}

		// 扣减余额
		if err := tx.Model(&user).Update("balance_usd", gorm.Expr("balance_usd - ?", totalPrice)).Error; err != nil {
			return status.Error(codes.Internal, "failed to deduct balance")
		}

		// 增加配额
		expiresAt := time.Now().Add(30 * 24 * time.Hour)
		if err := tx.Model(&user).Updates(map[string]any{
			"remaining_quota":  gorm.Expr("remaining_quota + ?", totalQuota),
			"total_quota":      gorm.Expr("total_quota + ?", totalQuota),
			"quota_expires_at": expiresAt,
		}).Error; err != nil {
			return status.Error(codes.Internal, "failed to update quota")
		}

		// 创建购买记录
		purchase := models.QuotaPurchase{
			UserID:      req.GetAccountId(),
			PackageType: req.GetPackageType().String(),
			QuotaBytes:  totalQuota,
			CostUSD:     totalPrice,
			CellLevel:   cellLevelToInt(cellLevel),
			ExpiresAt:   &expiresAt,
		}
		if err := tx.Create(&purchase).Error; err != nil {
			return status.Error(codes.Internal, "failed to create purchase record")
		}

		// 记录计费流水
		billingLog := models.BillingLog{
			UserID:  req.GetAccountId(),
			CostUSD: totalPrice,
			LogType: "purchase",
		}
		if err := tx.Create(&billingLog).Error; err != nil {
			return status.Error(codes.Internal, "failed to create billing log")
		}

		newBalance := user.BalanceUSD - totalPrice
		resp = &pb.PurchaseResponse{
			Success:          true,
			Message:          "purchase successful",
			CostUsd:          priceInfo.PriceUSD * uint64(quantity),
			RemainingBalance: uint64(newBalance * 100),
			QuotaAdded:       uint64(totalQuota),
			ExpiresAt:        expiresAt.Unix(),
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// 事务已 commit → 异步同步 Redis + QuotaManager（最终一致性）
	if s.quotaBridge != nil && resp != nil {
		expiresAt := time.Unix(resp.ExpiresAt, 0)
		go s.quotaBridge.SyncAfterPurchase(ctx, req.GetAccountId(), int64(resp.QuotaAdded), expiresAt)
	}

	return resp, nil
}

// GetBillingLogs 查询计费流水
func (s *BillingServiceImpl) GetBillingLogs(ctx context.Context, req *pb.BillingLogsRequest) (*pb.BillingLogsResponse, error) {
	if req.GetAccountId() == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}

	query := s.db.Model(&models.BillingLog{}).Where("user_id = ?", req.GetAccountId())

	if req.GetStartTime() > 0 {
		query = query.Where("created_at >= ?", time.Unix(req.GetStartTime(), 0))
	}
	if req.GetEndTime() > 0 {
		query = query.Where("created_at <= ?", time.Unix(req.GetEndTime(), 0))
	}

	var totalCount int64
	query.Count(&totalCount)

	limit := int(req.GetLimit())
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var logs []models.BillingLog
	if err := query.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, status.Error(codes.Internal, "failed to query billing logs")
	}

	var pbLogs []*pb.BillingLog
	for _, l := range logs {
		logType := pb.LogType_LOG_UNKNOWN
		switch l.LogType {
		case "traffic":
			logType = pb.LogType_LOG_TRAFFIC
		case "deposit":
			logType = pb.LogType_LOG_DEPOSIT
		case "purchase":
			logType = pb.LogType_LOG_PURCHASE
		case "refund":
			logType = pb.LogType_LOG_REFUND
		}

		pbLogs = append(pbLogs, &pb.BillingLog{
			LogId:               l.LogID,
			GatewayId:           l.GatewayID,
			Timestamp:           l.CreatedAt.Unix(),
			BaseTrafficBytes:    uint64(l.BusinessBytes),
			DefenseTrafficBytes: uint64(l.DefenseBytes),
			CostUsd:             uint64(l.CostUSD * 100),
			CellId:              l.CellID,
			CostMultiplier:      float32(l.CostMultiplier),
			LogType:             logType,
		})
	}

	return &pb.BillingLogsResponse{
		Success:    true,
		Message:    "ok",
		Logs:       pbLogs,
		TotalCount: uint32(totalCount),
	}, nil
}

func cellLevelToInt(level string) int {
	switch level {
	case "platinum":
		return 2
	case "diamond":
		return 3
	default:
		return 1
	}
}

// PurchaseQuotaPure 纯函数版本，用于属性测试
func PurchaseQuotaPure(balanceUSD float64, remainingQuota int64, pkg pb.PackageType, cellLevel string, quantity uint32) (newBalance float64, newQuota int64, err error) {
	priceInfo, ok := GetPackagePrice(pkg, cellLevel)
	if !ok {
		return balanceUSD, remainingQuota, fmt.Errorf("invalid package or level")
	}
	if quantity == 0 {
		quantity = 1
	}
	totalPrice := float64(priceInfo.PriceUSD*uint64(quantity)) / 100.0
	totalQuota := priceInfo.QuotaBytes * int64(quantity)

	if balanceUSD < totalPrice {
		return balanceUSD, remainingQuota, fmt.Errorf("insufficient balance")
	}

	return balanceUSD - totalPrice, remainingQuota + totalQuota, nil
}

// PurchaseTierSubscription 购买等级订阅
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

		// 降级拒绝：目标等级 < 当前等级时返回错误，降级在到期后由定时任务处理
		if newLevel < user.CellLevel {
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

		// 创建购买记录（月费不含流量，quota_bytes=0）
		purchase := models.QuotaPurchase{
			UserID:      req.GetAccountId(),
			PackageType: planType,
			QuotaBytes:  0,
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
