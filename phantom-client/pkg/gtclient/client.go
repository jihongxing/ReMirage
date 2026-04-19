package gtclient

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20poly1305"

	"phantom-client/pkg/token"
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

// GTunnelClient manages the G-Tunnel connection to a gateway.
type GTunnelClient struct {
	config     *token.BootstrapConfig
	fec        *FECCodec
	sampler    *OverlapSampler
	routeTable *RouteTable
	currentGW  token.GatewayEndpoint
	psk        []byte
	switchFn   func(newIP string)
	connected  bool
	mu         sync.RWMutex
}

// NewGTunnelClient creates a new G-Tunnel client.
func NewGTunnelClient(config *token.BootstrapConfig) *GTunnelClient {
	fec, _ := NewFECCodec(8, 4)
	return &GTunnelClient{
		config:     config,
		fec:        fec,
		sampler:    NewOverlapSampler(),
		routeTable: &RouteTable{},
		psk:        config.PreSharedKey,
	}
}

// ProbeAndConnect concurrently probes bootstrap nodes, connects to first responder.
func (c *GTunnelClient) ProbeAndConnect(ctx context.Context, pool []token.GatewayEndpoint) error {
	if len(pool) == 0 {
		return fmt.Errorf("empty bootstrap pool")
	}

	type result struct {
		gw  token.GatewayEndpoint
		err error
	}

	ch := make(chan result, len(pool))
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, gw := range pool {
		go func(gw token.GatewayEndpoint) {
			err := c.probe(probeCtx, gw)
			ch <- result{gw: gw, err: err}
		}(gw)
	}

	for i := 0; i < len(pool); i++ {
		select {
		case r := <-ch:
			if r.err == nil {
				c.mu.Lock()
				c.currentGW = r.gw
				c.connected = true
				c.mu.Unlock()
				return nil
			}
		case <-probeCtx.Done():
			return fmt.Errorf("probe timeout: %w", probeCtx.Err())
		}
	}

	return fmt.Errorf("all bootstrap nodes unreachable")
}

// probe attempts to connect to a single gateway.
func (c *GTunnelClient) probe(ctx context.Context, gw token.GatewayEndpoint) error {
	// In production: establish QUIC connection to gw.IP:gw.Port
	// For now, this is a placeholder that simulates the probe
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Real implementation would use quic-go to dial
		return fmt.Errorf("probe not implemented for %s:%d", gw.IP, gw.Port)
	}
}

// Send encrypts and sends a packet through the tunnel.
// Pipeline: data → overlap split → FEC encode → ChaCha20 encrypt → QUIC send
func (c *GTunnelClient) Send(packet []byte) error {
	if len(packet) == 0 {
		return nil
	}

	// 1. Overlap sampling split
	fragments := c.sampler.Split(packet)

	// 2. For each fragment: FEC encode
	for _, frag := range fragments {
		shards, err := c.fec.Encode(frag.Data)
		if err != nil {
			return fmt.Errorf("fec encode: %w", err)
		}

		// 3. Encrypt each shard
		for _, shard := range shards {
			encrypted, err := c.encrypt(shard)
			if err != nil {
				return fmt.Errorf("encrypt: %w", err)
			}

			// 4. Send via QUIC (placeholder)
			_ = encrypted
			// In production: c.quicStream.Write(encrypted)
		}
	}

	return nil
}

// Receive reads and decrypts a packet from the tunnel.
// Pipeline: QUIC receive → decrypt → FEC decode → reassemble
func (c *GTunnelClient) Receive() ([]byte, error) {
	// Placeholder: in production, read from QUIC stream
	// 1. Read encrypted shards from QUIC
	// 2. Decrypt each shard
	// 3. FEC decode to recover fragments
	// 4. Reassemble fragments into original packet
	return nil, fmt.Errorf("receive not implemented: no active QUIC connection")
}

// PullRouteTable fetches the dynamic route table through the tunnel.
func (c *GTunnelClient) PullRouteTable(ctx context.Context) error {
	// In production: send route table request through QUIC stream,
	// receive response, parse and update route table in memory.
	// Route table is NEVER written to disk.
	return nil
}

// Reconnect attempts failover to next available gateway within 5s.
func (c *GTunnelClient) Reconnect(ctx context.Context) error {
	c.mu.RLock()
	currentIP := c.currentGW.IP
	c.mu.RUnlock()

	reconnCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Try route table first
	if c.routeTable.Count() > 0 {
		next, err := c.routeTable.NextAvailable(currentIP)
		if err == nil {
			if err := c.probe(reconnCtx, next); err == nil {
				c.mu.Lock()
				oldIP := c.currentGW.IP
				c.currentGW = next
				c.connected = true
				c.mu.Unlock()
				if c.switchFn != nil && oldIP != next.IP {
					c.switchFn(next.IP)
				}
				return nil
			}
		}
	}

	// Fallback to bootstrap pool
	return c.ProbeAndConnect(reconnCtx, c.config.BootstrapPool)
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

// Close shuts down the tunnel client.
func (c *GTunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	// In production: close QUIC connection
	return nil
}

// IsConnected returns connection status.
func (c *GTunnelClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
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
