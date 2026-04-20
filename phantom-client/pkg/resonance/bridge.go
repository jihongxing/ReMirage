// Package resonance - bridge.go
// 提供 SignalCrypto → OpenFunc 的适配桥接
// 将 mirage-gateway/pkg/gswitch.SignalCrypto 的 OpenSignal 输出转换为 resonance 包的 GatewayInfo 格式
//
// 使用方式：
//
//	import "mirage-gateway/pkg/gswitch"
//	crypto := gswitch.NewSignalCryptoResolver(verifyKey, decryptPrivKey)
//	openFn := resonance.BridgeSignalCrypto(crypto)
//	resolver := resonance.NewResolver(config, openFn)
package resonance

import (
	"fmt"
	"net"
)

// SignalPayloadLike 信令载荷接口（解耦 gswitch 包依赖）
type SignalPayloadLike struct {
	Gateways []RawGatewayEntry
	Domains  []string
}

// RawGatewayEntry 原始网关条目（与 gswitch.GatewayEntry 对齐）
type RawGatewayEntry struct {
	IP       [4]byte
	Port     uint16
	Priority uint8
}

// SignalCryptoOpener 信令解密器接口
type SignalCryptoOpener interface {
	OpenSignal(sealed []byte) (interface{}, error)
}

// BridgeOpenFunc 创建一个通用的 OpenFunc 桥接函数
// 接受一个 openSignal 函数，该函数返回的 interface{} 必须具有 Gateways 和 Domains 字段
// 实际使用时传入 gswitch.SignalCrypto.OpenSignal
func BridgeOpenFunc(openSignal func([]byte) (gateways []RawGatewayEntry, domains []string, err error)) OpenFunc {
	return func(sealed []byte) ([]GatewayInfo, []string, error) {
		rawGWs, domains, err := openSignal(sealed)
		if err != nil {
			return nil, nil, err
		}

		gateways := make([]GatewayInfo, 0, len(rawGWs))
		for _, raw := range rawGWs {
			ip := net.IPv4(raw.IP[0], raw.IP[1], raw.IP[2], raw.IP[3]).String()
			gateways = append(gateways, GatewayInfo{
				IP:       ip,
				Port:     int(raw.Port),
				Priority: raw.Priority,
			})
		}

		return gateways, domains, nil
	}
}

// MakeOpenFunc 最简桥接：直接包装一个 func([]byte) → (gateways, domains, error)
// 用于直接对接 SignalCrypto.OpenSignal 的返回值
func MakeOpenFunc(openSignalRaw func(sealed []byte) (interface{}, error)) OpenFunc {
	return func(sealed []byte) ([]GatewayInfo, []string, error) {
		result, err := openSignalRaw(sealed)
		if err != nil {
			return nil, nil, err
		}

		// 通过类型断言提取字段（SignalPayload 结构）
		type payloadAccessor interface {
			GetGateways() []RawGatewayEntry
			GetDomains() []string
		}

		if p, ok := result.(payloadAccessor); ok {
			rawGWs := p.GetGateways()
			gateways := make([]GatewayInfo, 0, len(rawGWs))
			for _, raw := range rawGWs {
				ip := net.IPv4(raw.IP[0], raw.IP[1], raw.IP[2], raw.IP[3]).String()
				gateways = append(gateways, GatewayInfo{
					IP:       ip,
					Port:     int(raw.Port),
					Priority: raw.Priority,
				})
			}
			return gateways, p.GetDomains(), nil
		}

		return nil, nil, fmt.Errorf("unsupported signal payload type: %T", result)
	}
}
