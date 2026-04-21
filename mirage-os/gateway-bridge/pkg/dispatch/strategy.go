package dispatch

import (
	"context"
	"fmt"
	"log"
	"sync"

	pb "mirage-proto/gen"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type grpcConn struct {
	conn   *grpc.ClientConn
	client pb.GatewayDownlinkClient
	addr   string
}

type StrategyDispatcher struct {
	rdb         *redis.Client
	connections map[string]*grpcConn
	mu          sync.RWMutex
	pendingPush map[string]*pb.StrategyPush
	registry    Registry
}

// Registry 拓扑查询接口（解耦 topology 包依赖）
type Registry interface {
	GetGatewaysByCell(cellID string) []*GatewayInfoRef
	GetAllOnline() []*GatewayInfoRef
}

// GatewayInfoRef 拓扑查询结果引用（避免直接依赖 topology.GatewayInfo）
type GatewayInfoRef struct {
	GatewayID string
}

// SetRegistry 设置拓扑索引（启动时注入）
func (sd *StrategyDispatcher) SetRegistry(r Registry) {
	sd.registry = r
}

func NewStrategyDispatcher(rdb *redis.Client) *StrategyDispatcher {
	return &StrategyDispatcher{
		rdb:         rdb,
		connections: make(map[string]*grpcConn),
		pendingPush: make(map[string]*pb.StrategyPush),
	}
}

// RegisterGateway 注册 Gateway 下行连接
func (sd *StrategyDispatcher) RegisterGateway(gatewayID, downlinkAddr string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if existing, ok := sd.connections[gatewayID]; ok {
		if existing.addr == downlinkAddr {
			return nil
		}
		existing.conn.Close()
	}

	conn, err := grpc.NewClient(downlinkAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial gateway %s: %w", gatewayID, err)
	}

	sd.connections[gatewayID] = &grpcConn{
		conn:   conn,
		client: pb.NewGatewayDownlinkClient(conn),
		addr:   downlinkAddr,
	}
	return nil
}

// PushStrategyToCell 向蜂窝下所有在线 Gateway 推送策略
// 优先通过 Registry.GetGatewaysByCell 查询目标列表，fallback 到 Redis SCAN
func (sd *StrategyDispatcher) PushStrategyToCell(cellID string, strategy *pb.StrategyPush) error {
	ctx := context.Background()

	var targets []string
	if sd.registry != nil {
		gws := sd.registry.GetGatewaysByCell(cellID)
		for _, gw := range gws {
			targets = append(targets, gw.GatewayID)
		}
	}

	// fallback: Redis SCAN（registry 未设置或无结果时）
	if len(targets) == 0 && sd.registry == nil {
		pattern := fmt.Sprintf("gateway:*:cell")
		iter := sd.rdb.Scan(ctx, 0, pattern, 100).Iterator()
		for iter.Next(ctx) {
			key := iter.Val()
			val, err := sd.rdb.Get(ctx, key).Result()
			if err != nil || val != cellID {
				continue
			}
			targets = append(targets, extractGatewayID(key))
		}
	}

	sd.mu.RLock()
	defer sd.mu.RUnlock()

	var lastErr error
	for _, gwID := range targets {
		if gc, ok := sd.connections[gwID]; ok {
			_, err := gc.client.PushStrategy(ctx, strategy)
			if err != nil {
				log.Printf("[WARN] push strategy to %s failed: %v", gwID, err)
				sd.pendingPush[gwID] = strategy
				lastErr = err
			}
		}
	}
	return lastErr
}

// PushBlacklistToAll 向所有在线 Gateway 推送黑名单
// 优先通过 Registry.GetAllOnline 查询目标列表，fallback 到遍历 connections map
func (sd *StrategyDispatcher) PushBlacklistToAll(entries []*pb.BlacklistEntryProto) error {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	ctx := context.Background()
	push := &pb.BlacklistPush{Entries: entries}

	if sd.registry != nil {
		gws := sd.registry.GetAllOnline()
		for _, gw := range gws {
			if gc, ok := sd.connections[gw.GatewayID]; ok {
				_, err := gc.client.PushBlacklist(ctx, push)
				if err != nil {
					log.Printf("[WARN] push blacklist to %s failed: %v", gw.GatewayID, err)
				}
			}
		}
	} else {
		// fallback: 遍历所有连接
		for gwID, gc := range sd.connections {
			_, err := gc.client.PushBlacklist(ctx, push)
			if err != nil {
				log.Printf("[WARN] push blacklist to %s failed: %v", gwID, err)
			}
		}
	}
	return nil
}

// PushQuotaToGateway 向指定 Gateway 推送配额（按用户维度）
func (sd *StrategyDispatcher) PushQuotaToGateway(gatewayID string, remainingBytes uint64) error {
	return sd.PushQuotaToGatewayForUser(gatewayID, "", remainingBytes)
}

// PushQuotaToGatewayForUser 向指定 Gateway 推送指定用户的配额
func (sd *StrategyDispatcher) PushQuotaToGatewayForUser(gatewayID, userID string, remainingBytes uint64) error {
	sd.mu.RLock()
	gc, ok := sd.connections[gatewayID]
	sd.mu.RUnlock()

	if !ok {
		return fmt.Errorf("gateway %s not connected", gatewayID)
	}

	ctx := context.Background()
	_, err := gc.client.PushQuota(ctx, &pb.QuotaPush{RemainingBytes: remainingBytes, UserId: userID})
	return err
}

// RetryPending 重试待推送的策略
func (sd *StrategyDispatcher) RetryPending(gatewayID string) error {
	sd.mu.Lock()
	strategy, ok := sd.pendingPush[gatewayID]
	if !ok {
		sd.mu.Unlock()
		return nil
	}
	delete(sd.pendingPush, gatewayID)
	gc, hasConn := sd.connections[gatewayID]
	sd.mu.Unlock()

	if !hasConn {
		return fmt.Errorf("gateway %s not connected", gatewayID)
	}

	ctx := context.Background()
	_, err := gc.client.PushStrategy(ctx, strategy)
	return err
}

func extractGatewayID(key string) string {
	// key format: gateway:{id}:cell
	if len(key) > 12 {
		end := len(key) - 5 // remove ":cell"
		if end > 8 {
			return key[8:end]
		}
	}
	return ""
}
