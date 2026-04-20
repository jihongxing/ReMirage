//go:build linux

// Package gswitch - DNS-less eBPF 劫持控制面 (Linux)
// 通过 cgroup sock_addr 实现零开销透明代理
//
// 工作流：
//  1. 加载 bpf/sock_hijack.o
//  2. 挂载到 cgroup（connect4 + sendmsg4 + recvmsg4）
//  3. ResonanceResolver 拉到新 IP 后，写入 gw_ip_map
//  4. 所有发往 198.18.0.1 的流量自动重定向到真实 Gateway
package gswitch

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

// DNSlessHijacker DNS-less eBPF 劫持器 (Linux)
type DNSlessHijacker struct {
	collection *ebpf.Collection
	links      []link.Link
	gwIPMap    *ebpf.Map
	enabledMap *ebpf.Map
	cgroupPath string
}

// NewDNSlessHijacker 创建劫持器
func NewDNSlessHijacker(cgroupPath string) *DNSlessHijacker {
	if cgroupPath == "" {
		cgroupPath = "/sys/fs/cgroup"
	}
	return &DNSlessHijacker{
		cgroupPath: cgroupPath,
	}
}

// LoadAndAttach 加载 eBPF 程序并挂载到 cgroup
func (dh *DNSlessHijacker) LoadAndAttach() error {
	// 1. 加载 .o 文件
	spec, err := ebpf.LoadCollectionSpec("bpf/sock_hijack.o")
	if err != nil {
		return fmt.Errorf("加载 sock_hijack.o 失败: %w", err)
	}

	objs, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("创建 collection 失败: %w", err)
	}
	dh.collection = objs

	// 2. 获取 Map 引用
	dh.gwIPMap = objs.Maps["gw_ip_map"]
	dh.enabledMap = objs.Maps["hijack_enabled_map"]
	if dh.gwIPMap == nil || dh.enabledMap == nil {
		return fmt.Errorf("Map 不存在: gw_ip_map=%v, hijack_enabled_map=%v",
			dh.gwIPMap != nil, dh.enabledMap != nil)
	}

	// 3. 挂载 connect4
	if prog := objs.Programs["hijack_tcp_connect"]; prog != nil {
		l, err := link.AttachCgroup(link.CgroupOptions{
			Path:    dh.cgroupPath,
			Attach:  ebpf.AttachCGroupInet4Connect,
			Program: prog,
		})
		if err != nil {
			return fmt.Errorf("挂载 connect4 失败: %w", err)
		}
		dh.links = append(dh.links, l)
	}

	// 4. 挂载 sendmsg4
	if prog := objs.Programs["hijack_udp_sendmsg"]; prog != nil {
		l, err := link.AttachCgroup(link.CgroupOptions{
			Path:    dh.cgroupPath,
			Attach:  ebpf.AttachCGroupUDP4Sendmsg,
			Program: prog,
		})
		if err != nil {
			return fmt.Errorf("挂载 sendmsg4 失败: %w", err)
		}
		dh.links = append(dh.links, l)
	}

	// 5. 挂载 recvmsg4
	if prog := objs.Programs["hijack_udp_recvmsg"]; prog != nil {
		l, err := link.AttachCgroup(link.CgroupOptions{
			Path:    dh.cgroupPath,
			Attach:  ebpf.AttachCGroupUDP4Recvmsg,
			Program: prog,
		})
		if err != nil {
			return fmt.Errorf("挂载 recvmsg4 失败: %w", err)
		}
		dh.links = append(dh.links, l)
	}

	log.Println("[DNSlessHijacker] ✅ eBPF sock_addr 已挂载到 cgroup")
	return nil
}

// SetGatewayIP 设置真实 Gateway IP（ResonanceResolver 回调）
func (dh *DNSlessHijacker) SetGatewayIP(ip net.IP, port uint16) error {
	if dh.gwIPMap == nil {
		return fmt.Errorf("gw_ip_map 未初始化")
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("仅支持 IPv4: %s", ip.String())
	}

	// key=0: IP（网络字节序）
	ipKey := uint32(0)
	ipVal := binary.BigEndian.Uint32(ip4)
	if err := dh.gwIPMap.Put(&ipKey, &ipVal); err != nil {
		return fmt.Errorf("写入 gw_ip_map[0] 失败: %w", err)
	}

	// key=1: Port
	// bpf_sock_addr.user_port 是 __be16（网络字节序 16-bit），存储在 __u32 中
	// 内核期望的格式：高 16 位为 0，低 16 位为 port 的网络字节序
	portKey := uint32(1)
	portBE := uint32(port>>8) | uint32(port&0xFF)<<8 // htons
	if err := dh.gwIPMap.Put(&portKey, &portBE); err != nil {
		return fmt.Errorf("写入 gw_ip_map[1] 失败: %w", err)
	}

	log.Printf("[DNSlessHijacker] Gateway IP 已更新: %s:%d", ip4.String(), port)
	return nil
}

// Enable 启用劫持
func (dh *DNSlessHijacker) Enable() error {
	if dh.enabledMap == nil {
		return fmt.Errorf("hijack_enabled_map 未初始化")
	}
	key := uint32(0)
	val := uint32(1)
	return dh.enabledMap.Put(&key, &val)
}

// Disable 禁用劫持（流量直通）
func (dh *DNSlessHijacker) Disable() error {
	if dh.enabledMap == nil {
		return fmt.Errorf("hijack_enabled_map 未初始化")
	}
	key := uint32(0)
	val := uint32(0)
	return dh.enabledMap.Put(&key, &val)
}

// Close 卸载并清理
func (dh *DNSlessHijacker) Close() error {
	for _, l := range dh.links {
		l.Close()
	}
	if dh.collection != nil {
		dh.collection.Close()
	}
	log.Println("[DNSlessHijacker] eBPF sock_addr 已卸载")
	return nil
}

// IsAvailable 检查 eBPF sock_addr 是否可用
func IsHijackAvailable() bool {
	_, err := os.Stat("/sys/fs/cgroup")
	return err == nil
}
