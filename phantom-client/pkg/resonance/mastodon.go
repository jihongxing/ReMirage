package resonance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ============================================================
// 通道 ③：Mastodon Hashtag Search
// ============================================================
// OS 侧以 unlisted 可见性发布 Toot：#cdn_health <base64_signal>
// 客户端侧：搜索 hashtag → 取最新 Toot → 提取信令部分 → 解密

// mastodonStatus Mastodon API 状态响应（精简）
type mastodonStatus struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// resolveMastodon 从 Mastodon hashtag 时间线拉取信令
func (r *Resolver) resolveMastodon(ctx context.Context) (*ResolvedSignal, error) {
	// Mastodon Public API: GET /api/v1/timelines/tag/:hashtag?limit=1
	// 无需认证即可访问公开 hashtag 时间线
	url := fmt.Sprintf("%s/api/v1/timelines/tag/%s?limit=5",
		r.config.MastodonInstance, r.config.MastodonHashtag)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Mastodon 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Mastodon 返回 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32768))
	if err != nil {
		return nil, fmt.Errorf("读取 Mastodon 响应失败: %w", err)
	}

	var statuses []mastodonStatus
	if err := json.Unmarshal(body, &statuses); err != nil {
		return nil, fmt.Errorf("解析 Mastodon JSON 失败: %w", err)
	}

	if len(statuses) == 0 {
		return nil, fmt.Errorf("Mastodon hashtag #%s 无结果", r.config.MastodonHashtag)
	}

	// 遍历最新的 Toot，尝试提取信令
	for _, status := range statuses {
		encoded := extractSignalFromToot(status.Content, r.config.MastodonHashtag)
		if encoded == "" {
			continue
		}

		signal, err := r.decodeAndOpen(encoded)
		if err != nil {
			// 可能是旧信令（过期/重放），继续尝试下一条
			continue
		}
		return signal, nil
	}

	return nil, fmt.Errorf("Mastodon: 未找到有效信令")
}

// extractSignalFromToot 从 Toot 内容中提取 Base64 信令
// Toot 格式：#cdn_health <base64_encoded_signal>
// Mastodon API 返回的 content 是 HTML，需要剥离标签
func extractSignalFromToot(htmlContent, hashtag string) string {
	// 简易 HTML 标签剥离（Mastodon 返回 <p>...<a href="...">#tag</a> signal</p>）
	text := stripHTML(htmlContent)
	text = strings.TrimSpace(text)

	// 查找 hashtag 后的内容
	// 格式可能是 "#cdn_health SIGNAL" 或 "cdn_health SIGNAL"
	patterns := []string{
		"#" + hashtag + " ",
		hashtag + " ",
	}

	for _, prefix := range patterns {
		idx := strings.Index(strings.ToLower(text), strings.ToLower(prefix))
		if idx >= 0 {
			signal := strings.TrimSpace(text[idx+len(prefix):])
			// Base64 RawURL 字符集验证（A-Z, a-z, 0-9, -, _）
			if isValidBase64RawURL(signal) && len(signal) > 50 {
				return signal
			}
		}
	}

	// Fallback：取最后一个空格后的长字符串
	parts := strings.Fields(text)
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if isValidBase64RawURL(last) && len(last) > 50 {
			return last
		}
	}

	return ""
}

// stripHTML 简易 HTML 标签剥离
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ') // 标签替换为空格
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isValidBase64RawURL 验证字符串是否为合法的 Base64 RawURL 编码
func isValidBase64RawURL(s string) bool {
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
