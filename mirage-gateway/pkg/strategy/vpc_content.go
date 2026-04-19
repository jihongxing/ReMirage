// Package strategy - VPC 内容注入
package strategy

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
)

// ContentPool 内容池
type ContentPool struct {
	webFragments  [][]byte
	jsonFragments [][]byte
	apiFragments  [][]byte
}

// NewContentPool 创建内容池
func NewContentPool() *ContentPool {
	pool := &ContentPool{
		webFragments:  make([][]byte, 0),
		jsonFragments: make([][]byte, 0),
		apiFragments:  make([][]byte, 0),
	}
	
	pool.loadFragments()
	
	return pool
}

// loadFragments 加载内容片段
func (cp *ContentPool) loadFragments() {
	// Web 页面片段
	cp.webFragments = [][]byte{
		[]byte(`<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Loading...</title>`),
		[]byte(`<script src="/static/js/main.js"></script><link rel="stylesheet" href="/css/style.css">`),
		[]byte(`<div class="container"><div class="row"><div class="col-md-12"><h1>Welcome</h1></div></div></div>`),
		[]byte(`<footer><p>&copy; 2024 Company Inc. All rights reserved.</p></footer></body></html>`),
		[]byte(`<nav class="navbar"><ul><li><a href="/">Home</a></li><li><a href="/about">About</a></li></ul></nav>`),
	}
	
	// JSON API 响应片段
	cp.jsonFragments = [][]byte{
		[]byte(`{"status":"success","data":{"id":12345,"name":"user","email":"user@example.com"}}`),
		[]byte(`{"error":null,"result":{"items":[{"id":1,"title":"Item 1"},{"id":2,"title":"Item 2"}]}}`),
		[]byte(`{"timestamp":1234567890,"version":"1.0.0","api":"v2","endpoint":"/api/data"}`),
		[]byte(`{"pagination":{"page":1,"per_page":20,"total":100},"data":[]}`),
		[]byte(`{"meta":{"request_id":"abc123","duration_ms":45},"payload":{}}`),
	}
	
	// API 请求片段
	cp.apiFragments = [][]byte{
		[]byte(`GET /api/v1/users HTTP/1.1\r\nHost: api.example.com\r\nUser-Agent: Mozilla/5.0\r\n\r\n`),
		[]byte(`POST /api/v1/auth HTTP/1.1\r\nContent-Type: application/json\r\nContent-Length: 45\r\n\r\n`),
		[]byte(`{"username":"user","password":"********","remember":true}`),
		[]byte(`Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0`),
	}
}

// GetRandomFragment 获取随机片段
func (cp *ContentPool) GetRandomFragment(fragmentType string) []byte {
	var pool [][]byte
	
	switch fragmentType {
	case "web":
		pool = cp.webFragments
	case "json":
		pool = cp.jsonFragments
	case "api":
		pool = cp.apiFragments
	default:
		pool = cp.webFragments
	}
	
	if len(pool) == 0 {
		return nil
	}
	
	// 随机选择
	idx := randomInt(len(pool))
	return pool[idx]
}

// GenerateNoisePacket 生成噪声包
func (cp *ContentPool) GenerateNoisePacket(size int) []byte {
	packet := make([]byte, 0, size)
	
	// 随机选择内容类型
	contentTypes := []string{"web", "json", "api"}
	contentType := contentTypes[randomInt(len(contentTypes))]
	
	// 填充内容
	for len(packet) < size {
		fragment := cp.GetRandomFragment(contentType)
		if fragment == nil {
			break
		}
		
		// 如果剩余空间不足，截断
		remaining := size - len(packet)
		if len(fragment) > remaining {
			fragment = fragment[:remaining]
		}
		
		packet = append(packet, fragment...)
	}
	
	// 如果还有空间，用随机数据填充
	if len(packet) < size {
		padding := make([]byte, size-len(packet))
		rand.Read(padding)
		packet = append(packet, padding...)
	}
	
	return packet
}

// GenerateHTTPLikePacket 生成类 HTTP 包
func (cp *ContentPool) GenerateHTTPLikePacket() []byte {
	// HTTP 请求行
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	paths := []string{"/api/v1/data", "/api/v2/users", "/health", "/metrics", "/status"}
	
	method := methods[randomInt(len(methods))]
	path := paths[randomInt(len(paths))]
	
	request := fmt.Sprintf("%s %s HTTP/1.1\r\n", method, path)
	request += "Host: api.example.com\r\n"
	request += "User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64)\r\n"
	request += "Accept: application/json\r\n"
	request += "Connection: keep-alive\r\n"
	
	// 随机添加 body
	if method == "POST" || method == "PUT" {
		body := cp.GetRandomFragment("json")
		request += "Content-Type: application/json\r\n"
		request += fmt.Sprintf("Content-Length: %d\r\n", len(body))
		request += "\r\n"
		request += string(body)
	} else {
		request += "\r\n"
	}
	
	return []byte(request)
}

// GenerateJSONLikePacket 生成类 JSON 包
func (cp *ContentPool) GenerateJSONLikePacket() []byte {
	// 构造看起来合法的 JSON
	data := map[string]interface{}{
		"timestamp": randomInt(1000000000),
		"request_id": fmt.Sprintf("%x", randomBytes(16)),
		"status": "ok",
		"data": map[string]interface{}{
			"id":    randomInt(100000),
			"value": randomInt(1000),
			"items": []int{randomInt(10), randomInt(10), randomInt(10)},
		},
	}
	
	jsonData, _ := json.Marshal(data)
	return jsonData
}

// GenerateWebLikePacket 生成类 Web 包
func (cp *ContentPool) GenerateWebLikePacket() []byte {
	fragments := [][]byte{
		[]byte(`<!DOCTYPE html><html><head><meta charset="UTF-8">`),
		[]byte(`<title>Page Title</title>`),
		[]byte(`<link rel="stylesheet" href="/css/style.css">`),
		[]byte(`<script src="/js/app.js"></script>`),
		[]byte(`</head><body>`),
		cp.GetRandomFragment("web"),
		[]byte(`</body></html>`),
	}
	
	packet := make([]byte, 0, 512)
	for _, frag := range fragments {
		packet = append(packet, frag...)
	}
	
	return packet
}

// randomInt 生成随机整数
func randomInt(max int) int {
	if max <= 0 {
		return 0
	}
	
	b := make([]byte, 4)
	rand.Read(b)
	
	n := int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24
	if n < 0 {
		n = -n
	}
	
	return n % max
}

// randomBytes 生成随机字节
func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}
