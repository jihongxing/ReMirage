package transport

import (
	"context"
	"fmt"
	"sync"

	orchestrator "mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
)

type transportFabric struct {
	mu           sync.RWMutex
	scorer       PathScorerIface
	linkMgr      orchestrator.LinkStateManager
	commitEngine commit.CommitEngine
	policy       *TransportPolicy
	// sessionPaths 记录每个 session 的活跃路径
	sessionPaths map[string][]string
}

// NewTransportFabric 创建 TransportFabric
func NewTransportFabric(
	scorer PathScorerIface,
	linkMgr orchestrator.LinkStateManager,
	commitEngine commit.CommitEngine,
	policy *TransportPolicy,
) TransportFabricIface {
	return &transportFabric{
		scorer:       scorer,
		linkMgr:      linkMgr,
		commitEngine: commitEngine,
		policy:       policy,
		sessionPaths: make(map[string][]string),
	}
}

func (f *transportFabric) SelectBestPath(ctx context.Context, _ string) (*PathScore, error) {
	f.mu.RLock()
	policy := f.policy
	f.mu.RUnlock()

	links, err := f.linkMgr.ListByGateway(ctx, "default")
	if err != nil {
		return nil, err
	}

	var best *PathScore
	for _, link := range links {
		if !link.Available {
			continue
		}
		score := f.scorer.Score(link, policy)
		if best == nil || score > best.Score {
			best = &PathScore{LinkID: link.LinkID, Score: score}
		}
	}

	if best == nil {
		return nil, fmt.Errorf("no available path")
	}
	return best, nil
}

func (f *transportFabric) SwitchPath(ctx context.Context, sessionID string, newLinkID string) error {
	tx, err := f.commitEngine.BeginTransaction(ctx, &commit.BeginTxRequest{
		TxType:          commit.TxTypeLinkMigration,
		TargetSessionID: sessionID,
		TargetLinkID:    newLinkID,
	})
	if err != nil {
		return err
	}
	return f.commitEngine.ExecuteTransaction(ctx, tx.TxID)
}

func (f *transportFabric) GetPathScores(ctx context.Context) ([]*PathScore, error) {
	f.mu.RLock()
	policy := f.policy
	f.mu.RUnlock()

	links, err := f.linkMgr.ListByGateway(ctx, "default")
	if err != nil {
		return nil, err
	}

	scores := make([]*PathScore, 0, len(links))
	for _, link := range links {
		if !link.Available {
			continue
		}
		score := f.scorer.Score(link, policy)
		scores = append(scores, &PathScore{LinkID: link.LinkID, Score: score})
	}
	return scores, nil
}

func (f *transportFabric) ApplyPolicy(policy *TransportPolicy) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.policy = policy
}

func (f *transportFabric) PrewarmBackup(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	paths := f.sessionPaths[sessionID]
	if len(paths) >= f.policy.MaxParallelPaths {
		return fmt.Errorf("max parallel paths (%d) reached for session %s", f.policy.MaxParallelPaths, sessionID)
	}
	// 预热逻辑：标记备用路径
	return nil
}

func (f *transportFabric) GetActivePaths(_ context.Context, sessionID string) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	paths := f.sessionPaths[sessionID]
	maxPaths := f.policy.MaxParallelPaths
	if len(paths) > maxPaths {
		return paths[:maxPaths], nil
	}
	return paths, nil
}

// AddSessionPath 添加 session 路径（内部使用）
func (f *transportFabric) AddSessionPath(sessionID string, linkID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	paths := f.sessionPaths[sessionID]
	if len(paths) >= f.policy.MaxParallelPaths {
		return fmt.Errorf("max parallel paths (%d) reached", f.policy.MaxParallelPaths)
	}
	f.sessionPaths[sessionID] = append(paths, linkID)
	return nil
}
