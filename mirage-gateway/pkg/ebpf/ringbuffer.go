package ebpf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
)

// RingBufferReader Ring Buffer 读取器
type RingBufferReader struct {
	reader  *ringbuf.Reader
	handler ThreatEventHandler
	stopCh  chan struct{}
}

// ThreatEventHandler 威胁事件处理器
type ThreatEventHandler func(*ThreatEvent) error

// NewRingBufferReader 创建 Ring Buffer 读取器
func NewRingBufferReader(eventsMap *ebpf.Map, handler ThreatEventHandler) (*RingBufferReader, error) {
	reader, err := ringbuf.NewReader(eventsMap)
	if err != nil {
		return nil, fmt.Errorf("创建 Ring Buffer 读取器失败: %w", err)
	}

	return &RingBufferReader{
		reader:  reader,
		handler: handler,
		stopCh:  make(chan struct{}),
	}, nil
}

// Start 启动监听
func (r *RingBufferReader) Start() {
	go r.readLoop()
}

// Stop 停止监听
func (r *RingBufferReader) Stop() error {
	close(r.stopCh)
	return r.reader.Close()
}

// readLoop 读取循环
func (r *RingBufferReader) readLoop() {
	log.Println("[Ring Buffer] 开始监听威胁事件...")

	for {
		record, err := r.reader.Read()
		if err != nil {
			if err == ringbuf.ErrClosed {
				log.Println("[Ring Buffer] 停止监听")
				return
			}
			log.Printf("[Ring Buffer] 读取错误: %v\n", err)
			continue
		}

		event, err := r.parseEvent(record.RawSample)
		if err != nil {
			log.Printf("[Ring Buffer] 解析事件失败: %v\n", err)
			continue
		}

		if err := r.handler(event); err != nil {
			log.Printf("[Ring Buffer] 处理事件失败: %v\n", err)
		}
	}
}

// parseEvent 解析事件
func (r *RingBufferReader) parseEvent(data []byte) (*ThreatEvent, error) {
	if len(data) < 28 { // sizeof(threat_event) = 28 字节
		return nil, fmt.Errorf("数据长度不足: %d", len(data))
	}

	event := &ThreatEvent{}
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &event.Timestamp); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &event.ThreatType); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &event.SourceIP); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &event.SourcePort); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &event.DestPort); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &event.PacketCount); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &event.Severity); err != nil {
		return nil, err
	}

	return event, nil
}
