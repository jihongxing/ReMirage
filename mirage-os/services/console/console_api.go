// Package console - 影子控制台 HTTP API
// 桥接 gRPC 服务实现，为 React 前端提供 RESTful JSON 接口
// 安全入口：mTLS 客户端证书认证（无证书 → 400 Bad Request）
package console

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"mirage-os/pkg/models"
	"mirage-os/services/billing"
	"mirage-os/services/provisioning"
	"net/http"
	"os"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConsoleServer 控制台 API 服务器
type ConsoleServer struct {
	db          *gorm.DB
	billing     *billing.BillingServiceImpl
	invitations *billing.InvitationService
	tierRouter  *provisioning.TierRouter
	provisioner *provisioning.Provisioner
	mux         *http.ServeMux
}

// Config 控制台配置
type Config struct {
	ListenAddr string // 默认 127.0.0.1:8443（仅本地）
	CertFile   string // 服务端证书
	KeyFile    string // 服务端私钥
	ClientCA   string // 客户端 CA（mTLS）
}

// NewConsoleServer 创建控制台服务器
func NewConsoleServer(
	db *gorm.DB,
	billingSvc *billing.BillingServiceImpl,
	inviteSvc *billing.InvitationService,
	tierRouter *provisioning.TierRouter,
	prov *provisioning.Provisioner,
) *ConsoleServer {
	s := &ConsoleServer{
		db:          db,
		billing:     billingSvc,
		invitations: inviteSvc,
		tierRouter:  tierRouter,
		provisioner: prov,
		mux:         http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Start 启动控制台（mTLS 或仅本地绑定）
func (s *ConsoleServer) Start(cfg Config) error {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:8443"
	}

	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: s.mux,
	}

	// 如果提供了 ClientCA → 启用 mTLS
	if cfg.ClientCA != "" && cfg.CertFile != "" {
		caCert, err := os.ReadFile(cfg.ClientCA)
		if err != nil {
			return fmt.Errorf("读取 ClientCA 失败: %w", err)
		}
		caPool := x509.NewCertPool()
		caPool.AppendCertsFromPEM(caCert)

		server.TLSConfig = &tls.Config{
			ClientCAs:  caPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS13,
		}

		log.Printf("🔐 [Console] mTLS 控制台启动: %s (需要客户端证书)", cfg.ListenAddr)
		return server.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
	}

	// 无 mTLS → 仅绑定 127.0.0.1（物理隔离，SSH 隧道访问）
	log.Printf("🔐 [Console] 控制台启动: %s (仅本地访问，建议 SSH 隧道)", cfg.ListenAddr)
	if cfg.CertFile != "" {
		return server.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
	}
	return server.ListenAndServe()
}

// registerRoutes 注册所有路由
func (s *ConsoleServer) registerRoutes() {
	// ═══ 用户管理 ═══
	s.mux.HandleFunc("/api/v1/users", s.handleListUsers)
	s.mux.HandleFunc("/api/v1/users/ban", s.handleBanUser)

	// ═══ 财务大盘 ═══
	s.mux.HandleFunc("/api/v1/finance/overview", s.handleFinanceOverview)
	s.mux.HandleFunc("/api/v1/finance/deposits", s.handleDepositHistory)

	// ═══ 邀请码 ═══
	s.mux.HandleFunc("/api/v1/invitations/generate", s.handleGenerateInvite)
	s.mux.HandleFunc("/api/v1/invitations/list", s.handleListInvitations)

	// ═══ 蜂窝生命周期（意图驱动） ═══
	s.mux.HandleFunc("/api/v1/cells/list", s.handleListCells)
	s.mux.HandleFunc("/api/v1/cells/gateways", s.handleListGateways)
	s.mux.HandleFunc("/api/v1/cells/promote-to-calibration", s.handlePromoteToCalibration)
	s.mux.HandleFunc("/api/v1/cells/activate", s.handleActivateGateway)
	s.mux.HandleFunc("/api/v1/cells/retire", s.handleRetireGateway)

	// ═══ 配额总览 ═══
	s.mux.HandleFunc("/api/v1/quota/overview", s.handleQuotaOverview)
}

// ═══════════════════════════════════════════════════════════════
// 用户管理
// ═══════════════════════════════════════════════════════════════

func (s *ConsoleServer) handleListUsers(w http.ResponseWriter, r *http.Request) {
	var users []models.User
	query := s.db.Order("created_at DESC").Limit(100)

	// 可选过滤
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if level := r.URL.Query().Get("level"); level != "" {
		query = query.Where("cell_level = ?", level)
	}

	if err := query.Find(&users).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}

	type UserSummary struct {
		UserID         string  `json:"user_id"`
		CellLevel      int     `json:"cell_level"`
		BalanceUSD     float64 `json:"balance_usd"`
		RemainingQuota int64   `json:"remaining_quota"`
		TotalQuota     int64   `json:"total_quota"`
		TrustScore     int     `json:"trust_score"`
		Status         string  `json:"status"`
		CreatedAt      string  `json:"created_at"`
	}

	result := make([]UserSummary, 0, len(users))
	for _, u := range users {
		result = append(result, UserSummary{
			UserID:         u.UserID,
			CellLevel:      u.CellLevel,
			BalanceUSD:     u.BalanceUSD,
			RemainingQuota: u.RemainingQuota,
			TotalQuota:     u.TotalQuota,
			TrustScore:     u.TrustScore,
			Status:         u.Status,
			CreatedAt:      u.CreatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, map[string]any{"users": result, "total": len(result)})
}

func (s *ConsoleServer) handleBanUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		UserID string `json:"user_id"`
		Action string `json:"action"` // "ban" or "unban"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	newStatus := "banned"
	if req.Action == "unban" {
		newStatus = "active"
	}

	if err := s.db.Model(&models.User{}).Where("user_id = ?", req.UserID).
		Update("status", newStatus).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "操作失败")
		return
	}

	writeJSON(w, map[string]string{"status": "ok", "new_status": newStatus})
}

// ═══════════════════════════════════════════════════════════════
// 财务大盘
// ═══════════════════════════════════════════════════════════════

func (s *ConsoleServer) handleFinanceOverview(w http.ResponseWriter, r *http.Request) {
	type Overview struct {
		TotalUsers       int64   `json:"total_users"`
		ActiveUsers      int64   `json:"active_users"`
		TotalBalanceUSD  float64 `json:"total_balance_usd"`
		TotalDepositsUSD float64 `json:"total_deposits_usd"`
		TotalQuotaBytes  int64   `json:"total_quota_bytes"`
		UsedQuotaBytes   int64   `json:"used_quota_bytes"`
		PendingDeposits  int64   `json:"pending_deposits"`
	}

	var ov Overview
	s.db.Model(&models.User{}).Count(&ov.TotalUsers)
	s.db.Model(&models.User{}).Where("status = 'active'").Count(&ov.ActiveUsers)

	s.db.Model(&models.User{}).Select("COALESCE(SUM(balance_usd), 0)").Row().Scan(&ov.TotalBalanceUSD)
	s.db.Model(&models.User{}).Select("COALESCE(SUM(total_quota), 0)").Row().Scan(&ov.TotalQuotaBytes)
	s.db.Model(&models.User{}).Select("COALESCE(SUM(total_quota - remaining_quota), 0)").Row().Scan(&ov.UsedQuotaBytes)

	s.db.Model(&models.Deposit{}).Where("status = 'confirmed'").
		Select("COALESCE(SUM(amount_usd), 0)").Row().Scan(&ov.TotalDepositsUSD)
	s.db.Model(&models.Deposit{}).Where("status = 'pending'").Count(&ov.PendingDeposits)

	writeJSON(w, ov)
}

func (s *ConsoleServer) handleDepositHistory(w http.ResponseWriter, r *http.Request) {
	var deposits []models.Deposit
	s.db.Order("created_at DESC").Limit(50).Find(&deposits)

	type DepositItem struct {
		UserID        string  `json:"user_id"`
		TxHash        string  `json:"tx_hash"`
		AmountXMR     float64 `json:"amount_xmr"`
		AmountUSD     float64 `json:"amount_usd"`
		ExchangeRate  float64 `json:"exchange_rate"`
		Status        string  `json:"status"`
		Confirmations int     `json:"confirmations"`
		CreatedAt     string  `json:"created_at"`
	}

	result := make([]DepositItem, 0, len(deposits))
	for _, d := range deposits {
		result = append(result, DepositItem{
			UserID:        d.UserID,
			TxHash:        d.TxHash,
			AmountXMR:     d.AmountXMR,
			AmountUSD:     d.AmountUSD,
			ExchangeRate:  d.ExchangeRate,
			Status:        d.Status,
			Confirmations: d.Confirmations,
			CreatedAt:     d.CreatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, map[string]any{"deposits": result})
}

// ═══════════════════════════════════════════════════════════════
// 邀请码
// ═══════════════════════════════════════════════════════════════

func (s *ConsoleServer) handleGenerateInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		CreatorUID string `json:"creator_uid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	code, err := s.invitations.GenerateInviteCode(r.Context(), req.CreatorUID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, map[string]string{"code": code})
}

func (s *ConsoleServer) handleListInvitations(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		// 管理员视角：查所有
		var invitations []models.Invitation
		s.db.Order("created_at DESC").Limit(100).Find(&invitations)
		writeJSON(w, map[string]any{"invitations": invitations})
		return
	}

	invitations, err := s.invitations.GetUserInvitations(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"invitations": invitations})
}

// ═══════════════════════════════════════════════════════════════
// 蜂窝生命周期（意图驱动 API）
// 状态机：潜伏(Phase 0) → 校准(Phase 1) → 服役(Phase 2) → 退役
// ═══════════════════════════════════════════════════════════════

func (s *ConsoleServer) handleListCells(w http.ResponseWriter, r *http.Request) {
	var cells []models.Cell
	s.db.Order("cell_level DESC, created_at ASC").Find(&cells)
	writeJSON(w, map[string]any{"cells": cells})
}

func (s *ConsoleServer) handleListGateways(w http.ResponseWriter, r *http.Request) {
	cellID := r.URL.Query().Get("cell_id")

	var gateways []models.Gateway
	query := s.db.Order("phase ASC, active_connections DESC")
	if cellID != "" {
		query = query.Where("cell_id = ?", cellID)
	}
	query.Limit(200).Find(&gateways)

	writeJSON(w, map[string]any{"gateways": gateways})
}

// handlePromoteToCalibration 晋升至校准期
// POST /api/v1/cells/promote-to-calibration
// 前置条件：Gateway 必须处于 Phase 0（潜伏期）
// 后置动作：下发网络质量测量指令，开始 RTT/丢包率基线采集
func (s *ConsoleServer) handlePromoteToCalibration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		GatewayID string `json:"gateway_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var gw models.Gateway
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("gateway_id = ? AND phase = 0", req.GatewayID).
			First(&gw).Error; err != nil {
			return fmt.Errorf("Gateway 不存在或不在潜伏期")
		}

		now := time.Now()
		return tx.Model(&gw).Updates(map[string]any{
			"phase":                 1,
			"incubation_started_at": &now,
		}).Error
	})

	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("⬆️ [Console] Gateway %s 晋升至校准期", req.GatewayID)
	writeJSON(w, map[string]string{"status": "calibrating", "gateway_id": req.GatewayID})
}

// handleActivateGateway 正式服役
// POST /api/v1/cells/activate
// 前置条件：Gateway 必须处于 Phase 1（校准期）且 network_quality >= 60
// 后置动作：接入路由池，开始接受用户连接
func (s *ConsoleServer) handleActivateGateway(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		GatewayID string `json:"gateway_id"`
		Force     bool   `json:"force"` // 强制激活（跳过质量检查）
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var gw models.Gateway
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("gateway_id = ? AND phase = 1", req.GatewayID).
			First(&gw).Error; err != nil {
			return fmt.Errorf("Gateway 不存在或不在校准期")
		}

		// 质量门槛检查
		if !req.Force && gw.NetworkQuality < 60 {
			return fmt.Errorf("网络质量不达标 (%.1f < 60.0)，使用 force=true 强制激活", gw.NetworkQuality)
		}

		return tx.Model(&gw).Updates(map[string]any{
			"phase":     2,
			"is_online": true,
		}).Error
	})

	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("✅ [Console] Gateway %s 正式服役", req.GatewayID)
	writeJSON(w, map[string]string{"status": "active", "gateway_id": req.GatewayID})
}

// handleRetireGateway 退役
// POST /api/v1/cells/retire
// 后置动作：标记离线，静默排空现有连接（不立即断开）
func (s *ConsoleServer) handleRetireGateway(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		GatewayID string `json:"gateway_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	// 退役：标记离线，不再接受新连接，现有连接自然超时断开
	result := s.db.Model(&models.Gateway{}).Where("gateway_id = ?", req.GatewayID).
		Updates(map[string]any{
			"is_online": false,
			"phase":     0, // 回到潜伏态
		})

	if result.RowsAffected == 0 {
		writeError(w, http.StatusNotFound, "Gateway 不存在")
		return
	}

	log.Printf("🛑 [Console] Gateway %s 已退役", req.GatewayID)
	writeJSON(w, map[string]string{"status": "retired", "gateway_id": req.GatewayID})
}

// ═══════════════════════════════════════════════════════════════
// 配额总览
// ═══════════════════════════════════════════════════════════════

func (s *ConsoleServer) handleQuotaOverview(w http.ResponseWriter, r *http.Request) {
	type QuotaStats struct {
		TotalAllocated int64   `json:"total_allocated_bytes"`
		TotalUsed      int64   `json:"total_used_bytes"`
		TotalRemaining int64   `json:"total_remaining_bytes"`
		BurnRateGBDay  float64 `json:"burn_rate_gb_per_day"`
		UsersNearLimit int64   `json:"users_near_limit"` // 剩余 < 10%
	}

	var stats QuotaStats
	s.db.Model(&models.User{}).Where("status = 'active'").
		Select("COALESCE(SUM(total_quota), 0)").Row().Scan(&stats.TotalAllocated)
	s.db.Model(&models.User{}).Where("status = 'active'").
		Select("COALESCE(SUM(remaining_quota), 0)").Row().Scan(&stats.TotalRemaining)
	stats.TotalUsed = stats.TotalAllocated - stats.TotalRemaining

	// 近 24h 消耗速率
	var consumed24h int64
	s.db.Model(&models.BillingLog{}).
		Where("log_type = 'traffic' AND created_at > ?", time.Now().Add(-24*time.Hour)).
		Select("COALESCE(SUM(total_bytes), 0)").Row().Scan(&consumed24h)
	stats.BurnRateGBDay = float64(consumed24h) / (1024 * 1024 * 1024)

	// 配额即将耗尽的用户
	s.db.Model(&models.User{}).
		Where("status = 'active' AND total_quota > 0 AND remaining_quota < total_quota * 0.1").
		Count(&stats.UsersNearLimit)

	writeJSON(w, stats)
}

// ═══════════════════════════════════════════════════════════════
// 工具函数
// ═══════════════════════════════════════════════════════════════

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
