// Package gtunnel - 流量形态整形器 (Traffic Shape Shaper)
// O5 深度隐匿：抗 ML 统计流分析
//
// 问题：真实代理流量呈现"突发-静默"模型（Burst-Idle），
// 与伪装目标（视频会议/CDN）的平稳连续流完全不同。
// ML 分类器通过 Packet Size Distribution + IAT Distribution 即可识别。
//
// 解决方案：马尔可夫链驱动的废包注入 + 等长分布填充
// 在无真实数据时，按目标流量模型的概率分布持续发送 Dummy Packets，
// 使整体流量形态在统计学上与真实视频流不可区分。
package gtunnel

import (
	"context"
	"crypto/rand"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// TrafficProfile 流量模型配置（描述目标伪装流量的统计特征）
type TrafficProfile struct {
	Name string

	// 包大小分布参数（正态分布近似）
	PktSizeMean   float64 // 平均包大小 (bytes)
	PktSizeStddev float64 // 标准差

	// 发包间隔分布参数（指数分布）
	IATMeanMs float64 // 平均 IAT (毫秒)

	// 马尔可夫链状态转移矩阵
	// States: 0=Idle, 1=LowRate, 2=MediumRate, 3=HighRate (burst)
	TransitionMatrix [4][4]float64

	// 各状态的发包速率 (packets/second)
	StateRates [4]float64
}

// 预设流量模型
var (
	// ProfileVideoConference 视频会议模型（Zoom/Teams 特征）
	// 特征：持续的中等速率 + 偶尔的关键帧突发
	ProfileVideoConference = &TrafficProfile{
		Name:          "video-conference",
		PktSizeMean:   1100,
		PktSizeStddev: 300,
		IATMeanMs:     8.3, // ~120 pkt/s
		TransitionMatrix: [4][4]float64{
			{0.10, 0.60, 0.25, 0.05}, // From Idle
			{0.05, 0.70, 0.20, 0.05}, // From LowRate
			{0.02, 0.15, 0.73, 0.10}, // From MediumRate
			{0.05, 0.10, 0.60, 0.25}, // From HighRate
		},
		StateRates: [4]float64{5, 30, 80, 150}, // pkt/s per state
	}

	// ProfileCDNStreaming CDN 流媒体模型（Netflix/YouTube 特征）
	// 特征：大包为主 + 周期性的 buffer refill burst
	ProfileCDNStreaming = &TrafficProfile{
		Name:          "cdn-streaming",
		PktSizeMean:   1400,
		PktSizeStddev: 100,
		IATMeanMs:     5.0, // ~200 pkt/s
		TransitionMatrix: [4][4]float64{
			{0.20, 0.50, 0.25, 0.05}, // From Idle
			{0.10, 0.60, 0.25, 0.05}, // From LowRate
			{0.05, 0.20, 0.65, 0.10}, // From MediumRate
			{0.10, 0.15, 0.50, 0.25}, // From HighRate
		},
		StateRates: [4]float64{2, 50, 150, 300},
	}
)

// TrafficShaper 流量形态整形器
type TrafficShaper struct {
	profile *TrafficProfile
	conn    TransportConn // 底层传输连接

	// 马尔可夫链当前状态
	currentState int32

	// 真实流量活动检测
	lastRealPacketTime atomic.Int64 // Unix nano
	realPacketCount    atomic.Int64

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex

	// 统计
	dummyPacketsSent atomic.Int64
	dummyBytesSent   atomic.Int64
}

// NewTrafficShaper 创建流量整形器
func NewTrafficShaper(conn TransportConn, profile *TrafficProfile) *TrafficShaper {
	ctx, cancel := context.WithCancel(context.Background())
	return &TrafficShaper{
		profile: profile,
		conn:    conn,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动废包注入循环
func (ts *TrafficShaper) Start() {
	go ts.dummyInjectionLoop()
	log.Printf("🎭 [TrafficShaper] 已启动: profile=%s", ts.profile.Name)
}

// Stop 停止
func (ts *TrafficShaper) Stop() {
	ts.cancel()
}

// NotifyRealPacket 通知有真实数据包发送（用于调整废包注入节奏）
func (ts *TrafficShaper) NotifyRealPacket(size int) {
	ts.lastRealPacketTime.Store(time.Now().UnixNano())
	ts.realPacketCount.Add(1)
}

// dummyInjectionLoop 马尔可夫链驱动的废包注入主循环
func (ts *TrafficShaper) dummyInjectionLoop() {
	state := 1 // 初始状态：LowRate

	for {
		select {
		case <-ts.ctx.Done():
			return
		default:
		}

		// 1. 计算当前状态的发包间隔
		rate := ts.profile.StateRates[state]
		if rate <= 0 {
			rate = 1
		}
		interval := time.Duration(float64(time.Second) / rate)

		// 2. 添加抖动（±20%）使间隔不完全规律
		jitter := ts.randomJitter(interval, 0.2)
		sleepDuration := interval + jitter

		// 3. 等待
		timer := time.NewTimer(sleepDuration)
		select {
		case <-ts.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		// 4. 检查是否有真实流量活动
		// 如果最近 50ms 内有真实包发送，跳过本轮废包（避免叠加导致速率异常）
		lastReal := ts.lastRealPacketTime.Load()
		if lastReal > 0 && time.Now().UnixNano()-lastReal < int64(50*time.Millisecond) {
			// 真实流量活跃，不注入废包
			goto transition
		}

		// 5. 生成并发送废包
		ts.sendDummyPacket(state)

	transition:
		// 6. 马尔可夫链状态转移
		state = ts.nextState(state)
		atomic.StoreInt32(&ts.currentState, int32(state))
	}
}

// sendDummyPacket 生成并发送一个废包
func (ts *TrafficShaper) sendDummyPacket(_ int) {
	// 包大小：从正态分布采样
	size := ts.samplePacketSize()
	if size < 64 {
		size = 64
	}
	if size > 1400 {
		size = 1400
	}

	// 生成随机填充数据（高熵，不可压缩）
	dummy := make([]byte, size)
	rand.Read(dummy)

	// 标记为废包（头部 magic，接收端识别后丢弃）
	// Magic: 0xDE 0xAD（废包标识，接收端 strip 后不交付上层）
	if len(dummy) >= 2 {
		dummy[0] = 0xDE
		dummy[1] = 0xAD
	}

	// 发送
	if err := ts.conn.Send(dummy); err != nil {
		// 发送失败不中断循环
		return
	}

	ts.dummyPacketsSent.Add(1)
	ts.dummyBytesSent.Add(int64(size))
}

// nextState 马尔可夫链状态转移
func (ts *TrafficShaper) nextState(current int) int {
	// 生成 [0, 1) 随机数
	var buf [4]byte
	rand.Read(buf[:])
	r := float64(uint32(buf[0])<<24|uint32(buf[1])<<16|uint32(buf[2])<<8|uint32(buf[3])) / float64(math.MaxUint32)

	// 按转移概率选择下一状态
	row := ts.profile.TransitionMatrix[current]
	cumulative := 0.0
	for i, prob := range row {
		cumulative += prob
		if r < cumulative {
			return i
		}
	}
	return current // fallback
}

// samplePacketSize 从正态分布采样包大小
func (ts *TrafficShaper) samplePacketSize() int {
	// Box-Muller 变换生成正态分布
	var buf [8]byte
	rand.Read(buf[:])
	u1 := float64(uint32(buf[0])<<24|uint32(buf[1])<<16|uint32(buf[2])<<8|uint32(buf[3])) / float64(math.MaxUint32)
	u2 := float64(uint32(buf[4])<<24|uint32(buf[5])<<16|uint32(buf[6])<<8|uint32(buf[7])) / float64(math.MaxUint32)

	if u1 < 1e-10 {
		u1 = 1e-10
	}

	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	size := ts.profile.PktSizeMean + z*ts.profile.PktSizeStddev

	return int(size)
}

// randomJitter 生成 ±ratio 范围内的随机抖动
func (ts *TrafficShaper) randomJitter(base time.Duration, ratio float64) time.Duration {
	var buf [4]byte
	rand.Read(buf[:])
	r := float64(uint32(buf[0])<<24|uint32(buf[1])<<16|uint32(buf[2])<<8|uint32(buf[3])) / float64(math.MaxUint32)

	// [-ratio, +ratio]
	jitterFactor := (r*2 - 1) * ratio
	return time.Duration(float64(base) * jitterFactor)
}

// GetStats 获取统计信息
func (ts *TrafficShaper) GetStats() (dummyPackets, dummyBytes int64) {
	return ts.dummyPacketsSent.Load(), ts.dummyBytesSent.Load()
}
