// Package health - App 上下文感知战损评分
// 将底层指标与业务逻辑耦合，提升"精准干扰"的判定精度
package health

import (
	"log"
	"sync"
	"time"
)

// DamageType 战损类型
type DamageType int

const (
	DamageNone           DamageType = 0
	DamageLatency        DamageType = 1 // 延迟攻击
	DamagePacketLoss     DamageType = 2 // 丢包攻击
	DamageProtocolBlock  DamageType = 3 // 协议封锁
	DamageSNIBlock       DamageType = 4 // SNI 封锁
	DamageQUICBlock      DamageType = 5 // QUIC 封锁
	DamageWebSocketBlock DamageType = 6 // WebSocket 封锁
	DamageFullBlock      DamageType = 7 // 全域封锁
)

// ContextAwareScore 上下文感知评分
type ContextAwareScore struct {
	AppName       string     `json:"app_name"`
	Category      AppCategory `json:"category"`
	BaseScore     float64    `json:"base_score"`      // 基础分数 (0-1)
	WeightedScore float64    `json:"weighted_score"`  // 加权分数
	DamageType    DamageType `json:"damage_type"`
	Confidence    float64    `json:"confidence"`      // 置信度
	Timestamp     time.Time  `json:"timestamp"`
}

// WeightMatrix 权重矩阵
type WeightMatrix struct {
	// 协议权重
	QUICWeight      float64 `json:"quic_weight"`
	WebSocketWeight float64 `json:"websocket_weight"`
	UDPWeight       float64 `json:"udp_weight"`
	TCPWeight       float64 `json:"tcp_weight"`

	// 指标权重
	RTTWeight       float64 `json:"rtt_weight"`
	JitterWeight    float64 `json:"jitter_weight"`
	PacketLossWeight float64 `json:"packet_loss_weight"`
	ResetWeight     float64 `json:"reset_weight"`
}

// CategoryWeights 类别权重预设
var CategoryWeights = map[AppCategory]*WeightMatrix{
	CategoryMessaging: {
		QUICWeight:       1.0,
		WebSocketWeight:  1.2,
		UDPWeight:        0.8,
		TCPWeight:        1.0,
		RTTWeight:        1.0,
		JitterWeight:     0.8,
		PacketLossWeight: 1.0,
		ResetWeight:      1.5,
	},
	CategoryVideo: {
		QUICWeight:       1.5, // 视频依赖 QUIC
		WebSocketWeight:  0.5,
		UDPWeight:        1.2,
		TCPWeight:        0.8,
		RTTWeight:        0.6,
		JitterWeight:     1.2, // 视频对抖动敏感
		PacketLossWeight: 1.5,
		ResetWeight:      1.0,
	},
	CategoryVoIP: {
		QUICWeight:       0.8,
		WebSocketWeight:  1.5, // Discord 等依赖 WebSocket
		UDPWeight:        1.8, // VoIP 依赖 UDP
		TCPWeight:        0.5,
		RTTWeight:        2.0, // VoIP 对延迟极度敏感
		JitterWeight:     2.0,
		PacketLossWeight: 1.5,
		ResetWeight:      1.0,
	},
	CategorySocial: {
		QUICWeight:       1.2,
		WebSocketWeight:  1.0,
		UDPWeight:        0.6,
		TCPWeight:        1.0,
		RTTWeight:        0.8,
		JitterWeight:     0.6,
		PacketLossWeight: 1.0,
		ResetWeight:      1.2,
	},
	CategoryCloud: {
		QUICWeight:       1.0,
		WebSocketWeight:  0.5,
		UDPWeight:        0.3,
		TCPWeight:        1.2,
		RTTWeight:        0.5,
		JitterWeight:     0.3,
		PacketLossWeight: 1.5, // 云存储对丢包敏感
		ResetWeight:      1.0,
	},
}

// DynamicThreshold 动态阈值
type DynamicThreshold struct {
	YellowRTT      time.Duration
	RedRTT         time.Duration
	YellowJitter   float64
	RedJitter      float64
	YellowLoss     float64
	RedLoss        float64
}

// CategoryThresholds 类别阈值预设
var CategoryThresholds = map[AppCategory]*DynamicThreshold{
	CategoryMessaging: {
		YellowRTT:    200 * time.Millisecond,
		RedRTT:       500 * time.Millisecond,
		YellowJitter: 20,
		RedJitter:    50,
		YellowLoss:   5,
		RedLoss:      15,
	},
	CategoryVideo: {
		YellowRTT:    400 * time.Millisecond,
		RedRTT:       800 * time.Millisecond,
		YellowJitter: 40, // 视频容忍更高抖动
		RedJitter:    80,
		YellowLoss:   8,
		RedLoss:      20,
	},
	CategoryVoIP: {
		YellowRTT:    100 * time.Millisecond, // VoIP 对延迟极度敏感
		RedRTT:       200 * time.Millisecond,
		YellowJitter: 10,
		RedJitter:    30,
		YellowLoss:   3,
		RedLoss:      10,
	},
	CategorySocial: {
		YellowRTT:    300 * time.Millisecond,
		RedRTT:       600 * time.Millisecond,
		YellowJitter: 30,
		RedJitter:    60,
		YellowLoss:   10,
		RedLoss:      25,
	},
	CategoryCloud: {
		YellowRTT:    500 * time.Millisecond,
		RedRTT:       1000 * time.Millisecond,
		YellowJitter: 50,
		RedJitter:    100,
		YellowLoss:   5,
		RedLoss:      15,
	},
}

// ContextAwareEvaluator 上下文感知评估器
type ContextAwareEvaluator struct {
	mu sync.RWMutex

	// 当前拟态模式
	mimicryApp    string
	mimicryWeight *WeightMatrix
	threshold     *DynamicThreshold

	// 评分历史
	scoreHistory  map[string][]*ContextAwareScore
	historySize   int

	// 当前评分
	currentScores map[string]*ContextAwareScore
}

// NewContextAwareEvaluator 创建评估器
func NewContextAwareEvaluator() *ContextAwareEvaluator {
	return &ContextAwareEvaluator{
		mimicryWeight: CategoryWeights[CategoryMessaging], // 默认
		threshold:     CategoryThresholds[CategoryMessaging],
		scoreHistory:  make(map[string][]*ContextAwareScore),
		historySize:   100,
		currentScores: make(map[string]*ContextAwareScore),
	}
}

// SetMimicryMode 设置拟态模式
func (cae *ContextAwareEvaluator) SetMimicryMode(appName string) {
	cae.mu.Lock()
	defer cae.mu.Unlock()

	profile := GetAppProfile(appName)
	if profile == nil {
		return
	}

	cae.mimicryApp = appName
	cae.mimicryWeight = CategoryWeights[profile.Category]
	cae.threshold = CategoryThresholds[profile.Category]

	log.Printf("🎭 拟态模式切换: %s (类别: %d)", appName, profile.Category)
}

// Evaluate 评估链路质量
func (cae *ContextAwareEvaluator) Evaluate(
	quality *LinkQuality,
	probeResult *ProbeResult,
) *ContextAwareScore {
	cae.mu.Lock()
	defer cae.mu.Unlock()

	appName := "unknown"
	category := CategoryMessaging
	if probeResult != nil {
		appName = probeResult.AppName
		if profile := GetAppProfile(appName); profile != nil {
			category = profile.Category
		}
	}

	score := &ContextAwareScore{
		AppName:   appName,
		Category:  category,
		Timestamp: time.Now(),
	}

	// 计算基础分数
	score.BaseScore = cae.calculateBaseScore(quality, probeResult)

	// 应用权重
	score.WeightedScore = cae.applyWeights(score.BaseScore, quality, probeResult)

	// 判定战损类型
	score.DamageType = cae.classifyDamage(quality, probeResult)

	// 计算置信度
	score.Confidence = cae.calculateConfidence(quality, probeResult)

	// 记录历史
	cae.recordScore(appName, score)
	cae.currentScores[appName] = score

	return score
}

// calculateBaseScore 计算基础分数
func (cae *ContextAwareEvaluator) calculateBaseScore(
	quality *LinkQuality,
	probeResult *ProbeResult,
) float64 {
	var score float64

	// 丢包率贡献
	if quality != nil {
		if quality.LossRate > cae.threshold.RedLoss {
			score += 0.4
		} else if quality.LossRate > cae.threshold.YellowLoss {
			score += 0.2
		}

		// RTT 贡献
		rttMs := quality.RTTMean
		if rttMs > float64(cae.threshold.RedRTT.Milliseconds()) {
			score += 0.3
		} else if rttMs > float64(cae.threshold.YellowRTT.Milliseconds()) {
			score += 0.15
		}

		// 抖动贡献
		if quality.JitterEntropy > cae.threshold.RedJitter {
			score += 0.2
		} else if quality.JitterEntropy > cae.threshold.YellowJitter {
			score += 0.1
		}

		// 精准干扰加成
		if quality.IsPreciseJam {
			score += 0.3
		}
	}

	// 探测结果贡献
	if probeResult != nil && !probeResult.Success {
		switch probeResult.BlockingType {
		case "rst_injection":
			score += 0.5
		case "http_403_inject", "http_302_redirect":
			score += 0.4
		case "tcp_zero_window":
			score += 0.35
		case "connection_refused":
			score += 0.3
		case "timeout":
			score += 0.2
		}
	}

	// 限制在 0-1
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// applyWeights 应用权重
func (cae *ContextAwareEvaluator) applyWeights(
	baseScore float64,
	quality *LinkQuality,
	probeResult *ProbeResult,
) float64 {
	if cae.mimicryWeight == nil {
		return baseScore
	}

	weight := cae.mimicryWeight
	multiplier := 1.0

	// 根据协议类型调整
	if probeResult != nil {
		profile := GetAppProfile(probeResult.AppName)
		if profile != nil {
			switch profile.Protocol {
			case "QUIC":
				multiplier *= weight.QUICWeight
			case "UDP":
				multiplier *= weight.UDPWeight
			case "TCP":
				multiplier *= weight.TCPWeight
			}
		}
	}

	// 根据指标类型调整
	if quality != nil {
		if quality.RTTMean > float64(cae.threshold.YellowRTT.Milliseconds()) {
			multiplier *= weight.RTTWeight
		}
		if quality.JitterEntropy > cae.threshold.YellowJitter {
			multiplier *= weight.JitterWeight
		}
		if quality.LossRate > cae.threshold.YellowLoss {
			multiplier *= weight.PacketLossWeight
		}
	}

	return baseScore * multiplier
}

// classifyDamage 分类战损类型
func (cae *ContextAwareEvaluator) classifyDamage(
	quality *LinkQuality,
	probeResult *ProbeResult,
) DamageType {
	if probeResult == nil {
		if quality != nil && quality.IsPreciseJam {
			return DamageProtocolBlock
		}
		return DamageNone
	}

	switch probeResult.BlockingType {
	case "rst_injection":
		return DamageProtocolBlock
	case "http_403_inject", "http_302_redirect":
		return DamageSNIBlock
	case "tcp_zero_window":
		return DamageLatency
	case "quic_version_neg":
		return DamageQUICBlock
	case "timeout":
		if quality != nil && quality.LossRate > 20 {
			return DamagePacketLoss
		}
		return DamageLatency
	}

	// 检查是否为全域封锁
	if probeResult.BlockingType == "network_unreachable" {
		return DamageFullBlock
	}

	return DamageNone
}

// calculateConfidence 计算置信度
func (cae *ContextAwareEvaluator) calculateConfidence(
	quality *LinkQuality,
	probeResult *ProbeResult,
) float64 {
	confidence := 0.5 // 基础置信度

	// 多个指标一致性提升置信度
	indicators := 0
	if quality != nil {
		if quality.LossRate > cae.threshold.YellowLoss {
			indicators++
		}
		if quality.RTTMean > float64(cae.threshold.YellowRTT.Milliseconds()) {
			indicators++
		}
		if quality.IsPreciseJam {
			indicators++
		}
	}
	if probeResult != nil && !probeResult.Success {
		indicators++
	}

	confidence += float64(indicators) * 0.1

	// 历史一致性
	if history, ok := cae.scoreHistory[probeResult.AppName]; ok && len(history) > 5 {
		recentFails := 0
		for i := len(history) - 5; i < len(history); i++ {
			if history[i].BaseScore > 0.5 {
				recentFails++
			}
		}
		if recentFails >= 3 {
			confidence += 0.2
		}
	}

	if confidence > 1.0 {
		confidence = 1.0
	}
	return confidence
}

// recordScore 记录分数
func (cae *ContextAwareEvaluator) recordScore(appName string, score *ContextAwareScore) {
	history := cae.scoreHistory[appName]
	history = append(history, score)
	if len(history) > cae.historySize {
		history = history[1:]
	}
	cae.scoreHistory[appName] = history
}

// GetAlertLevel 获取告警等级
func (cae *ContextAwareEvaluator) GetAlertLevel(appName string) AlertLevel {
	cae.mu.RLock()
	defer cae.mu.RUnlock()

	score, ok := cae.currentScores[appName]
	if !ok {
		return AlertNone
	}

	if score.WeightedScore > 0.7 {
		return AlertRed
	}
	if score.WeightedScore > 0.4 {
		return AlertYellow
	}
	return AlertNone
}

// GetCurrentScores 获取当前评分
func (cae *ContextAwareEvaluator) GetCurrentScores() map[string]*ContextAwareScore {
	cae.mu.RLock()
	defer cae.mu.RUnlock()

	scores := make(map[string]*ContextAwareScore)
	for k, v := range cae.currentScores {
		scores[k] = v
	}
	return scores
}

// ShouldTriggerEscape 是否应触发逃逸
func (cae *ContextAwareEvaluator) ShouldTriggerEscape(appName string) (bool, DamageType) {
	cae.mu.RLock()
	defer cae.mu.RUnlock()

	score, ok := cae.currentScores[appName]
	if !ok {
		return false, DamageNone
	}

	// 高置信度 + 高分数 = 触发逃逸
	if score.WeightedScore > 0.7 && score.Confidence > 0.7 {
		return true, score.DamageType
	}

	return false, DamageNone
}
