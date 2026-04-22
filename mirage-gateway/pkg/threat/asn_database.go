package threat

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
)

// asnEntry 内部 ASN 条目（排序数组元素）
type asnEntry struct {
	Network *net.IPNet
	Start   uint32 // 网段起始 IP（主机字节序，用于二分查找）
	ASN     uint32
	Org     string
	Country string
}

// asnFileEntry JSON 文件中的 ASN 条目格式
type asnFileEntry struct {
	Network string `json:"network"`
	ASN     uint32 `json:"asn"`
	Org     string `json:"org"`
	Country string `json:"country"`
}

// ASNDatabase ASN 离线数据库（内存排序数组 + 二分查找）
type ASNDatabase struct {
	entries []asnEntry
}

// LoadASNDatabase 从本地 JSON 文件加载 ASN 条目到内存排序数组
func LoadASNDatabase(path string) (*ASNDatabase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 ASN 数据库文件失败: %w", err)
	}

	var fileEntries []asnFileEntry
	if err := json.Unmarshal(data, &fileEntries); err != nil {
		return nil, fmt.Errorf("解析 ASN 数据库 JSON 失败: %w", err)
	}

	entries := make([]asnEntry, 0, len(fileEntries))
	for _, fe := range fileEntries {
		_, ipNet, err := net.ParseCIDR(fe.Network)
		if err != nil {
			continue // 跳过无效条目
		}
		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			continue // 仅支持 IPv4
		}
		start := binary.BigEndian.Uint32(ip4)
		entries = append(entries, asnEntry{
			Network: ipNet,
			Start:   start,
			ASN:     fe.ASN,
			Org:     fe.Org,
			Country: fe.Country,
		})
	}

	// 按网段起始 IP 排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Start < entries[j].Start
	})

	return &ASNDatabase{entries: entries}, nil
}

// Lookup 使用二分查找进行前缀匹配
func (db *ASNDatabase) Lookup(ip net.IP) *ASNInfo {
	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}
	target := binary.BigEndian.Uint32(ip4)

	// 二分查找：找到最后一个 Start <= target 的条目
	idx := sort.Search(len(db.entries), func(i int) bool {
		return db.entries[i].Start > target
	}) - 1

	if idx < 0 {
		return nil
	}

	// 从 idx 向前检查（可能有多个网段包含该 IP）
	for i := idx; i >= 0; i-- {
		entry := &db.entries[i]
		if entry.Network.Contains(ip) {
			return &ASNInfo{
				ASN:          entry.ASN,
				Org:          entry.Org,
				Country:      entry.Country,
				IsDataCenter: true,
			}
		}
		// 如果起始 IP 差距过大，提前退出
		if target-entry.Start > 0x00FFFFFF {
			break
		}
	}

	return nil
}

// Count 返回条目数量
func (db *ASNDatabase) Count() int {
	return len(db.entries)
}
