package resonance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ============================================================
// 通道 ①：DNS TXT via DoH（DNS over HTTPS）
// ============================================================
// 绝不使用系统 net.LookupTXT（会被 ISP 投毒/缓存/拦截）
// 强制通过 HTTPS 发往 1.1.1.1/dns-query 或 8.8.8.8/dns-query

// dohResponse RFC 8484 JSON 响应格式
type dohResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Type int    `json:"type"`
		Data string `json:"data"`
	} `json:"Answer"`
}

// resolveDNSTXT 通过 DoH 查询 TXT 记录
func (r *Resolver) resolveDNSTXT(ctx context.Context) (*ResolvedSignal, error) {
	var lastErr error

	// 尝试多个 DoH 服务器（任一成功即返回）
	for _, server := range r.config.DoHServers {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		encoded, err := r.queryDoH(ctx, server, r.config.DNSRecordName)
		if err != nil {
			lastErr = err
			continue
		}

		signal, err := r.decodeAndOpen(encoded)
		if err != nil {
			lastErr = err
			continue
		}

		return signal, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("无可用 DoH 服务器")
}

// queryDoH 执行单次 DoH 查询（RFC 8484 JSON API）
func (r *Resolver) queryDoH(ctx context.Context, server, name string) (string, error) {
	// 使用 JSON API（比 wire-format 更简单，且 Cloudflare/Google 均支持）
	url := fmt.Sprintf("%s?name=%s&type=TXT", server, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/dns-json")
	// 伪装为普通浏览器请求
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("DoH 请求失败 (%s): %w", server, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DoH 返回 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return "", fmt.Errorf("读取 DoH 响应失败: %w", err)
	}

	var doh dohResponse
	if err := json.Unmarshal(body, &doh); err != nil {
		return "", fmt.Errorf("解析 DoH JSON 失败: %w", err)
	}

	if doh.Status != 0 {
		return "", fmt.Errorf("DoH 查询失败: status=%d", doh.Status)
	}

	// 提取 TXT 记录（type=16）
	for _, ans := range doh.Answer {
		if ans.Type == 16 {
			// TXT 记录可能被引号包裹
			data := ans.Data
			if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
				data = data[1 : len(data)-1]
			}
			if data != "" {
				return data, nil
			}
		}
	}

	return "", fmt.Errorf("未找到 TXT 记录: %s", name)
}

// decodeAndOpen Base64 RawURL 解码 → SignalCrypto.OpenSignal
func (r *Resolver) decodeAndOpen(encoded string) (*ResolvedSignal, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("Base64 解码失败: %w", err)
	}

	gateways, domains, err := r.openFn(sealed)
	if err != nil {
		return nil, fmt.Errorf("信令解密/验签失败: %w", err)
	}

	return &ResolvedSignal{
		Gateways: gateways,
		Domains:  domains,
	}, nil
}
