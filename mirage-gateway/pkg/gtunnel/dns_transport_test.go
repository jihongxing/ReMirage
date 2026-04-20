package gtunnel

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: multi-path-adaptive-transport, Property 2: DNS Base32 编码往返一致性
func TestProperty_DNSBase32RoundTrip(t *testing.T) {
	domain := "t.example.com"

	rapid.Check(t, func(t *rapid.T) {
		// 生成长度不超过 DNS 分片限制的随机字节切片
		dataLen := rapid.IntRange(1, dnsFragPayloadSize).Draw(t, "dataLen")
		data := make([]byte, dataLen)
		for i := range data {
			data[i] = byte(rapid.IntRange(0, 255).Draw(t, "byte"))
		}

		seq := uint32(rapid.IntRange(1, 65535).Draw(t, "seq"))
		fragID := rapid.IntRange(0, 3).Draw(t, "fragID")
		fragTotal := rapid.IntRange(fragID+1, fragID+4).Draw(t, "fragTotal")

		// 编码
		fqdn := DNSEncodeSubdomain(data, seq, fragID, fragTotal, domain)

		// 验证 FQDN 长度不超过 253
		if len(fqdn) > 253 {
			t.Fatalf("FQDN 超长: %d > 253", len(fqdn))
		}

		// 解码
		decoded, gotFragID, gotFragTotal, gotSeq, isPoll, err := DNSDecodeSubdomain(fqdn, domain)
		if err != nil {
			t.Fatalf("解码失败: %v (fqdn=%s)", err, fqdn)
		}

		if isPoll {
			t.Fatal("数据包不应被识别为 poll")
		}

		if gotSeq != seq {
			t.Fatalf("seq 不一致: got %d, want %d", gotSeq, seq)
		}
		if gotFragID != fragID {
			t.Fatalf("fragID 不一致: got %d, want %d", gotFragID, fragID)
		}
		if gotFragTotal != fragTotal {
			t.Fatalf("fragTotal 不一致: got %d, want %d", gotFragTotal, fragTotal)
		}

		// 验证数据往返一致性
		if len(decoded) != len(data) {
			t.Fatalf("长度不一致: got %d, want %d", len(decoded), len(data))
		}
		for i := range data {
			if decoded[i] != data[i] {
				t.Fatalf("字节 %d 不一致: got 0x%02x, want 0x%02x", i, decoded[i], data[i])
			}
		}
	})
}

// TestProperty_DNSPollRoundTrip 轮询包编解码一致性
func TestProperty_DNSPollRoundTrip(t *testing.T) {
	domain := "t.example.com"

	rapid.Check(t, func(t *rapid.T) {
		seq := uint32(rapid.IntRange(1, 65535).Draw(t, "seq"))

		fqdn := DNSEncodePoll(seq, domain)

		_, _, _, gotSeq, isPoll, err := DNSDecodeSubdomain(fqdn, domain)
		if err != nil {
			t.Fatalf("解码失败: %v (fqdn=%s)", err, fqdn)
		}

		if !isPoll {
			t.Fatal("poll 包应被识别为 poll")
		}
		if gotSeq != seq {
			t.Fatalf("seq 不一致: got %d, want %d", gotSeq, seq)
		}
	})
}
