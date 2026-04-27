package gtclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"phantom-client/pkg/resonance"
	"phantom-client/pkg/token"

	"golang.org/x/crypto/chacha20poly1305"
)

// RouteTable holds in-memory gateway node list.
type RouteTable struct {
	nodes   []token.GatewayEndpoint
	mu      sync.RWMutex
	updated time.Time
}

// Update replaces the node list.
func (rt *RouteTable) Update(nodes []token.GatewayEndpoint) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.nodes = make([]token.GatewayEndpoint, len(nodes))
	copy(rt.nodes, nodes)
	rt.updated = time.Now()
}

// NextAvailable returns the next node excluding the given IP.
func (rt *RouteTable) NextAvailable(exclude string) (token.GatewayEndpoint, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, n := range rt.nodes {
		if n.IP != exclude {
			return n, nil
		}
	}
	return token.GatewayEndpoint{}, fmt.Errorf("no available gateway")
}

// Count returns the number of nodes.
func (rt *RouteTable) Count() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.nodes)
}

// probeResult holds the result of a successful probe.
type probeResult struct {
	gw     token.GatewayEndpoint
	engine *QUICEngine
}

// Transport 统一传输接口，用于将 GTunnelClient 从单一 QUICEngine 解耦。
// 当注入 Transport 后，Send/Receive 走 Transport 而非直接走 quic。
// 这是 Orchestrator 接入 Client 的桥接点（审计 spec A-01）。
type Transport interface {
	SendDatagram(data []byte) error
	ReceiveDatagram(ctx context.Context) ([]byte, error)
	IsConnected() bool
	Close() error
}

// QUICTransportAdapter 将 QUICEngine 适配为 Transport 接口。
// 在 ProbeAndConnect 成功后自动注入，确保 Send/Receive 走统一 Transport 路径。
type QUICTransportAdapter struct {
	engine *QUICEngine
}

// NewQUICTransportAdapter 创建 QUICEngine 的 Transport 适配器。
func NewQUICTransportAdapter(engine *QUICEngine) *QUICTransportAdapter {
	return &QUICTransportAdapter{engine: engine}
}

func (a *QUICTransportAdapter) SendDatagram(data []byte) error {
	return a.engine.SendDatagram(data)
}

func (a *QUICTransportAdapter) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	return a.engine.ReceiveDatagram(ctx)
}

func (a *QUICTransportAdapter) IsConnected() bool {
	return a.engine.IsConnected()
}

func (a *QUICTransportAdapter) Close() error {
	return a.engine.Close()
}

// GTunnelClient manages the G-Tunnel connection to a gateway.
type GTunnelClient struct {
	config      *token.BootstrapConfig
	fec         *FECCodec
	sampler     *OverlapSampler
	reassembler *Reassembler
	routeTable  *RouteTable
	currentGW   token.GatewayEndpoint
	psk         []byte
	switchFn    func(newIP string)
	quic        *QUICEngine
	transport   Transport    // 注入后替代 quic，用于 Orchestrator 接入
	state       atomic.Int32 // ConnState
	reconnMu    sync.Mutex
	mu          sync.RWMutex
	recvCancel  context.CancelFunc

	// 双池分离：启动种子池（只读）+ 运行时拓扑池（可更新）
	bootstrapPool []token.GatewayEndpoint // 来自 token/URI，整个生命周期不变
	runtimeTopo   *RuntimeTopology        // 来自 PullRouteTable，可更新

	// 退化等级跟踪
	degradationLevel atomic.Int32                 // DegradationLevel
	degradationAt    time.Time                    // 进入当前退化等级的时间
	onDegradation    func(event DegradationEvent) // 退化事件回调

	// 事务切换回调
	switchPreAddFn   func(string) error
	switchCommitFn   func(string, string)
	switchRollbackFn func(string)

	// 信令共振发现器（绝境复活）
	resonance *resonance.Resolver

	// WSS 降级配置（可选，由上层注入）
	wssConfig *WSSOverrideConfig

	// 拓扑刷新器引用（用于绝境发现成功后立即触发拉取）
	topoRefresher *TopoRefresher

	// NIC 检测器（注入后用于 probe() 传递给 QUICEngine）
	nicDetector NICDetector

	// Send-path shim：加密后、SendDatagram 前的 Padding + IAT 控制层
	sendShim *SendPathShim
}

// NewGTunnelClient creates a new G-Tunnel client.
func NewGTunnelClient(config *token.BootstrapConfig) *GTunnelClient {
	fec, _ := NewFECCodec(8, 4)
	sampler := NewOverlapSampler()

	// Copy bootstrap pool (immutable snapshot)
	bp := make([]token.GatewayEndpoint, len(config.BootstrapPool))
	copy(bp, config.BootstrapPool)

	c := &GTunnelClient{
		config:        config,
		fec:           fec,
		sampler:       sampler,
		reassembler:   NewReassembler(fec, sampler),
		routeTable:    &RouteTable{},
		bootstrapPool: bp,
		runtimeTopo:   &RuntimeTopology{},
		psk:           config.PreSharedKey,
	}
	c.state.Store(int32(StateInit))

	// 尝试从本地缓存加载路由表作为初始拓扑
	if config.CachePath != "" {
		cache := NewTopoCache(config.CachePath)
		resp, err := cache.Load()
		if err == nil && len(resp.Gateways) > 0 {
			c.runtimeTopo.Update(resp.Gateways, resp.Version, resp.PublishedAt)
			log.Printf("[GTunnel] 从缓存加载 %d 个节点作为初始拓扑", len(resp.Gateways))
		}
	}

	return c
}

// transition atomically switches state and logs the change.
func (c *GTunnelClient) transition(newState ConnState, reason string) {
	old := ConnState(c.state.Swap(int32(newState)))
	if old != newState {
		log.Printf("[GTunnel] %s → %s (%s)", old, newState, reason)
	}
}

// State returns the current connection state.
func (c *GTunnelClient) State() ConnState {
	return ConnState(c.state.Load())
}

// IsConnected returns connection status.
func (c *GTunnelClient) IsConnected() bool {
	return c.State() == StateConnected
}

// probeFirst concurrently probes gateways, returns the first successful candidate.
func (c *GTunnelClient) probeFirst(ctx context.Context, pool []token.GatewayEndpoint) (*probeResult, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ch := make(chan *probeResult, 1)

	for _, gw := range pool {
		go func(gw token.GatewayEndpoint) {
			engine, err := c.probe(probeCtx, gw)
			if err != nil {
				return
			}
			select {
			case ch <- &probeResult{gw: gw, engine: engine}:
				cancel() // 取消其余探测
			default:
				engine.Close() // 已有胜者，关闭多余连接
			}
		}(gw)
	}

	select {
	case result := <-ch:
		return result, nil
	case <-probeCtx.Done():
		return nil, fmt.Errorf("all probes failed or timeout")
	}
}

// adoptConnection atomically takes over a new connection.
// 如果 transport 是 ClientOrchestrator，更新其内部活跃传输；
// 否则直接设置 transport 为新的 QUICTransportAdapter。
func (c *GTunnelClient) adoptConnection(result *probeResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.quic != nil && c.quic != result.engine {
		c.quic.Close()
	}
	c.quic = result.engine
	c.currentGW = result.gw

	// 如果已有 ClientOrchestrator，通过它管理传输切换
	if orch, ok := c.transport.(*ClientOrchestrator); ok {
		newTransport := NewQUICTransportAdapter(result.engine)
		orch.mu.Lock()
		old := orch.active
		orch.active = newTransport
		orch.activeType = "quic"
		orch.mu.Unlock()
		// 关闭旧传输（可能是 WSS fallback 或旧 QUIC 适配器）
		// QUICTransportAdapter.Close() 会关闭底层 QUICEngine，
		// 但 engine 已在上面被替换，所以只关闭旧适配器持有的旧 engine
		if old != nil && old != newTransport {
			old.Close()
		}
		// 重建 sendShim 指向 Orchestrator 的 SendDatagram
		c.rebuildSendShim(orch.SendDatagram)
	} else {
		// 没有 Orchestrator，直接设置 transport
		c.transport = NewQUICTransportAdapter(result.engine)
		// 重建 sendShim 指向新 transport 的 SendDatagram
		c.rebuildSendShim(c.transport.SendDatagram)
	}

	c.transition(StateConnected, fmt.Sprintf("adopted %s", result.gw.IP))
}

// switchWithTransaction performs atomic connection + route switch.
// Sequence: ① PreAdd new route → ② adoptConnection → ③ CommitSwitch (delete old route)
func (c *GTunnelClient) switchWithTransaction(result *probeResult, oldIP string) error {
	newIP := result.gw.IP
	if newIP == oldIP {
		c.adoptConnection(result)
		return nil
	}

	// Step 1: 预加新路由
	if c.switchPreAddFn != nil {
		if err := c.switchPreAddFn(newIP); err != nil {
			result.engine.Close()
			return fmt.Errorf("pre-add route failed: %w", err)
		}
	}

	// Step 2: 接管连接
	c.adoptConnection(result)

	// Step 3: 提交路由（删除旧路由）
	if c.switchCommitFn != nil {
		c.switchCommitFn(oldIP, newIP)
	}

	return nil
}

// ProbeAndConnect concurrently probes bootstrap nodes, connects to first responder.
// 通过 ClientOrchestrator 建立连接：先尝试 QUIC，超时后降级到 WSS，降级后后台探测回升。
// ClientOrchestrator 实现 Transport 接口，自动注入为 GTunnelClient 的统一传输层。
func (c *GTunnelClient) ProbeAndConnect(ctx context.Context, pool []token.GatewayEndpoint) error {
	if len(pool) == 0 {
		return fmt.Errorf("empty bootstrap pool")
	}

	// 构建 QUIC DialFunc：并发探测 bootstrap pool，返回第一个成功的 QUICEngine
	quicDial := func(dialCtx context.Context) (Transport, error) {
		result, err := c.probeFirst(dialCtx, pool)
		if err != nil {
			return nil, err
		}
		// 更新内部状态（gateway、quic 引用）
		c.mu.Lock()
		if c.quic != nil {
			c.quic.Close()
		}
		c.quic = result.engine
		c.currentGW = result.gw
		c.mu.Unlock()
		return NewQUICTransportAdapter(result.engine), nil
	}

	// 构建 WSS 降级 DialFunc
	// WSS 降级通过 gorilla/websocket + mTLS 与 Gateway ChameleonListener 对接。
	// 连接参数从 bootstrap config 获取；证书路径由上层配置注入。
	var wssDial func(ctx context.Context) (Transport, error)
	if len(pool) > 0 {
		wssCfg := WSSTransportConfig{
			Addr:   fmt.Sprintf("%s:8443", pool[0].IP),
			SNI:    "cdn.cloudflare.com",
			WSPath: "/api/v2/stream",
		}
		// 如果上层注入了 WSS 配置，优先使用
		if c.wssConfig != nil {
			if c.wssConfig.WSSSNI != "" {
				wssCfg.SNI = c.wssConfig.WSSSNI
			}
			if c.wssConfig.WSSPath != "" {
				wssCfg.WSPath = c.wssConfig.WSSPath
			}
			if c.wssConfig.WSSPort != 0 {
				wssCfg.Addr = fmt.Sprintf("%s:%d", pool[0].IP, c.wssConfig.WSSPort)
			}
			wssCfg.CertFile = c.wssConfig.WSSCertFile
			wssCfg.KeyFile = c.wssConfig.WSSKeyFile
			wssCfg.CAFile = c.wssConfig.WSSCAFile
		}
		wssDial = func(dialCtx context.Context) (Transport, error) {
			return NewWSSTransport(dialCtx, wssCfg)
		}
	}

	// 构建 ClientOrchestrator
	orch := NewClientOrchestrator(ClientOrchestratorConfig{
		QUICDial:         quicDial,
		WSSDial:          wssDial,
		FallbackTimeout:  10 * time.Second,
		ProbeInterval:    30 * time.Second,
		PromoteThreshold: 3,
	})

	// 通过 Orchestrator 建立连接
	if err := orch.Connect(ctx); err != nil {
		return err
	}

	// 注入 Orchestrator 作为统一传输层
	c.mu.Lock()
	c.transport = orch
	// 初始化 sendShim 指向 Orchestrator 的 SendDatagram
	c.rebuildSendShim(orch.SendDatagram)
	c.mu.Unlock()

	c.transition(StateConnected, fmt.Sprintf("orchestrator→%s via %s", c.currentGW.IP, orch.ActiveType()))
	return nil
}

// probe attempts to connect to a single gateway via QUIC Datagram.
// Returns the QUICEngine on success (caller is responsible for closing on discard).
func (c *GTunnelClient) probe(ctx context.Context, gw token.GatewayEndpoint) (*QUICEngine, error) {
	addr := fmt.Sprintf("%s:%d", gw.IP, gw.Port)

	// 解码证书指纹为 PinnedCertHash
	var pinnedHash []byte
	if c.config.CertFingerprint != "" {
		decoded, err := hex.DecodeString(c.config.CertFingerprint)
		if err == nil && len(decoded) == 32 {
			pinnedHash = decoded
		} else {
			log.Printf("[GTunnel] ⚠️ CertFingerprint 解码失败或长度不为 32，证书钉扎未生效")
		}
	}

	// NIC 检测器降级：未注入时使用 legacy fallback
	nicDet := c.nicDetector
	if nicDet == nil {
		log.Printf("[GTunnel] ⚠️ NICDetector 未注入，使用 legacyDetectOutbound fallback")
	}

	engine := NewQUICEngine(&QUICEngineConfig{
		GatewayAddr:    addr,
		PinnedCertHash: pinnedHash,
		NICDetector:    nicDet,
	})

	if err := engine.Connect(ctx); err != nil {
		return nil, err
	}

	return engine, nil
}

// Send encrypts and sends a packet through the tunnel via QUIC Datagram.
// Pipeline: IP packet → overlap split → FEC encode → ChaCha20 encrypt → QUIC Datagram
// If a Transport is injected (via SetTransport), uses it instead of the raw QUICEngine.
func (c *GTunnelClient) Send(packet []byte) error {
	if len(packet) == 0 {
		return nil
	}

	// Resolve transport: injected Transport > legacy QUICEngine
	c.mu.RLock()
	transport := c.transport
	quicEngine := c.quic
	c.mu.RUnlock()

	if transport != nil {
		if !transport.IsConnected() {
			return fmt.Errorf("tunnel not connected")
		}
	} else if quicEngine == nil || !quicEngine.IsConnected() {
		return fmt.Errorf("tunnel not connected")
	}

	// 1. Overlap sampling split
	fragments := c.sampler.Split(packet)
	fragCount := len(fragments)

	// 2. For each fragment: FEC encode → encrypt → send
	for _, frag := range fragments {
		shards, err := c.fec.Encode(frag.Data)
		if err != nil {
			return fmt.Errorf("fec encode: %w", err)
		}

		// 3. Encrypt and fire each shard with 12-byte extended header
		for shardIdx, shard := range shards {
			header := EncodeShardHeader(frag.SeqNum, len(frag.Data), frag.OverlapID, shardIdx, fragCount)
			payload := append(header, shard...)

			encrypted, err := c.encrypt(payload)
			if err != nil {
				continue // skip this shard on encrypt failure
			}

			// 4. Fire via SendPathShim (Padding + IAT) → actual transport
			var sendErr error
			if c.sendShim != nil {
				sendErr = c.sendShim.Send(encrypted)
			} else if transport != nil {
				sendErr = transport.SendDatagram(encrypted)
			} else {
				sendErr = quicEngine.SendDatagram(encrypted)
			}
			if sendErr != nil {
				// Congestion or buffer full — drop and let FEC recover
				continue
			}
		}
	}

	return nil
}

// Receive reads and decrypts a packet from the tunnel.
// Pipeline: QUIC Datagram → decrypt → Reassembler (shard buffer → FEC decode → overlap reassemble)
// Returns a complete reassembled IP packet.
// If a Transport is injected (via SetTransport), uses it instead of the raw QUICEngine.
func (c *GTunnelClient) Receive(ctx context.Context) ([]byte, error) {
	// Resolve transport: injected Transport > legacy QUICEngine
	c.mu.RLock()
	transport := c.transport
	quicEngine := c.quic
	c.mu.RUnlock()

	if transport != nil {
		if !transport.IsConnected() {
			return nil, fmt.Errorf("tunnel not connected")
		}
	} else if quicEngine == nil || !quicEngine.IsConnected() {
		return nil, fmt.Errorf("tunnel not connected")
	}

	// Check if reassembler already has a completed packet
	select {
	case pkt := <-c.reassembler.Completed():
		return pkt, nil
	default:
	}

	// Feed shards into reassembler until a complete packet emerges
	for {
		select {
		case pkt := <-c.reassembler.Completed():
			return pkt, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Read single datagram (one encrypted shard)
		var msg []byte
		var err error
		if transport != nil {
			msg, err = transport.ReceiveDatagram(ctx)
		} else {
			msg, err = quicEngine.ReceiveDatagram(ctx)
		}
		if err != nil {
			return nil, err
		}

		// Decrypt
		plaintext, err := c.decrypt(msg)
		if err != nil {
			continue // corrupted shard, skip
		}

		// Feed into reassembler (12-byte header + shard data)
		if len(plaintext) > 12 {
			c.reassembler.IngestShard(plaintext)
		}

		// Check if a packet was completed
		select {
		case pkt := <-c.reassembler.Completed():
			return pkt, nil
		default:
			// Need more shards, continue loop
		}
	}
}

// PullRouteTable fetches the dynamic route table through the tunnel.
func (c *GTunnelClient) PullRouteTable(ctx context.Context) error {
	if c.topoRefresher == nil {
		log.Println("[GTunnel] PullRouteTable: topoRefresher 未注入，跳过")
		return nil
	}

	if err := c.topoRefresher.PullOnce(ctx); err != nil {
		log.Printf("[GTunnel] PullRouteTable 失败: %v", err)
		return err
	}

	log.Printf("[GTunnel] PullRouteTable 成功: %d 个节点", c.runtimeTopo.Count())
	return nil
}

// Reconnect attempts failover to next available gateway within 5s.
// 单飞保护：同一时刻只允许一个重连流程执行。
func (c *GTunnelClient) Reconnect(ctx context.Context) error {
	if c.State() == StateStopped {
		return fmt.Errorf("client stopped")
	}
	if c.State() == StateConnected {
		return nil
	}

	// 单飞：如果已在重连中，等待结果
	c.reconnMu.Lock()
	if c.State() == StateReconnecting {
		c.reconnMu.Unlock()
		return c.waitReconnComplete(ctx)
	}
	c.transition(StateReconnecting, "reconnect triggered")
	disconnectStart := time.Now()
	c.reconnMu.Unlock()

	// 使用 RecoveryFSM 进行恢复
	fsm := NewRecoveryFSM()
	disconnectDuration := time.Since(disconnectStart)
	phase := fsm.Evaluate(disconnectDuration)

	result, err := fsm.Execute(ctx, phase, c)
	if err != nil {
		c.transition(StateExhausted, err.Error())
		if result != nil {
			log.Printf("[GTunnel] 恢复失败: phase=%s, duration=%v, attempts=%d",
				result.Phase, result.Duration, result.Attempts)
		}
		return err
	}

	// 恢复成功
	c.transition(StateConnected, fmt.Sprintf("recovered via %s", result.Phase))
	return nil
}

// waitReconnComplete polls state until reconnection finishes or ctx expires.
func (c *GTunnelClient) waitReconnComplete(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait reconnect timeout: %w", ctx.Err())
		case <-ticker.C:
			st := c.State()
			if st == StateReconnecting {
				continue
			}
			if st == StateConnected {
				return nil
			}
			return fmt.Errorf("reconnect ended in state %s", st)
		}
	}
}

// doReconnect contains the three-level degradation logic.
// 三级降级策略：L1 RuntimeTopology → L2 BootstrapPool → L3 信令共振发现（绝境复活）
func (c *GTunnelClient) doReconnect(ctx context.Context) error {
	c.mu.RLock()
	oldIP := c.currentGW.IP
	c.mu.RUnlock()

	reconnCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var success bool

	// Level 1: Try RuntimeTopology first (运行时拓扑池)
	if !c.runtimeTopo.IsEmpty() {
		node, err := c.runtimeTopo.NextByPriority(oldIP)
		if err == nil {
			gw := token.GatewayEndpoint{IP: node.IP, Port: node.Port, Region: node.Region}
			engine, probeErr := c.probe(reconnCtx, gw)
			if probeErr == nil {
				result := &probeResult{gw: gw, engine: engine}
				if err := c.switchWithTransaction(result, oldIP); err == nil {
					success = true
				}
			}
		}
	}

	// Level 1 fallback: Try legacy routeTable if runtimeTopo empty (backward compat)
	if !success && c.routeTable.Count() > 0 {
		next, err := c.routeTable.NextAvailable(oldIP)
		if err == nil {
			engine, probeErr := c.probe(reconnCtx, next)
			if probeErr == nil {
				result := &probeResult{gw: next, engine: engine}
				if err := c.switchWithTransaction(result, oldIP); err == nil {
					success = true
				}
			}
		}
	}

	// Level 2: Fallback to BootstrapPool (启动种子池，只读)
	if !success {
		result, err := c.probeFirst(reconnCtx, c.bootstrapPool)
		if err == nil {
			if err := c.switchWithTransaction(result, oldIP); err == nil {
				success = true
			}
		}
	}

	// Level 2 fallback: Try config.BootstrapPool if bootstrapPool empty (backward compat)
	if !success && len(c.bootstrapPool) == 0 {
		result, err := c.probeFirst(reconnCtx, c.config.BootstrapPool)
		if err == nil {
			if err := c.switchWithTransaction(result, oldIP); err == nil {
				success = true
			}
		}
	}

	// Level 3: 信令共振发现（绝境复活 — Doom Race）
	if !success && c.resonance != nil {
		log.Println("[GTunnel] ⚠️ 所有已知节点不可达，启动信令共振发现...")
		resCtx, resCancel := context.WithTimeout(ctx, 15*time.Second)
		defer resCancel()

		signal, err := c.resonance.Resolve(resCtx)
		if err == nil {
			pool := make([]token.GatewayEndpoint, 0, len(signal.Gateways))
			for _, gw := range signal.Gateways {
				pool = append(pool, token.GatewayEndpoint{
					IP:   gw.IP,
					Port: gw.Port,
				})
			}
			if len(pool) > 0 {
				c.routeTable.Update(pool)
				// Write discovered nodes to RuntimeTopology (Req 3.3)
				nodes := make([]GatewayNode, len(signal.Gateways))
				for i, gw := range signal.Gateways {
					nodes[i] = GatewayNode{IP: gw.IP, Port: gw.Port, Priority: uint8(i)}
				}
				c.runtimeTopo.Update(nodes, c.runtimeTopo.Version()+1, time.Now().UTC())

				result, probeErr := c.probeFirst(resCtx, pool)
				if probeErr == nil {
					if err := c.switchWithTransaction(result, oldIP); err == nil {
						success = true
						// Req 3.5: 绝境发现成功且建链成功后，立即触发一次 PullRouteTable
						c.triggerImmediateTopoPull()
					}
				}
			}
		}
	}

	if !success {
		return fmt.Errorf("all reconnection strategies exhausted")
	}

	return nil
}

// SetResonanceResolver 注入信令共振发现器
func (c *GTunnelClient) SetResonanceResolver(resolver *resonance.Resolver) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resonance = resolver
}

// SetNICDetector 注入物理网卡检测器
func (c *GTunnelClient) SetNICDetector(detector NICDetector) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nicDetector = detector
}

// rebuildSendShim creates or replaces the sendShim with the given sendFn.
// Must be called with c.mu held.
func (c *GTunnelClient) rebuildSendShim(sendFn func([]byte) error) {
	if c.sendShim != nil {
		// Preserve existing config, just swap sendFn
		c.sendShim.mu.Lock()
		c.sendShim.sendFn = sendFn
		c.sendShim.mu.Unlock()
	} else {
		// Default config: no padding, no IAT (passthrough)
		c.sendShim = NewSendPathShim(SendPathShimConfig{
			MaxMTU: 1200,
		}, sendFn)
	}
}

// SetSendPathShimConfig 配置 send-path shim 的 Padding 和 IAT 参数。
// 必须在 Connect 之后调用（需要 transport 已就绪）。
func (c *GTunnelClient) SetSendPathShimConfig(cfg SendPathShimConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sendShim != nil {
		c.sendShim.mu.Lock()
		c.sendShim.paddingMean = cfg.PaddingMean
		c.sendShim.paddingStddev = cfg.PaddingStddev
		c.sendShim.maxMTU = cfg.MaxMTU
		c.sendShim.iatMode = cfg.IATMode
		c.sendShim.iatMeanUs = cfg.IATMeanUs
		c.sendShim.iatStddevUs = cfg.IATStddevUs
		c.sendShim.mu.Unlock()
	}
}

// WSSOverrideConfig 可选的 WSS 降级参数覆盖。
// 由上层（main.go 或配置文件）注入，覆盖默认的硬编码值。
type WSSOverrideConfig struct {
	WSSSNI      string // TLS SNI 覆盖
	WSSPath     string // WebSocket 路径覆盖
	WSSPort     int    // 端口覆盖（默认 443）
	WSSCertFile string // mTLS 客户端证书路径
	WSSKeyFile  string // mTLS 客户端私钥路径
	WSSCAFile   string // Gateway CA 证书路径
}

// SetWSSConfig 注入 WSS 降级配置。必须在 Connect 之前调用。
func (c *GTunnelClient) SetWSSConfig(cfg *WSSOverrideConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wssConfig = cfg
}

// SetTransport 注入统一传输层（Orchestrator 适配器）。
// 注入后 Send/Receive 走 Transport 而非直接走 QUICEngine。
// 这是将 Client 从单一 QUIC 路径升级为多协议编排的接入点。
func (c *GTunnelClient) SetTransport(t Transport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.transport = t
}

// SetTopoRefresher 注入拓扑刷新器引用（用于绝境发现成功后立即触发拉取）
func (c *GTunnelClient) SetTopoRefresher(tr *TopoRefresher) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.topoRefresher = tr
}

// triggerImmediateTopoPull 绝境发现成功后立即触发一次完整拓扑拉取
func (c *GTunnelClient) triggerImmediateTopoPull() {
	c.mu.RLock()
	tr := c.topoRefresher
	c.mu.RUnlock()
	if tr != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := tr.PullOnce(ctx); err != nil {
				log.Printf("[GTunnel] 绝境发现后拓扑拉取失败: %v", err)
			} else {
				log.Printf("[GTunnel] 绝境发现后拓扑拉取成功")
			}
		}()
	}
}

// CurrentGateway returns the currently connected gateway.
func (c *GTunnelClient) CurrentGateway() token.GatewayEndpoint {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentGW
}

// BootstrapPool returns a copy of the immutable bootstrap pool.
func (c *GTunnelClient) BootstrapPool() []token.GatewayEndpoint {
	cp := make([]token.GatewayEndpoint, len(c.bootstrapPool))
	copy(cp, c.bootstrapPool)
	return cp
}

// RuntimeTopo returns the runtime topology reference.
func (c *GTunnelClient) RuntimeTopo() *RuntimeTopology {
	return c.runtimeTopo
}

// DegradationLevel returns the current degradation level.
func (c *GTunnelClient) DegradationLevel() DegradationLevel {
	return DegradationLevel(c.degradationLevel.Load())
}

// setDegradation updates the degradation level and emits an event.
func (c *GTunnelClient) setDegradation(level DegradationLevel, reason string, attempts int) {
	old := DegradationLevel(c.degradationLevel.Swap(int32(level)))
	now := time.Now()

	var event DegradationEvent
	if level < old {
		// Recovery: from higher to lower level
		duration := now.Sub(c.degradationAt)
		event = NewRecoveryEvent(level, reason, attempts, duration)
	} else {
		event = NewDegradationEvent(level, reason, attempts)
	}

	c.mu.Lock()
	c.degradationAt = now
	cb := c.onDegradation
	c.mu.Unlock()

	if cb != nil {
		cb(event)
	}
}

// SetOnDegradation registers a callback for degradation level changes.
func (c *GTunnelClient) SetOnDegradation(fn func(DegradationEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDegradation = fn
}

// OnGatewaySwitch registers a callback for gateway IP changes.
func (c *GTunnelClient) OnGatewaySwitch(fn func(newIP string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.switchFn = fn
}

// SetSwitchPreAdd registers the pre-add route callback for transactional switch.
func (c *GTunnelClient) SetSwitchPreAdd(fn func(string) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.switchPreAddFn = fn
}

// SetSwitchCommit registers the commit route callback for transactional switch.
func (c *GTunnelClient) SetSwitchCommit(fn func(string, string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.switchCommitFn = fn
}

// SetSwitchRollback registers the rollback route callback for transactional switch.
func (c *GTunnelClient) SetSwitchRollback(fn func(string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.switchRollbackFn = fn
}

// Close shuts down the tunnel client.
func (c *GTunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.transition(StateStopped, "client closed")
	// 优先走统一 transport，回退到 legacy quic
	if c.transport != nil {
		return c.transport.Close()
	}
	if c.quic != nil {
		return c.quic.Close()
	}
	return nil
}

// ControlledDisconnect performs a graceful disconnect without fully stopping the client.
// Unlike Close(), this allows the client to potentially reconnect later.
// Used for banned/expired scenarios where we need to drop the connection but keep the client alive.
func (c *GTunnelClient) ControlledDisconnect(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.transition(StateStopped, fmt.Sprintf("controlled disconnect: %s", reason))
	if c.transport != nil {
		c.transport.Close()
		c.transport = nil
	}
	if c.quic != nil {
		c.quic.Close()
		c.quic = nil
	}
}

// encrypt uses ChaCha20-Poly1305 with the pre-shared key.
func (c *GTunnelClient) encrypt(plaintext []byte) ([]byte, error) {
	if len(c.psk) < chacha20poly1305.KeySize {
		return nil, fmt.Errorf("PSK too short")
	}
	aead, err := chacha20poly1305.New(c.psk[:chacha20poly1305.KeySize])
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt uses ChaCha20-Poly1305 with the pre-shared key.
func (c *GTunnelClient) decrypt(ciphertext []byte) ([]byte, error) {
	if len(c.psk) < chacha20poly1305.KeySize {
		return nil, fmt.Errorf("PSK too short")
	}
	aead, err := chacha20poly1305.New(c.psk[:chacha20poly1305.KeySize])
	if err != nil {
		return nil, err
	}
	nonceSize := aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return aead.Open(nil, nonce, ct, nil)
}
