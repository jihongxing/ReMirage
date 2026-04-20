// Mock services for Genesis Drill
// 模拟 DoH / GitHub Gist / Mastodon 三个信令共振通道
// 从 /shared/signal_payload.json 读取当前信令，返回给 Phantom Client
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	mockType := os.Getenv("MOCK_TYPE")
	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = "0.0.0.0:443"
	}

	mux := http.NewServeMux()

	switch mockType {
	case "doh":
		mux.HandleFunc("/dns-query", handleDoH)
		log.Printf("[Mock DoH] 启动: %s", listenAddr)
	case "gist":
		mux.HandleFunc("/", handleGist)
		log.Printf("[Mock Gist] 启动: %s", listenAddr)
	case "mastodon":
		mux.HandleFunc("/api/v1/timelines/tag/", handleMastodon)
		mux.HandleFunc("/api/v1/statuses", handleMastodonPost)
		log.Printf("[Mock Mastodon] 启动: %s", listenAddr)
	default:
		log.Fatalf("未知 MOCK_TYPE: %s", mockType)
	}

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	// 使用 HTTP（混沌测试环境不需要真实 TLS）
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// getSignalEncoded 从共享文件读取信令并 Base64 编码
func getSignalEncoded() string {
	data, err := os.ReadFile("/shared/signal_payload.json")
	if err != nil {
		// 返回默认空信令
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

// ============================================================
// DoH 模拟（RFC 8484 JSON API）
// ============================================================
func handleDoH(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	qtype := r.URL.Query().Get("type")

	log.Printf("[DoH] 查询: name=%s, type=%s", name, qtype)

	encoded := getSignalEncoded()
	if encoded == "" {
		// 无信令时返回空响应
		w.Header().Set("Content-Type", "application/dns-json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"Status": 3, // NXDOMAIN
			"Answer": []interface{}{},
		})
		return
	}

	resp := map[string]interface{}{
		"Status": 0,
		"Answer": []map[string]interface{}{
			{
				"name": name,
				"type": 16, // TXT
				"TTL":  60,
				"data": fmt.Sprintf(`"%s"`, encoded),
			},
		},
	}

	w.Header().Set("Content-Type", "application/dns-json")
	json.NewEncoder(w).Encode(resp)
}

// ============================================================
// GitHub Gist 模拟
// ============================================================
func handleGist(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Gist] 请求: %s", r.URL.Path)

	encoded := getSignalEncoded()
	if encoded == "" {
		http.Error(w, "not found", 404)
		return
	}

	payload := map[string]interface{}{
		"v":    1,
		"ts":   time.Now().Unix(),
		"data": encoded,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

// ============================================================
// Mastodon 模拟
// ============================================================
func handleMastodon(w http.ResponseWriter, r *http.Request) {
	// 提取 hashtag
	parts := strings.Split(r.URL.Path, "/")
	hashtag := ""
	if len(parts) > 0 {
		hashtag = parts[len(parts)-1]
	}
	log.Printf("[Mastodon] 搜索 hashtag: %s", hashtag)

	encoded := getSignalEncoded()
	if encoded == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	statuses := []map[string]interface{}{
		{
			"id":         "mock-toot-001",
			"content":    fmt.Sprintf(`<p><a href="#">#%s</a> %s</p>`, hashtag, encoded),
			"created_at": time.Now().Format(time.RFC3339),
			"visibility": "unlisted",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statuses)
}

// handleMastodonPost 处理 OS 侧的 Toot 发布（模拟）
func handleMastodonPost(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		log.Printf("[Mastodon] 收到 Toot 发布请求")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id": fmt.Sprintf("toot-%d", time.Now().UnixNano()),
		})
		return
	}
	http.Error(w, "method not allowed", 405)
}
