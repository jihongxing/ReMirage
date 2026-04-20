package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	xmrUSDCacheKey  = "mirage:xmr_usd_rate"
	defaultCacheTTL = 5 * time.Minute
)

// CoinGeckoProvider CoinGecko 汇率提供者
type CoinGeckoProvider struct {
	httpClient *http.Client
}

// NewCoinGeckoProvider 创建 CoinGecko 提供者
func NewCoinGeckoProvider() *CoinGeckoProvider {
	return &CoinGeckoProvider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// GetXMRUSDRate 从 CoinGecko 获取 XMR/USD 汇率
func (p *CoinGeckoProvider) GetXMRUSDRate(ctx context.Context) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.coingecko.com/api/v3/simple/price?ids=monero&vs_currencies=usd", nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("coingecko request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("coingecko returned status %d", resp.StatusCode)
	}

	var result struct {
		Monero struct {
			USD float64 `json:"usd"`
		} `json:"monero"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode coingecko response: %w", err)
	}

	if result.Monero.USD <= 0 {
		return 0, fmt.Errorf("invalid rate from coingecko: %f", result.Monero.USD)
	}

	return result.Monero.USD, nil
}

// KrakenProvider Kraken 汇率提供者
type KrakenProvider struct {
	httpClient *http.Client
}

// NewKrakenProvider 创建 Kraken 提供者
func NewKrakenProvider() *KrakenProvider {
	return &KrakenProvider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// GetXMRUSDRate 从 Kraken 获取 XMR/USD 汇率
func (p *KrakenProvider) GetXMRUSDRate(ctx context.Context) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.kraken.com/0/public/Ticker?pair=XMRUSD", nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("kraken request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("kraken returned status %d", resp.StatusCode)
	}

	var result struct {
		Error  []string               `json:"error"`
		Result map[string]interface{} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode kraken response: %w", err)
	}

	if len(result.Error) > 0 {
		return 0, fmt.Errorf("kraken error: %v", result.Error)
	}

	// Kraken 返回 XXMRZUSD 键
	for _, v := range result.Result {
		tickerData, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		// "c" 是最近成交价 [price, lot-volume]
		c, ok := tickerData["c"].([]interface{})
		if !ok || len(c) == 0 {
			continue
		}
		priceStr, ok := c[0].(string)
		if !ok {
			continue
		}
		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			return 0, fmt.Errorf("parse kraken price: %w", err)
		}
		if price <= 0 {
			return 0, fmt.Errorf("invalid rate from kraken: %f", price)
		}
		return price, nil
	}

	return 0, fmt.Errorf("no ticker data in kraken response")
}

// CachedExchangeRateProvider 带 Redis 缓存的汇率提供者
type CachedExchangeRateProvider struct {
	rdb      *redis.Client
	primary  ExchangeRateProvider
	fallback ExchangeRateProvider
	cacheTTL time.Duration
	cacheKey string
}

// NewCachedExchangeRateProvider 创建带缓存的汇率提供者
func NewCachedExchangeRateProvider(rdb *redis.Client, primary, fallback ExchangeRateProvider) *CachedExchangeRateProvider {
	return &CachedExchangeRateProvider{
		rdb:      rdb,
		primary:  primary,
		fallback: fallback,
		cacheTTL: defaultCacheTTL,
		cacheKey: xmrUSDCacheKey,
	}
}

// GetXMRUSDRate 获取汇率（缓存 → primary → fallback）
func (p *CachedExchangeRateProvider) GetXMRUSDRate(ctx context.Context) (float64, error) {
	// 1. 尝试缓存
	cached, err := p.rdb.Get(ctx, p.cacheKey).Result()
	if err == nil {
		rate, parseErr := strconv.ParseFloat(cached, 64)
		if parseErr == nil && rate > 0 {
			return rate, nil
		}
	}

	// 2. 尝试 primary
	rate, err := p.primary.GetXMRUSDRate(ctx)
	if err == nil {
		p.rdb.Set(ctx, p.cacheKey, strconv.FormatFloat(rate, 'f', 8, 64), p.cacheTTL)
		return rate, nil
	}
	primaryErr := err

	// 3. 尝试 fallback
	rate, err = p.fallback.GetXMRUSDRate(ctx)
	if err == nil {
		p.rdb.Set(ctx, p.cacheKey, strconv.FormatFloat(rate, 'f', 8, 64), p.cacheTTL)
		return rate, nil
	}

	return 0, fmt.Errorf("all exchange rate providers failed: primary=%v, fallback=%v", primaryErr, err)
}
