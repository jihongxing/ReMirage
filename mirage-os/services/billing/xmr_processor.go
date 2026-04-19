// XMR 异步计费引擎 - Monero RPC 集成
package billing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type XMRProcessor struct {
	mu              sync.RWMutex
	rpcURL          string
	walletRPC       string
	pendingDeposits map[string]*PendingDeposit
	balances        map[string]uint64 // UID -> piconero
	confirmRequired int
	onConfirmed     func(uid string, amount uint64)
	stopCh          chan struct{}
	
	// 动态确认策略阈值
	smallAmountThreshold uint64 // 小额阈值 (piconero)
	smallConfirmRequired int    // 小额确认数
	largeConfirmRequired int    // 大额确认数
}

type PendingDeposit struct {
	UID             string    `json:"uid"`
	Address         string    `json:"address"`
	PaymentID       string    `json:"payment_id"`
	TxHash          string    `json:"tx_hash,omitempty"`
	Amount          uint64    `json:"amount"` // piconero
	Confirmations   int       `json:"confirmations"`
	Status          string    `json:"status"` // pending, confirming, confirmed
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type IntegratedAddress struct {
	Address   string `json:"integrated_address"`
	PaymentID string `json:"payment_id"`
}

func NewXMRProcessor(rpcURL, walletRPC string, confirmRequired int) *XMRProcessor {
	return &XMRProcessor{
		rpcURL:               rpcURL,
		walletRPC:            walletRPC,
		pendingDeposits:      make(map[string]*PendingDeposit),
		balances:             make(map[string]uint64),
		confirmRequired:      confirmRequired,
		stopCh:               make(chan struct{}),
		smallAmountThreshold: 1_000_000_000_000, // 1 XMR = 10^12 piconero
		smallConfirmRequired: 3,
		largeConfirmRequired: 10,
	}
}

// SetDynamicConfirmPolicy 设置动态确认策略
func (p *XMRProcessor) SetDynamicConfirmPolicy(smallThreshold uint64, smallConfirm, largeConfirm int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.smallAmountThreshold = smallThreshold
	p.smallConfirmRequired = smallConfirm
	p.largeConfirmRequired = largeConfirm
}

// getRequiredConfirmations 根据金额动态计算所需确认数
func (p *XMRProcessor) getRequiredConfirmations(amount uint64) int {
	if amount < p.smallAmountThreshold {
		return p.smallConfirmRequired // 小额：3 确认
	}
	return p.largeConfirmRequired // 大额：10 确认防双花
}

// GenerateDepositAddress 生成一次性 Integrated Address
func (p *XMRProcessor) GenerateDepositAddress(uid string) (*PendingDeposit, error) {
	// 调用 Monero Wallet RPC 生成 Integrated Address
	resp, err := p.callWalletRPC("make_integrated_address", nil)
	if err != nil {
		return nil, fmt.Errorf("rpc error: %w", err)
	}
	
	var result struct {
		IntegratedAddress string `json:"integrated_address"`
		PaymentID         string `json:"payment_id"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	
	deposit := &PendingDeposit{
		UID:           uid,
		Address:       result.IntegratedAddress,
		PaymentID:     result.PaymentID,
		Status:        "pending",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(1 * time.Hour),
	}
	
	p.mu.Lock()
	p.pendingDeposits[result.PaymentID] = deposit
	p.mu.Unlock()
	
	return deposit, nil
}

// StartWatcher 启动区块链监听
func (p *XMRProcessor) StartWatcher() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-p.stopCh:
				return
			case <-ticker.C:
				p.checkPendingDeposits()
			}
		}
	}()
}

// checkPendingDeposits 检查待确认交易
func (p *XMRProcessor) checkPendingDeposits() {
	p.mu.RLock()
	deposits := make([]*PendingDeposit, 0, len(p.pendingDeposits))
	for _, d := range p.pendingDeposits {
		if d.Status != "confirmed" {
			deposits = append(deposits, d)
		}
	}
	p.mu.RUnlock()
	
	for _, deposit := range deposits {
		// 检查是否过期
		if time.Now().After(deposit.ExpiresAt) && deposit.Status == "pending" {
			p.mu.Lock()
			delete(p.pendingDeposits, deposit.PaymentID)
			p.mu.Unlock()
			continue
		}
		
		// 查询交易状态
		p.updateDepositStatus(deposit)
	}
}

// updateDepositStatus 更新交易状态
func (p *XMRProcessor) updateDepositStatus(deposit *PendingDeposit) {
	// 调用 get_payments 查询
	params := map[string]interface{}{
		"payment_id": deposit.PaymentID,
	}
	
	resp, err := p.callWalletRPC("get_payments", params)
	if err != nil {
		return
	}
	
	var result struct {
		Payments []struct {
			TxHash        string `json:"tx_hash"`
			Amount        uint64 `json:"amount"`
			BlockHeight   uint64 `json:"block_height"`
			UnlockTime    uint64 `json:"unlock_time"`
		} `json:"payments"`
	}
	
	if err := json.Unmarshal(resp, &result); err != nil || len(result.Payments) == 0 {
		return
	}
	
	payment := result.Payments[0]
	
	// 获取当前区块高度计算确认数
	height, err := p.getBlockHeight()
	if err != nil {
		return
	}
	
	confirmations := int(height - payment.BlockHeight)
	
	p.mu.Lock()
	deposit.TxHash = payment.TxHash
	deposit.Amount = payment.Amount
	deposit.Confirmations = confirmations
	
	if confirmations >= 1 && deposit.Status == "pending" {
		deposit.Status = "confirming"
	}
	
	// 动态确认策略：根据金额决定所需确认数
	requiredConfirms := p.getRequiredConfirmations(deposit.Amount)
	
	if confirmations >= requiredConfirms && deposit.Status != "confirmed" {
		deposit.Status = "confirmed"
		// 写入余额
		p.balances[deposit.UID] += deposit.Amount
		
		if p.onConfirmed != nil {
			go p.onConfirmed(deposit.UID, deposit.Amount)
		}
	}
	p.mu.Unlock()
}

// getBlockHeight 获取当前区块高度
func (p *XMRProcessor) getBlockHeight() (uint64, error) {
	resp, err := p.callDaemonRPC("get_block_count", nil)
	if err != nil {
		return 0, err
	}
	
	var result struct {
		Count uint64 `json:"count"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, err
	}
	
	return result.Count, nil
}

// GetBalance 获取用户余额 (piconero)
func (p *XMRProcessor) GetBalance(uid string) uint64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.balances[uid]
}

// DeductBalance 扣除余额
func (p *XMRProcessor) DeductBalance(uid string, amount uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.balances[uid] < amount {
		return errors.New("insufficient balance")
	}
	p.balances[uid] -= amount
	return nil
}

// GetDepositStatus 获取充值状态
func (p *XMRProcessor) GetDepositStatus(paymentID string) *PendingDeposit {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pendingDeposits[paymentID]
}

// SetOnConfirmed 设置确认回调
func (p *XMRProcessor) SetOnConfirmed(fn func(uid string, amount uint64)) {
	p.onConfirmed = fn
}

// Stop 停止监听
func (p *XMRProcessor) Stop() {
	close(p.stopCh)
}

// callWalletRPC 调用钱包 RPC
func (p *XMRProcessor) callWalletRPC(method string, params interface{}) (json.RawMessage, error) {
	return p.callRPC(p.walletRPC, method, params)
}

// callDaemonRPC 调用守护进程 RPC
func (p *XMRProcessor) callDaemonRPC(method string, params interface{}) (json.RawMessage, error) {
	return p.callRPC(p.rpcURL, method, params)
}

func (p *XMRProcessor) callRPC(url, method string, params interface{}) (json.RawMessage, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "0",
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}
	
	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(url+"/json_rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	
	return rpcResp.Result, nil
}
