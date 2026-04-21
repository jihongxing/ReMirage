package cortex

import (
	"testing"

	"mirage-gateway/pkg/threat"
)

// ============================================================
// 安全回归测试 — Gateway 侧联动链路
// ============================================================

// --- 回归 5: 联动链路（RiskScorer → 自动封禁） ---

func TestSecurityRegression_Linkage_HoneypotAndFingerprintAutoBan(t *testing.T) {
	bm := threat.NewBlacklistManager(nil, 65536)
	scorer := NewRiskScorer(bm)

	ip := "203.0.113.50"

	// 蜜罐命中 +30
	scorer.AddScore(ip, HoneypotHitScore, "honeypot")
	if scorer.GetScore(ip) != 30 {
		t.Fatalf("蜜罐命中后评分应为 30，实际: %d", scorer.GetScore(ip))
	}

	// 指纹命中 +40 → 总分 70 >= AutoBanThreshold
	scorer.AddScore(ip, DangerousFPScore, "fingerprint")
	if scorer.GetScore(ip) != 70 {
		t.Fatalf("指纹命中后评分应为 70，实际: %d", scorer.GetScore(ip))
	}

	// 验证自动封禁已生效
	entry := bm.Get(ip + "/32")
	if entry == nil {
		t.Fatal("评分达到阈值后应自动封禁到黑名单")
	}
}

func TestSecurityRegression_Linkage_BelowThresholdNoBan(t *testing.T) {
	bm := threat.NewBlacklistManager(nil, 65536)
	scorer := NewRiskScorer(bm)

	ip := "203.0.113.51"

	// 仅蜜罐命中 +30 → 低于阈值 70
	scorer.AddScore(ip, HoneypotHitScore, "honeypot")

	entry := bm.Get(ip + "/32")
	if entry != nil {
		t.Fatal("评分未达阈值时不应封禁")
	}
}
