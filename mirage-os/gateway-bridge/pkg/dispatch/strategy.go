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
func (sd *StrategyDispatcher) PushStrategyToCell(cellID string, strategy *pb.StrategyPush) error {
	ctx := context.Background()
	// 从 Redis 获取该 cell 下在线 gateway
	pattern := fmt.Sprintf("gateway:*:cell")
	iter := sd.rdb.Scan(ctx, 0, pattern, 100).Iterator()

	sd.mu.RLock()
	defer sd.mu.RUnlock()

	var lastErr error
	for iter.Next(ctx) {
		key := iter.Val()
		val, err := sd.rdb.Get(ctx, key).Result()
		if err != nil || val != cellID {
			continue
		}
		// 提取 gateway_id
		gwID := extractGatewayID(key)
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
func (sd *StrategyDispatcher) PushBlacklistToAll(entries []*pb.BlacklistEntryProto) error {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	ctx := context.Background()
	push := &pb.BlacklistPush{Entries: entries}

	for gwID, gc := range sd.connections {
		_, err := gc.client.PushBlacklist(ctx, push)
		if err != nil {
			log.Printf("[WARN] push blacklist to %s failed: %v", gwID, err)
		}
	}
	return nil
}

// PushQuotaToGateway 向指定 Gateway 推送配额
func (sd *StrategyDispatcher) PushQuotaToGateway(gatewayID string, remainingBytes uint64) error {
	sd.mu.RLock()
	gc, ok := sd.connections[gatewayID]
	sd.mu.RUnlock()

	if !ok {
		return fmt.Errorf("gateway %s not connected", gatewayID)
	}

	ctx := context.Background()
	_, err := gc.client.PushQuota(ctx, &pb.QuotaPush{RemainingBytes: remainingBytes})
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
