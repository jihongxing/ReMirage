package resonance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
