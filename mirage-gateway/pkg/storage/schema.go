// Package storage - 核心资产 Bucket 设计
// 基于 bbolt 的加密存储结构
// 性能优化：内存层 + BoltDB 冷存储 + LRU 淘汰 + 版本戳保护
package storage

import (
	"container/list"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
)

// 内存层配置
const (
	FlushInterval   = 30 * time.Second // 刷盘间隔
	FlushBatchSize  = 100              // 批量刷盘阈值
	WriteBufferSize = 1000             // 写缓冲区大小
	MaxIPCacheSize  = 100000           // IP 缓存最大条目数
	EvictBatchSize  = 1000             // 每次淘汰数量
)

// Bucket 名称定义
const (
	BucketDNA          = "mirage_dna"           // DNA 指纹库
	BucketIPReputation = "mirage_ip_reputation" // IP 信誉库
	BucketSystem       = "mirage_sys"           // 系统状态
)

// 系统状态 Key
const (
	KeyKillSwitchState  = "kill_switch_state"
	KeyLastActiveDomain = "last_active_domain"
	KeyLastBootTime     = "last_boot_time"
	KeyVaultLocked      = "vault_locked"
)

// SystemState 系统状态
type SystemState string

const (
	StateRunning  SystemState = "RUNNING"
	StateShutdown SystemState = "SHUTDOWN"
	StateDead     SystemState = "DEAD"
)

// DNARecord DNA 指纹记录
type DNARecord struct {
	ProfileID     string            `json:"profile_id"`     // chrome_v124_linux
	Browser       string            `json:"browser"`
	Version       string            `json:"version"`
	OS            string            `json:"os"`
	JA4           string            `json:"ja4"`
	TLSExtensions []uint16          `json:"tls_extensions"`
	TCPWindow     uint16            `json:"tcp_window"`
	TTL           uint8             `json:"ttl"`
	MSS           uint16            `json:"mss"`
	QUICParams    map[string]uint64 `json:"quic_params"`
	Headers       map[string]string `json:"headers"`
	CreatedAt     int64             `json:"created_at"`
	UpdatedAt     int64             `json:"updated_at"`
	Checksum      string            `json:"checksum"`
}

// IPReputationRecord IP 信誉记录
type IPReputationRecord struct {
	IP              string  `json:"ip"`
	Latency         float64 `json:"latency"`          // ms
	ReputationScore float64 `json:"reputation_score"` // 0-100
	LastSeenTS      int64   `json:"last_seen_ts"`
	SuccessCount    int64   `json:"success_count"`
	FailCount       int64   `json:"fail_count"`
	Region          string  `json:"region"`
	Provider        string  `json:"provider"`
	Version         int64   `json:"version"` // 版本戳，防止旧数据覆盖新数据
}

// writeOp 写操作类型
type writeOp struct {
	bucket  string
	key     string
	value   []byte // 已加密
	version int64  // 版本戳
}

// ipCacheEntry IP 缓存条目（用于 LRU）
type ipCacheEntry struct {
	record  *IPReputationRecord
	element *list.Element // LRU 链表元素
}

// VaultStorage 核心资产保险箱
type VaultStorage struct {
	mu        sync.RWMutex
	db        *bolt.DB
	dbPath    string
	gcm       cipher.AEAD
	masterKey []byte
	keyInfo   *HardwareKeyInfo
	locked    bool
	failSafe  bool // Fail-Safe 锁定模式

	// === 内存层 ===
	dnaCache sync.Map // DNA 模板缓存: string -> *DNARecord

	// === IP 缓存 + LRU ===
	ipCacheMu   sync.RWMutex
	ipCacheMap  map[string]*ipCacheEntry // IP -> 缓存条目
	ipLRUList   *list.List               // LRU 链表
	ipCacheSize atomic.Int64             // 当前缓存大小

	// === 异步写入 ===
	writeCh       chan writeOp
	flushCtx      context.Context
	flushCancel   context.CancelFunc
	flushWg       sync.WaitGroup
	dirty         atomic.Int64            // 脏数据计数
	pendingWrites sync.Map                // key -> version，记录待刷盘的最新版本

	// === 统计指标 ===
	readHits   atomic.Int64 // 读命中次数
	readTotal  atomic.Int64 // 读总次数
	evictCount atomic.Int64 // 淘汰次数
}


// ErrHardwareFingerprintInsufficient 硬件指纹不足错误
var ErrHardwareFingerprintInsufficient = errors.New("硬件指纹不足，系统进入 Fail-Safe 锁定模式")

// HardwareKeySource 硬件密钥来源
type HardwareKeySource int

const (
	KeySourceHardware HardwareKeySource = iota // 硬件指纹派生
	KeySourceTPM                               // TPM 模块
	KeySourceFailSafe                          // Fail-Safe 模式（锁定）
)

// HardwareKeyInfo 硬件密钥信息
type HardwareKeyInfo struct {
	Key       []byte
	Source    HardwareKeySource
	Entropy   int // 熵值（字节数）
	Timestamp int64
}

// DeriveHardwareKey 从硬件指纹派生密钥（仅存内存）
// 安全策略：硬件指纹不足时进入 Fail-Safe 锁定模式，而非生成随机数
func DeriveHardwareKey() ([]byte, error) {
	info, err := DeriveHardwareKeyWithInfo()
	if err != nil {
		return nil, err
	}
	return info.Key, nil
}

// DeriveHardwareKeyWithInfo 派生硬件密钥并返回详细信息
func DeriveHardwareKeyWithInfo() (*HardwareKeyInfo, error) {
	var fingerprint []byte
	var sources []string

	// 1. 尝试从 TPM 读取（最高优先级）
	tpmKey, err := readTPMKey()
	if err == nil && len(tpmKey) >= 32 {
		hash := sha256.Sum256(tpmKey)
		return &HardwareKeyInfo{
			Key:       hash[:],
			Source:    KeySourceTPM,
			Entropy:   len(tpmKey),
			Timestamp: time.Now().Unix(),
		}, nil
	}

	// 2. 收集硬件指纹（多源聚合）
	
	// 2.1 CPU ID / Product UUID
	if cpuID, err := os.ReadFile("/sys/class/dmi/id/product_uuid"); err == nil && len(cpuID) > 0 {
		fingerprint = append(fingerprint, cpuID...)
		sources = append(sources, "product_uuid")
	}

	// 2.2 主板序列号
	if boardSerial, err := os.ReadFile("/sys/class/dmi/id/board_serial"); err == nil && len(boardSerial) > 0 {
		fingerprint = append(fingerprint, boardSerial...)
		sources = append(sources, "board_serial")
	}

	// 2.3 BIOS UUID
	if biosUUID, err := os.ReadFile("/sys/class/dmi/id/product_serial"); err == nil && len(biosUUID) > 0 {
		fingerprint = append(fingerprint, biosUUID...)
		sources = append(sources, "product_serial")
	}

	// 2.4 多网卡 MAC 地址
	netInterfaces := []string{"eth0", "eth1", "ens3", "ens4", "enp0s3", "bond0"}
	for _, iface := range netInterfaces {
		if macAddr, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/address", iface)); err == nil && len(macAddr) > 0 {
			fingerprint = append(fingerprint, macAddr...)
			sources = append(sources, "mac_"+iface)
		}
	}

	// 2.5 机器 ID
	if machineID, err := os.ReadFile("/etc/machine-id"); err == nil && len(machineID) > 0 {
		fingerprint = append(fingerprint, machineID...)
		sources = append(sources, "machine_id")
	}

	// 2.6 Boot ID（每次启动唯一，增加熵）
	if bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil && len(bootID) > 0 {
		fingerprint = append(fingerprint, bootID...)
		sources = append(sources, "boot_id")
	}

	// 2.7 磁盘序列号
	if diskSerial, err := os.ReadFile("/sys/block/sda/device/serial"); err == nil && len(diskSerial) > 0 {
		fingerprint = append(fingerprint, diskSerial...)
		sources = append(sources, "disk_serial")
	}

	// 3. 安全检查：熵值必须足够
	// 要求至少 32 字节且来自 2 个以上独立源
	minEntropy := 32
	minSources := 2

	if len(fingerprint) < minEntropy || len(sources) < minSources {
		// ⚠️ 关键安全决策：不生成随机数，进入 Fail-Safe 模式
		return &HardwareKeyInfo{
			Key:       nil,
			Source:    KeySourceFailSafe,
			Entropy:   len(fingerprint),
			Timestamp: time.Now().Unix(),
		}, ErrHardwareFingerprintInsufficient
	}

	// 4. 多轮哈希派生（HKDF 简化版）
	// 增加盐值防止彩虹表攻击
	salt := []byte("mirage-vault-key-derivation-v1")
	saltedFingerprint := append(salt, fingerprint...)

	hash1 := sha256.Sum256(saltedFingerprint)
	hash2 := sha256.Sum256(hash1[:])
	hash3 := sha256.Sum256(hash2[:])

	return &HardwareKeyInfo{
		Key:       hash3[:],
		Source:    KeySourceHardware,
		Entropy:   len(fingerprint),
		Timestamp: time.Now().Unix(),
	}, nil
}

// readTPMKey 尝试从 TPM 读取密钥
func readTPMKey() ([]byte, error) {
	// 尝试 TPM 2.0 设备
	tpmPaths := []string{"/dev/tpm0", "/dev/tpmrm0"}
	
	for _, path := range tpmPaths {
		if _, err := os.Stat(path); err == nil {
			// TPM 存在，尝试读取 PCR 值作为密钥种子
			// 注意：完整实现需要使用 go-tpm 库
			// 这里仅检测 TPM 存在性
			pcrData, err := os.ReadFile("/sys/class/tpm/tpm0/pcr-sha256/0")
			if err == nil && len(pcrData) > 0 {
				return pcrData, nil
			}
		}
	}
	
	return nil, errors.New("TPM 不可用")
}

// NewVaultStorage 创建保险箱存储
func NewVaultStorage(dbPath string) (*VaultStorage, error) {
	// 从硬件派生密钥
	keyInfo, err := DeriveHardwareKeyWithInfo()

	// 检查是否进入 Fail-Safe 模式
	if err == ErrHardwareFingerprintInsufficient {
		return &VaultStorage{
			dbPath:   dbPath,
			keyInfo:  keyInfo,
			locked:   true,
			failSafe: true,
		}, fmt.Errorf("⚠️ 硬件指纹不足（熵值: %d 字节），系统进入 Fail-Safe 锁定模式", keyInfo.Entropy)
	}

	if err != nil {
		return nil, fmt.Errorf("硬件密钥派生失败: %w", err)
	}

	// 创建 AES-GCM
	block, err := aes.NewCipher(keyInfo.Key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 打开数据库
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}

	// 初始化 Buckets
	err = db.Update(func(tx *bolt.Tx) error {
		buckets := []string{BucketDNA, BucketIPReputation, BucketSystem}
		for _, name := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	vs := &VaultStorage{
		db:          db,
		dbPath:      dbPath,
		gcm:         gcm,
		masterKey:   keyInfo.Key,
		keyInfo:     keyInfo,
		locked:      false,
		failSafe:    false,
		ipCacheMap:  make(map[string]*ipCacheEntry),
		ipLRUList:   list.New(),
		writeCh:     make(chan writeOp, WriteBufferSize),
		flushCtx:    ctx,
		flushCancel: cancel,
	}

	// 从 BoltDB 加载数据到内存（带完整性校验）
	if err := vs.loadToMemory(); err != nil {
		db.Close()
		cancel()
		return nil, fmt.Errorf("加载数据到内存失败: %w", err)
	}

	// 启动异步刷盘 goroutine
	vs.flushWg.Add(1)
	go vs.flushLoop()

	return vs, nil
}

// loadToMemory 启动时从 BoltDB 加载所有数据到内存（带完整性校验）
func (vs *VaultStorage) loadToMemory() error {
	var corruptedDNA, corruptedIP int

	err := vs.db.View(func(tx *bolt.Tx) error {
		// 加载 DNA 模板（带 Checksum 校验）
		dnaBucket := tx.Bucket([]byte(BucketDNA))
		if dnaBucket != nil {
			dnaBucket.ForEach(func(k, v []byte) error {
				data, err := vs.decrypt(v)
				if err != nil {
					corruptedDNA++
					return nil // 跳过解密失败
				}
				var record DNARecord
				if err := json.Unmarshal(data, &record); err != nil {
					corruptedDNA++
					return nil
				}
				// 完整性校验
				expectedChecksum := vs.calculateDNAChecksum(&record)
				if record.Checksum != "" && record.Checksum != expectedChecksum {
					corruptedDNA++
					return nil // 跳过校验失败的记录
				}
				vs.dnaCache.Store(string(k), &record)
				return nil
			})
		}

		// 加载 IP 信誉（带数据完整性校验）
		ipBucket := tx.Bucket([]byte(BucketIPReputation))
		if ipBucket != nil {
			ipBucket.ForEach(func(k, v []byte) error {
				data, err := vs.decrypt(v)
				if err != nil {
					corruptedIP++
					return nil
				}
				var record IPReputationRecord
				if err := json.Unmarshal(data, &record); err != nil {
					corruptedIP++
					return nil
				}
				// 基本完整性校验
				if record.IP == "" || record.IP != string(k) {
					corruptedIP++
					return nil
				}
				vs.ipCacheAdd(&record)
				return nil
			})
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 记录损坏数据（不阻止启动，但记录日志）
	if corruptedDNA > 0 || corruptedIP > 0 {
		// 可通过日志系统记录
		_ = corruptedDNA
		_ = corruptedIP
	}

	return nil
}

// ipCacheAdd 添加 IP 到缓存（内部方法，需持有锁或初始化时调用）
func (vs *VaultStorage) ipCacheAdd(record *IPReputationRecord) {
	vs.ipCacheMu.Lock()
	defer vs.ipCacheMu.Unlock()

	// 检查是否已存在
	if entry, exists := vs.ipCacheMap[record.IP]; exists {
		// 更新记录并移到链表头部
		entry.record = record
		vs.ipLRUList.MoveToFront(entry.element)
		return
	}

	// 新增条目
	element := vs.ipLRUList.PushFront(record.IP)
	vs.ipCacheMap[record.IP] = &ipCacheEntry{
		record:  record,
		element: element,
	}
	vs.ipCacheSize.Add(1)

	// 检查是否需要淘汰
	if vs.ipCacheSize.Load() > MaxIPCacheSize {
		vs.evictLRU()
	}
}

// evictLRU 淘汰低信誉且长期未活跃的 IP（需持有写锁）
func (vs *VaultStorage) evictLRU() {
	// 收集候选淘汰项
	type evictCandidate struct {
		ip    string
		score float64
		ts    int64
	}

	candidates := make([]evictCandidate, 0, EvictBatchSize*2)
	now := time.Now().Unix()
	inactiveThreshold := now - 86400*7 // 7 天未活跃

	for ip, entry := range vs.ipCacheMap {
		// 优先淘汰：信誉分 < 30 且 7 天未活跃
		if entry.record.ReputationScore < 30 && entry.record.LastSeenTS < inactiveThreshold {
			candidates = append(candidates, evictCandidate{
				ip:    ip,
				score: entry.record.ReputationScore,
				ts:    entry.record.LastSeenTS,
			})
		}
	}

	// 按信誉分升序排序（优先淘汰低分）
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score < candidates[j].score
		}
		return candidates[i].ts < candidates[j].ts
	})

	// 淘汰
	evicted := 0
	for _, c := range candidates {
		if evicted >= EvictBatchSize {
			break
		}
		if entry, exists := vs.ipCacheMap[c.ip]; exists {
			vs.ipLRUList.Remove(entry.element)
			delete(vs.ipCacheMap, c.ip)
			vs.ipCacheSize.Add(-1)
			evicted++
		}
	}

	vs.evictCount.Add(int64(evicted))
}

// flushLoop 异步刷盘循环（带版本戳保护）
func (vs *VaultStorage) flushLoop() {
	defer vs.flushWg.Done()

	ticker := time.NewTicker(FlushInterval)
	defer ticker.Stop()

	batch := make([]writeOp, 0, FlushBatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}

		// 批量写入 BoltDB（带版本戳检查）
		vs.db.Update(func(tx *bolt.Tx) error {
			for _, op := range batch {
				// 检查版本戳：只写入最新版本
				if pendingVer, ok := vs.pendingWrites.Load(op.bucket + ":" + op.key); ok {
					if op.version < pendingVer.(int64) {
						// 跳过旧版本
						continue
					}
				}

				b := tx.Bucket([]byte(op.bucket))
				if b != nil {
					b.Put([]byte(op.key), op.value)
				}

				// 清除待刷盘标记
				vs.pendingWrites.Delete(op.bucket + ":" + op.key)
			}
			return nil
		})

		vs.dirty.Add(-int64(len(batch)))
		batch = batch[:0]
	}

	for {
		select {
		case <-vs.flushCtx.Done():
			// 关闭前刷盘
			flush()
			// 处理剩余
			for {
				select {
				case op := <-vs.writeCh:
					batch = append(batch, op)
					if len(batch) >= FlushBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}

		case op := <-vs.writeCh:
			batch = append(batch, op)
			if len(batch) >= FlushBatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// Flush 强制刷盘（Kill Switch 前调用）
func (vs *VaultStorage) Flush() {
	// 等待写缓冲区清空
	for vs.dirty.Load() > 0 {
		time.Sleep(10 * time.Millisecond)
	}
}

// IsFailSafe 检查是否处于 Fail-Safe 模式
func (vs *VaultStorage) IsFailSafe() bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.failSafe
}

// GetKeyInfo 获取密钥信息（用于诊断）
func (vs *VaultStorage) GetKeyInfo() *HardwareKeyInfo {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.keyInfo
}

// encrypt 加密数据
func (vs *VaultStorage) encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, vs.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return vs.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt 解密数据
func (vs *VaultStorage) decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < vs.gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:vs.gcm.NonceSize()], ciphertext[vs.gcm.NonceSize():]
	return vs.gcm.Open(nil, nonce, ciphertext, nil)
}

// --- DNA 操作 ---

// SaveDNA 保存 DNA 指纹（内存优先 + 异步刷盘）
func (vs *VaultStorage) SaveDNA(record *DNARecord) error {
	record.UpdatedAt = time.Now().Unix()
	record.Checksum = vs.calculateDNAChecksum(record)

	// 1. 更新内存缓存
	vs.dnaCache.Store(record.ProfileID, record)

	// 2. 异步写入 BoltDB
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	encrypted, err := vs.encrypt(data)
	if err != nil {
		return err
	}

	vs.dirty.Add(1)
	vs.writeCh <- writeOp{
		bucket: BucketDNA,
		key:    record.ProfileID,
		value:  encrypted,
	}

	return nil
}

// GetDNA 获取 DNA 指纹（100% 内存命中）
func (vs *VaultStorage) GetDNA(profileID string) (*DNARecord, error) {
	if v, ok := vs.dnaCache.Load(profileID); ok {
		return v.(*DNARecord), nil
	}
	return nil, errors.New("DNA not found")
}

// ListDNA 列出所有 DNA（从内存读取）
func (vs *VaultStorage) ListDNA() ([]*DNARecord, error) {
	var records []*DNARecord
	vs.dnaCache.Range(func(key, value any) bool {
		records = append(records, value.(*DNARecord))
		return true
	})
	return records, nil
}

// calculateDNAChecksum 计算 DNA 校验和
func (vs *VaultStorage) calculateDNAChecksum(r *DNARecord) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%d|%d",
		r.ProfileID, r.JA4, r.Browser, r.OS, r.TCPWindow, r.TTL)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// --- IP 信誉操作 ---

// SaveIPReputation 保存 IP 信誉（内存优先 + 异步刷盘 + 版本戳）
func (vs *VaultStorage) SaveIPReputation(record *IPReputationRecord) error {
	record.LastSeenTS = time.Now().Unix()
	record.Version++ // 递增版本戳

	// 1. 更新内存缓存（带 LRU）
	vs.ipCacheAdd(record)

	// 2. 异步写入 BoltDB
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	encrypted, err := vs.encrypt(data)
	if err != nil {
		return err
	}

	// 记录待刷盘的最新版本
	key := BucketIPReputation + ":" + record.IP
	vs.pendingWrites.Store(key, record.Version)

	vs.dirty.Add(1)
	vs.writeCh <- writeOp{
		bucket:  BucketIPReputation,
		key:     record.IP,
		value:   encrypted,
		version: record.Version,
	}

	return nil
}

// GetIPReputation 获取 IP 信誉（100% 内存命中）
func (vs *VaultStorage) GetIPReputation(ip string) (*IPReputationRecord, error) {
	vs.readTotal.Add(1)

	vs.ipCacheMu.RLock()
	entry, exists := vs.ipCacheMap[ip]
	vs.ipCacheMu.RUnlock()

	if exists {
		vs.readHits.Add(1)
		// 更新 LRU（移到头部）
		vs.ipCacheMu.Lock()
		vs.ipLRUList.MoveToFront(entry.element)
		vs.ipCacheMu.Unlock()
		return entry.record, nil
	}
	return nil, errors.New("IP not found")
}

// UpdateIPMetrics 更新 IP 指标（内存优先）
func (vs *VaultStorage) UpdateIPMetrics(ip string, latency float64, success bool) error {
	// 从内存获取或创建
	var record *IPReputationRecord

	vs.ipCacheMu.RLock()
	entry, exists := vs.ipCacheMap[ip]
	vs.ipCacheMu.RUnlock()

	if exists {
		record = entry.record
	} else {
		record = &IPReputationRecord{IP: ip, Version: 0}
	}

	// 更新指标
	record.Latency = (record.Latency + latency) / 2
	record.LastSeenTS = time.Now().Unix()

	if success {
		record.SuccessCount++
		record.ReputationScore = min(100, record.ReputationScore+1)
	} else {
		record.FailCount++
		record.ReputationScore = max(0, record.ReputationScore-5)
	}

	// 保存（内存 + 异步刷盘）
	return vs.SaveIPReputation(record)
}

// GetTopIPs 获取高信誉 IP 列表（从内存读取）
func (vs *VaultStorage) GetTopIPs(limit int) ([]*IPReputationRecord, error) {
	vs.ipCacheMu.RLock()
	defer vs.ipCacheMu.RUnlock()

	records := make([]*IPReputationRecord, 0, len(vs.ipCacheMap))
	for _, entry := range vs.ipCacheMap {
		records = append(records, entry.record)
	}

	// 按信誉分降序排序
	sort.Slice(records, func(i, j int) bool {
		return records[i].ReputationScore > records[j].ReputationScore
	})

	if len(records) > limit {
		records = records[:limit]
	}

	return records, nil
}


// --- 系统状态操作 ---

// SetSystemState 设置系统状态
func (vs *VaultStorage) SetSystemState(state SystemState) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	return vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketSystem))
		return b.Put([]byte(KeyKillSwitchState), []byte(state))
	})
}

// GetSystemState 获取系统状态
func (vs *VaultStorage) GetSystemState() SystemState {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	var state SystemState = StateRunning
	vs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketSystem))
		data := b.Get([]byte(KeyKillSwitchState))
		if data != nil {
			state = SystemState(data)
		}
		return nil
	})

	return state
}

// SetLastActiveDomain 设置最后活跃域名
func (vs *VaultStorage) SetLastActiveDomain(domain string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	return vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketSystem))
		return b.Put([]byte(KeyLastActiveDomain), []byte(domain))
	})
}

// GetLastActiveDomain 获取最后活跃域名
func (vs *VaultStorage) GetLastActiveDomain() string {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	var domain string
	vs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketSystem))
		data := b.Get([]byte(KeyLastActiveDomain))
		if data != nil {
			domain = string(data)
		}
		return nil
	})

	return domain
}

// RecordBootTime 记录启动时间
func (vs *VaultStorage) RecordBootTime() error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	return vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketSystem))
		ts := fmt.Sprintf("%d", time.Now().Unix())
		return b.Put([]byte(KeyLastBootTime), []byte(ts))
	})
}

// IsVaultLocked 检查保险箱是否锁定
func (vs *VaultStorage) IsVaultLocked() bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.locked
}

// Lock 锁定保险箱
func (vs *VaultStorage) Lock() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.locked = true
}

// Unlock 解锁保险箱（需要重新派生密钥）
func (vs *VaultStorage) Unlock() error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// 重新派生密钥验证
	newKey, err := DeriveHardwareKey()
	if err != nil {
		return err
	}

	// 验证密钥一致性
	if hex.EncodeToString(newKey) != hex.EncodeToString(vs.masterKey) {
		return errors.New("硬件指纹不匹配，解锁失败")
	}

	vs.locked = false
	return nil
}

// --- 自毁与清理 ---

// PhysicalWipe 物理抹除（Kill Switch 调用）
func (vs *VaultStorage) PhysicalWipe() error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// 0. 停止异步刷盘
	vs.flushCancel()
	vs.flushWg.Wait()

	// 1. 安全清空 IP 缓存（逐条擦除敏感数据）
	vs.ipCacheMu.Lock()
	for ip, entry := range vs.ipCacheMap {
		// 擦除记录内容
		entry.record.IP = ""
		entry.record.ReputationScore = 0
		entry.record.Region = ""
		entry.record.Provider = ""
		vs.ipLRUList.Remove(entry.element)
		delete(vs.ipCacheMap, ip)
	}
	vs.ipCacheMu.Unlock()

	// 2. 安全清空 DNA 缓存（擦除敏感指纹数据）
	vs.dnaCache.Range(func(key, value any) bool {
		record := value.(*DNARecord)
		// 擦除敏感字段
		record.JA4 = ""
		record.ProfileID = ""
		for k := range record.Headers {
			record.Headers[k] = ""
		}
		vs.dnaCache.Delete(key)
		return true
	})

	// 3. 强制 GC 回收内存
	runtime.GC()

	// 4. 设置状态为 DEAD
	vs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketSystem))
		return b.Put([]byte(KeyKillSwitchState), []byte(StateDead))
	})

	// 5. 关闭数据库
	vs.db.Close()

	// 6. 三次覆盖
	patterns := []byte{0x00, 0xFF, 0xAA}
	for _, pattern := range patterns {
		vs.overwriteFile(pattern)
	}

	// 7. 删除文件
	os.Remove(vs.dbPath)

	// 8. 安全清除内存密钥
	for i := range vs.masterKey {
		vs.masterKey[i] = 0
	}

	// 9. 再次 GC
	runtime.GC()

	return nil
}

// overwriteFile 覆盖文件
func (vs *VaultStorage) overwriteFile(pattern byte) error {
	info, err := os.Stat(vs.dbPath)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(vs.dbPath, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	data := make([]byte, 4096)
	for i := range data {
		data[i] = pattern
	}

	size := info.Size()
	for written := int64(0); written < size; written += 4096 {
		f.Write(data)
	}

	return f.Sync()
}

// Close 关闭存储
func (vs *VaultStorage) Close() error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// 停止异步刷盘
	vs.flushCancel()
	vs.flushWg.Wait()

	// 清除密钥
	for i := range vs.masterKey {
		vs.masterKey[i] = 0
	}

	return vs.db.Close()
}

// GetStats 获取存储统计（用于 UI 展示）
func (vs *VaultStorage) GetStats() map[string]any {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	stats := make(map[string]any)

	// 文件大小
	if info, err := os.Stat(vs.dbPath); err == nil {
		stats["file_size"] = info.Size()
	}

	// IP 缓存统计
	vs.ipCacheMu.RLock()
	stats["ip_cache_count"] = len(vs.ipCacheMap)
	stats["ip_cache_max"] = MaxIPCacheSize
	vs.ipCacheMu.RUnlock()

	// DNA 缓存统计
	dnaCount := 0
	vs.dnaCache.Range(func(key, value any) bool {
		dnaCount++
		return true
	})
	stats["dna_cache_count"] = dnaCount

	// 脏数据计数（待刷盘队列）
	stats["dirty_count"] = vs.dirty.Load()

	// 缓存命中率
	total := vs.readTotal.Load()
	hits := vs.readHits.Load()
	if total > 0 {
		stats["cache_hit_rate"] = float64(hits) / float64(total) * 100
	} else {
		stats["cache_hit_rate"] = 100.0
	}
	stats["read_hits"] = hits
	stats["read_total"] = total

	// 淘汰统计
	stats["evict_count"] = vs.evictCount.Load()

	stats["locked"] = vs.locked
	stats["state"] = vs.GetSystemState()

	return stats
}

// MemoryMetrics 内存指标（用于 GlobalTacticalHUD）
type MemoryMetrics struct {
	IPCacheCount   int     `json:"ip_cache_count"`
	IPCacheMax     int     `json:"ip_cache_max"`
	DNACacheCount  int     `json:"dna_cache_count"`
	DirtyCount     int64   `json:"dirty_count"`
	CacheHitRate   float64 `json:"cache_hit_rate"`
	ReadHits       int64   `json:"read_hits"`
	ReadTotal      int64   `json:"read_total"`
	EvictCount     int64   `json:"evict_count"`
	FileSizeBytes  int64   `json:"file_size_bytes"`
}

// GetMemoryMetrics 获取内存指标（用于 UI 可视化）
func (vs *VaultStorage) GetMemoryMetrics() *MemoryMetrics {
	vs.ipCacheMu.RLock()
	ipCount := len(vs.ipCacheMap)
	vs.ipCacheMu.RUnlock()

	dnaCount := 0
	vs.dnaCache.Range(func(key, value any) bool {
		dnaCount++
		return true
	})

	total := vs.readTotal.Load()
	hits := vs.readHits.Load()
	hitRate := 100.0
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	var fileSize int64
	if info, err := os.Stat(vs.dbPath); err == nil {
		fileSize = info.Size()
	}

	return &MemoryMetrics{
		IPCacheCount:  ipCount,
		IPCacheMax:    MaxIPCacheSize,
		DNACacheCount: dnaCount,
		DirtyCount:    vs.dirty.Load(),
		CacheHitRate:  hitRate,
		ReadHits:      hits,
		ReadTotal:     total,
		EvictCount:    vs.evictCount.Load(),
		FileSizeBytes: fileSize,
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
