// Package main - Gateway 心跳服务实现
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	pb "mirage-os/api/proto"
	"mirage-os/pkg/geo"
	"mirage-os/pkg/models"
	"mirage-os/pkg/redact"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// Server gRPC 服务器
type Server struct {
	pb.UnimplementedGatewayServiceServer
	DB                 *gorm.DB
	RedisClient        *redis.Client // Redis 客户端（用于实时推送）
	Locator            *geo.Locator  // GeoIP 定位器（全球视野坐标对齐）
	BaseTrafficRate    float64       // 业务流量费率（$/GB）
	DefenseTrafficRate float64       // 防御流量费率（$/GB）
	ThreatThreshold    int64         // 威胁阈值

	// 异步计费通道
	billingChan chan *billingTask
}

// billingTask 计费任务（异步处理）
type billingTask struct {
	GatewayID      string
	UserID         string
	CellID         string
	BusinessBytes  int64
	DefenseBytes   int64
	CostMultiplier float64
}

// NewServer 创建服务器实例
func NewServer(db *gorm.DB, rdb *redis.Client, locator *geo.Locator) *Server {
	s := &Server{
		DB:                 db,
		RedisClient:        rdb,
		Locator:            locator,
		BaseTrafficRate:    0.10, // $0.10/GB
		DefenseTrafficRate: 0.05, // $0.05/GB
		ThreatThreshold:    100,  // 100 次命中触发全局封禁
		billingChan:        make(chan *billingTask, 1000),
	}

	// 启动异步计费处理器
	go s.billingWorker()

	return s
}

// billingWorker 异步计费处理器（解耦心跳事务）
func (s *Server) billingWorker() {
	for task := range s.billingChan {
		s.processBillingTask(task)
	}
}

// processBillingTask 处理单个计费任务
func (s *Server) processBillingTask(task *billingTask) {
	if task.UserID == "" {
		return
	}

	totalBytes := task.BusinessBytes + task.DefenseBytes
	if totalBytes == 0 {
		return
	}

	// 计算费用
	businessCost := float64(task.BusinessBytes) / (1024 * 1024 * 1024) * s.BaseTrafficRate
	defenseCost := float64(task.DefenseBytes) / (1024 * 1024 * 1024) * s.DefenseTrafficRate
	totalCost := (businessCost + defenseCost) * task.CostMultiplier

	// 异步事务：配额扣减 + 流水记录
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// 扣减配额
		if err := tx.Model(&models.User{}).
			Where("user_id = ?", task.UserID).
			Update("remaining_quota", gorm.Expr("remaining_quota - ?", totalBytes)).Error; err != nil {
			return err
		}

		// 记录流水
		billingLog := models.BillingLog{
			GatewayID:      task.GatewayID,
			UserID:         task.UserID,
			CellID:         task.CellID,
			BusinessBytes:  task.BusinessBytes,
			DefenseBytes:   task.DefenseBytes,
			TotalBytes:     totalBytes,
			CostUSD:        totalCost,
			CostMultiplier: task.CostMultiplier,
			LogType:        "traffic",
		}
		return tx.Create(&billingLog).Error
	})

	if err != nil {
		log.Printf("⚠️ [异步计费] 处理失败: %v", err)
	}
}

// SyncHeartbeat 心跳同步（生死裁决核心）- 事务拆分版
func (s *Server) SyncHeartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	log.Printf("📡 [心跳] Gateway=%s, 威胁等级=%d", req.GatewayId, req.CurrentThreatLevel)

	resp := &pb.HeartbeatResponse{
		Success:               true,
		Message:               "心跳同步成功",
		NextHeartbeatInterval: 30, // 30 秒
	}

	// ============================================
	// 1. 轻量事务：仅更新 Gateway 状态（快速返回）
	// ============================================
	now := time.Now()
	gateway := models.Gateway{
		GatewayID:          req.GatewayId,
		IsOnline:           true,
		Version:            req.Version,
		CurrentThreatLevel: int(req.CurrentThreatLevel),
		LastHeartbeatAt:    &now,
	}

	if req.Status != nil {
		gateway.ActiveConnections = int(req.Status.ActiveConnections)
		gateway.CellID = req.Status.CellId
	}

	if req.Resource != nil {
		gateway.CPUPercent = float64(req.Resource.CpuPercent)
		gateway.MemoryBytes = int64(req.Resource.MemoryBytes)
		gateway.BandwidthBps = int64(req.Resource.BandwidthBps)
	}

	// 更新或创建 Gateway 记录
	result := s.DB.Where("gateway_id = ?", req.GatewayId).
		Assign(gateway).
		FirstOrCreate(&gateway)

	if result.Error != nil {
		log.Printf("❌ [心跳] 更新 Gateway 状态失败: %v", result.Error)
		return &pb.HeartbeatResponse{
			Success: false,
			Message: fmt.Sprintf("心跳处理失败: %v", result.Error),
		}, nil
	}

	// Redis 缓存在线状态（TTL 60s）
	if s.RedisClient != nil {
		s.RedisClient.Set(ctx, fmt.Sprintf("gateway:%s:status", req.GatewayId), "ONLINE", 60*time.Second)
		// 缓存 Gateway 地址用于 Downlink 连接
		if req.GatewayId != "" {
			s.RedisClient.Set(ctx, fmt.Sprintf("gateway:%s:addr", req.GatewayId), req.GatewayId, 0)
		}
	}

	// ============================================
	// 2. 从 Redis 读取实时配额（避免数据库查询）
	// ============================================
	var remainingQuota int64 = -1 // -1 表示无限
	if gateway.UserID != "" && s.RedisClient != nil {
		quotaKey := fmt.Sprintf("mirage:quota:%s", gateway.UserID)
		if val, err := s.RedisClient.Get(ctx, quotaKey).Int64(); err == nil {
			remainingQuota = val
		} else {
			// Redis 无缓存，从数据库读取并缓存
			var user models.User
			if err := s.DB.Select("remaining_quota").Where("user_id = ?", gateway.UserID).First(&user).Error; err == nil {
				remainingQuota = user.RemainingQuota
				s.RedisClient.Set(ctx, quotaKey, remainingQuota, 5*time.Minute)
			}
		}
	}

	// ============================================
	// 3. 熔断决策（基于缓存配额）
	// ============================================
	if remainingQuota == 0 {
		resp.RemainingQuota = 0
		resp.Message = "配额已耗尽，服务已熔断"
		log.Printf("🚨 [熔断] Gateway=%s, 用户=%s, 配额耗尽", req.GatewayId, redact.Token(gateway.UserID))
		resp.DefenseConfig = &pb.DefenseConfig{
			DefenseLevel:   5,
			JitterMeanUs:   100000,
			JitterStddevUs: 30000,
			NoiseIntensity: 30,
			PaddingRate:    30,
		}
	} else if remainingQuota > 0 {
		resp.RemainingQuota = uint64(remainingQuota)
		defenseLevel := min(req.CurrentThreatLevel, 5)
		resp.DefenseConfig = s.getDefenseConfig(defenseLevel)
	} else {
		// 无限配额模式
		resp.RemainingQuota = ^uint64(0)
		resp.DefenseConfig = s.getDefenseConfig(req.CurrentThreatLevel)
	}

	// ============================================
	// 4. Desired State 对齐（幂等状态机）
	// ============================================
	if s.RedisClient != nil {
		desiredStateKey := fmt.Sprintf("gateway:%s:desired_state", req.GatewayId)
		stateHashKey := fmt.Sprintf("gateway:%s:state_hash", req.GatewayId)

		// 读取 OS 侧期望状态的 Hash
		desiredHash, _ := s.RedisClient.Get(ctx, stateHashKey).Result()

		// 如果 Gateway 上报的 CurrentStateHash 与期望不一致，下发全量 Desired State
		// Gateway 通过 HeartbeatRequest 的 version 字段携带 state hash（复用）
		if desiredHash != "" && desiredHash != req.Version {
			// 读取全量 Desired State
			stateJSON, err := s.RedisClient.Get(ctx, desiredStateKey).Result()
			if err == nil && stateJSON != "" {
				var desiredState map[string]interface{}
				if json.Unmarshal([]byte(stateJSON), &desiredState) == nil {
					// 从 Desired State 中提取防御配置覆盖响应
					if dl, ok := desiredState["defense_level"].(float64); ok {
						resp.DefenseConfig = s.getDefenseConfig(uint32(dl))
					}
					log.Printf("🔄 [状态对齐] Gateway=%s, 下发全量 Desired State", req.GatewayId)
				}
			}
		}

		// 处理一次性事件队列（转生指令等）
		eventsKey := fmt.Sprintf("mirage:downlink:events:%s", req.GatewayId)
		for {
			eventJSON, err := s.RedisClient.LPop(ctx, eventsKey).Result()
			if err != nil {
				break
			}
			// 通过 Redis Pub/Sub 推送给 Gateway 的 WebSocket 连接
			s.RedisClient.Publish(ctx, fmt.Sprintf("mirage:gateway:%s:commands", req.GatewayId), eventJSON)
		}
	}

	// ============================================
	// 5. 异步推送实时数据（不阻塞响应）
	// ============================================
	go func() {
		if s.Locator != nil && s.RedisClient != nil {
			gatewayIP := req.GatewayId
			lat, lng, country, city := s.Locator.Resolve(gatewayIP)

			event := map[string]any{
				"type":      "heartbeat",
				"timestamp": time.Now().Unix(),
				"data": map[string]any{
					"gwId":        req.GatewayId,
					"lat":         lat,
					"lng":         lng,
					"country":     country,
					"city":        city,
					"location":    fmt.Sprintf("%s, %s", city, country),
					"threatLevel": req.CurrentThreatLevel,
					"status":      "online",
					"cellId":      gateway.CellID,
				},
			}

			eventJSON, _ := json.Marshal(event)
			s.RedisClient.Publish(ctx, "mirage:events:all", eventJSON)

			if gateway.UserID != "" {
				userChannel := fmt.Sprintf("mirage:user:%s:events", gateway.UserID)
				s.RedisClient.Publish(ctx, userChannel, eventJSON)
			}
		}
	}()

	return resp, nil
}

// ReportTraffic 流量上报（10 秒一次）
func (s *Server) ReportTraffic(ctx context.Context, req *pb.TrafficReport) (*pb.TrafficResponse, error) {
	log.Printf("📊 [流量] Gateway=%s, 业务=%d字节, 防御=%d字节",
		req.GatewayId, req.BaseTrafficBytes, req.DefenseTrafficBytes)

	resp := &pb.TrafficResponse{
		Success: true,
		Message: "流量上报成功",
	}

	// 开启事务
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// 查询 Gateway
		var gateway models.Gateway
		if err := tx.Where("gateway_id = ?", req.GatewayId).First(&gateway).Error; err != nil {
			return fmt.Errorf("查询 Gateway 失败: %w", err)
		}

		// 查询蜂窝（获取成本倍率）
		var cell models.Cell
		if err := tx.Where("cell_id = ?", gateway.CellID).First(&cell).Error; err != nil {
			log.Printf("⚠️  未找到蜂窝 %s，使用默认倍率 1.0", gateway.CellID)
			cell.CostMultiplier = 1.0
		}

		// 计算费用
		businessCost := float64(req.BaseTrafficBytes) / (1024 * 1024 * 1024) * s.BaseTrafficRate
		defenseCost := float64(req.DefenseTrafficBytes) / (1024 * 1024 * 1024) * s.DefenseTrafficRate
		totalCost := (businessCost + defenseCost) * cell.CostMultiplier

		resp.CurrentCostUsd = float32(totalCost)

		// 如果有用户绑定，扣减配额
		if gateway.UserID != "" {
			var user models.User
			if err := tx.Where("user_id = ?", gateway.UserID).First(&user).Error; err != nil {
				return fmt.Errorf("查询用户失败: %w", err)
			}

			// 原子扣减配额
			totalBytes := int64(req.BaseTrafficBytes + req.DefenseTrafficBytes)
			newQuota := user.RemainingQuota - totalBytes

			if err := tx.Model(&user).Update("remaining_quota", newQuota).Error; err != nil {
				return fmt.Errorf("扣减配额失败: %w", err)
			}

			resp.RemainingQuota = uint64(newQuota)

			// 配额告警
			if newQuota <= 0 {
				resp.QuotaWarning = true
				resp.Message = "配额已耗尽"
			} else if float64(newQuota)/float64(user.TotalQuota) < 0.1 {
				resp.QuotaWarning = true
				resp.Message = "配额不足 10%"
			}

			// 记录计费流水
			billingLog := models.BillingLog{
				GatewayID:      req.GatewayId,
				UserID:         gateway.UserID,
				CellID:         gateway.CellID,
				BusinessBytes:  int64(req.BaseTrafficBytes),
				DefenseBytes:   int64(req.DefenseTrafficBytes),
				TotalBytes:     totalBytes,
				CostUSD:        totalCost,
				CostMultiplier: cell.CostMultiplier,
				LogType:        "traffic",
			}

			if err := tx.Create(&billingLog).Error; err != nil {
				return fmt.Errorf("记录计费流水失败: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		log.Printf("❌ [流量] 处理失败: %v", err)
		return &pb.TrafficResponse{
			Success: false,
			Message: fmt.Sprintf("流量上报失败: %v", err),
		}, nil
	}

	// 推送流量数据到 Redis
	if s.RedisClient != nil {
		event := map[string]interface{}{
			"type":      "traffic",
			"timestamp": time.Now().Unix(),
			"data": map[string]interface{}{
				"gwId":          req.GatewayId,
				"businessBytes": req.BaseTrafficBytes,
				"defenseBytes":  req.DefenseTrafficBytes,
				"totalBytes":    req.BaseTrafficBytes + req.DefenseTrafficBytes,
				"costUsd":       resp.CurrentCostUsd,
				"quotaWarning":  resp.QuotaWarning,
			},
		}
		eventJSON, _ := json.Marshal(event)
		s.RedisClient.Publish(ctx, "mirage:events:all", eventJSON)
	}

	return resp, nil
}

// ReportThreat 威胁上报（实时）
func (s *Server) ReportThreat(ctx context.Context, req *pb.ThreatReport) (*pb.ThreatResponse, error) {
	log.Printf("🚨 [威胁] Gateway=%s, 类型=%s, 源IP=%s, 严重程度=%d",
		req.GatewayId, req.ThreatType.String(), redact.IP(req.SourceIp), req.Severity)

	resp := &pb.ThreatResponse{
		Success: true,
		Message: "威胁上报成功",
		Action:  pb.ThreatAction_ACTION_NONE,
	}

	// 查询或创建威胁情报记录
	var threat models.ThreatIntel
	result := s.DB.Where("src_ip = ? AND threat_type = ?", req.SourceIp, int(req.ThreatType)).
		First(&threat)

	if result.Error == gorm.ErrRecordNotFound {
		// 新威胁
		threat = models.ThreatIntel{
			GatewayID:      req.GatewayId,
			SrcIP:          req.SourceIp,
			SrcPort:        int(req.SourcePort),
			ThreatType:     int(req.ThreatType),
			Severity:       int(req.Severity),
			JA4Fingerprint: string(req.Ja4Fingerprint),
			PacketCount:    int(req.PacketCount),
			HitCount:       1,
		}
		s.DB.Create(&threat)
	} else {
		// 已存在，累加命中次数
		s.DB.Model(&threat).Updates(map[string]interface{}{
			"hit_count":    gorm.Expr("hit_count + ?", 1),
			"packet_count": gorm.Expr("packet_count + ?", req.PacketCount),
			"last_seen":    time.Now(),
		})
		threat.HitCount++ // 本地更新用于判断
	}

	// ============================================
	// 全局威胁决策（severity 分级映射）
	// ============================================
	if threat.HitCount >= s.ThreatThreshold {
		resp.Action = pb.ThreatAction_ACTION_BLOCK_IP
		resp.NewDefenseLevel = 5
		resp.Message = fmt.Sprintf("IP %s 已触发全局封禁（命中 %d 次）", redact.IP(req.SourceIp), threat.HitCount)
		log.Printf("🚫 [全局封禁] IP=%s, 命中次数=%d", redact.IP(req.SourceIp), threat.HitCount)
	} else if req.Severity >= 8 {
		resp.Action = pb.ThreatAction_ACTION_EMERGENCY_SHUTDOWN
		resp.NewDefenseLevel = 5
		resp.Message = "检测到致命威胁，建议紧急关闭"
	} else if req.Severity >= 6 {
		resp.Action = pb.ThreatAction_ACTION_SWITCH_CELL
		resp.NewDefenseLevel = 4
		resp.Message = "检测到高危威胁，建议切换蜂窝"
	} else if req.Severity >= 4 {
		resp.Action = pb.ThreatAction_ACTION_BLOCK_IP
		resp.NewDefenseLevel = 3
		resp.Message = "检测到中危威胁，封禁源 IP"
	} else if req.Severity >= 2 {
		resp.Action = pb.ThreatAction_ACTION_INCREASE_DEFENSE
		resp.NewDefenseLevel = 2
		resp.Message = "检测到低危威胁，提升防御等级"
	}

	// ============================================
	// 实时推送到 WebSocket（通过 Redis）
	// ============================================
	if s.RedisClient != nil {
		s.publishThreatEvent(req, &threat)
	}

	return resp, nil
}

// publishThreatEvent 发布威胁事件
func (s *Server) publishThreatEvent(req *pb.ThreatReport, threat *models.ThreatIntel) {
	// GeoIP 定位威胁源
	lat, lng, country, city := 0.0, 0.0, "Unknown", "Unknown"
	if s.Locator != nil {
		lat, lng, country, city = s.Locator.Resolve(req.SourceIp)
	}

	event := map[string]interface{}{
		"type":      "threat",
		"timestamp": time.Now().Unix(),
		"data": map[string]interface{}{
			"srcIp":      req.SourceIp,
			"threatType": req.ThreatType.String(),
			"severity":   req.Severity,
			"hitCount":   threat.HitCount,
			"lat":        lat,
			"lng":        lng,
			"country":    country,
			"city":       city,
			"label":      fmt.Sprintf("%s-%s", city, req.ThreatType.String()),
			"intensity":  int(req.Severity),
		},
	}

	// 发布到 Redis 频道
	if s.RedisClient != nil {
		eventJSON, _ := json.Marshal(event)
		// 推送到全局频道
		s.RedisClient.Publish(context.Background(), "mirage:events:all", eventJSON)
	}
}

// GetQuota 配额查询
func (s *Server) GetQuota(ctx context.Context, req *pb.QuotaRequest) (*pb.QuotaResponse, error) {
	var user models.User
	if err := s.DB.Where("user_id = ?", req.UserId).First(&user).Error; err != nil {
		return &pb.QuotaResponse{
			Success: false,
			Message: fmt.Sprintf("查询用户失败: %v", err),
		}, nil
	}

	resp := &pb.QuotaResponse{
		Success:        true,
		Message:        "查询成功",
		RemainingBytes: uint64(user.RemainingQuota),
		TotalBytes:     uint64(user.TotalQuota),
		AutoRenew:      user.AutoRenew,
	}

	if user.QuotaExpiresAt != nil {
		resp.ExpiresAt = user.QuotaExpiresAt.Unix()
	}

	return resp, nil
}

// getDefenseConfig 根据威胁等级获取防御配置
func (s *Server) getDefenseConfig(level uint32) *pb.DefenseConfig {
	configs := map[uint32]*pb.DefenseConfig{
		0: {DefenseLevel: 0, JitterMeanUs: 10000, JitterStddevUs: 3000, NoiseIntensity: 5, PaddingRate: 5},
		1: {DefenseLevel: 1, JitterMeanUs: 20000, JitterStddevUs: 6000, NoiseIntensity: 10, PaddingRate: 10},
		2: {DefenseLevel: 2, JitterMeanUs: 30000, JitterStddevUs: 9000, NoiseIntensity: 15, PaddingRate: 15},
		3: {DefenseLevel: 3, JitterMeanUs: 50000, JitterStddevUs: 15000, NoiseIntensity: 20, PaddingRate: 20},
		4: {DefenseLevel: 4, JitterMeanUs: 80000, JitterStddevUs: 24000, NoiseIntensity: 25, PaddingRate: 25},
		5: {DefenseLevel: 5, JitterMeanUs: 100000, JitterStddevUs: 30000, NoiseIntensity: 30, PaddingRate: 30},
	}

	if cfg, ok := configs[level]; ok {
		return cfg
	}
	return configs[0]
}

// getGlobalBlacklist 获取全局黑名单（Top 100 威胁 IP）
func (s *Server) getGlobalBlacklist() []string {
	var threats []models.ThreatIntel

	// 查询最近 1 小时内活跃度最高的 Top 100 IP
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	s.DB.Where("last_seen > ?", oneHourAgo).
		Order("hit_count DESC").
		Limit(100).
		Find(&threats)

	blacklist := make([]string, 0, len(threats))
	for _, threat := range threats {
		blacklist = append(blacklist, threat.SrcIP)
	}

	log.Printf("🛡️  [全局黑名单] 已加载 %d 个威胁 IP", len(blacklist))
	return blacklist
}
