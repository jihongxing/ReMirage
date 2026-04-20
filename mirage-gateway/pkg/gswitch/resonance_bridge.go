// Package gswitch - 共振桥接器
// 将 ResonanceResolver 集成到 GSwitchManager 的失联检测逻辑中
// 当主连接失败 N 次后，自动触发共振发现并切换到新 Gateway
package gswitch

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ResonanceBridge 共振桥接器
type ResonanceBridge struct {
	resolver   *ResonanceResolver
	gswitchMgr *GSwitchManager

	// 失联检测
	consecutiveFails atomic.Int32
	failThreshold    int32 // 连续失败 N 次触发共振发现

	// 状态
	isResolving atomic.Bool
	mu          sync.Mutex

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
}

// NewResonanceBridge 创建桥接器
func NewResonanceBridge(resolver *ResonanceResolver, gswitchMgr *GSwitchManager) *ResonanceBridge {
	ctx, cancel := context.WithCancel(context.Background())
	rb := &ResonanceBridge{
		resolver:      resolver,
		gswitchMgr:    gswitchMgr,
		failThreshold: 3,
		ctx:           ctx,
		cancel:        cancel,
	}

	// 注册解析成功回调：获取新 IP 后更新 GSwitchManager
	resolver.SetOnResolved(func(payload *SignalPayload) {
		rb.applyNewGateways(payload)
	})

	return rb
}

// ReportConnectionFailure 上报连接失败（由传输层调用）
func (rb *ResonanceBridge) ReportConnectionFailure() {
	fails := rb.consecutiveFails.Add(1)
	if fails >= rb.failThreshold && !rb.isResolving.Load() {
		log.Printf("[ResonanceBridge] 连续失败 %d 次，触发共振发现", fails)
		go rb.triggerResonance()
	}
}

// ReportConnectionSuccess 上报连接成功（重置计数器）
func (rb *ResonanceBridge) ReportConnectionSuccess() {
	rb.consecutiveFails.Store(0)
}

// triggerResonance 触发共振发现
func (rb *ResonanceBridge) triggerResonance() {
	if !rb.isResolving.CompareAndSwap(false, true) {
		return // 已有解析在进行
	}
	defer rb.isResolving.Store(false)

	result, err := rb.resolver.Resolve(rb.ctx)
	if err != nil {
		log.Printf("[ResonanceBridge] ⚠️ 共振发现失败: %v", err)
		return
	}

	log.Printf("[ResonanceBridge] ✅ 共振发现成功: %d 个网关, %d 个域名 (via %s, %v)",
		len(result.Payload.Gateways), len(result.Payload.Domains),
		result.Channel, result.Latency)

	// 重置失败计数
	rb.consecutiveFails.Store(0)
}

// applyNewGateways 应用新网关列表
func (rb *ResonanceBridge) applyNewGateways(payload *SignalPayload) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// 1. 将新域名导入 GSwitchManager 热备池
	for _, domain := range payload.Domains {
		rb.gswitchMgr.AddDomain(domain, "")
	}

	// 2. 将新 Gateway IP 注入域名池（带优先级）
	for _, gw := range payload.Gateways {
		ip := net.IPv4(gw.IP[0], gw.IP[1], gw.IP[2], gw.IP[3]).String()
		name := fmt.Sprintf("gw-%s:%d", ip, gw.Port)
		rb.gswitchMgr.AddDomain(name, ip)
	}

	// 3. 如果当前无活跃域名，立即激活优先级最高的
	if rb.gswitchMgr.GetCurrentDomain() == nil && len(payload.Gateways) > 0 {
		gw := payload.Gateways[0]
		ip := net.IPv4(gw.IP[0], gw.IP[1], gw.IP[2], gw.IP[3]).String()
		domain := &Domain{
			Name:      fmt.Sprintf("gw-%s:%d", ip, gw.Port),
			IP:        ip,
			Status:    DomainActive,
			CreatedAt: time.Now(),
		}
		if err := rb.gswitchMgr.ActivateDomain(domain); err != nil {
			log.Printf("[ResonanceBridge] ⚠️ 激活新网关失败: %v", err)
		}
	}
}

// Stop 停止
func (rb *ResonanceBridge) Stop() {
	rb.cancel()
}
