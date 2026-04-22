// Package cellular - 蜂窝管理 gRPC 服务
package cellular

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"time"

	pb "mirage-os/api/proto"
	"mirage-os/pkg/models"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// CellServiceImpl 蜂窝管理服务实现
type CellServiceImpl struct {
	pb.UnimplementedCellServiceServer
	db  *gorm.DB
	rdb *redis.Client
}

// NewCellServiceImpl 创建蜂窝管理服务
func NewCellServiceImpl(db *gorm.DB, rdb *redis.Client) *CellServiceImpl {
	return &CellServiceImpl{db: db, rdb: rdb}
}

// ValidateRegisterCellRequest 验证注册请求（纯函数，用于属性测试）
func ValidateRegisterCellRequest(req *pb.RegisterCellRequest) error {
	if req.GetCellId() == "" {
		return fmt.Errorf("cell_id 不能为空")
	}
	if req.GetLevel() == pb.CellLevel_LEVEL_UNKNOWN {
		return fmt.Errorf("等级不能为 LEVEL_UNKNOWN")
	}
	if req.GetLocation() == nil || req.GetLocation().GetCountry() == "" {
		return fmt.Errorf("location.country 不能为空")
	}
	return nil
}

// RegisterCell 注册蜂窝
func (s *CellServiceImpl) RegisterCell(ctx context.Context, req *pb.RegisterCellRequest) (*pb.RegisterCellResponse, error) {
	if err := ValidateRegisterCellRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	cell := models.Cell{
		CellID:         req.GetCellId(),
		CellName:       req.GetCellName(),
		CellLevel:      int(req.GetLevel()),
		Country:        req.GetLocation().GetCountry(),
		RegionCode:     req.GetLocation().GetRegion(),
		City:           req.GetLocation().GetCity(),
		Latitude:       float64(req.GetLocation().GetLatitude()),
		Longitude:      float64(req.GetLocation().GetLongitude()),
		Jurisdiction:   req.GetLocation().GetJurisdiction(),
		CostMultiplier: float64(req.GetCostMultiplier()),
		MaxGateways:    int(req.GetMaxGateways()),
		Status:         "active",
	}
	if cell.CostMultiplier == 0 {
		cell.CostMultiplier = 1.0
	}
	if cell.MaxGateways == 0 {
		cell.MaxGateways = 100
	}

	if err := s.db.Create(&cell).Error; err != nil {
		if isDuplicateKeyError(err) {
			return nil, status.Errorf(codes.AlreadyExists, "cell_id %s 已存在", req.GetCellId())
		}
		return nil, status.Errorf(codes.Internal, "创建蜂窝失败: %v", err)
	}

	return &pb.RegisterCellResponse{
		Success:      true,
		Message:      "蜂窝注册成功",
		CellId:       req.GetCellId(),
		RegisteredAt: time.Now().Unix(),
	}, nil
}

// CellWithLoad 蜂窝及其负载信息（用于纯函数）
type CellWithLoad struct {
	Cell         models.Cell
	GatewayCount int
	MaxGateways  int
	LoadPercent  float32
}

// FilterCells 筛选蜂窝（纯函数，用于属性测试）
func FilterCells(cells []CellWithLoad, level pb.CellLevel, country string, onlineOnly bool) []CellWithLoad {
	var result []CellWithLoad
	for _, c := range cells {
		if level != pb.CellLevel_LEVEL_UNKNOWN && c.Cell.CellLevel != int(level) {
			continue
		}
		if country != "" && c.Cell.Country != country {
			continue
		}
		if onlineOnly && c.Cell.Status != "active" {
			continue
		}
		result = append(result, c)
	}
	return result
}

// SelectBestCell 选择负载最低的蜂窝（纯函数，用于属性测试）
func SelectBestCell(cells []CellWithLoad, preferredLevel pb.CellLevel, preferredCountry string) (*CellWithLoad, error) {
	filtered := FilterCells(cells, preferredLevel, preferredCountry, true)
	if len(filtered) == 0 {
		return nil, fmt.Errorf("无满足条件的可用蜂窝")
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].LoadPercent < filtered[j].LoadPercent
	})
	return &filtered[0], nil
}

// getTierLoadThresholdLocal 获取等级负载阈值（本地版本，避免跨包依赖）
func getTierLoadThresholdLocal(level int) float32 {
	switch level {
	case 3:
		return 40
	case 2:
		return 60
	default:
		return 80
	}
}

// SelectBestCellForTier 按等级选择最优蜂窝（纯函数，用于属性测试）
// 支持降级分配：如果目标等级无可用蜂窝，降级到低一级
// 返回选中的蜂窝和实际分配的等级
func SelectBestCellForTier(cells []CellWithLoad, userLevel int) (*CellWithLoad, int, error) {
	for level := userLevel; level >= 1; level-- {
		threshold := getTierLoadThresholdLocal(level)
		var candidates []CellWithLoad
		for _, c := range cells {
			if c.Cell.CellLevel == level && c.Cell.Status == "active" && c.LoadPercent < threshold {
				candidates = append(candidates, c)
			}
		}
		if len(candidates) > 0 {
			sort.Slice(candidates, func(i, j int) bool {
				return candidates[i].LoadPercent < candidates[j].LoadPercent
			})
			return &candidates[0], level, nil
		}
	}
	return nil, 0, fmt.Errorf("no available cells for user level %d", userLevel)
}

// SelectSwitchTarget 选择切换目标蜂窝（纯函数，用于属性测试）
func SelectSwitchTarget(cells []CellWithLoad, currentCellID string, currentLevel int, currentJurisdiction string) (*CellWithLoad, error) {
	var candidates []CellWithLoad
	for _, c := range cells {
		if c.Cell.CellID == currentCellID {
			continue
		}
		if c.Cell.CellLevel != currentLevel {
			continue
		}
		if c.Cell.Jurisdiction == currentJurisdiction {
			continue
		}
		if c.Cell.Status != "active" {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("无满足条件的目标蜂窝")
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LoadPercent < candidates[j].LoadPercent
	})
	return &candidates[0], nil
}

// ListCells 查询蜂窝列表
func (s *CellServiceImpl) ListCells(ctx context.Context, req *pb.ListCellsRequest) (*pb.ListCellsResponse, error) {
	query := s.db.Model(&models.Cell{})

	if req.GetLevel() != pb.CellLevel_LEVEL_UNKNOWN {
		query = query.Where("cell_level = ?", int(req.GetLevel()))
	}
	if req.GetCountry() != "" {
		query = query.Where("country = ?", req.GetCountry())
	}
	if req.GetOnlineOnly() {
		query = query.Where("status = ?", "active")
	}

	var cells []models.Cell
	if err := query.Find(&cells).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "查询蜂窝失败: %v", err)
	}

	var cellInfos []*pb.CellInfo
	for _, c := range cells {
		var gwCount int64
		s.db.Model(&models.Gateway{}).Where("cell_id = ?", c.CellID).Count(&gwCount)

		loadPercent := float32(0)
		if c.MaxGateways > 0 {
			loadPercent = float32(gwCount) / float32(c.MaxGateways) * 100
		}

		cellInfos = append(cellInfos, &pb.CellInfo{
			CellId:   c.CellID,
			CellName: c.CellName,
			Level:    pb.CellLevel(c.CellLevel),
			Location: &pb.GeoLocation{
				Country:      c.Country,
				Region:       c.RegionCode,
				City:         c.City,
				Latitude:     float32(c.Latitude),
				Longitude:    float32(c.Longitude),
				Jurisdiction: c.Jurisdiction,
			},
			Status:         &pb.CellStatus{Online: c.Status == "active"},
			GatewayCount:   uint32(gwCount),
			MaxGateways:    uint32(c.MaxGateways),
			CostMultiplier: float32(c.CostMultiplier),
			LoadPercent:    loadPercent,
		})
	}

	return &pb.ListCellsResponse{
		Success: true,
		Message: "查询成功",
		Cells:   cellInfos,
	}, nil
}

// AllocateGateway 分配 Gateway 到蜂窝
func (s *CellServiceImpl) AllocateGateway(ctx context.Context, req *pb.AllocateRequest) (*pb.AllocateResponse, error) {
	query := s.db.Model(&models.Cell{}).Where("status = ?", "active")
	if req.GetPreferredLevel() != pb.CellLevel_LEVEL_UNKNOWN {
		query = query.Where("cell_level = ?", int(req.GetPreferredLevel()))
	}
	if req.GetPreferredCountry() != "" {
		query = query.Where("country = ?", req.GetPreferredCountry())
	}

	var cells []models.Cell
	if err := query.Find(&cells).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "查询蜂窝失败: %v", err)
	}
	if len(cells) == 0 {
		return nil, status.Errorf(codes.NotFound, "无满足条件的可用蜂窝")
	}

	// 计算负载并排序
	type cellLoad struct {
		cell    models.Cell
		load    float32
		gwCount int64
	}
	var loads []cellLoad
	for _, c := range cells {
		var gwCount int64
		s.db.Model(&models.Gateway{}).Where("cell_id = ?", c.CellID).Count(&gwCount)
		load := float32(0)
		if c.MaxGateways > 0 {
			load = float32(gwCount) / float32(c.MaxGateways) * 100
		}
		loads = append(loads, cellLoad{cell: c, load: load, gwCount: gwCount})
	}
	sort.Slice(loads, func(i, j int) bool { return loads[i].load < loads[j].load })

	best := loads[0]

	// 生成 32 字节 hex 连接令牌
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, status.Errorf(codes.Internal, "生成令牌失败: %v", err)
	}
	token := hex.EncodeToString(tokenBytes)

	return &pb.AllocateResponse{
		Success:         true,
		Message:         "分配成功",
		CellId:          best.cell.CellID,
		ConnectionToken: token,
		AllocatedAt:     time.Now().Unix(),
		CellInfo: &pb.CellInfo{
			CellId:   best.cell.CellID,
			CellName: best.cell.CellName,
			Level:    pb.CellLevel(best.cell.CellLevel),
			Location: &pb.GeoLocation{
				Country:      best.cell.Country,
				Region:       best.cell.RegionCode,
				Jurisdiction: best.cell.Jurisdiction,
			},
			GatewayCount:   uint32(best.gwCount),
			MaxGateways:    uint32(best.cell.MaxGateways),
			CostMultiplier: float32(best.cell.CostMultiplier),
			LoadPercent:    best.load,
		},
	}, nil
}

// HealthCheck 蜂窝健康检查
func (s *CellServiceImpl) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	query := s.db.Model(&models.Cell{})
	if req.GetCellId() != "" {
		query = query.Where("cell_id = ?", req.GetCellId())
	}

	var cells []models.Cell
	if err := query.Find(&cells).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "查询蜂窝失败: %v", err)
	}

	var healthList []*pb.CellHealth
	for _, c := range cells {
		var gwCount, failedCount int64
		s.db.Model(&models.Gateway{}).Where("cell_id = ?", c.CellID).Count(&gwCount)
		s.db.Model(&models.Gateway{}).Where("cell_id = ? AND is_online = ?", c.CellID, false).Count(&failedCount)

		var avgLatency float64
		s.db.Model(&models.Gateway{}).Where("cell_id = ? AND is_online = ?", c.CellID, true).
			Select("COALESCE(AVG(baseline_rtt), 0)").Scan(&avgLatency)

		var threatCount int64
		s.db.Model(&models.ThreatIntel{}).Where("gateway_id IN (SELECT gateway_id FROM gateways WHERE cell_id = ?) AND last_seen > ?",
			c.CellID, time.Now().Add(-24*time.Hour)).Count(&threatCount)

		healthList = append(healthList, &pb.CellHealth{
			CellId:          c.CellID,
			Healthy:         failedCount == 0 && gwCount > 0,
			GatewayCount:    uint32(gwCount),
			FailedGateways:  uint32(failedCount),
			AvgLatencyMs:    float32(avgLatency) / 1000.0, // μs → ms
			ThreatCount_24H: uint32(threatCount),
			LastCheck:       time.Now().Unix(),
		})
	}

	return &pb.HealthCheckResponse{
		Success:      true,
		Message:      "健康检查完成",
		HealthStatus: healthList,
	}, nil
}

// SwitchCell 切换蜂窝
func (s *CellServiceImpl) SwitchCell(ctx context.Context, req *pb.SwitchCellRequest) (*pb.SwitchCellResponse, error) {
	var targetCellID string

	if req.GetTargetCellId() != "" {
		targetCellID = req.GetTargetCellId()
	} else {
		// 自动选择：同等级、不同管辖区、负载最低
		var currentCell models.Cell
		if err := s.db.Where("cell_id = ?", req.GetCurrentCellId()).First(&currentCell).Error; err != nil {
			return nil, status.Errorf(codes.NotFound, "当前蜂窝不存在: %v", err)
		}

		var candidates []models.Cell
		s.db.Where("cell_level = ? AND jurisdiction != ? AND status = ? AND cell_id != ?",
			currentCell.CellLevel, currentCell.Jurisdiction, "active", currentCell.CellID).
			Find(&candidates)

		if len(candidates) == 0 {
			return nil, status.Errorf(codes.NotFound, "无满足条件的目标蜂窝")
		}

		// 按负载排序
		type cellLoad struct {
			cell models.Cell
			load float32
		}
		var loads []cellLoad
		for _, c := range candidates {
			var gwCount int64
			s.db.Model(&models.Gateway{}).Where("cell_id = ?", c.CellID).Count(&gwCount)
			load := float32(0)
			if c.MaxGateways > 0 {
				load = float32(gwCount) / float32(c.MaxGateways) * 100
			}
			loads = append(loads, cellLoad{cell: c, load: load})
		}
		sort.Slice(loads, func(i, j int) bool { return loads[i].load < loads[j].load })
		targetCellID = loads[0].cell.CellID
	}

	var targetCell models.Cell
	if err := s.db.Where("cell_id = ?", targetCellID).First(&targetCell).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "目标蜂窝不存在")
	}

	// 更新 Gateway 的 cell_id
	if req.GetGatewayId() != "" {
		s.db.Model(&models.Gateway{}).Where("gateway_id = ?", req.GetGatewayId()).
			Update("cell_id", targetCellID)
	}

	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	log.Printf("[CellService] 蜂窝切换: %s → %s (原因: %s)", req.GetCurrentCellId(), targetCellID, req.GetReason().String())

	return &pb.SwitchCellResponse{
		Success:         true,
		Message:         "蜂窝切换成功",
		NewCellId:       targetCellID,
		ConnectionToken: token,
		SwitchedAt:      time.Now().Unix(),
		NewCellInfo: &pb.CellInfo{
			CellId:   targetCell.CellID,
			CellName: targetCell.CellName,
			Level:    pb.CellLevel(targetCell.CellLevel),
			Location: &pb.GeoLocation{
				Country:      targetCell.Country,
				Jurisdiction: targetCell.Jurisdiction,
			},
		},
	}, nil
}

// isDuplicateKeyError 检查是否为重复键错误
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "duplicate key") || contains(errStr, "UNIQUE constraint") || contains(errStr, "Duplicate entry")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
