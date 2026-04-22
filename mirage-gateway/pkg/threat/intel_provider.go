package threat

import (
	"fmt"
	"net"
	"sync"
)

// ASNInfo ASN 查询结果
type ASNInfo struct {
	ASN          uint32
	Org          string
	Country      string
	IsDataCenter bool
}

// CloudProvider 云厂商类型
type CloudProvider string

const (
	CloudAWS     CloudProvider = "aws"
	CloudAzure   CloudProvider = "azure"
	CloudGCP     CloudProvider = "gcp"
	CloudAliyun  CloudProvider = "aliyun"
	CloudTencent CloudProvider = "tencent"
)

// ThreatIntelProvider 威胁情报提供器（纯本地内存查询，禁止网络 I/O）
type ThreatIntelProvider struct {
	mu          sync.RWMutex
	asnDB       *ASNDatabase
	cloudRanges *CloudRangeDB
}

// NewThreatIntelProvider 从本地文件加载威胁情报库
func NewThreatIntelProvider(asnPath, cloudRangesPath string) (*ThreatIntelProvider, error) {
	asnDB, err := LoadASNDatabase(asnPath)
	if err != nil {
		return nil, fmt.Errorf("加载 ASN 数据库失败: %w", err)
	}

	cloudRanges, err := LoadCloudRanges(cloudRangesPath)
	if err != nil {
		return nil, fmt.Errorf("加载云厂商网段失败: %w", err)
	}

	return &ThreatIntelProvider{
		asnDB:       asnDB,
		cloudRanges: cloudRanges,
	}, nil
}

// LookupASN 查询 IP 的 ASN 信息（纯内存查询，O(log n)）
func (tip *ThreatIntelProvider) LookupASN(ip string) *ASNInfo {
	tip.mu.RLock()
	defer tip.mu.RUnlock()

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil
	}
	return tip.asnDB.Lookup(parsed)
}

// IsCloudIP 检查 IP 是否属于云厂商数据中心
func (tip *ThreatIntelProvider) IsCloudIP(ip string) (bool, CloudProvider) {
	tip.mu.RLock()
	defer tip.mu.RUnlock()

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false, ""
	}
	return tip.cloudRanges.Match(parsed)
}

// Reload 热更新威胁情报库（OS 下发新数据后调用）
func (tip *ThreatIntelProvider) Reload(asnPath, cloudRangesPath string) error {
	newASN, err := LoadASNDatabase(asnPath)
	if err != nil {
		return fmt.Errorf("重载 ASN 数据库失败: %w", err)
	}

	newCloud, err := LoadCloudRanges(cloudRangesPath)
	if err != nil {
		return fmt.Errorf("重载云厂商网段失败: %w", err)
	}

	tip.mu.Lock()
	tip.asnDB = newASN
	tip.cloudRanges = newCloud
	tip.mu.Unlock()

	return nil
}

// GetASNBlockEntries 导出 ASN 数据库条目为 eBPF 可用的 ASNBlockEntry 格式
func (tip *ThreatIntelProvider) GetASNBlockEntries() []ASNBlockEntryExport {
	tip.mu.RLock()
	defer tip.mu.RUnlock()

	if tip.asnDB == nil {
		return nil
	}

	entries := make([]ASNBlockEntryExport, 0, len(tip.asnDB.entries))
	for _, e := range tip.asnDB.entries {
		entries = append(entries, ASNBlockEntryExport{
			CIDR: e.Network.String(),
			ASN:  e.ASN,
		})
	}
	return entries
}

// ASNBlockEntryExport ASN 黑名单导出条目（与 ebpf.ASNBlockEntry 对齐）
type ASNBlockEntryExport struct {
	CIDR string
	ASN  uint32
}
