// Package gswitch - 信令共振解析器 (ResonanceResolver)
// 客户端侧：从多个公告板并发拉取加密信令，解密验签获取新 Gateway IP
//
// 突围策略：
//   - DNS TXT: 内置 DoH 客户端（https://1.1.1.1/dns-query），绕过 ISP 流氓缓存
//   - GitHub Gist: 通过 CF Worker 反代，审查者只看到普通 CF 域名
//   - Mastodon: 直接 HTTPS 拉取（Mastodon 实例通常不被封锁）
//
// 极限竞速：3 通道并发，首个成功解密验签的结果立即返回，cancel 其余通道
package gswitch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ResolverConfig 解析器配置
type ResolverConfig struct {
	// DoH DNS TXT
	DoHEndpoint string `yaml:"doh_endpoint"` // e.g. https://1.1.1.1/dns-query
	TXTName     string `yaml:"txt_name"`     // e.g. _sig.cdn-telemetry.example.com

	// CF Worker 反代 GitHub Gist
	CFWorkerURL string `yaml:"cf_worker_url"` // e.g. https://cdn-health.yourdomain.com/status

	// Mastodon
	MastodonAccountURL string `yaml:"mastodon_account_url"` // e.g. https://mastodon.social/api/v1/accounts/{id}/statuses
	MastodonHashtag    string `yaml:"mastodon_hashtag"`     // e.g. #cdn_health
}

// ResolveResult 解析结果
type ResolveResult struct {
	Payload *SignalPayload
	Channel string
	Latency time.Duration
}

// ResonanceResolver 信令共振解析器
type ResonanceResolver struct {
	config     *ResolverConfig
	crypto     *SignalCrypto
	httpClient *http.Client

	// 回调：成功获取新 Gateway 列表后触发
	onResolved func(payload *SignalPayload)
}

// NewResonanceResolver 创建解析器
func NewResonanceResolver(config *ResolverConfig, crypto *SignalCrypto) *ResonanceResolver {
	return &ResonanceResolver{
		config: config,
		crypto: crypto,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

// SetOnResolved 设置解析成功回调
func (rr *ResonanceResolver) SetOnResolved(fn func(*SignalPayload)) {
	rr.onResolved = fn
}

// Resolve 执行共振发现（极限竞速：首个成功即返回）
// 超时 5s，3 通道并发，任一通道率先返回有效数据即 cancel 其余
func (rr *ResonanceResolver) Resolve(ctx context.Context) (*ResolveResult, error) {
	raceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	type raceEntry struct {
		result *ResolveResult
		err    error
	}

	ch := make(chan raceEntry, 3)
	var wg sync.WaitGroup

	// 通道 ①：DoH DNS TXT
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		payload, err := rr.resolveDoH(raceCtx)
		if err != nil {
			ch <- raceEntry{err: err}
			return
		}
		ch <- raceEntry{result: &ResolveResult{
			Payload: payload,
			Channel: "DoH-TXT",
			Latency: time.Since(start),
		}}
	}()

	// 通道 ②：CF Worker (GitHub Gist)
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		payload, err := rr.resolveCFWorker(raceCtx)
		if err != nil {
			ch <- raceEntry{err: err}
			return
		}
		ch <- raceEntry{result: &ResolveResult{
			Payload: payload,
			Channel: "CF-Worker",
			Latency: time.Since(start),
		}}
	}()

	// 通道 ③：Mastodon
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		payload, err := rr.resolveMastodon(raceCtx)
		if err != nil {
			ch <- raceEntry{err: err}
			return
		}
		ch <- raceEntry{result: &ResolveResult{
			Payload: payload,
			Channel: "Mastodon",
			Latency: time.Since(start),
		}}
	}()

	// 后台关闭 channel
	go func() {
		wg.Wait()
		close(ch)
	}()

	// 极限竞速：首个成功即返回
	var lastErr error
	for entry := range ch {
		if entry.result != nil {
			cancel() // 斩断其余通道
			log.Printf("[ResonanceResolver] ✅ 共振发现成功 (channel=%s, latency=%v)",
				entry.result.Channel, entry.result.Latency)
			if rr.onResolved != nil {
				rr.onResolved(entry.result.Payload)
			}
			return entry.result, nil
		}
		lastErr = entry.err
	}

	return nil, fmt.Errorf("所有通道均失败，最后错误: %v", lastErr)
}

// ============================================================
// 通道 ①：DoH DNS TXT（绕过 ISP 流氓缓存 + UDP 截断）
// ============================================================

// dohResponse Cloudflare DoH JSON 响应
type dohResponse struct {
	Answer []struct {
		Data string `json:"data"`
	} `json:"Answer"`
}

func (rr *ResonanceResolver) resolveDoH(ctx context.Context) (*SignalPayload, error) {
	endpoint := rr.config.DoHEndpoint
	if endpoint == "" {
		endpoint = "https://1.1.1.1/dns-query"
	}

	// 构造 DoH 请求（application/dns-json，Cloudflare 格式）
	url := fmt.Sprintf("%s?name=%s&type=TXT", endpoint, rr.config.TXTName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")

	resp, err := rr.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DoH 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH 返回 %d", resp.StatusCode)
	}

	var doh dohResponse
	if err := json.NewDecoder(resp.Body).Decode(&doh); err != nil {
		return nil, fmt.Errorf("DoH 响应解析失败: %w", err)
	}

	if len(doh.Answer) == 0 {
		return nil, fmt.Errorf("DoH 无 TXT 记录")
	}

	// TXT 记录值可能带引号
	encoded := strings.Trim(doh.Answer[0].Data, "\"")

	return rr.decodeAndVerify(encoded)
}

// ============================================================
// 通道 ②：CF Worker 反代 GitHub Gist
// ============================================================

func (rr *ResonanceResolver) resolveCFWorker(ctx context.Context) (*SignalPayload, error) {
	if rr.config.CFWorkerURL == "" {
		return nil, fmt.Errorf("CF Worker URL 未配置")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rr.config.CFWorkerURL, nil)
	if err != nil {
		return nil, err
	}
	// 看起来像普通的 CDN 健康检查
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HealthCheck/1.0)")

	resp, err := rr.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CF Worker 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CF Worker 返回 %d", resp.StatusCode)
	}

	// 解析 JSON 包装：{"v":1,"ts":xxx,"data":"<base64>"}
	var wrapper struct {
		V    int    `json:"v"`
		TS   int64  `json:"ts"`
		Data string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("CF Worker 响应解析失败: %w", err)
	}

	if wrapper.Data == "" {
		return nil, fmt.Errorf("CF Worker 响应无 data 字段")
	}

	return rr.decodeAndVerify(wrapper.Data)
}

// ============================================================
// 通道 ③：Mastodon Toot
// ============================================================

func (rr *ResonanceResolver) resolveMastodon(ctx context.Context) (*SignalPayload, error) {
	if rr.config.MastodonAccountURL == "" {
		return nil, fmt.Errorf("Mastodon 账号 URL 未配置")
	}

	// 拉取最新的 Toot
	url := rr.config.MastodonAccountURL + "?limit=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := rr.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Mastodon 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mastodon 返回 %d", resp.StatusCode)
	}

	var toots []struct {
		Content string `json:"content"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &toots); err != nil {
		return nil, fmt.Errorf("Mastodon 响应解析失败: %w", err)
	}

	if len(toots) == 0 {
		return nil, fmt.Errorf("Mastodon 无 Toot")
	}

	// 从 Toot 内容中提取 Base64 信令
	// 格式：#cdn_health <base64_data>
	// Mastodon HTML 会包裹 <p> 标签，需要清理
	content := toots[0].Content
	content = stripHTML(content)

	hashtag := rr.config.MastodonHashtag
	if hashtag == "" {
		hashtag = "#cdn_health"
	}

	// 提取 hashtag 后面的 Base64 数据
	idx := strings.Index(content, hashtag)
	if idx < 0 {
		// 尝试不带 # 的 tag
		plainTag := strings.TrimPrefix(hashtag, "#")
		idx = strings.Index(content, plainTag)
		if idx < 0 {
			return nil, fmt.Errorf("Mastodon Toot 中未找到 hashtag: %s", hashtag)
		}
		idx += len(plainTag)
	} else {
		idx += len(hashtag)
	}

	encoded := strings.TrimSpace(content[idx:])
	if encoded == "" {
		return nil, fmt.Errorf("Mastodon Toot 中无信令数据")
	}

	return rr.decodeAndVerify(encoded)
}

// ============================================================
// 公共解码 + 验证
// ============================================================

// decodeAndVerify Base64 RawURL 解码 → SignalCrypto.OpenSignal 解密验签
func (rr *ResonanceResolver) decodeAndVerify(encoded string) (*SignalPayload, error) {
	// Base64 RawURL 解码
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("Base64 解码失败: %w", err)
	}

	// 解密 + 验签 + 反重放
	payload, err := rr.crypto.OpenSignal(sealed)
	if err != nil {
		return nil, fmt.Errorf("信令验证失败: %w", err)
	}

	return payload, nil
}

// stripHTML 简易 HTML 标签清理（Mastodon 返回 HTML 格式）
func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			result.WriteRune(' ') // 标签替换为空格
		case !inTag:
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}
