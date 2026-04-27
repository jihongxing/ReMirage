// Package cortex - 威胁情报总线
// 实现高危事件的实时推送与地理坐标标记
package cortex

import (
	"encoding/json"
	"log"
	"mirage-gateway/pkg/redact"
	"net"
	"sync"
)

// 事件类型常量
const (
	EventThreat      = "threat"
	EventHoneypot    = "honeypot"
	EventFingerprint = "fingerprint"
)

// ThreatBus 威胁情报总线
type ThreatBus struct {
	mu          sync.RWMutex
	subscribers []chan *HighSeverityEvent
	geoIP       GeoIPResolver
	minSeverity int // 最低推送等级（Stealth 模式下为 9）
}

// HighSeverityEvent 高危事件（带地理坐标）
type HighSeverityEvent struct {
	ID          string `json:"id"`
	Timestamp   int64  `json:"timestamp"`
	ThreatType  string `json:"threatType"`
	Severity    int    `json:"severity"`
	SourceIP    string `json:"srcIp"`
	SourcePort  uint16 `json:"srcPort"`
	DestPort    uint16 `json:"destPort"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Blocked     bool   `json:"blocked"`

	// 地理坐标（用于地图联动）
	Pulse     bool      `json:"pulse"`     // 是否触发脉冲
	GeoCoords []float64 `json:"geoCoords"` // [lat, lng]
	Country   string    `json:"country"`
	City      string    `json:"city"`
	ASN       string    `json:"asn"`
}

// GeoIPResolver GeoIP 解析接口
type GeoIPResolver interface {
	Lookup(ip string) (*GeoLocation, error)
}

// GeoLocation 地理位置
type GeoLocation struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Country string  `json:"country"`
	City    string  `json:"city"`
	ASN     string  `json:"asn"`
}

// NewThreatBus 创建威胁总线
func NewThreatBus(geoIP GeoIPResolver) *ThreatBus {
	return &ThreatBus{
		subscribers: make([]chan *HighSeverityEvent, 0),
		geoIP:       geoIP,
		minSeverity: 7, // 默认 >= 7 触发脉冲
	}
}

// SetMinSeverity 设置最低推送等级（用于 Stealth 模式）
func (tb *ThreatBus) SetMinSeverity(level int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.minSeverity = level
	log.Printf("[ThreatBus] 最低推送等级: %d", level)
}

// Subscribe 订阅高危事件
func (tb *ThreatBus) Subscribe() <-chan *HighSeverityEvent {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	ch := make(chan *HighSeverityEvent, 100)
	tb.subscribers = append(tb.subscribers, ch)
	return ch
}

// Unsubscribe 取消订阅
func (tb *ThreatBus) Unsubscribe(ch <-chan *HighSeverityEvent) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for i, sub := range tb.subscribers {
		if sub == ch {
			close(sub)
			tb.subscribers = append(tb.subscribers[:i], tb.subscribers[i+1:]...)
			break
		}
	}
}

// EmitHighSeverityEvent 发射高危事件
func (tb *ThreatBus) EmitHighSeverityEvent(event *HighSeverityEvent) {
	tb.mu.RLock()
	minSev := tb.minSeverity
	tb.mu.RUnlock()

	// 检查是否达到推送阈值
	if event.Severity < minSev {
		return
	}

	// 解析地理坐标
	if tb.geoIP != nil && event.SourceIP != "" {
		if geo, err := tb.geoIP.Lookup(event.SourceIP); err == nil {
			event.GeoCoords = []float64{geo.Lat, geo.Lng}
			event.Country = geo.Country
			event.City = geo.City
			event.ASN = geo.ASN
		}
	}

	// 高危事件触发脉冲
	event.Pulse = event.Severity >= 7

	// 广播给所有订阅者（含断路器保护）
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	for _, sub := range tb.subscribers {
		// 断路器：队列利用率 > 80% 时丢弃低优先级事件（severity < 5）
		queueCap := cap(sub)
		queueLen := len(sub)
		if queueCap > 0 && float64(queueLen)/float64(queueCap) > 0.8 && event.Severity < 5 {
			log.Printf("[ThreatBus] 断路器触发: 丢弃低优先级事件 (severity=%d, queue=%d/%d)",
				event.Severity, queueLen, queueCap)
			continue
		}
		select {
		case sub <- event:
		default:
			// 通道满，跳过
		}
	}

	log.Printf("[ThreatBus] 高危事件: %s (Severity=%d, IP=%s, Coords=%v)",
		event.ThreatType, event.Severity, redact.RedactIP(event.SourceIP), event.GeoCoords)
}

// ToJSON 序列化为 JSON（用于 WebSocket 推送）
func (e *HighSeverityEvent) ToJSON() []byte {
	data, _ := json.Marshal(e)
	return data
}

// DefaultGeoIPResolver 默认 GeoIP 解析器（基于 IP 段估算）
type DefaultGeoIPResolver struct {
	// 可扩展为 MaxMind GeoIP2 数据库
}

// NewDefaultGeoIPResolver 创建默认解析器
func NewDefaultGeoIPResolver() *DefaultGeoIPResolver {
	return &DefaultGeoIPResolver{}
}

// Lookup 查询 IP 地理位置
func (r *DefaultGeoIPResolver) Lookup(ipStr string) (*GeoLocation, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, nil
	}

	// 简化实现：基于 IP 段估算
	// 生产环境应使用 MaxMind GeoIP2
	ipv4 := ip.To4()
	if ipv4 == nil {
		return &GeoLocation{Lat: 0, Lng: 0, Country: "Unknown"}, nil
	}

	firstOctet := int(ipv4[0])
	secondOctet := int(ipv4[1])

	// 基于 IP 段的粗略地理估算
	switch {
	case firstOctet >= 1 && firstOctet <= 126:
		// A 类地址 - 北美
		return &GeoLocation{
			Lat:     37.0 + float64(secondOctet%30),
			Lng:     -122.0 + float64(secondOctet%60),
			Country: "US",
			City:    "San Francisco",
			ASN:     "AS15169",
		}, nil
	case firstOctet >= 128 && firstOctet <= 191:
		// B 类地址 - 欧洲
		return &GeoLocation{
			Lat:     48.0 + float64(secondOctet%20),
			Lng:     2.0 + float64(secondOctet%30),
			Country: "EU",
			City:    "Paris",
			ASN:     "AS3215",
		}, nil
	case firstOctet >= 192 && firstOctet <= 223:
		// C 类地址 - 亚太
		return &GeoLocation{
			Lat:     22.0 + float64(secondOctet%30),
			Lng:     114.0 + float64(secondOctet%40),
			Country: "APAC",
			City:    "Hong Kong",
			ASN:     "AS4134",
		}, nil
	default:
		return &GeoLocation{Lat: 0, Lng: 0, Country: "Unknown"}, nil
	}
}
