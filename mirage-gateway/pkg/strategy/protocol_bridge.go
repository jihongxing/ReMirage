// 协议微调桥接 - 用户个性化 eBPF 配置
package strategy

import (
	"encoding/binary"
	"errors"
	"sync"

	"github.com/cilium/ebpf"
)

type ProtocolBridge struct {
	mu              sync.RWMutex
	userConfigMap   *ebpf.Map // per-UID 配置
	globalConfigMap *ebpf.Map
	userConfigs     map[string]*UserProtocolConfig
}

type UserProtocolConfig struct {
	UID string `json:"uid"`
	
	// B-DNA 拟态配置
	BDNA struct {
		ForcedMimicry  string `json:"forced_mimicry"`  // zoom, teams, netflix, etc.
		AutoSwitch     bool   `json:"auto_switch"`
		SwitchInterval int    `json:"switch_interval"` // seconds
	} `json:"bdna"`
	
	// G-Tunnel 配置
	GTunnel struct {
		FECRedundancy       int  `json:"fec_redundancy"`        // RS(10, N)
		CIDRotationInterval int  `json:"cid_rotation_interval"` // seconds
		MultipathEnabled    bool `json:"multipath_enabled"`
		PathCount           int  `json:"path_count"`
	} `json:"gtunnel"`
	
	// Jitter-Lite 配置
	Jitter struct {
		Enabled  bool   `json:"enabled"`
		Profile  string `json:"profile"` // residential, mobile, corporate, satellite
		Variance int    `json:"variance"` // ms
	} `json:"jitter"`
	
	// NPM 配置
	NPM struct {
		PaddingEnabled bool `json:"padding_enabled"`
		PaddingSize    int  `json:"padding_size"` // bytes
		BurstMode      bool `json:"burst_mode"`
	} `json:"npm"`
}

// eBPF Map 中的配置结构
type EBPFUserConfig struct {
	MimicryType     uint8
	AutoSwitch      uint8
	SwitchInterval  uint16
	FECRedundancy   uint8
	CIDInterval     uint8
	MultipathCount  uint8
	JitterEnabled   uint8
	JitterProfile   uint8
	JitterVariance  uint16
	PaddingEnabled  uint8
	PaddingSize     uint16
	BurstMode       uint8
	_padding        [2]byte
}

func NewProtocolBridge(userConfigMap, globalConfigMap *ebpf.Map) *ProtocolBridge {
	return &ProtocolBridge{
		userConfigMap:   userConfigMap,
		globalConfigMap: globalConfigMap,
		userConfigs:     make(map[string]*UserProtocolConfig),
	}
}

// ValidateConfig 校验配置参数（沙箱化安全检查）
func (b *ProtocolBridge) ValidateConfig(cfg *UserProtocolConfig) error {
	// ===== 严格范围检查（防止资源耗尽攻击）=====
	
	// FEC 冗余度校验：2-8 范围，超出可能导致 CPU 过载
	if cfg.GTunnel.FECRedundancy < 2 || cfg.GTunnel.FECRedundancy > 8 {
		return errors.New("fec_redundancy must be between 2 and 8")
	}
	
	// CID 轮换间隔校验：10-120 秒，过短会导致状态表爆炸
	if cfg.GTunnel.CIDRotationInterval < 10 || cfg.GTunnel.CIDRotationInterval > 120 {
		return errors.New("cid_rotation_interval must be between 10 and 120")
	}
	
	// 路径数校验：2-5 条，过多会耗尽连接资源
	if cfg.GTunnel.MultipathEnabled && (cfg.GTunnel.PathCount < 2 || cfg.GTunnel.PathCount > 5) {
		return errors.New("path_count must be between 2 and 5")
	}
	
	// Jitter 方差校验：5-50ms，过大会导致连接超时
	if cfg.Jitter.Enabled && (cfg.Jitter.Variance < 5 || cfg.Jitter.Variance > 50) {
		return errors.New("jitter_variance must be between 5 and 50")
	}
	
	// Padding 大小校验：32-512 bytes，过大会浪费带宽
	if cfg.NPM.PaddingEnabled && (cfg.NPM.PaddingSize < 32 || cfg.NPM.PaddingSize > 512) {
		return errors.New("padding_size must be between 32 and 512")
	}
	
	// 切换间隔校验：600-7200 秒，过短会导致频繁切换
	if cfg.BDNA.AutoSwitch && (cfg.BDNA.SwitchInterval < 600 || cfg.BDNA.SwitchInterval > 7200) {
		return errors.New("switch_interval must be between 600 and 7200")
	}
	
	// 拟态类型校验（白名单）
	validMimicry := map[string]bool{
		"":           true, // auto
		"zoom":       true,
		"teams":      true,
		"netflix":    true,
		"youtube":    true,
		"cloudflare": true,
	}
	if !validMimicry[cfg.BDNA.ForcedMimicry] {
		return errors.New("invalid mimicry type")
	}
	
	// Jitter Profile 校验（白名单）
	validProfile := map[string]bool{
		"":            true,
		"residential": true,
		"mobile":      true,
		"corporate":   true,
		"satellite":   true,
	}
	if !validProfile[cfg.Jitter.Profile] {
		return errors.New("invalid jitter profile")
	}
	
	return nil
}

// ApplyConfig 应用用户配置到 eBPF
func (b *ProtocolBridge) ApplyConfig(cfg *UserProtocolConfig) error {
	if err := b.ValidateConfig(cfg); err != nil {
		return err
	}
	
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// 保存配置
	b.userConfigs[cfg.UID] = cfg
	
	// 转换为 eBPF 结构
	ebpfCfg := b.toEBPFConfig(cfg)
	
	// 写入 eBPF Map
	return b.writeToMap(cfg.UID, ebpfCfg)
}

// toEBPFConfig 转换为 eBPF 配置结构
func (b *ProtocolBridge) toEBPFConfig(cfg *UserProtocolConfig) *EBPFUserConfig {
	ebpfCfg := &EBPFUserConfig{
		SwitchInterval: uint16(cfg.BDNA.SwitchInterval),
		FECRedundancy:  uint8(cfg.GTunnel.FECRedundancy),
		CIDInterval:    uint8(cfg.GTunnel.CIDRotationInterval),
		JitterVariance: uint16(cfg.Jitter.Variance),
		PaddingSize:    uint16(cfg.NPM.PaddingSize),
	}
	
	// 拟态类型映射
	mimicryMap := map[string]uint8{
		"":           0, // auto
		"zoom":       1,
		"teams":      2,
		"netflix":    3,
		"youtube":    4,
		"cloudflare": 5,
	}
	ebpfCfg.MimicryType = mimicryMap[cfg.BDNA.ForcedMimicry]
	
	// 布尔值转换
	if cfg.BDNA.AutoSwitch {
		ebpfCfg.AutoSwitch = 1
	}
	if cfg.GTunnel.MultipathEnabled {
		ebpfCfg.MultipathCount = uint8(cfg.GTunnel.PathCount)
	}
	if cfg.Jitter.Enabled {
		ebpfCfg.JitterEnabled = 1
	}
	if cfg.NPM.PaddingEnabled {
		ebpfCfg.PaddingEnabled = 1
	}
	if cfg.NPM.BurstMode {
		ebpfCfg.BurstMode = 1
	}
	
	// Jitter Profile 映射
	profileMap := map[string]uint8{
		"":            0,
		"residential": 1,
		"mobile":      2,
		"corporate":   3,
		"satellite":   4,
	}
	ebpfCfg.JitterProfile = profileMap[cfg.Jitter.Profile]
	
	return ebpfCfg
}

// writeToMap 写入 eBPF Map
func (b *ProtocolBridge) writeToMap(uid string, cfg *EBPFUserConfig) error {
	if b.userConfigMap == nil {
		return nil
	}
	
	var key [12]byte
	copy(key[:], uid)
	
	// 序列化配置
	value := make([]byte, 16)
	value[0] = cfg.MimicryType
	value[1] = cfg.AutoSwitch
	binary.LittleEndian.PutUint16(value[2:4], cfg.SwitchInterval)
	value[4] = cfg.FECRedundancy
	value[5] = cfg.CIDInterval
	value[6] = cfg.MultipathCount
	value[7] = cfg.JitterEnabled
	value[8] = cfg.JitterProfile
	binary.LittleEndian.PutUint16(value[9:11], cfg.JitterVariance)
	value[11] = cfg.PaddingEnabled
	binary.LittleEndian.PutUint16(value[12:14], cfg.PaddingSize)
	value[14] = cfg.BurstMode
	
	return b.userConfigMap.Put(key, value)
}

// GetConfig 获取用户配置
func (b *ProtocolBridge) GetConfig(uid string) *UserProtocolConfig {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.userConfigs[uid]
}

// RemoveConfig 移除用户配置
func (b *ProtocolBridge) RemoveConfig(uid string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	delete(b.userConfigs, uid)
	
	if b.userConfigMap == nil {
		return nil
	}
	
	var key [12]byte
	copy(key[:], uid)
	
	return b.userConfigMap.Delete(key)
}

// GetDefaultConfig 获取默认配置
func GetDefaultConfig(uid string) *UserProtocolConfig {
	cfg := &UserProtocolConfig{UID: uid}
	cfg.BDNA.AutoSwitch = true
	cfg.BDNA.SwitchInterval = 3600
	cfg.GTunnel.FECRedundancy = 4
	cfg.GTunnel.CIDRotationInterval = 30
	cfg.GTunnel.MultipathEnabled = true
	cfg.GTunnel.PathCount = 3
	cfg.Jitter.Enabled = true
	cfg.Jitter.Profile = "residential"
	cfg.Jitter.Variance = 15
	cfg.NPM.PaddingEnabled = true
	cfg.NPM.PaddingSize = 128
	return cfg
}
