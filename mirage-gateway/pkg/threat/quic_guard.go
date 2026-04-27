package threat

import (
	"encoding/binary"
	"log"
	"mirage-gateway/pkg/redact"
	"net"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/quic-go/quic-go"
)

// ============================================================
// 第一层：UDP 首包预过滤 — 在 quic.Listener 之前拦截非法 QUIC Initial
// ============================================================

// UDPPreFilter 对 UDP 首包做 QUIC Initial 格式校验
// 不合法的直接丢弃不回包，避免暴露 Gateway 存在
type UDPPreFilter struct {
	blacklist *BlacklistManager
	mu        sync.Mutex
	dropCount map[string]int // IP → 连续丢弃计数
}

// NewUDPPreFilter 创建 UDP 首包预过滤器
func NewUDPPreFilter(bl *BlacklistManager) *UDPPreFilter {
	return &UDPPreFilter{
		blacklist: bl,
		dropCount: make(map[string]int),
	}
}

// FilterPacket 校验 UDP 首包是否为合法 QUIC Initial
// 检查：Long Header bit、QUIC Version、DCID Length、Packet Type
// 返回 true 表示合法，false 表示应丢弃
func (f *UDPPreFilter) FilterPacket(buf []byte, addr *net.UDPAddr) bool {
	if len(buf) < 5 {
		f.recordDrop(addr)
		return false
	}

	// Long Header: first bit must be 1 for Initial packets
	if buf[0]&0x80 == 0 {
		f.recordDrop(addr)
		return false
	}

	// Version check: QUIC v1 = 0x00000001, QUIC v2 = 0x6b3343cf
	version := binary.BigEndian.Uint32(buf[1:5])
	if version != 0x00000001 && version != 0x6b3343cf {
		f.recordDrop(addr)
		return false
	}

	// DCID Length check (byte 5): must be <= 20 per RFC 9000
	if len(buf) < 6 {
		f.recordDrop(addr)
		return false
	}
	dcidLen := int(buf[5])
	if dcidLen > 20 {
		f.recordDrop(addr)
		return false
	}

	// Packet Type check: Initial = 0x00 in the two type bits (bits 4-5 of first byte for Long Header)
	// For QUIC v1 Long Header: Form(1) | Fixed(1) | Type(2) | Reserved(2) | PktNumLen(2)
	// Initial type = 0b00
	packetType := (buf[0] >> 4) & 0x03
	if packetType != 0x00 {
		// Not an Initial packet — could be 0-RTT, Handshake, or Retry
		// Allow Retry (0x03) and Handshake (0x02) as they are part of normal flow
		// But for first packet from unknown source, only Initial (0x00) is expected
		f.recordDrop(addr)
		return false
	}

	return true
}

// recordDrop 记录丢弃事件，频繁丢弃的 IP 加入黑名单
func (f *UDPPreFilter) recordDrop(addr *net.UDPAddr) {
	QUICPreFilterDropTotal.WithLabelValues(GetGatewayID()).Inc()

	if addr == nil {
		return
	}
	ip := addr.IP.String()

	f.mu.Lock()
	defer f.mu.Unlock()

	f.dropCount[ip]++
	if f.dropCount[ip] > 10 && f.blacklist != nil {
		_ = f.blacklist.Add(ip+"/32", time.Now().Add(1*time.Hour), SourceLocal)
		log.Printf("[UDPPreFilter] IP %s 连续 %d 次非法 Initial，已加入黑名单", redact.RedactIP(ip), f.dropCount[ip])
		delete(f.dropCount, ip)
	}
}

// ============================================================
// 第二层：Accept 后 ConnectionState 校验
// ============================================================

// QUICPostAcceptValidator 在 quic.Listener.Accept() 返回后校验连接
// 检查 NegotiatedProtocol 是否为 h3，非 h3 则快速关闭并集成风险评分
type QUICPostAcceptValidator struct {
	blacklist  *BlacklistManager
	riskScorer RiskScoreAdder
	mu         sync.Mutex
	failCount  map[string]int // IP → 连续失败计数
}

// NewQUICPostAcceptValidator 创建 QUIC 连接后校验器
func NewQUICPostAcceptValidator(bl *BlacklistManager, rs RiskScoreAdder) *QUICPostAcceptValidator {
	return &QUICPostAcceptValidator{
		blacklist:  bl,
		riskScorer: rs,
		failCount:  make(map[string]int),
	}
}

// Validate 校验已建立的 QUIC 连接
// 返回 true 表示合法 h3 连接，false 表示已关闭
func (v *QUICPostAcceptValidator) Validate(conn *quic.Conn) bool {
	state := conn.ConnectionState()
	if state.TLS.NegotiatedProtocol != "h3" {
		QUICPostAcceptRejectTotal.WithLabelValues(GetGatewayID()).Inc()

		host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
		if err != nil {
			host = conn.RemoteAddr().String()
		}

		// 风险评分 +15
		if v.riskScorer != nil {
			v.riskScorer.AddScore(host, 15, "invalid_quic_alpn")
		}

		// 频繁异常 → 黑名单
		v.mu.Lock()
		v.failCount[host]++
		count := v.failCount[host]
		if count > 5 && v.blacklist != nil {
			_ = v.blacklist.Add(host+"/32", time.Now().Add(1*time.Hour), SourceLocal)
			log.Printf("[QUICPostAcceptValidator] IP %s 连续 %d 次非 h3 协商，已加入黑名单", redact.RedactIP(host), count)
			delete(v.failCount, host)
		}
		v.mu.Unlock()

		// 快速关闭，不泄露错误信息
		conn.CloseWithError(0, "")
		return false
	}
	return true
}

// ============================================================
// Prometheus 指标
// ============================================================

var (
	// QUICPreFilterDropTotal QUIC 首包预过滤丢弃计数器
	QUICPreFilterDropTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_quic_prefilter_drop_total",
		Help: "Total QUIC Initial packets dropped by UDP pre-filter",
	}, []string{"gateway_id"})

	// QUICPostAcceptRejectTotal QUIC 连接后校验拒绝计数器
	QUICPostAcceptRejectTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mirage_quic_postaccept_reject_total",
		Help: "Total QUIC connections rejected by post-accept validator",
	}, []string{"gateway_id"})
)
