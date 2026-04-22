package threat

import (
	"bufio"
	"bytes"
	"log"
	"net"
	"time"
)

// protocolSignatures 非 TLS 协议签名（前缀匹配）
var protocolSignatures = map[string][]byte{
	"ssh":       []byte("SSH-"),
	"http_get":  []byte("GET "),
	"http_post": []byte("POST"),
	"http_head": []byte("HEAD"),
	"http_conn": []byte("CONN"),
}

// ProtocolDetector 协议异常检测器
// 在 TLS 握手前 Peek 连接首字节，识别非预期协议（SSH/HTTP 等扫描行为）
type ProtocolDetector struct {
	riskScorer RiskScoreAdder
	blacklist  *BlacklistManager
}

// NewProtocolDetector 创建协议检测器
func NewProtocolDetector(rs RiskScoreAdder, bl *BlacklistManager) *ProtocolDetector {
	return &ProtocolDetector{
		riskScorer: rs,
		blacklist:  bl,
	}
}

// bufferedConn 包装 bufio.Reader + net.Conn，使 Peek 后 TLS 握手仍能读取完整数据
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

// Read 优先从 bufio.Reader 读取（含 Peek 缓冲），保证数据不丢失
func (bc *bufferedConn) Read(b []byte) (int, error) {
	return bc.reader.Read(b)
}

// Detect 读取连接前 8 字节，检测非 TLS 协议
// 返回 (isMalicious, protocolType, bufferedConn)
// bufferedConn 保留了 Peek 数据，后续 TLS 握手可正常读取完整 ClientHello
func (pd *ProtocolDetector) Detect(conn net.Conn) (isMalicious bool, protocolType string, wrapped net.Conn) {
	reader := bufio.NewReader(conn)
	bc := &bufferedConn{Conn: conn, reader: reader}

	// 设置 100ms 读取超时，避免慢速连接阻塞
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

	peeked, err := reader.Peek(8)

	// 恢复无超时状态，供后续 TLS 握手使用
	_ = conn.SetReadDeadline(time.Time{})

	if err != nil {
		// Peek 失败（超时/EOF/数据不足）→ 视为正常 TLS 流量
		return false, "", bc
	}

	for proto, sig := range protocolSignatures {
		if bytes.HasPrefix(peeked, sig) {
			return true, proto, bc
		}
	}

	return false, "", bc
}

// HandleMalicious 处理恶意协议检测
// 1. RiskScorer +40
// 2. 递增 ProtocolScanTotal Prometheus 指标
func (pd *ProtocolDetector) HandleMalicious(sourceIP string, protocolType string) {
	if pd.riskScorer != nil {
		pd.riskScorer.AddScore(sourceIP, 40, "protocol_scan:"+protocolType)
	}

	ProtocolScanTotal.WithLabelValues(GetGatewayID(), protocolType).Inc()

	log.Printf("[ProtocolDetector] 检测到非预期协议: IP=%s, Protocol=%s", sourceIP, protocolType)
}
