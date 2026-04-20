// chaos-harness — Phantom Client 混沌测试 Harness
// 暴露 HTTP 状态 API 供 genesis-drill 脚本查询
// 模拟 G-Tunnel 连接并报告传输协议状态
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	state = &ClientState{
		Transport: "quic",
		Status:    "connecting",
	}
)

// ClientState 客户端状态
type ClientState struct {
	mu        sync.RWMutex
	GatewayIP string `json:"gateway_ip"`
	Transport string `json:"transport"`
	Status    string `json:"status"`
	Connected bool   `json:"connected"`
	TxBytes   int64  `json:"tx_bytes"`
	RxBytes   int64  `json:"rx_bytes"`
	txCounter atomic.Int64
	rxCounter atomic.Int64
}

func (s *ClientState) SetConnected(ip, transport string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.GatewayIP = ip
	s.Transport = transport
	s.Status = "connected"
	s.Connected = true
}

func (s *ClientState) SetDisconnected() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = "dead"
	s.Connected = false
	s.GatewayIP = ""
}

func (s *ClientState) SetTransport(t string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Transport = t
}

func (s *ClientState) Snapshot() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]interface{}{
		"gateway_ip": s.GatewayIP,
		"transport":  s.Transport,
		"status":     s.Status,
		"connected":  s.Connected,
		"tx_bytes":   s.txCounter.Load(),
		"rx_bytes":   s.rxCounter.Load(),
	}
}

func main() {
	bootstrapGW := os.Getenv("BOOTSTRAP_GATEWAY")
	if bootstrapGW == "" {
		bootstrapGW = "10.99.0.20:443"
	}

	// 提取 Gateway IP
	gwHost, _, _ := net.SplitHostPort(bootstrapGW)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动连接探测
	go connectLoop(ctx, gwHost)

	// 启动 HTTP 状态 API
	mux := http.NewServeMux()
	mux.HandleFunc("/status", handleStatus)
	mux.HandleFunc("/test/send", handleTestSend)

	server := &http.Server{Addr: ":9090", Handler: mux}
	go func() {
		log.Printf("[chaos-harness] 状态 API 监听 :9090")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancel()
	server.Close()
}

// connectLoop 持续探测 Gateway 连接状态
func connectLoop(ctx context.Context, gwIP string) {
	// 初始等待 Gateway 启动
	time.Sleep(5 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 尝试 TCP 连接到 Gateway（模拟 QUIC 握手）
		conn, err := net.DialTimeout("tcp", gwIP+":443", 3*time.Second)
		if err != nil {
			// 尝试备用端口
			conn, err = net.DialTimeout("tcp", gwIP+":50847", 3*time.Second)
		}

		if err == nil {
			conn.Close()
			state.SetConnected(gwIP, "quic")
		} else {
			// 检查是否能通过其他方式连接
			state.SetDisconnected()
		}

		time.Sleep(3 * time.Second)
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state.Snapshot())
}

func handleTestSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Bytes int64 `json:"bytes"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Bytes <= 0 {
		req.Bytes = 1024
	}

	if !state.Connected {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "reason": "not connected"})
		return
	}

	state.txCounter.Add(req.Bytes)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "sent",
		"bytes":    req.Bytes,
		"tx_total": state.txCounter.Load(),
	})
	fmt.Fprintf(os.Stderr, "[chaos-harness] sent %d bytes\n", req.Bytes)
}
