// Package domain - 信令共振发布器 (ResonancePublisher)
// OS 侧：将加密信令并发推送到多个公告板通道
//
// OS 处于不受审查的自由网络，直连官方 API 即可：
//   - Cloudflare DNS API → 更新 TXT 记录
//   - GitHub REST API → 更新 Gist
//   - Mastodon API → 发 Toot（同时删除旧 Toot）
//
// 容灾架构：3 通道并发，任一成功即标记本轮发布 Success
package domain

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// PublishChannel 发布通道
type PublishChannel int

const (
	ChannelDNSTXT   PublishChannel = 1
	ChannelGist     PublishChannel = 2
	ChannelMastodon PublishChannel = 3
)

// ChannelConfig 通道配置
type ChannelConfig struct {
	// Cloudflare DNS
	CFZoneID     string `yaml:"cf_zone_id"`
	CFRecordID   string `yaml:"cf_record_id"`
	CFAPIToken   string `yaml:"cf_api_token"`
	CFRecordName string `yaml:"cf_record_name"` // e.g. _sig.cdn-telemetry.example.com

	// GitHub Gist
	GistID       string `yaml:"gist_id"`
	GistToken    string `yaml:"gist_token"`
	GistFileName string `yaml:"gist_file_name"` // e.g. telemetry.json

	// Mastodon
	MastodonInstance string `yaml:"mastodon_instance"` // e.g. https://mastodon.social
	MastodonToken    string `yaml:"mastodon_token"`
	MastodonHashtag  string `yaml:"mastodon_hashtag"` // e.g. #cdn_health
}

// PublishResult 单通道发布结果
type PublishResult struct {
	Channel PublishChannel
	Success bool
	Error   error
	Latency time.Duration
}

// ResonancePublisher 信令共振发布器
type ResonancePublisher struct {
	config     *ChannelConfig
	httpClient *http.Client
	stopCh     chan struct{}
	wg         sync.WaitGroup

	// Mastodon 旧 Toot ID（用于幽灵清理）
	lastTootID atomic.Value // string

	// 发布统计
	totalPublishes atomic.Uint64
	totalFailures  atomic.Uint64

	// 信令生成回调（由上层注入，返回加密后的信令字节）
	sealFn func() ([]byte, error)
}

// NewResonancePublisher 创建发布器
func NewResonancePublisher(config *ChannelConfig, sealFn func() ([]byte, error)) *ResonancePublisher {
	rp := &ResonancePublisher{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		stopCh: make(chan struct{}),
		sealFn: sealFn,
	}
	rp.lastTootID.Store("")
	return rp
}

// Start 启动定时发布循环（60s 间隔）
func (rp *ResonancePublisher) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	rp.wg.Add(1)
	go rp.publishLoop(ctx, interval)
	log.Printf("[ResonancePublisher] 已启动，发布间隔: %v", interval)
}

// Stop 停止
func (rp *ResonancePublisher) Stop() {
	close(rp.stopCh)
	rp.wg.Wait()
	log.Println("[ResonancePublisher] 已停止")
}

// PublishOnce 立即执行一次发布（手动触发）
func (rp *ResonancePublisher) PublishOnce(ctx context.Context) []PublishResult {
	return rp.publish(ctx)
}

func (rp *ResonancePublisher) publishLoop(ctx context.Context, interval time.Duration) {
	defer rp.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 启动后立即发布一次
	rp.publish(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-rp.stopCh:
			return
		case <-ticker.C:
			rp.publish(ctx)
		}
	}
}

func (rp *ResonancePublisher) publish(ctx context.Context) []PublishResult {
	// 1. 生成加密信令
	sealed, err := rp.sealFn()
	if err != nil {
		log.Printf("[ResonancePublisher] ⚠️ 信令生成失败: %v", err)
		return nil
	}

	// 2. Base64 RawURL 编码（无 padding，URL-safe，节省 33% 体积）
	encoded := base64.RawURLEncoding.EncodeToString(sealed)

	// 3. 并发推送到 3 个通道
	results := make([]PublishResult, 3)
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		results[0] = rp.publishDNSTXT(ctx, encoded)
	}()
	go func() {
		defer wg.Done()
		results[1] = rp.publishGist(ctx, encoded)
	}()
	go func() {
		defer wg.Done()
		results[2] = rp.publishMastodon(ctx, encoded)
	}()
	wg.Wait()

	// 4. 统计
	rp.totalPublishes.Add(1)
	anySuccess := false
	for _, r := range results {
		if r.Success {
			anySuccess = true
		}
	}
	if !anySuccess {
		rp.totalFailures.Add(1)
		log.Println("[ResonancePublisher] ⚠️ 本轮所有通道发布失败")
	} else {
		log.Printf("[ResonancePublisher] ✅ 信令已发布 (%d bytes encoded)", len(encoded))
	}

	return results
}

// ============================================================
// 通道 ①：Cloudflare DNS TXT
// ============================================================

func (rp *ResonancePublisher) publishDNSTXT(ctx context.Context, encoded string) PublishResult {
	start := time.Now()
	result := PublishResult{Channel: ChannelDNSTXT}

	if rp.config.CFZoneID == "" || rp.config.CFAPIToken == "" {
		result.Error = fmt.Errorf("Cloudflare DNS 配置缺失")
		return result
	}

	// Cloudflare API: PUT /zones/{zone_id}/dns_records/{record_id}
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s",
		rp.config.CFZoneID, rp.config.CFRecordID)

	body := map[string]interface{}{
		"type":    "TXT",
		"name":    rp.config.CFRecordName,
		"content": encoded,
		"ttl":     60, // 最短 TTL
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(jsonBody))
	if err != nil {
		result.Error = err
		return result
	}
	req.Header.Set("Authorization", "Bearer "+rp.config.CFAPIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := rp.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("CF DNS 请求失败: %w", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		result.Error = fmt.Errorf("CF DNS 返回 %d: %s", resp.StatusCode, string(respBody))
		return result
	}

	result.Success = true
	result.Latency = time.Since(start)
	return result
}

// ============================================================
// 通道 ②：GitHub Gist
// ============================================================

func (rp *ResonancePublisher) publishGist(ctx context.Context, encoded string) PublishResult {
	start := time.Now()
	result := PublishResult{Channel: ChannelGist}

	if rp.config.GistID == "" || rp.config.GistToken == "" {
		result.Error = fmt.Errorf("GitHub Gist 配置缺失")
		return result
	}

	// GitHub API: PATCH /gists/{gist_id}
	url := fmt.Sprintf("https://api.github.com/gists/%s", rp.config.GistID)

	fileName := rp.config.GistFileName
	if fileName == "" {
		fileName = "telemetry.json"
	}

	// 包装为 JSON（看起来像监控数据）
	content := fmt.Sprintf(`{"v":1,"ts":%d,"data":"%s"}`, time.Now().Unix(), encoded)

	body := map[string]interface{}{
		"files": map[string]interface{}{
			fileName: map[string]string{
				"content": content,
			},
		},
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(jsonBody))
	if err != nil {
		result.Error = err
		return result
	}
	req.Header.Set("Authorization", "Bearer "+rp.config.GistToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := rp.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("GitHub Gist 请求失败: %w", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		result.Error = fmt.Errorf("GitHub Gist 返回 %d: %s", resp.StatusCode, string(respBody))
		return result
	}

	result.Success = true
	result.Latency = time.Since(start)
	return result
}

// ============================================================
// 通道 ③：Mastodon Toot（含幽灵清理）
// ============================================================

func (rp *ResonancePublisher) publishMastodon(ctx context.Context, encoded string) PublishResult {
	start := time.Now()
	result := PublishResult{Channel: ChannelMastodon}

	if rp.config.MastodonInstance == "" || rp.config.MastodonToken == "" {
		result.Error = fmt.Errorf("Mastodon 配置缺失")
		return result
	}

	// 幽灵清理：先删除旧 Toot
	if oldID, ok := rp.lastTootID.Load().(string); ok && oldID != "" {
		rp.deleteToot(ctx, oldID)
	}

	// 发布新 Toot
	hashtag := rp.config.MastodonHashtag
	if hashtag == "" {
		hashtag = "#cdn_health"
	}
	status := fmt.Sprintf("%s %s", hashtag, encoded)

	url := fmt.Sprintf("%s/api/v1/statuses", rp.config.MastodonInstance)
	body := map[string]interface{}{
		"status":     status,
		"visibility": "unlisted", // 不出现在公共时间线
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		result.Error = err
		return result
	}
	req.Header.Set("Authorization", "Bearer "+rp.config.MastodonToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := rp.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("Mastodon 请求失败: %w", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		result.Error = fmt.Errorf("Mastodon 返回 %d: %s", resp.StatusCode, string(respBody))
		return result
	}

	// 解析新 Toot ID（用于下次幽灵清理）
	var tootResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tootResp); err == nil && tootResp.ID != "" {
		rp.lastTootID.Store(tootResp.ID)
	}

	result.Success = true
	result.Latency = time.Since(start)
	return result
}

// deleteToot 删除旧 Toot（幽灵清理）
func (rp *ResonancePublisher) deleteToot(ctx context.Context, tootID string) {
	url := fmt.Sprintf("%s/api/v1/statuses/%s", rp.config.MastodonInstance, tootID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+rp.config.MastodonToken)

	resp, err := rp.httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// GetStats 获取发布统计
func (rp *ResonancePublisher) GetStats() (total, failures uint64) {
	return rp.totalPublishes.Load(), rp.totalFailures.Load()
}
