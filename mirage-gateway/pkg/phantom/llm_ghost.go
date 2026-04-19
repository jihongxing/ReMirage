// Package phantom 图灵诱导模块
// 为影子系统增加具备"人类感"的延迟反馈逻辑
package phantom

import (
	"math/rand"
	"strings"
	"sync"
	"time"
)

// LLMGhost 幽灵对话引擎
type LLMGhost struct {
	mu sync.RWMutex

	// 会话状态
	sessions map[string]*GhostSession

	// 响应模板库
	templates *TemplateLibrary

	// 配置
	config GhostConfig

	// 统计
	stats GhostStats

	// 回调
	onResponse func(uid string, response string)
}

// GhostSession 幽灵会话
type GhostSession struct {
	UID           string
	StartTime     time.Time
	MessageCount  int
	LastMessage   time.Time
	State         SessionState
	EmotionLevel  int // -5 到 5，负数表示攻击性
	LockedUntil   *time.Time
	Context       []string // 上下文记忆
}

// SessionState 会话状态
type SessionState int

const (
	StateNormal    SessionState = iota
	StateEscalated              // 升级处理中
	StateLocked                 // 已锁定
	StateAbandoned              // 已放弃
)

// GhostConfig 配置
type GhostConfig struct {
	MinResponseDelayMs int     // 最小响应延迟
	MaxResponseDelayMs int     // 最大响应延迟
	TypingSpeedCPM     int     // 打字速度（字符/分钟）
	LockDurationMin    int     // 锁定时长（分钟）
	MaxContextSize     int     // 上下文记忆大小
	AggressionThreshold int    // 攻击性阈值
}

// GhostStats 统计
type GhostStats struct {
	TotalSessions     int64
	TotalMessages     int64
	AvgEngagementMin  float64
	LockedSessions    int64
	EscalatedSessions int64
}

// TemplateLibrary 模板库
type TemplateLibrary struct {
	// 通用响应
	Generic []string
	// 权限相关
	Permission []string
	// 技术支持
	Support []string
	// 升级处理
	Escalation []string
	// 锁定提示
	Locked []string
	// 攻击性检测关键词
	AggressiveKeywords []string
}

// DefaultGhostConfig 默认配置
func DefaultGhostConfig() GhostConfig {
	return GhostConfig{
		MinResponseDelayMs:  60000,  // 1 分钟
		MaxResponseDelayMs:  180000, // 3 分钟
		TypingSpeedCPM:      200,    // 200 字符/分钟
		LockDurationMin:     30,
		MaxContextSize:      10,
		AggressionThreshold: -3,
	}
}

// NewLLMGhost 创建幽灵引擎
func NewLLMGhost(config GhostConfig) *LLMGhost {
	ghost := &LLMGhost{
		sessions:  make(map[string]*GhostSession),
		templates: initTemplates(),
		config:    config,
	}
	return ghost
}

// initTemplates 初始化模板库
func initTemplates() *TemplateLibrary {
	return &TemplateLibrary{
		Generic: []string{
			"感谢您的反馈，我们已收到您的请求。",
			"您的问题已记录，技术团队将尽快处理。",
			"感谢您的耐心等待，我们正在核实相关信息。",
			"您的请求已进入处理队列，预计 24-48 小时内回复。",
			"我们已注意到您的问题，正在协调相关部门。",
			"感谢您联系我们，您的工单编号是 #%TICKET%。",
		},
		Permission: []string{
			"您的权限申请正在审核中，请稍后查看邮件通知。",
			"管理员正在处理您的权限请求，预计 1-2 个工作日。",
			"您的账户权限变更需要上级审批，请耐心等待。",
			"权限申请已提交，系统将在审核通过后自动开通。",
			"您的访问级别调整请求已收到，正在验证身份信息。",
		},
		Support: []string{
			"技术支持团队已收到您的问题，正在分析中。",
			"我们的工程师正在查看您描述的情况。",
			"感谢您提供的详细信息，这对我们排查问题很有帮助。",
			"您的问题已升级至二线支持，他们会尽快联系您。",
			"我们正在复现您描述的问题，请保持在线。",
		},
		Escalation: []string{
			"您的问题已升级至高级技术团队处理。",
			"鉴于问题的复杂性，我们已安排专人跟进。",
			"您的案例已标记为优先处理，感谢您的理解。",
			"我们的安全团队正在审查您的请求。",
		},
		Locked: []string{
			"由于检测到异常行为，您的临时账号已被锁定。",
			"系统检测到可疑操作，账户已进入安全保护状态。",
			"您的会话因安全原因已被暂停，请联系管理员解锁。",
			"账户已被临时冻结，如有疑问请通过官方渠道申诉。",
			"检测到多次异常请求，账户已进入冷却期。",
		},
		AggressiveKeywords: []string{
			"hack", "inject", "exploit", "bypass", "crack",
			"admin", "root", "sudo", "shell", "exec",
			"drop", "delete", "truncate", "union", "select",
			"<script", "javascript:", "onerror", "onload",
			"../", "..\\", "/etc/", "passwd", "shadow",
		},
	}
}

// ProcessMessage 处理消息
func (lg *LLMGhost) ProcessMessage(uid string, message string) *GhostResponse {
	session := lg.getOrCreateSession(uid)

	lg.mu.Lock()
	session.MessageCount++
	session.LastMessage = time.Now()
	lg.stats.TotalMessages++

	// 添加到上下文
	if len(session.Context) >= lg.config.MaxContextSize {
		session.Context = session.Context[1:]
	}
	session.Context = append(session.Context, message)
	lg.mu.Unlock()

	// 检查锁定状态
	if session.State == StateLocked {
		if session.LockedUntil != nil && time.Now().Before(*session.LockedUntil) {
			return lg.generateLockedResponse(session)
		}
		// 解锁
		lg.mu.Lock()
		session.State = StateNormal
		session.LockedUntil = nil
		lg.mu.Unlock()
	}

	// 分析情绪/攻击性
	emotionDelta := lg.analyzeEmotion(message)
	lg.mu.Lock()
	session.EmotionLevel += emotionDelta
	lg.mu.Unlock()

	// 检查是否触发锁定
	if session.EmotionLevel <= lg.config.AggressionThreshold {
		return lg.lockSession(session)
	}

	// 生成响应
	return lg.generateResponse(session, message)
}

// getOrCreateSession 获取或创建会话
func (lg *LLMGhost) getOrCreateSession(uid string) *GhostSession {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	session, exists := lg.sessions[uid]
	if !exists {
		session = &GhostSession{
			UID:          uid,
			StartTime:    time.Now(),
			State:        StateNormal,
			EmotionLevel: 0,
			Context:      make([]string, 0),
		}
		lg.sessions[uid] = session
		lg.stats.TotalSessions++
	}

	return session
}

// analyzeEmotion 分析情绪
func (lg *LLMGhost) analyzeEmotion(message string) int {
	msgLower := strings.ToLower(message)
	delta := 0

	// 检查攻击性关键词
	for _, keyword := range lg.templates.AggressiveKeywords {
		if strings.Contains(msgLower, keyword) {
			delta -= 2
		}
	}

	// 检查礼貌用语
	politeWords := []string{"please", "thank", "sorry", "help", "请", "谢谢", "麻烦"}
	for _, word := range politeWords {
		if strings.Contains(msgLower, word) {
			delta += 1
		}
	}

	// 检查长度（过长可能是攻击载荷）
	if len(message) > 500 {
		delta -= 1
	}
	if len(message) > 1000 {
		delta -= 2
	}

	return delta
}

// lockSession 锁定会话
func (lg *LLMGhost) lockSession(session *GhostSession) *GhostResponse {
	lg.mu.Lock()
	session.State = StateLocked
	lockUntil := time.Now().Add(time.Duration(lg.config.LockDurationMin) * time.Minute)
	session.LockedUntil = &lockUntil
	lg.stats.LockedSessions++
	lg.mu.Unlock()

	return lg.generateLockedResponse(session)
}

// generateLockedResponse 生成锁定响应
func (lg *LLMGhost) generateLockedResponse(session *GhostSession) *GhostResponse {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	template := lg.templates.Locked[rng.Intn(len(lg.templates.Locked))]

	return &GhostResponse{
		Message:     template,
		DelayMs:     lg.calculateDelay(template),
		IsLocked:    true,
		TypingMs:    lg.calculateTypingTime(template),
		SessionState: StateLocked,
	}
}

// GhostResponse 幽灵响应
type GhostResponse struct {
	Message      string
	DelayMs      int          // 响应延迟
	TypingMs     int          // 打字时间
	IsLocked     bool
	SessionState SessionState
	TicketID     string
}

// generateResponse 生成响应
func (lg *LLMGhost) generateResponse(session *GhostSession, message string) *GhostResponse {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 根据上下文选择模板类别
	var templates []string
	msgLower := strings.ToLower(message)

	if containsAny(msgLower, []string{"permission", "access", "权限", "访问"}) {
		templates = lg.templates.Permission
	} else if containsAny(msgLower, []string{"error", "bug", "issue", "问题", "错误"}) {
		templates = lg.templates.Support
	} else if session.MessageCount > 5 {
		// 多次交互后升级
		templates = lg.templates.Escalation
		lg.mu.Lock()
		if session.State != StateEscalated {
			session.State = StateEscalated
			lg.stats.EscalatedSessions++
		}
		lg.mu.Unlock()
	} else {
		templates = lg.templates.Generic
	}

	template := templates[rng.Intn(len(templates))]

	// 替换占位符
	ticketID := generateTicketID(rng)
	template = strings.ReplaceAll(template, "%TICKET%", ticketID)

	return &GhostResponse{
		Message:      template,
		DelayMs:      lg.calculateDelay(template),
		TypingMs:     lg.calculateTypingTime(template),
		IsLocked:     false,
		SessionState: session.State,
		TicketID:     ticketID,
	}
}

// calculateDelay 计算响应延迟
func (lg *LLMGhost) calculateDelay(message string) int {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	baseDelay := lg.config.MinResponseDelayMs +
		rng.Intn(lg.config.MaxResponseDelayMs-lg.config.MinResponseDelayMs)

	// 根据消息长度调整
	lengthFactor := float64(len(message)) / 100.0
	if lengthFactor > 2 {
		lengthFactor = 2
	}

	return int(float64(baseDelay) * (1 + lengthFactor*0.2))
}

// calculateTypingTime 计算打字时间
func (lg *LLMGhost) calculateTypingTime(message string) int {
	// 字符数 / (字符/分钟) * 60000 毫秒
	chars := len(message)
	baseTime := float64(chars) / float64(lg.config.TypingSpeedCPM) * 60000

	// 添加随机波动 ±20%
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	variance := baseTime * 0.2 * (rng.Float64()*2 - 1)

	return int(baseTime + variance)
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func generateTicketID(rng *rand.Rand) string {
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	id := make([]byte, 8)
	for i := range id {
		id[i] = chars[rng.Intn(len(chars))]
	}
	return string(id)
}

// GetSession 获取会话
func (lg *LLMGhost) GetSession(uid string) *GhostSession {
	lg.mu.RLock()
	defer lg.mu.RUnlock()
	return lg.sessions[uid]
}

// GetStats 获取统计
func (lg *LLMGhost) GetStats() GhostStats {
	lg.mu.RLock()
	defer lg.mu.RUnlock()

	stats := lg.stats

	// 计算平均参与时间
	if len(lg.sessions) > 0 {
		var totalEngagement time.Duration
		for _, session := range lg.sessions {
			totalEngagement += time.Since(session.StartTime)
		}
		stats.AvgEngagementMin = totalEngagement.Minutes() / float64(len(lg.sessions))
	}

	return stats
}

// OnResponse 设置响应回调
func (lg *LLMGhost) OnResponse(fn func(uid string, response string)) {
	lg.mu.Lock()
	defer lg.mu.Unlock()
	lg.onResponse = fn
}

// CleanupSessions 清理过期会话
func (lg *LLMGhost) CleanupSessions(maxAge time.Duration) int {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	cleaned := 0
	now := time.Now()

	for uid, session := range lg.sessions {
		if now.Sub(session.LastMessage) > maxAge {
			delete(lg.sessions, uid)
			cleaned++
		}
	}

	return cleaned
}
