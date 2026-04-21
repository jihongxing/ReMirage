package dispatch

import (
	"context"
	"database/sql"
	"log"
)

// QuotaDownlink 配额下行推送接口
type QuotaDownlink interface {
	PushQuotaToGatewayForUser(gatewayID, userID string, remainingBytes uint64) error
}

// QuotaDispatcher 按用户维度配额下发
type QuotaDispatcher struct {
	db       *sql.DB
	downlink QuotaDownlink
}

// NewQuotaDispatcher 创建配额下发器
func NewQuotaDispatcher(db *sql.DB, downlink QuotaDownlink) *QuotaDispatcher {
	return &QuotaDispatcher{db: db, downlink: downlink}
}

// PushQuotaToGateway 查询 Gateway 上所有活跃用户，为每个用户生成 QuotaPush
func (d *QuotaDispatcher) PushQuotaToGateway(ctx context.Context, gatewayID string) error {
	rows, err := d.db.QueryContext(ctx, `
		SELECT DISTINCT gs.user_id, COALESCE(u.remaining_quota, 0)
		FROM gateway_sessions gs
		JOIN users u ON gs.user_id::uuid = u.id::uuid
		WHERE gs.gateway_id = $1 AND gs.status = 'active'
	`, gatewayID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var userID string
		var quota float64
		if err := rows.Scan(&userID, &quota); err != nil {
			log.Printf("[WARN] scan user quota: %v", err)
			continue
		}
		// 转换 GB → bytes
		quotaBytes := uint64(quota * 1e9)
		if err := d.downlink.PushQuotaToGatewayForUser(gatewayID, userID, quotaBytes); err != nil {
			log.Printf("[WARN] push quota to gateway %s for user %s: %v", gatewayID, userID, err)
		}
	}
	return rows.Err()
}

// PushQuotaForUser 当用户配额变更时，向该用户所有活跃 Gateway 下发配额
func (d *QuotaDispatcher) PushQuotaForUser(ctx context.Context, userID string) error {
	// 查询该用户当前配额
	var quota float64
	err := d.db.QueryRowContext(ctx, `
		SELECT COALESCE(remaining_quota, 0) FROM users WHERE id = $1
	`, userID).Scan(&quota)
	if err != nil {
		return err
	}
	quotaBytes := uint64(quota * 1e9)

	// 查询该用户活跃会话所在的所有 Gateway
	rows, err := d.db.QueryContext(ctx, `
		SELECT DISTINCT gateway_id FROM gateway_sessions
		WHERE user_id = $1 AND status = 'active'
	`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var gatewayID string
		if err := rows.Scan(&gatewayID); err != nil {
			continue
		}
		if err := d.downlink.PushQuotaToGatewayForUser(gatewayID, userID, quotaBytes); err != nil {
			log.Printf("[WARN] push quota to gateway %s for user %s: %v", gatewayID, userID, err)
		}
	}
	return rows.Err()
}
