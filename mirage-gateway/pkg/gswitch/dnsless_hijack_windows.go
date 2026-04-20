//go:build windows

// Package gswitch - DNS-less IP 头重写 (Windows/Wintun)
// 在 Wintun TUN 接口的读取循环中，对发往假 IP 的包进行目标地址替换
//
// 工作流：
//  1. 客户端配置代理地址为 198.18.0.1
//  2. 系统路由将 198.18.0.1/32 指向 Wintun 接口
//  3. Wintun 读出 IP 包后，替换 dst_ip 为真实 Gateway IP
//  4. 重算 IPv4 Checksum + L4 伪首部 Checksum
//  5. 回包时反向替换 src_ip（真实 IP → 假 IP）
package gswitch

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync/atomic"
)

// FakeIP 客户端配置的假代理地址
var FakeIPv4 = net.IPv4(198, 18, 0, 1).To4()

// DNSlessHijacker Windows DNS-less 劫持器（IP 头重写）
type DNSlessHijacker struct {
	realIP   atomic.Value // net.IP (4 bytes)
	realPort atomic.Uint32
	enabled  atomic.Bool
}

// NewDNSlessHijacker 创建 Windows 劫持器
func NewDNSlessHijacker(_ string) *DNSlessHijacker {
	dh := &DNSlessHijacker{}
	dh.realIP.Store(net.IPv4zero.To4())
	return dh
}

// LoadAndAttach Windows 上无需加载 eBPF，仅初始化
func (dh *DNSlessHijacker) LoadAndAttach() error {
	log.Println("[DNSlessHijacker] Windows 模式：IP 头重写已就绪")
	return nil
}

// SetGatewayIP 设置真实 Gateway IP
func (dh *DNSlessHijacker) SetGatewayIP(ip net.IP, port uint16) error {
	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("仅支持 IPv4: %s", ip.String())
	}
	dh.realIP.Store(ip4)
	dh.realPort.Store(uint32(port))
	log.Printf("[DNSlessHijacker] Gateway IP 已更新: %s:%d", ip4.String(), port)
	return nil
}

// Enable 启用劫持
func (dh *DNSlessHijacker) Enable() error {
	dh.enabled.Store(true)
	return nil
}

// Disable 禁用劫持
func (dh *DNSlessHijacker) Disable() error {
	dh.enabled.Store(false)
	return nil
}

// Close 清理
func (dh *DNSlessHijacker) Close() error {
	dh.enabled.Store(false)
	return nil
}

// RewriteOutbound 重写出站包的目标 IP（Wintun 读取后调用）
// 如果 dst_ip == FakeIP，替换为真实 Gateway IP 并修正 Checksum
// 返回 true 表示已重写
func (dh *DNSlessHijacker) RewriteOutbound(packet []byte) bool {
	if !dh.enabled.Load() {
		return false
	}
	if len(packet) < 20 {
		return false
	}

	// IPv4 版本检查
	if packet[0]>>4 != 4 {
		return false
	}

	// DNS 泄漏防护：拦截所有发往外部 UDP 53 的 DNS 查询
	// 防止 Windows NCSI、systemd-resolved 等系统进程泄漏 DNS
	proto := packet[9]
	if proto == 17 { // UDP
		ihl := int(packet[0]&0x0F) * 4
		if ihl+4 <= len(packet) {
			dstPort := binary.BigEndian.Uint16(packet[ihl+2 : ihl+4])
			if dstPort == 53 {
				// 检查目标 IP 是否为本地 DNS（允许）或外部 DNS（拦截）
				dstIP := packet[16:20]
				if !isLocalDNS(dstIP) {
					// 静默丢弃外部 DNS 查询，防止泄漏
					return true // 返回 true 表示"已处理"（实际丢弃）
				}
			}
		}
	}

	// 目标 IP 在 offset 16-19
	dstIP := packet[16:20]
	if dstIP[0] != FakeIPv4[0] || dstIP[1] != FakeIPv4[1] ||
		dstIP[2] != FakeIPv4[2] || dstIP[3] != FakeIPv4[3] {
		return false
	}

	// 获取真实 IP
	realIP, ok := dh.realIP.Load().(net.IP)
	if !ok || realIP.Equal(net.IPv4zero) {
		return false
	}

	// 替换目标 IP
	copy(packet[16:20], realIP.To4())

	// 重算 IPv4 Header Checksum
	ihl2 := int(packet[0]&0x0F) * 4
	recalcIPv4Checksum(packet[:ihl2])

	// 重算 L4 Checksum（TCP/UDP 伪首部包含 IP）
	if proto == 6 || proto == 17 { // TCP or UDP
		recalcL4Checksum(packet, ihl2, proto)
	}

	return true
}

// RewriteInbound 重写入站包的源 IP（发送到 Wintun 前调用）
// 如果 src_ip == 真实 Gateway IP，替换为假 IP
func (dh *DNSlessHijacker) RewriteInbound(packet []byte) bool {
	if !dh.enabled.Load() {
		return false
	}
	if len(packet) < 20 {
		return false
	}
	if packet[0]>>4 != 4 {
		return false
	}

	realIP, ok := dh.realIP.Load().(net.IP)
	if !ok || realIP.Equal(net.IPv4zero) {
		return false
	}
	realIP4 := realIP.To4()

	// 源 IP 在 offset 12-15
	srcIP := packet[12:16]
	if srcIP[0] != realIP4[0] || srcIP[1] != realIP4[1] ||
		srcIP[2] != realIP4[2] || srcIP[3] != realIP4[3] {
		return false
	}

	// 替换源 IP 为假 IP
	copy(packet[12:16], FakeIPv4)

	// 重算 Checksum
	ihl := int(packet[0]&0x0F) * 4
	recalcIPv4Checksum(packet[:ihl])

	proto := packet[9]
	if proto == 6 || proto == 17 {
		recalcL4Checksum(packet, ihl, proto)
	}

	return true
}

// recalcIPv4Checksum 重算 IPv4 头部校验和
func recalcIPv4Checksum(header []byte) {
	// 清零原校验和
	header[10] = 0
	header[11] = 0

	var sum uint32
	for i := 0; i < len(header)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(header[i : i+2]))
	}
	if len(header)%2 == 1 {
		sum += uint32(header[len(header)-1]) << 8
	}

	// 折叠进位
	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}

	checksum := ^uint16(sum)
	binary.BigEndian.PutUint16(header[10:12], checksum)
}

// recalcL4Checksum 重算 TCP/UDP 校验和（含伪首部）
func recalcL4Checksum(packet []byte, ihl int, proto byte) {
	totalLen := int(binary.BigEndian.Uint16(packet[2:4]))
	l4Len := totalLen - ihl
	if l4Len <= 0 || ihl+l4Len > len(packet) {
		return
	}

	l4Data := packet[ihl:]
	if len(l4Data) < l4Len {
		return
	}
	l4Data = l4Data[:l4Len]

	// 清零 L4 校验和
	var csumOffset int
	switch proto {
	case 6: // TCP: offset 16
		if len(l4Data) < 18 {
			return
		}
		csumOffset = 16
	case 17: // UDP: offset 6
		if len(l4Data) < 8 {
			return
		}
		csumOffset = 6
	default:
		return
	}
	l4Data[csumOffset] = 0
	l4Data[csumOffset+1] = 0

	// 伪首部：src_ip(4) + dst_ip(4) + zero(1) + proto(1) + l4_len(2)
	var sum uint32
	// src IP
	sum += uint32(binary.BigEndian.Uint16(packet[12:14]))
	sum += uint32(binary.BigEndian.Uint16(packet[14:16]))
	// dst IP
	sum += uint32(binary.BigEndian.Uint16(packet[16:18]))
	sum += uint32(binary.BigEndian.Uint16(packet[18:20]))
	// proto + length
	sum += uint32(proto)
	sum += uint32(l4Len)

	// L4 数据
	for i := 0; i < len(l4Data)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(l4Data[i : i+2]))
	}
	if len(l4Data)%2 == 1 {
		sum += uint32(l4Data[len(l4Data)-1]) << 8
	}

	// 折叠
	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}

	checksum := ^uint16(sum)
	if proto == 17 && checksum == 0 {
		checksum = 0xFFFF // UDP 校验和 0 表示未计算
	}
	binary.BigEndian.PutUint16(l4Data[csumOffset:csumOffset+2], checksum)
}

// IsHijackAvailable Windows 上始终可用
func IsHijackAvailable() bool {
	return true
}

// isLocalDNS 检查 IP 是否为本地/内网 DNS（允许通过）
// 本地 DNS: 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
func isLocalDNS(ip []byte) bool {
	if len(ip) < 4 {
		return false
	}
	// 127.0.0.0/8 (loopback)
	if ip[0] == 127 {
		return true
	}
	// 10.0.0.0/8
	if ip[0] == 10 {
		return true
	}
	// 172.16.0.0/12
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return true
	}
	// 192.168.0.0/16
	if ip[0] == 192 && ip[1] == 168 {
		return true
	}
	return false
}
