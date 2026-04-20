// Package gtunnel - DNS Tunnel 传输
// 伪装形态：域名解析（Base32 编码子域名 + TXT 记录）
// 极端环境最后求生通道，带宽极低延迟极高，仅承载紧急控制指令
//
// 工程红线：
//  1. 缓存击穿: 每次查询域名全球唯一 [payload].[seq].[nonce].t.domain
//  2. 轮询泵: 无上行数据时定期发送 poll 查询拉取下行数据
//  3. 分片重组: 大包切片为 ≤80 字节 Shard，跨多次查询传输
//
// 域名格式:
//
//	上行数据: [base32_payload].[fragID]-[fragTotal]-[seq].[nonce8].d.domain.
//	轮询空包: p.[seq].[nonce8].d.domain.
package gtunnel

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// MaxDNSDatagramSize DNS 隧道单次可承载的最大原始字节数
// RFC 1035: Label ≤ 63B, FQDN ≤ 253B
// 扣除元数据标签（seq/nonce/domain 约 60B），数据标签可用 ~190B
// Base32 膨胀 1.6x → 实际原始数据 ≤ 110B
// 保守值，确保不触碰 253 上限
const MaxDNSDatagramSize = 110

// DNS 分片参数
const (
	// 单个 DNS 查询可承载的原始数据字节数（Base32 编码后填入子域名标签）
	dnsFragPayloadSize = 80
	// 轮询间隔
	dnsPollInterval = 200 * time.Millisecond
	// 轮询前缀
	dnsPollPrefix = "p"
	// 数据子域名前缀
	dnsDataPrefix = "d"
)

// dnsBase32 无填充的 Base32 编码器（小写）
var dnsBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// ═══════════════════════════════════════════════════════════════
// 编解码工具函数
// ═══════════════════════════════════════════════════════════════

// genNonce 生成 8 字节随机 Nonce（hex 编码 = 16 字符）
func genNonce() string {
	var buf [8]byte
	rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// DNSEncodeSubdomain 将数据 Base32 编码为 DNS 子域名标签序列
// 格式: [label1].[label2]...[labelN].[fragID-fragTotal-seq].[nonce].d.domain.
func DNSEncodeSubdomain(data []byte, seq uint32, fragID, fragTotal int, domain string) string {
	encoded := strings.ToLower(dnsBase32.EncodeToString(data))

	// 按 63 字节切分为多个 Label（RFC 1035 单标签上限）
	var labels []string
	for len(encoded) > 0 {
		end := 63
		if end > len(encoded) {
			end = len(encoded)
		}
		labels = append(labels, encoded[:end])
		encoded = encoded[end:]
	}

	// 元数据标签: fragID-fragTotal-seq
	meta := fmt.Sprintf("%d-%d-%d", fragID, fragTotal, seq)
	nonce := genNonce()

	// 完整 FQDN: [data_labels].[meta].[nonce].d.domain.
	parts := append(labels, meta, nonce, dnsDataPrefix, domain)
	return strings.Join(parts, ".") + "."
}

// DNSEncodePoll 编码轮询查询域名
// 格式: p.[seq].[nonce].d.domain.
func DNSEncodePoll(seq uint32, domain string) string {
	nonce := genNonce()
	return fmt.Sprintf("%s.%d.%s.%s.%s.", dnsPollPrefix, seq, nonce, dnsDataPrefix, domain)
}

// DNSDecodeSubdomain 从 DNS 子域名解码数据和元信息
// 返回: 原始数据, fragID, fragTotal, seq, isPoll, error
func DNSDecodeSubdomain(fqdn string, domain string) (data []byte, fragID, fragTotal int, seq uint32, isPoll bool, err error) {
	suffix := "." + dnsDataPrefix + "." + domain + "."
	if !strings.HasSuffix(fqdn, suffix) {
		return nil, 0, 0, 0, false, fmt.Errorf("域名后缀不匹配: %s", fqdn)
	}

	// 去掉后缀
	body := strings.TrimSuffix(fqdn, suffix)
	parts := strings.Split(body, ".")

	if len(parts) < 2 {
		return nil, 0, 0, 0, false, fmt.Errorf("域名标签不足: %s", fqdn)
	}

	// 检查是否为轮询包
	if parts[0] == dnsPollPrefix {
		// p.[seq].[nonce]
		if len(parts) >= 2 {
			s, _ := strconv.ParseUint(parts[1], 10, 32)
			seq = uint32(s)
		}
		return nil, 0, 0, seq, true, nil
	}

	// 数据包: [data_labels...].[meta].[nonce]
	// 最后一个是 nonce，倒数第二个是 meta
	if len(parts) < 3 {
		return nil, 0, 0, 0, false, fmt.Errorf("数据标签不足: %s", fqdn)
	}

	// nonce 是最后一个
	// meta 是倒数第二个
	metaLabel := parts[len(parts)-2]
	dataLabels := parts[:len(parts)-2]

	// 解析 meta: fragID-fragTotal-seq
	metaParts := strings.Split(metaLabel, "-")
	if len(metaParts) != 3 {
		return nil, 0, 0, 0, false, fmt.Errorf("meta 格式错误: %s", metaLabel)
	}
	fid, _ := strconv.Atoi(metaParts[0])
	ftotal, _ := strconv.Atoi(metaParts[1])
	s, _ := strconv.ParseUint(metaParts[2], 10, 32)

	// 拼接数据标签并解码
	encoded := strings.ToUpper(strings.Join(dataLabels, ""))
	decoded, err := dnsBase32.DecodeString(encoded)
	if err != nil {
		return nil, 0, 0, 0, false, fmt.Errorf("Base32 解码失败: %w", err)
	}

	return decoded, fid, ftotal, uint32(s), false, nil
}

// ═══════════════════════════════════════════════════════════════
// 客户端侧: DNSTransport（含轮询泵 + 分片重组）
// ═══════════════════════════════════════════════════════════════

// DNSTransport DNS 隧道客户端传输
type DNSTransport struct {
	domain    string      // 权威域名（如 t.yourdomain.com）
	resolver  string      // DNS 服务器地址（如 8.8.8.8:53）
	queryType uint16      // dns.TypeTXT
	recvChan  chan []byte // 下行数据通道
	rtt       time.Duration
	connected int32
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	client    *dns.Client
	seqNum    uint32 // 全局递增序列号（缓存击穿核心）

	// 轮询泵控制
	pollInterval time.Duration
	pollActive   int32 // 是否有上行活动（有则暂停轮询）

	// 分片重组缓冲（下行）
	reassembly sync.Map // seq(uint32) → *dnsReassemblyBuf
}

// dnsReassemblyBuf 分片重组缓冲
type dnsReassemblyBuf struct {
	fragments map[int][]byte
	total     int
	received  int
	createdAt time.Time
}

// NewDNSTransport 创建 DNS 隧道客户端
func NewDNSTransport(config DNSTransportConfig) (*DNSTransport, error) {
	if config.Domain == "" {
		return nil, fmt.Errorf("DNS domain 不能为空")
	}
	if config.Resolver == "" {
		config.Resolver = "8.8.8.8:53"
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &DNSTransport{
		domain:       config.Domain,
		resolver:     config.Resolver,
		queryType:    dns.TypeTXT,
		recvChan:     make(chan []byte, 64),
		ctx:          ctx,
		cancel:       cancel,
		client:       &dns.Client{Timeout: 5 * time.Second},
		pollInterval: dnsPollInterval,
	}

	atomic.StoreInt32(&d.connected, 1)

	// 启动轮询泵
	go d.pollingPump()

	return d, nil
}

// Send 发送数据（分片 + Base32 编码为子域名查询）
func (d *DNSTransport) Send(data []byte) error {
	if atomic.LoadInt32(&d.connected) == 0 {
		return io.ErrClosedPipe
	}

	// 标记上行活跃（暂停轮询泵一个周期）
	atomic.StoreInt32(&d.pollActive, 1)
	defer atomic.StoreInt32(&d.pollActive, 0)

	// 分片
	fragments := d.fragment(data)
	seq := atomic.AddUint32(&d.seqNum, 1)

	for i, frag := range fragments {
		fqdn := DNSEncodeSubdomain(frag, seq, i, len(fragments), d.domain)

		// 验证 FQDN 长度不超过 253
		if len(fqdn) > 253 {
			return fmt.Errorf("FQDN 超长 (%d > 253)，数据需进一步分片", len(fqdn))
		}

		resp, err := d.query(fqdn)
		if err != nil {
			return err
		}

		// 解析响应中的下行数据
		d.extractDownlink(resp)
	}

	return nil
}

// Recv 接收数据（从轮询泵或 Send 响应中获取）
func (d *DNSTransport) Recv() ([]byte, error) {
	select {
	case data, ok := <-d.recvChan:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-d.ctx.Done():
		return nil, io.EOF
	}
}

// Close 关闭连接
func (d *DNSTransport) Close() error {
	atomic.StoreInt32(&d.connected, 0)
	d.cancel()
	return nil
}

// Type 返回传输类型
func (d *DNSTransport) Type() TransportType { return TransportDNS }

// RTT 返回 RTT
func (d *DNSTransport) RTT() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.rtt
}

// RemoteAddr 远端地址
func (d *DNSTransport) RemoteAddr() net.Addr {
	addr, _ := net.ResolveUDPAddr("udp", d.resolver)
	return addr
}

// MaxDatagramSize 返回最大数据报大小
func (d *DNSTransport) MaxDatagramSize() int { return MaxDNSDatagramSize }

// ═══════════════════════════════════════════════════════════════
// 客户端内部方法
// ═══════════════════════════════════════════════════════════════

// pollingPump 后台轮询泵：无上行数据时定期发送空查询拉取下行
func (d *DNSTransport) pollingPump() {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			// 如果有上行活动，跳过本轮轮询（Send 已经在拉取下行）
			if atomic.LoadInt32(&d.pollActive) == 1 {
				continue
			}
			if atomic.LoadInt32(&d.connected) == 0 {
				return
			}

			seq := atomic.AddUint32(&d.seqNum, 1)
			fqdn := DNSEncodePoll(seq, d.domain)

			resp, err := d.query(fqdn)
			if err != nil {
				continue // 轮询失败不中断
			}

			d.extractDownlink(resp)
		}
	}
}

// query 发送 DNS 查询并返回响应
func (d *DNSTransport) query(fqdn string) (*dns.Msg, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(fqdn, d.queryType)
	msg.RecursionDesired = true

	start := time.Now()
	resp, _, err := d.client.Exchange(msg, d.resolver)
	rtt := time.Since(start)

	d.mu.Lock()
	d.rtt = rtt
	d.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("DNS 查询失败: %w", err)
	}
	return resp, nil
}

// extractDownlink 从 DNS 响应中提取下行数据
func (d *DNSTransport) extractDownlink(resp *dns.Msg) {
	if resp == nil {
		return
	}

	for _, rr := range resp.Answer {
		txt, ok := rr.(*dns.TXT)
		if !ok {
			continue
		}
		for _, record := range txt.Txt {
			if record == "" || record == "empty" {
				continue // 空响应标记
			}

			// TXT 记录格式: [fragID]-[fragTotal]-[seq]:[base32_data]
			colonIdx := strings.IndexByte(record, ':')
			if colonIdx < 0 {
				// 单片数据（无分片头）
				decoded, err := dnsBase32.DecodeString(strings.ToUpper(record))
				if err == nil && len(decoded) > 0 {
					d.deliverDownlink(decoded)
				}
				continue
			}

			header := record[:colonIdx]
			payload := record[colonIdx+1:]

			// 解析分片头: fragID-fragTotal-seq
			parts := strings.Split(header, "-")
			if len(parts) != 3 {
				continue
			}
			fragID, _ := strconv.Atoi(parts[0])
			fragTotal, _ := strconv.Atoi(parts[1])
			seq, _ := strconv.ParseUint(parts[2], 10, 32)

			decoded, err := dnsBase32.DecodeString(strings.ToUpper(payload))
			if err != nil {
				continue
			}

			if fragTotal <= 1 {
				// 单片，直接交付
				d.deliverDownlink(decoded)
			} else {
				// 多片，进入重组
				d.reassemble(uint32(seq), fragID, fragTotal, decoded)
			}
		}
	}
}

// reassemble 分片重组
func (d *DNSTransport) reassemble(seq uint32, fragID, fragTotal int, data []byte) {
	val, _ := d.reassembly.LoadOrStore(seq, &dnsReassemblyBuf{
		fragments: make(map[int][]byte),
		total:     fragTotal,
		createdAt: time.Now(),
	})
	buf := val.(*dnsReassemblyBuf)

	if _, exists := buf.fragments[fragID]; exists {
		return // 去重
	}
	buf.fragments[fragID] = data
	buf.received++

	if buf.received >= buf.total {
		// 所有分片到齐，按序拼接
		var assembled []byte
		for i := 0; i < buf.total; i++ {
			assembled = append(assembled, buf.fragments[i]...)
		}
		d.reassembly.Delete(seq)
		d.deliverDownlink(assembled)
	}
}

// deliverDownlink 交付下行数据到接收通道
func (d *DNSTransport) deliverDownlink(data []byte) {
	select {
	case d.recvChan <- data:
	default:
		// 缓冲区满，丢弃最旧
		select {
		case <-d.recvChan:
		default:
		}
		d.recvChan <- data
	}
}

// fragment 将数据切分为 DNS 可承载的分片
func (d *DNSTransport) fragment(data []byte) [][]byte {
	if len(data) <= dnsFragPayloadSize {
		return [][]byte{data}
	}

	var frags [][]byte
	for len(data) > 0 {
		end := dnsFragPayloadSize
		if end > len(data) {
			end = len(data)
		}
		frag := make([]byte, end)
		copy(frag, data[:end])
		frags = append(frags, frag)
		data = data[end:]
	}
	return frags
}

// ═══════════════════════════════════════════════════════════════
// 网关侧: 权威 DNS 服务器（监听 UDP 53）
// ═══════════════════════════════════════════════════════════════

// DNSSession DNS 会话（每个客户端 IP 一个）
type DNSSession struct {
	ClientID string
	LastSeen time.Time
	TxQueue  chan []byte // 下行数据队列（等待客户端轮询取走）
}

// DNSServer 网关侧权威 DNS 服务器
type DNSServer struct {
	domain   string
	addr     string
	server   *dns.Server
	sessions sync.Map // clientIP → *DNSSession
	onRecv   func(clientID string, data []byte)
	mu       sync.RWMutex

	// 分片重组（上行大包跨多次查询）
	reassembly sync.Map // "clientIP:seq" → *dnsReassemblyBuf
}

// NewDNSServer 创建权威 DNS 服务器
func NewDNSServer(domain string, listenAddr string) (*DNSServer, error) {
	if domain == "" {
		return nil, fmt.Errorf("DNS domain 不能为空")
	}
	if listenAddr == "" {
		listenAddr = ":53"
	}
	return &DNSServer{
		domain: domain,
		addr:   listenAddr,
	}, nil
}

// SetRecvCallback 设置收到上行数据的回调
func (s *DNSServer) SetRecvCallback(cb func(clientID string, data []byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRecv = cb
}

// Start 启动 DNS 服务器
func (s *DNSServer) Start() error {
	mux := dns.NewServeMux()
	mux.HandleFunc(s.domain+".", s.handleQuery)

	s.server = &dns.Server{
		Addr:    s.addr,
		Net:     "udp",
		Handler: mux,
	}

	log.Printf("🌐 [DNS-Server] 权威 DNS 服务器启动: %s (域名: %s)", s.addr, s.domain)
	return s.server.ListenAndServe()
}

// Stop 停止 DNS 服务器
func (s *DNSServer) Stop() error {
	if s.server != nil {
		return s.server.Shutdown()
	}
	return nil
}

// SendToClient 向客户端发送下行数据（缓存到 TxQueue，等待轮询取走）
func (s *DNSServer) SendToClient(clientID string, data []byte) error {
	session := s.getOrCreateSession(clientID)
	select {
	case session.TxQueue <- data:
		return nil
	default:
		return fmt.Errorf("客户端 %s 下行队列已满", clientID)
	}
}

// handleQuery 处理 DNS 查询
func (s *DNSServer) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	clientID := w.RemoteAddr().String()

	for _, q := range r.Question {
		// 解码查询域名
		data, fragID, fragTotal, seq, isPoll, err := DNSDecodeSubdomain(q.Name, s.domain)
		if err != nil {
			continue
		}

		// 更新会话活跃时间
		session := s.getOrCreateSession(clientID)
		session.LastSeen = time.Now()

		if !isPoll && len(data) > 0 {
			// 上行数据
			if fragTotal <= 1 {
				// 单片，直接交付
				s.deliverUplink(clientID, data)
			} else {
				// 多片，重组
				s.reassembleUplink(clientID, seq, fragID, fragTotal, data)
			}
		}

		// 无论是 poll 还是数据查询，都尝试返回下行数据
		s.appendDownlink(msg, q.Name, session)
	}

	w.WriteMsg(msg)
}

// appendDownlink 将下行数据编码为 TXT 记录附加到响应
func (s *DNSServer) appendDownlink(msg *dns.Msg, qname string, session *DNSSession) {
	select {
	case txData := <-session.TxQueue:
		// 下行数据分片编码为 TXT 记录
		fragments := s.fragmentDownlink(txData)
		seq := time.Now().UnixNano() // 下行 seq 用时间戳

		for i, frag := range fragments {
			encoded := strings.ToLower(dnsBase32.EncodeToString(frag))
			var record string
			if len(fragments) == 1 {
				record = encoded // 单片无头
			} else {
				record = fmt.Sprintf("%d-%d-%d:%s", i, len(fragments), seq, encoded)
			}

			rr := &dns.TXT{
				Hdr: dns.RR_Header{
					Name:   qname,
					Rrtype: dns.TypeTXT,
					Class:  dns.ClassINET,
					Ttl:    0, // TTL=0 禁止缓存
				},
				Txt: []string{record},
			}
			msg.Answer = append(msg.Answer, rr)
		}
	default:
		// 无下行数据，返回空标记
		rr := &dns.TXT{
			Hdr: dns.RR_Header{
				Name:   qname,
				Rrtype: dns.TypeTXT,
				Class:  dns.ClassINET,
				Ttl:    0,
			},
			Txt: []string{"empty"},
		}
		msg.Answer = append(msg.Answer, rr)
	}
}

// reassembleUplink 上行分片重组
func (s *DNSServer) reassembleUplink(clientID string, seq uint32, fragID, fragTotal int, data []byte) {
	key := fmt.Sprintf("%s:%d", clientID, seq)

	val, _ := s.reassembly.LoadOrStore(key, &dnsReassemblyBuf{
		fragments: make(map[int][]byte),
		total:     fragTotal,
		createdAt: time.Now(),
	})
	buf := val.(*dnsReassemblyBuf)

	if _, exists := buf.fragments[fragID]; exists {
		return
	}
	buf.fragments[fragID] = data
	buf.received++

	if buf.received >= buf.total {
		var assembled []byte
		for i := 0; i < buf.total; i++ {
			assembled = append(assembled, buf.fragments[i]...)
		}
		s.reassembly.Delete(key)
		s.deliverUplink(clientID, assembled)
	}
}

// deliverUplink 交付上行数据
func (s *DNSServer) deliverUplink(clientID string, data []byte) {
	s.mu.RLock()
	cb := s.onRecv
	s.mu.RUnlock()
	if cb != nil {
		cb(clientID, data)
	}
}

// fragmentDownlink 下行数据分片（TXT 记录单条上限 255 字节，Base32 编码后约 150 字节原始数据）
func (s *DNSServer) fragmentDownlink(data []byte) [][]byte {
	const maxFrag = 150 // TXT 单条 255B，Base32 编码后约 150B 原始
	if len(data) <= maxFrag {
		return [][]byte{data}
	}

	var frags [][]byte
	for len(data) > 0 {
		end := maxFrag
		if end > len(data) {
			end = len(data)
		}
		frag := make([]byte, end)
		copy(frag, data[:end])
		frags = append(frags, frag)
		data = data[end:]
	}
	return frags
}

// getOrCreateSession 获取或创建会话
func (s *DNSServer) getOrCreateSession(clientID string) *DNSSession {
	val, loaded := s.sessions.LoadOrStore(clientID, &DNSSession{
		ClientID: clientID,
		LastSeen: time.Now(),
		TxQueue:  make(chan []byte, 32),
	})
	if !loaded {
		log.Printf("🌐 [DNS-Server] 新会话: %s", clientID)
	}
	return val.(*DNSSession)
}

// CleanStaleSessions 清理过期会话（建议定期调用）
func (s *DNSServer) CleanStaleSessions(maxIdle time.Duration) {
	now := time.Now()
	s.sessions.Range(func(key, value interface{}) bool {
		session := value.(*DNSSession)
		if now.Sub(session.LastSeen) > maxIdle {
			s.sessions.Delete(key)
			log.Printf("🗑️ [DNS-Server] 清理过期会话: %s", session.ClientID)
		}
		return true
	})
}
