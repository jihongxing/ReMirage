package transport

import "context"

// TransportFabricIface 传输织网层接口
type TransportFabricIface interface {
	SelectBestPath(ctx context.Context, sessionID string) (*PathScore, error)
	SwitchPath(ctx context.Context, sessionID string, newLinkID string) error
	GetPathScores(ctx context.Context) ([]*PathScore, error)
	ApplyPolicy(policy *TransportPolicy)
	PrewarmBackup(ctx context.Context, sessionID string) error
	GetActivePaths(ctx context.Context, sessionID string) ([]string, error)
}
