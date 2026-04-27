package threat

import (
	"encoding/binary"
	"net"
	"testing"
)

// TestUDPPreFilter_ValidQUICInitial 验证合法 QUIC Initial 包通过过滤
func TestUDPPreFilter_ValidQUICInitial(t *testing.T) {
	filter := NewUDPPreFilter(nil)

	// 构造合法 QUIC v1 Initial 包
	// Long Header (0x80 set) | Initial type (bits 4-5 = 0x00) | Fixed bit (0x40)
	buf := make([]byte, 64)
	buf[0] = 0xC0                                    // 1100_0000: Long Header + Fixed + Initial type 00
	binary.BigEndian.PutUint32(buf[1:5], 0x00000001) // QUIC v1
	buf[5] = 8                                       // DCID length = 8

	addr := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 12345}
	if !filter.FilterPacket(buf, addr) {
		t.Error("合法 QUIC v1 Initial 应通过过滤")
	}
}

// TestUDPPreFilter_ValidQUICv2Initial 验证 QUIC v2 Initial 包通过过滤
func TestUDPPreFilter_ValidQUICv2Initial(t *testing.T) {
	filter := NewUDPPreFilter(nil)

	buf := make([]byte, 64)
	buf[0] = 0xC0
	binary.BigEndian.PutUint32(buf[1:5], 0x6b3343cf) // QUIC v2
	buf[5] = 8

	addr := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 12345}
	if !filter.FilterPacket(buf, addr) {
		t.Error("合法 QUIC v2 Initial 应通过过滤")
	}
}

// TestUDPPreFilter_TooShort 验证过短包被丢弃
func TestUDPPreFilter_TooShort(t *testing.T) {
	filter := NewUDPPreFilter(nil)
	addr := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 12345}

	if filter.FilterPacket([]byte{0xC0, 0x00}, addr) {
		t.Error("过短包应被丢弃")
	}
}

// TestUDPPreFilter_ShortHeader 验证 Short Header 包被丢弃
func TestUDPPreFilter_ShortHeader(t *testing.T) {
	filter := NewUDPPreFilter(nil)

	buf := make([]byte, 64)
	buf[0] = 0x40 // Short Header (bit 7 = 0)
	binary.BigEndian.PutUint32(buf[1:5], 0x00000001)
	buf[5] = 8

	addr := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 12345}
	if filter.FilterPacket(buf, addr) {
		t.Error("Short Header 包应被丢弃")
	}
}

// TestUDPPreFilter_InvalidVersion 验证非法版本号被丢弃
func TestUDPPreFilter_InvalidVersion(t *testing.T) {
	filter := NewUDPPreFilter(nil)

	buf := make([]byte, 64)
	buf[0] = 0xC0
	binary.BigEndian.PutUint32(buf[1:5], 0xDEADBEEF) // 非法版本
	buf[5] = 8

	addr := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 12345}
	if filter.FilterPacket(buf, addr) {
		t.Error("非法版本号应被丢弃")
	}
}

// TestUDPPreFilter_DCIDTooLong 验证 DCID 超长被丢弃
func TestUDPPreFilter_DCIDTooLong(t *testing.T) {
	filter := NewUDPPreFilter(nil)

	buf := make([]byte, 64)
	buf[0] = 0xC0
	binary.BigEndian.PutUint32(buf[1:5], 0x00000001)
	buf[5] = 21 // DCID length > 20

	addr := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 12345}
	if filter.FilterPacket(buf, addr) {
		t.Error("DCID 超长应被丢弃")
	}
}

// TestUDPPreFilter_NonInitialType 验证非 Initial 类型被丢弃
func TestUDPPreFilter_NonInitialType(t *testing.T) {
	filter := NewUDPPreFilter(nil)

	// Handshake type = 0x02 → bits 4-5 = 10
	buf := make([]byte, 64)
	buf[0] = 0xE0 // 1110_0000: Long Header + Fixed + type=10 (Handshake)
	binary.BigEndian.PutUint32(buf[1:5], 0x00000001)
	buf[5] = 8

	addr := &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 12345}
	if filter.FilterPacket(buf, addr) {
		t.Error("非 Initial 类型应被丢弃")
	}
}
