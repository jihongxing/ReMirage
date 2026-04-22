package transport

import (
	"context"
	"fmt"
	"math"
	"testing"

	orchestrator "mirage-gateway/pkg/orchestrator"

	"pgregory.net/rapid"
)

// mockLinkStateManager 用于测试的 mock
type mockLinkStateManager struct {
	links []*orchestrator.LinkState
}

func (m *mockLinkStateManager) Create(_ context.Context, _ *orchestrator.LinkState) error { return nil }
func (m *mockLinkStateManager) Get(_ context.Context, linkID string) (*orchestrator.LinkState, error) {
	for _, l := range m.links {
		if l.LinkID == linkID {
			return l, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (m *mockLinkStateManager) ListByGateway(_ context.Context, _ string) ([]*orchestrator.LinkState, error) {
	return m.links, nil
}
func (m *mockLinkStateManager) TransitionPhase(_ context.Context, _ string, _ orchestrator.LinkPhase, _ string) error {
	return nil
}
func (m *mockLinkStateManager) UpdateHealth(_ context.Context, _ string, _ float64, _ int64, _ float64, _ int64) error {
	return nil
}
func (m *mockLinkStateManager) Delete(_ context.Context, _ string) error { return nil }

// Property 9: 最优路径选择
func TestProperty9_SelectBestPathReturnsHighestScore(t *testing.T) {
	scorer := NewPathScorer()
	policy := DefaultTransportPolicies[orchestrator.SurvivalModeNormal]

	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(1, 10).Draw(t, "link_count")
		links := make([]*orchestrator.LinkState, count)

		for i := 0; i < count; i++ {
			links[i] = &orchestrator.LinkState{
				LinkID:      fmt.Sprintf("link-%d", i),
				HealthScore: rapid.Float64Range(0, 100).Draw(t, "health"),
				RttMs:       rapid.Int64Range(0, 1000).Draw(t, "rtt"),
				LossRate:    rapid.Float64Range(0, 1).Draw(t, "loss"),
				JitterMs:    rapid.Int64Range(0, 500).Draw(t, "jitter"),
				Available:   true,
			}
		}

		mockMgr := &mockLinkStateManager{links: links}
		fabric := NewTransportFabric(scorer, mockMgr, nil, policy)

		best, err := fabric.SelectBestPath(context.Background(), "session-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 计算期望最高分
		var maxScore float64
		for _, link := range links {
			score := scorer.Score(link, policy)
			if score > maxScore {
				maxScore = score
			}
		}

		if math.Abs(best.Score-maxScore) > 0.01 {
			t.Fatalf("expected best score %.4f, got %.4f", maxScore, best.Score)
		}
	})
}

// Property 14: 并行路径数量上限
func TestProperty14_MaxParallelPathsLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		modeIdx := rapid.IntRange(0, len(allSurvivalModes)-1).Draw(t, "mode")
		mode := allSurvivalModes[modeIdx]
		policy := DefaultTransportPolicies[mode]

		scorer := NewPathScorer()
		mockMgr := &mockLinkStateManager{}
		fabric := NewTransportFabric(scorer, mockMgr, nil, policy).(*transportFabric)

		addCount := rapid.IntRange(1, 10).Draw(t, "add_count")
		successCount := 0
		for i := 0; i < addCount; i++ {
			err := fabric.AddSessionPath("session-1", fmt.Sprintf("link-%d", i))
			if err == nil {
				successCount++
			}
		}

		if successCount > policy.MaxParallelPaths {
			t.Fatalf("added %d paths, exceeds max %d", successCount, policy.MaxParallelPaths)
		}

		paths, _ := fabric.GetActivePaths(context.Background(), "session-1")
		if len(paths) > policy.MaxParallelPaths {
			t.Fatalf("active paths %d exceeds max %d", len(paths), policy.MaxParallelPaths)
		}
	})
}
