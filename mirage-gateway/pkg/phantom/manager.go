// Package phantom 实现影子欺骗控制面
// 管理蜜罐重定向、威胁诱导、金丝雀追踪
package phantom

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
)

// PhantomEvent 欺骗事件
type PhantomEvent struct {
	Timestamp  uint64
	SrcIP      uint32
	DstIP      uint32
	SrcPort    uint16
	DstPort    uint16
	HoneypotIP uint32
	EventType  uint8 // 0=redirect, 1=trap_hit
}

// TrapRecord 陷阱记录
type TrapRecord struct {
	SrcIP        string
	FirstSeen    time.Time
	LastSeen     time.Time
	RequestCount uint64
	Trapped      bool
	HoneypotID   int
}

// Manager 影子欺骗管理器
type Manager struct {
	mu sync.RWMutex

	// eBPF Maps
	phishingListMap *ebpf.Map
	honeypotConfig  *ebpf.Map
	phantomStats    *ebpf.Map
	phantomEvents   *ebpf.Map

	// 蜜罐配置
	honeypotIP net.IP

	// 陷阱记录
	trapRecords map[string]*TrapRecord

	// 事件通道
	eventChan chan *PhantomEvent
	stopChan  chan struct{}

	// 回调
	onRedirect func(event *PhantomEvent)
}

// NewManager 创建影子欺骗管理器
func NewManager() *Manager {
	return &Manager{
		trapRecords: make(map[string]*TrapRecord),
		eventChan:   make(chan *PhantomEvent, 1000),
		stopChan:    make(chan struct{}),
	}
}

// SetMaps 设置 eBPF Maps
func (m *Manager) SetMaps(maps map[string]*ebpf.Map) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var ok bool
	if m.phishingListMap, ok = maps["phishing_list_map"]; !ok {
		return fmt.Errorf("phishing_list_map not found")
	}
	if m.honeypotConfig, ok = maps["honeypot_config"]; !ok {
		return fmt.Errorf("honeypot_config not found")
	}
	if m.phantomStats, ok = maps["phantom_stats"]; !ok {
		return fmt.Errorf("phantom_stats not found")
	}
	if m.phantomEvents, ok = maps["phantom_events"]; !ok {
		return fmt.Errorf("phantom_events not found")
	}

	return nil
}

// SetHoneypotIP 配置蜜罐 IP
func (m *Manager) SetHoneypotIP(ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return fmt.Errorf("invalid IP: %s", ip)
	}

	m.honeypotIP = parsed.To4()
	if m.honeypotIP == nil {
		return fmt.Errorf("IPv6 not supported")
	}

	// 写入 eBPF Map
	if m.honeypotConfig != nil {
		key := uint32(0)
		value := binary.BigEndian.Uint32(m.honeypotIP)
		if err := m.honeypotConfig.Put(key, value); err != nil {
			return fmt.Errorf("failed to set honeypot config: %w", err)
		}
	}

	return nil
}

// AddToPhishingList 添加 IP 到钓鱼名单
func (m *Manager) AddToPhishingList(ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return fmt.Errorf("invalid IP: %s", ip)
	}

	ipv4 := parsed.To4()
	if ipv4 == nil {
		return fmt.Errorf("IPv6 not supported")
	}

	key := binary.BigEndian.Uint32(ipv4)
	value := uint64(time.Now().UnixNano())

	if m.phishingListMap != nil {
		if err := m.phishingListMap.Put(key, value); err != nil {
			return fmt.Errorf("failed to add to phishing list: %w", err)
		}
	}

	// 记录陷阱
	m.trapRecords[ip] = &TrapRecord{
		SrcIP:     ip,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
		Trapped:   true,
	}

	return nil
}

// RemoveFromPhishingList 从钓鱼名单移除
func (m *Manager) RemoveFromPhishingList(ip string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	parsed := net.ParseIP(ip)
	if parsed == nil {
		return fmt.Errorf("invalid IP: %s", ip)
	}

	ipv4 := parsed.To4()
	if ipv4 == nil {
		return fmt.Errorf("IPv6 not supported")
	}

	key := binary.BigEndian.Uint32(ipv4)

	if m.phishingListMap != nil {
		if err := m.phishingListMap.Delete(key); err != nil {
			return fmt.Errorf("failed to remove from phishing list: %w", err)
		}
	}

	delete(m.trapRecords, ip)
	return nil
}

// StartEventMonitor 启动事件监控
func (m *Manager) StartEventMonitor() error {
	if m.phantomEvents == nil {
		return fmt.Errorf("phantom_events map not set")
	}

	reader, err := ringbuf.NewReader(m.phantomEvents)
	if err != nil {
		return fmt.Errorf("failed to create ringbuf reader: %w", err)
	}

	go func() {
		defer reader.Close()
		for {
			select {
			case <-m.stopChan:
				return
			default:
				record, err := reader.Read()
				if err != nil {
					continue
				}

				var event PhantomEvent
				if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &event); err != nil {
					continue
				}

				m.handleEvent(&event)
			}
		}
	}()

	return nil
}

// handleEvent 处理欺骗事件
func (m *Manager) handleEvent(event *PhantomEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	srcIP := uint32ToIP(event.SrcIP)

	// 更新陷阱记录
	if record, ok := m.trapRecords[srcIP]; ok {
		record.LastSeen = time.Now()
		record.RequestCount++
	}

	// 发送到通道
	select {
	case m.eventChan <- event:
	default:
		// 通道满，丢弃
	}

	// 回调
	if m.onRedirect != nil {
		m.onRedirect(event)
	}
}

// GetTrapRecords 获取所有陷阱记录
func (m *Manager) GetTrapRecords() []*TrapRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	records := make([]*TrapRecord, 0, len(m.trapRecords))
	for _, r := range m.trapRecords {
		records = append(records, r)
	}
	return records
}

// GetStats 获取统计信息
func (m *Manager) GetStats() (redirected, passed, trapped, errors uint64) {
	if m.phantomStats == nil {
		return
	}

	var val uint64
	keys := []uint32{0, 1, 2, 3}
	vals := []*uint64{&redirected, &passed, &trapped, &errors}

	for i, key := range keys {
		if err := m.phantomStats.Lookup(key, &val); err == nil {
			*vals[i] = val
		}
	}
	return
}

// OnRedirect 设置重定向回调
func (m *Manager) OnRedirect(fn func(event *PhantomEvent)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRedirect = fn
}

// EventChan 获取事件通道
func (m *Manager) EventChan() <-chan *PhantomEvent {
	return m.eventChan
}

// Stop 停止管理器
func (m *Manager) Stop() {
	close(m.stopChan)
}

func uint32ToIP(n uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}
