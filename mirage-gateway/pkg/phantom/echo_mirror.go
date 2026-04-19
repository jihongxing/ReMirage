// Package phantom TCP/IP 协议栈伪装
// 动态回响：让影子系统具备"协议层面的性格"
package phantom

import (
	"math/rand"
	"net"
	"sync"
	"time"
)

// EchoMirror 协议栈镜像伪装器
type EchoMirror struct {
	mu sync.RWMutex

	// OS 指纹配置
	osProfiles map[string]*OSProfile

	// 地理延迟配置
	geoLatency map[string]*LatencyProfile

	// 活跃会话
	sessions map[string]*MirrorSession

	// 统计
	stats MirrorStats
}

// OSProfile 操作系统指纹配置
type OSProfile struct {
	Name          string
	WindowSize    uint16
	TTL           uint8
	MSS           uint16
	WindowScale   uint8
	SackPermitted bool
	Timestamps    bool
	NOP           bool
	// TCP 选项顺序
	OptionOrder []string
}

// LatencyProfile 延迟指纹配置
type LatencyProfile struct {
	Region       string
	BaseLatency  time.Duration
	JitterMin    time.Duration
	JitterMax    time.Duration
	PacketLoss   float32 // 模拟丢包率
	Variance     float32 // 延迟方差系数
}

// MirrorSession 镜像会话
type MirrorSession struct {
	UID          string
	AssignedOS   string
	AssignedGeo  string
	StartTime    time.Time
	PacketCount  int64
	TotalLatency time.Duration
}

// MirrorStats 统计
type MirrorStats struct {
	TotalSessions    int64
	ActiveSessions   int
	AvgLatencyAdded  time.Duration
	OSDistribution   map[string]int
	GeoDistribution  map[string]int
}

// NewEchoMirror 创建协议栈镜像器
func NewEchoMirror() *EchoMirror {
	em := &EchoMirror{
		osProfiles: make(map[string]*OSProfile),
		geoLatency: make(map[string]*LatencyProfile),
		sessions:   make(map[string]*MirrorSession),
		stats: MirrorStats{
			OSDistribution:  make(map[string]int),
			GeoDistribution: make(map[string]int),
		},
	}
	em.initOSProfiles()
	em.initGeoLatency()
	return em
}

// initOSProfiles 初始化 OS 指纹库
func (em *EchoMirror) initOSProfiles() {
	// Linux 2.6 内核特征
	em.osProfiles["linux_2.6"] = &OSProfile{
		Name:          "Linux 2.6.x",
		WindowSize:    5840,
		TTL:           64,
		MSS:           1460,
		WindowScale:   2,
		SackPermitted: true,
		Timestamps:    true,
		NOP:           true,
		OptionOrder:   []string{"MSS", "SACK", "TS", "NOP", "WS"},
	}

	// Linux 4.x/5.x 内核特征
	em.osProfiles["linux_modern"] = &OSProfile{
		Name:          "Linux 4.x/5.x",
		WindowSize:    29200,
		TTL:           64,
		MSS:           1460,
		WindowScale:   7,
		SackPermitted: true,
		Timestamps:    true,
		NOP:           true,
		OptionOrder:   []string{"MSS", "SACK", "TS", "NOP", "WS"},
	}

	// Windows Server 2012
	em.osProfiles["windows_2012"] = &OSProfile{
		Name:          "Windows Server 2012",
		WindowSize:    8192,
		TTL:           128,
		MSS:           1460,
		WindowScale:   8,
		SackPermitted: true,
		Timestamps:    false,
		NOP:           true,
		OptionOrder:   []string{"MSS", "NOP", "WS", "NOP", "NOP", "SACK"},
	}

	// Windows Server 2019
	em.osProfiles["windows_2019"] = &OSProfile{
		Name:          "Windows Server 2019",
		WindowSize:    65535,
		TTL:           128,
		MSS:           1460,
		WindowScale:   8,
		SackPermitted: true,
		Timestamps:    false,
		NOP:           true,
		OptionOrder:   []string{"MSS", "NOP", "WS", "SACK", "NOP"},
	}

	// FreeBSD
	em.osProfiles["freebsd"] = &OSProfile{
		Name:          "FreeBSD 12.x",
		WindowSize:    65535,
		TTL:           64,
		MSS:           1460,
		WindowScale:   6,
		SackPermitted: true,
		Timestamps:    true,
		NOP:           true,
		OptionOrder:   []string{"MSS", "NOP", "WS", "SACK", "TS"},
	}

	// 老旧路由器/嵌入式设备
	em.osProfiles["embedded"] = &OSProfile{
		Name:          "Embedded Linux",
		WindowSize:    4096,
		TTL:           64,
		MSS:           1400,
		WindowScale:   0,
		SackPermitted: false,
		Timestamps:    false,
		NOP:           false,
		OptionOrder:   []string{"MSS"},
	}
}

// initGeoLatency 初始化地理延迟配置
func (em *EchoMirror) initGeoLatency() {
	// 新加坡机房
	em.geoLatency["singapore"] = &LatencyProfile{
		Region:      "Singapore DC",
		BaseLatency: 35 * time.Millisecond,
		JitterMin:   1 * time.Millisecond,
		JitterMax:   8 * time.Millisecond,
		PacketLoss:  0.001,
		Variance:    0.15,
	}

	// 冰岛机房
	em.geoLatency["iceland"] = &LatencyProfile{
		Region:      "Iceland DC",
		BaseLatency: 120 * time.Millisecond,
		JitterMin:   5 * time.Millisecond,
		JitterMax:   25 * time.Millisecond,
		PacketLoss:  0.005,
		Variance:    0.25,
	}

	// 瑞士机房
	em.geoLatency["switzerland"] = &LatencyProfile{
		Region:      "Switzerland DC",
		BaseLatency: 85 * time.Millisecond,
		JitterMin:   2 * time.Millisecond,
		JitterMax:   12 * time.Millisecond,
		PacketLoss:  0.002,
		Variance:    0.18,
	}

	// 美国东海岸
	em.geoLatency["us_east"] = &LatencyProfile{
		Region:      "US East DC",
		BaseLatency: 180 * time.Millisecond,
		JitterMin:   3 * time.Millisecond,
		JitterMax:   15 * time.Millisecond,
		PacketLoss:  0.003,
		Variance:    0.20,
	}

	// 德国法兰克福
	em.geoLatency["frankfurt"] = &LatencyProfile{
		Region:      "Frankfurt DC",
		BaseLatency: 95 * time.Millisecond,
		JitterMin:   2 * time.Millisecond,
		JitterMax:   10 * time.Millisecond,
		PacketLoss:  0.002,
		Variance:    0.16,
	}
}

// GetOrCreateSession 获取或创建镜像会话
func (em *EchoMirror) GetOrCreateSession(uid string, seed int64) *MirrorSession {
	em.mu.Lock()
	defer em.mu.Unlock()

	if session, exists := em.sessions[uid]; exists {
		return session
	}

	rng := rand.New(rand.NewSource(seed))

	// 基于 seed 确定性选择 OS 和地理位置
	osKeys := []string{"linux_2.6", "linux_modern", "windows_2012", "windows_2019", "freebsd", "embedded"}
	geoKeys := []string{"singapore", "iceland", "switzerland", "us_east", "frankfurt"}

	selectedOS := osKeys[rng.Intn(len(osKeys))]
	selectedGeo := geoKeys[rng.Intn(len(geoKeys))]

	session := &MirrorSession{
		UID:         uid,
		AssignedOS:  selectedOS,
		AssignedGeo: selectedGeo,
		StartTime:   time.Now(),
	}

	em.sessions[uid] = session
	em.stats.TotalSessions++
	em.stats.ActiveSessions = len(em.sessions)
	em.stats.OSDistribution[selectedOS]++
	em.stats.GeoDistribution[selectedGeo]++

	return session
}

// GetTCPFingerprint 获取 TCP 指纹参数
func (em *EchoMirror) GetTCPFingerprint(uid string, seed int64) *TCPFingerprint {
	session := em.GetOrCreateSession(uid, seed)

	em.mu.RLock()
	profile := em.osProfiles[session.AssignedOS]
	em.mu.RUnlock()

	if profile == nil {
		profile = em.osProfiles["linux_modern"]
	}

	return &TCPFingerprint{
		WindowSize:    profile.WindowSize,
		TTL:           profile.TTL,
		MSS:           profile.MSS,
		WindowScale:   profile.WindowScale,
		SackPermitted: profile.SackPermitted,
		Timestamps:    profile.Timestamps,
		OptionOrder:   profile.OptionOrder,
		OSName:        profile.Name,
	}
}

// TCPFingerprint TCP 指纹
type TCPFingerprint struct {
	WindowSize    uint16
	TTL           uint8
	MSS           uint16
	WindowScale   uint8
	SackPermitted bool
	Timestamps    bool
	OptionOrder   []string
	OSName        string
}

// CalculateLatency 计算模拟延迟
func (em *EchoMirror) CalculateLatency(uid string, seed int64, packetNum int64) time.Duration {
	session := em.GetOrCreateSession(uid, seed)

	em.mu.RLock()
	latencyProfile := em.geoLatency[session.AssignedGeo]
	em.mu.RUnlock()

	if latencyProfile == nil {
		latencyProfile = em.geoLatency["singapore"]
	}

	// 基于包序号生成确定性但看似随机的抖动
	rng := rand.New(rand.NewSource(seed + packetNum))

	// 基础延迟
	latency := latencyProfile.BaseLatency

	// 添加非均匀抖动（模拟真实网络）
	jitterRange := latencyProfile.JitterMax - latencyProfile.JitterMin
	jitter := latencyProfile.JitterMin + time.Duration(rng.Float64()*float64(jitterRange))

	// 添加方差（模拟网络拥塞波动）
	variance := 1.0 + (rng.Float64()-0.5)*2*float64(latencyProfile.Variance)
	latency = time.Duration(float64(latency) * variance)

	// 偶尔的延迟尖峰（模拟路由抖动）
	if rng.Float32() < 0.02 {
		latency += time.Duration(rng.Intn(50)+20) * time.Millisecond
	}

	// 更新统计
	em.mu.Lock()
	session.PacketCount++
	session.TotalLatency += latency + jitter
	em.mu.Unlock()

	return latency + jitter
}

// ShouldDropPacket 判断是否模拟丢包
func (em *EchoMirror) ShouldDropPacket(uid string, seed int64, packetNum int64) bool {
	session := em.GetOrCreateSession(uid, seed)

	em.mu.RLock()
	latencyProfile := em.geoLatency[session.AssignedGeo]
	em.mu.RUnlock()

	if latencyProfile == nil {
		return false
	}

	rng := rand.New(rand.NewSource(seed + packetNum*7))
	return rng.Float32() < latencyProfile.PacketLoss
}

// GenerateTCPOptions 生成 TCP 选项字节序列
func (em *EchoMirror) GenerateTCPOptions(fp *TCPFingerprint) []byte {
	var options []byte

	for _, opt := range fp.OptionOrder {
		switch opt {
		case "MSS":
			options = append(options, 0x02, 0x04) // MSS kind + length
			options = append(options, byte(fp.MSS>>8), byte(fp.MSS&0xff))
		case "SACK":
			if fp.SackPermitted {
				options = append(options, 0x04, 0x02) // SACK permitted
			}
		case "TS":
			if fp.Timestamps {
				options = append(options, 0x08, 0x0a) // Timestamp kind + length
				ts := uint32(time.Now().UnixNano() / 1000000)
				options = append(options, byte(ts>>24), byte(ts>>16), byte(ts>>8), byte(ts))
				options = append(options, 0, 0, 0, 0) // Echo reply
			}
		case "NOP":
			options = append(options, 0x01) // NOP
		case "WS":
			if fp.WindowScale > 0 {
				options = append(options, 0x03, 0x03, fp.WindowScale) // Window scale
			}
		}
	}

	// 填充到 4 字节对齐
	for len(options)%4 != 0 {
		options = append(options, 0x00) // End of options
	}

	return options
}

// GetSessionInfo 获取会话信息
func (em *EchoMirror) GetSessionInfo(uid string) *MirrorSessionInfo {
	em.mu.RLock()
	defer em.mu.RUnlock()

	session, exists := em.sessions[uid]
	if !exists {
		return nil
	}

	osProfile := em.osProfiles[session.AssignedOS]
	geoProfile := em.geoLatency[session.AssignedGeo]

	info := &MirrorSessionInfo{
		UID:         uid,
		OSName:      osProfile.Name,
		GeoRegion:   geoProfile.Region,
		PacketCount: session.PacketCount,
		Duration:    time.Since(session.StartTime),
	}

	if session.PacketCount > 0 {
		info.AvgLatency = session.TotalLatency / time.Duration(session.PacketCount)
	}

	return info
}

// MirrorSessionInfo 会话信息
type MirrorSessionInfo struct {
	UID         string
	OSName      string
	GeoRegion   string
	PacketCount int64
	AvgLatency  time.Duration
	Duration    time.Duration
}

// GetStats 获取统计
func (em *EchoMirror) GetStats() MirrorStats {
	em.mu.RLock()
	defer em.mu.RUnlock()

	stats := em.stats
	stats.ActiveSessions = len(em.sessions)

	// 计算平均延迟
	var totalLatency time.Duration
	var totalPackets int64
	for _, session := range em.sessions {
		totalLatency += session.TotalLatency
		totalPackets += session.PacketCount
	}
	if totalPackets > 0 {
		stats.AvgLatencyAdded = totalLatency / time.Duration(totalPackets)
	}

	return stats
}

// CleanupSessions 清理过期会话
func (em *EchoMirror) CleanupSessions(maxAge time.Duration) int {
	em.mu.Lock()
	defer em.mu.Unlock()

	cleaned := 0
	now := time.Now()

	for uid, session := range em.sessions {
		if now.Sub(session.StartTime) > maxAge {
			delete(em.sessions, uid)
			cleaned++
		}
	}

	em.stats.ActiveSessions = len(em.sessions)
	return cleaned
}

// ApplyToConn 应用指纹到连接（需要 root 权限）
func (em *EchoMirror) ApplyToConn(conn net.Conn, uid string, seed int64) error {
	fp := em.GetTCPFingerprint(uid, seed)

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}

	// 设置 TCP 窗口大小（通过 SO_RCVBUF）
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return err
	}

	_ = rawConn.Control(func(fd uintptr) {
		// 注意：实际的 TTL 和窗口大小修改需要在 eBPF 层完成
		// 这里只是接口预留
		_ = fp
	})

	return nil
}
