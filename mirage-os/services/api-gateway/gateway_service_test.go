package main

import (
	"testing"

	"pgregory.net/rapid"
)

// ThreatAction 威胁动作类型
type ThreatAction int

const (
	ThreatActionNone              ThreatAction = 0
	ThreatActionIncreaseDefense   ThreatAction = 1
	ThreatActionBlockIP           ThreatAction = 2
	ThreatActionSwitchCell        ThreatAction = 3
	ThreatActionEmergencyShutdown ThreatAction = 4
)

// CalculateQuotaDeduction 计算配额扣减与告警（纯函数）
func CalculateQuotaDeduction(remainingQuota, totalQuota, trafficBytes int64) (newRemaining int64, warning bool) {
	newRemaining = remainingQuota - trafficBytes
	if newRemaining < 0 {
		newRemaining = 0
	}
	if totalQuota > 0 {
		warning = newRemaining < totalQuota/10
	}
	return
}

// MapSeverityToAction 威胁严重程度到动作的映射（纯函数）
func MapSeverityToAction(severity uint32) ThreatAction {
	switch {
	case severity >= 8:
		return ThreatActionEmergencyShutdown
	case severity >= 6:
		return ThreatActionSwitchCell
	case severity >= 4:
		return ThreatActionBlockIP
	case severity >= 2:
		return ThreatActionIncreaseDefense
	default:
		return ThreatActionNone
	}
}

// Feature: mirage-os-completion, Property 6: 流量配额扣减与告警
// **Validates: Requirements 4.2**
func TestProperty_QuotaDeductionAndWarning(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		totalQuota := rapid.Int64Range(1, 1_000_000_000_000).Draw(t, "total_quota")
		remainingQuota := rapid.Int64Range(0, totalQuota).Draw(t, "remaining_quota")
		trafficBytes := rapid.Int64Range(0, totalQuota*2).Draw(t, "traffic_bytes")

		newRemaining, warning := CalculateQuotaDeduction(remainingQuota, totalQuota, trafficBytes)

		// 验证: 剩余 = max(Q-T, 0)
		expected := remainingQuota - trafficBytes
		if expected < 0 {
			expected = 0
		}
		if newRemaining != expected {
			t.Fatalf("remaining=%d, expected=%d (Q=%d, T=%d)", newRemaining, expected, remainingQuota, trafficBytes)
		}

		// 验证: warning 在 <10% 时为 true
		threshold := totalQuota / 10
		expectedWarning := newRemaining < threshold
		if warning != expectedWarning {
			t.Fatalf("warning=%v, expected=%v (remaining=%d, threshold=%d)", warning, expectedWarning, newRemaining, threshold)
		}
	})
}

// Feature: mirage-os-completion, Property 7: 威胁严重程度到动作的映射
// **Validates: Requirements 4.3**
func TestProperty_SeverityToActionMapping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		severity := rapid.Uint32Range(0, 10).Draw(t, "severity")

		action := MapSeverityToAction(severity)

		var expected ThreatAction
		switch {
		case severity >= 8:
			expected = ThreatActionEmergencyShutdown
		case severity >= 6:
			expected = ThreatActionSwitchCell
		case severity >= 4:
			expected = ThreatActionBlockIP
		case severity >= 2:
			expected = ThreatActionIncreaseDefense
		default:
			expected = ThreatActionNone
		}

		if action != expected {
			t.Fatalf("severity=%d: action=%d, expected=%d", severity, action, expected)
		}
	})
}
