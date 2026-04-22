package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpClient 全局 HTTP 客户端
var httpClient = &http.Client{Timeout: 10 * time.Second}

// gatewayGet 向 Gateway 发起 GET 请求
func gatewayGet(path string) ([]byte, error) {
	url := fmt.Sprintf("http://%s%s", gatewayAddr, path)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("无法连接 Gateway (%s): %w", gatewayAddr, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode >= 400 {
		return body, fmt.Errorf("Gateway 返回 HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// gatewayPost 向 Gateway 发起 POST 请求
func gatewayPost(path string, contentType string, payload io.Reader) ([]byte, error) {
	url := fmt.Sprintf("http://%s%s", gatewayAddr, path)
	resp, err := httpClient.Post(url, contentType, payload)
	if err != nil {
		return nil, fmt.Errorf("无法连接 Gateway (%s): %w", gatewayAddr, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode >= 400 {
		return body, fmt.Errorf("Gateway 返回 HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// printJSON 格式化输出 JSON
func printJSON(data interface{}) {
	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("%v\n", data)
		return
	}
	fmt.Println(string(formatted))
}

// printRawJSON 格式化输出原始 JSON 字节
func printRawJSON(body []byte) {
	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		fmt.Println(string(body))
		return
	}
	formatted, _ := json.MarshalIndent(raw, "", "  ")
	fmt.Println(string(formatted))
}

// humanDuration 人类可读时长
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}
