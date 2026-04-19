// Package phantom 影子数据库伪造器
// 生成可无限挖掘的虚假业务数据
package phantom

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// DBFaker 数据库伪造器
type DBFaker struct {
	mu sync.RWMutex

	// 种子（确保同一 UID 看到一致数据）
	seeds map[string]int64

	// 陷阱触发记录
	trapTriggers map[string]*TrapTrigger

	// 配置
	config FakerConfig

	// 统计
	stats FakerStats

	// 回调
	onTrapTriggered func(uid string, trapType string, payload string)
}

// FakerConfig 配置
type FakerConfig struct {
	MaxRecordsPerQuery int
	TrapProbability    float32
	EnableOverflowProbe bool
}

// TrapTrigger 陷阱触发记录
type TrapTrigger struct {
	UID       string
	TrapType  string
	Timestamp time.Time
	Payload   string
	ClientInfo string
}

// FakerStats 统计
type FakerStats struct {
	TotalQueries      int64
	TotalRecords      int64
	TrapsTriggered    int64
	OverflowsDetected int64
	UniqueUIDs        int
}

// NewDBFaker 创建伪造器
func NewDBFaker(config FakerConfig) *DBFaker {
	if config.MaxRecordsPerQuery == 0 {
		config.MaxRecordsPerQuery = 1000
	}
	if config.TrapProbability == 0 {
		config.TrapProbability = 0.05
	}

	return &DBFaker{
		seeds:        make(map[string]int64),
		trapTriggers: make(map[string]*TrapTrigger),
		config:       config,
	}
}

// getSeed 获取 UID 对应的种子
func (df *DBFaker) getSeed(uid string) int64 {
	df.mu.Lock()
	defer df.mu.Unlock()

	if seed, exists := df.seeds[uid]; exists {
		return seed
	}

	seed := time.Now().UnixNano()
	df.seeds[uid] = seed
	df.stats.UniqueUIDs = len(df.seeds)
	return seed
}

// GenerateUsers 生成虚假用户数据
func (df *DBFaker) GenerateUsers(uid string, offset, limit int) []FakeUser {
	seed := df.getSeed(uid)
	rng := rand.New(rand.NewSource(seed + int64(offset)))

	df.mu.Lock()
	df.stats.TotalQueries++
	df.mu.Unlock()

	if limit > df.config.MaxRecordsPerQuery {
		limit = df.config.MaxRecordsPerQuery
	}

	users := make([]FakeUser, limit)
	for i := 0; i < limit; i++ {
		users[i] = df.generateUser(rng, offset+i)
	}

	df.mu.Lock()
	df.stats.TotalRecords += int64(limit)
	df.mu.Unlock()

	return users
}

// FakeUser 虚假用户
type FakeUser struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	Balance      float64   `json:"balance"`
	Currency     string    `json:"currency"`
	CreatedAt    time.Time `json:"created_at"`
	LastLogin    time.Time `json:"last_login"`
	Status       string    `json:"status"`
	VIPLevel     int       `json:"vip_level"`
	TotalSpent   float64   `json:"total_spent"`
	ReferralCode string    `json:"referral_code"`
}

func (df *DBFaker) generateUser(rng *rand.Rand, id int) FakeUser {
	firstNames := []string{"James", "Michael", "Robert", "David", "William", "Richard", "Joseph", "Thomas", "Christopher", "Charles", "Emma", "Olivia", "Ava", "Isabella", "Sophia", "Mia", "Charlotte", "Amelia", "Harper", "Evelyn"}
	lastNames := []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas", "Taylor", "Moore", "Jackson", "Martin"}
	domains := []string{"gmail.com", "yahoo.com", "outlook.com", "protonmail.com", "icloud.com", "hotmail.com"}
	currencies := []string{"USD", "EUR", "GBP", "SGD", "CHF", "JPY"}
	statuses := []string{"active", "active", "active", "suspended", "pending"}

	firstName := firstNames[rng.Intn(len(firstNames))]
	lastName := lastNames[rng.Intn(len(lastNames))]
	domain := domains[rng.Intn(len(domains))]

	createdAt := time.Now().AddDate(0, 0, -rng.Intn(365*3))
	lastLogin := createdAt.Add(time.Duration(rng.Intn(int(time.Since(createdAt).Hours()))) * time.Hour)

	return FakeUser{
		ID:           1000000 + id,
		Username:     fmt.Sprintf("%s%s%d", strings.ToLower(firstName), strings.ToLower(lastName[:3]), rng.Intn(999)),
		Email:        fmt.Sprintf("%s.%s%d@%s", strings.ToLower(firstName), strings.ToLower(lastName), rng.Intn(99), domain),
		Balance:      float64(rng.Intn(100000)) / 100,
		Currency:     currencies[rng.Intn(len(currencies))],
		CreatedAt:    createdAt,
		LastLogin:    lastLogin,
		Status:       statuses[rng.Intn(len(statuses))],
		VIPLevel:     rng.Intn(5),
		TotalSpent:   float64(rng.Intn(1000000)) / 100,
		ReferralCode: df.generateReferralCode(rng),
	}
}

func (df *DBFaker) generateReferralCode(rng *rand.Rand) string {
	chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, 8)
	for i := range code {
		code[i] = chars[rng.Intn(len(chars))]
	}
	return string(code)
}

// GenerateTransactions 生成虚假交易记录
func (df *DBFaker) GenerateTransactions(uid string, offset, limit int) []FakeTransaction {
	seed := df.getSeed(uid)
	rng := rand.New(rand.NewSource(seed + int64(offset)*7))

	df.mu.Lock()
	df.stats.TotalQueries++
	df.mu.Unlock()

	if limit > df.config.MaxRecordsPerQuery {
		limit = df.config.MaxRecordsPerQuery
	}

	txs := make([]FakeTransaction, limit)
	for i := 0; i < limit; i++ {
		txs[i] = df.generateTransaction(rng, offset+i)
	}

	df.mu.Lock()
	df.stats.TotalRecords += int64(limit)
	df.mu.Unlock()

	return txs
}

// FakeTransaction 虚假交易
type FakeTransaction struct {
	ID          string    `json:"id"`
	UserID      int       `json:"user_id"`
	Type        string    `json:"type"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	Gateway     string    `json:"gateway"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	Metadata    string    `json:"metadata"`
}

func (df *DBFaker) generateTransaction(rng *rand.Rand, id int) FakeTransaction {
	types := []string{"deposit", "withdrawal", "transfer", "subscription", "refund"}
	statuses := []string{"completed", "completed", "completed", "pending", "failed"}
	gateways := []string{"stripe", "paypal", "crypto_btc", "crypto_xmr", "wire_transfer"}
	currencies := []string{"USD", "EUR", "BTC", "XMR", "USDT"}

	txType := types[rng.Intn(len(types))]
	amount := float64(rng.Intn(100000)+100) / 100

	hash := md5.Sum([]byte(fmt.Sprintf("%d-%d", id, rng.Int63())))
	txID := hex.EncodeToString(hash[:])

	return FakeTransaction{
		ID:          txID,
		UserID:      1000000 + rng.Intn(100000),
		Type:        txType,
		Amount:      amount,
		Currency:    currencies[rng.Intn(len(currencies))],
		Status:      statuses[rng.Intn(len(statuses))],
		Gateway:     gateways[rng.Intn(len(gateways))],
		Timestamp:   time.Now().Add(-time.Duration(rng.Intn(365*24)) * time.Hour),
		Description: df.generateDescription(rng, txType),
		Metadata:    df.generateMetadata(rng),
	}
}

func (df *DBFaker) generateDescription(rng *rand.Rand, txType string) string {
	switch txType {
	case "deposit":
		return fmt.Sprintf("Account funding via %s", []string{"bank transfer", "card", "crypto"}[rng.Intn(3)])
	case "withdrawal":
		return fmt.Sprintf("Withdrawal to %s", []string{"bank account", "crypto wallet", "PayPal"}[rng.Intn(3)])
	case "subscription":
		return fmt.Sprintf("Monthly subscription - %s plan", []string{"Basic", "Pro", "Enterprise"}[rng.Intn(3)])
	default:
		return "Transaction processed"
	}
}

func (df *DBFaker) generateMetadata(rng *rand.Rand) string {
	return fmt.Sprintf(`{"ip":"%d.%d.%d.%d","ua":"Mozilla/5.0"}`, rng.Intn(256), rng.Intn(256), rng.Intn(256), rng.Intn(256))
}

// GenerateNodeLinks 生成虚假节点链路数据
func (df *DBFaker) GenerateNodeLinks(uid string, offset, limit int) []FakeNodeLink {
	seed := df.getSeed(uid)
	rng := rand.New(rand.NewSource(seed + int64(offset)*13))

	df.mu.Lock()
	df.stats.TotalQueries++
	df.mu.Unlock()

	if limit > df.config.MaxRecordsPerQuery {
		limit = df.config.MaxRecordsPerQuery
	}

	links := make([]FakeNodeLink, limit)
	for i := 0; i < limit; i++ {
		links[i] = df.generateNodeLink(rng, offset+i)
	}

	df.mu.Lock()
	df.stats.TotalRecords += int64(limit)
	df.mu.Unlock()

	return links
}

// FakeNodeLink 虚假节点链路
type FakeNodeLink struct {
	ID          int       `json:"id"`
	NodeID      string    `json:"node_id"`
	Region      string    `json:"region"`
	IP          string    `json:"ip"`
	Port        int       `json:"port"`
	Protocol    string    `json:"protocol"`
	Bandwidth   int64     `json:"bandwidth_mbps"`
	Latency     int       `json:"latency_ms"`
	Status      string    `json:"status"`
	LastCheck   time.Time `json:"last_check"`
	Uptime      float64   `json:"uptime_percent"`
	TotalTraffic int64    `json:"total_traffic_gb"`
}

func (df *DBFaker) generateNodeLink(rng *rand.Rand, id int) FakeNodeLink {
	regions := []string{"sg-1", "hk-2", "jp-1", "de-1", "ch-1", "is-1", "us-east-1", "us-west-2"}
	protocols := []string{"vmess", "vless", "trojan", "shadowsocks", "wireguard"}
	statuses := []string{"online", "online", "online", "online", "degraded", "offline"}

	region := regions[rng.Intn(len(regions))]
	hash := md5.Sum([]byte(fmt.Sprintf("node-%d-%s", id, region)))

	return FakeNodeLink{
		ID:           id,
		NodeID:       hex.EncodeToString(hash[:8]),
		Region:       region,
		IP:           fmt.Sprintf("%d.%d.%d.%d", 10+rng.Intn(200), rng.Intn(256), rng.Intn(256), rng.Intn(256)),
		Port:         443 + rng.Intn(1000),
		Protocol:     protocols[rng.Intn(len(protocols))],
		Bandwidth:    int64(100 + rng.Intn(900)),
		Latency:      10 + rng.Intn(200),
		Status:       statuses[rng.Intn(len(statuses))],
		LastCheck:    time.Now().Add(-time.Duration(rng.Intn(300)) * time.Second),
		Uptime:       95.0 + rng.Float64()*5,
		TotalTraffic: int64(rng.Intn(10000)),
	}
}

// GenerateTrapData 生成带陷阱的数据
func (df *DBFaker) GenerateTrapData(uid string, dataType string) interface{} {
	seed := df.getSeed(uid)
	rng := rand.New(rand.NewSource(seed))

	// 根据概率决定是否插入陷阱
	if rng.Float32() < df.config.TrapProbability {
		return df.generateTrap(uid, rng, dataType)
	}

	switch dataType {
	case "config":
		return df.generateFakeConfig(rng)
	case "credentials":
		return df.generateFakeCredentials(rng)
	case "backup":
		return df.generateFakeBackup(rng)
	default:
		return nil
	}
}

// TrapData 陷阱数据
type TrapData struct {
	Type        string `json:"type"`
	Content     string `json:"content"`
	HiddenProbe []byte `json:"-"` // 隐藏的溢出探测字节
}

func (df *DBFaker) generateTrap(uid string, rng *rand.Rand, dataType string) *TrapData {
	trapTypes := []string{"git_url", "internal_api", "ssh_key", "database_url"}
	trapType := trapTypes[rng.Intn(len(trapTypes))]

	var content string
	switch trapType {
	case "git_url":
		content = fmt.Sprintf("git@internal-git.mirage.local:infrastructure/secrets-%s.git", df.generateReferralCode(rng))
	case "internal_api":
		content = fmt.Sprintf("https://api-internal.mirage.local/v2/admin?token=%s", df.generateToken(rng))
	case "ssh_key":
		content = df.generateFakeSSHKey(rng)
	case "database_url":
		content = fmt.Sprintf("postgresql://admin:%s@db-master.mirage.local:5432/production", df.generateToken(rng))
	}

	trap := &TrapData{
		Type:    trapType,
		Content: content,
	}

	// 添加溢出探测字节序列
	if df.config.EnableOverflowProbe {
		trap.HiddenProbe = df.generateOverflowProbe(rng)
	}

	// 记录陷阱
	df.mu.Lock()
	df.trapTriggers[uid] = &TrapTrigger{
		UID:       uid,
		TrapType:  trapType,
		Timestamp: time.Now(),
		Payload:   content,
	}
	df.stats.TrapsTriggered++
	df.mu.Unlock()

	if df.onTrapTriggered != nil {
		go df.onTrapTriggered(uid, trapType, content)
	}

	return trap
}

func (df *DBFaker) generateToken(rng *rand.Rand) string {
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	token := make([]byte, 32)
	for i := range token {
		token[i] = chars[rng.Intn(len(chars))]
	}
	return string(token)
}

func (df *DBFaker) generateFakeSSHKey(rng *rand.Rand) string {
	return fmt.Sprintf(`-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACB%sAAAAEHJvb3RAbWlyYWdlLmxvY2Fs
-----END OPENSSH PRIVATE KEY-----`, df.generateToken(rng)[:32])
}

// generateOverflowProbe 生成溢出探测字节序列
func (df *DBFaker) generateOverflowProbe(rng *rand.Rand) []byte {
	// 常见溢出漏洞探测模式
	patterns := [][]byte{
		// 格式化字符串漏洞探测
		[]byte("%n%n%n%n%n%n%n%n"),
		// 堆溢出探测
		[]byte{0x41, 0x41, 0x41, 0x41, 0x42, 0x42, 0x42, 0x42},
		// 整数溢出探测
		[]byte{0xff, 0xff, 0xff, 0x7f},
		// NULL 字节注入
		[]byte{0x00, 0x00, 0x00, 0x00},
	}

	return patterns[rng.Intn(len(patterns))]
}

func (df *DBFaker) generateFakeConfig(rng *rand.Rand) map[string]interface{} {
	return map[string]interface{}{
		"version":     "2.4.1",
		"environment": "production",
		"database": map[string]string{
			"host":     "db-replica-" + fmt.Sprintf("%d", rng.Intn(10)) + ".mirage.local",
			"port":     "5432",
			"name":     "mirage_prod",
			"user":     "app_user",
			"password": "[REDACTED]",
		},
		"redis": map[string]string{
			"host": "redis-cluster.mirage.local",
			"port": "6379",
		},
		"features": map[string]bool{
			"enable_audit_log": true,
			"enable_2fa":       true,
			"maintenance_mode": false,
		},
		"last_updated": time.Now().AddDate(0, 0, -rng.Intn(30)).Format(time.RFC3339),
	}
}

func (df *DBFaker) generateFakeCredentials(rng *rand.Rand) map[string]string {
	return map[string]string{
		"aws_access_key":     "AKIA" + strings.ToUpper(df.generateToken(rng)[:16]),
		"aws_secret_key":     "[ENCRYPTED:" + df.generateToken(rng) + "]",
		"stripe_api_key":     "sk_live_" + df.generateToken(rng),
		"sendgrid_api_key":   "SG." + df.generateToken(rng),
		"github_token":       "ghp_" + df.generateToken(rng),
		"slack_webhook":      "https://hooks.slack.com/services/T" + df.generateToken(rng)[:8],
		"encryption_key":     "[HSM_PROTECTED]",
		"jwt_secret":         "[VAULT_REF:secret/jwt]",
	}
}

func (df *DBFaker) generateFakeBackup(rng *rand.Rand) map[string]interface{} {
	return map[string]interface{}{
		"backup_id":   "bkp_" + df.generateToken(rng)[:12],
		"created_at":  time.Now().AddDate(0, 0, -rng.Intn(7)).Format(time.RFC3339),
		"size_bytes":  int64(rng.Intn(1000000000) + 100000000),
		"type":        []string{"full", "incremental", "differential"}[rng.Intn(3)],
		"status":      "completed",
		"storage_url": "s3://mirage-backups-" + []string{"us-east-1", "eu-west-1", "ap-southeast-1"}[rng.Intn(3)] + "/",
		"checksum":    "sha256:" + hex.EncodeToString([]byte(df.generateToken(rng))),
		"encrypted":   true,
		"retention_days": 30,
	}
}

// GenerateSQLResponse 生成 SQL 查询响应（带逻辑悖论）
func (df *DBFaker) GenerateSQLResponse(uid string, query string) *SQLResponse {
	seed := df.getSeed(uid)
	rng := rand.New(rand.NewSource(seed))

	df.mu.Lock()
	df.stats.TotalQueries++
	df.mu.Unlock()

	resp := &SQLResponse{
		Query:      query,
		RowCount:   rng.Intn(1000) + 1,
		ExecTimeMs: float64(rng.Intn(500)+10) / 10,
		Columns:    []string{"id", "data", "created_at", "metadata"},
	}

	// 生成数据行
	for i := 0; i < min(resp.RowCount, 100); i++ {
		row := df.generateSQLRow(rng, i)

		// 5% 概率插入逻辑悖论
		if rng.Float32() < 0.05 {
			row = df.injectParadox(rng, row)
		}

		resp.Rows = append(resp.Rows, row)
	}

	return resp
}

// SQLResponse SQL 响应
type SQLResponse struct {
	Query      string              `json:"query"`
	RowCount   int                 `json:"row_count"`
	ExecTimeMs float64             `json:"exec_time_ms"`
	Columns    []string            `json:"columns"`
	Rows       []map[string]string `json:"rows"`
}

func (df *DBFaker) generateSQLRow(rng *rand.Rand, id int) map[string]string {
	return map[string]string{
		"id":         fmt.Sprintf("%d", 1000000+id),
		"data":       df.generateToken(rng),
		"created_at": time.Now().AddDate(0, 0, -rng.Intn(365)).Format(time.RFC3339),
		"metadata":   fmt.Sprintf(`{"source":"node-%d","verified":true}`, rng.Intn(100)),
	}
}

func (df *DBFaker) injectParadox(rng *rand.Rand, row map[string]string) map[string]string {
	paradoxes := []func(map[string]string) map[string]string{
		// 悖论1：指向不存在的内部服务
		func(r map[string]string) map[string]string {
			r["metadata"] = `{"internal_ref":"https://vault.mirage.local/v1/secret/data/master-key","access":"restricted"}`
			return r
		},
		// 悖论2：看似有价值的加密数据
		func(r map[string]string) map[string]string {
			r["data"] = "ENC[AES256," + df.generateToken(rng) + "]"
			r["metadata"] = `{"key_id":"kmip://hsm.mirage.local/keys/prod-2024"}`
			return r
		},
		// 悖论3：假的管理员凭证
		func(r map[string]string) map[string]string {
			r["data"] = "admin:" + df.generateToken(rng)[:16]
			r["metadata"] = `{"role":"superadmin","mfa_bypass":true}`
			return r
		},
	}

	return paradoxes[rng.Intn(len(paradoxes))](row)
}

// OnTrapTriggered 设置陷阱触发回调
func (df *DBFaker) OnTrapTriggered(fn func(uid string, trapType string, payload string)) {
	df.mu.Lock()
	defer df.mu.Unlock()
	df.onTrapTriggered = fn
}

// GetStats 获取统计
func (df *DBFaker) GetStats() FakerStats {
	df.mu.RLock()
	defer df.mu.RUnlock()
	return df.stats
}

// GetTrapTriggers 获取陷阱触发记录
func (df *DBFaker) GetTrapTriggers() map[string]*TrapTrigger {
	df.mu.RLock()
	defer df.mu.RUnlock()

	result := make(map[string]*TrapTrigger)
	for k, v := range df.trapTriggers {
		result[k] = v
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
