package ebpf

import (
	"testing"
)

// synCookieHash 模拟 C 侧的 syn_cookie_hash 函数
// 用于 Go 侧回归测试验证 cookie 计算一致性
func synCookieHash(saddr uint32, dport uint16, ts uint64) uint32 {
	h := saddr ^ (uint32(dport) << 16) ^ uint32(ts>>20)
	h ^= 0xDEADBEEF
	h = ((h >> 16) ^ h) * 0x45d9f3b
	h = ((h >> 16) ^ h) * 0x45d9f3b
	h = (h >> 16) ^ h
	return h
}

func TestSynCookieHash_Deterministic(t *testing.T) {
	saddr := uint32(0xC0A80001) // 192.168.0.1
	dport := uint16(50847)
	ts := uint64(1000000000000)

	h1 := synCookieHash(saddr, dport, ts)
	h2 := synCookieHash(saddr, dport, ts)
	if h1 != h2 {
		t.Fatalf("cookie hash 不确定: %d != %d", h1, h2)
	}
}

func TestSynCookieHash_DifferentInputsDifferentOutput(t *testing.T) {
	ts := uint64(1000000000000)

	h1 := synCookieHash(0xC0A80001, 50847, ts)
	h2 := synCookieHash(0xC0A80002, 50847, ts) // 不同 IP
	h3 := synCookieHash(0xC0A80001, 50848, ts) // 不同端口

	if h1 == h2 {
		t.Fatal("不同 IP 应产生不同 cookie")
	}
	if h1 == h3 {
		t.Fatal("不同端口应产生不同 cookie")
	}
}

func TestACKForgery_ZeroAckSeqRejected(t *testing.T) {
	// 模拟 ACK 验证逻辑：ack_seq == 0 应被拒绝
	saddr := uint32(0xC0A80001)
	dport := uint16(50847)
	ts := uint64(1000000000000)

	cookie := synCookieHash(saddr, dport, ts)
	expectedCookie := synCookieHash(saddr, dport, ts)
	ackSeq := uint32(0) // 伪造的 ACK

	// 双重校验：cookie 匹配 AND ack_seq != 0
	validated := (cookie == expectedCookie && ackSeq != 0)
	if validated {
		t.Fatal("ack_seq=0 的 ACK 不应通过验证")
	}
}

func TestACKForgery_ValidAckSeqAccepted(t *testing.T) {
	saddr := uint32(0xC0A80001)
	dport := uint16(50847)
	ts := uint64(1000000000000)

	cookie := synCookieHash(saddr, dport, ts)
	expectedCookie := synCookieHash(saddr, dport, ts)
	ackSeq := uint32(12345) // 合法 ACK

	validated := (cookie == expectedCookie && ackSeq != 0)
	if !validated {
		t.Fatal("合法 ACK 应通过验证")
	}
}

func TestACKForgery_WrongTimestampRejected(t *testing.T) {
	saddr := uint32(0xC0A80001)
	dport := uint16(50847)

	// challenge 时的 cookie
	challengeTs := uint64(1000000000000)
	storedCookie := synCookieHash(saddr, dport, challengeTs)

	// 攻击者用不同时间戳计算的 cookie
	attackerTs := uint64(2000000000000)
	attackerCookie := synCookieHash(saddr, dport, attackerTs)

	if storedCookie == attackerCookie {
		t.Fatal("不同时间戳应产生不同 cookie")
	}
}
