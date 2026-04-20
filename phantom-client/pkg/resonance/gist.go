package resonance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ============================================================
// 通道 ②：GitHub Gist
// ============================================================
// OS 侧将信令包装为 {"v":1,"ts":...,"data":"<base64>"} 写入 Gist
// 客户端侧：GET raw Gist → 解析 JSON → 提取 data 字段 → 解密

// gistPayload Gist 中的伪装 JSON 结构
type gistPayload struct {
	V    int    `json:"v"`
	Ts   int64  `json:"ts"`
	Data string `json:"data"`
}

// resolveGist 从 GitHub Gist 拉取信令
func (r *Resolver) resolveGist(ctx context.Context) (*ResolvedSignal, error) {
	// GitHub Raw URL（无需认证即可访问公开 Gist）
	fileName := r.config.GistFileName
	if fileName == "" {
		fileName = "telemetry.json"
	}
	url := fmt.Sprintf("https://gist.githubusercontent.com/raw/%s/%s", r.config.GistID, fileName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// 伪装为普通浏览器
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gist 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gist 返回 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if err != nil {
		return nil, fmt.Errorf("读取 Gist 响应失败: %w", err)
	}

	var payload gistPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析 Gist JSON 失败: %w", err)
	}

	if payload.Data == "" {
		return nil, fmt.Errorf("Gist data 字段为空")
	}

	return r.decodeAndOpen(payload.Data)
}
