// Package ebpf - eBPF 加载器与管理器
// Go 控制面：负责加载 C 数据面到内核
//
// 加载策略（MapReplacements 纯内存动态链接）：
//   - jitter.o 作为"Map 母体"，首个加载，真正创建 common.h 中的所有共享 Map
//   - 后续 .o (npm, bdna, phantom, chameleon, h3_shaper) 通过 MapReplacements
//     将同名 Map 指针替换为母体实例，实现纯内存级共享
//   - sockmap.o 完全独立（不引用 common.h），单独加载
//   - 进程退出时所有 Map 随 FD 关闭自动销毁，绝对无痕
package ebpf

import (
	"fmt"
	"log"
	"os"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/vishvananda/netlink" // TC clsact qdisc + BPF filter 挂载，Linux 专用
)

// common.h 中声明的所有共享 Map 名称
var sharedMapNames = []string{
	"ctrl_map",
	"npm_config_map",
	"jitter_config_map",
	"dna_template_map",
	"vpc_config_map",
	"traffic_stats",
	"quota_map",
	"cell_phase_map",
	"emergency_ctrl_map",
	"global_policy_map",
	"ghost_mode_map",
	"threat_events",
	"perf_stats_events",
}

// bpfProgram 描述一个待加载的 BPF 程序
type bpfProgram struct {
	name     string // 人类可读名称
	path     string // .o 文件路径
	critical bool   // true = 加载失败终止; false = 降级
	attachFn func(l *Loader, objs *ebpf.Collection) error
}

// Loader eBPF 加载器
type Loader struct {
	collections []*ebpf.Collection
	links       []link.Link
	tcFilters   []netlink.Filter
	iface       string
	ifaceIdx    int
	cgroupPath  string
	maps        map[string]*ebpf.Map // 所有 Map 的统一索引
	sharedMaps  map[string]*ebpf.Map // common.h 共享 Map（母体实例）
}

// NewLoader 创建加载器
func NewLoader(iface string) *Loader {
	return &Loader{
		iface:      iface,
		cgroupPath: "/sys/fs/cgroup",
		maps:       make(map[string]*ebpf.Map),
		sharedMaps: make(map[string]*ebpf.Map),
	}
}

// LoadAndAttach 加载并挂载所有 eBPF 程序
func (l *Loader) LoadAndAttach() error {
	// 0. 移除内存限制
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("移除内存限制失败: %w", err)
	}

	// 1. 解析网卡索引
	iface, err := netlink.LinkByName(l.iface)
	if err != nil {
		return fmt.Errorf("获取网卡 %s 失败: %w", l.iface, err)
	}
	l.ifaceIdx = iface.Attrs().Index

	// 2. 创建 clsact qdisc（TC 挂载前置条件）
	if err := l.ensureClsactQdisc(iface); err != nil {
		return fmt.Errorf("创建 clsact qdisc 失败: %w", err)
	}

	// ============================================================
	// 3. 加载 Map 母体：jitter.o（首个加载，创建所有共享 Map）
	// ============================================================
	if err := l.loadMasterProgram(); err != nil {
		return fmt.Errorf("[FATAL] 加载 Map 母体 (jitter.o) 失败: %w", err)
	}

	// ============================================================
	// 4. 加载后续程序（通过 MapReplacements 共享母体 Map）
	// ============================================================
	followers := []bpfProgram{
		{
			name:     "NPM 流量伪装 (XDP)",
			path:     "bpf/npm.o",
			critical: true,
			attachFn: l.attachNPM,
		},
		{
			name:     "B-DNA 指纹拟态 (TC)",
			path:     "bpf/bdna.o",
			critical: true,
			attachFn: l.attachBDNA,
		},
		{
			name:     "Phantom 影子欺骗 (TC)",
			path:     "bpf/phantom.o",
			critical: false,
			attachFn: l.attachPhantom,
		},
		{
			name:     "Chameleon 协议变色龙 (TC)",
			path:     "bpf/chameleon.o",
			critical: false,
			attachFn: l.attachChameleon,
		},
		{
			name:     "H3 流量整形 (TC)",
			path:     "bpf/h3_shaper.o",
			critical: false,
			attachFn: l.attachH3Shaper,
		},
	}

	for _, prog := range followers {
		if err := l.loadFollowerProgram(prog); err != nil {
			if prog.critical {
				return fmt.Errorf("[FATAL] 加载 %s 失败: %w", prog.name, err)
			}
			log.Printf("⚠️  加载 %s 失败（降级运行）: %v", prog.name, err)
		} else {
			log.Printf("✅ %s 已加载并挂载", prog.name)
		}
	}

	// 5. 初始化 quota_map（打破 Hash Map 首次 Lookup 空指针）
	if err := l.initQuotaMap(); err != nil {
		log.Printf("⚠️  quota_map 初始化失败: %v", err)
	}

	// 6. 加载 Sockmap（完全独立，不共享 common.h Map）
	if err := l.loadSockmap(); err != nil {
		log.Printf("⚠️  Sockmap 加载失败（降级到用户态转发）: %v", err)
	}

	log.Printf("✅ eBPF 全量加载完成: %s (ifindex=%d), 共享 Map %d 个, 总 Map %d 个",
		l.iface, l.ifaceIdx, len(l.sharedMaps), len(l.maps))
	return nil
}

// loadMasterProgram 加载 Map 母体 (jitter.o)
// 它是第一个加载的 .o，负责真正创建 common.h 中的所有共享 Map
func (l *Loader) loadMasterProgram() error {
	masterPath := "bpf/jitter.o"
	if _, err := os.Stat(masterPath); os.IsNotExist(err) {
		return fmt.Errorf("%s 不存在，请先编译 (make bpf)", masterPath)
	}

	spec, err := ebpf.LoadCollectionSpec(masterPath)
	if err != nil {
		return fmt.Errorf("加载 jitter.o spec 失败: %w", err)
	}

	// 无 MapReplacements，直接创建所有 Map
	objs, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("创建 jitter collection 失败: %w", err)
	}
	l.collections = append(l.collections, objs)

	// 提取共享 Map 到母体字典
	for _, name := range sharedMapNames {
		if m, ok := objs.Maps[name]; ok {
			l.sharedMaps[name] = m
			l.maps[name] = m
		}
	}

	// 同时收集 jitter.o 自身的非共享 Map（如 vpc_noise_profiles 等）
	for name, m := range objs.Maps {
		if _, exists := l.maps[name]; !exists {
			l.maps[name] = m
		}
	}

	// 挂载 jitter TC 程序
	// TC egress: jitter_lite_egress（主时域扰动 + 配额熔断）
	if err := l.attachTCFilter(objs, "jitter_lite_egress", netlink.HANDLE_MIN_EGRESS); err != nil {
		return fmt.Errorf("挂载 jitter_lite_egress 失败: %w", err)
	}

	// TC ingress: vpc_ingress_detect（入口威胁检测）
	if err := l.attachTCFilter(objs, "vpc_ingress_detect", netlink.HANDLE_MIN_INGRESS); err != nil {
		log.Printf("⚠️  vpc_ingress_detect 挂载失败（降级）: %v", err)
	}

	log.Printf("✅ Map 母体 (jitter.o) 已加载: 共享 Map %d 个", len(l.sharedMaps))
	return nil
}

// loadFollowerProgram 加载后续 BPF 程序（通过 MapReplacements 共享母体 Map）
func (l *Loader) loadFollowerProgram(prog bpfProgram) error {
	if _, err := os.Stat(prog.path); os.IsNotExist(err) {
		return fmt.Errorf("%s 不存在，请先编译", prog.path)
	}

	spec, err := ebpf.LoadCollectionSpec(prog.path)
	if err != nil {
		return fmt.Errorf("加载 %s spec 失败: %w", prog.path, err)
	}

	// 核心：构建 MapReplacements
	// 将 spec 中与母体同名的 Map 替换为母体已创建的实例
	replacements := make(map[string]*ebpf.Map)
	for name := range spec.Maps {
		if sharedMap, ok := l.sharedMaps[name]; ok {
			replacements[name] = sharedMap
		}
	}

	opts := ebpf.CollectionOptions{
		MapReplacements: replacements,
	}

	objs, err := ebpf.NewCollectionWithOptions(spec, opts)
	if err != nil {
		return fmt.Errorf("创建 %s collection 失败: %w", prog.path, err)
	}
	l.collections = append(l.collections, objs)

	// 收集该 .o 独有的 Map（不覆盖已有的共享 Map）
	for name, m := range objs.Maps {
		if _, exists := l.maps[name]; !exists {
			l.maps[name] = m
		}
	}

	// 执行挂载
	if prog.attachFn != nil {
		return prog.attachFn(l, objs)
	}
	return nil
}

// ============================================================
// 各程序的挂载函数
// ============================================================

// attachNPM 挂载 NPM (XDP)
func (l *Loader) attachNPM(loader *Loader, objs *ebpf.Collection) error {
	// XDP 单入口：npm_xdp_main（合并了 padding + strip）
	if err := l.attachXDP(objs, "npm_xdp_main"); err != nil {
		return fmt.Errorf("挂载 npm_xdp_main 失败: %w", err)
	}
	return nil
}

// attachBDNA 挂载 B-DNA (TC)
func (l *Loader) attachBDNA(loader *Loader, objs *ebpf.Collection) error {
	// TC egress: bdna_tcp_rewrite（主防线）
	if err := l.attachTCFilter(objs, "bdna_tcp_rewrite", netlink.HANDLE_MIN_EGRESS); err != nil {
		return fmt.Errorf("挂载 bdna_tcp_rewrite 失败: %w", err)
	}
	// TC egress: bdna_quic_rewrite（辅助）
	if prog := objs.Programs["bdna_quic_rewrite"]; prog != nil {
		if err := l.attachTCFilter(objs, "bdna_quic_rewrite", netlink.HANDLE_MIN_EGRESS); err != nil {
			log.Printf("  ⚠️  bdna_quic_rewrite 挂载失败（降级）: %v", err)
		}
	}
	// TC ingress: bdna_ja4_capture（辅助）
	if prog := objs.Programs["bdna_ja4_capture"]; prog != nil {
		if err := l.attachTCFilter(objs, "bdna_ja4_capture", netlink.HANDLE_MIN_INGRESS); err != nil {
			log.Printf("  ⚠️  bdna_ja4_capture 挂载失败（降级）: %v", err)
		}
	}
	return nil
}

// attachPhantom 挂载 Phantom (TC ingress)
func (l *Loader) attachPhantom(loader *Loader, objs *ebpf.Collection) error {
	return l.attachTCFilter(objs, "phantom_redirect", netlink.HANDLE_MIN_INGRESS)
}

// attachChameleon 挂载 Chameleon (TC egress)
func (l *Loader) attachChameleon(loader *Loader, objs *ebpf.Collection) error {
	if err := l.attachTCFilter(objs, "chameleon_tls_rewrite", netlink.HANDLE_MIN_EGRESS); err != nil {
		return err
	}
	if prog := objs.Programs["chameleon_quic_rewrite"]; prog != nil {
		if err := l.attachTCFilter(objs, "chameleon_quic_rewrite", netlink.HANDLE_MIN_EGRESS); err != nil {
			log.Printf("  ⚠️  chameleon_quic_rewrite 挂载失败（降级）: %v", err)
		}
	}
	return nil
}

// attachH3Shaper 挂载 H3 流量整形 (TC egress)
func (l *Loader) attachH3Shaper(loader *Loader, objs *ebpf.Collection) error {
	return l.attachTCFilter(objs, "h3_shaper_egress", netlink.HANDLE_MIN_EGRESS)
}

// ============================================================
// TC / XDP / Sockmap 底层挂载
// ============================================================

// ensureClsactQdisc 确保网卡上存在 clsact qdisc
func (l *Loader) ensureClsactQdisc(iface netlink.Link) error {
	qdiscs, err := netlink.QdiscList(iface)
	if err != nil {
		return fmt.Errorf("列举 qdisc 失败: %w", err)
	}

	for _, qdisc := range qdiscs {
		if qdisc.Attrs().Parent == netlink.HANDLE_CLSACT {
			return nil // 已存在
		}
	}

	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: iface.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}

	if err := netlink.QdiscAdd(qdisc); err != nil {
		return fmt.Errorf("添加 clsact qdisc 失败: %w", err)
	}

	log.Printf("  ✅ clsact qdisc 已创建: %s", l.iface)
	return nil
}

// attachTCFilter 通过 netlink 将 BPF 程序挂载到 TC
func (l *Loader) attachTCFilter(objs *ebpf.Collection, progName string, parent uint32) error {
	prog := objs.Programs[progName]
	if prog == nil {
		return fmt.Errorf("程序 %s 不存在于 collection 中", progName)
	}

	filter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: l.ifaceIdx,
			Parent:    parent,
			Protocol:  3, // ETH_P_ALL
			Priority:  1,
		},
		Fd:           prog.FD(),
		Name:         progName,
		DirectAction: true,
	}

	if err := netlink.FilterAdd(filter); err != nil {
		return fmt.Errorf("添加 TC filter %s 失败: %w", progName, err)
	}

	l.tcFilters = append(l.tcFilters, filter)
	log.Printf("  ✅ TC filter: %s → %s (parent=0x%x)", progName, l.iface, parent)
	return nil
}

// attachXDP 将 BPF 程序挂载到 XDP
func (l *Loader) attachXDP(objs *ebpf.Collection, progName string) error {
	prog := objs.Programs[progName]
	if prog == nil {
		return fmt.Errorf("程序 %s 不存在于 collection 中", progName)
	}

	xdpLink, err := link.AttachXDP(link.XDPOptions{
		Program:   prog,
		Interface: l.ifaceIdx,
		Flags:     link.XDPGenericMode, // 通用模式，兼容 Kernel 5.15+
	})
	if err != nil {
		return fmt.Errorf("挂载 XDP %s 失败: %w", progName, err)
	}

	l.links = append(l.links, xdpLink)
	log.Printf("  ✅ XDP: %s → %s (ifindex=%d, generic)", progName, l.iface, l.ifaceIdx)
	return nil
}

// initQuotaMap 初始化 quota_map，写入最大值避免首次 Lookup 空指针
func (l *Loader) initQuotaMap() error {
	quotaMap := l.maps["quota_map"]
	if quotaMap == nil {
		return fmt.Errorf("quota_map 不存在")
	}

	key := uint32(0)
	value := uint64(^uint64(0)) // 最大值 = 无限配额（等待 OS 下发真实值）
	if err := quotaMap.Put(&key, &value); err != nil {
		return fmt.Errorf("初始化 quota_map 失败: %w", err)
	}

	log.Println("  ✅ quota_map 已初始化（无限配额，等待 OS 下发）")
	return nil
}

// ============================================================
// Sockmap 独立加载（不共享 common.h Map）
// ============================================================

func (l *Loader) loadSockmap() error {
	sockmapPath := "bpf/sockmap.o"
	if _, err := os.Stat(sockmapPath); os.IsNotExist(err) {
		return fmt.Errorf("sockmap.o 不存在，请先编译")
	}

	spec, err := ebpf.LoadCollectionSpec(sockmapPath)
	if err != nil {
		return fmt.Errorf("加载 Sockmap spec 失败: %w", err)
	}

	objs, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("创建 Sockmap 集合失败: %w", err)
	}
	l.collections = append(l.collections, objs)

	// 收集 Sockmap 专有 Map
	for name, m := range objs.Maps {
		l.maps[name] = m
	}

	// 挂载 sockops 到 cgroup
	if err := l.attachSockops(objs); err != nil {
		return fmt.Errorf("挂载 sockops 失败: %w", err)
	}

	// 挂载 sk_msg 到 sockmap
	if err := l.attachSkMsg(objs); err != nil {
		return fmt.Errorf("挂载 sk_msg 失败: %w", err)
	}

	log.Println("✅ Sockmap 零拷贝路径已激活")
	return nil
}

func (l *Loader) attachSockops(objs *ebpf.Collection) error {
	prog := objs.Programs["sockmap_sockops"]
	if prog == nil {
		prog = objs.Programs["sockmap_sockops_hash"]
	}
	if prog == nil {
		return fmt.Errorf("sockops 程序不存在")
	}

	lnk, err := link.AttachCgroup(link.CgroupOptions{
		Path:    l.cgroupPath,
		Attach:  ebpf.AttachCGroupSockOps,
		Program: prog,
	})
	if err != nil {
		return fmt.Errorf("AttachCgroup 失败: %w", err)
	}

	l.links = append(l.links, lnk)
	log.Printf("  ✅ sockops 已挂载到 %s", l.cgroupPath)
	return nil
}

func (l *Loader) attachSkMsg(objs *ebpf.Collection) error {
	prog := objs.Programs["sockmap_redirect"]
	if prog == nil {
		prog = objs.Programs["sockmap_redirect_hash"]
	}
	if prog == nil {
		return fmt.Errorf("sk_msg 程序不存在")
	}

	sockMap := l.maps["sock_map"]
	if sockMap == nil {
		sockMap = l.maps["sock_hash"]
	}
	if sockMap == nil {
		return fmt.Errorf("sockmap 不存在")
	}

	// sk_msg 必须通过 RawAttachProgram 挂载到 Map FD（非 RawLink）
	err := link.RawAttachProgram(link.RawAttachProgramOptions{
		Target:  sockMap.FD(),
		Program: prog,
		Attach:  ebpf.AttachSkMsgVerdict,
	})
	if err != nil {
		return fmt.Errorf("RawAttachProgram sk_msg 失败: %w", err)
	}

	log.Println("  ✅ sk_msg 已挂载到 sockmap")
	return nil
}

// ============================================================
// 公共 API
// ============================================================

// UpdateStrategy 更新防御策略（写入 eBPF Map）
func (l *Loader) UpdateStrategy(strategy *DefenseStrategy) error {
	key := uint32(0)

	if jitterMap := l.maps["jitter_config_map"]; jitterMap != nil {
		jitterCfg := JitterConfig{
			Enabled:     1,
			MeanIATUs:   strategy.JitterMeanUs,
			StddevIATUs: strategy.JitterStddevUs,
			TemplateID:  strategy.TemplateID,
		}
		if err := jitterMap.Put(&key, &jitterCfg); err != nil {
			return fmt.Errorf("更新 Jitter 配置失败: %w", err)
		}
	}

	if vpcMap := l.maps["vpc_config_map"]; vpcMap != nil {
		vpcCfg := VPCConfig{
			Enabled:        1,
			FiberJitterUs:  strategy.FiberJitterUs,
			RouterDelayUs:  strategy.RouterDelayUs,
			NoiseIntensity: strategy.NoiseIntensity,
		}
		if err := vpcMap.Put(&key, &vpcCfg); err != nil {
			return fmt.Errorf("更新 VPC 配置失败: %w", err)
		}
	}

	log.Printf("✅ 防御策略已更新: Jitter=%dus±%dus, Noise=%d%%",
		strategy.JitterMeanUs, strategy.JitterStddevUs, strategy.NoiseIntensity)
	return nil
}

// GetMap 获取 Map 引用
func (l *Loader) GetMap(name string) *ebpf.Map {
	return l.maps[name]
}

// GetSockMap 获取 Sockmap 引用（供 TPROXY 使用）
func (l *Loader) GetSockMap() *ebpf.Map {
	if m := l.maps["sock_map"]; m != nil {
		return m
	}
	return l.maps["sock_hash"]
}

// GetProxyMap 获取代理对 Map 引用
func (l *Loader) GetProxyMap() *ebpf.Map {
	return l.maps["proxy_map"]
}

// GetConnStateMap 获取连接状态 Map 引用
func (l *Loader) GetConnStateMap() *ebpf.Map {
	return l.maps["conn_state_map"]
}

// GetSockmapStats 获取 Sockmap 统计
func (l *Loader) GetSockmapStats() (map[string]uint64, error) {
	statsMap := l.maps["sockmap_stats"]
	if statsMap == nil {
		return nil, fmt.Errorf("sockmap_stats 不存在")
	}

	stats := make(map[string]uint64)
	keys := []string{"redirect_ok", "redirect_fail", "bytes_tx", "bytes_rx"}
	for i, key := range keys {
		var value uint64
		k := uint32(i)
		if err := statsMap.Lookup(&k, &value); err == nil {
			stats[key] = value
		}
	}
	return stats, nil
}

// StartMonitoring 启动威胁监控（Ring Buffer）
func (l *Loader) StartMonitoring(handler ThreatEventHandler) error {
	reader, err := NewRingBufferReader(l.maps["threat_events"], handler)
	if err != nil {
		return fmt.Errorf("创建 Ring Buffer 读取器失败: %w", err)
	}
	reader.Start()
	log.Println("🔍 威胁监控已启动（Ring Buffer）")
	return nil
}

// Close 关闭加载器，清理所有资源（纯内存，无 bpffs 残留）
func (l *Loader) Close() error {
	// 1. 卸载 TC filters
	for _, filter := range l.tcFilters {
		if err := netlink.FilterDel(filter); err != nil {
			log.Printf("⚠️  卸载 TC filter 失败: %v", err)
		}
	}

	// 2. 卸载 link（XDP, cgroup, sk_msg）
	for _, lnk := range l.links {
		if err := lnk.Close(); err != nil {
			log.Printf("⚠️  卸载 link 失败: %v", err)
		}
	}

	// 3. 关闭所有 collection（Map FD 随之关闭，内核自动回收）
	for _, coll := range l.collections {
		coll.Close()
	}

	log.Println("✅ eBPF 加载器已关闭，所有 Map 随进程销毁（无痕）")
	return nil
}

// CheckKernelVersion 检查内核版本
func CheckKernelVersion() error {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return fmt.Errorf("读取内核版本失败: %w", err)
	}
	log.Printf("内核版本: %s", string(data))
	return nil
}
