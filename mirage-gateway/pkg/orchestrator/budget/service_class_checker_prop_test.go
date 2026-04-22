package budget

import (
	"context"
	"testing"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"

	"pgregory.net/rapid"
)

// mockSessionGetter 测试用 SessionGetter 实现
type mockSessionGetter struct {
	serviceClass orchestrator.ServiceClass
}

func (m *mockSessionGetter) Get(_ context.Context, _ string) (*orchestrator.SessionState, error) {
	return &orchestrator.SessionState{
		SessionID:    "test-session",
		ServiceClass: m.serviceClass,
	}, nil
}

// allServiceClasses 所有合法 ServiceClass
var allServiceClasses = []orchestrator.ServiceClass{
	orchestrator.ServiceClassStandard,
	orchestrator.ServiceClassPlatinum,
	orchestrator.ServiceClassDiamond,
}

// allSurvivalModes 所有合法 SurvivalMode
var allSurvivalModes = []orchestrator.SurvivalMode{
	orchestrator.SurvivalModeNormal,
	orchestrator.SurvivalModeLowNoise,
	orchestrator.SurvivalModeHardened,
	orchestrator.SurvivalModeDegraded,
	orchestrator.SurvivalModeEscape,
	orchestrator.SurvivalModeLastResort,
}

// Validates: Requirements 6.3, 6.4, 6.5, 6.6, 6.7
// Feature: v2-budget-engine, Property 5: ServiceClassChecker 权限校验正确性
func TestProperty5_ServiceClassCheckerPermission(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机 ServiceClass
		scIdx := rapid.IntRange(0, len(allServiceClasses)-1).Draw(t, "serviceClassIdx")
		serviceClass := allServiceClasses[scIdx]

		// 生成随机 SurvivalMode
		smIdx := rapid.IntRange(0, len(allSurvivalModes)-1).Draw(t, "survivalModeIdx")
		survivalMode := allSurvivalModes[smIdx]

		// 生成随机 TxType
		txTypeIdx := rapid.IntRange(0, len(commit.AllTxTypes)-1).Draw(t, "txTypeIdx")
		txType := commit.AllTxTypes[txTypeIdx]

		// 构建 checker
		slaPolicy := NewDefaultSLAPolicy()
		sessionGetter := &mockSessionGetter{serviceClass: serviceClass}
		checker := NewServiceClassCheckerImpl(slaPolicy, sessionGetter)

		// 构建事务
		tx := &commit.CommitTransaction{
			TxID:               "test-tx",
			TxType:             txType,
			TargetSessionID:    "test-session",
			TargetSurvivalMode: string(survivalMode),
		}

		ctx := context.Background()
		err := checker.Check(ctx, tx)
		policy := slaPolicy.GetPolicy(serviceClass)

		// 验证：非 SurvivalModeSwitch 事务始终通过
		if txType != commit.TxTypeSurvivalModeSwitch {
			if err != nil {
				t.Fatalf("non-SurvivalModeSwitch tx (type=%s) should always pass, got error: %v", txType, err)
			}
			return
		}

		// 验证：SurvivalModeSwitch 事务结果与 SLAPolicy 一致
		var expectedAllowed bool
		switch survivalMode {
		case orchestrator.SurvivalModeHardened:
			expectedAllowed = policy.HardenedAllowed
		case orchestrator.SurvivalModeEscape:
			expectedAllowed = policy.EscapeAllowed
		case orchestrator.SurvivalModeLastResort:
			expectedAllowed = policy.LastResortAllowed
		default:
			expectedAllowed = true
		}

		if expectedAllowed {
			if err != nil {
				t.Fatalf("expected allowed for ServiceClass=%s, SurvivalMode=%s, got error: %v",
					serviceClass, survivalMode, err)
			}
		} else {
			if err == nil {
				t.Fatalf("expected denied for ServiceClass=%s, SurvivalMode=%s, got nil",
					serviceClass, survivalMode)
			}
			scErr, ok := err.(*ErrServiceClassDenied)
			if !ok {
				t.Fatalf("expected *ErrServiceClassDenied, got %T", err)
			}
			if scErr.ServiceClass != string(serviceClass) {
				t.Fatalf("expected ServiceClass=%s, got %s", serviceClass, scErr.ServiceClass)
			}
			if scErr.DeniedMode != string(survivalMode) {
				t.Fatalf("expected DeniedMode=%s, got %s", survivalMode, scErr.DeniedMode)
			}
		}
	})
}
