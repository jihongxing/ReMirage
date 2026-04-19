// Package phantom 真实性细节注入
// 根据区域动态生成逼真的影子页面内容
package phantom

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// TruthInjector 真实性注入器
type TruthInjector struct {
	// 区域配置
	regionConfigs map[string]*RegionConfig

	// 新闻缓存
	newsCache []NewsItem
	newsUpdated time.Time
}

// RegionConfig 区域配置
type RegionConfig struct {
	CompanyName    string
	Address        string
	Phone          string
	Currency       string
	Language       string
	LegalNotice    string
	TimeZone       string
	DateFormat     string
}

// NewsItem 新闻条目
type NewsItem struct {
	Title     string
	Summary   string
	Date      time.Time
	Category  string
}

// NewTruthInjector 创建注入器
func NewTruthInjector() *TruthInjector {
	ti := &TruthInjector{
		regionConfigs: make(map[string]*RegionConfig),
	}
	ti.initRegionConfigs()
	ti.initFakeNews()
	return ti
}

// initRegionConfigs 初始化区域配置
func (ti *TruthInjector) initRegionConfigs() {
	ti.regionConfigs["europe_gdpr"] = &RegionConfig{
		CompanyName: "Global Solutions GmbH",
		Address:     "Friedrichstraße 123, 10117 Berlin, Germany",
		Phone:       "+49 30 1234 5678",
		Currency:    "EUR",
		Language:    "de",
		LegalNotice: `<div class="gdpr-notice">This website uses cookies to ensure you get the best experience. By continuing to use this site, you consent to our use of cookies in accordance with GDPR. <a href="/privacy">Privacy Policy</a> | <a href="/cookies">Cookie Settings</a></div>`,
		TimeZone:    "Europe/Berlin",
		DateFormat:  "02.01.2006",
	}

	ti.regionConfigs["asia_pacific"] = &RegionConfig{
		CompanyName: "Global Solutions Pte. Ltd.",
		Address:     "1 Raffles Place, #20-01, Singapore 048616",
		Phone:       "+65 6123 4567",
		Currency:    "SGD",
		Language:    "en",
		LegalNotice: `<div class="legal-notice">© 2026 Global Solutions. All rights reserved. Registered in Singapore.</div>`,
		TimeZone:    "Asia/Singapore",
		DateFormat:  "2006-01-02",
	}

	ti.regionConfigs["north_america"] = &RegionConfig{
		CompanyName: "Global Solutions Inc.",
		Address:     "350 Fifth Avenue, Suite 4500, New York, NY 10118",
		Phone:       "+1 (212) 555-0123",
		Currency:    "USD",
		Language:    "en",
		LegalNotice: `<div class="legal-notice">© 2026 Global Solutions Inc. All rights reserved. | <a href="/terms">Terms of Service</a> | <a href="/privacy">Privacy Policy</a></div>`,
		TimeZone:    "America/New_York",
		DateFormat:  "01/02/2006",
	}

	ti.regionConfigs["nordic"] = &RegionConfig{
		CompanyName: "Global Solutions ehf.",
		Address:     "Borgartún 26, 105 Reykjavík, Iceland",
		Phone:       "+354 520 1000",
		Currency:    "ISK",
		Language:    "is",
		LegalNotice: `<div class="legal-notice">© 2026 Global Solutions ehf. Skráð á Íslandi.</div>`,
		TimeZone:    "Atlantic/Reykjavik",
		DateFormat:  "02.01.2006",
	}

	ti.regionConfigs["global"] = &RegionConfig{
		CompanyName: "Global Solutions Ltd.",
		Address:     "123 Business Park, Tech City",
		Phone:       "+1 800 123 4567",
		Currency:    "USD",
		Language:    "en",
		LegalNotice: `<div class="legal-notice">© 2026 Global Solutions. All rights reserved.</div>`,
		TimeZone:    "UTC",
		DateFormat:  "2006-01-02",
	}
}

// initFakeNews 初始化假新闻
func (ti *TruthInjector) initFakeNews() {
	ti.newsCache = []NewsItem{
		{Title: "Q4 2025 Financial Results Exceed Expectations", Summary: "Strong performance across all business segments drives record revenue.", Date: time.Now().AddDate(0, 0, -1), Category: "Business"},
		{Title: "New Partnership Announced with Leading Cloud Provider", Summary: "Strategic alliance to enhance enterprise solutions portfolio.", Date: time.Now().AddDate(0, 0, -3), Category: "Technology"},
		{Title: "Global Solutions Expands Operations in Asia Pacific", Summary: "New regional headquarters opens in Singapore.", Date: time.Now().AddDate(0, 0, -5), Category: "Corporate"},
		{Title: "Industry Recognition: Best Enterprise Solution 2025", Summary: "Awarded for innovation in digital transformation.", Date: time.Now().AddDate(0, 0, -7), Category: "Awards"},
		{Title: "Upcoming Webinar: Future of Enterprise Technology", Summary: "Join our experts for insights on emerging trends.", Date: time.Now().AddDate(0, 0, 2), Category: "Events"},
	}
	ti.newsUpdated = time.Now()
}

// GetRegionConfig 获取区域配置
func (ti *TruthInjector) GetRegionConfig(regionBias string) *RegionConfig {
	if config, ok := ti.regionConfigs[regionBias]; ok {
		return config
	}
	return ti.regionConfigs["global"]
}

// GenerateCorporatePage 生成公司页面
func (ti *TruthInjector) GenerateCorporatePage(regionBias string, seed int64) string {
	config := ti.GetRegionConfig(regionBias)
	rng := rand.New(rand.NewSource(seed))

	// 选择新闻
	news := ti.selectNews(rng, 3)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - Enterprise Solutions</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f8f9fa; color: #333; }
        .header { background: linear-gradient(135deg, #1a365d 0%%, #2d5a87 100%%); color: white; padding: 20px 40px; }
        .header h1 { font-size: 1.5em; }
        .nav { background: #fff; border-bottom: 1px solid #e2e8f0; padding: 15px 40px; }
        .nav a { color: #4a5568; text-decoration: none; margin-right: 30px; }
        .hero { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 80px 40px; text-align: center; }
        .hero h2 { font-size: 2.5em; margin-bottom: 20px; }
        .content { max-width: 1200px; margin: 40px auto; padding: 0 40px; }
        .card { background: white; border-radius: 8px; padding: 30px; margin-bottom: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.05); }
        .news-item { border-bottom: 1px solid #e2e8f0; padding: 15px 0; }
        .news-item:last-child { border-bottom: none; }
        .news-date { color: #718096; font-size: 0.85em; }
        .footer { background: #1a365d; color: #a0aec0; padding: 40px; text-align: center; }
        .contact { background: #f7fafc; padding: 20px; border-radius: 8px; margin-top: 20px; }
        .gdpr-notice, .legal-notice { background: #edf2f7; padding: 15px; font-size: 0.85em; text-align: center; }
        .captcha-overlay { display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 1000; }
        .captcha-box { background: white; padding: 30px; border-radius: 8px; max-width: 400px; margin: 100px auto; text-align: center; }
    </style>
</head>
<body>
    %s
    <div class="header"><h1>%s</h1></div>
    <div class="nav">
        <a href="/">Home</a>
        <a href="/solutions">Solutions</a>
        <a href="/about">About Us</a>
        <a href="/contact">Contact</a>
        <a href="/careers">Careers</a>
    </div>
    <div class="hero">
        <h2>Enterprise Technology Solutions</h2>
        <p>Transforming businesses through innovation</p>
    </div>
    <div class="content">
        <div class="card">
            <h3>Latest News</h3>
            %s
        </div>
        <div class="card contact">
            <h3>Contact Us</h3>
            <p><strong>Address:</strong> %s</p>
            <p><strong>Phone:</strong> %s</p>
        </div>
    </div>
    <div class="footer">
        %s
        <p style="margin-top: 20px;">%s</p>
    </div>
    %s
</body>
</html>`,
		config.Language,
		config.CompanyName,
		config.LegalNotice,
		config.CompanyName,
		ti.renderNews(news, config.DateFormat),
		config.Address,
		config.Phone,
		config.LegalNotice,
		config.CompanyName,
		ti.generateCaptchaScript(rng),
	)
}

// selectNews 选择新闻
func (ti *TruthInjector) selectNews(rng *rand.Rand, count int) []NewsItem {
	if len(ti.newsCache) <= count {
		return ti.newsCache
	}

	// 随机选择但保持一致性（基于 seed）
	indices := rng.Perm(len(ti.newsCache))[:count]
	result := make([]NewsItem, count)
	for i, idx := range indices {
		result[i] = ti.newsCache[idx]
	}
	return result
}

// renderNews 渲染新闻
func (ti *TruthInjector) renderNews(news []NewsItem, dateFormat string) string {
	var sb strings.Builder
	for _, item := range news {
		sb.WriteString(fmt.Sprintf(`<div class="news-item">
            <div class="news-date">%s | %s</div>
            <h4>%s</h4>
            <p>%s</p>
        </div>`, item.Date.Format(dateFormat), item.Category, item.Title, item.Summary))
	}
	return sb.String()
}

// generateCaptchaScript 生成假验证码脚本
func (ti *TruthInjector) generateCaptchaScript(rng *rand.Rand) string {
	// 10% 概率显示验证码
	if rng.Float32() > 0.1 {
		return ""
	}

	return `<div class="captcha-overlay" id="captcha-overlay">
        <div class="captcha-box">
            <h3>Security Check</h3>
            <p>Please verify you are human</p>
            <div style="margin: 20px 0; padding: 20px; border: 1px solid #ddd; border-radius: 4px;">
                <input type="checkbox" id="captcha-check" onchange="verifyCaptcha()">
                <label for="captcha-check">I'm not a robot</label>
            </div>
            <p style="font-size: 0.8em; color: #666;">Protected by reCAPTCHA</p>
        </div>
    </div>
    <script>
        setTimeout(function() {
            document.getElementById('captcha-overlay').style.display = 'block';
        }, 3000 + Math.random() * 5000);
        function verifyCaptcha() {
            setTimeout(function() {
                document.getElementById('captcha-overlay').style.display = 'none';
            }, 1500);
        }
    </script>`
}

// GenerateNetworkErrorPage 生成网络错误页面
func (ti *TruthInjector) GenerateNetworkErrorPage(regionBias string) string {
	config := ti.GetRegionConfig(regionBias)

	ispNames := map[string]string{
		"europe_gdpr":   "Deutsche Telekom",
		"asia_pacific":  "Singtel",
		"north_america": "AT&T",
		"nordic":        "Síminn",
		"global":        "Network Provider",
	}

	isp := ispNames[regionBias]
	if isp == "" {
		isp = ispNames["global"]
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
    <title>504 Gateway Timeout</title>
    <style>
        body { font-family: Arial, sans-serif; text-align: center; padding: 50px; background: #f5f5f5; }
        .error-box { background: white; padding: 40px; border-radius: 8px; display: inline-block; box-shadow: 0 2px 10px rgba(0,0,0,0.1); max-width: 500px; }
        h1 { color: #e74c3c; margin: 0 0 20px; }
        p { color: #666; margin: 10px 0; }
        .code { font-family: monospace; background: #f5f5f5; padding: 10px; border-radius: 4px; margin-top: 20px; font-size: 0.9em; }
        .isp { color: #999; font-size: 0.85em; margin-top: 30px; }
    </style>
</head>
<body>
    <div class="error-box">
        <h1>504 Gateway Timeout</h1>
        <p>The upstream server is taking too long to respond.</p>
        <p>Please try again later or contact your network administrator.</p>
        <div class="code">Error Code: ETIMEDOUT_UPSTREAM_%d<br>Timestamp: %s</div>
        <div class="isp">Network: %s</div>
    </div>
</body>
</html>`,
		config.Language,
		rand.Intn(999)+1,
		time.Now().Format(time.RFC3339),
		isp,
	)
}

// GetResourceSinkRatio 计算资源损耗比
func (ti *TruthInjector) GetResourceSinkRatio(requestCount int64, responseBytes int64) float64 {
	if requestCount == 0 {
		return 0
	}
	// 每个请求平均消耗的响应字节数
	avgResponseSize := float64(responseBytes) / float64(requestCount)
	// 假设攻击者每个请求消耗 1KB 带宽，我们返回的数据量比例
	return avgResponseSize / 1024.0
}
