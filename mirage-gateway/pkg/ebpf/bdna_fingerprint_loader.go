package ebpf

import (
	"fmt"
	"log"
	"os"

	"go.yaml.in/yaml/v2"
)

// BrowserFingerprint 浏览器指纹模板（Go 侧，对应 C 侧 struct stack_fingerprint）
type BrowserFingerprint struct {
	ProfileID    uint32 `yaml:"profile_id"`
	ProfileName  string `yaml:"profile_name"`
	Browser      string `yaml:"browser"`
	Version      string `yaml:"version"`
	OS           string `yaml:"os"`
	TCPWindow    uint16 `yaml:"tcp_window"`
	TCPWScale    uint8  `yaml:"tcp_wscale"`
	TCPMSS       uint16 `yaml:"tcp_mss"`
	TCPSackOK    uint8  `yaml:"tcp_sack_ok"`
	TCPTimestamp uint8  `yaml:"tcp_timestamps"`
	// QUIC 参数
	QUICMaxIdle       uint32 `yaml:"quic_max_idle"`
	QUICMaxData       uint32 `yaml:"quic_max_data"`
	QUICMaxStreamsBi  uint32 `yaml:"quic_max_streams_bi"`
	QUICMaxStreamsUni uint32 `yaml:"quic_max_streams_uni"`
	QUICAckDelayExp   uint16 `yaml:"quic_ack_delay_exp"`
}

// fingerprintConfig YAML 配置文件根结构
type fingerprintConfig struct {
	Fingerprints []BrowserFingerprint `yaml:"fingerprints"`
}

// LoadFingerprintsFromYAML 从 YAML 文件加载指纹模板
func LoadFingerprintsFromYAML(path string) ([]BrowserFingerprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取指纹配置文件失败: %w", err)
	}

	var cfg fingerprintConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析指纹配置文件失败: %w", err)
	}

	if len(cfg.Fingerprints) == 0 {
		return nil, fmt.Errorf("指纹配置文件为空")
	}

	return cfg.Fingerprints, nil
}

// stackFingerprint C 侧 struct stack_fingerprint 的 Go 对齐结构体
// 用于写入 eBPF fingerprint_map
type stackFingerprint struct {
	TCPWindow         uint16
	TCPWScale         uint8
	TCPMSS            uint16
	TCPSackOK         uint8
	TCPTimestamps     uint8
	_pad0             uint8 // alignment
	QUICMaxIdle       uint32
	QUICMaxData       uint32
	QUICMaxStreamsBi  uint32
	QUICMaxStreamsUni uint32
	QUICAckDelayExp   uint16
	TLSVersion        uint16
	TLSExtOrder       [32]uint8
	TLSExtCount       uint8
	_pad1             [3]uint8 // alignment
	ProfileID         uint32
	ProfileName       [32]byte
}

// toStackFingerprint 将 BrowserFingerprint 转换为 C 侧结构体
func (bf *BrowserFingerprint) toStackFingerprint() stackFingerprint {
	sf := stackFingerprint{
		TCPWindow:         bf.TCPWindow,
		TCPWScale:         bf.TCPWScale,
		TCPMSS:            bf.TCPMSS,
		TCPSackOK:         bf.TCPSackOK,
		TCPTimestamps:     bf.TCPTimestamp,
		QUICMaxIdle:       bf.QUICMaxIdle,
		QUICMaxData:       bf.QUICMaxData,
		QUICMaxStreamsBi:  bf.QUICMaxStreamsBi,
		QUICMaxStreamsUni: bf.QUICMaxStreamsUni,
		QUICAckDelayExp:   bf.QUICAckDelayExp,
		TLSVersion:        0x0304, // TLS 1.3
		ProfileID:         bf.ProfileID,
	}
	copy(sf.ProfileName[:], bf.ProfileName)
	return sf
}

// SyncFingerprintsToMap 将指纹模板写入 eBPF fingerprint_map
func (da *DefenseApplier) SyncFingerprintsToMap(fingerprints []BrowserFingerprint) error {
	fpMap := da.loader.GetMap("fingerprint_map")
	if fpMap == nil {
		return fmt.Errorf("fingerprint_map 不存在")
	}

	var failCount int
	for _, fp := range fingerprints {
		if fp.ProfileID >= 64 {
			log.Printf("[B-DNA] ⚠️ 跳过 profile_id=%d（超出 max_entries=64）", fp.ProfileID)
			failCount++
			continue
		}
		sf := fp.toStackFingerprint()
		key := fp.ProfileID
		if err := fpMap.Put(&key, &sf); err != nil {
			log.Printf("[B-DNA] ❌ 写入 fingerprint_map[%d] 失败: %v", fp.ProfileID, err)
			failCount++
		}
	}

	if failCount > 0 {
		log.Printf("[B-DNA] 指纹模板同步: %d/%d 成功, %d 失败",
			len(fingerprints)-failCount, len(fingerprints), failCount)
	} else {
		log.Printf("[B-DNA] ✅ 指纹模板同步完成: %d 个模板", len(fingerprints))
	}

	return nil
}

// LoadAndSyncFingerprints 从 YAML 文件加载指纹模板并写入 eBPF Map
func (da *DefenseApplier) LoadAndSyncFingerprints(configPath string) error {
	fingerprints, err := LoadFingerprintsFromYAML(configPath)
	if err != nil {
		return fmt.Errorf("加载指纹模板失败: %w", err)
	}

	return da.SyncFingerprintsToMap(fingerprints)
}
