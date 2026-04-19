// Package provisioning - 硬件旁路网关固件配置
// 为预刷入 Mirage 环境的物理设备生成固件级配置
// 目标硬件：ARM64 SBC（伪装为智能家居网关/电视盒子）
package provisioning

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// HardwareProfile 硬件设备配置档案
type HardwareProfile struct {
	// 设备标识
	DeviceID     string `json:"device_id"`
	HardwareType string `json:"hardware_type"` // rpi4, nanopi-r5s, gl-mt3000
	FirmwareVer  string `json:"firmware_ver"`

	// 网络配置（旁路模式）
	NetworkMode  string `json:"network_mode"` // bridge, gateway, transparent
	LANInterface string `json:"lan_interface"`
	WANInterface string `json:"wan_interface"`
	LANIP        string `json:"lan_ip"`
	LANSubnet    string `json:"lan_subnet"`
	DNSOverride  string `json:"dns_override"` // 劫持 DNS 到本地

	// Mirage 核心配置
	ClientConfig *ClientConfig `json:"client_config"`

	// eBPF 配置（设备端也运行精简版 eBPF）
	EBPFMode     string `json:"ebpf_mode"` // full, lite, none
	XDPInterface string `json:"xdp_interface"`

	// 伪装配置
	DisguiseType string `json:"disguise_type"` // smart_home, tv_box, nas
	MDNSName     string `json:"mdns_name"`     // 局域网广播名称
	UPnPProfile  string `json:"upnp_profile"`  // UPnP 设备描述

	// 自毁配置
	SelfDestructEnabled bool `json:"self_destruct_enabled"`
	HeartbeatTimeout    int  `json:"heartbeat_timeout_sec"`
	TamperDetection     bool `json:"tamper_detection"` // 物理拆机检测
	WipeOnUSBRemoval    bool `json:"wipe_on_usb_removal"`
}

// SupportedHardware 支持的硬件平台
var SupportedHardware = map[string]HardwareSpec{
	"rpi4": {
		Name:         "Raspberry Pi 4",
		Arch:         "arm64",
		MinRAM:       2048,
		EBPFSupport:  true,
		XDPSupport:   false, // RPi4 网卡不支持 XDP native
		Interfaces:   []string{"eth0", "wlan0"},
		DisguiseHint: "smart_home",
	},
	"nanopi-r5s": {
		Name:         "NanoPi R5S",
		Arch:         "arm64",
		MinRAM:       4096,
		EBPFSupport:  true,
		XDPSupport:   true, // 双 2.5G 网口支持 XDP
		Interfaces:   []string{"eth0", "eth1", "eth2"},
		DisguiseHint: "nas",
	},
	"gl-mt3000": {
		Name:         "GL.iNet MT3000",
		Arch:         "arm64",
		MinRAM:       512,
		EBPFSupport:  true,
		XDPSupport:   false,
		Interfaces:   []string{"eth0", "ra0"},
		DisguiseHint: "smart_home",
	},
}

// HardwareSpec 硬件规格
type HardwareSpec struct {
	Name         string
	Arch         string
	MinRAM       int // MB
	EBPFSupport  bool
	XDPSupport   bool
	Interfaces   []string
	DisguiseHint string
}

// GenerateHardwareProfile 为指定硬件生成完整固件配置
func (p *Provisioner) GenerateHardwareProfile(
	uid string,
	hardwareType string,
	networkMode string,
) (*HardwareProfile, error) {
	spec, ok := SupportedHardware[hardwareType]
	if !ok {
		return nil, fmt.Errorf("不支持的硬件类型: %s", hardwareType)
	}

	// 生成设备专属 Ed25519 密钥对
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// 分配蜂窝
	cellID, cellLevel, region, err := p.allocateShadowCell(uid)
	if err != nil {
		return nil, err
	}

	endpoints := p.getAvailableEndpoints(region)

	// 生成设备 ID
	devIDBytes := make([]byte, 8)
	rand.Read(devIDBytes)
	deviceID := fmt.Sprintf("hw-%s-%s", hardwareType, hex.EncodeToString(devIDBytes))

	// 确定 eBPF 模式
	ebpfMode := "none"
	if spec.EBPFSupport {
		ebpfMode = "lite"
		if spec.XDPSupport {
			ebpfMode = "full"
		}
	}

	// 确定网络接口
	lanIface := "eth0"
	wanIface := "eth0"
	if len(spec.Interfaces) >= 2 {
		lanIface = spec.Interfaces[0]
		wanIface = spec.Interfaces[1]
	}

	profile := &HardwareProfile{
		DeviceID:     deviceID,
		HardwareType: hardwareType,
		FirmwareVer:  "1.0.0",
		NetworkMode:  networkMode,
		LANInterface: lanIface,
		WANInterface: wanIface,
		LANIP:        "192.168.8.1",
		LANSubnet:    "192.168.8.0/24",
		DNSOverride:  "192.168.8.1",
		ClientConfig: &ClientConfig{
			Version:     1,
			GeneratedAt: time.Now(),
			ExpiresAt:   time.Now().Add(365 * 24 * time.Hour), // 硬件设备 1 年有效
			UID:         uid,
			CellID:      cellID,
			CellLevel:   cellLevel,
			Region:      region,
			PrivateKey:  hex.EncodeToString(privKey),
			PublicKey:   hex.EncodeToString(pubKey),
			Endpoints:   endpoints,
			SNI:         "cdn.cloudflare.com",
		},
		EBPFMode:            ebpfMode,
		XDPInterface:        wanIface,
		DisguiseType:        spec.DisguiseHint,
		MDNSName:            generateDisguiseName(spec.DisguiseHint),
		UPnPProfile:         generateUPnPProfile(spec.DisguiseHint),
		SelfDestructEnabled: true,
		HeartbeatTimeout:    300,
		TamperDetection:     true,
		WipeOnUSBRemoval:    false,
	}

	// 持久化公钥
	p.persistCredentials(uid, hex.EncodeToString(pubKey))

	return profile, nil
}

// ExportFirmwareConfig 导出固件配置（写入 SD 卡/eMMC 的 JSON）
func ExportFirmwareConfig(profile *HardwareProfile) ([]byte, error) {
	return json.MarshalIndent(profile, "", "  ")
}

// generateDisguiseName 生成伪装 mDNS 名称
func generateDisguiseName(disguiseType string) string {
	switch disguiseType {
	case "smart_home":
		return "SmartHub-Pro"
	case "tv_box":
		return "MediaBox-4K"
	case "nas":
		return "HomeNAS-Mini"
	default:
		return "IoT-Device"
	}
}

// generateUPnPProfile 生成 UPnP 设备描述
func generateUPnPProfile(disguiseType string) string {
	switch disguiseType {
	case "smart_home":
		return "urn:schemas-upnp-org:device:SmartHomeGateway:1"
	case "tv_box":
		return "urn:schemas-upnp-org:device:MediaRenderer:1"
	case "nas":
		return "urn:schemas-upnp-org:device:MediaServer:1"
	default:
		return "urn:schemas-upnp-org:device:Basic:1"
	}
}
