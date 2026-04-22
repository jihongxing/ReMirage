package threat

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
)

// CloudRangeDB 云厂商网段数据库
type CloudRangeDB struct {
	ranges map[CloudProvider][]*net.IPNet
}

// LoadCloudRanges 从本地 JSON 文件加载各云厂商 CIDR 列表
// JSON 格式: {"aws": ["3.0.0.0/15", ...], "azure": [...], ...}
func LoadCloudRanges(path string) (*CloudRangeDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取云厂商网段文件失败: %w", err)
	}

	var raw map[string][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析云厂商网段 JSON 失败: %w", err)
	}

	db := &CloudRangeDB{
		ranges: make(map[CloudProvider][]*net.IPNet),
	}

	for provider, cidrs := range raw {
		cp := CloudProvider(provider)
		nets := make([]*net.IPNet, 0, len(cidrs))
		for _, cidr := range cidrs {
			_, ipNet, err := net.ParseCIDR(cidr)
			if err != nil {
				continue // 跳过无效 CIDR
			}
			nets = append(nets, ipNet)
		}
		db.ranges[cp] = nets
	}

	return db, nil
}

// Match 检查 IP 是否命中云厂商网段
func (db *CloudRangeDB) Match(ip net.IP) (bool, CloudProvider) {
	for provider, nets := range db.ranges {
		for _, ipNet := range nets {
			if ipNet.Contains(ip) {
				return true, provider
			}
		}
	}
	return false, ""
}

// Count 返回所有云厂商网段总数
func (db *CloudRangeDB) Count() int {
	total := 0
	for _, nets := range db.ranges {
		total += len(nets)
	}
	return total
}
