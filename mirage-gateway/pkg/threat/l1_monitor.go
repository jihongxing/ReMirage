package threat

import (
	"bytes"
	"context"
	"encoding/binary"
	"log"
	"net"
	"time"

	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/redact"

	"github.com/cilium/ebpf/ringbuf"
)

// RiskScoreAdder 风险评分接口（避免 threat ↔ cortex 循环依赖）
type RiskScoreAdder interface {
	AddScore(ip string, delta int, source string)
}

// L1Monitor L1 纵深防御事件监听器
type L1Monitor struct {
	loader        *ebpf.Loader
	riskScorer    RiskScoreAdder
	intelProvider *ThreatIntelProvider
	lastStats     ebpf.L1Stats // 上次快照，用于 delta 计算
}

// NewL1Monitor 创建 L1 事件监听器
func NewL1Monitor(loader *ebpf.Loader, riskScorer RiskScoreAdder) *L1Monitor {
	return &L1Monitor{
		loader:     loader,
		riskScorer: riskScorer,
	}
}

// SetIntelProvider 注入威胁情报提供器（用于 LookupASN/IsCloudIP 运行时调用）
func (m *L1Monitor) SetIntelProvider(provider *ThreatIntelProvider) {
	m.intelProvider = provider
}

// StartEventLoop 启动事件监听循环
func (m *L1Monitor) StartEventLoop(ctx context.Context) {
	go m.readEvents(ctx)
	go m.updateStats(ctx)
}

// readEvents 从 l1_defense_events Ring Buffer 读取速率限制事件
func (m *L1Monitor) readEvents(ctx context.Context) {
	eventsMap := m.loader.GetMap("l1_defense_events")
	if eventsMap == nil {
		log.Println("[L1Monitor] ⚠️ l1_defense_events Map 不存在，跳过事件监听")
		return
	}

	reader, err := ringbuf.NewReader(eventsMap)
	if err != nil {
		log.Printf("[L1Monitor] ❌ 创建 Ring Buffer 读取器失败: %v", err)
		return
	}
	defer reader.Close()

	log.Println("[L1Monitor] ✅ 开始监听 L1 防御事件")

	go func() {
		<-ctx.Done()
		reader.Close()
	}()

	for {
		record, err := reader.Read()
		if err != nil {
			if err == ringbuf.ErrClosed {
				log.Println("[L1Monitor] Ring Buffer 已关闭")
				return
			}
			log.Printf("[L1Monitor] 读取事件错误: %v", err)
			continue
		}
		m.handleRateEvent(record.RawSample)
	}
}

// handleRateEvent 处理速率限制事件
func (m *L1Monitor) handleRateEvent(data []byte) {
	var event ebpf.RateEvent
	buf := bytes.NewReader(data)
	if err := binary.Read(buf, binary.LittleEndian, &event); err != nil {
		log.Printf("[L1Monitor] 解析 rate_event 失败: %v", err)
		return
	}

	ip := uint32ToIP(event.SourceIP)
	m.riskScorer.AddScore(ip, 20, "rate_limit")

	// LookupASN/IsCloudIP 运行时调用点
	if m.intelProvider != nil {
		if asnInfo := m.intelProvider.LookupASN(ip); asnInfo != nil {
			log.Printf("[L1Monitor] 速率限制触发: IP=%s, ASN=%d (%s)", redact.RedactIP(ip), asnInfo.ASN, asnInfo.Org)
			m.riskScorer.AddScore(ip, 10, "asn_datacenter")
		}
		if isCloud, provider := m.intelProvider.IsCloudIP(ip); isCloud {
			log.Printf("[L1Monitor] 云厂商 IP 检测: IP=%s, Provider=%s", redact.RedactIP(ip), provider)
			m.riskScorer.AddScore(ip, 15, "cloud_ip")
		}
	}

	log.Printf("[L1Monitor] 速率限制触发: IP=%s, Type=%d, Rate=%d",
		redact.RedactIP(ip), event.TriggerType, event.CurrentRate)
}

// updateStats 定期从 l1_stats_map 读取统计并更新 Prometheus 指标
func (m *L1Monitor) updateStats(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.syncStats()
		}
	}
}

// syncStats 从 l1_stats_map 读取统计并按 delta 更新 Prometheus 指标
func (m *L1Monitor) syncStats() {
	statsMap := m.loader.GetMap("l1_stats_map")
	if statsMap == nil {
		return
	}

	key := uint32(0)
	var stats ebpf.L1Stats
	if err := statsMap.Lookup(&key, &stats); err != nil {
		log.Printf("[L1Monitor] 读取 l1_stats_map 失败: %v", err)
		return
	}

	gatewayID := GetGatewayID()

	// 计算 delta（当前值 - 上次快照）
	asnDelta := stats.ASNDrops - m.lastStats.ASNDrops
	rateDelta := stats.RateDrops - m.lastStats.RateDrops
	silentDelta := stats.SilentDrops - m.lastStats.SilentDrops

	if asnDelta > 0 {
		ASNDropTotal.WithLabelValues(gatewayID).Add(float64(asnDelta))
	}
	if rateDelta > 0 {
		RateLimitDropTotal.WithLabelValues(gatewayID, "syn").Add(float64(rateDelta))
	}
	if silentDelta > 0 {
		SilentDropTotal.WithLabelValues(gatewayID).Add(float64(silentDelta))
	}

	// 保存当前快照
	m.lastStats = stats
}

// uint32ToIP 将网络字节序 uint32 转换为 IP 字符串
func uint32ToIP(ipUint32 uint32) string {
	ipBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(ipBytes, ipUint32)
	return net.IP(ipBytes).String()
}
