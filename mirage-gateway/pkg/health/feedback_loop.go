// Package health - 用户侧主动逃逸反馈
// 利用真实流量作为最好的探针
package health

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// ClientHeartbeat 客户端心跳
type ClientHeartbeat struct {
	ClientID    string    `json:"client_id"`
	Domain      string    `json:"domain"`
	Timestamp   time.Time `json:"timestamp"`
	RTT         int64     `json:"rtt_ms"`
	Success     bool      `json:"success"`
	ErrorCode   string    `json:"error_code,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	Region      string    `json:"region,omitempty"`
}

// HotSwapSignal 热切换信号
type HotSwapSignal struct {
	Action    string `json:"action"`     // rotate, retry, fallback
	NewDomain string `json:"new_domain,omitempty"`
	Reason    string `json:"reason,omitempty"`
	TTL       int    `json:"ttl_seconds"`
}

// ClientState 客户端状态
type ClientState struct {
	ClientID      string
	LastHeartbeat time.Time
	FailCount     int
	SuccessCount  int
	CurrentDomain string
	IsBlocked     bool
}

// FeedbackLoop 反馈循环
type FeedbackLoop struct {
	mu sync.RWMutex

	// 客户端状态
	clients map[string]*ClientState

	// 心跳通道
	heartbeatChan chan *ClientHeartbeat

	// 域名健康度（基于真实流量）
	domainHealth map[string]float64

	// 回调
	onClientBlocked func(clientID, domain string)
	getDomainFunc   func() string // 获取当前活跃域名

	// 配置
	heartbeatTimeout time.Duration
	blockThreshold   int
	cleanupInterval  time.Duration

	// HTTP 服务
	server *http.Server

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewFeedbackLoop 创建反馈循环
func NewFeedbackLoop() *FeedbackLoop {
	ctx, cancel := context.WithCancel(context.Background())

	return &FeedbackLoop{
		clients:          make(map[string]*ClientState),
		heartbeatChan:    make(chan *ClientHeartbeat, 1000),
		domainHealth:     make(map[string]float64),
		heartbeatTimeout: 30 * time.Second,
		blockThreshold:   3,
		cleanupInterval:  5 * time.Minute,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// SetCallbacks 设置回调
func (fl *FeedbackLoop) SetCallbacks(onBlocked func(string, string), getDomain func() string) {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	fl.onClientBlocked = onBlocked
	fl.getDomainFunc = getDomain
}

// Start 启动反馈循环
func (fl *FeedbackLoop) Start(listenAddr string) error {
	// 启动心跳处理
	fl.wg.Add(1)
	go fl.heartbeatProcessor()

	// 启动清理循环
	fl.wg.Add(1)
	go fl.cleanupLoop()

	// 启动 HTTP 服务
	mux := http.NewServeMux()
	mux.HandleFunc("/health/heartbeat", fl.handleHeartbeat)
	mux.HandleFunc("/health/signal", fl.handleSignal)
	mux.HandleFunc("/health/check.js", fl.handleHealthCheckJS)

	fl.server = &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	fl.wg.Add(1)
	go func() {
		defer fl.wg.Done()
		if err := fl.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("⚠️  反馈循环 HTTP 服务错误: %v", err)
		}
	}()

	log.Printf("📡 用户侧反馈循环已启动 (listen: %s)", listenAddr)
	return nil
}

// Stop 停止反馈循环
func (fl *FeedbackLoop) Stop() {
	fl.cancel()
	if fl.server != nil {
		fl.server.Shutdown(context.Background())
	}
	close(fl.heartbeatChan)
	fl.wg.Wait()
	log.Println("🛑 反馈循环已停止")
}

// handleHeartbeat 处理心跳
func (fl *FeedbackLoop) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var hb ClientHeartbeat
	if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	hb.Timestamp = time.Now()

	// 非阻塞发送
	select {
	case fl.heartbeatChan <- &hb:
	default:
		// 通道满，丢弃
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleSignal 处理信号请求
func (fl *FeedbackLoop) handleSignal(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		http.Error(w, "Missing client_id", http.StatusBadRequest)
		return
	}

	fl.mu.RLock()
	state, exists := fl.clients[clientID]
	getDomain := fl.getDomainFunc
	fl.mu.RUnlock()

	signal := HotSwapSignal{
		Action: "none",
		TTL:    60,
	}

	if exists && state.IsBlocked {
		// 客户端被封锁，下发切换信号
		newDomain := ""
		if getDomain != nil {
			newDomain = getDomain()
		}

		signal = HotSwapSignal{
			Action:    "rotate",
			NewDomain: newDomain,
			Reason:    "domain_blocked",
			TTL:       300,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(signal)
}

// handleHealthCheckJS 返回健康检查脚本
func (fl *FeedbackLoop) handleHealthCheckJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")

	// 极简的健康检查脚本
	script := `(function(){
var cid=localStorage.getItem('_mcid')||Math.random().toString(36).substr(2);
localStorage.setItem('_mcid',cid);
var domain=location.hostname;
var start=Date.now();
fetch('/health/heartbeat',{
method:'POST',
headers:{'Content-Type':'application/json'},
body:JSON.stringify({client_id:cid,domain:domain,success:true,rtt_ms:Date.now()-start})
}).catch(function(){
fetch('/health/signal?client_id='+cid).then(function(r){return r.json()}).then(function(s){
if(s.action==='rotate'&&s.new_domain){location.hostname=s.new_domain}
}).catch(function(){})
});
setInterval(function(){
fetch('/health/heartbeat',{method:'POST',headers:{'Content-Type':'application/json'},
body:JSON.stringify({client_id:cid,domain:domain,success:true,rtt_ms:0})}).catch(function(){});
},25000);
})();`

	w.Write([]byte(script))
}

// heartbeatProcessor 心跳处理器
func (fl *FeedbackLoop) heartbeatProcessor() {
	defer fl.wg.Done()

	for {
		select {
		case <-fl.ctx.Done():
			return
		case hb, ok := <-fl.heartbeatChan:
			if !ok {
				return
			}
			fl.processHeartbeat(hb)
		}
	}
}

// processHeartbeat 处理单个心跳
func (fl *FeedbackLoop) processHeartbeat(hb *ClientHeartbeat) {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	state, exists := fl.clients[hb.ClientID]
	if !exists {
		state = &ClientState{
			ClientID:      hb.ClientID,
			CurrentDomain: hb.Domain,
		}
		fl.clients[hb.ClientID] = state
	}

	state.LastHeartbeat = hb.Timestamp
	state.CurrentDomain = hb.Domain

	if hb.Success {
		state.SuccessCount++
		state.FailCount = 0
		state.IsBlocked = false

		// 更新域名健康度
		fl.domainHealth[hb.Domain] = fl.domainHealth[hb.Domain]*0.9 + 10
		if fl.domainHealth[hb.Domain] > 100 {
			fl.domainHealth[hb.Domain] = 100
		}
	} else {
		state.FailCount++

		// 更新域名健康度
		fl.domainHealth[hb.Domain] = fl.domainHealth[hb.Domain]*0.9 - 5
		if fl.domainHealth[hb.Domain] < 0 {
			fl.domainHealth[hb.Domain] = 0
		}

		// 检测封锁
		if state.FailCount >= fl.blockThreshold {
			state.IsBlocked = true

			// 触发回调
			if fl.onClientBlocked != nil {
				go fl.onClientBlocked(hb.ClientID, hb.Domain)
			}

			log.Printf("🚫 检测到客户端被封锁: client=%s, domain=%s",
				hb.ClientID, hb.Domain)
		}
	}
}

// cleanupLoop 清理循环
func (fl *FeedbackLoop) cleanupLoop() {
	defer fl.wg.Done()

	ticker := time.NewTicker(fl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-fl.ctx.Done():
			return
		case <-ticker.C:
			fl.cleanup()
		}
	}
}

// cleanup 清理过期客户端
func (fl *FeedbackLoop) cleanup() {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	now := time.Now()
	for clientID, state := range fl.clients {
		if now.Sub(state.LastHeartbeat) > 10*time.Minute {
			delete(fl.clients, clientID)
		}
	}
}

// GetDomainHealth 获取域名健康度
func (fl *FeedbackLoop) GetDomainHealth(domain string) float64 {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	return fl.domainHealth[domain]
}

// GetAllDomainHealth 获取所有域名健康度
func (fl *FeedbackLoop) GetAllDomainHealth() map[string]float64 {
	fl.mu.RLock()
	defer fl.mu.RUnlock()

	result := make(map[string]float64)
	for k, v := range fl.domainHealth {
		result[k] = v
	}
	return result
}

// GetBlockedClients 获取被封锁的客户端
func (fl *FeedbackLoop) GetBlockedClients() []string {
	fl.mu.RLock()
	defer fl.mu.RUnlock()

	var blocked []string
	for clientID, state := range fl.clients {
		if state.IsBlocked {
			blocked = append(blocked, clientID)
		}
	}
	return blocked
}

// GetClientCount 获取客户端数量
func (fl *FeedbackLoop) GetClientCount() int {
	fl.mu.RLock()
	defer fl.mu.RUnlock()
	return len(fl.clients)
}

// ReportHeartbeat 手动上报心跳（供内部使用）
func (fl *FeedbackLoop) ReportHeartbeat(clientID, domain string, success bool) {
	hb := &ClientHeartbeat{
		ClientID:  clientID,
		Domain:    domain,
		Timestamp: time.Now(),
		Success:   success,
	}

	select {
	case fl.heartbeatChan <- hb:
	default:
	}
}
