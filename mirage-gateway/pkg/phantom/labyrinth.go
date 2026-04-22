// Package phantom 无限迷宫引擎
// 生成语义化的无限深度 API 路径，消耗攻击者资源
package phantom

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MaxLabyrinthDepth 迷宫最大深度
const MaxLabyrinthDepth = 5

// LabyrinthEngine 有限迷宫引擎
type LabyrinthEngine struct {
	mu sync.RWMutex

	// 业务画像
	persona Persona

	// 最大深度
	maxDepth int

	// 路径深度统计
	depthStats map[string]int // IP -> 最大深度

	// 延迟配置
	baseDelay   time.Duration
	delayFactor float64 // 指数增长因子
	maxDelay    time.Duration

	// 语义词库
	pathSegments []string
	entityTypes  []string
	actionTypes  []string

	// 回调
	onDeepDive func(ip string, depth int, path string)
}

// NewLabyrinthEngine 创建迷宫引擎
func NewLabyrinthEngine() *LabyrinthEngine {
	return &LabyrinthEngine{
		persona:     DefaultPersona,
		maxDepth:    MaxLabyrinthDepth,
		depthStats:  make(map[string]int),
		baseDelay:   50 * time.Millisecond,
		delayFactor: 1.5,
		maxDelay:    3 * time.Second,
		pathSegments: []string{
			"api", "v2", "v3", "internal", "admin", "system",
			"users", "accounts", "transactions", "audit",
			"logs", "archives", "backups", "exports",
			"config", "settings", "permissions", "roles",
		},
		entityTypes: []string{
			"user", "account", "session", "token", "key",
			"record", "entry", "item", "resource", "asset",
		},
		actionTypes: []string{
			"details", "history", "audit_logs", "metadata",
			"permissions", "access_log", "changelog", "revisions",
		},
	}
}

// Handler 返回迷宫 HTTP 处理器
func (l *LabyrinthEngine) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r.RemoteAddr)
		path := r.URL.Path
		depth := l.calculateDepth(path)

		// 更新深度统计
		l.mu.Lock()
		if depth > l.depthStats[ip] {
			l.depthStats[ip] = depth
		}
		l.mu.Unlock()

		// 计算指数延迟
		delay := l.calculateDelay(depth)
		time.Sleep(delay)

		// 回调
		if l.onDeepDive != nil {
			l.onDeepDive(ip, depth, path)
		}

		// 生成响应
		l.generateResponse(w, r, depth)
	})
}

// calculateDepth 计算路径深度
func (l *LabyrinthEngine) calculateDepth(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts)
}

// calculateDelay 计算指数延迟
func (l *LabyrinthEngine) calculateDelay(depth int) time.Duration {
	delay := l.baseDelay
	for i := 0; i < depth; i++ {
		delay = time.Duration(float64(delay) * l.delayFactor)
		if delay > l.maxDelay {
			return l.maxDelay
		}
	}
	return delay
}

// generateResponse 生成迷宫响应
func (l *LabyrinthEngine) generateResponse(w http.ResponseWriter, r *http.Request, depth int) {
	l.mu.RLock()
	maxDepth := l.maxDepth
	p := l.persona
	l.mu.RUnlock()

	// 超过最大深度，返回自然 404
	if depth > maxDepth {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "not_found",
			"message": "The requested resource does not exist.",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// 生成伪造数据
	data := l.generateFakeData(depth)

	response := map[string]interface{}{
		"status":  "success",
		"data":    data,
		"page":    1,
		"total":   len(data.([]map[string]interface{})),
		"version": p.APIVersion,
		"service": p.CompanyName,
	}

	// 仅在未达最大深度时包含 next 链接（自然分页风格）
	if depth < maxDepth {
		response["next"] = fmt.Sprintf("%s?page=2", r.URL.Path)
	}

	json.NewEncoder(w).Encode(response)
}

// generateFakeData 生成伪造数据
func (l *LabyrinthEngine) generateFakeData(depth int) interface{} {
	count := 10 - depth
	if count < 3 {
		count = 3
	}

	items := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		items[i] = map[string]interface{}{
			"id":         fmt.Sprintf("%s_%s", l.randomEntity(), l.randomID()),
			"created_at": time.Now().Add(-time.Duration(i*24) * time.Hour).Format(time.RFC3339),
			"updated_at": time.Now().Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
			"status":     l.randomStatus(),
			"metadata": map[string]interface{}{
				"source":   "internal",
				"verified": true,
				"level":    depth,
			},
		}
	}
	return items
}

// generateDeeperLinks 生成更深层链接
func (l *LabyrinthEngine) generateDeeperLinks(currentPath string, count int) []map[string]string {
	links := make([]map[string]string, count)
	for i := 0; i < count; i++ {
		segment := l.generateSemanticSegment()
		links[i] = map[string]string{
			"rel":  l.randomAction(),
			"href": fmt.Sprintf("%s/%s", currentPath, segment),
			"type": "application/json",
		}
	}
	return links
}

// generateSemanticSegment 生成语义化路径段
func (l *LabyrinthEngine) generateSemanticSegment() string {
	patterns := []func() string{
		func() string { return fmt.Sprintf("%s_%s", l.randomEntity(), l.randomID()) },
		func() string { return l.randomAction() },
		func() string { return fmt.Sprintf("part_%d", l.randomInt(100)) },
		func() string { return fmt.Sprintf("archive_%s", l.randomID()[:8]) },
		func() string { return fmt.Sprintf("%s_v%d", l.randomSegment(), l.randomInt(5)+1) },
	}
	return patterns[l.randomInt(len(patterns))]()
}

// generatePagination 生成分页信息
func (l *LabyrinthEngine) generatePagination(depth int) map[string]interface{} {
	total := 1000 - depth*50
	if total < 100 {
		total = 100
	}
	return map[string]interface{}{
		"page":        1,
		"per_page":    20,
		"total":       total,
		"total_pages": total / 20,
		"has_more":    true,
	}
}

func (l *LabyrinthEngine) randomSegment() string {
	return l.pathSegments[l.randomInt(len(l.pathSegments))]
}

func (l *LabyrinthEngine) randomEntity() string {
	return l.entityTypes[l.randomInt(len(l.entityTypes))]
}

func (l *LabyrinthEngine) randomAction() string {
	return l.actionTypes[l.randomInt(len(l.actionTypes))]
}

func (l *LabyrinthEngine) randomStatus() string {
	statuses := []string{"active", "pending", "archived", "processing"}
	return statuses[l.randomInt(len(statuses))]
}

func (l *LabyrinthEngine) randomID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func (l *LabyrinthEngine) randomInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

// OnDeepDive 设置深度回调
func (l *LabyrinthEngine) OnDeepDive(fn func(ip string, depth int, path string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onDeepDive = fn
}

// GetDepthStats 获取深度统计
func (l *LabyrinthEngine) GetDepthStats() map[string]int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make(map[string]int, len(l.depthStats))
	for k, v := range l.depthStats {
		result[k] = v
	}
	return result
}

// SetDelayConfig 设置延迟配置
func (l *LabyrinthEngine) SetDelayConfig(base time.Duration, factor float64, max time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.baseDelay = base
	l.delayFactor = factor
	l.maxDelay = max
}

// SetPersona 设置业务画像
func (l *LabyrinthEngine) SetPersona(p Persona) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.persona = p
}

// SetMaxDepth 设置最大深度
func (l *LabyrinthEngine) SetMaxDepth(depth int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxDepth = depth
}

func extractIP(addr string) string {
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}
