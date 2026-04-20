// Package wsgateway - Redis 多频道订阅器 + 节流窗口
// 订阅 3 个频道，200ms 窗口合并同类消息，防止前端 Render Thrashing
package wsgateway

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// 订阅频道
const (
	ChannelHeartbeat = "mirage:gateway:heartbeat"
	ChannelThreat    = "mirage:threat:events"
	ChannelTunnel    = "mirage:tunnel:status"
)

// ThrottledSubscriber 节流订阅器
type ThrottledSubscriber struct {
	rdb        *redis.Client
	hub        *Hub
	windowMs   time.Duration
	mu         sync.Mutex
	heartbeats map[string]json.RawMessage // gateway_id → latest heartbeat
	threats    []json.RawMessage          // 窗口内累积的威胁事件
	tunnels    map[string]json.RawMessage // gateway_id → latest tunnel status
}

// NewThrottledSubscriber 创建节流订阅器
func NewThrottledSubscriber(rdb *redis.Client, hub *Hub) *ThrottledSubscriber {
	return &ThrottledSubscriber{
		rdb:        rdb,
		hub:        hub,
		windowMs:   200 * time.Millisecond,
		heartbeats: make(map[string]json.RawMessage),
		threats:    make([]json.RawMessage, 0),
		tunnels:    make(map[string]json.RawMessage),
	}
}

// Start 启动多频道订阅 + 节流刷新
func (ts *ThrottledSubscriber) Start(ctx context.Context) {
	// 订阅 3 个频道
	pubsub := ts.rdb.Subscribe(ctx, ChannelHeartbeat, ChannelThreat, ChannelTunnel)
	defer pubsub.Close()

	log.Printf("📡 [Redis] 已订阅频道: %s, %s, %s", ChannelHeartbeat, ChannelThreat, ChannelTunnel)

	// 启动节流刷新循环
	go ts.flushLoop(ctx)

	// 消费消息
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			ts.ingest(msg.Channel, msg.Payload)
		}
	}
}

// ingest 摄入消息到窗口缓冲区
func (ts *ThrottledSubscriber) ingest(channel, payload string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	switch channel {
	case ChannelHeartbeat:
		// 按 gateway_id 去重，只保留最新
		var hb struct {
			GatewayID string `json:"gateway_id"`
		}
		if json.Unmarshal([]byte(payload), &hb) == nil && hb.GatewayID != "" {
			ts.heartbeats[hb.GatewayID] = json.RawMessage(payload)
		}

	case ChannelThreat:
		// 累积（窗口内合并）
		ts.threats = append(ts.threats, json.RawMessage(payload))

	case ChannelTunnel:
		// 按 gateway_id 去重
		var t struct {
			GatewayID string `json:"gateway_id"`
		}
		if json.Unmarshal([]byte(payload), &t) == nil && t.GatewayID != "" {
			ts.tunnels[t.GatewayID] = json.RawMessage(payload)
		}
	}
}

// flushLoop 200ms 节流刷新
func (ts *ThrottledSubscriber) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(ts.windowMs)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ts.flush()
		}
	}
}

// flush 将缓冲区内容推送到 Hub
func (ts *ThrottledSubscriber) flush() {
	ts.mu.Lock()

	// 快照并清空
	heartbeats := ts.heartbeats
	threats := ts.threats
	tunnels := ts.tunnels

	ts.heartbeats = make(map[string]json.RawMessage)
	ts.threats = make([]json.RawMessage, 0)
	ts.tunnels = make(map[string]json.RawMessage)

	ts.mu.Unlock()

	// 推送心跳（每个 gateway 一条）
	for _, hb := range heartbeats {
		msg := wrapMessage("heartbeat", hb)
		ts.hub.broadcast <- msg
	}

	// 推送威胁（合并为数组）
	if len(threats) > 0 {
		// 限制单次推送最多 20 条
		if len(threats) > 20 {
			threats = threats[len(threats)-20:]
		}
		batch := map[string]interface{}{
			"count":  len(threats),
			"events": threats,
		}
		msg := wrapMessage("threat", batch)
		ts.hub.broadcast <- msg
	}

	// 推送隧道状态
	for _, t := range tunnels {
		msg := wrapMessage("tunnel", t)
		ts.hub.broadcast <- msg
	}
}

// wrapMessage 包装为标准 WS 消息格式
func wrapMessage(msgType string, data interface{}) []byte {
	msg := map[string]interface{}{
		"type":      msgType,
		"timestamp": time.Now().Unix(),
		"data":      data,
	}
	b, _ := json.Marshal(msg)
	return b
}

// SendSnapshot 发送全量快照（冷启动时调用）
func (ts *ThrottledSubscriber) SendSnapshot(client *Client) {
	ctx := context.Background()

	// 从 Redis 拉取所有 gateway 当前状态
	keys, err := ts.rdb.Keys(ctx, "gateway:*:status").Result()
	if err != nil {
		log.Printf("⚠️ [Snapshot] 拉取 gateway 状态失败: %v", err)
		return
	}

	gateways := make([]json.RawMessage, 0, len(keys))
	for _, key := range keys {
		val, err := ts.rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		gateways = append(gateways, json.RawMessage(val))
	}

	// 打包为 snapshot 消息
	snapshot := map[string]interface{}{
		"type":      "system",
		"timestamp": time.Now().Unix(),
		"data": map[string]interface{}{
			"action":   "snapshot",
			"gateways": gateways,
		},
	}
	b, _ := json.Marshal(snapshot)

	select {
	case client.send <- b:
		log.Printf("📸 [Snapshot] 已发送全量快照: %d 个 gateway", len(gateways))
	default:
		log.Println("⚠️ [Snapshot] 客户端缓冲区满，跳过快照")
	}
}
