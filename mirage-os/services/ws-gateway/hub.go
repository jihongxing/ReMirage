// Package wsgateway - WebSocket 实时推送中枢
package wsgateway

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Hub 管理所有 WebSocket 连接
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	rdb        *redis.Client
}

// Client WebSocket 客户端
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// NewHub 创建 Hub
func NewHub(rdb *redis.Client) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		rdb:        rdb,
	}
}

// Run 启动 Hub
func (h *Hub) Run() {
	// 启动 Redis 订阅
	go h.subscribeRedis()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("📡 [WS] 新客户端连接，当前连接数: %d", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("📡 [WS] 客户端断开，当前连接数: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// subscribeRedis 订阅 Redis 频道
func (h *Hub) subscribeRedis() {
	ctx := context.Background()
	
	// 订阅全局频道
	pubsub := h.rdb.Subscribe(ctx, "mirage:events:all")
	defer pubsub.Close()

	log.Println("📡 [Redis] 已订阅频道: mirage:events:all")

	ch := pubsub.Channel()
	for msg := range ch {
		// 广播到所有 WebSocket 客户端
		h.broadcast <- []byte(msg.Payload)
	}
}

// HandleWS 处理 WebSocket 连接
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ [WS] 升级失败: %v", err)
		return
	}

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	client.hub.register <- client

	// 启动读写协程
	go client.writePump()
	go client.readPump()
}

// readPump 读取客户端消息（处理战术指令）
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("⚠️  [WS] 连接异常关闭: %v", err)
			}
			break
		}

		// 解析并处理指令
		c.hub.handleCommand(message)
	}
}

// CommandMessage 指令消息
type CommandMessage struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

// TacticalCommand 战术指令
type TacticalCommand struct {
	Mode      int    `json:"mode"`      // 0=Normal, 1=Sleep, 2=Aggressive, 3=Stealth
	GatewayID string `json:"gatewayId"` // 空=全局
}

// GhostModeCommand Ghost Mode 指令
type GhostModeCommand struct {
	Enabled bool `json:"enabled"`
}

// GatewayConfigCommand 网关配置指令
type GatewayConfigCommand struct {
	GatewayID       string `json:"gatewayId"`
	SocialJitter    int    `json:"socialJitter"`
	CIDRotationRate int    `json:"cidRotationRate"`
	FECRedundancy   int    `json:"fecRedundancy"`
}

// SelfDestructCommand 自毁指令
type SelfDestructCommand struct {
	ConfirmCode string `json:"confirmCode"`
	WipeMemory  bool   `json:"wipeMemory"`
	WipeEBPF    bool   `json:"wipeEbpf"`
	WipeLogs    bool   `json:"wipeLogs"`
}

// handleCommand 处理客户端指令
func (h *Hub) handleCommand(message []byte) {
	var cmd CommandMessage
	if err := json.Unmarshal(message, &cmd); err != nil {
		log.Printf("⚠️  [WS] 解析指令失败: %v", err)
		return
	}

	ctx := context.Background()

	switch cmd.Type {
	case "tactical:update":
		var data TacticalCommand
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			log.Printf("⚠️  [WS] 解析战术指令失败: %v", err)
			return
		}
		h.handleTacticalUpdate(ctx, &data)

	case "ghost_mode:toggle":
		var data GhostModeCommand
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			log.Printf("⚠️  [WS] 解析 Ghost Mode 指令失败: %v", err)
			return
		}
		h.handleGhostModeToggle(ctx, &data)

	case "gateway:config":
		var data GatewayConfigCommand
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			log.Printf("⚠️  [WS] 解析网关配置指令失败: %v", err)
			return
		}
		h.handleGatewayConfig(ctx, &data)

	case "self_destruct":
		var data SelfDestructCommand
		if err := json.Unmarshal(cmd.Data, &data); err != nil {
			log.Printf("⚠️  [WS] 解析自毁指令失败: %v", err)
			return
		}
		h.handleSelfDestruct(ctx, &data)

	default:
		log.Printf("⚠️  [WS] 未知指令类型: %s", cmd.Type)
	}
}

// handleTacticalUpdate 处理战术模式更新
func (h *Hub) handleTacticalUpdate(ctx context.Context, cmd *TacticalCommand) {
	log.Printf("🎯 [Tactical] 收到战术模式更新: mode=%d, gateway=%s", cmd.Mode, cmd.GatewayID)

	// 发布到 Redis，由 Raft Leader 处理
	event := map[string]interface{}{
		"mode":      cmd.Mode,
		"gatewayId": cmd.GatewayID,
	}
	h.rdb.Publish(ctx, "mirage:commands:tactical", mustMarshal(event))

	// 广播确认
	h.broadcastAck("tactical:updated", map[string]interface{}{
		"mode":      cmd.Mode,
		"gatewayId": cmd.GatewayID,
		"status":    "pending",
	})
}

// handleGhostModeToggle 处理 Ghost Mode 切换
func (h *Hub) handleGhostModeToggle(ctx context.Context, cmd *GhostModeCommand) {
	log.Printf("👻 [GhostMode] 切换: enabled=%v", cmd.Enabled)

	// 发布到 Redis
	h.rdb.Publish(ctx, "mirage:commands:ghost_mode", mustMarshal(cmd))

	// 广播确认
	h.broadcastAck("ghost_mode:toggled", map[string]interface{}{
		"enabled": cmd.Enabled,
	})
}

// handleGatewayConfig 处理网关配置
func (h *Hub) handleGatewayConfig(ctx context.Context, cmd *GatewayConfigCommand) {
	log.Printf("🌐 [Gateway] 配置更新: gateway=%s", cmd.GatewayID)

	// 发布到 Redis
	h.rdb.Publish(ctx, "mirage:commands:gateway_config", mustMarshal(cmd))

	// 广播确认
	h.broadcastAck("gateway:configured", map[string]interface{}{
		"gatewayId": cmd.GatewayID,
		"status":    "applied",
	})
}

// handleSelfDestruct 处理自毁指令
func (h *Hub) handleSelfDestruct(ctx context.Context, cmd *SelfDestructCommand) {
	log.Printf("💀 [SelfDestruct] 收到自毁指令")

	// 验证确认码
	if cmd.ConfirmCode != "MIRAGE-DESTRUCT-CONFIRM" {
		h.broadcastAck("self_destruct:rejected", map[string]interface{}{
			"reason": "invalid_confirm_code",
		})
		return
	}

	// 发布到 Redis（所有节点执行）
	h.rdb.Publish(ctx, "mirage:commands:self_destruct", mustMarshal(cmd))

	// 广播确认
	h.broadcastAck("self_destruct:initiated", map[string]interface{}{
		"status": "executing",
	})
}

// broadcastAck 广播确认消息
func (h *Hub) broadcastAck(eventType string, data interface{}) {
	event := map[string]interface{}{
		"type":      eventType,
		"timestamp": time.Now().Unix(),
		"data":      data,
	}
	payload, _ := json.Marshal(event)
	h.broadcast <- payload
}

// mustMarshal JSON 序列化
func mustMarshal(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// writePump 向客户端写入消息
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 批量发送队列中的消息
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// PublishEvent 发布事件到 Redis（供其他服务调用）
func PublishEvent(rdb *redis.Client, eventType string, data interface{}) error {
	ctx := context.Background()

	event := map[string]interface{}{
		"type":      eventType,
		"timestamp": time.Now().Unix(),
		"data":      data,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return rdb.Publish(ctx, "mirage:events:all", payload).Err()
}
