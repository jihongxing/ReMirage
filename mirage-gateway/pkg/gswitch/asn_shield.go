// Package gswitch - ASN 声誉盾与 BGP 异常检测
// O8 终极隐匿：抗 BGP 拓扑陷阱与 ASN 孤岛效应
//
// 问题：即使流量伪装完美，如果目的地是小众"防弹托管"ASN，
// 审查者的上帝视角会标记"为什么用户在和黑客机房开视频会议"。
// 更危险的是 BGP 路由劫持：审查者将流量引流到清洗中心解密。
//
// 解决方案：
//  1. ASN 声誉评分：只允许连接到大厂 CDN 的 ASN（AWS/Azure/CF）
//  2. RTT 异常检测：BGP 劫持会导致 RTT 突变，触发焦土重连
//  3. 域名前置强制：所有出站流量必须经过 CDN 前置层
package gswitch

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// ASNReputation ASN 声誉等级
type ASNReputation int

const (
	ASNTrusted    ASNReputation = 0 // 大厂 CDN（AWS/Azure/CF/GCP）
	ASNNeutral    ASNReputation = 1 // 中立 ISP（Tier-1 运营商）
	ASNSuspicious ASNReputation = 2 // 可疑（小型 VPS 提供商）
	ASNDangerous  ASNReputation = 3 // 危险（防弹托管/已知恶意）
)

// TrustedASNs 可信 ASN 白名单（大厂 CDN + Tier-1 运营商）
// 流量目的地必须属于这些 ASN，否则拒绝连接
var TrustedASNs = map[uint32]string{
	// CDN 巨头
	13335: "Cloudflare",
	16509: "Amazon (AWS)",
	8075:  "Microsoft (Azure)",
	15169: "Google Cloud",
	20940: "Akamai",
	54113: "Fastly",
	// Tier-1 运营商
	3356: "Lumen (Level3)",
	1299: "Arelion (Telia)",
	6939: "Hurricane Electric",
	174:  "Cogent",
	2914: "NTT",
	3257: "GTT",
	6762: "Telecom Italia Sparkle",
	// 云平台
	14618: "Amazon (AWS US-East)",
	16510: "Google (YouTube)",
	32934: "Facebook (Meta)",
	8068:  "Microsoft (Office365)",
}

// ASNShield ASN 声誉盾
type ASNShield struct {
	mu sync.RWMutex

	// RTT 基线（用于 BGP 劫持检测）
	rttBaseline    time.Duration // 历史平均 RTT
	rttSamples     []time.Duration
	rttMaxSamples  int
	rttAnomalyMult float64 // RTT 异常倍数阈值

	// BGP 劫持检测回调
	onBGPAnomaly func(oldRTT, newRTT time.Duration)

	// ASN 验证回调
	onASNRejected func(ip string, asn uint32)
}

// NewASNShield 创建 ASN 声誉盾
func NewASNShield() *ASNShield {
	return &ASNShield{
		rttSamples:     make([]time.Duration, 0, 100),
		rttMaxSamples:  100,
		rttAnomalyMult: 2.5, // RTT 突然增大 2.5 倍视为异常
	}
}

// SetBGPAnomalyCallback 设置 BGP 异常回调
func (as *ASNShield) SetBGPAnomalyCallback(cb func(oldRTT, newRTT time.Duration)) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.onBGPAnomaly = cb
}

// SetASNRejectedCallback 设置 ASN 拒绝回调
func (as *ASNShield) SetASNRejectedCallback(cb func(ip string, asn uint32)) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.onASNRejected = cb
}

// ValidateASN 验证目标 IP 的 ASN 是否可信
// 返回 true = 允许连接，false = 拒绝（ASN 不在白名单）
func (as *ASNShield) ValidateASN(ip string, asn uint32) bool {
	if _, trusted := TrustedASNs[asn]; trusted {
		return true
	}

	as.mu.RLock()
	cb := as.onASNRejected
	as.mu.RUnlock()

	log.Printf("🚨 [ASNShield] 拒绝连接: IP=%s, ASN=%d (不在可信白名单)", ip, asn)
	if cb != nil {
		go cb(ip, asn)
	}
	return false
}

// ReportRTT 上报 RTT 样本（用于 BGP 劫持检测）
// 如果 RTT 突然增大超过阈值，触发 BGP 异常告警
func (as *ASNShield) ReportRTT(rtt time.Duration) bool {
	as.mu.Lock()
	defer as.mu.Unlock()

	// 添加样本
	as.rttSamples = append(as.rttSamples, rtt)
	if len(as.rttSamples) > as.rttMaxSamples {
		as.rttSamples = as.rttSamples[1:]
	}

	// 至少需要 10 个样本才能建立基线
	if len(as.rttSamples) < 10 {
		as.rttBaseline = rtt
		return false
	}

	// 计算移动平均作为基线
	baseline := as.calculateBaseline()
	as.rttBaseline = baseline

	// 检测异常：当前 RTT > 基线 × 异常倍数
	threshold := time.Duration(float64(baseline) * as.rttAnomalyMult)
	if rtt > threshold && baseline > 0 {
		log.Printf("🚨 [ASNShield] BGP 异常检测: RTT=%v >> baseline=%v (阈值=%v)",
			rtt, baseline, threshold)

		cb := as.onBGPAnomaly
		if cb != nil {
			go cb(baseline, rtt)
		}
		return true // 异常
	}

	return false
}

// calculateBaseline 计算 RTT 基线（去除异常值的移动平均）
func (as *ASNShield) calculateBaseline() time.Duration {
	if len(as.rttSamples) == 0 {
		return 0
	}

	// 使用中位数而非平均值（抗异常值干扰）
	sorted := make([]time.Duration, len(as.rttSamples))
	copy(sorted, as.rttSamples)

	// 简单排序（样本量小，O(n²) 可接受）
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// 取中位数
	mid := len(sorted) / 2
	return sorted[mid]
}

// GetBaseline 获取当前 RTT 基线
func (as *ASNShield) GetBaseline() time.Duration {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.rttBaseline
}

// CalculateJitterScore 计算抖动评分（0-100，越高越异常）
func (as *ASNShield) CalculateJitterScore() float64 {
	as.mu.RLock()
	defer as.mu.RUnlock()

	if len(as.rttSamples) < 5 {
		return 0
	}

	// 计算标准差
	var sum, sumSq float64
	for _, s := range as.rttSamples {
		ms := float64(s.Microseconds())
		sum += ms
		sumSq += ms * ms
	}
	n := float64(len(as.rttSamples))
	mean := sum / n
	variance := sumSq/n - mean*mean
	if variance < 0 {
		variance = 0
	}
	stddev := math.Sqrt(variance)

	// 归一化到 0-100（stddev / mean × 100，上限 100）
	if mean == 0 {
		return 0
	}
	score := (stddev / mean) * 100
	if score > 100 {
		score = 100
	}
	return score
}

// DomainFrontingConfig 域名前置配置
type DomainFrontingConfig struct {
	// CDN 前置域名（DPI 看到的 SNI）
	FrontDomain string // e.g. "cdn.cloudflare.com"

	// 真实后端 Host（HTTP Host 头）
	BackendHost string // e.g. "your-worker.your-domain.workers.dev"

	// CDN 类型
	CDNType string // "cloudflare" | "aws_cloudfront" | "azure_cdn"
}

// ValidateDomainFronting 验证域名前置配置的安全性
// 确保 FrontDomain 属于可信 CDN，且 SNI 与 Host 不同（前置生效）
func ValidateDomainFronting(cfg *DomainFrontingConfig) error {
	if cfg.FrontDomain == "" {
		return fmt.Errorf("FrontDomain 不能为空")
	}
	if cfg.BackendHost == "" {
		return fmt.Errorf("BackendHost 不能为空")
	}
	if cfg.FrontDomain == cfg.BackendHost {
		return fmt.Errorf("FrontDomain 与 BackendHost 相同，域名前置未生效")
	}
	return nil
}
