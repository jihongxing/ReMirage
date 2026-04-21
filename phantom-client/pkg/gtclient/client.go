package gtclient

import (
	"context"
	"crypto/rand"
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
	state       atomic.Int32 // ConnState
	reconnMu    sync.Mutex
	mu          sync.RWMutex
	recvCancel  context.CancelFunc

	// 事务切换回调
	switchPreAddFn   func(string) error
	switchCommitFn   func(string, string)
	switchRollbackFn func(string)

	// 信令共振发现器（绝境复活）
	resonance *resonance.Resolver
}

// NewGTunnelClient creates a new G-Tunnel client.
func NewGTunnelClient(config *token.BootstrapConfig) *GTunnelClient {
	fec, _ := NewFECCodec(8, 4)
	sampler := NewOverlapSampler()
	c := &GTunnelClient{
		config:      config,
		fec:         fec,
		sampler:     sampler,
		reassembler: NewReassembler(fec, sampler),
		routeTable:  &RouteTable{},
		psk:         config.PreSharedKey,
	}
	c.state.Store(int32(StateInit))
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
func (c *GTunnelClient) adoptConnection(result *probeResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.quic != nil {
		c.quic.Close()
	}
	c.quic = result.engine
	c.currentGW = result.gw
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
func (c *GTunnelClient) ProbeAndConnect(ctx context.Context, pool []token.GatewayEndpoint) error {
	if len(pool) == 0 {
		return fmt.Errorf("empty bootstrap pool")
	}

	result, err := c.probeFirst(ctx, pool)
	if err != nil {
		return err
	}

	c.adoptConnection(result)
	return nil
}

// probe attempts to connect to a single gateway via QUIC Datagram.
// Returns the QUICEngine on success (caller is responsible for closing on discard).
func (c *GTunnelClient) probe(ctx context.Context, gw token.GatewayEndpoint) (*QUICEngine, error) {
	addr := fmt.Sprintf("%s:%d", gw.IP, gw.Port)
	engine := NewQUICEngine(&QUICEngineConfig{
		GatewayAddr: addr,
	})

	if err := engine.Connect(ctx); err != nil {
		return nil, err
	}

	return engine, nil
}

// Send encrypts and sends a packet through the tunnel via QUIC Datagram.
// Pipeline: IP packet → overlap split → FEC encode → ChaCha20 encrypt → QUIC Datagram
func (c *GTunnelClient) Send(packet []byte) error {
	if len(packet) == 0 {
		return nil
	}

	// If QUIC engine not connected, drop silently
	if c.quic == nil || !c.quic.IsConnected() {
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

			// 4. Fire via QUIC Datagram (unreliable, FEC handles loss)
			if err := c.quic.SendDatagram(encrypted); err != nil {
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
func (c *GTunnelClient) Receive(ctx context.Context) ([]byte, error) {
	if c.quic == nil || !c.quic.IsConnected() {
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
		msg, err := c.quic.ReceiveDatagram(ctx)
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
	c.reconnMu.Unlock()

	err := c.doReconnect(ctx)
	if err != nil {
		c.transition(StateExhausted, err.Error())
	}
	return err
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
// 三级降级策略：RouteTable → Bootstrap Pool → 信令共振发现（绝境复活）
func (c *GTunnelClient) doReconnect(ctx context.Context) error {
	c.mu.RLock()
	oldIP := c.currentGW.IP
	c.mu.RUnlock()

	reconnCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var success bool

	// Level 1: Try route table first
	if c.routeTable.Count() > 0 {
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

	// Level 2: Fallback to bootstrap pool
	if !success {
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
				result, probeErr := c.probeFirst(resCtx, pool)
				if probeErr == nil {
					if err := c.switchWithTransaction(result, oldIP); err == nil {
						success = true
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

// CurrentGateway returns the currently connected gateway.
func (c *GTunnelClient) CurrentGateway() token.GatewayEndpoint {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentGW
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
	if c.quic != nil {
		return c.quic.Close()
	}
	return nil
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
