package quota

import (
	"database/sql"
	"fmt"

	"mirage-os/gateway-bridge/pkg/config"
)

type Enforcer struct {
	db            *sql.DB
	businessPrice float64
	defensePrice  float64
}

func NewEnforcer(db *sql.DB, cfg config.PricingConfig) *Enforcer {
	bp := cfg.BusinessPricePerGB
	if bp == 0 {
		bp = 0.10
	}
	dp := cfg.DefensePricePerGB
	if dp == 0 {
		dp = 0.05
	}
	return &Enforcer{db: db, businessPrice: bp, defensePrice: dp}
}

// CalculateCost 纯函数：计算费用
func (e *Enforcer) CalculateCost(businessBytes, defenseBytes uint64, multiplier float64) (businessCost, defenseCost, totalCost float64) {
	businessCost = (float64(businessBytes) / 1e9) * e.businessPrice * multiplier
	defenseCost = (float64(defenseBytes) / 1e9) * e.defensePrice * multiplier
	totalCost = businessCost + defenseCost
	return
}

// Settle 结算流量（事务原子操作）
// 优先使用 userID 精确扣费；userID 为空时 fallback 到 gateway→cell 关联
func (e *Enforcer) Settle(gatewayID string, businessBytes, defenseBytes uint64, periodSeconds int32) (remainingQuota float64, err error) {
	return e.SettleForUser(gatewayID, "", businessBytes, defenseBytes, periodSeconds, "", 0)
}

// SettleForUser 按精确 user_id 结算流量
func (e *Enforcer) SettleForUser(gatewayID, userID string, businessBytes, defenseBytes uint64, periodSeconds int32, sessionID ...interface{}) (remainingQuota float64, err error) {
	// 解析可选参数 sessionID 和 sequenceNumber
	var sessID string
	var seqNum uint64
	if len(sessionID) >= 1 {
		if v, ok := sessionID[0].(string); ok {
			sessID = v
		}
	}
	if len(sessionID) >= 2 {
		switch v := sessionID[1].(type) {
		case uint64:
			seqNum = v
		case int:
			seqNum = uint64(v)
		}
	}
	if businessBytes == 0 && defenseBytes == 0 {
		if userID != "" {
			return e.GetRemainingQuotaByUser(userID)
		}
		return e.GetRemainingQuota(gatewayID)
	}
	if periodSeconds == 0 {
		if userID != "" {
			return e.GetRemainingQuotaByUser(userID)
		}
		return e.GetRemainingQuota(gatewayID)
	}

	tx, err := e.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// 查询 cost_multiplier
	var multiplier float64
	err = tx.QueryRow(`
		SELECT COALESCE(c.cost_multiplier, 1.0)
		FROM gateways g
		LEFT JOIN cells c ON g.cell_id = c.id
		WHERE g.id = $1
	`, gatewayID).Scan(&multiplier)
	if err != nil {
		return 0, fmt.Errorf("query gateway cell: %w", err)
	}

	// 如果没有精确 user_id，通过 gateway→cell 关联查找
	// 注意：这里改为查找该 cell 下配额最高的活跃用户（避免随机扣错人）
	if userID == "" {
		err = tx.QueryRow(`
			SELECT u.id FROM users u
			JOIN gateways g ON u.cell_id = g.cell_id
			WHERE g.id = $1 AND u.is_active = true
			ORDER BY u.remaining_quota DESC
			LIMIT 1
		`, gatewayID).Scan(&userID)
		if err != nil {
			return 0, fmt.Errorf("query user for gateway: %w", err)
		}
	}

	businessCost, defenseCost, totalCost := e.CalculateCost(businessBytes, defenseBytes, multiplier)

	// 扣减 quota + 更新 total_consumed
	err = tx.QueryRow(`
		UPDATE users
		SET remaining_quota = remaining_quota - $1,
		    total_consumed = total_consumed + $1,
		    updated_at = NOW()
		WHERE id = $2
		RETURNING remaining_quota
	`, totalCost, userID).Scan(&remainingQuota)
	if err != nil {
		return 0, fmt.Errorf("update quota: %w", err)
	}

	// 插入 billing_log（携带 session_id 和 sequence_number）
	if seqNum > 0 {
		_, err = tx.Exec(`
			INSERT INTO billing_logs (user_id, gateway_id, business_bytes, defense_bytes, business_cost, defense_cost, total_cost, period_seconds, session_id, sequence_number)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (gateway_id, sequence_number) DO NOTHING
		`, userID, gatewayID, businessBytes, defenseBytes, businessCost, defenseCost, totalCost, periodSeconds, nullIfEmpty(sessID), seqNum)
	} else {
		_, err = tx.Exec(`
			INSERT INTO billing_logs (user_id, gateway_id, business_bytes, defense_bytes, business_cost, defense_cost, total_cost, period_seconds, session_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, userID, gatewayID, businessBytes, defenseBytes, businessCost, defenseCost, totalCost, periodSeconds, nullIfEmpty(sessID))
	}
	if err != nil {
		return 0, fmt.Errorf("insert billing log: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return remainingQuota, nil
}

// GetRemainingQuotaByUser 按 user_id 查询剩余配额
func (e *Enforcer) GetRemainingQuotaByUser(userID string) (float64, error) {
	var quota float64
	err := e.db.QueryRow(`
		SELECT COALESCE(remaining_quota, 0) FROM users WHERE id = $1 AND is_active = true
	`, userID).Scan(&quota)
	if err != nil {
		return 0, fmt.Errorf("get remaining quota by user: %w", err)
	}
	return quota, nil
}

// GetRemainingQuota 查询用户剩余配额（通过 gateway_id 关联）
func (e *Enforcer) GetRemainingQuota(gatewayID string) (float64, error) {
	var quota float64
	err := e.db.QueryRow(`
		SELECT COALESCE(u.remaining_quota, 0)
		FROM users u
		JOIN gateways g ON u.cell_id = g.cell_id
		WHERE g.id = $1 AND u.is_active = true
		LIMIT 1
	`, gatewayID).Scan(&quota)
	if err != nil {
		return 0, fmt.Errorf("get remaining quota: %w", err)
	}
	return quota, nil
}

// nullIfEmpty returns nil for empty strings (maps to SQL NULL)
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
