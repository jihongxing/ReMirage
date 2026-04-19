// Package gtunnel - 多路径调度器
package gtunnel

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Path 传输路径
type Path struct {
	ID          string        // 路径 ID
	CellID      string        // 蜂窝 ID
	Interface   string        // 网络接口名称
	RemoteAddr  *net.UDPAddr  // 远程地址
	LocalAddr   *net.UDPAddr  // 本地地址
	Conn        *net.UDPConn  // UDP 连接
	RTT         time.Duration // 往返时延
	LossRate    float64       // 丢包率
	Bandwidth   int64         // 带宽（bps）
	IsActive    bool          // 是否活跃
	LastUsed    time.Time     // 最后使用时间
	PacketsSent int64         // 已发送包数
	PacketsLost int64         // 丢失包数
}

// PathScheduler 路径调度器
type PathScheduler struct {
	paths      map[string]*Path
	activePath *Path
	mu         sync.RWMutex
	strategy   string // 调度策略：round-robin, lowest-rtt, redundant
}

// NewPathScheduler 创建路径调度器
func NewPathScheduler(strategy string) *PathScheduler {
	return &PathScheduler{
		paths:    make(map[string]*Path),
		strategy: strategy,
	}
}

// AddPath 添加路径
func (ps *PathScheduler) AddPath(path *Path) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	
	// 创建 UDP 连接
	conn, err := net.DialUDP("udp", path.LocalAddr, path.RemoteAddr)
	if err != nil {
		return fmt.Errorf("创建 UDP 连接失败: %w", err)
	}
	
	path.Conn = conn
	path.IsActive = true
	path.LastUsed = time.Now()
	
	ps.paths[path.ID] = path
	
	// 如果是第一条路径，设为活跃路径
	if ps.activePath == nil {
		ps.activePath = path
	}
	
	log.Printf("🛤️  [G-Tunnel] 添加路径: %s (蜂窝: %s, 接口: %s)", 
		path.ID, path.CellID, path.Interface)
	
	return nil
}

// RemovePath 移除路径
func (ps *PathScheduler) RemovePath(pathID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	
	path, exists := ps.paths[pathID]
	if !exists {
		return fmt.Errorf("路径不存在: %s", pathID)
	}
	
	// 关闭连接
	if path.Conn != nil {
		path.Conn.Close()
	}
	
	delete(ps.paths, pathID)
	
	// 如果删除的是活跃路径，切换到其他路径
	if ps.activePath != nil && ps.activePath.ID == pathID {
		ps.activePath = ps.selectBestPath()
	}
	
	log.Printf("🛤️  [G-Tunnel] 移除路径: %s", pathID)
	
	return nil
}

// SendShard 发送分片（根据策略选择路径）
func (ps *PathScheduler) SendShard(shard []byte) error {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	
	switch ps.strategy {
	case "round-robin":
		return ps.sendRoundRobin(shard)
	case "lowest-rtt":
		return ps.sendLowestRTT(shard)
	case "redundant":
		return ps.sendRedundant(shard)
	default:
		return ps.sendRoundRobin(shard)
	}
}

// sendRoundRobin 轮询发送
func (ps *PathScheduler) sendRoundRobin(shard []byte) error {
	if len(ps.paths) == 0 {
		return fmt.Errorf("无可用路径")
	}
	
	// 选择下一条路径
	path := ps.selectNextPath()
	if path == nil {
		return fmt.Errorf("无活跃路径")
	}
	
	// 发送数据
	_, err := path.Conn.Write(shard)
	if err != nil {
		path.PacketsLost++
		return err
	}
	
	path.PacketsSent++
	path.LastUsed = time.Now()
	
	return nil
}

// sendLowestRTT 选择最低延迟路径发送
func (ps *PathScheduler) sendLowestRTT(shard []byte) error {
	path := ps.selectBestPath()
	if path == nil {
		return fmt.Errorf("无可用路径")
	}
	
	_, err := path.Conn.Write(shard)
	if err != nil {
		path.PacketsLost++
		return err
	}
	
	path.PacketsSent++
	path.LastUsed = time.Now()
	
	return nil
}

// sendRedundant 冗余发送（所有路径同时发送）
func (ps *PathScheduler) sendRedundant(shard []byte) error {
	if len(ps.paths) == 0 {
		return fmt.Errorf("无可用路径")
	}
	
	var lastErr error
	successCount := 0
	
	for _, path := range ps.paths {
		if !path.IsActive {
			continue
		}
		
		_, err := path.Conn.Write(shard)
		if err != nil {
			path.PacketsLost++
			lastErr = err
		} else {
			path.PacketsSent++
			path.LastUsed = time.Now()
			successCount++
		}
	}
	
	if successCount == 0 {
		return fmt.Errorf("所有路径发送失败: %v", lastErr)
	}
	
	return nil
}

// selectNextPath 选择下一条路径（轮询）
func (ps *PathScheduler) selectNextPath() *Path {
	var nextPath *Path
	var oldestUsed time.Time = time.Now()
	
	for _, path := range ps.paths {
		if !path.IsActive {
			continue
		}
		
		if path.LastUsed.Before(oldestUsed) {
			oldestUsed = path.LastUsed
			nextPath = path
		}
	}
	
	return nextPath
}

// selectBestPath 选择最佳路径（最低 RTT）
func (ps *PathScheduler) selectBestPath() *Path {
	var bestPath *Path
	var lowestRTT time.Duration = time.Hour
	
	for _, path := range ps.paths {
		if !path.IsActive {
			continue
		}
		
		if path.RTT < lowestRTT {
			lowestRTT = path.RTT
			bestPath = path
		}
	}
	
	return bestPath
}

// UpdatePathMetrics 更新路径指标
func (ps *PathScheduler) UpdatePathMetrics(pathID string, rtt time.Duration, lossRate float64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	
	path, exists := ps.paths[pathID]
	if !exists {
		return
	}
	
	path.RTT = rtt
	path.LossRate = lossRate
	
	// 如果丢包率过高，标记为不活跃
	if lossRate > 0.5 {
		path.IsActive = false
		log.Printf("⚠️  [G-Tunnel] 路径 %s 丢包率过高 (%.1f%%)，已禁用", pathID, lossRate*100)
	}
}

// SwitchPath 切换活跃路径（转生协议）
func (ps *PathScheduler) SwitchPath(newPathID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	
	newPath, exists := ps.paths[newPathID]
	if !exists {
		return fmt.Errorf("路径不存在: %s", newPathID)
	}
	
	if !newPath.IsActive {
		return fmt.Errorf("路径不活跃: %s", newPathID)
	}
	
	oldPathID := ""
	if ps.activePath != nil {
		oldPathID = ps.activePath.ID
	}
	
	ps.activePath = newPath
	
	log.Printf("🔄 [G-Tunnel] 路径切换: %s → %s", oldPathID, newPathID)
	
	return nil
}

// GetActivePath 获取当前活跃路径
func (ps *PathScheduler) GetActivePath() *Path {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	
	return ps.activePath
}

// GetAllPaths 获取所有路径
func (ps *PathScheduler) GetAllPaths() []*Path {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	
	paths := make([]*Path, 0, len(ps.paths))
	for _, path := range ps.paths {
		paths = append(paths, path)
	}
	
	return paths
}

// MonitorPaths 监控路径状态
func (ps *PathScheduler) MonitorPaths() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		ps.mu.RLock()
		for _, path := range ps.paths {
			if !path.IsActive {
				continue
			}
			
			// 计算丢包率
			if path.PacketsSent > 0 {
				lossRate := float64(path.PacketsLost) / float64(path.PacketsSent)
				path.LossRate = lossRate
				
				if lossRate > 0.3 {
					log.Printf("⚠️  [G-Tunnel] 路径 %s 丢包率: %.1f%%", path.ID, lossRate*100)
				}
			}
		}
		ps.mu.RUnlock()
	}
}

// ============================================
// 双发选收模式 (Multi-Path Buffering)
// ============================================

// DualSendMode 双发选收模式状态
type DualSendMode struct {
	Enabled       bool          // 是否启用
	OldPath       *Path         // 旧路径
	NewPath       *Path         // 新路径
	StartTime     time.Time     // 开始时间
	Duration      time.Duration // 持续时间
	PacketBuffer  *PacketBuffer // 包缓冲区
	SeqTracker    *SeqTracker   // 序列号追踪器
}

// PacketBuffer 包缓冲区（用于去重）
type PacketBuffer struct {
	mu       sync.RWMutex
	buffer   map[uint64]*BufferedPacket // seq -> packet
	maxSize  int
	received int64
	deduped  int64
}

// BufferedPacket 缓冲的数据包
type BufferedPacket struct {
	Seq       uint64
	Data      []byte
	RecvTime  time.Time
	FromPath  string
	Delivered bool
}

// SeqTracker 序列号追踪器
type SeqTracker struct {
	mu           sync.RWMutex
	lastDelivered uint64
	window       map[uint64]bool // 已接收的序列号窗口
	windowSize   int
}

// NewPacketBuffer 创建包缓冲区
func NewPacketBuffer(maxSize int) *PacketBuffer {
	return &PacketBuffer{
		buffer:  make(map[uint64]*BufferedPacket),
		maxSize: maxSize,
	}
}

// NewSeqTracker 创建序列号追踪器
func NewSeqTracker(windowSize int) *SeqTracker {
	return &SeqTracker{
		window:     make(map[uint64]bool),
		windowSize: windowSize,
	}
}

// MultiPathBuffer G-Switch 转生期间的多路径缓冲器
type MultiPathBuffer struct {
	mu            sync.RWMutex
	scheduler     *PathScheduler
	dualMode      *DualSendMode
	packetBuffer  *PacketBuffer
	seqTracker    *SeqTracker
	
	// 配置
	dualModeDuration time.Duration // 双发模式持续时间
	bufferSize       int           // 缓冲区大小
	
	// 统计
	stats MultiPathStats
	
	// 回调
	OnPacketDelivered func(seq uint64, fromPath string)
	OnDualModeEnd     func(stats MultiPathStats)
}

// MultiPathStats 多路径统计
type MultiPathStats struct {
	TotalSent       int64         // 总发送数
	TotalReceived   int64         // 总接收数
	Deduplicated    int64         // 去重数
	OldPathPackets  int64         // 旧路径包数
	NewPathPackets  int64         // 新路径包数
	AvgLatencyOld   time.Duration // 旧路径平均延迟
	AvgLatencyNew   time.Duration // 新路径平均延迟
	SwitchSeamless  bool          // 切换是否无缝
}

// NewMultiPathBuffer 创建多路径缓冲器
func NewMultiPathBuffer(scheduler *PathScheduler) *MultiPathBuffer {
	return &MultiPathBuffer{
		scheduler:        scheduler,
		dualModeDuration: 100 * time.Millisecond, // 默认 100ms
		bufferSize:       1000,
		packetBuffer:     NewPacketBuffer(1000),
		seqTracker:       NewSeqTracker(100),
	}
}

// EnableDualSend 启用双发选收模式（G-Switch 触发时调用）
func (mpb *MultiPathBuffer) EnableDualSend(oldPath, newPath *Path) error {
	mpb.mu.Lock()
	defer mpb.mu.Unlock()
	
	if mpb.dualMode != nil && mpb.dualMode.Enabled {
		return fmt.Errorf("双发模式已启用")
	}
	
	mpb.dualMode = &DualSendMode{
		Enabled:      true,
		OldPath:      oldPath,
		NewPath:      newPath,
		StartTime:    time.Now(),
		Duration:     mpb.dualModeDuration,
		PacketBuffer: mpb.packetBuffer,
		SeqTracker:   mpb.seqTracker,
	}
	
	log.Printf("🔀 [G-Tunnel] 双发选收模式启用: %s + %s (duration=%v)", 
		oldPath.ID, newPath.ID, mpb.dualModeDuration)
	
	// 启动自动关闭定时器
	go mpb.autoDisableDualSend()
	
	return nil
}

// autoDisableDualSend 自动关闭双发模式
func (mpb *MultiPathBuffer) autoDisableDualSend() {
	time.Sleep(mpb.dualModeDuration)
	
	mpb.mu.Lock()
	defer mpb.mu.Unlock()
	
	if mpb.dualMode == nil || !mpb.dualMode.Enabled {
		return
	}
	
	mpb.dualMode.Enabled = false
	
	// 计算统计
	mpb.stats.SwitchSeamless = mpb.stats.Deduplicated > 0 && 
		mpb.stats.TotalReceived > mpb.stats.TotalSent/2
	
	log.Printf("🔀 [G-Tunnel] 双发选收模式结束: sent=%d, recv=%d, dedup=%d, seamless=%v",
		mpb.stats.TotalSent, mpb.stats.TotalReceived, mpb.stats.Deduplicated, mpb.stats.SwitchSeamless)
	
	if mpb.OnDualModeEnd != nil {
		mpb.OnDualModeEnd(mpb.stats)
	}
}

// SendDual 双发数据（同时通过新旧路径发送）
func (mpb *MultiPathBuffer) SendDual(seq uint64, data []byte) error {
	mpb.mu.RLock()
	defer mpb.mu.RUnlock()
	
	if mpb.dualMode == nil || !mpb.dualMode.Enabled {
		// 非双发模式，使用默认调度
		return mpb.scheduler.SendShard(data)
	}
	
	// 双发模式：同时发送到新旧路径
	var errOld, errNew error
	var wg sync.WaitGroup
	wg.Add(2)
	
	// 发送到旧路径
	go func() {
		defer wg.Done()
		if mpb.dualMode.OldPath != nil && mpb.dualMode.OldPath.IsActive {
			_, errOld = mpb.dualMode.OldPath.Conn.Write(data)
			if errOld == nil {
				mpb.dualMode.OldPath.PacketsSent++
			}
		}
	}()
	
	// 发送到新路径
	go func() {
		defer wg.Done()
		if mpb.dualMode.NewPath != nil && mpb.dualMode.NewPath.IsActive {
			_, errNew = mpb.dualMode.NewPath.Conn.Write(data)
			if errNew == nil {
				mpb.dualMode.NewPath.PacketsSent++
			}
		}
	}()
	
	wg.Wait()
	mpb.stats.TotalSent += 2
	
	// 只要有一条路径成功即可
	if errOld != nil && errNew != nil {
		return fmt.Errorf("双发失败: old=%v, new=%v", errOld, errNew)
	}
	
	return nil
}

// ReceiveAndDedupe 接收并去重（选收逻辑）
func (mpb *MultiPathBuffer) ReceiveAndDedupe(seq uint64, data []byte, fromPath string) ([]byte, bool) {
	mpb.mu.Lock()
	defer mpb.mu.Unlock()
	
	mpb.stats.TotalReceived++
	
	// 更新路径统计
	if mpb.dualMode != nil {
		if mpb.dualMode.OldPath != nil && fromPath == mpb.dualMode.OldPath.ID {
			mpb.stats.OldPathPackets++
		} else if mpb.dualMode.NewPath != nil && fromPath == mpb.dualMode.NewPath.ID {
			mpb.stats.NewPathPackets++
		}
	}
	
	// 检查是否已接收（去重）
	if mpb.seqTracker.HasReceived(seq) {
		mpb.stats.Deduplicated++
		return nil, false // 重复包，丢弃
	}
	
	// 标记为已接收
	mpb.seqTracker.MarkReceived(seq)
	
	// 缓存包
	mpb.packetBuffer.Add(seq, data, fromPath)
	
	// 回调
	if mpb.OnPacketDelivered != nil {
		mpb.OnPacketDelivered(seq, fromPath)
	}
	
	return data, true
}

// IsDualModeActive 检查双发模式是否激活
func (mpb *MultiPathBuffer) IsDualModeActive() bool {
	mpb.mu.RLock()
	defer mpb.mu.RUnlock()
	return mpb.dualMode != nil && mpb.dualMode.Enabled
}

// GetStats 获取统计信息
func (mpb *MultiPathBuffer) GetStats() MultiPathStats {
	mpb.mu.RLock()
	defer mpb.mu.RUnlock()
	return mpb.stats
}

// SetDualModeDuration 设置双发模式持续时间
func (mpb *MultiPathBuffer) SetDualModeDuration(d time.Duration) {
	mpb.mu.Lock()
	defer mpb.mu.Unlock()
	mpb.dualModeDuration = d
}

// --- PacketBuffer 方法 ---

// Add 添加包到缓冲区
func (pb *PacketBuffer) Add(seq uint64, data []byte, fromPath string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	
	if _, exists := pb.buffer[seq]; exists {
		pb.deduped++
		return
	}
	
	// 清理旧包（如果超过最大大小）
	if len(pb.buffer) >= pb.maxSize {
		pb.evictOldest()
	}
	
	pb.buffer[seq] = &BufferedPacket{
		Seq:      seq,
		Data:     data,
		RecvTime: time.Now(),
		FromPath: fromPath,
	}
	pb.received++
}

// evictOldest 清理最旧的包
func (pb *PacketBuffer) evictOldest() {
	var oldestSeq uint64
	var oldestTime time.Time
	first := true
	
	for seq, pkt := range pb.buffer {
		if first || pkt.RecvTime.Before(oldestTime) {
			oldestSeq = seq
			oldestTime = pkt.RecvTime
			first = false
		}
	}
	
	if !first {
		delete(pb.buffer, oldestSeq)
	}
}

// --- SeqTracker 方法 ---

// HasReceived 检查序列号是否已接收
func (st *SeqTracker) HasReceived(seq uint64) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.window[seq]
}

// MarkReceived 标记序列号为已接收
func (st *SeqTracker) MarkReceived(seq uint64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	
	st.window[seq] = true
	
	// 清理旧窗口
	if len(st.window) > st.windowSize {
		// 找到最小序列号并删除
		var minSeq uint64 = seq
		for s := range st.window {
			if s < minSeq {
				minSeq = s
			}
		}
		delete(st.window, minSeq)
	}
	
	// 更新最后交付序列号
	if seq > st.lastDelivered {
		st.lastDelivered = seq
	}
}
