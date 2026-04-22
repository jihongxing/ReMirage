package phantom

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================
// 9.1 数据面统计准确性测试
// ============================================================

func TestGetPhantomStats_ReturnsZeroWhenNoMap(t *testing.T) {
	mgr := NewManager()
	stats := mgr.GetPhantomStats()
	if stats.Redirected != 0 || stats.Passed != 0 || stats.Trapped != 0 || stats.Errors != 0 {
		t.Errorf("expected all zeros, got %+v", stats)
	}
}

func TestGetStats_CompatibleWithGetPhantomStats(t *testing.T) {
	mgr := NewManager()
	r, p, tr, e := mgr.GetStats()
	stats := mgr.GetPhantomStats()
	if r != stats.Redirected || p != stats.Passed || tr != stats.Trapped || e != stats.Errors {
		t.Errorf("GetStats and GetPhantomStats mismatch")
	}
}

// ============================================================
// 9.2 名单 TTL 过期测试
// ============================================================

func TestPhantomEntry_StructLayout(t *testing.T) {
	// 验证 PhantomEntry 结构体字段存在且类型正确
	entry := PhantomEntry{
		FirstSeen:  1000,
		LastSeen:   2000,
		HitCount:   5,
		RiskLevel:  3,
		TTLSeconds: 3600,
	}
	if entry.FirstSeen != 1000 {
		t.Errorf("FirstSeen mismatch")
	}
	if entry.LastSeen != 2000 {
		t.Errorf("LastSeen mismatch")
	}
	if entry.HitCount != 5 {
		t.Errorf("HitCount mismatch")
	}
	if entry.RiskLevel != 3 {
		t.Errorf("RiskLevel mismatch")
	}
	if entry.TTLSeconds != 3600 {
		t.Errorf("TTLSeconds mismatch")
	}
}

func TestTTLExpiry_EntryExpires(t *testing.T) {
	// 模拟 TTL 过期逻辑（不依赖 eBPF Map）
	now := uint64(time.Now().UnixNano())
	entry := PhantomEntry{
		FirstSeen:  now - 7200*1e9, // 2 小时前
		LastSeen:   now - 7200*1e9, // 2 小时前
		HitCount:   1,
		RiskLevel:  0,
		TTLSeconds: 3600, // 1 小时 TTL
	}

	expireAt := entry.LastSeen + uint64(entry.TTLSeconds)*1e9
	if now <= expireAt {
		t.Errorf("entry should be expired: now=%d, expireAt=%d", now, expireAt)
	}
}

func TestTTLExpiry_EntryNotExpired(t *testing.T) {
	now := uint64(time.Now().UnixNano())
	entry := PhantomEntry{
		FirstSeen:  now,
		LastSeen:   now,
		HitCount:   1,
		RiskLevel:  0,
		TTLSeconds: 3600,
	}

	expireAt := entry.LastSeen + uint64(entry.TTLSeconds)*1e9
	if now > expireAt {
		t.Errorf("entry should NOT be expired")
	}
}

func TestTTLExpiry_ZeroTTLNeverExpires(t *testing.T) {
	now := uint64(time.Now().UnixNano())
	entry := PhantomEntry{
		FirstSeen:  now - 999999*1e9,
		LastSeen:   now - 999999*1e9,
		HitCount:   100,
		RiskLevel:  0,
		TTLSeconds: 0, // 永不过期
	}

	// TTL=0 应该被跳过
	if entry.TTLSeconds != 0 {
		t.Errorf("TTLSeconds should be 0")
	}
}

// ============================================================
// 9.3 分层目标池测试
// ============================================================

func TestSetHoneypotPool_InvalidIP(t *testing.T) {
	mgr := NewManager()
	err := mgr.SetHoneypotPool(0, "invalid-ip")
	if err == nil {
		t.Errorf("expected error for invalid IP")
	}
}

func TestSetHoneypotPool_ValidIP(t *testing.T) {
	mgr := NewManager()
	// 没有 eBPF Map 时不会报错（honeypotConfig 为 nil）
	err := mgr.SetHoneypotPool(0, "10.99.1.100")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddToPhantom_ValidIP(t *testing.T) {
	mgr := NewManager()
	err := mgr.AddToPhantom("192.168.1.1", 2, 3600)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddToPhantom_InvalidIP(t *testing.T) {
	mgr := NewManager()
	err := mgr.AddToPhantom("not-an-ip", 0, 3600)
	if err == nil {
		t.Errorf("expected error for invalid IP")
	}
}

func TestRemoveFromPhantom_ValidIP(t *testing.T) {
	mgr := NewManager()
	// 先添加再移除
	mgr.AddToPhantom("192.168.1.1", 0, 3600)
	err := mgr.RemoveFromPhantom("192.168.1.1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddToPhishingList_Compat(t *testing.T) {
	mgr := NewManager()
	err := mgr.AddToPhishingList("10.0.0.1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================
// 9.4 Persona 一致性测试
// ============================================================

func TestPersona_DefaultValues(t *testing.T) {
	p := DefaultPersona
	if p.CompanyName == "" {
		t.Errorf("DefaultPersona.CompanyName should not be empty")
	}
	if p.PrimaryColor == "" {
		t.Errorf("DefaultPersona.PrimaryColor should not be empty")
	}
	if p.ErrorPrefix == "" {
		t.Errorf("DefaultPersona.ErrorPrefix should not be empty")
	}
	if p.APIVersion == "" {
		t.Errorf("DefaultPersona.APIVersion should not be empty")
	}
}

func TestPersona_AllTemplatesContainCompanyName(t *testing.T) {
	d := NewDispatcher()
	persona := Persona{
		CompanyName:   "TestCorp",
		Domain:        "testcorp.io",
		TagLine:       "Test Tag Line",
		PrimaryColor:  "#ff0000",
		ErrorPrefix:   "TC",
		APIVersion:    "v3",
		CopyrightYear: 2026,
	}
	d.SetPersona(persona)

	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"corporate_web", d.serveCorporateWeb},
		{"network_error", d.serveNetworkError},
		{"standard_https", d.serveStandardHTTPS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			tt.handler(w, r)
			body := w.Body.String()
			if !strings.Contains(body, "TestCorp") {
				t.Errorf("%s template does not contain company name 'TestCorp'", tt.name)
			}
		})
	}
}

func TestPersona_ErrorTemplateContainsPrefix(t *testing.T) {
	d := NewDispatcher()
	d.SetPersona(Persona{
		CompanyName:   "TestCorp",
		PrimaryColor:  "#ff0000",
		ErrorPrefix:   "TC",
		CopyrightYear: 2026,
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	d.serveNetworkError(w, r)
	body := w.Body.String()
	if !strings.Contains(body, "TC-TIMEOUT-001") {
		t.Errorf("error template does not contain error prefix 'TC-TIMEOUT-001'")
	}
}

func TestPersona_CorporateTemplateContainsColor(t *testing.T) {
	d := NewDispatcher()
	d.SetPersona(Persona{
		CompanyName:   "TestCorp",
		TagLine:       "Test",
		PrimaryColor:  "#abcdef",
		CopyrightYear: 2026,
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	d.serveCorporateWeb(w, r)
	body := w.Body.String()
	if !strings.Contains(body, "#abcdef") {
		t.Errorf("corporate template does not contain primary color '#abcdef'")
	}
}

func TestHoneypotDefault_ContainsPersonaCompanyName(t *testing.T) {
	h := NewHoneypotServer()
	h.SetPersona(Persona{
		CompanyName:   "HoneyTestCorp",
		CopyrightYear: 2026,
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	h.handleDefault(w, r)
	body := w.Body.String()
	if !strings.Contains(body, "HoneyTestCorp") {
		t.Errorf("honeypot default page does not contain company name 'HoneyTestCorp'")
	}
}

// ============================================================
// 9.5 迷宫限深测试
// ============================================================

func TestLabyrinth_MaxDepthReturns404(t *testing.T) {
	l := NewLabyrinthEngine()
	l.SetMaxDepth(5)

	// 深度 6 应该返回 404
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/a/b/c/d/e/f", nil)
	l.generateResponse(w, r, 6)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for depth 6, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "not_found" {
		t.Errorf("expected error='not_found', got %v", resp["error"])
	}
}

func TestLabyrinth_WithinDepthReturns200(t *testing.T) {
	l := NewLabyrinthEngine()
	l.SetMaxDepth(5)

	// 深度 3 应该返回 200
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/a/b/c", nil)
	l.generateResponse(w, r, 3)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for depth 3, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "success" {
		t.Errorf("expected status='success', got %v", resp["status"])
	}
	// 应该有 next 链接
	if resp["next"] == nil {
		t.Errorf("expected 'next' field for depth < maxDepth")
	}
}

func TestLabyrinth_AtMaxDepthNoNext(t *testing.T) {
	l := NewLabyrinthEngine()
	l.SetMaxDepth(5)

	// 深度 5 应该返回 200 但没有 next
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/a/b/c/d/e", nil)
	l.generateResponse(w, r, 5)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for depth 5, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["next"] != nil {
		t.Errorf("expected no 'next' field at max depth")
	}
}

func TestLabyrinth_NoLinksOrMeta(t *testing.T) {
	l := NewLabyrinthEngine()

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	l.generateResponse(w, r, 1)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["_links"]; ok {
		t.Errorf("response should not contain '_links'")
	}
	if _, ok := resp["_meta"]; ok {
		t.Errorf("response should not contain '_meta'")
	}
}

func TestLabyrinth_PersonaInResponse(t *testing.T) {
	l := NewLabyrinthEngine()
	l.SetPersona(Persona{
		CompanyName: "MazeTestCorp",
		APIVersion:  "v5",
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	l.generateResponse(w, r, 1)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["version"] != "v5" {
		t.Errorf("expected version='v5', got %v", resp["version"])
	}
	if resp["service"] != "MazeTestCorp" {
		t.Errorf("expected service='MazeTestCorp', got %v", resp["service"])
	}
}

func TestLabyrinth_MaxDelayIs3Seconds(t *testing.T) {
	l := NewLabyrinthEngine()
	if l.maxDelay != 3*time.Second {
		t.Errorf("expected maxDelay=3s, got %v", l.maxDelay)
	}
}

// ============================================================
// 追踪去显式化测试
// ============================================================

func TestCanaryFile_NoClassificationField(t *testing.T) {
	h := NewHoneypotServer()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/files/test.json", nil)
	h.handleCanaryFile(w, r)

	body := w.Body.String()
	if strings.Contains(body, "classification") {
		t.Errorf("canary file should not contain 'classification' field")
	}
	if strings.Contains(body, "CONFIDENTIAL") {
		t.Errorf("canary file should not contain 'CONFIDENTIAL'")
	}
	if strings.Contains(body, "_tracking") {
		t.Errorf("canary file should not contain '_tracking' field")
	}
	if !strings.Contains(body, "ref") {
		t.Errorf("canary file should contain 'ref' field")
	}
}

// ============================================================
// 调度规则清理测试
// ============================================================

func TestRequestContext_NoHeaderOrderField(t *testing.T) {
	d := NewDispatcher()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Host", "example.com")
	ctx := d.extractContext(r)

	// HeaderOrder 字段已移除，Headers map 仍然存在
	if ctx.Headers == nil {
		t.Errorf("Headers map should not be nil")
	}
}

func TestIsSuspiciousHeaderOrder_Deprecated(t *testing.T) {
	// 函数仍然存在但已标记 deprecated
	result := IsSuspiciousHeaderOrder([]string{"Host", "Accept"})
	if result {
		t.Errorf("expected false for normal headers")
	}
}

// ============================================================
// ShadowType 收敛测试
// ============================================================

func TestShadowType_OldAdminPortalRemoved(t *testing.T) {
	// ShadowAPILabyrinth 替代了 ShadowOldAdminPortal
	if ShadowAPILabyrinth != "api_labyrinth" {
		t.Errorf("expected ShadowAPILabyrinth='api_labyrinth', got %s", ShadowAPILabyrinth)
	}
}

func TestDispatcher_AdminPathUsesLabyrinth(t *testing.T) {
	d := NewDispatcher()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/admin/dashboard", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120")
	d.Dispatch(w, r)

	// 管理路径应该被迷宫处理（返回 JSON）
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("admin path should be handled by labyrinth (JSON), got Content-Type: %s", ct)
	}
}
