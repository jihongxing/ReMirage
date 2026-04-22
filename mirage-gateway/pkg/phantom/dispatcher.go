// Package phantom 多模态调度器
// 根据流量特征智能分发影子模板
package phantom

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

// ShadowType 影子类型
type ShadowType string

const (
	ShadowCorporateWeb  ShadowType = "corporate_web"
	ShadowNetworkError  ShadowType = "network_error"
	ShadowAPILabyrinth  ShadowType = "api_labyrinth"
	ShadowStandardHTTPS ShadowType = "standard_https"
)

// Dispatcher 多模态调度器
type Dispatcher struct {
	mu sync.RWMutex

	// 业务画像
	persona Persona

	// 模板处理器
	templates map[ShadowType]http.Handler

	// 分类规则
	rules []DispatchRule

	// 迷宫引擎
	labyrinth *LabyrinthEngine

	// 统计
	stats DispatchStats

	// 回调
	onDispatch func(shadowType ShadowType, reason string)
}

// DispatchRule 分发规则
type DispatchRule struct {
	Name     string
	Priority int
	Matcher  func(ctx *RequestContext) bool
	Target   ShadowType
}

// RequestContext 请求上下文
type RequestContext struct {
	UserAgent      string
	CipherSuite    uint16
	Path           string
	Method         string
	AcceptLanguage string
	RemoteAddr     string
	TLSVersion     uint16
	Headers        map[string]string
}

// DispatchStats 调度统计
type DispatchStats struct {
	TotalDispatched  int64
	ByCorporateWeb   int64
	ByNetworkError   int64
	ByOldAdminPortal int64 // Deprecated: use ByAPILabyrinth
	ByAPILabyrinth   int64
	ByStandardHTTPS  int64
}

// NewDispatcher 创建调度器
func NewDispatcher() *Dispatcher {
	d := &Dispatcher{
		persona:   DefaultPersona,
		templates: make(map[ShadowType]http.Handler),
		labyrinth: NewLabyrinthEngine(),
	}
	d.initDefaultRules()
	d.initDefaultTemplates()
	return d
}

// SetPersona 设置业务画像
func (d *Dispatcher) SetPersona(p Persona) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.persona = p
	// 同步 persona 到迷宫引擎
	d.labyrinth.SetPersona(p)
}

// initDefaultRules 初始化默认规则
func (d *Dispatcher) initDefaultRules() {
	d.rules = []DispatchRule{
		// 规则 1: 扫描器特征 -> 404
		{
			Name:     "scanner_detection",
			Priority: 100,
			Matcher: func(ctx *RequestContext) bool {
				scannerUAs := []string{
					"masscan", "zmap", "nmap", "zgrab",
					"censys", "shodan", "nuclei", "httpx",
				}
				ua := strings.ToLower(ctx.UserAgent)
				for _, scanner := range scannerUAs {
					if strings.Contains(ua, scanner) {
						return true
					}
				}
				return false
			},
			Target: ShadowStandardHTTPS,
		},
		// 规则 2: 管理路径探测 -> 迷宫
		{
			Name:     "admin_path_probe",
			Priority: 90,
			Matcher: func(ctx *RequestContext) bool {
				adminPaths := []string{
					"/admin", "/wp-admin", "/manager", "/console",
					"/phpmyadmin", "/cpanel", "/webmail", "/api/admin",
				}
				pathLower := strings.ToLower(ctx.Path)
				for _, p := range adminPaths {
					if strings.HasPrefix(pathLower, p) {
						return true
					}
				}
				return false
			},
			Target: ShadowAPILabyrinth,
		},
		// 规则 3: 空 UA 或异常 UA -> 网络错误
		{
			Name:     "anomalous_ua",
			Priority: 80,
			Matcher: func(ctx *RequestContext) bool {
				if ctx.UserAgent == "" {
					return true
				}
				// 检查异常短 UA
				if len(ctx.UserAgent) < 10 {
					return true
				}
				return false
			},
			Target: ShadowNetworkError,
		},
		// 规则 4: 敏感文件探测 -> 迷宫
		{
			Name:     "sensitive_file_probe",
			Priority: 70,
			Matcher: func(ctx *RequestContext) bool {
				sensitivePatterns := []string{
					".git", ".env", ".htaccess", "wp-config",
					"config.php", "database.yml", ".ssh",
					"id_rsa", "passwd", "shadow",
				}
				pathLower := strings.ToLower(ctx.Path)
				for _, p := range sensitivePatterns {
					if strings.Contains(pathLower, p) {
						return true
					}
				}
				return false
			},
			Target: ShadowAPILabyrinth,
		},
		// 规则 5: 正常浏览器特征 -> 公司官网
		{
			Name:     "normal_browser",
			Priority: 50,
			Matcher: func(ctx *RequestContext) bool {
				browserPatterns := []string{
					"Mozilla/5.0", "Chrome/", "Firefox/", "Safari/",
				}
				for _, p := range browserPatterns {
					if strings.Contains(ctx.UserAgent, p) {
						return true
					}
				}
				return false
			},
			Target: ShadowCorporateWeb,
		},
		// 规则 6: 默认 -> 404
		{
			Name:     "default",
			Priority: 0,
			Matcher:  func(ctx *RequestContext) bool { return true },
			Target:   ShadowStandardHTTPS,
		},
	}
}

// initDefaultTemplates 初始化默认模板
func (d *Dispatcher) initDefaultTemplates() {
	d.templates[ShadowCorporateWeb] = http.HandlerFunc(d.serveCorporateWeb)
	d.templates[ShadowNetworkError] = http.HandlerFunc(d.serveNetworkError)
	d.templates[ShadowAPILabyrinth] = d.labyrinth.Handler()
	d.templates[ShadowStandardHTTPS] = http.HandlerFunc(d.serveStandardHTTPS)
}

// Dispatch 分发请求
func (d *Dispatcher) Dispatch(w http.ResponseWriter, r *http.Request) {
	ctx := d.extractContext(r)
	shadowType, ruleName := d.matchRule(ctx)

	d.mu.Lock()
	d.stats.TotalDispatched++
	switch shadowType {
	case ShadowCorporateWeb:
		d.stats.ByCorporateWeb++
	case ShadowNetworkError:
		d.stats.ByNetworkError++
	case ShadowAPILabyrinth:
		d.stats.ByAPILabyrinth++
	case ShadowStandardHTTPS:
		d.stats.ByStandardHTTPS++
	}
	d.mu.Unlock()

	if d.onDispatch != nil {
		go d.onDispatch(shadowType, ruleName)
	}

	handler := d.templates[shadowType]
	if handler != nil {
		handler.ServeHTTP(w, r)
	} else {
		d.serveStandardHTTPS(w, r)
	}
}

// extractContext 提取请求上下文
func (d *Dispatcher) extractContext(r *http.Request) *RequestContext {
	ctx := &RequestContext{
		UserAgent:      r.UserAgent(),
		Path:           r.URL.Path,
		Method:         r.Method,
		AcceptLanguage: r.Header.Get("Accept-Language"),
		RemoteAddr:     r.RemoteAddr,
		Headers:        make(map[string]string),
	}

	// 提取 Headers
	for key := range r.Header {
		ctx.Headers[key] = r.Header.Get(key)
	}

	// TLS 信息
	if r.TLS != nil {
		ctx.CipherSuite = r.TLS.CipherSuite
		ctx.TLSVersion = r.TLS.Version
	}

	return ctx
}

// matchRule 匹配规则
func (d *Dispatcher) matchRule(ctx *RequestContext) (ShadowType, string) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 按优先级排序匹配
	for _, rule := range d.rules {
		if rule.Matcher(ctx) {
			return rule.Target, rule.Name
		}
	}

	return ShadowStandardHTTPS, "default"
}

// serveCorporateWeb 公司官网模板
func (d *Dispatcher) serveCorporateWeb(w http.ResponseWriter, r *http.Request) {
	p := d.persona
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 0; background: #f5f5f5; }
        .header { background: %s; color: white; padding: 60px 20px; text-align: center; }
        .header h1 { margin: 0; font-size: 2.5em; }
        .header p { margin: 10px 0 0; opacity: 0.9; }
        .content { max-width: 1200px; margin: 40px auto; padding: 0 20px; }
        .card { background: white; border-radius: 8px; padding: 30px; margin-bottom: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .footer { text-align: center; padding: 40px; color: #666; }
    </style>
</head>
<body>
    <div class="header">
        <h1>%s</h1>
        <p>%s</p>
    </div>
    <div class="content">
        <div class="card">
            <h2>Welcome</h2>
            <p>Please authenticate to continue.</p>
        </div>
    </div>
    <div class="footer">
        <p>&copy; %d %s. All rights reserved.</p>
    </div>
</body>
</html>`, p.CompanyName, p.PrimaryColor, p.CompanyName, p.TagLine, p.CopyrightYear, p.CompanyName)))
}

// serveNetworkError 网络错误模板
func (d *Dispatcher) serveNetworkError(w http.ResponseWriter, r *http.Request) {
	p := d.persona
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusGatewayTimeout)
	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>504 Gateway Timeout</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; text-align: center; padding: 50px; background: #f5f5f5; }
        .error-box { background: white; padding: 40px; border-radius: 8px; display: inline-block; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: %s; margin: 0 0 20px; }
        p { color: #666; margin: 10px 0; }
        .code { font-family: monospace; background: #f5f5f5; padding: 10px; border-radius: 4px; margin-top: 20px; }
        .footer { text-align: center; padding: 40px; color: #999; font-size: 0.85em; }
    </style>
</head>
<body>
    <div class="error-box">
        <h1>504 Gateway Timeout</h1>
        <p>The upstream server is taking too long to respond.</p>
        <p>Please try again later.</p>
        <div class="code">Error Code: %s-TIMEOUT-001</div>
    </div>
    <div class="footer">&copy; %d %s</div>
</body>
</html>`, p.PrimaryColor, p.ErrorPrefix, p.CopyrightYear, p.CompanyName)))
}

// serveStandardHTTPS 标准 404 模板
func (d *Dispatcher) serveStandardHTTPS(w http.ResponseWriter, r *http.Request) {
	p := d.persona
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>404 Not Found - %s</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; text-align: center; padding: 50px; background: #f5f5f5; }
        h1 { color: #333; }
        p { color: #666; }
        .footer { padding: 40px; color: #999; font-size: 0.85em; }
    </style>
</head>
<body>
    <h1>404 Not Found</h1>
    <p>The requested URL was not found on this server.</p>
    <div class="footer">&copy; %d %s</div>
</body>
</html>`, p.CompanyName, p.CopyrightYear, p.CompanyName)))
}

// AddRule 添加自定义规则
func (d *Dispatcher) AddRule(rule DispatchRule) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 按优先级插入
	inserted := false
	for i, r := range d.rules {
		if rule.Priority > r.Priority {
			d.rules = append(d.rules[:i], append([]DispatchRule{rule}, d.rules[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		d.rules = append(d.rules, rule)
	}
}

// SetTemplate 设置自定义模板
func (d *Dispatcher) SetTemplate(shadowType ShadowType, handler http.Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.templates[shadowType] = handler
}

// GetStats 获取统计
func (d *Dispatcher) GetStats() DispatchStats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.stats
}

// OnDispatch 设置分发回调
func (d *Dispatcher) OnDispatch(fn func(shadowType ShadowType, reason string)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onDispatch = fn
}

// Handler 返回 HTTP 处理器
func (d *Dispatcher) Handler() http.Handler {
	return http.HandlerFunc(d.Dispatch)
}

// GetLabyrinth 获取迷宫引擎
func (d *Dispatcher) GetLabyrinth() *LabyrinthEngine {
	return d.labyrinth
}

// MatchUserAgentPattern 匹配 UA 模式
func MatchUserAgentPattern(ua string, patterns []string) bool {
	uaLower := strings.ToLower(ua)
	for _, p := range patterns {
		if strings.Contains(uaLower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// MatchPathPattern 匹配路径模式
func MatchPathPattern(path string, pattern string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(path)
}

// IsSuspiciousHeaderOrder 检查可疑的 Header 顺序
// Deprecated: Header 顺序依赖 Go map 迭代顺序，不可信。请勿在调度规则中使用。
func IsSuspiciousHeaderOrder(order []string) bool {
	// 正常浏览器通常 Host 在前
	if len(order) == 0 {
		return true
	}

	// 检查是否缺少常见 Header
	hasHost := false
	hasAccept := false
	for _, h := range order {
		if strings.EqualFold(h, "Host") {
			hasHost = true
		}
		if strings.EqualFold(h, "Accept") {
			hasAccept = true
		}
	}

	return !hasHost || !hasAccept
}
