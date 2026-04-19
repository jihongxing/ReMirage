// Package cortex 幻影一致性
// 确保同一指纹看到完全一致的虚拟世界
package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ShadowContext 影子上下文
type ShadowContext struct {
	UID              string    `json:"uid"`
	AssignedTemplate string    `json:"assignedTemplate"`
	Seed             int64     `json:"seed"`
	FirstSeen        time.Time `json:"firstSeen"`
	LastSeen         time.Time `json:"lastSeen"`
	Region           string    `json:"region"`
	RegionBias       string    `json:"regionBias"`
	MazeDepth        int       `json:"mazeDepth"`
	RequestCount     int64     `json:"requestCount"`
}

// ContextPersistence 上下文持久化管理器
type ContextPersistence struct {
	mu sync.RWMutex

	// 内存缓存（生产环境应使用 Redis）
	contexts map[string]*ShadowContext

	// Redis 客户端接口
	redis RedisClient

	// 配置
	config PersistenceConfig

	// 统计
	stats PersistenceStats
}

// RedisClient Redis 客户端接口
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, expiration time.Duration) error
	Del(ctx context.Context, key string) error
}

// PersistenceConfig 持久化配置
type PersistenceConfig struct {
	TTL           time.Duration
	KeyPrefix     string
	UseRedis      bool
	MaxCacheSize  int
}

// PersistenceStats 统计
type PersistenceStats struct {
	ContextHits    int64 // 一致性命中
	ContextMisses  int64 // 未命中
	TotalContexts  int
	AvgMazeDepth   float64
}

// DefaultPersistenceConfig 默认配置
func DefaultPersistenceConfig() PersistenceConfig {
	return PersistenceConfig{
		TTL:          24 * time.Hour,
		KeyPrefix:    "mirage:shadow_context:",
		UseRedis:     false,
		MaxCacheSize: 50000,
	}
}

// NewContextPersistence 创建持久化管理器
func NewContextPersistence(config PersistenceConfig) *ContextPersistence {
	return &ContextPersistence{
		contexts: make(map[string]*ShadowContext),
		config:   config,
	}
}

// SetRedis 设置 Redis 客户端
func (cp *ContextPersistence) SetRedis(client RedisClient) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.redis = client
	cp.config.UseRedis = true
}

// GetOrCreate 获取或创建影子上下文
func (cp *ContextPersistence) GetOrCreate(uid string, region string, createFn func() string) (*ShadowContext, bool) {
	// 先尝试获取
	ctx, exists := cp.Get(uid)
	if exists {
		cp.mu.Lock()
		cp.stats.ContextHits++
		ctx.LastSeen = time.Now()
		ctx.RequestCount++
		cp.mu.Unlock()
		return ctx, true
	}

	// 创建新上下文
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.stats.ContextMisses++

	template := createFn()
	ctx = &ShadowContext{
		UID:              uid,
		AssignedTemplate: template,
		Seed:             time.Now().UnixNano(),
		FirstSeen:        time.Now(),
		LastSeen:         time.Now(),
		Region:           region,
		RegionBias:       cp.getRegionBias(region),
		RequestCount:     1,
	}

	// 存储
	cp.contexts[uid] = ctx
	cp.stats.TotalContexts = len(cp.contexts)

	// 异步写入 Redis
	if cp.config.UseRedis && cp.redis != nil {
		go cp.saveToRedis(uid, ctx)
	}

	// 检查容量
	if len(cp.contexts) > cp.config.MaxCacheSize {
		cp.evictOldest()
	}

	return ctx, false
}

// Get 获取影子上下文
func (cp *ContextPersistence) Get(uid string) (*ShadowContext, bool) {
	cp.mu.RLock()
	ctx, exists := cp.contexts[uid]
	cp.mu.RUnlock()

	if exists {
		return ctx, true
	}

	// 尝试从 Redis 获取
	if cp.config.UseRedis && cp.redis != nil {
		ctx, err := cp.loadFromRedis(uid)
		if err == nil && ctx != nil {
			cp.mu.Lock()
			cp.contexts[uid] = ctx
			cp.mu.Unlock()
			return ctx, true
		}
	}

	return nil, false
}

// UpdateMazeDepth 更新迷宫深度
func (cp *ContextPersistence) UpdateMazeDepth(uid string, depth int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if ctx, exists := cp.contexts[uid]; exists {
		if depth > ctx.MazeDepth {
			ctx.MazeDepth = depth
		}
	}
}

// getRegionBias 获取区域偏好
func (cp *ContextPersistence) getRegionBias(region string) string {
	regionBiasMap := map[string]string{
		"SG": "asia_pacific",
		"HK": "asia_pacific",
		"JP": "asia_pacific",
		"KR": "asia_pacific",
		"TW": "asia_pacific",
		"DE": "europe_gdpr",
		"FR": "europe_gdpr",
		"NL": "europe_gdpr",
		"CH": "europe_gdpr",
		"GB": "europe_gdpr",
		"US": "north_america",
		"CA": "north_america",
		"BR": "latin_america",
		"AU": "oceania",
		"IS": "nordic",
	}

	if bias, ok := regionBiasMap[region]; ok {
		return bias
	}
	return "global"
}

// saveToRedis 保存到 Redis
func (cp *ContextPersistence) saveToRedis(uid string, ctx *ShadowContext) {
	if cp.redis == nil {
		return
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		return
	}

	key := cp.config.KeyPrefix + uid
	cp.redis.Set(context.Background(), key, string(data), cp.config.TTL)
}

// loadFromRedis 从 Redis 加载
func (cp *ContextPersistence) loadFromRedis(uid string) (*ShadowContext, error) {
	if cp.redis == nil {
		return nil, fmt.Errorf("redis not configured")
	}

	key := cp.config.KeyPrefix + uid
	data, err := cp.redis.Get(context.Background(), key)
	if err != nil {
		return nil, err
	}

	var ctx ShadowContext
	if err := json.Unmarshal([]byte(data), &ctx); err != nil {
		return nil, err
	}

	return &ctx, nil
}

// evictOldest 驱逐最旧条目
func (cp *ContextPersistence) evictOldest() {
	var oldest *ShadowContext
	var oldestUID string

	for uid, ctx := range cp.contexts {
		if oldest == nil || ctx.LastSeen.Before(oldest.LastSeen) {
			oldest = ctx
			oldestUID = uid
		}
	}

	if oldestUID != "" {
		delete(cp.contexts, oldestUID)
	}
}

// GetStats 获取统计
func (cp *ContextPersistence) GetStats() PersistenceStats {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	stats := cp.stats
	stats.TotalContexts = len(cp.contexts)

	// 计算平均迷宫深度
	if len(cp.contexts) > 0 {
		totalDepth := 0
		for _, ctx := range cp.contexts {
			totalDepth += ctx.MazeDepth
		}
		stats.AvgMazeDepth = float64(totalDepth) / float64(len(cp.contexts))
	}

	return stats
}

// GetContext 获取指定上下文
func (cp *ContextPersistence) GetContext(uid string) *ShadowContext {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.contexts[uid]
}

// Delete 删除上下文
func (cp *ContextPersistence) Delete(uid string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	delete(cp.contexts, uid)

	if cp.config.UseRedis && cp.redis != nil {
		key := cp.config.KeyPrefix + uid
		cp.redis.Del(context.Background(), key)
	}
}
