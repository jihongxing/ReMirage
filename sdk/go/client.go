package mirage

import (
	"context"
	"crypto/tls"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	conn    *grpc.ClientConn
	token   string
	Gateway *GatewayService
	Billing *BillingService
	Cell    *CellService
}

type Option func(*clientConfig)

type clientConfig struct {
	token   string
	tls     bool
	timeout time.Duration
}

func WithToken(token string) Option {
	return func(c *clientConfig) { c.token = token }
}

func WithTLS(enabled bool) Option {
	return func(c *clientConfig) { c.tls = enabled }
}

func WithTimeout(d time.Duration) Option {
	return func(c *clientConfig) { c.timeout = d }
}

func NewClient(endpoint string, opts ...Option) (*Client, error) {
	cfg := &clientConfig{tls: true, timeout: 30 * time.Second}
	for _, opt := range opts {
		opt(cfg)
	}

	var dialOpts []grpc.DialOption
	if cfg.tls {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(
			credentials.NewTLS(&tls.Config{}),
		))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	conn, err := grpc.Dial(endpoint, dialOpts...)
	if err != nil {
		return nil, err
	}

	c := &Client{conn: conn, token: cfg.token}
	c.Gateway = &GatewayService{client: c}
	c.Billing = &BillingService{client: c}
	c.Cell = &CellService{client: c}
	return c, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) withAuth(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.token)
}

// === Gateway Service ===

type GatewayService struct{ client *Client }

type HeartbeatRequest struct {
	GatewayID   string
	Version     string
	ThreatLevel uint32
}

type HeartbeatResponse struct {
	Success               bool
	Message               string
	RemainingQuota        uint64
	DefenseLevel          uint32
	NextHeartbeatInterval int64
}

func (s *GatewayService) SyncHeartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	ctx = s.client.withAuth(ctx)
	// 实际实现需要 protobuf 生成的代码
	return &HeartbeatResponse{
		Success:               true,
		RemainingQuota:        1073741824,
		NextHeartbeatInterval: 30,
	}, nil
}

type TrafficReport struct {
	GatewayID           string
	BaseTrafficBytes    uint64
	DefenseTrafficBytes uint64
	CellLevel           string
}

type TrafficResponse struct {
	Success        bool
	RemainingQuota uint64
	CurrentCostUSD float32
	QuotaWarning   bool
}

func (s *GatewayService) ReportTraffic(ctx context.Context, req *TrafficReport) (*TrafficResponse, error) {
	ctx = s.client.withAuth(ctx)
	return &TrafficResponse{Success: true, RemainingQuota: 1073741824}, nil
}

type ThreatReport struct {
	GatewayID  string
	ThreatType string
	SourceIP   string
	Severity   uint32
}

type ThreatResponse struct {
	Success         bool
	Action          string
	NewDefenseLevel uint32
}

func (s *GatewayService) ReportThreat(ctx context.Context, req *ThreatReport) (*ThreatResponse, error) {
	ctx = s.client.withAuth(ctx)
	return &ThreatResponse{Success: true, Action: "INCREASE_DEFENSE", NewDefenseLevel: 2}, nil
}

type QuotaResponse struct {
	Success        bool
	RemainingBytes uint64
	TotalBytes     uint64
	ExpiresAt      int64
}

func (s *GatewayService) GetQuota(ctx context.Context, gatewayID, userID string) (*QuotaResponse, error) {
	ctx = s.client.withAuth(ctx)
	return &QuotaResponse{Success: true, RemainingBytes: 1073741824, TotalBytes: 10737418240}, nil
}

// === Billing Service ===

type BillingService struct{ client *Client }

type CreateAccountResponse struct {
	Success   bool
	AccountID string
	CreatedAt int64
}

func (s *BillingService) CreateAccount(ctx context.Context, userID, publicKey string) (*CreateAccountResponse, error) {
	ctx = s.client.withAuth(ctx)
	return &CreateAccountResponse{Success: true, AccountID: "acc-" + userID[:8]}, nil
}

type BalanceResponse struct {
	Success        bool
	BalanceUSD     uint64
	TotalBytes     uint64
	UsedBytes      uint64
	RemainingBytes uint64
}

func (s *BillingService) GetBalance(ctx context.Context, accountID string) (*BalanceResponse, error) {
	ctx = s.client.withAuth(ctx)
	return &BalanceResponse{Success: true, BalanceUSD: 10000, RemainingBytes: 9663676416}, nil
}

type PurchaseRequest struct {
	AccountID   string
	PackageType string
	CellLevel   string
	Quantity    uint32
}

type PurchaseResponse struct {
	Success          bool
	CostUSD          uint64
	RemainingBalance uint64
	QuotaAdded       uint64
}

func (s *BillingService) PurchaseQuota(ctx context.Context, req *PurchaseRequest) (*PurchaseResponse, error) {
	ctx = s.client.withAuth(ctx)
	return &PurchaseResponse{Success: true, CostUSD: 1000, QuotaAdded: 10737418240}, nil
}

// === Cell Service ===

type CellService struct{ client *Client }

type ListCellsRequest struct {
	Level      string
	Country    string
	OnlineOnly bool
}

type CellInfo struct {
	CellID       string
	CellName     string
	Level        string
	Country      string
	Region       string
	LoadPercent  float32
	GatewayCount uint32
	MaxGateways  uint32
}

type ListCellsResponse struct {
	Success bool
	Cells   []*CellInfo
}

func (s *CellService) ListCells(ctx context.Context, req *ListCellsRequest) (*ListCellsResponse, error) {
	ctx = s.client.withAuth(ctx)
	return &ListCellsResponse{Success: true, Cells: []*CellInfo{}}, nil
}

type AllocateRequest struct {
	UserID           string
	GatewayID        string
	PreferredLevel   string
	PreferredCountry string
}

type AllocateResponse struct {
	Success         bool
	CellID          string
	ConnectionToken string
}

func (s *CellService) AllocateGateway(ctx context.Context, req *AllocateRequest) (*AllocateResponse, error) {
	ctx = s.client.withAuth(ctx)
	return &AllocateResponse{Success: true, CellID: "cell-001", ConnectionToken: "token_xxx"}, nil
}

type SwitchCellRequest struct {
	UserID        string
	GatewayID     string
	CurrentCellID string
	TargetCellID  string
	Reason        string
}

type SwitchCellResponse struct {
	Success         bool
	NewCellID       string
	ConnectionToken string
}

func (s *CellService) SwitchCell(ctx context.Context, req *SwitchCellRequest) (*SwitchCellResponse, error) {
	ctx = s.client.withAuth(ctx)
	return &SwitchCellResponse{Success: true, NewCellID: "cell-002", ConnectionToken: "token_yyy"}, nil
}
