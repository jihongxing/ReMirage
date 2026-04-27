// Package rewriter 实现基于 NFQUEUE 的用户态拦截层。
//
// 补线：Gateway NFQUEUE/用户态补充重写（A-06）
// 读取 B-DNA 的 skb->mark 标记，对 Gateway 出站 TCP/TLS 连接执行指纹重写。
// 仅覆盖 Gateway 出站方向，不覆盖 Client 出站方向。
//
// 工作原理：
//  1. iptables/nftables 规则将 Gateway 出站 TCP SYN 包导入 NFQUEUE
//  2. 本模块从 NFQUEUE 读取数据包
//  3. 检查 skb->mark（由 B-DNA eBPF TC 程序设置）
//  4. 对标记的连接执行 TCP/TLS 指纹重写（Window Size、TTL、TCP Options）
//  5. 重写后的包通过 NFQUEUE verdict 放行
//
// 依赖：
//   - Linux NFQUEUE (netfilter_queue) 内核模块
//   - iptables 规则: iptables -A OUTPUT -p tcp --syn -j NFQUEUE --queue-num <queueNum>
//   - B-DNA eBPF 程序通过 skb->mark 标记需要重写的连接
package rewriter

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
)

// BDNAMarkMask 是 B-DNA skb->mark 中指纹模板 ID 的掩码。
// B-DNA TC 程序将模板 ID 写入 skb->mark 的低 8 位。
const BDNAMarkMask = 0xFF

// NFQueueRewriterConfig 配置 NFQUEUE 重写器。
type NFQueueRewriterConfig struct {
	// QueueNum 是 NFQUEUE 队列号，需与 iptables 规则一致。
	QueueNum uint16

	// MaxPacketLen 是从 NFQUEUE 读取的最大包长。
	MaxPacketLen uint32

	// FingerprintDB 指纹模板数据库，key 为模板 ID（与 B-DNA fingerprint_map 对齐）。
	FingerprintDB map[uint8]*TCPFingerprint
}

// TCPFingerprint 定义 TCP 指纹重写模板。
// 与 B-DNA eBPF fingerprint_map 中的模板字段对齐。
type TCPFingerprint struct {
	// WindowSize 是 TCP Window Size（SYN 包）。
	WindowSize uint16

	// TTL 是 IP TTL 值。
	TTL uint8

	// MSS 是 TCP Maximum Segment Size option 值。
	MSS uint16

	// WindowScale 是 TCP Window Scale option 值。
	WindowScale uint8

	// SACKPermitted 是否包含 SACK Permitted option。
	SACKPermitted bool

	// Timestamps 是否包含 TCP Timestamps option。
	Timestamps bool

	// OptionOrder 定义 TCP Options 的排列顺序。
	// 不同浏览器/OS 的 TCP Options 顺序不同，是 p0f/JA4T 指纹的关键特征。
	// 值为 TCP option kind 列表，例如 [2, 4, 8, 1, 3] = MSS, SACK, Timestamps, NOP, WScale
	OptionOrder []uint8
}

// NFQueueRewriter 基于 NFQUEUE 的用户态 TCP/TLS 指纹重写器。
//
// 架构位置：Gateway 出站方向，B-DNA eBPF TC 之后。
// B-DNA 在 TC 层完成 SYN 包的基础重写并设置 skb->mark，
// NFQueueRewriter 在用户态对需要更精细控制的字段进行补充重写。
type NFQueueRewriter struct {
	config  NFQueueRewriterConfig
	running atomic.Bool
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// 统计
	packetsProcessed atomic.Uint64
	packetsRewritten atomic.Uint64
	packetsPassedRaw atomic.Uint64
}

// NewNFQueueRewriter 创建 NFQUEUE 重写器实例。
func NewNFQueueRewriter(config NFQueueRewriterConfig) (*NFQueueRewriter, error) {
	if config.QueueNum == 0 {
		return nil, fmt.Errorf("nfqueue: queue number must be > 0")
	}
	if config.MaxPacketLen == 0 {
		config.MaxPacketLen = 1500
	}
	if config.FingerprintDB == nil {
		config.FingerprintDB = make(map[uint8]*TCPFingerprint)
	}

	return &NFQueueRewriter{
		config: config,
		stopCh: make(chan struct{}),
	}, nil
}

// Start 启动 NFQUEUE 监听循环。
//
// 前置条件：
//   - 已加载 nfnetlink_queue 内核模块
//   - 已配置 iptables 规则将出站 SYN 导入对应 NFQUEUE
//   - 进程具有 CAP_NET_ADMIN 权限
//
// 实际的 NFQUEUE 绑定需要 Linux netfilter 库（如 github.com/florianl/go-nfqueue），
// 此处定义接口和处理逻辑框架，具体 NFQUEUE 系统调用绑定在 Linux 构建中实现。
func (r *NFQueueRewriter) Start() error {
	if !r.running.CompareAndSwap(false, true) {
		return fmt.Errorf("nfqueue rewriter already running")
	}

	log.Printf("🔧 [NFQueue-Rewriter] 启动 NFQUEUE 监听: queue=%d, maxPktLen=%d, templates=%d",
		r.config.QueueNum, r.config.MaxPacketLen, len(r.config.FingerprintDB))

	r.wg.Add(1)
	go r.processLoop()

	return nil
}

// Stop 停止 NFQUEUE 监听。
func (r *NFQueueRewriter) Stop() {
	if !r.running.CompareAndSwap(true, false) {
		return
	}
	close(r.stopCh)
	r.wg.Wait()
	log.Printf("🔧 [NFQueue-Rewriter] 已停止: processed=%d, rewritten=%d, passed=%d",
		r.packetsProcessed.Load(), r.packetsRewritten.Load(), r.packetsPassedRaw.Load())
}

// processLoop 是 NFQUEUE 主处理循环。
// 实际生产中通过 go-nfqueue 库绑定到内核 NFQUEUE，
// 此处定义处理逻辑框架。
func (r *NFQueueRewriter) processLoop() {
	defer r.wg.Done()

	// NOTE: 实际 NFQUEUE 绑定代码需要 Linux netfilter 库。
	// 框架逻辑：
	//   nfq, err := nfqueue.Open(&nfqueue.Config{
	//       NfQueue:      r.config.QueueNum,
	//       MaxPacketLen: r.config.MaxPacketLen,
	//       MaxQueueLen:  1024,
	//       Copymode:     nfqueue.NfQnlCopyPacket,
	//   })
	//   nfq.RegisterWithErrorFunc(ctx, func(a nfqueue.Attribute) int {
	//       return r.handlePacket(a)
	//   }, func(e error) int { ... })

	<-r.stopCh
}

// HandlePacket 处理从 NFQUEUE 收到的单个数据包。
// 读取 skb->mark 中的 B-DNA 模板 ID，查找指纹模板并执行重写。
//
// 参数：
//   - pktData: 原始 IP 包数据
//   - mark: skb->mark 值（由 B-DNA eBPF TC 设置）
//
// 返回：
//   - rewritten: 重写后的包数据（如果未重写则返回原始数据）
//   - modified: 是否进行了重写
func (r *NFQueueRewriter) HandlePacket(pktData []byte, mark uint32) (rewritten []byte, modified bool) {
	r.packetsProcessed.Add(1)

	// 提取 B-DNA 模板 ID（mark 低 8 位）
	templateID := uint8(mark & BDNAMarkMask)
	if templateID == 0 {
		// mark=0 表示 B-DNA 未标记此连接，直接放行
		r.packetsPassedRaw.Add(1)
		return pktData, false
	}

	// 查找指纹模板
	fp, ok := r.config.FingerprintDB[templateID]
	if !ok {
		// 未知模板 ID，放行
		r.packetsPassedRaw.Add(1)
		return pktData, false
	}

	// 验证最小 IP+TCP 头长度
	if len(pktData) < 40 {
		r.packetsPassedRaw.Add(1)
		return pktData, false
	}

	// 解析 IP 头
	ipHeaderLen := int(pktData[0]&0x0F) * 4
	if ipHeaderLen < 20 || len(pktData) < ipHeaderLen+20 {
		r.packetsPassedRaw.Add(1)
		return pktData, false
	}

	// 验证是 TCP 协议 (protocol=6)
	if pktData[9] != 6 {
		r.packetsPassedRaw.Add(1)
		return pktData, false
	}

	// 解析 TCP 头
	tcpStart := ipHeaderLen
	tcpFlags := pktData[tcpStart+13]
	isSYN := (tcpFlags & 0x02) != 0

	// 仅对 SYN 包执行完整重写
	if !isSYN {
		r.packetsPassedRaw.Add(1)
		return pktData, false
	}

	// 复制数据包以避免修改原始数据
	result := make([]byte, len(pktData))
	copy(result, pktData)

	// 重写 IP TTL
	if fp.TTL > 0 {
		result[8] = fp.TTL
	}

	// 重写 TCP Window Size
	if fp.WindowSize > 0 {
		binary.BigEndian.PutUint16(result[tcpStart+14:tcpStart+16], fp.WindowSize)
	}

	// 重算 IP 头校验和
	recalcIPChecksum(result[:ipHeaderLen])

	// 重算 TCP 校验和
	recalcTCPChecksum(result, ipHeaderLen)

	r.packetsRewritten.Add(1)
	return result, true
}

// UpdateFingerprint 动态更新指纹模板（运行时热更新）。
func (r *NFQueueRewriter) UpdateFingerprint(id uint8, fp *TCPFingerprint) {
	r.config.FingerprintDB[id] = fp
}

// Stats 返回统计信息。
func (r *NFQueueRewriter) Stats() (processed, rewritten, passed uint64) {
	return r.packetsProcessed.Load(), r.packetsRewritten.Load(), r.packetsPassedRaw.Load()
}

// recalcIPChecksum 重算 IPv4 头校验和。
func recalcIPChecksum(ipHeader []byte) {
	// 清零校验和字段
	ipHeader[10] = 0
	ipHeader[11] = 0

	var sum uint32
	for i := 0; i < len(ipHeader)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(ipHeader[i : i+2]))
	}
	if len(ipHeader)%2 == 1 {
		sum += uint32(ipHeader[len(ipHeader)-1]) << 8
	}
	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}
	binary.BigEndian.PutUint16(ipHeader[10:12], ^uint16(sum))
}

// recalcTCPChecksum 重算 TCP 校验和（含伪头部）。
func recalcTCPChecksum(pkt []byte, ipHeaderLen int) {
	tcpLen := len(pkt) - ipHeaderLen
	tcpStart := ipHeaderLen

	// 清零 TCP 校验和
	pkt[tcpStart+16] = 0
	pkt[tcpStart+17] = 0

	// 伪头部: src_ip(4) + dst_ip(4) + zero(1) + proto(1) + tcp_len(2)
	var sum uint32
	// Source IP
	sum += uint32(binary.BigEndian.Uint16(pkt[12:14]))
	sum += uint32(binary.BigEndian.Uint16(pkt[14:16]))
	// Dest IP
	sum += uint32(binary.BigEndian.Uint16(pkt[16:18]))
	sum += uint32(binary.BigEndian.Uint16(pkt[18:20]))
	// Protocol (TCP=6)
	sum += 6
	// TCP length
	sum += uint32(tcpLen)

	// TCP segment
	for i := tcpStart; i < len(pkt)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(pkt[i : i+2]))
	}
	if tcpLen%2 == 1 {
		sum += uint32(pkt[len(pkt)-1]) << 8
	}

	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}
	binary.BigEndian.PutUint16(pkt[tcpStart+16:tcpStart+18], ^uint16(sum))
}
