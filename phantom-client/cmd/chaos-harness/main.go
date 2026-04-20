// chaos-harness — Phantom Client 混沌测试 Harness
// 暴露 HTTP 状态 API 供 genesis-drill 脚本查询
// 模拟 G-Tunnel 多路径连接：UDP(QUIC) → TCP(WSS) → 信令共振发现
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
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var state = &ClientState{
	Transport: "unknown",
	Status:    "connecting",
}

// ClientState 客户端状态
type ClientState struct {
	mu        sync.RWMutex
	GatewayIP string
	Transport string
	Status    string
	Connected bool
	txCounter atomic.Int64
	rxCounter atomic.Int64
}

func (s *ClientState) Set(ip, transport, status string, connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.GatewayIP = ip
	s.Transport = transport
	s.Status = status
	s.Connected = connected
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
	gwHost, _, _ := net.SplitHostPort(bootstrapGW)

	// 备用 Gateway 列表（从环境变量或信令共振获取）
	backupGateways := []string{"10.99.0.30"}
	if env := os.Getenv("BACKUP_GATEWAYS"); env != "" {
		backupGateways = strings.Split(env, ",")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go connectLoop(ctx, gwHost, backupGateways)

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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancel()
	server.Close()
}

// probeUDP 探测 UDP 连通性（模拟 QUIC）
func probeUDP(host string, port string, timeout time.Duration) bool {
	addr, err := net.ResolveUDPAddr("udp", host+":"+port)
	if err != nil {
		return false
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return false
	}
	defer conn.Close()
	// 发送一个探测包，检查是否被 iptables DROP
	conn.SetDeadline(time.Now().Add(timeout))
	// 发送 QUIC Initial 风格的探测
	conn.Write([]byte{0xc0, 0x00, 0x00, 0x01})
	buf := make([]byte, 64)
	_, err = conn.Read(buf)
	// UDP 是无连接的，如果没被 DROP，Write 会成功
	// 但如果 iptables DROP 了出站 UDP，Write 也会成功（本地缓冲）
	// 所以我们用 ICMP unreachable 来判断：如果端口不可达会收到错误
	// 最可靠的方式：尝试 TCP 到同一端口作为 fallback 判断
	if err != nil {
		// 超时 = 可能被 DROP（无 ICMP 回复）
		return false
	}
	return true
}

// probeTCP 探测 TCP 连通性（模拟 WSS/H2）
func probeTCP(host string, port string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", host+":"+port, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// connectLoop 多路径探测循环
// 优先级：UDP:443(QUIC) → TCP:443(WSS) → TCP:50847(gRPC) → 备用 Gateway
func connectLoop(ctx context.Context, primaryGW string, backupGWs []string) {
	time.Sleep(3 * time.Second) // 等待 Gateway 启动

	currentGW := primaryGW
	failCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 探测当前 Gateway
		if probeUDP(currentGW, "443", 2*time.Second) {
			state.Set(currentGW, "quic", "connected", true)
			failCount = 0
			time.Sleep(2 * time.Second)
			continue
		}

		if probeTCP(currentGW, "443", 2*time.Second) {
			state.Set(currentGW, "wss", "connected", true)
			failCount = 0
			time.Sleep(2 * time.Second)
			continue
		}

		if probeTCP(currentGW, "50847", 2*time.Second) {
			state.Set(currentGW, "tcp", "connected", true)
			failCount = 0
			time.Sleep(2 * time.Second)
			continue
		}

		// 当前 Gateway 不可达
		failCount++

		if failCount >= 2 {
			// 尝试备用 Gateway（信令共振）
			switched := false
			candidates := backupGWs
			// 如果当前不是主 Gateway，也尝试回主 Gateway
			if currentGW != primaryGW {
				candidates = append([]string{primaryGW}, backupGWs...)
			}
			for _, bkGW := range candidates {
				if bkGW == currentGW {
					continue
				}
				if probeTCP(bkGW, "443", 2*time.Second) || probeTCP(bkGW, "50847", 2*time.Second) {
					log.Printf("[chaos-harness] 信令共振：切换到 Gateway %s", bkGW)
					currentGW = bkGW
					state.Set(bkGW, "quic", "connected", true)
					failCount = 0
					switched = true
					break
				}
			}
			if !switched {
				state.Set("", "dead", "dead", false)
			}
		} else {
			state.Set(currentGW, "dead", "dead", false)
		}

		time.Sleep(2 * time.Second)
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

	state.mu.RLock()
	connected := state.Connected
	state.mu.RUnlock()

	if !connected {
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
