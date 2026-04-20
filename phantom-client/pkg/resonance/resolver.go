// Package resonance - 信令共振发现器 (Resonance Resolver)
// 客户端侧：从多个公告板通道并发拉取加密信令，First-Win-Cancels-All 竞速模式
//
// 三通道：
//   - DNS TXT via DoH (1.1.1.1 / 8.8.8.8)：绕过本地 ISP 投毒
//   - GitHub Gist：公开 JSON 伪装为监控数据
//   - Mastodon Hashtag Search：去中心化社交网络
//
// 安全设计：
//   - 强制 DoH（DNS over HTTPS），绝不使用系统 DNS
//   - 获取密文后调用 SignalCrypto.OpenSignal() 完成解密+验签+反重放
//   - 所有 HTTP 请求带 context 超时，任一通道成功立即取消其余
package resonance

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ResolverConfig 发现器配置
type ResolverConfig struct {
	// DNS TXT (DoH)
	DNSRecordName string   `yaml:"dns_record_name"` // e.g. _sig.cdn-telemetry.example.com
	DoHServers    []string `yaml:"doh_servers"`     // e.g. ["https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query"]

	// GitHub Gist
	GistID       string `yaml:"gist_id"`
	GistFileName string `yaml:"gist_file_name"` // e.g. telemetry.json

	// Mastodon
	MastodonInstance string `yaml:"mastodon_instance"` // e.g. https://mastodon.social
	MastodonHashtag  string `yaml:"mastodon_hashtag"`  // 不含 # 前缀，e.g. cdn_health

	// 超时
	ChannelTimeout time.Duration `yaml:"channel_timeout"` // 单通道超时，默认 10s
}

// SignalOpener 信令解密接口（由 SignalCrypto 实现）
type SignalOpener interface {
	OpenSignal(sealed []byte) (payload interface{}, err error)
}

// ResolvedSignal 解析成功的信令结果
type ResolvedSignal struct {
	Gateways []GatewayInfo
	Domains  []string
	Channel  string // 哪个通道率先成功
	Latency  time.Duration
}

// GatewayInfo 网关信息
type GatewayInfo struct {
	IP       string
	Port     int
	Priority uint8
}

// OpenFunc 解密函数签名（解耦 SignalCrypto 依赖）
// 输入：密文字节，输出：网关列表 + 域名列表
type OpenFunc func(sealed []byte) (gateways []GatewayInfo, domains []string, err error)

// Resolver 信令共振发现器
type Resolver struct {
	config     *ResolverConfig
	openFn     OpenFunc
	httpClient *http.Client

	// 统计
	totalAttempts  atomic.Uint64
	totalSuccesses atomic.Uint64
	totalFailures  atomic.Uint64
}

// NewResolver 创建发现器
func NewResolver(config *ResolverConfig, openFn OpenFunc) *Resolver {
	if len(config.DoHServers) == 0 {
		config.DoHServers = []string{
			"https://1.1.1.1/dns-query",
			"https://8.8.8.8/dns-query",
		}
	}
	if config.ChannelTimeout <= 0 {
		config.ChannelTimeout = 10 * time.Second
	}

	return &Resolver{
		config: config,
		openFn: openFn,
		httpClient: &http.Client{
			Timeout: config.ChannelTimeout,
			// 强制不使用系统代理（防止代理泄漏）
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 5 * time.Second,
				DisableKeepAlives:   true,
			},
		},
	}
}

// Resolve 并发竞速拉取信令（First-Win-Cancels-All）
// 任一通道率先返回有效信令，立即取消其余通道
func (r *Resolver) Resolve(ctx context.Context) (*ResolvedSignal, error) {
	r.totalAttempts.Add(1)

	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		signal *ResolvedSignal
		err    error
	}

	resultCh := make(chan result, 3)
	var wg sync.WaitGroup

	// 通道 1：DNS TXT via DoH
	if r.config.DNSRecordName != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			sig, err := r.resolveDNSTXT(raceCtx)
			if err == nil {
				sig.Channel = "dns_txt"
				sig.Latency = time.Since(start)
				resultCh <- result{signal: sig}
			} else {
				resultCh <- result{err: fmt.Errorf("dns_txt: %w", err)}
			}
		}()
	}

	// 通道 2：GitHub Gist
	if r.config.GistID != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			sig, err := r.resolveGist(raceCtx)
			if err == nil {
				sig.Channel = "gist"
				sig.Latency = time.Since(start)
				resultCh <- result{signal: sig}
			} else {
				resultCh <- result{err: fmt.Errorf("gist: %w", err)}
			}
		}()
	}

	// 通道 3：Mastodon
	if r.config.MastodonInstance != "" && r.config.MastodonHashtag != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			sig, err := r.resolveMastodon(raceCtx)
			if err == nil {
				sig.Channel = "mastodon"
				sig.Latency = time.Since(start)
				resultCh <- result{signal: sig}
			} else {
				resultCh <- result{err: fmt.Errorf("mastodon: %w", err)}
			}
		}()
	}

	// 关闭通道（所有 goroutine 完成后）
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// First-Win-Cancels-All
	var errs []string
	for res := range resultCh {
		if res.signal != nil {
			cancel() // 斩断其余通道
			r.totalSuccesses.Add(1)
			log.Printf("[Resonance] ✅ 信令发现成功 via %s (%v)", res.signal.Channel, res.signal.Latency)
			return res.signal, nil
		}
		errs = append(errs, res.err.Error())
	}

	r.totalFailures.Add(1)
	return nil, fmt.Errorf("所有通道均失败: %s", strings.Join(errs, "; "))
}

// Stats 返回统计信息
func (r *Resolver) Stats() (attempts, successes, failures uint64) {
	return r.totalAttempts.Load(), r.totalSuccesses.Load(), r.totalFailures.Load()
}
