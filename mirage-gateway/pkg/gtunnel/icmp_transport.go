// Package gtunnel - ICMP Tunnel 传输（Go 控制面）
// 伪装形态：网络连通性诊断（Ping）
//
// 架构：
//
//	发送 (Egress): Go Raw Socket 构造合法 ICMP Echo Request（用户态发包）
//	接收 (Ingress): eBPF TC Hook 拦截 ICMP Reply → Ring Buffer → Go 读取
//
// 设计原则：
//   - 令牌桶限速：宁可牺牲带宽，绝不触发 ICMP Flood 封杀
//   - Raw Socket 发包：比内核态凭空造包稳定，天然跨平台
//   - Identifier 固定 + Sequence 严格递增：伪装合法 Ping 行为
package gtunnel

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
	"golang.org/x/time/rate"

	ebpfTypes "mirage-gateway/pkg/ebpf"
)

// ICMPTransport ICMP 隧道传输
type ICMPTransport struct {
	// eBPF 接收通道
	configMap *ebpf.Map       // icmp_config_map: 配置下发
	rxReader  *ringbuf.Reader // icmp_data_events: C → Go

	// Raw Socket 发送通道
	rawConn net.PacketConn // ip4:icmp raw socket
	target  net.IP         // 目标 Gateway IP

	// 接收缓冲
	recvChan chan []byte

	// 令牌桶限速器（golang.org/x/time/rate）
	limiter *rate.Limiter

	// ICMP 伪装状态
	identifier uint16 // 连接时随机生成，生命周期内固定
	seqNum     uint32 // 从 1 开始严格递增

	// RTT 探测
	pendingPings sync.Map // seq(uint16) → sentTime(time.Time)
	rtt          time.Duration
	remoteAddr   net.Addr

	// 状态
	connected int32
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewICMPTransport 创建 ICMP 隧道传输
func NewICMPTransport(configMap, txMap *ebpf.Map, rxRingbuf *ebpf.Map, config ICMPTransportConfig) (*ICMPTransport, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// 创建 Ring Buffer Reader（接收通道）
	reader, err := ringbuf.NewReader(rxRingbuf)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建 Ring Buffer Reader 失败: %w", err)
	}

	// 创建 Raw Socket（发送通道）
	rawConn, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		reader.Close()
		cancel()
		return nil, fmt.Errorf("创建 ICMP Raw Socket 失败: %w", err)
	}

	// 生成随机 Identifier（固定不变，伪装合法 Ping 进程）
	var idBuf [2]byte
	rand.Read(idBuf[:])
	identifier := binary.BigEndian.Uint16(idBuf[:])
	if identifier == 0 {
		identifier = 1 // 避免零值
	}

	t := &ICMPTransport{
		configMap:  configMap,
		rxReader:   reader,
		rawConn:    rawConn,
		target:     config.TargetIP.To4(),
		recvChan:   make(chan []byte, 256),
		limiter:    rate.NewLimiter(rate.Limit(2), 10), // 2 pkt/s 稳态, 10 突发
		identifier: identifier,
		seqNum:     0,
		ctx:        ctx,
		cancel:     cancel,
	}

	if config.TargetIP != nil {
		t.remoteAddr = &net.IPAddr{IP: config.TargetIP}
	}

	// 使用配置中的 Identifier（如果指定）
	if config.Identifier != 0 {
		t.identifier = config.Identifier
	}

	// 下发配置到 eBPF Map（告诉内核拦截哪些 Reply）
	icmpCfg := ebpfTypes.ICMPConfig{
		Enabled:    1,
		TargetIP:   ipToUint32(config.TargetIP),
		GatewayIP:  ipToUint32(config.GatewayIP),
		Identifier: t.identifier,
	}
	key := uint32(0)
	if err := configMap.Put(key, icmpCfg); err != nil {
		reader.Close()
		rawConn.Close()
		cancel()
		return nil, fmt.Errorf("写入 icmp_config_map 失败: %w", err)
	}

	atomic.StoreInt32(&t.connected, 1)

	// 启动接收循环
	go t.recvLoop()

	log.Printf("[ICMP] 隧道已建立: target=%s, id=0x%04X, rate=2pkt/s",
		config.TargetIP, t.identifier)

	return t, nil
}

// ============================================================
// 发送：Go Raw Socket 构造合法 ICMP Echo Request
// ============================================================

// Send 发送数据（令牌桶限速 + Raw Socket 发包）
func (t *ICMPTransport) Send(data []byte) error {
	if atomic.LoadInt32(&t.connected) == 0 {
		return io.ErrClosedPipe
	}
	if len(data) > 1024 {
		return fmt.Errorf("数据超过 ICMP MTU: %d > 1024", len(data))
	}

	// 令牌桶限速：宁可阻塞/丢包，绝不越线
	if !t.limiter.Allow() {
		return fmt.Errorf("ICMP 速率限制：令牌桶为空")
	}

	// Sequence 严格递增（从 1 开始）
	seq := uint16(atomic.AddUint32(&t.seqNum, 1))

	// 记录发送时间（RTT 探测）
	t.pendingPings.Store(seq, time.Now())

	// 构造 ICMP Echo Request 报文
	packet := t.buildEchoRequest(seq, data)

	// 通过 Raw Socket 发送
	dst := &net.IPAddr{IP: t.target}
	if _, err := t.rawConn.WriteTo(packet, dst); err != nil {
		t.pendingPings.Delete(seq)
		return fmt.Errorf("ICMP 发送失败: %w", err)
	}

	return nil
}

// buildEchoRequest 构造合法的 ICMP Echo Request
// 格式: [Type 1B][Code 1B][Checksum 2B][Identifier 2B][Sequence 2B][Payload...]
func (t *ICMPTransport) buildEchoRequest(seq uint16, payload []byte) []byte {
	pktLen := 8 + len(payload) // ICMP header(8) + payload
	pkt := make([]byte, pktLen)

	pkt[0] = 8 // Type: Echo Request
	pkt[1] = 0 // Code: 0
	pkt[2] = 0 // Checksum (先置零)
	pkt[3] = 0
	binary.BigEndian.PutUint16(pkt[4:6], t.identifier)
	binary.BigEndian.PutUint16(pkt[6:8], seq)

	// 填充 Payload（加密后的数据）
	copy(pkt[8:], payload)

	// 计算 ICMP Checksum
	csum := icmpChecksum(pkt)
	binary.BigEndian.PutUint16(pkt[2:4], csum)

	return pkt
}

// icmpChecksum 计算 ICMP 校验和
func icmpChecksum(data []byte) uint16 {
	var sum uint32
	length := len(data)

	for i := 0; i < length-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if length%2 == 1 {
		sum += uint32(data[length-1]) << 8
	}

	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}

	return ^uint16(sum)
}

// ============================================================
// 接收：eBPF Ring Buffer → Go
// ============================================================

// recvLoop 从 Ring Buffer 读取 eBPF 拦截的 ICMP Reply Payload
func (t *ICMPTransport) recvLoop() {
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		record, err := t.rxReader.Read()
		if err != nil {
			if err == ringbuf.ErrClosed {
				return
			}
			log.Printf("⚠️  [ICMP] Ring Buffer 读取错误: %v", err)
			continue
		}

		var event ebpfTypes.ICMPRxEvent
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Printf("⚠️  [ICMP] 反序列化事件失败: %v", err)
			continue
		}

		if event.DataLen > 0 && event.DataLen <= 1024 {
			data := make([]byte, event.DataLen)
			copy(data, event.Data[:event.DataLen])

			// RTT 探测：匹配 Sequence 计算往返时间
			if sentAt, ok := t.pendingPings.LoadAndDelete(event.Seq); ok {
				rtt := time.Since(sentAt.(time.Time))
				t.mu.Lock()
				// 指数移动平均 (EMA) 平滑 RTT
				if t.rtt == 0 {
					t.rtt = rtt
				} else {
					t.rtt = t.rtt*7/8 + rtt/8
				}
				t.mu.Unlock()
			}

			select {
			case t.recvChan <- data:
			default:
				// 缓冲区满，丢弃最旧的
			}
		}
	}
}

// Recv 接收数据
func (t *ICMPTransport) Recv() ([]byte, error) {
	select {
	case data, ok := <-t.recvChan:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-t.ctx.Done():
		return nil, io.EOF
	}
}

// ============================================================
// 接口实现 + 生命周期管理
// ============================================================

// Close 关闭连接并释放资源
func (t *ICMPTransport) Close() error {
	atomic.StoreInt32(&t.connected, 0)
	t.cancel()

	// 禁用 eBPF 配置（停止内核态拦截）
	key := uint32(0)
	disabledCfg := ebpfTypes.ICMPConfig{Enabled: 0}
	t.configMap.Put(key, disabledCfg)

	if t.rxReader != nil {
		t.rxReader.Close()
	}
	if t.rawConn != nil {
		t.rawConn.Close()
	}

	log.Printf("[ICMP] 隧道已关闭: id=0x%04X", t.identifier)
	return nil
}

// Type 传输类型
func (t *ICMPTransport) Type() TransportType { return TransportICMP }

// RTT 返回平滑后的 RTT
func (t *ICMPTransport) RTT() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.rtt
}

// RemoteAddr 远端地址
func (t *ICMPTransport) RemoteAddr() net.Addr { return t.remoteAddr }

// MaxDatagramSize ICMP Payload 上限
func (t *ICMPTransport) MaxDatagramSize() int { return 1024 }

// IsConnected 连接状态
func (t *ICMPTransport) IsConnected() bool {
	return atomic.LoadInt32(&t.connected) == 1
}

// SetRate 动态调整发送速率（Orchestrator 可根据网络状况调整）
func (t *ICMPTransport) SetRate(packetsPerSecond float64) {
	t.limiter.SetLimit(rate.Limit(packetsPerSecond))
	log.Printf("[ICMP] 速率已调整: %.1f pkt/s", packetsPerSecond)
}

// ipToUint32 将 net.IP 转换为网络字节序 uint32
func ipToUint32(ip net.IP) uint32 {
	if ip == nil {
		return 0
	}
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip)
}
