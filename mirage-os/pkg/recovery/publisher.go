// Package recovery - 独立恢复发布平面
// 受击节点即使完全假死，Client 仍能通过多通道获得新的有效入口
package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// GatewayEndpoint 网关端点
type GatewayEndpoint struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// ResonanceSignal 共振信号
type ResonanceSignal struct {
	Gateways []GatewayEndpoint `json:"gateways"`
	Version  int64             `json:"version"`
	CellID   string            `json:"cell_id"`
}

// PublishChannel 发布通道接口
type PublishChannel interface {
	Publish(ctx context.Context, signal *ResonanceSignal) error
	Name() string
}

// RecoveryPublisher 恢复拓扑发布器
type RecoveryPublisher struct {
	channels []PublishChannel
	mu       sync.RWMutex
}

// NewRecoveryPublisher 创建恢复发布器
func NewRecoveryPublisher(channels ...PublishChannel) *RecoveryPublisher {
	return &RecoveryPublisher{
		channels: channels,
	}
}

// PublishReplacement 通过所有通道并发发布替补拓扑
func (rp *RecoveryPublisher) PublishReplacement(ctx context.Context, cellID string, newGateways []GatewayEndpoint) error {
	rp.mu.RLock()
	channels := make([]PublishChannel, len(rp.channels))
	copy(channels, rp.channels)
	rp.mu.RUnlock()

	signal := &ResonanceSignal{
		Gateways: newGateways,
		Version:  time.Now().UnixNano(),
		CellID:   cellID,
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(channels))

	for _, ch := range channels {
		wg.Add(1)
		go func(c PublishChannel) {
			defer wg.Done()
			if err := c.Publish(ctx, signal); err != nil {
				log.Printf("[RecoveryPublisher] ⚠️ %s 发布失败: %v", c.Name(), err)
				errCh <- err
			} else {
				log.Printf("[RecoveryPublisher] ✅ %s 发布成功: cell=%s, nodes=%d",
					c.Name(), cellID, len(newGateways))
			}
		}(ch)
	}

	wg.Wait()
	close(errCh)

	// 只要有一个通道成功就算成功
	var lastErr error
	failCount := 0
	for err := range errCh {
		lastErr = err
		failCount++
	}

	if failCount == len(channels) {
		return fmt.Errorf("all publish channels failed, last: %w", lastErr)
	}

	return nil
}

// DNSTXTChannel DNS TXT 记录发布通道
type DNSTXTChannel struct {
	domain    string
	apiKey    string
	apiSecret string
}

// NewDNSTXTChannel 创建 DNS TXT 通道
func NewDNSTXTChannel(domain, apiKey, apiSecret string) *DNSTXTChannel {
	return &DNSTXTChannel{domain: domain, apiKey: apiKey, apiSecret: apiSecret}
}

// Name 返回通道名称
func (d *DNSTXTChannel) Name() string { return "DNS_TXT" }

// Publish 通过 DNS TXT 记录发布信号
func (d *DNSTXTChannel) Publish(ctx context.Context, signal *ResonanceSignal) error {
	data, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("marshal signal: %w", err)
	}
	// DNS TXT 记录更新（实际实现需要调用 DNS API）
	log.Printf("[DNS_TXT] 发布信号到 %s: %d bytes", d.domain, len(data))
	_ = data
	return nil
}

// GistChannel GitHub Gist 发布通道
type GistChannel struct {
	gistID string
	token  string
}

// NewGistChannel 创建 Gist 通道
func NewGistChannel(gistID, token string) *GistChannel {
	return &GistChannel{gistID: gistID, token: token}
}

// Name 返回通道名称
func (g *GistChannel) Name() string { return "Gist" }

// Publish 通过 Gist 发布信号
func (g *GistChannel) Publish(ctx context.Context, signal *ResonanceSignal) error {
	data, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("marshal signal: %w", err)
	}
	// Gist 更新（实际实现需要调用 GitHub API）
	log.Printf("[Gist] 发布信号到 gist %s: %d bytes", g.gistID, len(data))
	_ = data
	return nil
}
