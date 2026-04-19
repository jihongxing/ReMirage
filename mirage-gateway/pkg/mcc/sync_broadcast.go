// Package mcc - M.C.C. 同步广播模块
// 实现 G-Switch 切换指令的全网广播，确保分布式一致性
package mcc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// SyncMessageType 同步消息类型
type SyncMessageType int

const (
	MsgGSwitchRotate    SyncMessageType = 1 // 域名切换
	MsgBDNAReset        SyncMessageType = 2 // B-DNA 重置
	MsgThreatBroadcast  SyncMessageType = 3 // 威胁广播
	MsgQuarantineDomain SyncMessageType = 4 // 域名隔离
	MsgEmergencyWipe    SyncMessageType = 5 // 紧急擦除
)

// SyncMessage 同步消息
type SyncMessage struct {
	ID        string          `json:"id"`
	Type      SyncMessageType `json:"type"`
	Timestamp int64           `json:"timestamp"`
	SourceNode string         `json:"source_node"`
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
	TTL       int             `json:"ttl"` // 跳数限制
}

// GSwitchPayload G-Switch 切换载荷
type GSwitchPayload struct {
	OldDomain       string  `json:"old_domain"`
	NewDomain       string  `json:"new_domain"`
	Reason          string  `json:"reason"`
	ReputationScore float64 `json:"reputation_score"`
	ForceJA4Reset   bool    `json:"force_ja4_reset"`
}

// BDNAResetPayload B-DNA 重置载荷
type BDNAResetPayload struct {
	TemplateID   uint32 `json:"template_id"`
	Reason       string `json:"reason"`
	AffectedIPRange string `json:"affected_ip_range"`
}

// ThreatPayload 威胁广播载荷
type ThreatPayload struct {
	ThreatType    string   `json:"threat_type"`
	Indicators    []string `json:"indicators"`
	Severity      int      `json:"severity"`
	Countermeasure string  `json:"countermeasure"`
}

// PeerNode 对等节点
type PeerNode struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"` // .onion 地址或直连地址
	Region   string `json:"region"`
	LastSeen int64  `json:"last_seen"`
	Latency  int64  `json:"latency_ms"`
	IsAlive  bool   `json:"is_alive"`
}

// SyncBroadcaster 同步广播器
type SyncBroadcaster struct {
	mu sync.RWMutex

	// 节点信息
	nodeID    string
	nodeKey   []byte // 签名密钥

	// 对等节点
	peers map[string]*PeerNode

	// 消息去重
	seenMessages map[string]int64 // messageID -> timestamp
	seenTTL      time.Duration

	// 消息队列
	outbound chan *SyncMessage
	inbound  chan *SyncMessage

	// 回调
	onGSwitchReceived func(payload *GSwitchPayload)
	onBDNAResetReceived func(payload *BDNAResetPayload)
	onThreatReceived func(payload *ThreatPayload)

	// 统计
	stats struct {
		MessagesSent     uint64
		MessagesReceived uint64
		MessagesDropped  uint64
		BroadcastLatency int64 // 平均广播延迟 (ms)
	}

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSyncBroadcaster 创建同步广播器
func NewSyncBroadcaster(nodeID string, nodeKey []byte) *SyncBroadcaster {
	ctx, cancel := context.WithCancel(context.Background())

	return &SyncBroadcaster{
		nodeID:       nodeID,
		nodeKey:      nodeKey,
		peers:        make(map[string]*PeerNode),
		seenMessages: make(map[string]int64),
		seenTTL:      5 * time.Minute,
		outbound:     make(chan *SyncMessage, 100),
		inbound:      make(chan *SyncMessage, 100),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// SetCallbacks 设置回调
func (sb *SyncBroadcaster) SetCallbacks(
	onGSwitch func(*GSwitchPayload),
	onBDNAReset func(*BDNAResetPayload),
	onThreat func(*ThreatPayload),
) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.onGSwitchReceived = onGSwitch
	sb.onBDNAResetReceived = onBDNAReset
	sb.onThreatReceived = onThreat
}

// Start 启动广播器
func (sb *SyncBroadcaster) Start() {
	sb.wg.Add(3)
	go sb.outboundLoop()
	go sb.inboundLoop()
	go sb.cleanupLoop()

	log.Printf("📡 M.C.C. 同步广播器已启动 (node=%s)", sb.nodeID)
}

// Stop 停止广播器
func (sb *SyncBroadcaster) Stop() {
	sb.cancel()
	sb.wg.Wait()
	log.Println("🛑 M.C.C. 同步广播器已停止")
}

// AddPeer 添加对等节点
func (sb *SyncBroadcaster) AddPeer(id, endpoint, region string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	sb.peers[id] = &PeerNode{
		ID:       id,
		Endpoint: endpoint,
		Region:   region,
		LastSeen: time.Now().Unix(),
		IsAlive:  true,
	}
	log.Printf("🔗 添加对等节点: %s (%s)", id, region)
}

// RemovePeer 移除对等节点
func (sb *SyncBroadcaster) RemovePeer(id string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	delete(sb.peers, id)
}

// BroadcastGSwitch 广播 G-Switch 切换
func (sb *SyncBroadcaster) BroadcastGSwitch(payload *GSwitchPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	msg := sb.createMessage(MsgGSwitchRotate, data)
	sb.outbound <- msg

	log.Printf("📤 广播 G-Switch: %s → %s", payload.OldDomain, payload.NewDomain)
	return nil
}

// BroadcastBDNAReset 广播 B-DNA 重置
func (sb *SyncBroadcaster) BroadcastBDNAReset(payload *BDNAResetPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	msg := sb.createMessage(MsgBDNAReset, data)
	sb.outbound <- msg

	log.Printf("📤 广播 B-DNA Reset: template=%d", payload.TemplateID)
	return nil
}

// BroadcastThreat 广播威胁情报
func (sb *SyncBroadcaster) BroadcastThreat(payload *ThreatPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	msg := sb.createMessage(MsgThreatBroadcast, data)
	sb.outbound <- msg

	log.Printf("📤 广播威胁情报: %s (severity=%d)", payload.ThreatType, payload.Severity)
	return nil
}

// createMessage 创建消息
func (sb *SyncBroadcaster) createMessage(msgType SyncMessageType, payload []byte) *SyncMessage {
	msg := &SyncMessage{
		ID:         sb.generateMessageID(),
		Type:       msgType,
		Timestamp:  time.Now().UnixNano(),
		SourceNode: sb.nodeID,
		Payload:    payload,
		TTL:        3, // 最多转发 3 跳
	}
	msg.Signature = sb.signMessage(msg)
	return msg
}

// generateMessageID 生成消息 ID
func (sb *SyncBroadcaster) generateMessageID() string {
	data := fmt.Sprintf("%s-%d-%d", sb.nodeID, time.Now().UnixNano(), len(sb.seenMessages))
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// signMessage 签名消息
func (sb *SyncBroadcaster) signMessage(msg *SyncMessage) string {
	data := fmt.Sprintf("%s:%d:%d:%s", msg.ID, msg.Type, msg.Timestamp, msg.SourceNode)
	hash := sha256.Sum256(append([]byte(data), sb.nodeKey...))
	return hex.EncodeToString(hash[:16])
}

// outboundLoop 出站消息循环
func (sb *SyncBroadcaster) outboundLoop() {
	defer sb.wg.Done()

	for {
		select {
		case <-sb.ctx.Done():
			return
		case msg := <-sb.outbound:
			sb.broadcastToPeers(msg)
		}
	}
}

// broadcastToPeers 广播到所有对等节点
func (sb *SyncBroadcaster) broadcastToPeers(msg *SyncMessage) {
	sb.mu.RLock()
	peers := make([]*PeerNode, 0, len(sb.peers))
	for _, p := range sb.peers {
		if p.IsAlive {
			peers = append(peers, p)
		}
	}
	sb.mu.RUnlock()

	startTime := time.Now()
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for _, peer := range peers {
		wg.Add(1)
		go func(p *PeerNode) {
			defer wg.Done()
			if err := sb.sendToPeer(p, msg); err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(peer)
	}

	wg.Wait()

	// 更新统计
	sb.mu.Lock()
	sb.stats.MessagesSent++
	sb.stats.BroadcastLatency = time.Since(startTime).Milliseconds()
	sb.mu.Unlock()

	log.Printf("📡 广播完成: %d/%d 节点成功 (latency=%dms)",
		successCount, len(peers), time.Since(startTime).Milliseconds())
}

// sendToPeer 发送到单个节点 (模拟实现)
func (sb *SyncBroadcaster) sendToPeer(peer *PeerNode, msg *SyncMessage) error {
	// 实际实现应通过 Tor 隐藏服务或加密通道发送
	// 这里模拟网络延迟
	time.Sleep(time.Duration(10+peer.Latency/10) * time.Millisecond)

	// 更新节点状态
	sb.mu.Lock()
	if p, ok := sb.peers[peer.ID]; ok {
		p.LastSeen = time.Now().Unix()
	}
	sb.mu.Unlock()

	return nil
}

// inboundLoop 入站消息循环
func (sb *SyncBroadcaster) inboundLoop() {
	defer sb.wg.Done()

	for {
		select {
		case <-sb.ctx.Done():
			return
		case msg := <-sb.inbound:
			sb.handleInboundMessage(msg)
		}
	}
}

// handleInboundMessage 处理入站消息
func (sb *SyncBroadcaster) handleInboundMessage(msg *SyncMessage) {
	// 去重检查
	sb.mu.Lock()
	if _, seen := sb.seenMessages[msg.ID]; seen {
		sb.stats.MessagesDropped++
		sb.mu.Unlock()
		return
	}
	sb.seenMessages[msg.ID] = time.Now().Unix()
	sb.stats.MessagesReceived++
	sb.mu.Unlock()

	// 处理消息
	switch msg.Type {
	case MsgGSwitchRotate:
		sb.handleGSwitchMessage(msg)
	case MsgBDNAReset:
		sb.handleBDNAResetMessage(msg)
	case MsgThreatBroadcast:
		sb.handleThreatMessage(msg)
	}

	// 转发 (TTL > 0)
	if msg.TTL > 1 {
		msg.TTL--
		sb.outbound <- msg
	}
}

// handleGSwitchMessage 处理 G-Switch 消息
func (sb *SyncBroadcaster) handleGSwitchMessage(msg *SyncMessage) {
	var payload GSwitchPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Printf("⚠️  解析 G-Switch 消息失败: %v", err)
		return
	}

	log.Printf("📥 收到 G-Switch 广播: %s → %s (from=%s)",
		payload.OldDomain, payload.NewDomain, msg.SourceNode)

	if sb.onGSwitchReceived != nil {
		sb.onGSwitchReceived(&payload)
	}
}

// handleBDNAResetMessage 处理 B-DNA Reset 消息
func (sb *SyncBroadcaster) handleBDNAResetMessage(msg *SyncMessage) {
	var payload BDNAResetPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Printf("⚠️  解析 B-DNA Reset 消息失败: %v", err)
		return
	}

	log.Printf("📥 收到 B-DNA Reset 广播: template=%d (from=%s)",
		payload.TemplateID, msg.SourceNode)

	if sb.onBDNAResetReceived != nil {
		sb.onBDNAResetReceived(&payload)
	}
}

// handleThreatMessage 处理威胁消息
func (sb *SyncBroadcaster) handleThreatMessage(msg *SyncMessage) {
	var payload ThreatPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Printf("⚠️  解析威胁消息失败: %v", err)
		return
	}

	log.Printf("📥 收到威胁广播: %s severity=%d (from=%s)",
		payload.ThreatType, payload.Severity, msg.SourceNode)

	if sb.onThreatReceived != nil {
		sb.onThreatReceived(&payload)
	}
}

// cleanupLoop 清理循环
func (sb *SyncBroadcaster) cleanupLoop() {
	defer sb.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sb.ctx.Done():
			return
		case <-ticker.C:
			sb.cleanupSeenMessages()
			sb.checkPeerHealth()
		}
	}
}

// cleanupSeenMessages 清理已见消息
func (sb *SyncBroadcaster) cleanupSeenMessages() {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	cutoff := time.Now().Add(-sb.seenTTL).Unix()
	for id, ts := range sb.seenMessages {
		if ts < cutoff {
			delete(sb.seenMessages, id)
		}
	}
}

// checkPeerHealth 检查节点健康
func (sb *SyncBroadcaster) checkPeerHealth() {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute).Unix()
	for _, peer := range sb.peers {
		peer.IsAlive = peer.LastSeen > cutoff
	}
}

// ReceiveMessage 接收外部消息 (供外部调用)
func (sb *SyncBroadcaster) ReceiveMessage(msg *SyncMessage) {
	select {
	case sb.inbound <- msg:
	default:
		sb.mu.Lock()
		sb.stats.MessagesDropped++
		sb.mu.Unlock()
	}
}

// GetStats 获取统计信息
func (sb *SyncBroadcaster) GetStats() map[string]interface{} {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	return map[string]interface{}{
		"messages_sent":     sb.stats.MessagesSent,
		"messages_received": sb.stats.MessagesReceived,
		"messages_dropped":  sb.stats.MessagesDropped,
		"broadcast_latency": sb.stats.BroadcastLatency,
		"peer_count":        len(sb.peers),
		"seen_cache_size":   len(sb.seenMessages),
	}
}

// GetPeers 获取对等节点列表
func (sb *SyncBroadcaster) GetPeers() []*PeerNode {
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	result := make([]*PeerNode, 0, len(sb.peers))
	for _, p := range sb.peers {
		result = append(result, p)
	}
	return result
}
