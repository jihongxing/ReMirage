package intel

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"mirage-os/gateway-bridge/pkg/config"
	pb "mirage-proto/gen"

	"github.com/redis/go-redis/v9"
)

type Distributor struct {
	db           *sql.DB
	rdb          *redis.Client
	banThreshold int
	cleanupDays  int
	cleanupMin   int
}

func NewDistributor(db *sql.DB, rdb *redis.Client, cfg config.IntelConfig) *Distributor {
	bt := cfg.BanThreshold
	if bt == 0 {
		bt = 100
	}
	cd := cfg.CleanupDays
	if cd == 0 {
		cd = 30
	}
	cm := cfg.CleanupMinHits
	if cm == 0 {
		cm = 10
	}
	return &Distributor{db: db, rdb: rdb, banThreshold: bt, cleanupDays: cd, cleanupMin: cm}
}

// RecordThreat UPSERT threat_intel
func (d *Distributor) RecordThreat(event *pb.ThreatEvent, gatewayID string) error {
	_, err := d.db.Exec(`
		INSERT INTO threat_intel (source_ip, source_port, threat_type, severity, hit_count, reported_by_gateway, last_seen)
		VALUES ($1, $2, $3, $4, 1, $5, NOW())
		ON CONFLICT (source_ip, threat_type)
		DO UPDATE SET
			hit_count = threat_intel.hit_count + 1,
			severity = GREATEST(threat_intel.severity, EXCLUDED.severity),
			last_seen = NOW(),
			reported_by_gateway = EXCLUDED.reported_by_gateway
	`, event.SourceIp, event.SourcePort, event.ThreatType.String(), event.Severity, gatewayID)
	return err
}

// CheckAndBan 检查是否达到封禁阈值
func (d *Distributor) CheckAndBan(sourceIP string) (bool, error) {
	var hitCount int
	var isBanned bool
	err := d.db.QueryRow(`
		SELECT COALESCE(SUM(hit_count), 0), BOOL_OR(is_banned)
		FROM threat_intel WHERE source_ip = $1
	`, sourceIP).Scan(&hitCount, &isBanned)
	if err != nil {
		return false, fmt.Errorf("check ban: %w", err)
	}

	if isBanned {
		return true, nil
	}

	if hitCount >= d.banThreshold {
		_, err = d.db.Exec(`
			UPDATE threat_intel SET is_banned = true WHERE source_ip = $1
		`, sourceIP)
		if err != nil {
			return false, fmt.Errorf("ban ip: %w", err)
		}

		// Redis 发布 + 缓存
		ctx := context.Background()
		msg, _ := json.Marshal(map[string]string{"ip": sourceIP})
		if pubErr := d.rdb.Publish(ctx, "mirage:blacklist", string(msg)).Err(); pubErr != nil {
			log.Printf("[WARN] redis publish blacklist: %v", pubErr)
		}
		if saddErr := d.rdb.SAdd(ctx, "mirage:blacklist:global", sourceIP).Err(); saddErr != nil {
			log.Printf("[WARN] redis sadd blacklist: %v", saddErr)
		}

		return true, nil
	}

	return false, nil
}

// LoadBannedIPs 启动时从 PostgreSQL 加载已封禁 IP 到 Redis
func (d *Distributor) LoadBannedIPs() error {
	rows, err := d.db.Query(`SELECT DISTINCT source_ip FROM threat_intel WHERE is_banned = true`)
	if err != nil {
		return fmt.Errorf("load banned ips: %w", err)
	}
	defer rows.Close()

	ctx := context.Background()
	var ips []interface{}
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return err
		}
		ips = append(ips, ip)
	}

	if len(ips) > 0 {
		if err := d.rdb.SAdd(ctx, "mirage:blacklist:global", ips...).Err(); err != nil {
			return fmt.Errorf("redis sadd: %w", err)
		}
	}

	log.Printf("[INFO] loaded %d banned IPs to Redis", len(ips))
	return nil
}

// GetGlobalBlacklist 从 Redis 获取全局黑名单
func (d *Distributor) GetGlobalBlacklist() ([]string, error) {
	ctx := context.Background()
	return d.rdb.SMembers(ctx, "mirage:blacklist:global").Result()
}

// StartSubscriber 启动 Redis Pub/Sub 订阅
func (d *Distributor) StartSubscriber(ctx context.Context) {
	go func() {
		sub := d.rdb.Subscribe(ctx, "mirage:blacklist")
		defer sub.Close()
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-ch:
				if msg != nil {
					log.Printf("[INFO] blacklist update received: %s", msg.Payload)
				}
			}
		}
	}()
}

// Cleanup 清理旧记录
func (d *Distributor) Cleanup() (int64, error) {
	result, err := d.db.Exec(`
		DELETE FROM threat_intel
		WHERE last_seen < NOW() - INTERVAL '1 day' * $1
		AND hit_count < $2
		AND is_banned = false
	`, d.cleanupDays, d.cleanupMin)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
