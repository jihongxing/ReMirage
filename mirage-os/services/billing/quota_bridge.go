// Package billing - 配额桥接器
// 确保 PostgreSQL 事务成功后，配额状态同步到内存态 QuotaManager + Redis
// 解决 PurchaseQuota 只写 PostgreSQL 不同步 Redis 的问题
package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"mirage-os/pkg/redact"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const (
	// Redis 配额键前缀
	redisQuotaKey = "mirage:quota:"
	// 配额有效期（与购买周期对齐）
	quotaRedisTTL = 31 * 24 * time.Hour
)

// QuotaBridge 配额桥接器（PostgreSQL ↔ Redis 最终一致性）
type QuotaBridge struct {
	db           *gorm.DB
	rdb          *redis.Client
	quotaManager *QuotaManager

	// 失败重试通道（Redis 写入失败时入队，后台消费重试）
	retryCh chan retryTask
}

// retryTask Redis 同步重试任务
type retryTask struct {
	UserID    string
	AddBytes  int64
	ExpiresAt time.Time
	Attempts  int
	TxID      string // 幂等 key，防止重复执行
}

// generateTxID 生成唯一事务 ID
func generateTxID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Redis 幂等 key 前缀和 TTL
const (
	redisTxDedupPrefix = "mirage:tx_dedup:"
	redisTxDedupTTL    = 24 * time.Hour
)

// NewQuotaBridge 创建配额桥接器
func NewQuotaBridge(db *gorm.DB, rdb *redis.Client, qm *QuotaManager) *QuotaBridge {
	b := &QuotaBridge{
		db:           db,
		rdb:          rdb,
		quotaManager: qm,
		retryCh:      make(chan retryTask, 256),
	}
	go b.retryLoop()
	return b
}

// SyncAfterPurchase 购买成功后同步配额到 Redis + QuotaManager
// 在 PurchaseQuota 事务 commit 后调用
func (b *QuotaBridge) SyncAfterPurchase(ctx context.Context, userID string, addedBytes int64, expiresAt time.Time) error {
	txID := generateTxID()
	key := redisQuotaKey + userID
	dedupKey := redisTxDedupPrefix + txID

	// 幂等检查 + 原子增加（Lua 脚本保证原子性）
	luaScript := redis.NewScript(`
		-- 幂等检查：如果 txID 已存在则跳过
		if redis.call('EXISTS', KEYS[1]) == 1 then
			return 0
		end
		-- 标记 txID 已处理
		redis.call('SET', KEYS[1], '1', 'EX', ARGV[4])
		-- 原子增加配额
		redis.call('INCRBY', KEYS[2], ARGV[1])
		redis.call('INCRBY', KEYS[3], ARGV[1])
		redis.call('SET', KEYS[4], ARGV[2], 'EX', ARGV[3])
		redis.call('EXPIRE', KEYS[2], ARGV[3])
		redis.call('EXPIRE', KEYS[3], ARGV[3])
		return 1
	`)

	ttlSeconds := int64(quotaRedisTTL.Seconds())
	dedupTTLSeconds := int64(redisTxDedupTTL.Seconds())

	result, err := luaScript.Run(ctx, b.rdb,
		[]string{dedupKey, key + ":remaining", key + ":total", key + ":expires"},
		addedBytes, expiresAt.Unix(), ttlSeconds, dedupTTLSeconds,
	).Int64()

	if err != nil {
		log.Printf("⚠️ [QuotaBridge] Redis 同步失败 (user=%s, tx=%s): %v，入队重试", redact.Token(userID), txID, err)
		select {
		case b.retryCh <- retryTask{UserID: userID, AddBytes: addedBytes, ExpiresAt: expiresAt, TxID: txID}:
		default:
			log.Printf("🚨 [QuotaBridge] CRITICAL: 重试队列已满，user=%s 配额可能不一致", redact.Token(userID))
		}
	} else if result == 0 {
		log.Printf("⚠️ [QuotaBridge] 重复事务跳过 (user=%s, tx=%s)", redact.Token(userID), txID)
	}

	// 2. 内存态 QuotaManager 追加配额
	if b.quotaManager != nil {
		duration := time.Until(expiresAt)
		if duration < 0 {
			duration = 30 * 24 * time.Hour
		}
		b.quotaManager.AllocateQuota(userID, uint64(addedBytes), "", duration)
	}

	return nil
}

// SyncAfterDeposit 充值确认后同步余额到 Redis
func (b *QuotaBridge) SyncAfterDeposit(ctx context.Context, userID string, amountUSD float64) error {
	key := redisQuotaKey + userID + ":balance_usd"

	// Redis INCRBYFLOAT 原子增加余额
	if err := b.rdb.IncrByFloat(ctx, key, amountUSD).Err(); err != nil {
		log.Printf("⚠️ [QuotaBridge] Redis 余额同步失败 (user=%s): %v", redact.Token(userID), err)
	}

	return nil
}

// ReconcileFromDB 从 PostgreSQL 对账修复 Redis（定期任务）
// 使用版本号机制避免覆盖实时数据：只有当 DB 版本 > Redis 版本时才更新
func (b *QuotaBridge) ReconcileFromDB(ctx context.Context) error {
	type UserQuota struct {
		UserID         string
		RemainingQuota int64
		TotalQuota     int64
		BalanceUSD     float64
		QuotaExpiresAt *time.Time
		QuotaVersion   int64 // DB 侧版本号（每次 DB 写入递增）
	}

	var users []UserQuota
	if err := b.db.Table("users").
		Select("user_id, remaining_quota, total_quota, balance_usd, quota_expires_at, quota_version").
		Where("status = 'active'").
		Find(&users).Error; err != nil {
		return fmt.Errorf("查询用户配额失败: %w", err)
	}

	// 使用 Lua 脚本实现 CAS 对账：仅当 DB 版本 > Redis 版本时才覆盖
	reconcileScript := redis.NewScript(`
		local currentVersion = tonumber(redis.call('GET', KEYS[1])) or 0
		local newVersion = tonumber(ARGV[5])
		if newVersion <= currentVersion then
			return 0
		end
		redis.call('SET', KEYS[1], ARGV[5], 'EX', ARGV[4])
		redis.call('SET', KEYS[2], ARGV[1], 'EX', ARGV[4])
		redis.call('SET', KEYS[3], ARGV[2], 'EX', ARGV[4])
		redis.call('SET', KEYS[4], ARGV[3], 'EX', ARGV[4])
		if ARGV[6] ~= '' then
			redis.call('SET', KEYS[5], ARGV[6], 'EX', ARGV[4])
		end
		return 1
	`)

	ttlSeconds := int64(quotaRedisTTL.Seconds())
	reconciled := 0

	for _, u := range users {
		key := redisQuotaKey + u.UserID
		expiresStr := ""
		if u.QuotaExpiresAt != nil {
			expiresStr = strconv.FormatInt(u.QuotaExpiresAt.Unix(), 10)
		}

		result, err := reconcileScript.Run(ctx, b.rdb,
			[]string{
				key + ":version",
				key + ":remaining",
				key + ":total",
				key + ":balance_usd",
				key + ":expires",
			},
			u.RemainingQuota,
			u.TotalQuota,
			fmt.Sprintf("%.2f", u.BalanceUSD),
			ttlSeconds,
			u.QuotaVersion,
			expiresStr,
		).Int64()

		if err != nil {
			log.Printf("⚠️ [QuotaBridge] 对账用户 %s 失败: %v", redact.Token(u.UserID), err)
			continue
		}
		if result == 1 {
			reconciled++
		}
	}

	log.Printf("✅ [QuotaBridge] 对账完成: %d/%d 用户已更新", reconciled, len(users))
	return nil
}

// StartReconcileLoop 启动定期对账循环（每 5 分钟）
func (b *QuotaBridge) StartReconcileLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := b.ReconcileFromDB(ctx); err != nil {
					log.Printf("⚠️ [QuotaBridge] 对账失败: %v", err)
				}
			}
		}
	}()
}

// retryLoop 后台重试循环（消费 retryCh 中的失败任务）
func (b *QuotaBridge) retryLoop() {
	for task := range b.retryCh {
		task.Attempts++
		if task.Attempts > 5 {
			log.Printf("🚨 [QuotaBridge] CRITICAL: 重试 5 次仍失败，user=%s，等待对账修复", redact.Token(task.UserID))
			continue
		}

		// 指数退避
		time.Sleep(time.Duration(task.Attempts*task.Attempts) * time.Second)

		ctx := context.Background()
		key := redisQuotaKey + task.UserID
		dedupKey := redisTxDedupPrefix + task.TxID

		// 使用 Lua 脚本保证幂等性
		luaScript := redis.NewScript(`
			if redis.call('EXISTS', KEYS[1]) == 1 then
				return 0
			end
			redis.call('SET', KEYS[1], '1', 'EX', ARGV[4])
			redis.call('INCRBY', KEYS[2], ARGV[1])
			redis.call('INCRBY', KEYS[3], ARGV[1])
			redis.call('SET', KEYS[4], ARGV[2], 'EX', ARGV[3])
			return 1
		`)

		ttlSeconds := int64(quotaRedisTTL.Seconds())
		dedupTTLSeconds := int64(redisTxDedupTTL.Seconds())

		_, err := luaScript.Run(ctx, b.rdb,
			[]string{dedupKey, key + ":remaining", key + ":total", key + ":expires"},
			task.AddBytes, task.ExpiresAt.Unix(), ttlSeconds, dedupTTLSeconds,
		).Int64()

		if err != nil {
			// 再次入队
			select {
			case b.retryCh <- task:
			default:
			}
		} else {
			log.Printf("✅ [QuotaBridge] 重试成功: user=%s, tx=%s, attempt=%d", redact.Token(task.UserID), task.TxID, task.Attempts)
		}
	}
}

// CheckQuotaWithOverdraft 网关侧配额检查（含透支缓冲）
// 当 Redis 查不到配额或为 0 时，允许 5MB 透支缓冲期
// 同时触发异步 DB 对齐
const OverdraftAllowance int64 = 5 * 1024 * 1024 // 5MB

// QueryQuotaWithFallback 查询配额（Redis → DB 降级）
// 返回剩余字节数。如果 Redis 不可用，回退到 DB 查询
func (b *QuotaBridge) QueryQuotaWithFallback(ctx context.Context, userID string) (remaining int64, fromDB bool, err error) {
	key := redisQuotaKey + userID + ":remaining"

	// 1. 尝试 Redis
	val, err := b.rdb.Get(ctx, key).Int64()
	if err == nil {
		return val, false, nil
	}

	// 2. Redis 不可用或 key 不存在 → 回退 DB
	var user struct {
		RemainingQuota int64
	}
	if err := b.db.Table("users").
		Select("remaining_quota").
		Where("user_id = ?", userID).
		First(&user).Error; err != nil {
		return 0, true, fmt.Errorf("用户不存在: %w", err)
	}

	// 3. 异步回写 Redis（修复缓存缺失）
	go func() {
		b.rdb.Set(context.Background(), key, user.RemainingQuota, quotaRedisTTL)
	}()

	return user.RemainingQuota, true, nil
}

// ConsumeWithOverdraft 消费配额（含透支保护）
// 网关调用此方法判断是否放行流量
// 返回 true = 放行, false = 拒绝
func (b *QuotaBridge) ConsumeWithOverdraft(ctx context.Context, userID string, bytes int64) (bool, error) {
	key := redisQuotaKey + userID + ":remaining"

	// 原子扣减
	newVal, err := b.rdb.DecrBy(ctx, key, bytes).Result()
	if err != nil {
		// Redis 不可用 → 允许透支缓冲，异步对齐
		log.Printf("⚠️ [QuotaBridge] Redis 扣减失败 (user=%s): %v，允许透支", redact.Token(userID), err)
		go b.forceReconcileUser(userID)
		return true, nil
	}

	// 配额充足
	if newVal >= 0 {
		return true, nil
	}

	// 进入透支区间：-5MB 以内仍放行
	if newVal >= -OverdraftAllowance {
		log.Printf("⚠️ [QuotaBridge] 用户 %s 进入透支缓冲 (剩余: %d bytes)，触发 DB 对齐", redact.Token(userID), newVal)
		go b.forceReconcileUser(userID)
		return true, nil
	}

	// 超出透支上限 → 拒绝
	// 回滚扣减（恢复到透支上限边界），失败时入队异步修复
	if _, rollbackErr := b.rdb.IncrBy(ctx, key, bytes).Result(); rollbackErr != nil {
		log.Printf("🚨 [QuotaBridge] 回滚扣减失败 (user=%s): %v，入队异步修复", redact.Token(userID), rollbackErr)
		select {
		case b.retryCh <- retryTask{
			UserID:   userID,
			AddBytes: bytes, // 正数，回滚用
			TxID:     generateTxID(),
		}:
		default:
			log.Printf("🚨 [QuotaBridge] CRITICAL: 回滚重试队列已满，user=%s 配额可能不一致", redact.Token(userID))
		}
	}
	return false, nil
}

// forceReconcileUser 强制单用户 DB → Redis 对齐
func (b *QuotaBridge) forceReconcileUser(userID string) {
	ctx := context.Background()

	var user struct {
		RemainingQuota int64
		TotalQuota     int64
		BalanceUSD     float64
	}
	if err := b.db.Table("users").
		Select("remaining_quota, total_quota, balance_usd").
		Where("user_id = ?", userID).
		First(&user).Error; err != nil {
		log.Printf("⚠️ [QuotaBridge] 强制对齐失败 (user=%s): %v", redact.Token(userID), err)
		return
	}

	key := redisQuotaKey + userID
	pipe := b.rdb.Pipeline()
	pipe.Set(ctx, key+":remaining", user.RemainingQuota, quotaRedisTTL)
	pipe.Set(ctx, key+":total", user.TotalQuota, quotaRedisTTL)
	pipe.Set(ctx, key+":balance_usd", fmt.Sprintf("%.2f", user.BalanceUSD), quotaRedisTTL)
	pipe.Exec(ctx)

	log.Printf("✅ [QuotaBridge] 强制对齐完成: user=%s, remaining=%d", redact.Token(userID), user.RemainingQuota)
}
