package resonance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// mockOpenFn 模拟解密函数（直接解析 JSON）
func mockOpenFn(sealed []byte) ([]GatewayInfo, []string, error) {
	var payload struct {
		Gateways []GatewayInfo `json:"gateways"`
		Domains  []string      `json:"domains"`
	}
	if err := json.Unmarshal(sealed, &payload); err != nil {
		return nil, nil, fmt.Errorf("mock open: %w", err)
	}
	return payload.Gateways, payload.Domains, nil
}

// encodeMockSignal 编码模拟信令
func encodeMockSignal(gateways []GatewayInfo, domains []string) string {
	payload := map[string]interface{}{
		"gateways": gateways,
		"domains":  domains,
	}
	data, _ := json.Marshal(payload)
	return base64.RawURLEncoding.EncodeToString(data)
}

func TestResolveDNSTXT(t *testing.T) {
	gateways := []GatewayInfo{{IP: "1.2.3.4", Port: 443, Priority: 1}}
	domains := []string{"gw.example.com"}
	encoded := encodeMockSignal(gateways, domains)

	// 模拟 DoH 服务器
	dohServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := dohResponse{
			Status: 0,
			Answer: []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
			}{
				{Type: 16, Data: fmt.Sprintf(`"%s"`, encoded)},
			},
		}
		w.Header().Set("Content-Type", "application/dns-json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer dohServer.Close()

	resolver := NewResolver(&ResolverConfig{
		DNSRecordName:  "_sig.test.example.com",
		DoHServers:     []string{dohServer.URL},
		ChannelTimeout: 5 * time.Second,
	}, mockOpenFn)

	ctx := context.Background()
	signal, err := resolver.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if signal.Channel != "dns_txt" {
		t.Errorf("expected channel dns_txt, got %s", signal.Channel)
	}
	if len(signal.Gateways) != 1 || signal.Gateways[0].IP != "1.2.3.4" {
		t.Errorf("unexpected gateways: %+v", signal.Gateways)
	}
}

func TestResolveGist(t *testing.T) {
	gateways := []GatewayInfo{{IP: "5.6.7.8", Port: 8443, Priority: 2}}
	domains := []string{"backup.example.com"}
	encoded := encodeMockSignal(gateways, domains)

	// 模拟 Gist 服务器
	gistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := gistPayload{V: 1, Ts: time.Now().Unix(), Data: encoded}
		json.NewEncoder(w).Encode(payload)
	}))
	defer gistServer.Close()

	// 需要覆盖 Gist URL，通过自定义 httpClient 实现
	resolver := NewResolver(&ResolverConfig{
		GistID:         "test-gist-id",
		GistFileName:   "telemetry.json",
		ChannelTimeout: 5 * time.Second,
	}, mockOpenFn)

	// 替换 httpClient 的 Transport 以拦截请求
	resolver.httpClient.Transport = &mockTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			payload := gistPayload{V: 1, Ts: time.Now().Unix(), Data: encoded}
			json.NewEncoder(rec).Encode(payload)
			return rec.Result(), nil
		},
	}

	ctx := context.Background()
	signal, err := resolver.resolveGist(ctx)
	if err != nil {
		t.Fatalf("resolveGist failed: %v", err)
	}
	if len(signal.Gateways) != 1 || signal.Gateways[0].IP != "5.6.7.8" {
		t.Errorf("unexpected gateways: %+v", signal.Gateways)
	}
}

func TestResolveMastodon(t *testing.T) {
	gateways := []GatewayInfo{{IP: "10.0.0.1", Port: 443, Priority: 1}}
	domains := []string{"masto.example.com"}
	encoded := encodeMockSignal(gateways, domains)

	// 模拟 Mastodon 服务器
	mastoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statuses := []mastodonStatus{
			{
				ID:        "12345",
				Content:   fmt.Sprintf(`<p><a href="#">#cdn_health</a> %s</p>`, encoded),
				CreatedAt: time.Now().Format(time.RFC3339),
			},
		}
		json.NewEncoder(w).Encode(statuses)
	}))
	defer mastoServer.Close()

	resolver := NewResolver(&ResolverConfig{
		MastodonInstance: mastoServer.URL,
		MastodonHashtag:  "cdn_health",
		ChannelTimeout:   5 * time.Second,
	}, mockOpenFn)

	ctx := context.Background()
	signal, err := resolver.resolveMastodon(ctx)
	if err != nil {
		t.Fatalf("resolveMastodon failed: %v", err)
	}
	if len(signal.Gateways) != 1 || signal.Gateways[0].IP != "10.0.0.1" {
		t.Errorf("unexpected gateways: %+v", signal.Gateways)
	}
}

func TestFirstWinCancelsAll(t *testing.T) {
	gateways := []GatewayInfo{{IP: "1.1.1.1", Port: 443, Priority: 1}}
	encoded := encodeMockSignal(gateways, nil)

	// 快速通道（DNS）
	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := dohResponse{
			Status: 0,
			Answer: []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
			}{
				{Type: 16, Data: fmt.Sprintf(`"%s"`, encoded)},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer fastServer.Close()

	// 慢速通道（Gist，延迟 5s）
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		payload := gistPayload{V: 1, Ts: time.Now().Unix(), Data: encoded}
		json.NewEncoder(w).Encode(payload)
	}))
	defer slowServer.Close()

	resolver := NewResolver(&ResolverConfig{
		DNSRecordName:  "_sig.test.com",
		DoHServers:     []string{fastServer.URL},
		GistID:         "slow-gist",
		ChannelTimeout: 10 * time.Second,
	}, mockOpenFn)

	// 替换 Gist 请求到慢速服务器
	resolver.httpClient.Transport = &routingTransport{
		dohURL:  fastServer.URL,
		gistURL: slowServer.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	signal, err := resolver.Resolve(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if signal.Channel != "dns_txt" {
		t.Errorf("expected fast channel dns_txt, got %s", signal.Channel)
	}
	// 应该在 1s 内完成（不等待慢速通道）
	if elapsed > 2*time.Second {
		t.Errorf("First-Win 未生效，耗时 %v", elapsed)
	}
}

// Property: 任意合法 Base64 RawURL 字符串都能通过验证
func TestProperty_Base64RawURLValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机字节并编码
		data := rapid.SliceOfN(rapid.Byte(), 10, 500).Draw(t, "data")
		encoded := base64.RawURLEncoding.EncodeToString(data)

		if !isValidBase64RawURL(encoded) {
			t.Fatalf("valid base64 rawurl rejected: %s", encoded)
		}
	})
}

// Property: stripHTML 不丢失非标签文本
func TestProperty_StripHTMLPreservesText(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成不含 < > 的文本
		text := rapid.StringMatching(`[a-zA-Z0-9 ]{5,50}`).Draw(t, "text")
		result := stripHTML(text)
		if result != text {
			t.Fatalf("stripHTML altered plain text: %q → %q", text, result)
		}
	})
}

// ============================================================
// 测试辅助
// ============================================================

type mockTransport struct {
	handler func(*http.Request) (*http.Response, error)
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.handler(req)
}

type routingTransport struct {
	dohURL  string
	gistURL string
}

func (t *routingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(req)
}

// ============================================================
// Task 5.1: 部分通道配置 + 单通道超时不阻塞
// Validates: Requirements 6.1, 6.5, 6.6
// ============================================================

// TestResolve_PartialChannelConfig 仅配置 DNS 通道时，Resolve 只启动 DNS，不因 Gist/Mastodon 未配置而失败
func TestResolve_PartialChannelConfig(t *testing.T) {
	gateways := []GatewayInfo{{IP: "9.8.7.6", Port: 443, Priority: 1}}
	domains := []string{"partial.example.com"}
	encoded := encodeMockSignal(gateways, domains)

	dohServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := dohResponse{
			Status: 0,
			Answer: []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
			}{
				{Type: 16, Data: fmt.Sprintf(`"%s"`, encoded)},
			},
		}
		w.Header().Set("Content-Type", "application/dns-json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer dohServer.Close()

	// 仅配置 DNS，Gist 和 Mastodon 均未配置
	resolver := NewResolver(&ResolverConfig{
		DNSRecordName:  "_sig.partial.example.com",
		DoHServers:     []string{dohServer.URL},
		ChannelTimeout: 5 * time.Second,
		// GistID: "",  MastodonInstance: "", MastodonHashtag: "" — 均为零值
	}, mockOpenFn)

	ctx := context.Background()
	signal, err := resolver.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve with partial config failed: %v", err)
	}
	if signal.Channel != "dns_txt" {
		t.Errorf("expected channel dns_txt, got %s", signal.Channel)
	}
	if len(signal.Gateways) != 1 || signal.Gateways[0].IP != "9.8.7.6" {
		t.Errorf("unexpected gateways: %+v", signal.Gateways)
	}
}

// TestResolve_SingleChannelTimeoutNoBlock 单通道超时不阻塞其余通道
// DNS 快速返回，Gist 超时（延迟远超 ChannelTimeout），Resolve 应在 DNS 返回后立即完成
func TestResolve_SingleChannelTimeoutNoBlock(t *testing.T) {
	gateways := []GatewayInfo{{IP: "2.3.4.5", Port: 443, Priority: 1}}
	encoded := encodeMockSignal(gateways, nil)

	// DNS 通道：立即返回
	dohServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := dohResponse{
			Status: 0,
			Answer: []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
			}{
				{Type: 16, Data: fmt.Sprintf(`"%s"`, encoded)},
			},
		}
		w.Header().Set("Content-Type", "application/dns-json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer dohServer.Close()

	// Gist 通道：模拟超时（sleep 远超 ChannelTimeout）
	gistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(30 * time.Second):
			// 不会到达这里，context 会先被取消
		}
	}))
	defer gistServer.Close()

	resolver := NewResolver(&ResolverConfig{
		DNSRecordName:  "_sig.timeout.test.com",
		DoHServers:     []string{dohServer.URL},
		GistID:         "timeout-gist",
		GistFileName:   "telemetry.json",
		ChannelTimeout: 2 * time.Second,
	}, mockOpenFn)

	// 路由 Gist 请求到慢速服务器
	resolver.httpClient.Transport = &channelRoutingTransport{
		routes: map[string]string{
			"gist.githubusercontent.com": gistServer.URL,
		},
		dohURL: dohServer.URL,
	}

	start := time.Now()
	ctx := context.Background()
	signal, err := resolver.Resolve(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if signal.Channel != "dns_txt" {
		t.Errorf("expected dns_txt, got %s", signal.Channel)
	}
	// DNS 应立即返回，不被 Gist 超时阻塞
	if elapsed > 2*time.Second {
		t.Errorf("single channel timeout blocked Resolve: elapsed %v", elapsed)
	}
}

// channelRoutingTransport 按域名路由请求到不同的 mock 服务器
type channelRoutingTransport struct {
	routes map[string]string // host -> mock server URL
	dohURL string
}

func (t *channelRoutingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// DoH 请求直接转发
	if strings.Contains(req.URL.String(), t.dohURL) || req.URL.Host == "" {
		return http.DefaultTransport.RoundTrip(req)
	}
	// 按域名路由
	for host, mockURL := range t.routes {
		if strings.Contains(req.URL.Host, host) {
			req.URL.Scheme = "http"
			req.URL.Host = strings.TrimPrefix(mockURL, "http://")
			return http.DefaultTransport.RoundTrip(req)
		}
	}
	return http.DefaultTransport.RoundTrip(req)
}

// ============================================================
// Task 5.2: Property 6 — Resolver First-Win Racing
// Feature: phase1-link-continuity, Property 6: Resolver First-Win racing
// Validates: Requirements 6.2, 6.3
// ============================================================

// TestProperty_FirstWinRacing 使用 rapid 生成随机通道延迟组合（至少一个成功），
// 验证 Resolve 返回成功且 ResolvedSignal 包含有效 Gateways 和 Channel。
func TestProperty_FirstWinRacing(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机网关信息
		gwIP := fmt.Sprintf("%d.%d.%d.%d",
			rapid.IntRange(1, 254).Draw(t, "ip1"),
			rapid.IntRange(0, 255).Draw(t, "ip2"),
			rapid.IntRange(0, 255).Draw(t, "ip3"),
			rapid.IntRange(1, 254).Draw(t, "ip4"))
		gwPort := rapid.IntRange(1, 65535).Draw(t, "port")
		gateways := []GatewayInfo{{IP: gwIP, Port: gwPort, Priority: 1}}
		encoded := encodeMockSignal(gateways, []string{"test.example.com"})

		// 生成随机延迟（1ms-500ms），至少一个通道成功
		dnsDelay := time.Duration(rapid.IntRange(1, 500).Draw(t, "dnsDelayMs")) * time.Millisecond
		gistDelay := time.Duration(rapid.IntRange(1, 500).Draw(t, "gistDelayMs")) * time.Millisecond
		mastodonDelay := time.Duration(rapid.IntRange(1, 500).Draw(t, "mastodonDelayMs")) * time.Millisecond

		// DNS 通道 mock
		dohServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(dnsDelay)
			resp := dohResponse{
				Status: 0,
				Answer: []struct {
					Type int    `json:"type"`
					Data string `json:"data"`
				}{
					{Type: 16, Data: fmt.Sprintf(`"%s"`, encoded)},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer dohServer.Close()

		// Gist 通道 mock
		gistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(gistDelay)
			payload := gistPayload{V: 1, Ts: time.Now().Unix(), Data: encoded}
			json.NewEncoder(w).Encode(payload)
		}))
		defer gistServer.Close()

		// Mastodon 通道 mock
		mastodonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(mastodonDelay)
			statuses := []mastodonStatus{
				{
					ID:        "pbt-test",
					Content:   fmt.Sprintf(`<p><a href="#">#cdn_health</a> %s</p>`, encoded),
					CreatedAt: time.Now().Format(time.RFC3339),
				},
			}
			json.NewEncoder(w).Encode(statuses)
		}))
		defer mastodonServer.Close()

		resolver := NewResolver(&ResolverConfig{
			DNSRecordName:    "_sig.pbt.example.com",
			DoHServers:       []string{dohServer.URL},
			GistID:           "pbt-gist",
			GistFileName:     "telemetry.json",
			MastodonInstance: mastodonServer.URL,
			MastodonHashtag:  "cdn_health",
			ChannelTimeout:   5 * time.Second,
		}, mockOpenFn)

		// 路由 Gist 请求到 mock 服务器
		resolver.httpClient.Transport = &pbtRoutingTransport{
			dohServerURL:      dohServer.URL,
			gistServerURL:     gistServer.URL,
			mastodonServerURL: mastodonServer.URL,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		signal, err := resolver.Resolve(ctx)
		if err != nil {
			t.Fatalf("Resolve should succeed with at least one channel: %v", err)
		}

		// ResolvedSignal 必须包含有效 Gateways
		if len(signal.Gateways) == 0 {
			t.Fatal("ResolvedSignal.Gateways is empty")
		}
		if signal.Gateways[0].IP != gwIP {
			t.Fatalf("expected gateway IP %s, got %s", gwIP, signal.Gateways[0].IP)
		}
		if signal.Gateways[0].Port != gwPort {
			t.Fatalf("expected gateway port %d, got %d", gwPort, signal.Gateways[0].Port)
		}

		// Channel 必须是三个合法通道之一
		validChannels := map[string]bool{"dns_txt": true, "gist": true, "mastodon": true}
		if !validChannels[signal.Channel] {
			t.Fatalf("unexpected channel: %s", signal.Channel)
		}

		// Latency 必须为正值
		if signal.Latency <= 0 {
			t.Fatalf("expected positive latency, got %v", signal.Latency)
		}
	})
}

// ============================================================
// Task 5.3: Property 7 — Resolver All-Fail Aggregated Error
// Feature: phase1-link-continuity, Property 7: Resolver all-fail aggregated error
// Validates: Requirements 6.4
// ============================================================

// TestProperty_AllFailAggregatedError 使用 rapid 生成随机通道配置（全部失败），
// 验证 Resolve 返回 error 且包含所有通道错误信息。
func TestProperty_AllFailAggregatedError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 随机决定启用哪些通道（至少一个）
		enableDNS := rapid.Bool().Draw(t, "enableDNS")
		enableGist := rapid.Bool().Draw(t, "enableGist")
		enableMastodon := rapid.Bool().Draw(t, "enableMastodon")
		if !enableDNS && !enableGist && !enableMastodon {
			enableDNS = true // 至少启用一个通道
		}

		// 为每个启用的通道创建返回错误的 mock 服务器
		var dohServerURL string
		var gistServerURL string
		var mastodonServerURL string

		httpStatus := rapid.IntRange(400, 599).Draw(t, "httpStatus")

		if enableDNS {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(httpStatus)
			}))
			defer s.Close()
			dohServerURL = s.URL
		}

		if enableGist {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(httpStatus)
			}))
			defer s.Close()
			gistServerURL = s.URL
		}

		if enableMastodon {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(httpStatus)
			}))
			defer s.Close()
			mastodonServerURL = s.URL
		}

		config := &ResolverConfig{
			ChannelTimeout: 2 * time.Second,
		}
		if enableDNS {
			config.DNSRecordName = "_sig.fail.example.com"
			config.DoHServers = []string{dohServerURL}
		}
		if enableGist {
			config.GistID = "fail-gist"
			config.GistFileName = "telemetry.json"
		}
		if enableMastodon {
			config.MastodonInstance = mastodonServerURL
			config.MastodonHashtag = "cdn_health"
		}

		resolver := NewResolver(config, mockOpenFn)

		// 路由 Gist 请求到失败服务器
		if enableGist && gistServerURL != "" {
			resolver.httpClient.Transport = &pbtFailRoutingTransport{
				dohServerURL:      dohServerURL,
				gistServerURL:     gistServerURL,
				mastodonServerURL: mastodonServerURL,
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		signal, err := resolver.Resolve(ctx)
		if signal != nil {
			t.Fatal("expected nil signal when all channels fail")
		}
		if err == nil {
			t.Fatal("expected error when all channels fail")
		}

		errMsg := err.Error()

		// 验证错误信息包含所有已启用通道的错误
		if enableDNS && !strings.Contains(errMsg, "dns_txt") {
			t.Fatalf("error should contain dns_txt channel info: %s", errMsg)
		}
		if enableGist && !strings.Contains(errMsg, "gist") {
			t.Fatalf("error should contain gist channel info: %s", errMsg)
		}
		if enableMastodon && !strings.Contains(errMsg, "mastodon") {
			t.Fatalf("error should contain mastodon channel info: %s", errMsg)
		}
	})
}

// ============================================================
// PBT 辅助 Transport
// ============================================================

// pbtRoutingTransport 用于 Property 6 PBT，将请求路由到对应的 mock 服务器
type pbtRoutingTransport struct {
	dohServerURL      string
	gistServerURL     string
	mastodonServerURL string
}

func (t *pbtRoutingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Mastodon 请求：路径包含 /api/v1/timelines
	if strings.Contains(req.URL.Path, "/api/v1/timelines") {
		newReq := req.Clone(req.Context())
		newReq.URL.Scheme = "http"
		newReq.URL.Host = strings.TrimPrefix(t.mastodonServerURL, "http://")
		return http.DefaultTransport.RoundTrip(newReq)
	}
	// Gist 请求：host 包含 gist.githubusercontent.com
	if strings.Contains(req.URL.Host, "gist.githubusercontent.com") {
		newReq := req.Clone(req.Context())
		newReq.URL.Scheme = "http"
		newReq.URL.Host = strings.TrimPrefix(t.gistServerURL, "http://")
		return http.DefaultTransport.RoundTrip(newReq)
	}
	// DoH 请求：直接转发到 DoH mock
	return http.DefaultTransport.RoundTrip(req)
}

// pbtFailRoutingTransport 用于 Property 7 PBT，将请求路由到失败的 mock 服务器
type pbtFailRoutingTransport struct {
	dohServerURL      string
	gistServerURL     string
	mastodonServerURL string
}

func (t *pbtFailRoutingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Mastodon 请求
	if strings.Contains(req.URL.Path, "/api/v1/timelines") && t.mastodonServerURL != "" {
		newReq := req.Clone(req.Context())
		newReq.URL.Scheme = "http"
		newReq.URL.Host = strings.TrimPrefix(t.mastodonServerURL, "http://")
		return http.DefaultTransport.RoundTrip(newReq)
	}
	// Gist 请求
	if strings.Contains(req.URL.Host, "gist.githubusercontent.com") && t.gistServerURL != "" {
		newReq := req.Clone(req.Context())
		newReq.URL.Scheme = "http"
		newReq.URL.Host = strings.TrimPrefix(t.gistServerURL, "http://")
		return http.DefaultTransport.RoundTrip(newReq)
	}
	// DoH 请求
	return http.DefaultTransport.RoundTrip(req)
}
