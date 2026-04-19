package phantom

import "github.com/cilium/ebpf"

// MapProvider 提供 eBPF Map 查询能力的接口
type MapProvider interface {
	GetMap(name string) *ebpf.Map
}

// BuildMapSet 从 Loader 构建 Phantom 所需的 Map 集合
func BuildMapSet(provider MapProvider) map[string]*ebpf.Map {
	return map[string]*ebpf.Map{
		"phishing_list_map": provider.GetMap("phishing_list_map"),
		"honeypot_config":   provider.GetMap("honeypot_config"),
		"phantom_stats":     provider.GetMap("phantom_stats"),
		"phantom_events":    provider.GetMap("phantom_events"),
	}
}
