package dispatch

import (
	"database/sql"
	"log"
	"time"

	pb "mirage-proto/gen"
)

// BlacklistSyncer 负责从 threat_intel 表查询已封禁条目并推送到所有在线 Gateway
type BlacklistSyncer struct {
	db         *sql.DB
	dispatcher *StrategyDispatcher
}

// NewBlacklistSyncer 创建黑名单同步器
func NewBlacklistSyncer(db *sql.DB, dispatcher *StrategyDispatcher) *BlacklistSyncer {
	return &BlacklistSyncer{db: db, dispatcher: dispatcher}
}

// SyncAll 全量同步：查询所有已封禁条目并推送到所有在线 Gateway
func (bs *BlacklistSyncer) SyncAll() error {
	entries, err := bs.queryBannedEntries()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		log.Println("[BlacklistSyncer] no banned entries to sync")
		return nil
	}

	if err := bs.dispatcher.PushBlacklistToAll(entries); err != nil {
		log.Printf("[BlacklistSyncer] push blacklist failed: %v", err)
		return err
	}

	log.Printf("[BlacklistSyncer] synced %d banned entries to all gateways", len(entries))
	return nil
}

// SyncSingleIP 单 IP 同步：将指定 IP 的封禁状态推送到所有在线 Gateway
func (bs *BlacklistSyncer) SyncSingleIP(sourceIP string) error {
	entries := []*pb.BlacklistEntryProto{
		{
			Cidr:     sourceIP + "/32",
			ExpireAt: time.Now().Add(24 * time.Hour).Unix(),
			Source:   pb.BlacklistSourceType_BL_GLOBAL,
		},
	}

	return bs.dispatcher.PushBlacklistToAll(entries)
}

// queryBannedEntries 从 threat_intel 表查询所有已封禁条目
func (bs *BlacklistSyncer) queryBannedEntries() ([]*pb.BlacklistEntryProto, error) {
	rows, err := bs.db.Query(`
		SELECT DISTINCT source_ip, ttl_seconds, expires_at
		FROM threat_intel
		WHERE is_banned = true
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*pb.BlacklistEntryProto
	for rows.Next() {
		var sourceIP string
		var ttlSeconds int
		var expiresAt sql.NullTime

		if err := rows.Scan(&sourceIP, &ttlSeconds, &expiresAt); err != nil {
			log.Printf("[BlacklistSyncer] scan row error: %v", err)
			continue
		}

		expiry := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()
		if expiresAt.Valid {
			expiry = expiresAt.Time.Unix()
		}

		entries = append(entries, &pb.BlacklistEntryProto{
			Cidr:     sourceIP + "/32",
			ExpireAt: expiry,
			Source:   pb.BlacklistSourceType_BL_GLOBAL,
		})
	}

	return entries, rows.Err()
}

// GetBannedSummary 获取封禁摘要（条目数 + 最新更新时间戳），用于心跳一致性校验
func (bs *BlacklistSyncer) GetBannedSummary() (count int, latestUpdatedAt int64, err error) {
	err = bs.db.QueryRow(`
		SELECT COUNT(*), COALESCE(EXTRACT(EPOCH FROM MAX(last_seen))::bigint, 0)
		FROM threat_intel
		WHERE is_banned = true
	`).Scan(&count, &latestUpdatedAt)
	return
}
