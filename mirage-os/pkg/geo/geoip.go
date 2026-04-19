// Package geo - IP 地理定位服务（全球视野坐标对齐）
package geo

import (
	"log"
	"net"

	"github.com/oschwald/geoip2-golang"
)

// Locator IP 地理定位器
type Locator struct {
	db *geoip2.Reader
}

// NewLocator 创建定位器
func NewLocator(dbPath string) (*Locator, error) {
	// 如果未提供数据库路径，使用占位实现
	if dbPath == "" {
		log.Println("GeoIP: 未提供数据库路径，使用占位实现")
		log.Println("GeoIP: 下载 MaxMind GeoLite2-City.mmdb 并配置路径以启用真实定位")
		return &Locator{db: nil}, nil
	}

	// 打开 GeoIP2 数据库
	db, err := geoip2.Open(dbPath)
	if err != nil {
		log.Printf("GeoIP: 无法打开数据库 %s，使用占位实现: %v", dbPath, err)
		return &Locator{db: nil}, nil
	}

	log.Printf("GeoIP: 成功加载数据库 %s", dbPath)
	return &Locator{db: db}, nil
}

// Resolve IP 定位（全球视野坐标对齐）
func (l *Locator) Resolve(ipStr string) (lat, lng float64, country, city string) {
	// 占位实现：返回模拟坐标
	if l.db == nil {
		// 根据 IP 最后一位模拟不同城市
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return 40.7128, -74.0060, "United States", "New York"
		}

		// 简单哈希分配到不同城市
		lastByte := ip[len(ip)-1]
		cities := []struct {
			lat, lng float64
			country  string
			city     string
		}{
			{40.7128, -74.0060, "United States", "New York"},
			{51.5074, -0.1278, "United Kingdom", "London"},
			{35.6762, 139.6503, "Japan", "Tokyo"},
			{1.3521, 103.8198, "Singapore", "Singapore"},
			{-33.8688, 151.2093, "Australia", "Sydney"},
			{48.8566, 2.3522, "France", "Paris"},
			{52.5200, 13.4050, "Germany", "Berlin"},
			{37.7749, -122.4194, "United States", "San Francisco"},
		}

		idx := int(lastByte) % len(cities)
		c := cities[idx]
		log.Printf("GeoIP Resolve (占位): %s → %s, %s (%.4f, %.4f)", ipStr, c.city, c.country, c.lat, c.lng)
		return c.lat, c.lng, c.country, c.city
	}

	// 解析 IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		log.Printf("GeoIP: 无效 IP %s", ipStr)
		return 0, 0, "Unknown", "Unknown"
	}

	// 查询 GeoIP2 数据库
	record, err := l.db.City(ip)
	if err != nil {
		log.Printf("GeoIP: 查询失败 %s: %v", ipStr, err)
		return 0, 0, "Unknown", "Unknown"
	}

	lat = record.Location.Latitude
	lng = record.Location.Longitude
	country = record.Country.Names["en"]
	city = record.City.Names["en"]

	log.Printf("GeoIP Resolve: %s → %s, %s (%.4f, %.4f)", ipStr, city, country, lat, lng)
	return lat, lng, country, city
}

// Close 关闭定位器
func (l *Locator) Close() {
	if l.db != nil {
		l.db.Close()
	}
}
