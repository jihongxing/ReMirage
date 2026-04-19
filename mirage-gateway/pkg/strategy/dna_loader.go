// Package strategy - DNA 指纹库动态加载器
// 支持通过 M.C.C. 动态拉取最新指纹模板
// 实现"加载-持久化-恢复"闭环逻辑
package strategy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"mirage-gateway/pkg/storage"
)

// BrowserDNATemplate 浏览器指纹模板（用于 JA4/TLS 伪装）
type BrowserDNATemplate struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Browser     string            `json:"browser"`
	OS          string            `json:"os"`
	JA4         string            `json:"ja4"`
	WindowSize  uint16            `json:"window_size"`
	TTL         uint8             `json:"ttl"`
	MSS         uint16            `json:"mss"`
	Options     []byte            `json:"options"`
	Headers     map[string]string `json:"headers"`
	CreatedAt   time.Time         `json:"created_at"`
	Checksum    string            `json:"checksum"`
}

// BrowserDNALibrary 浏览器指纹库
type BrowserDNALibrary struct {
	mu           sync.RWMutex
	templates    map[string]*BrowserDNATemplate
	activeID     string
	repoURL      string
	lastUpdate   time.Time
	updatePeriod time.Duration

	// 持久化存储
	vault *storage.VaultStorage

	// eBPF Map 更新回调
	OnTemplateUpdate func(template *BrowserDNATemplate) error
}

// NewBrowserDNALibrary 创建浏览器指纹库
func NewBrowserDNALibrary(repoURL string) *BrowserDNALibrary {
	lib := &BrowserDNALibrary{
		templates:    make(map[string]*BrowserDNATemplate),
		repoURL:      repoURL,
		updatePeriod: 24 * time.Hour,
	}

	// 加载内置模板
	lib.loadBuiltinTemplates()

	return lib
}

// NewBrowserDNALibraryWithVault 创建带持久化的指纹库
func NewBrowserDNALibraryWithVault(repoURL string, vault *storage.VaultStorage) *BrowserDNALibrary {
	lib := &BrowserDNALibrary{
		templates:    make(map[string]*BrowserDNATemplate),
		repoURL:      repoURL,
		updatePeriod: 24 * time.Hour,
		vault:        vault,
	}

	// 优先从 Vault 恢复
	if vault != nil {
		if err := lib.restoreFromVault(); err != nil {
			log.Printf("⚠️  [DNALibrary] Vault 恢复失败: %v，加载内置模板", err)
			lib.loadBuiltinTemplates()
		}
	} else {
		lib.loadBuiltinTemplates()
	}

	return lib
}

// restoreFromVault 从 Vault 恢复 DNA 模板（冷启动）
func (lib *BrowserDNALibrary) restoreFromVault() error {
	if lib.vault == nil {
		return fmt.Errorf("vault 未初始化")
	}

	records, err := lib.vault.ListDNA()
	if err != nil {
		return err
	}

	if len(records) == 0 {
		return fmt.Errorf("vault 中无 DNA 记录")
	}

	for _, record := range records {
		template := &BrowserDNATemplate{
			ID:         record.ProfileID,
			Name:       record.Browser + " " + record.Version + " on " + record.OS,
			Version:    record.Version,
			Browser:    record.Browser,
			OS:         record.OS,
			JA4:        record.JA4,
			WindowSize: record.TCPWindow,
			TTL:        record.TTL,
			MSS:        record.MSS,
			Headers:    record.Headers,
			CreatedAt:  time.Unix(record.CreatedAt, 0),
			Checksum:   record.Checksum,
		}
		lib.templates[template.ID] = template
	}

	// 设置默认活跃模板
	if len(lib.templates) > 0 {
		for id := range lib.templates {
			lib.activeID = id
			break
		}
	}

	log.Printf("📚 [DNALibrary] 从 Vault 恢复 %d 个模板", len(records))
	return nil
}

// persistToVault 持久化到 Vault
func (lib *BrowserDNALibrary) persistToVault(template *BrowserDNATemplate) error {
	if lib.vault == nil {
		return nil // 无 vault 时静默跳过
	}

	record := &storage.DNARecord{
		ProfileID: template.ID,
		Browser:   template.Browser,
		Version:   template.Version,
		OS:        template.OS,
		JA4:       template.JA4,
		TCPWindow: template.WindowSize,
		TTL:       template.TTL,
		MSS:       template.MSS,
		Headers:   template.Headers,
		CreatedAt: template.CreatedAt.Unix(),
		Checksum:  template.Checksum,
	}

	return lib.vault.SaveDNA(record)
}

// loadBuiltinTemplates 加载内置模板
func (lib *BrowserDNALibrary) loadBuiltinTemplates() {
	builtins := []*BrowserDNATemplate{
		{
			ID:         "chrome-130-win11",
			Name:       "Chrome 130 on Windows 11",
			Version:    "130.0.0.0",
			Browser:    "Chrome",
			OS:         "Windows 11",
			JA4:        "t13d1516h2_8daaf6152771_e5627efa2ab1",
			WindowSize: 65535,
			TTL:        128,
			MSS:        1460,
			Options:    []byte{0x02, 0x04, 0x05, 0xb4, 0x01, 0x03, 0x03, 0x08},
			Headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
				"Accept-Language": "en-US,en;q=0.9",
				"Accept-Encoding": "gzip, deflate, br",
			},
			CreatedAt: time.Now(),
		},
		{
			ID:         "firefox-125-linux",
			Name:       "Firefox 125 on Linux",
			Version:    "125.0",
			Browser:    "Firefox",
			OS:         "Linux",
			JA4:        "t13d1715h2_5b57614c22b0_3d5424432f57",
			WindowSize: 32768,
			TTL:        64,
			MSS:        1460,
			Options:    []byte{0x02, 0x04, 0x05, 0xb4, 0x04, 0x02, 0x08, 0x0a},
			Headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0",
				"Accept-Language": "en-US,en;q=0.5",
				"Accept-Encoding": "gzip, deflate, br",
			},
			CreatedAt: time.Now(),
		},
		{
			ID:         "safari-17-macos",
			Name:       "Safari 17 on macOS",
			Version:    "17.4",
			Browser:    "Safari",
			OS:         "macOS Sonoma",
			JA4:        "t13d1517h2_8daaf6152771_02713d6af862",
			WindowSize: 65535,
			TTL:        64,
			MSS:        1460,
			Options:    []byte{0x02, 0x04, 0x05, 0xb4, 0x01, 0x01, 0x08, 0x0a},
			Headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
				"Accept-Language": "en-US,en;q=0.9",
				"Accept-Encoding": "gzip, deflate, br",
			},
			CreatedAt: time.Now(),
		},
		{
			ID:         "edge-130-win11",
			Name:       "Edge 130 on Windows 11",
			Version:    "130.0.0.0",
			Browser:    "Edge",
			OS:         "Windows 11",
			JA4:        "t13d1516h2_8daaf6152771_e5627efa2ab1",
			WindowSize: 65535,
			TTL:        128,
			MSS:        1460,
			Options:    []byte{0x02, 0x04, 0x05, 0xb4, 0x01, 0x03, 0x03, 0x08},
			Headers: map[string]string{
				"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
				"Accept-Language": "en-US,en;q=0.9",
				"Accept-Encoding": "gzip, deflate, br",
			},
			CreatedAt: time.Now(),
		},
	}

	for _, t := range builtins {
		t.Checksum = lib.calculateChecksum(t)
		lib.templates[t.ID] = t
	}

	// 默认使用 Chrome
	lib.activeID = "chrome-130-win11"

	log.Printf("📚 [DNALibrary] 加载 %d 个内置模板", len(builtins))
}

// calculateChecksum 计算模板校验和
func (lib *BrowserDNALibrary) calculateChecksum(t *BrowserDNATemplate) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d",
		t.ID, t.JA4, t.Browser, t.OS, t.WindowSize, t.TTL, t.MSS)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// GetTemplate 获取指定模板
func (lib *BrowserDNALibrary) GetTemplate(id string) (*BrowserDNATemplate, bool) {
	lib.mu.RLock()
	defer lib.mu.RUnlock()

	t, ok := lib.templates[id]
	return t, ok
}

// GetActiveTemplate 获取当前活跃模板
func (lib *BrowserDNALibrary) GetActiveTemplate() *BrowserDNATemplate {
	lib.mu.RLock()
	defer lib.mu.RUnlock()

	return lib.templates[lib.activeID]
}

// SetActiveTemplate 设置活跃模板
func (lib *BrowserDNALibrary) SetActiveTemplate(id string) error {
	lib.mu.Lock()
	defer lib.mu.Unlock()

	if _, ok := lib.templates[id]; !ok {
		return fmt.Errorf("模板不存在: %s", id)
	}

	lib.activeID = id
	log.Printf("📚 [BrowserDNALibrary] 切换活跃模板: %s", id)

	// 触发 eBPF Map 更新
	if lib.OnTemplateUpdate != nil {
		return lib.OnTemplateUpdate(lib.templates[id])
	}

	return nil
}

// AddTemplate 添加新模板
func (lib *BrowserDNALibrary) AddTemplate(t *BrowserDNATemplate) error {
	lib.mu.Lock()
	defer lib.mu.Unlock()

	// 计算校验和
	t.Checksum = lib.calculateChecksum(t)

	// 验证校验和
	if existing, ok := lib.templates[t.ID]; ok {
		if existing.Checksum == t.Checksum {
			return nil // 相同模板，跳过
		}
	}

	lib.templates[t.ID] = t

	// 持久化到 Vault
	if err := lib.persistToVault(t); err != nil {
		log.Printf("⚠️  [DNALibrary] 持久化失败: %v", err)
	}

	log.Printf("📚 [BrowserDNALibrary] 添加模板: %s (%s)", t.ID, t.Name)

	return nil
}

// ListTemplates 列出所有模板
func (lib *BrowserDNALibrary) ListTemplates() []*BrowserDNATemplate {
	lib.mu.RLock()
	defer lib.mu.RUnlock()

	result := make([]*BrowserDNATemplate, 0, len(lib.templates))
	for _, t := range lib.templates {
		result = append(result, t)
	}
	return result
}

// --- M.C.C. 动态更新 ---

// BrowserDNAUpdatePayload M.C.C. 更新载荷
type BrowserDNAUpdatePayload struct {
	Templates []*BrowserDNATemplate `json:"templates"`
	Timestamp int64                 `json:"timestamp"`
	Signature string                `json:"signature"`
}

// IPSeedPayload M.C.C. IP 种子载荷（新节点初始生存包）
type IPSeedPayload struct {
	Seeds     []*IPSeedEntry `json:"seeds"`
	Timestamp int64          `json:"timestamp"`
	Region    string         `json:"region"`
}

// IPSeedEntry IP 种子条目
type IPSeedEntry struct {
	IP              string  `json:"ip"`
	Latency         float64 `json:"latency"`
	ReputationScore float64 `json:"reputation_score"`
	Region          string  `json:"region"`
	Provider        string  `json:"provider"`
}

// ProcessMCCUpdate 处理 M.C.C. 更新
func (lib *BrowserDNALibrary) ProcessMCCUpdate(payload []byte) error {
	var update BrowserDNAUpdatePayload
	if err := json.Unmarshal(payload, &update); err != nil {
		return fmt.Errorf("解析更新载荷失败: %w", err)
	}

	// 验证时间戳
	updateTime := time.Unix(update.Timestamp, 0)
	if time.Since(updateTime) > 24*time.Hour {
		return fmt.Errorf("更新载荷过期")
	}

	// 添加新模板（自动持久化）
	addedCount := 0
	for _, t := range update.Templates {
		if err := lib.AddTemplate(t); err == nil {
			addedCount++
		}
	}

	lib.mu.Lock()
	lib.lastUpdate = time.Now()
	lib.mu.Unlock()

	log.Printf("📚 [BrowserDNALibrary] M.C.C. 更新完成: 添加 %d 个模板（已持久化）", addedCount)

	return nil
}

// ProcessIPSeedUpdate 处理 M.C.C. IP 种子下发（新节点冷启动）
func (lib *BrowserDNALibrary) ProcessIPSeedUpdate(payload []byte) (int, error) {
	if lib.vault == nil {
		return 0, fmt.Errorf("vault 未初始化")
	}

	var seedPayload IPSeedPayload
	if err := json.Unmarshal(payload, &seedPayload); err != nil {
		return 0, fmt.Errorf("解析 IP 种子载荷失败: %w", err)
	}

	// 验证时间戳
	updateTime := time.Unix(seedPayload.Timestamp, 0)
	if time.Since(updateTime) > 1*time.Hour {
		return 0, fmt.Errorf("IP 种子载荷过期")
	}

	// 写入 Vault
	importedCount := 0
	for _, seed := range seedPayload.Seeds {
		record := &storage.IPReputationRecord{
			IP:              seed.IP,
			Latency:         seed.Latency,
			ReputationScore: seed.ReputationScore,
			Region:          seed.Region,
			Provider:        seed.Provider,
		}
		if err := lib.vault.SaveIPReputation(record); err == nil {
			importedCount++
		}
	}

	log.Printf("🌱 [DNALibrary] IP 种子导入完成: %d/%d 条", importedCount, len(seedPayload.Seeds))
	return importedCount, nil
}

// HasIPSeeds 检查是否有 IP 种子数据
func (lib *BrowserDNALibrary) HasIPSeeds() bool {
	if lib.vault == nil {
		return false
	}
	ips, err := lib.vault.GetTopIPs(1)
	return err == nil && len(ips) > 0
}

// GetIPSeedCount 获取 IP 种子数量
func (lib *BrowserDNALibrary) GetIPSeedCount() int {
	if lib.vault == nil {
		return 0
	}
	ips, err := lib.vault.GetTopIPs(1000)
	if err != nil {
		return 0
	}
	return len(ips)
}

// GetLastUpdateTime 获取最后更新时间
func (lib *BrowserDNALibrary) GetLastUpdateTime() time.Time {
	lib.mu.RLock()
	defer lib.mu.RUnlock()
	return lib.lastUpdate
}

// --- eBPF Map 同步 ---

// BrowserDNAMapEntry eBPF Map 条目
type BrowserDNAMapEntry struct {
	WindowSize uint16
	TTL        uint8
	MSS        uint16
	Options    [16]byte
	OptionsLen uint8
}

// ToMapEntry 转换为 eBPF Map 条目
func (t *BrowserDNATemplate) ToMapEntry() *BrowserDNAMapEntry {
	entry := &BrowserDNAMapEntry{
		WindowSize: t.WindowSize,
		TTL:        t.TTL,
		MSS:        t.MSS,
	}

	// 复制 Options
	copy(entry.Options[:], t.Options)
	entry.OptionsLen = uint8(len(t.Options))

	return entry
}
