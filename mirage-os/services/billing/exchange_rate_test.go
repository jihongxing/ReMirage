package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

// mockExchangeProvider 模拟汇率提供者
type mockExchangeProvider struct {
	rate float64
	err  error
}

func (m *mockExchangeProvider) GetXMRUSDRate(ctx context.Context) (float64, error) {
	return m.rate, m.err
}

// mockRedisClient 模拟 Redis 客户端（用于测试缓存逻辑）
type mockRedisClient struct {
	store map[string]string
	ttls  map[string]time.Duration
}

func newMockRedis() *mockRedisClient {
	return &mockRedisClient{
		store: make(map[string]string),
		ttls:  make(map[string]time.Duration),
	}
}

func (m *mockRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	val, ok := m.store[key]
	cmd := redis.NewStringCmd(ctx)
	if !ok {
		cmd.SetErr(redis.Nil)
	} else {
		cmd.SetVal(val)
	}
	return cmd
}

func (m *mockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	m.store[key] = fmt.Sprintf("%v", value)
	m.ttls[key] = expiration
	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("OK")
	return cmd
}

// testCachedProvider 使用 mock Redis 的缓存提供者
type testCachedProvider struct {
	mock     *mockRedisClient
	primary  ExchangeRateProvider
	fallback ExchangeRateProvider
	cacheTTL time.Duration
	cacheKey string
}

func newTestCachedProvider(mock *mockRedisClient, primary, fallback ExchangeRateProvider) *testCachedProvider {
	return &testCachedProvider{
		mock:     mock,
		primary:  primary,
		fallback: fallback,
		cacheTTL: 5 * time.Minute,
		cacheKey: xmrUSDCacheKey,
	}
}

func (p *testCachedProvider) GetXMRUSDRate(ctx context.Context) (float64, error) {
	// 1. 尝试缓存
	cmd := p.mock.Get(ctx, p.cacheKey)
	if cmd.Err() == nil {
		val, _ := cmd.Result()
		var rate float64
		if _, err := fmt.Sscanf(val, "%f", &rate); err == nil && rate > 0 {
			return rate, nil
		}
	}

	// 2. 尝试 primary
	rate, err := p.primary.GetXMRUSDRate(ctx)
	if err == nil {
		p.mock.Set(ctx, p.cacheKey, fmt.Sprintf("%.8f", rate), p.cacheTTL)
		return rate, nil
	}
	primaryErr := err

	// 3. 尝试 fallback
	rate, err = p.fallback.GetXMRUSDRate(ctx)
	if err == nil {
		p.mock.Set(ctx, p.cacheKey, fmt.Sprintf("%.8f", rate), p.cacheTTL)
		return rate, nil
	}

	return 0, fmt.Errorf("all providers failed: primary=%v, fallback=%v", primaryErr, err)
}

func TestCachedExchangeRate_CacheHit(t *testing.T) {
	mock := newMockRedis()
	mock.store[xmrUSDCacheKey] = "155.50000000"

	primary := &mockExchangeProvider{rate: 160.0}
	fallback := &mockExchangeProvider{rate: 158.0}

	provider := newTestCachedProvider(mock, primary, fallback)

	rate, err := provider.GetXMRUSDRate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应该返回缓存值，不调用 primary/fallback
	if rate < 155.0 || rate > 156.0 {
		t.Fatalf("expected cached rate ~155.5, got %f", rate)
	}
}

func TestCachedExchangeRate_CacheMiss_PrimarySuccess(t *testing.T) {
	mock := newMockRedis()
	// 缓存为空

	primary := &mockExchangeProvider{rate: 160.0}
	fallback := &mockExchangeProvider{rate: 158.0}

	provider := newTestCachedProvider(mock, primary, fallback)

	rate, err := provider.GetXMRUSDRate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 160.0 {
		t.Fatalf("expected primary rate 160.0, got %f", rate)
	}
	// 验证缓存已写入
	if _, ok := mock.store[xmrUSDCacheKey]; !ok {
		t.Fatal("expected cache to be set after primary success")
	}
}

func TestCachedExchangeRate_PrimaryFail_FallbackSuccess(t *testing.T) {
	mock := newMockRedis()

	primary := &mockExchangeProvider{err: fmt.Errorf("coingecko down")}
	fallback := &mockExchangeProvider{rate: 158.0}

	provider := newTestCachedProvider(mock, primary, fallback)

	rate, err := provider.GetXMRUSDRate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 158.0 {
		t.Fatalf("expected fallback rate 158.0, got %f", rate)
	}
}

func TestCachedExchangeRate_BothFail(t *testing.T) {
	mock := newMockRedis()

	primary := &mockExchangeProvider{err: fmt.Errorf("coingecko down")}
	fallback := &mockExchangeProvider{err: fmt.Errorf("kraken down")}

	provider := newTestCachedProvider(mock, primary, fallback)

	_, err := provider.GetXMRUSDRate(context.Background())
	if err == nil {
		t.Fatal("expected error when both providers fail")
	}
}
