// Package wsgateway - GeoIP 定位服务
package wsgateway

import (
	"net"

	"github.com/oschwald/geoip2-golang"
)

// GeoIPService GeoIP 服务
type GeoIPService struct {
	db *geoip2.Reader
}

// GeoLocation 地理位置
type GeoLocation struct {
	IP        string  `json:"ip"`
	Country   string  `json:"country"`
	City      string  `json:"city"`
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
	Timezone  string  `json:"timezone"`
}

// NewGeoIPService 创建 GeoIP 服务
func NewGeoIPService(dbPath string) (*GeoIPService, error) {
	db, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, err
	}

	return &GeoIPService{db: db}, nil
}

// Lookup 查询 IP 地理位置
func (g *GeoIPService) Lookup(ipStr string) (*GeoLocation, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, nil
	}

	record, err := g.db.City(ip)
	if err != nil {
		return nil, err
	}

	city := ""
	if len(record.City.Names) > 0 {
		if name, ok := record.City.Names["en"]; ok {
			city = name
		}
	}

	country := ""
	if len(record.Country.Names) > 0 {
		if name, ok := record.Country.Names["en"]; ok {
			country = name
		}
	}

	return &GeoLocation{
		IP:        ipStr,
		Country:   country,
		City:      city,
		Latitude:  record.Location.Latitude,
		Longitude: record.Location.Longitude,
		Timezone:  record.Location.TimeZone,
	}, nil
}

// Close 关闭数据库
func (g *GeoIPService) Close() error {
	return g.db.Close()
}
