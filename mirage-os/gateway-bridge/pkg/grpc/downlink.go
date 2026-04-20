// Package grpc - GatewayDownlink 下行推送服务（Desired State 模型）
package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "mirage-os/gateway-bridge/proto"
)

// DesiredState Gateway 期望状态（幂等对齐模型）
type DesiredState struct {
	DefenseLevel   int32             `json:"defense_level"`
	JitterMeanUs   uint32            `json:"jitter_mean_us"`
	JitterStddevUs uint32            `json:"jitter_stddev_us"`
	NoiseIntensity uint32            `json:"noise_intensity"`
	PaddingRate    uint32            `json:"padding_rate"`
	TemplateID     uint32            `json:"template_id"`
	RemainingBytes uint64            `json:"remaining_bytes"`
	Blacklist      []*BlacklistEntry `json:"blacklist,omitempty"`
	UpdatedAt      int64             `json:"updated_at"`
}

// BlacklistEntry 黑名单条目
type BlacklistEntry struct {
	CIDR     string `json:"cidr"`
	ExpireAt int64  `json:"expire_at"`
	Source   int32  `json:"source"`
}

// GatewayConnectionManager 管理到 Gateway 的 gRPC 连接
type GatewayConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]*grpc.ClientConn
	rdb         *goredis.Client
}

// NewGatewayConnectionManager 创建连接管理器
func NewGatewayConnectionManager(rdb *goredis.Client) *GatewayConnectionManager {
	return &GatewayConnectionManager{
		connections: make(map[string]*grpc.ClientConn),
		rdb:         rdb,
	}
}

// GetConn 获取到指定 Gateway 的连接
func (m *GatewayConnectionManager) GetConn(ctx context.Context, gatewayID string) (*grpc.ClientConn, error) {
	m.mu.RLock()
	if conn, ok := m.connections[gatewayID]; ok {
		m.mu.RUnlock()
		return conn, nil
	}
	m.mu.RUnlock()

	// 从 Redis 获取 Gateway 地址
	addr, err := m.rdb.Get(ctx, fmt.Sprintf("gateway:%s:addr", gatewayID)).Result()
	if err != nil {
		return nil, fmt.Errorf("gateway %s 地址未知: %w", gatewayID, err)
	}

	// 建立连接
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("连接 gateway %s 失败: %w", gatewayID, err)
	}

	m.mu.Lock()
	m.connections[gatewayID] = conn
	m.mu.Unlock()

	return conn, nil
}

// CloseConn 关闭指定 Gateway 的连接
func (m *GatewayConnectionManager) CloseConn(gatewayID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, ok := m.connections[gatewayID]; ok {
		conn.Close()
		delete(m.connections, gatewayID)
	}
}

// CloseAll 关闭所有连接
func (m *GatewayConnectionManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, conn := range m.connections {
		conn.Close()
		delete(m.connections, id)
	}
}

// DownlinkService 下行推送服务（Desired State 模型）
type DownlinkService struct {
	connMgr *GatewayConnectionManager
	rdb     *goredis.Client
}

// NewDownlinkService 创建下行服务
func NewDownlinkService(connMgr *GatewayConnectionManager, rdb *goredis.Client) *DownlinkService {
	return &DownlinkService{
		connMgr: connMgr,
		rdb:     rdb,
	}
}

// ComputeStateHash 计算状态哈希（SHA-256）
func ComputeStateHash(state *DesiredState) string {
	data, _ := json.Marshal(state)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]) // 取前 16 字节作为短哈希
}

// UpdateDesiredState 更新 Gateway 期望状态（覆盖式，幂等）
func (ds *DownlinkService) UpdateDesiredState(ctx context.Context, gatewayID string, state *DesiredState) error {
	state.UpdatedAt = time.Now().Unix()

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("序列化 desired state 失败: %w", err)
	}

	stateHash := ComputeStateHash(state)

	// 原子写入 state + hash
	pipe := ds.rdb.Pipeline()
	pipe.Set(ctx, fmt.Sprintf("gateway:%s:desired_state", gatewayID), stateJSON, 0)
	pipe.Set(ctx, fmt.Sprintf("gateway:%s:state_hash", gatewayID), stateHash, 0)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("写入 Redis 失败: %w", err)
	}

	log.Printf("[Downlink] 更新 Gateway %s 期望状态, hash=%s", gatewayID, stateHash)
	return nil
}

// GetDesiredState 获取 Gateway 期望状态
func (ds *DownlinkService) GetDesiredState(ctx context.Context, gatewayID string) (*DesiredState, string, error) {
	stateJSON, err := ds.rdb.Get(ctx, fmt.Sprintf("gateway:%s:desired_state", gatewayID)).Result()
	if err != nil {
		return nil, "", err
	}

	stateHash, _ := ds.rdb.Get(ctx, fmt.Sprintf("gateway:%s:state_hash", gatewayID)).Result()

	var state DesiredState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return nil, "", fmt.Errorf("反序列化 desired state 失败: %w", err)
	}

	return &state, stateHash, nil
}

// ReconcileState 状态对齐：对比 hash，不一致则通过 gRPC 下发全量状态
func (ds *DownlinkService) ReconcileState(ctx context.Context, gatewayID, currentHash string) (*DesiredState, bool, error) {
	expectedHash, err := ds.rdb.Get(ctx, fmt.Sprintf("gateway:%s:state_hash", gatewayID)).Result()
	if err != nil {
		// 无期望状态，无需对齐
		return nil, false, nil
	}

	if expectedHash == currentHash {
		// 状态一致，无需下发
		return nil, false, nil
	}

	// 状态不一致，读取全量 Desired State
	state, _, err := ds.GetDesiredState(ctx, gatewayID)
	if err != nil {
		return nil, false, err
	}

	log.Printf("[Downlink] 状态不一致 Gateway=%s, expected=%s, current=%s, 下发全量", gatewayID, expectedHash, currentHash)
	return state, true, nil
}

// PushBlacklist 更新黑名单到 Desired State
func (ds *DownlinkService) PushBlacklist(ctx context.Context, gatewayID string, entries []*pb.BlacklistEntryProto) error {
	state, _, err := ds.GetDesiredState(ctx, gatewayID)
	if err != nil {
		state = &DesiredState{}
	}

	state.Blacklist = make([]*BlacklistEntry, len(entries))
	for i, e := range entries {
		state.Blacklist[i] = &BlacklistEntry{
			CIDR:     e.GetCidr(),
			ExpireAt: e.GetExpireAt(),
			Source:   int32(e.GetSource()),
		}
	}

	return ds.UpdateDesiredState(ctx, gatewayID, state)
}

// PushStrategy 更新策略到 Desired State
func (ds *DownlinkService) PushStrategy(ctx context.Context, gatewayID string, strategy *pb.StrategyPush) error {
	state, _, err := ds.GetDesiredState(ctx, gatewayID)
	if err != nil {
		state = &DesiredState{}
	}

	state.DefenseLevel = strategy.GetDefenseLevel()
	state.JitterMeanUs = strategy.GetJitterMeanUs()
	state.JitterStddevUs = strategy.GetJitterStddevUs()
	state.NoiseIntensity = strategy.GetNoiseIntensity()
	state.PaddingRate = strategy.GetPaddingRate()
	state.TemplateID = strategy.GetTemplateId()

	return ds.UpdateDesiredState(ctx, gatewayID, state)
}

// PushQuota 更新配额到 Desired State
func (ds *DownlinkService) PushQuota(ctx context.Context, gatewayID string, remainingBytes uint64) error {
	state, _, err := ds.GetDesiredState(ctx, gatewayID)
	if err != nil {
		state = &DesiredState{}
	}

	state.RemainingBytes = remainingBytes
	return ds.UpdateDesiredState(ctx, gatewayID, state)
}

// PushReincarnation 推送转生指令（一次性事件，使用队列）
func (ds *DownlinkService) PushReincarnation(ctx context.Context, gatewayID string, push *pb.ReincarnationPush) error {
	event := map[string]interface{}{
		"type":      "reincarnation",
		"timestamp": time.Now().Unix(),
		"data": map[string]interface{}{
			"new_domain":       push.GetNewDomain(),
			"new_ip":           push.GetNewIp(),
			"reason":           push.GetReason(),
			"deadline_seconds": push.GetDeadlineSeconds(),
		},
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("序列化转生指令失败: %w", err)
	}

	eventsKey := fmt.Sprintf("mirage:downlink:events:%s", gatewayID)

	// 去重：检查队列中是否已有相同 new_domain 的转生指令
	existing, _ := ds.rdb.LRange(ctx, eventsKey, 0, -1).Result()
	for _, e := range existing {
		var existingEvent map[string]interface{}
		if json.Unmarshal([]byte(e), &existingEvent) == nil {
			if data, ok := existingEvent["data"].(map[string]interface{}); ok {
				if data["new_domain"] == push.NewDomain {
					return nil // 已存在，跳过
				}
			}
		}
	}

	// 入队，TTL 1 小时
	pipe := ds.rdb.Pipeline()
	pipe.RPush(ctx, eventsKey, eventJSON)
	pipe.Expire(ctx, eventsKey, 1*time.Hour)
	_, err = pipe.Exec(ctx)

	if err != nil {
		return fmt.Errorf("推送转生指令失败: %w", err)
	}

	log.Printf("[Downlink] 推送转生指令 Gateway=%s, domain=%s", gatewayID, push.NewDomain)
	return nil
}
