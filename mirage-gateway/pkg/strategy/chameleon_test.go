package strategy

import (
	"testing"
)

func TestChameleon_LoadProfiles(t *testing.T) {
	c := NewChameleon()
	
	if len(c.profiles) == 0 {
		t.Fatal("未加载任何配置文件")
	}
	
	expectedProfiles := []string{"zoom-windows", "chrome-windows", "teams-windows"}
	for _, name := range expectedProfiles {
		if _, ok := c.profiles[name]; !ok {
			t.Errorf("缺少配置文件: %s", name)
		}
	}
}

func TestChameleon_SetProfile(t *testing.T) {
	c := NewChameleon()
	
	// 测试设置有效配置文件
	err := c.SetProfile("zoom-windows")
	if err != nil {
		t.Fatalf("设置配置文件失败: %v", err)
	}
	
	if c.active == nil {
		t.Fatal("活跃配置文件为空")
	}
	
	if c.active.Name != "Zoom Windows Client" {
		t.Errorf("配置文件名称不匹配: %s", c.active.Name)
	}
	
	// 测试设置无效配置文件
	err = c.SetProfile("invalid-profile")
	if err == nil {
		t.Fatal("应该返回错误")
	}
}

func TestChameleon_GenerateTLSClientHello(t *testing.T) {
	c := NewChameleon()
	c.SetProfile("zoom-windows")
	
	clientHello, err := c.GenerateTLSClientHello()
	if err != nil {
		t.Fatalf("生成 ClientHello 失败: %v", err)
	}
	
	if len(clientHello) == 0 {
		t.Fatal("ClientHello 为空")
	}
	
	// 检查 TLS 记录类型
	if clientHello[0] != 0x16 {
		t.Errorf("TLS 记录类型错误: 0x%02x", clientHello[0])
	}
	
	// 检查 Handshake 类型
	if clientHello[5] != 0x01 {
		t.Errorf("Handshake 类型错误: 0x%02x", clientHello[5])
	}
}

func TestChameleon_GenerateQUICConnectionID(t *testing.T) {
	c := NewChameleon()
	c.SetProfile("zoom-windows")
	
	connID := c.GenerateQUICConnectionID()
	if connID == nil {
		t.Fatal("连接 ID 为空")
	}
	
	if len(connID) != 8 {
		t.Errorf("连接 ID 长度错误: %d", len(connID))
	}
}

func TestChameleon_GetJA4Fingerprint(t *testing.T) {
	c := NewChameleon()
	c.SetProfile("zoom-windows")
	
	fingerprint := c.GetJA4Fingerprint()
	if fingerprint == "" {
		t.Fatal("JA4 指纹为空")
	}
	
	t.Logf("JA4 指纹: %s", fingerprint)
}

func TestContentPool_GenerateNoisePacket(t *testing.T) {
	pool := NewContentPool()
	
	sizes := []int{128, 256, 512, 1024}
	for _, size := range sizes {
		packet := pool.GenerateNoisePacket(size)
		if len(packet) != size {
			t.Errorf("包大小不匹配: 期望 %d, 实际 %d", size, len(packet))
		}
	}
}

func TestContentPool_GenerateHTTPLikePacket(t *testing.T) {
	pool := NewContentPool()
	
	packet := pool.GenerateHTTPLikePacket()
	if len(packet) == 0 {
		t.Fatal("HTTP 包为空")
	}
	
	// 检查是否包含 HTTP 关键字
	packetStr := string(packet)
	if !contains(packetStr, "HTTP/1.1") {
		t.Error("不包含 HTTP/1.1")
	}
}

func TestContentPool_GenerateJSONLikePacket(t *testing.T) {
	pool := NewContentPool()
	
	packet := pool.GenerateJSONLikePacket()
	if len(packet) == 0 {
		t.Fatal("JSON 包为空")
	}
	
	// 检查是否为有效 JSON
	packetStr := string(packet)
	if packetStr[0] != '{' || packetStr[len(packetStr)-1] != '}' {
		t.Error("不是有效的 JSON")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
