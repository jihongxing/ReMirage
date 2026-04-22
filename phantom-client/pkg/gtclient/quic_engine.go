package gtclient

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

// NICDetector abstracts physical NIC outbound detection for QUICEngine.
type NICDetector interface {
	DetectOutbound(targetIP string) (net.IP, error)
}

// QUICEngine manages the real QUIC Datagram connection to a gateway.
type QUICEngine struct {
	conn        *quic.Conn
	addr        string
	tlsConf     *tls.Config
	quicConf    *quic.Config
	connected   atomic.Bool
	mu          sync.Mutex
	recvCh      chan []byte // buffered channel for incoming datagrams
	cancelFunc  context.CancelFunc
	nicDetector NICDetector // injected physical NIC detector (nil = legacy fallback)
}

// QUICEngineConfig holds configuration for the QUIC engine.
type QUICEngineConfig struct {
	GatewayAddr    string
	PSK            []byte // used to derive TLS cert verification (or skip in dev)
	PinnedCertHash []byte // SHA-256 of gateway's leaf certificate (nil = skip verify)
	KeepAlive      time.Duration
	RecvBufferSize int
	NICDetector    NICDetector // optional: physical NIC detector (nil = legacy fallback)
}

// NewQUICEngine creates a QUIC engine with Datagram support enabled.
func NewQUICEngine(cfg *QUICEngineConfig) *QUICEngine {
	keepAlive := cfg.KeepAlive
	if keepAlive == 0 {
		keepAlive = 10 * time.Second
	}
	bufSize := cfg.RecvBufferSize
	if bufSize == 0 {
		bufSize = 4096
	}

	tlsConf := &tls.Config{
		NextProtos: []string{"mirage-gtunnel"},
	}

	if len(cfg.PinnedCertHash) == 32 {
		// Production: verify server cert via SHA-256 pin
		tlsConf.InsecureSkipVerify = true
		tlsConf.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("no peer certificate")
			}
			leaf := cs.PeerCertificates[0]
			hash := sha256.Sum256(leaf.Raw)
			if !equal(hash[:], cfg.PinnedCertHash) {
				return fmt.Errorf("certificate pin mismatch")
			}
			return nil
		}
	} else {
		// Dev mode: skip verification
		tlsConf.InsecureSkipVerify = true
	}

	return &QUICEngine{
		addr:        cfg.GatewayAddr,
		tlsConf:     tlsConf,
		nicDetector: cfg.NICDetector,
		quicConf: &quic.Config{
			EnableDatagrams: true,
			KeepAlivePeriod: keepAlive,
			MaxIdleTimeout:  60 * time.Second,
		},
		recvCh: make(chan []byte, bufSize),
	}
}

// equal compares two byte slices in constant time.
func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// Connect establishes the QUIC connection and starts the receive pump.
// Uses PhysicalNICDetector to discover outbound IP, with legacy fallback.
func (e *QUICEngine) Connect(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 1. Resolve remote address first (needed for NIC detection target)
	remoteAddr, err := net.ResolveUDPAddr("udp4", e.addr)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", e.addr, err)
	}

	// 2. Discover physical outbound IP via NICDetector or legacy fallback
	var physicalIP net.IP
	if e.nicDetector != nil {
		physicalIP, err = e.nicDetector.DetectOutbound(remoteAddr.IP.String())
		if err != nil {
			// Fallback: legacy probe method
			physicalIP, err = legacyDetectOutbound()
			if err != nil {
				return fmt.Errorf("detect physical NIC: %w", err)
			}
		}
	} else {
		// No detector injected: use legacy method
		physicalIP, err = legacyDetectOutbound()
		if err != nil {
			return fmt.Errorf("detect physical NIC: %w", err)
		}
	}

	// 3. Bind UDP socket to physical NIC IP (bypass Wintun/WFP interference)
	localAddr := &net.UDPAddr{IP: physicalIP, Port: 0}
	udpConn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		return fmt.Errorf("bind physical NIC %s: %w", physicalIP, err)
	}

	// 4. QUIC dial with explicit bound socket
	conn, err := quic.Dial(ctx, udpConn, remoteAddr, e.tlsConf, e.quicConf)
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("quic dial %s: %w", e.addr, err)
	}

	// 5. Verify datagram support was negotiated
	state := conn.ConnectionState()
	if !state.SupportsDatagrams.Remote || !state.SupportsDatagrams.Local {
		conn.CloseWithError(0, "datagrams not supported")
		udpConn.Close()
		return fmt.Errorf("gateway does not support QUIC Datagrams")
	}

	e.conn = conn
	e.connected.Store(true)

	// 6. Start receive pump
	pumpCtx, cancel := context.WithCancel(ctx)
	e.cancelFunc = cancel
	go e.recvPump(pumpCtx)

	return nil
}

// legacyDetectOutbound uses the old net.Dial("udp4","8.8.8.8:53") approach as fallback.
func legacyDetectOutbound() (net.IP, error) {
	probeConn, err := net.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return nil, fmt.Errorf("legacy detect: %w", err)
	}
	ip := probeConn.LocalAddr().(*net.UDPAddr).IP
	probeConn.Close()
	return ip, nil
}

// MaxShardSize returns the maximum safe payload size for a single datagram.
// Accounts for: ChaCha20 nonce (12) + MAC (16) + Fragment header (8)
// QUIC Datagram MTU is typically ~1200 bytes.
func (e *QUICEngine) MaxShardSize() int {
	overhead := 12 + 16 + 8 // nonce + mac + header
	// Conservative estimate based on QUIC initial MTU
	return 1200 - overhead
}

// SendDatagram sends an encrypted shard as a single QUIC Datagram.
// The caller must ensure len(data) <= MaxDatagramFrameSize.
func (e *QUICEngine) SendDatagram(data []byte) error {
	if !e.connected.Load() {
		return fmt.Errorf("not connected")
	}
	return e.conn.SendDatagram(data)
}

// ReceiveDatagram returns the next incoming datagram (blocking).
// Returns a copied slice safe for long-term retention.
func (e *QUICEngine) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	select {
	case msg := <-e.recvCh:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// recvPump continuously reads datagrams and pushes to recvCh.
func (e *QUICEngine) recvPump(ctx context.Context) {
	for {
		msg, err := e.conn.ReceiveDatagram(ctx)
		if err != nil {
			e.connected.Store(false)
			return
		}
		// Copy immediately — quic-go may reuse the buffer
		copied := make([]byte, len(msg))
		copy(copied, msg)

		select {
		case e.recvCh <- copied:
		default:
			// Channel full — drop oldest to prevent backpressure
			select {
			case <-e.recvCh:
			default:
			}
			e.recvCh <- copied
		}
	}
}

// Close gracefully shuts down the QUIC connection.
func (e *QUICEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.connected.Store(false)
	if e.cancelFunc != nil {
		e.cancelFunc()
	}
	if e.conn != nil {
		return e.conn.CloseWithError(0, "client shutdown")
	}
	return nil
}

// IsConnected returns the connection status.
func (e *QUICEngine) IsConnected() bool {
	return e.connected.Load()
}

// RemoteAddr returns the gateway address.
func (e *QUICEngine) RemoteAddr() string {
	return e.addr
}
