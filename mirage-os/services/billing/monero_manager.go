// Package billing - Monero 匿名充值管理
package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mirage-os/pkg/models"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const (
	// MinConfirmations 最小确认数
	MinConfirmations = 10

	// DefaultMoneroWalletRPCEndpoint 钱包 RPC 地址（仅限 localhost）
	DefaultMoneroWalletRPCEndpoint = "http://127.0.0.1:18082/json_rpc"

	// PollInterval 轮询间隔
	PollInterval = 30 * time.Second
)

// TransferResult Monero 交易查询结果
type TransferResult struct {
	Confirmations int    `json:"confirmations"`
	Amount        uint64 `json:"amount"`
	Address       string `json:"address"`
	TxHash        string `json:"tx_hash"`
}

// MoneroRPCClient Monero JSON-RPC 客户端接口
type MoneroRPCClient interface {
	GetTransferByTxID(ctx context.Context, txHash string) (*TransferResult, error)
	CreateAddress(ctx context.Context, accountIndex uint32) (string, error)
}

// ExchangeRateProvider 汇率提供者接口
type ExchangeRateProvider interface {
	GetXMRUSDRate(ctx context.Context) (float64, error)
}

// HTTPMoneroRPCClient 基于 net/http 的 Monero JSON-RPC 客户端
type HTTPMoneroRPCClient struct {
	endpoint   string
	httpClient *http.Client
}

// NewHTTPMoneroRPCClient 创建 HTTP Monero RPC 客户端
func NewHTTPMoneroRPCClient(endpoint string) *HTTPMoneroRPCClient {
	if endpoint == "" {
		endpoint = DefaultMoneroWalletRPCEndpoint
	}
	return &HTTPMoneroRPCClient{
		endpoint:   endpoint,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// jsonRPCRequest JSON-RPC 2.0 请求
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse JSON-RPC 2.0 响应
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// GetTransferByTxID 通过交易哈希查询转账信息
func (c *HTTPMoneroRPCClient) GetTransferByTxID(ctx context.Context, txHash string) (*TransferResult, error) {
	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      "0",
		Method:  "get_transfer_by_txid",
		Params:  map[string]interface{}{"txid": txHash},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("monero rpc call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return ParseGetTransferResponse(respBody)
}

// ParseGetTransferResponse 解析 get_transfer_by_txid 响应
func ParseGetTransferResponse(data []byte) (*TransferResult, error) {
	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(data, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	var result struct {
		Transfer struct {
			Confirmations int    `json:"confirmations"`
			Amount        uint64 `json:"amount"`
			Address       string `json:"address"`
			TxID          string `json:"txid"`
		} `json:"transfer"`
	}
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal transfer: %w", err)
	}

	return &TransferResult{
		Confirmations: result.Transfer.Confirmations,
		Amount:        result.Transfer.Amount,
		Address:       result.Transfer.Address,
		TxHash:        result.Transfer.TxID,
	}, nil
}

// CreateAddress 创建子地址
func (c *HTTPMoneroRPCClient) CreateAddress(ctx context.Context, accountIndex uint32) (string, error) {
	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      "0",
		Method:  "create_address",
		Params:  map[string]interface{}{"account_index": accountIndex},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("monero rpc call failed: %w", err)
	}
	defer resp.Body.Close()

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return "", fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	var result struct {
		Address      string `json:"address"`
		AddressIndex uint32 `json:"address_index"`
	}
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		return "", fmt.Errorf("unmarshal address: %w", err)
	}

	return result.Address, nil
}

// MoneroManager Monero 充值管理器
type MoneroManager struct {
	DB               *gorm.DB
	Redis            *redis.Client
	RPCClient        MoneroRPCClient
	ExchangeProvider ExchangeRateProvider
	QuotaBridge      *QuotaBridge // 充值确认后同步 Redis
}

// NewMoneroManager 创建 Monero 管理器
func NewMoneroManager(db *gorm.DB, rdb *redis.Client, rpcClient MoneroRPCClient, exchangeProvider ExchangeRateProvider) *MoneroManager {
	return &MoneroManager{
		DB:               db,
		Redis:            rdb,
		RPCClient:        rpcClient,
		ExchangeProvider: exchangeProvider,
	}
}

// GenerateDepositAddress 生成充值地址（订单级子地址模型）
// 每次充值请求生成新子地址并关联到 Deposit 记录
func (mm *MoneroManager) GenerateDepositAddress(ctx context.Context, userID string) (string, error) {
	var user models.User
	if err := mm.DB.Where("user_id = ?", userID).First(&user).Error; err != nil {
		return "", fmt.Errorf("用户不存在: %w", err)
	}

	// 每次请求生成独立子地址
	address, err := mm.RPCClient.CreateAddress(ctx, 0)
	if err != nil {
		return "", fmt.Errorf("生成子地址失败: %w", err)
	}

	log.Printf("💰 [Monero] 为用户 %s 生成订单级充值子地址: %s", userID, address)
	return address, nil
}

// MonitorDeposits 监听充值（后台任务）
func (mm *MoneroManager) MonitorDeposits(ctx context.Context) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	log.Println("💰 [Monero] 充值监听器已启动")

	for {
		select {
		case <-ctx.Done():
			log.Println("💰 [Monero] 充值监听器已停止")
			return
		case <-ticker.C:
			mm.checkPendingDeposits(ctx)
		}
	}
}

// checkPendingDeposits 检查待确认的充值
func (mm *MoneroManager) checkPendingDeposits(ctx context.Context) {
	var deposits []models.Deposit
	if err := mm.DB.Where("status = ?", "pending").Find(&deposits).Error; err != nil {
		log.Printf("❌ [Monero] 查询待确认充值失败: %v", err)
		return
	}

	for _, deposit := range deposits {
		mm.processDeposit(ctx, &deposit)
	}
}

// processDeposit 处理单个充值
func (mm *MoneroManager) processDeposit(ctx context.Context, deposit *models.Deposit) {
	confirmations, err := mm.getTransactionConfirmations(ctx, deposit.TxHash)
	if err != nil {
		log.Printf("❌ [Monero] 查询交易 %s 失败: %v", deposit.TxHash, err)
		return
	}

	mm.DB.Model(deposit).Update("confirmations", confirmations)
	mm.publishDepositProgress(deposit.UserID, deposit.TxHash, confirmations)

	if confirmations >= MinConfirmations && deposit.Status == "pending" {
		mm.confirmDeposit(deposit)
	}
}

// getTransactionConfirmations 获取交易确认数
func (mm *MoneroManager) getTransactionConfirmations(ctx context.Context, txHash string) (int, error) {
	result, err := mm.RPCClient.GetTransferByTxID(ctx, txHash)
	if err != nil {
		return 0, err
	}
	return result.Confirmations, nil
}

// confirmDeposit 确认充值（单一真相源，幂等）
func (mm *MoneroManager) confirmDeposit(deposit *models.Deposit) {
	log.Printf("💰 [Monero] 确认充值: 用户=%s, 金额=%.8f XMR", deposit.UserID, deposit.AmountXMR)

	err := mm.DB.Transaction(func(tx *gorm.DB) error {
		// 幂等保护：仅 PENDING → CONFIRMED 可落账
		now := time.Now()
		result := tx.Model(deposit).
			Where("status = ?", "pending").
			Updates(map[string]any{
				"status":       "confirmed",
				"confirmed_at": &now,
			})
		if result.Error != nil {
			return fmt.Errorf("更新充值状态失败: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			// 状态前置条件不满足（已确认或已失败），跳过落账
			log.Printf("💰 [Monero] 充值 %s 状态非 pending，跳过落账", deposit.TxHash)
			return nil
		}

		if err := tx.Model(&models.User{}).
			Where("user_id = ?", deposit.UserID).
			Updates(map[string]any{
				"balance":     gorm.Expr("balance + ?", deposit.AmountXMR),
				"balance_usd": gorm.Expr("balance_usd + ?", deposit.AmountUSD),
			}).Error; err != nil {
			return fmt.Errorf("更新用户余额失败: %w", err)
		}

		billingLog := models.BillingLog{
			UserID:  deposit.UserID,
			CostUSD: deposit.AmountUSD,
			LogType: "deposit",
		}
		if err := tx.Create(&billingLog).Error; err != nil {
			return fmt.Errorf("记录计费流水失败: %w", err)
		}

		return nil
	})

	if err != nil {
		log.Printf("❌ [Monero] 确认充值失败: %v", err)
		return
	}

	// 事务已 commit → 同步 Redis 余额
	if mm.QuotaBridge != nil {
		go mm.QuotaBridge.SyncAfterDeposit(context.Background(), deposit.UserID, deposit.AmountUSD)
	}

	mm.publishDepositConfirmed(deposit.UserID, deposit.AmountXMR, deposit.AmountUSD)
}

// publishDepositProgress 推送充值进度
func (mm *MoneroManager) publishDepositProgress(userID, txHash string, confirmations int) {
	ctx := context.Background()
	channel := fmt.Sprintf("mirage:user:%s:events", userID)

	event := map[string]any{
		"type":      "deposit_progress",
		"timestamp": time.Now().Unix(),
		"data": map[string]any{
			"txHash":        txHash,
			"confirmations": confirmations,
			"required":      MinConfirmations,
			"message":       fmt.Sprintf("检测到转账，等待 %d/%d 个确认块...", confirmations, MinConfirmations),
		},
	}

	payload, _ := json.Marshal(event)
	mm.Redis.Publish(ctx, channel, payload)
}

// publishDepositConfirmed 推送到账通知
func (mm *MoneroManager) publishDepositConfirmed(userID string, amountXMR, amountUSD float64) {
	ctx := context.Background()
	channel := fmt.Sprintf("mirage:user:%s:events", userID)

	event := map[string]any{
		"type":      "deposit_confirmed",
		"timestamp": time.Now().Unix(),
		"data": map[string]any{
			"amountXMR": amountXMR,
			"amountUSD": amountUSD,
			"message":   fmt.Sprintf("充值成功：%.8f XMR ($%.2f)", amountXMR, amountUSD),
		},
	}

	payload, _ := json.Marshal(event)
	mm.Redis.Publish(ctx, channel, payload)

	log.Printf("💰 [Monero] 已推送到账通知: 用户=%s, 金额=%.8f XMR", userID, amountXMR)
}

// CreateDeposit 创建充值记录
func (mm *MoneroManager) CreateDeposit(userID, txHash string, amountXMR, exchangeRate float64) error {
	amountUSD := amountXMR * exchangeRate

	deposit := &models.Deposit{
		UserID:       userID,
		TxHash:       txHash,
		AmountXMR:    amountXMR,
		AmountUSD:    amountUSD,
		ExchangeRate: exchangeRate,
		Status:       "pending",
	}

	if err := mm.DB.Create(deposit).Error; err != nil {
		return fmt.Errorf("创建充值记录失败: %w", err)
	}

	log.Printf("💰 [Monero] 创建充值记录: 用户=%s, TxHash=%s, 金额=%.8f XMR", userID, txHash, amountXMR)
	return nil
}

// GetExchangeRate 获取 XMR/USD 汇率
func (mm *MoneroManager) GetExchangeRate(ctx context.Context) (float64, error) {
	if mm.ExchangeProvider == nil {
		return 0, fmt.Errorf("exchange rate provider not configured")
	}
	return mm.ExchangeProvider.GetXMRUSDRate(ctx)
}
