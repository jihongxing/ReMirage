// 去中心化加密数据库引擎 - BoltDB + AES-256-GCM
package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	BucketUsers        = "users"
	BucketTransactions = "transactions"
	BucketIntelAssets  = "intel_assets"
)

type EncryptedStorage struct {
	mu       sync.RWMutex
	db       *bolt.DB
	dbPath   string
	gcm      cipher.AEAD
	masterKey []byte
}

// AnonymousUser 匿名用户表
type AnonymousUser struct {
	UID          string `json:"uid"`           // SHA3-256 哈希，不可逆
	BalanceQuota uint64 `json:"balance_quota"` // 剩余配额
	LastSeenDay  int64  `json:"last_seen_day"` // 仅保留天级时间戳
	CreditLevel  int    `json:"credit_level"`
}

// AnonymousTransaction 匿名充值表
type AnonymousTransaction struct {
	ID        string `json:"id"`
	UID       string `json:"uid"`
	TxHash    string `json:"tx_hash,omitempty"` // 确认后抹除
	Amount    uint64 `json:"amount"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"` // 模糊化时间戳
}

// IntelAsset 环境知识库
type IntelAsset struct {
	IPGeoCache           map[string]string `json:"ip_geo_cache"`
	FingerprintTemplates []string          `json:"fingerprint_templates"`
	NodeHealth           map[string]int    `json:"node_health"`
}

// NewEncryptedStorage 创建加密存储引擎
func NewEncryptedStorage(dbPath string, masterKey []byte) (*EncryptedStorage, error) {
	// 派生 AES 密钥
	keyHash := sha256.Sum256(masterKey)
	
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		return nil, err
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	
	// 初始化 Buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range []string{BucketUsers, BucketTransactions, BucketIntelAssets} {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	
	return &EncryptedStorage{
		db:        db,
		dbPath:    dbPath,
		gcm:       gcm,
		masterKey: keyHash[:],
	}, nil
}

// encrypt 加密数据
func (s *EncryptedStorage) encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt 解密数据
func (s *EncryptedStorage) decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < s.gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:s.gcm.NonceSize()], ciphertext[s.gcm.NonceSize():]
	return s.gcm.Open(nil, nonce, ciphertext, nil)
}

// AnonymizeTimestamp 时间戳模糊化（±12小时随机偏移）
func AnonymizeTimestamp(t time.Time) int64 {
	// 随机偏移 ±12 小时
	maxOffset := int64(12 * 3600) // 12 hours in seconds
	offset, _ := rand.Int(rand.Reader, big.NewInt(maxOffset*2))
	jitter := offset.Int64() - maxOffset
	return t.Unix() + jitter
}

// ToDayTimestamp 转换为天级时间戳（去除时分秒）
func ToDayTimestamp(t time.Time) int64 {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Unix()
}

// GenerateAnonymousUID 从公钥生成匿名 UID（不可逆）
func GenerateAnonymousUID(pubKey []byte) string {
	// 双重哈希
	first := sha256.Sum256(pubKey)
	second := sha256.Sum256(first[:])
	return hex.EncodeToString(second[:12])
}

// SaveUser 保存用户（加密存储）
func (s *EncryptedStorage) SaveUser(user *AnonymousUser) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 强制天级时间戳
	user.LastSeenDay = ToDayTimestamp(time.Now())
	
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	
	encrypted, err := s.encrypt(data)
	if err != nil {
		return err
	}
	
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketUsers))
		return b.Put([]byte(user.UID), encrypted)
	})
}

// GetUser 获取用户
func (s *EncryptedStorage) GetUser(uid string) (*AnonymousUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var user AnonymousUser
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketUsers))
		encrypted := b.Get([]byte(uid))
		if encrypted == nil {
			return errors.New("user not found")
		}
		
		data, err := s.decrypt(encrypted)
		if err != nil {
			return err
		}
		
		return json.Unmarshal(data, &user)
	})
	
	return &user, err
}

// SaveTransaction 保存交易（时间戳模糊化）
func (s *EncryptedStorage) SaveTransaction(tx *AnonymousTransaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 时间戳模糊化
	tx.Timestamp = AnonymizeTimestamp(time.Now())
	
	data, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	
	encrypted, err := s.encrypt(data)
	if err != nil {
		return err
	}
	
	return s.db.Update(func(dbTx *bolt.Tx) error {
		b := dbTx.Bucket([]byte(BucketTransactions))
		return b.Put([]byte(tx.ID), encrypted)
	})
}

// PruneTxHash 确认后抹除 TxHash（防关联审查）
func (s *EncryptedStorage) PruneTxHash(txID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	return s.db.Update(func(dbTx *bolt.Tx) error {
		b := dbTx.Bucket([]byte(BucketTransactions))
		encrypted := b.Get([]byte(txID))
		if encrypted == nil {
			return nil
		}
		
		data, err := s.decrypt(encrypted)
		if err != nil {
			return err
		}
		
		var tx AnonymousTransaction
		if err := json.Unmarshal(data, &tx); err != nil {
			return err
		}
		
		// 抹除 TxHash
		tx.TxHash = ""
		
		newData, _ := json.Marshal(tx)
		newEncrypted, _ := s.encrypt(newData)
		
		return b.Put([]byte(txID), newEncrypted)
	})
}

// WipeUser 物理覆盖删除用户（安全擦除）
func (s *EncryptedStorage) WipeUser(uid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketUsers))
		
		// 先用随机数据覆盖
		randomData := make([]byte, 256)
		rand.Read(randomData)
		if err := b.Put([]byte(uid), randomData); err != nil {
			return err
		}
		
		// 再用 0x00 覆盖
		zeroData := make([]byte, 256)
		if err := b.Put([]byte(uid), zeroData); err != nil {
			return err
		}
		
		// 最后删除
		return b.Delete([]byte(uid))
	})
}

// WipeOnDestruct 自毁时安全擦除整个数据库
func (s *EncryptedStorage) WipeOnDestruct() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 关闭数据库
	if err := s.db.Close(); err != nil {
		return err
	}
	
	// 三次覆盖
	patterns := []byte{0x00, 0xFF, 0xAA}
	
	for _, pattern := range patterns {
		if err := s.overwriteFile(pattern); err != nil {
			continue
		}
	}
	
	// 删除文件
	return os.Remove(s.dbPath)
}

// overwriteFile 用指定字节覆盖文件
func (s *EncryptedStorage) overwriteFile(pattern byte) error {
	info, err := os.Stat(s.dbPath)
	if err != nil {
		return err
	}
	
	f, err := os.OpenFile(s.dbPath, os.O_WRONLY, 0600)
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

// SaveIntelAsset 保存环境知识库
func (s *EncryptedStorage) SaveIntelAsset(key string, asset *IntelAsset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	data, err := json.Marshal(asset)
	if err != nil {
		return err
	}
	
	encrypted, err := s.encrypt(data)
	if err != nil {
		return err
	}
	
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketIntelAssets))
		return b.Put([]byte(key), encrypted)
	})
}

// UpdateIPGeoCache 更新 IP 地理缓存
func (s *EncryptedStorage) UpdateIPGeoCache(ip, geo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketIntelAssets))
		key := []byte("ip_geo:" + ip)
		return b.Put(key, []byte(geo))
	})
}

// GetIPGeo 获取 IP 地理位置
func (s *EncryptedStorage) GetIPGeo(ip string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	var geo string
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketIntelAssets))
		data := b.Get([]byte("ip_geo:" + ip))
		if data != nil {
			geo = string(data)
		}
		return nil
	})
	return geo
}

// Close 关闭数据库
func (s *EncryptedStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 清除内存中的密钥
	for i := range s.masterKey {
		s.masterKey[i] = 0
	}
	
	return s.db.Close()
}

// UpdateUserQuota 更新用户配额
func (s *EncryptedStorage) UpdateUserQuota(uid string, delta int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketUsers))
		encrypted := b.Get([]byte(uid))
		if encrypted == nil {
			return errors.New("user not found")
		}
		
		data, err := s.decrypt(encrypted)
		if err != nil {
			return err
		}
		
		var user AnonymousUser
		if err := json.Unmarshal(data, &user); err != nil {
			return err
		}
		
		// 更新配额
		if delta > 0 {
			user.BalanceQuota += uint64(delta)
		} else if uint64(-delta) <= user.BalanceQuota {
			user.BalanceQuota -= uint64(-delta)
		} else {
			user.BalanceQuota = 0
		}
		
		// 更新天级时间戳
		user.LastSeenDay = ToDayTimestamp(time.Now())
		
		newData, _ := json.Marshal(user)
		newEncrypted, _ := s.encrypt(newData)
		
		return b.Put([]byte(uid), newEncrypted)
	})
}
