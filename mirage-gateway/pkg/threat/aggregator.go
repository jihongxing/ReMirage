package threat

import (
	"context"
	"fmt"
	"log"
	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/evaluator"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// dedupEntry 去重条目
type dedupEntry struct {
	event    *UnifiedThreatEvent
	lastSeen time.Time
}

// Aggregator 威胁事件聚合器
type Aggregator struct {
	inCh      chan *UnifiedThreatEvent
	outCh     chan *UnifiedThreatEvent
	dedup     map[string]*dedupEntry // key: "{sourceIP}:{eventType}"
	mu        sync.Mutex
	maxQueue  int
	dropCount uint64
}

// NewAggregator 创建聚合器
func NewAggregator(maxQueue int) *Aggregator {
	return &Aggregator{
		inCh:     make(chan *UnifiedThreatEvent, maxQueue),
		outCh:    make(chan *UnifiedThreatEvent, maxQueue),
		dedup:    make(map[string]*dedupEntry),
		maxQueue: maxQueue,
	}
}

// Start 启动聚合循环
func (a *Aggregator) Start(ctx context.Context) {
	go a.aggregateLoop(ctx)
	go a.cleanupLoop(ctx)
	log.Println("[Aggregator] 威胁事件聚合器已启动")
}

// Subscribe 获取输出通道
func (a *Aggregator) Subscribe() <-chan *UnifiedThreatEvent {
	return a.outCh
}

// IngestEBPF 接入 eBPF Monitor 事件
func (a *Aggregator) IngestEBPF(event *ebpf.ThreatEvent) {
	ip := net.IPv4(
		byte(event.SourceIP),
		byte(event.SourceIP>>8),
		byte(event.SourceIP>>16),
		byte(event.SourceIP>>24),
	).String()

	unified := &UnifiedThreatEvent{
		Timestamp:  time.Now(),
		EventType:  ThreatEventType(event.ThreatType),
		SourceIP:   ip,
		SourcePort: event.SourcePort,
		Severity:   int(event.Severity),
		Source:     SourceEBPF,
		Count:      1,
		RawData:    event,
	}
	a.ingest(unified)
}

// IngestCortex 接入 Cortex 高危指纹事件
func (a *Aggregator) IngestCortex(ip string, reason string) {
	unified := &UnifiedThreatEvent{
		Timestamp: time.Now(),
		EventType: ThreatHighRiskFingerprint,
		SourceIP:  ip,
		Severity:  7,
		Source:    SourceCortex,
		Count:     1,
		RawData:   reason,
	}
	a.ingest(unified)
}

// IngestEvaluator 接入 Evaluator 异常检测事件
func (a *Aggregator) IngestEvaluator(signal evaluator.FeedbackSignal) {
	severity := int(signal.Confidence / 10)
	if severity > 10 {
		severity = 10
	}
	unified := &UnifiedThreatEvent{
		Timestamp: time.Now(),
		EventType: ThreatAnomalyDetected,
		SourceIP:  "0.0.0.0",
		Severity:  severity,
		Source:    SourceEvaluator,
		Count:     1,
		RawData:   signal,
	}
	a.ingest(unified)
}

// ingest 内部入队
func (a *Aggregator) ingest(event *UnifiedThreatEvent) {
	select {
	case a.inCh <- event:
	default:
		atomic.AddUint64(&a.dropCount, 1)
	}
}

// GetDropCount 获取丢弃计数
func (a *Aggregator) GetDropCount() uint64 {
	return atomic.LoadUint64(&a.dropCount)
}

// aggregateLoop 聚合循环
func (a *Aggregator) aggregateLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-a.inCh:
			a.mu.Lock()
			key := fmt.Sprintf("%s:%d", event.SourceIP, event.EventType)
			if entry, ok := a.dedup[key]; ok && time.Since(entry.lastSeen) < 60*time.Second {
				entry.event.Count++
				entry.lastSeen = time.Now()
				a.mu.Unlock()
				continue
			}
			a.dedup[key] = &dedupEntry{event: event, lastSeen: time.Now()}
			a.mu.Unlock()

			select {
			case a.outCh <- event:
			default:
				atomic.AddUint64(&a.dropCount, 1)
			}
		}
	}
}

// cleanupLoop 清理过期去重条目
func (a *Aggregator) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.mu.Lock()
			now := time.Now()
			for key, entry := range a.dedup {
				if now.Sub(entry.lastSeen) > 60*time.Second {
					delete(a.dedup, key)
				}
			}
			a.mu.Unlock()
		}
	}
}
