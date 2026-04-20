// Package nerve - 神经缝合层
// 将 Gateway 的感知（上行）和执行（下行）与 Mirage-OS 完整对接
//
// 设计原则：
// - 计费通道唯一化：仅 gRPC，杜绝 HTTP 双重扣费
// - 威胁上报风暴控制：同源同类攻击 5s 聚合窗口
// - 物理粉碎：Seek(0)+Write+Sync 三遍覆写，非 inode unlink
// - 幂等 Hash：固定字段顺序 binary 拼接，绝对确定性
package nerve

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"mirage-gateway/pkg/api"
	pb "mirage-proto/gen"
	"mirage-gateway/pkg/ebpf"
)

// ============================================================
// 战役一：上行感知闭环 (The Sensory Uplink)
// ============================================================

// SensoryUplink 上行感知闭环：流量统计 + 威胁上报（唯一通道：gRPC）
type SensoryUplink struct {
	grpcClient    *api.GRPCClient
	loader        *ebpf.Loader
	gatewayID     string
	stopCh        chan struct{}
	wg            sync.WaitGroup
	threatLimiter *ThreatAggregator
	// 流量 offset：记录上次上报时的累计值，避免重复计费
	lastBusinessBytes atomic.Uint64
	lastDefenseBytes  atomic.Uint64
}

// NewSensoryUplink 创建上行感知
func NewSensoryUplink(client *api.GRPCClient, loader *ebpf.Loader, gatewayID string) *SensoryUplink {
	return &SensoryUplink{
		grpcClient:    client,
		loader:        loader,
		gatewayID:     gatewayID,
		stopCh:        make(chan struct{}),
		threatLimiter: NewThreatAggregator(5 * time.Second),
	}
}

// Start 启动上行感知循环
func (su *SensoryUplink) Start(ctx context.Context) {
	// 关键：启动前先读取一次 eBPF 计数器作为基线
	// 防止进程重启后 Pinned Map 残留值导致 Delta 尖刺（溢出截断陷阱）
	su.initBaseline()

	su.wg.Add(2)
	go su.trafficReportLoop(ctx)
	go su.threatFlushLoop(ctx)
	log.Println("[NerveSplice] 上行感知闭环已启动（gRPC 唯一通道）")
}

// initBaseline 初始化流量基线（防止重启后 Delta 尖刺）
// 场景：Gateway 进程重启，但 eBPF Map 如果是 Pinned 的，计数器不归零
// 此时 lastOffset=0 而 current=巨大值，会产生虚假的巨额扣费
func (su *SensoryUplink) initBaseline() {
	trafficMap := su.loader.GetMap("traffic_stats")
	if trafficMap == nil {
		return
	}
	var currentBiz, currentDef uint64
	baseKey := uint32(0)
	defenseKey := uint32(1)
	if err := trafficMap.Lookup(&baseKey, &currentBiz); err == nil {
		su.lastBusinessBytes.Store(currentBiz)
	}
	if err := trafficMap.Lookup(&defenseKey, &currentDef); err == nil {
		su.lastDefenseBytes.Store(currentDef)
	}
	log.Printf("[NerveSplice] 流量基线已初始化: biz=%d, def=%d", currentBiz, currentDef)
}

// Stop 停止
func (su *SensoryUplink) Stop() {
	close(su.stopCh)
	su.wg.Wait()
}

// trafficReportLoop 从 eBPF traffic_stats 读取增量并通过 gRPC 上报
// 关键：读取后计算 delta（增量），避免重复计费
func (su *SensoryUplink) trafficReportLoop(ctx context.Context) {
	defer su.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-su.stopCh:
			return
		case <-ticker.C:
			if !su.grpcClient.IsConnected() {
				continue
			}
			bizDelta, defDelta := su.readTrafficDelta()
			if bizDelta == 0 && defDelta == 0 {
				continue // 无新增流量，不发送空报文
			}
			su.grpcClient.ReportTrafficDirect(&pb.TrafficRequest{
				GatewayId:     su.gatewayID,
				Timestamp:     time.Now().Unix(),
				BusinessBytes: bizDelta,
				DefenseBytes:  defDelta,
				PeriodSeconds: 10,
			})
		}
	}
}

// readTrafficDelta 读取流量增量（当前值 - 上次 offset）
func (su *SensoryUplink) readTrafficDelta() (bizDelta, defDelta uint64) {
	trafficMap := su.loader.GetMap("traffic_stats")
	if trafficMap == nil {
		return 0, 0
	}

	var currentBiz, currentDef uint64
	baseKey := uint32(0)
	defenseKey := uint32(1)

	if err := trafficMap.Lookup(&baseKey, &currentBiz); err != nil {
		currentBiz = 0
	}
	if err := trafficMap.Lookup(&defenseKey, &currentDef); err != nil {
		currentDef = 0
	}

	// 计算增量
	lastBiz := su.lastBusinessBytes.Load()
	lastDef := su.lastDefenseBytes.Load()

	// 处理计数器回绕（内核重启或 Map 重建）
	if currentBiz >= lastBiz {
		bizDelta = currentBiz - lastBiz
	} else {
		bizDelta = currentBiz // 回绕，取当前值
	}
	if currentDef >= lastDef {
		defDelta = currentDef - lastDef
	} else {
		defDelta = currentDef
	}

	// 更新 offset
	su.lastBusinessBytes.Store(currentBiz)
	su.lastDefenseBytes.Store(currentDef)

	return bizDelta, defDelta
}

// threatFlushLoop 定期刷新聚合的威胁事件到 OS
func (su *SensoryUplink) threatFlushLoop(ctx context.Context) {
	defer su.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-su.stopCh:
			return
		case <-ticker.C:
			events := su.threatLimiter.Flush()
			if len(events) == 0 {
				continue
			}
			if su.grpcClient.IsConnected() {
				su.grpcClient.ReportThreat(events)
			}
		}
	}
}

// ReportThreat 上报威胁事件（经过聚合窗口限流，不会 DDoS OS）
func (su *SensoryUplink) ReportThreat(threatType pb.ThreatType, sourceIP string, severity int32) {
	su.threatLimiter.Ingest(threatType, sourceIP, severity)
}

// ============================================================
// 威胁聚合器：同源同类攻击 5s 窗口合并
// ============================================================

// threatKey 聚合键：源IP + 威胁类型
type threatKey struct {
	SourceIP   string
	ThreatType pb.ThreatType
}

// threatBucket 聚合桶
type threatBucket struct {
	FirstSeen   int64
	LastSeen    int64
	Count       uint32
	MaxSeverity int32
}

// ThreatAggregator 威胁聚合器（风暴控制）
type ThreatAggregator struct {
	mu      sync.Mutex
	window  time.Duration
	buckets map[threatKey]*threatBucket
}

// NewThreatAggregator 创建聚合器
func NewThreatAggregator(window time.Duration) *ThreatAggregator {
	return &ThreatAggregator{
		window:  window,
		buckets: make(map[threatKey]*threatBucket),
	}
}

// Ingest 摄入一条威胁事件（聚合到桶中）
func (ta *ThreatAggregator) Ingest(threatType pb.ThreatType, sourceIP string, severity int32) {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	key := threatKey{SourceIP: sourceIP, ThreatType: threatType}
	now := time.Now().Unix()

	if bucket, ok := ta.buckets[key]; ok {
		bucket.LastSeen = now
		bucket.Count++
		if severity > bucket.MaxSeverity {
			bucket.MaxSeverity = severity
		}
	} else {
		ta.buckets[key] = &threatBucket{
			FirstSeen:   now,
			LastSeen:    now,
			Count:       1,
			MaxSeverity: severity,
		}
	}
}

// Flush 刷出所有聚合桶，生成合并后的事件列表
func (ta *ThreatAggregator) Flush() []*pb.ThreatEvent {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	if len(ta.buckets) == 0 {
		return nil
	}

	events := make([]*pb.ThreatEvent, 0, len(ta.buckets))
	for key, bucket := range ta.buckets {
		events = append(events, &pb.ThreatEvent{
			Timestamp:   bucket.LastSeen,
			ThreatType:  key.ThreatType,
			SourceIp:    key.SourceIP,
			Severity:    bucket.MaxSeverity,
			PacketCount: bucket.Count,
		})
	}

	// 清空桶
	ta.buckets = make(map[threatKey]*threatBucket)
	return events
}

// ============================================================
// 战役二：下行 eBPF 状态机映射（幂等 Hash 校验）
// ============================================================

// DesiredStateConfig OS 下发的期望状态（纯值类型，无 map/slice，Hash 确定性有保证）
type DesiredStateConfig struct {
	JitterMeanUs   uint32
	JitterStddevUs uint32
	NoiseIntensity uint32
	PaddingRate    uint32
	TemplateID     uint32
	FiberJitterUs  uint32
	RouterDelayUs  uint32
}

// MotorDownlink 下行状态机映射器
// 采用双槽位 RCU 方案防止内核脏读撕裂：
// - jitter_config_map[0] 和 [1] 为两个配置槽位
// - ctrl_map[ACTIVE_SLOT_KEY] 存储当前活跃槽位索引（4 字节原子切换）
// - eBPF 数据面先读 active_slot，再读对应槽位的配置
type MotorDownlink struct {
	loader      *ebpf.Loader
	currentHash atomic.Value  // 存储 [32]byte
	activeSlot  atomic.Uint32 // 当前活跃槽位：0 或 1
	mu          sync.Mutex
}

// activeSlotKey ctrl_map 中存储活跃槽位索引的 key
const activeSlotKey = uint32(15) // ctrl_map[15] = active config slot

// NewMotorDownlink 创建下行映射器
func NewMotorDownlink(loader *ebpf.Loader) *MotorDownlink {
	md := &MotorDownlink{loader: loader}
	md.currentHash.Store([32]byte{})
	md.activeSlot.Store(0)
	return md
}

// ApplyDesiredState 应用期望状态（幂等 + RCU 双槽位无撕裂）
// 返回 applied=true 表示配置已变更并写入 eBPF Map
func (md *MotorDownlink) ApplyDesiredState(cfg *DesiredStateConfig) (applied bool, err error) {
	// 1. 计算配置 Hash（确定性：固定字段顺序 + binary.LittleEndian）
	newHash := md.computeHash(cfg)

	// 2. 无锁快速路径
	current := md.currentHash.Load().([32]byte)
	if newHash == current {
		return false, nil
	}

	// 3. 加锁写入
	md.mu.Lock()
	defer md.mu.Unlock()

	// Double-check after lock
	current = md.currentHash.Load().([32]byte)
	if newHash == current {
		return false, nil
	}

	// 4. RCU 写入：写入非活跃槽位，然后原子切换 active index
	currentSlot := md.activeSlot.Load()
	nextSlot := (currentSlot + 1) % 2 // 切换到另一个槽位

	// 写入 Jitter 配置到 nextSlot
	if jitterMap := md.loader.GetMap("jitter_config_map"); jitterMap != nil {
		jitterCfg := ebpf.JitterConfig{
			Enabled:     1,
			MeanIATUs:   cfg.JitterMeanUs,
			StddevIATUs: cfg.JitterStddevUs,
			TemplateID:  cfg.TemplateID,
		}
		key := nextSlot
		if err := jitterMap.Put(&key, &jitterCfg); err != nil {
			return false, fmt.Errorf("写入 jitter_config_map[%d] 失败: %w", nextSlot, err)
		}
	}

	// 写入 VPC 配置到 nextSlot
	if vpcMap := md.loader.GetMap("vpc_config_map"); vpcMap != nil {
		vpcCfg := ebpf.VPCConfig{
			Enabled:        1,
			FiberJitterUs:  cfg.FiberJitterUs,
			RouterDelayUs:  cfg.RouterDelayUs,
			NoiseIntensity: cfg.NoiseIntensity,
		}
		key := nextSlot
		if err := vpcMap.Put(&key, &vpcCfg); err != nil {
			return false, fmt.Errorf("写入 vpc_config_map[%d] 失败: %w", nextSlot, err)
		}
	}

	// 写入 NPM 配置到 nextSlot
	if npmMap := md.loader.GetMap("npm_config_map"); npmMap != nil {
		npmCfg := struct {
			Enabled     uint32
			PaddingRate uint32
		}{1, cfg.PaddingRate}
		key := nextSlot
		if err := npmMap.Put(&key, &npmCfg); err != nil {
			return false, fmt.Errorf("写入 npm_config_map[%d] 失败: %w", nextSlot, err)
		}
	}

	// 5. 原子切换活跃槽位（4 字节写入，内核保证原子性）
	if ctrlMap := md.loader.GetMap("ctrl_map"); ctrlMap != nil {
		key := activeSlotKey
		if err := ctrlMap.Put(&key, &nextSlot); err != nil {
			return false, fmt.Errorf("切换 active_slot 失败: %w", err)
		}
	}
	md.activeSlot.Store(nextSlot)

	// 6. 更新 Hash
	md.currentHash.Store(newHash)

	log.Printf("[MotorDownlink] 期望状态已应用 (slot %d→%d): Jitter=%dμs±%dμs, Noise=%d%%, Hash=%x",
		currentSlot, nextSlot, cfg.JitterMeanUs, cfg.JitterStddevUs, cfg.NoiseIntensity, newHash[:8])
	return true, nil
}

// computeHash 确定性 Hash：固定字段顺序，纯 uint32 拼接，无 map/slice
func (md *MotorDownlink) computeHash(cfg *DesiredStateConfig) [32]byte {
	data := make([]byte, 28) // 7 * 4 bytes
	binary.LittleEndian.PutUint32(data[0:4], cfg.JitterMeanUs)
	binary.LittleEndian.PutUint32(data[4:8], cfg.JitterStddevUs)
	binary.LittleEndian.PutUint32(data[8:12], cfg.NoiseIntensity)
	binary.LittleEndian.PutUint32(data[12:16], cfg.PaddingRate)
	binary.LittleEndian.PutUint32(data[16:20], cfg.TemplateID)
	binary.LittleEndian.PutUint32(data[20:24], cfg.FiberJitterUs)
	binary.LittleEndian.PutUint32(data[24:28], cfg.RouterDelayUs)
	return sha256.Sum256(data)
}

// GetCurrentHash 获取当前配置 Hash
func (md *MotorDownlink) GetCurrentHash() [32]byte {
	return md.currentHash.Load().([32]byte)
}

// ============================================================
// 战役三：物理级焦土协议 (The Scorched Earth)
// ============================================================

// ScorchedEarth 焦土协议执行器
// 触发条件：心跳超时 300s / OS 下发 0xDEADBEEF / 调试器检测
type ScorchedEarth struct {
	loader       *ebpf.Loader
	emergencyMgr *ebpf.EmergencyManager
	certPaths    []string // TLS 证书/密钥路径
	configPaths  []string // 临时配置文件路径
	sessionKeys  [][]byte // G-Tunnel 会话密钥引用（直接覆写原始 slice）
	mu           sync.Mutex
}

// NewScorchedEarth 创建焦土执行器
func NewScorchedEarth(loader *ebpf.Loader, emergencyMgr *ebpf.EmergencyManager) *ScorchedEarth {
	return &ScorchedEarth{
		loader:       loader,
		emergencyMgr: emergencyMgr,
		certPaths:    make([]string, 0),
		configPaths:  make([]string, 0),
		sessionKeys:  make([][]byte, 0),
	}
}

// RegisterCertPaths 注册 TLS 证书/密钥路径（自毁时物理覆写）
// 安全要求：证书/私钥必须存储在 tmpfs 中（断电即消失）
// 如果检测到非 tmpfs 路径，记录安全告警但仍注册（兼容旧部署）
func (se *ScorchedEarth) RegisterCertPaths(paths ...string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	for _, p := range paths {
		if p != "" {
			if !isTmpfsPath(p) {
				log.Printf("🚨 [ScorchedEarth] 安全告警: 证书路径 %s 不在 tmpfs 中！SSD 磨损均衡可能导致私钥残留", p)
				log.Printf("🚨 [ScorchedEarth] 建议: 将证书移至 /var/mirage/certs/ (tmpfs) 或设置 MIRAGE_TMPFS_MODE=1")
			}
			se.certPaths = append(se.certPaths, p)
		}
	}
}

// RegisterConfigPaths 注册临时配置文件路径
func (se *ScorchedEarth) RegisterConfigPaths(paths ...string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.configPaths = append(se.configPaths, paths...)
}

// RegisterSessionKey 注册会话密钥引用（传入的 slice 将被原地覆写）
func (se *ScorchedEarth) RegisterSessionKey(key []byte) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.sessionKeys = append(se.sessionKeys, key)
}

// Execute 执行焦土协议（不可逆，进程将退出）
func (se *ScorchedEarth) Execute() {
	log.Println("[ScorchedEarth] 🔥🔥🔥 焦土协议启动 🔥🔥🔥")

	// Phase 0: 卸载 Pinned eBPF 幽灵（XDP/TC Hook + bpffs）
	se.detachKernelHooks()

	// Phase 1: 清空 eBPF 所有特征 Map（内核态数据优先）
	se.wipeEBPFMaps()

	// Phase 2: 内存中的会话密钥 3 次覆写
	se.wipeMemorySecrets()

	// Phase 3: 磁盘证书文件 Seek+Write+Sync 三遍覆写后 unlink
	se.wipeDiskCerts()

	// Phase 4: 删除临时配置文件 + 清理临时目录
	se.wipeConfigFiles()

	log.Println("[ScorchedEarth] 🔥 焦土协议执行完毕，进程退出")
	os.Exit(1)
}

// detachKernelHooks 卸载内核中的 eBPF 幽灵
// 即使 Go 进程死亡，Pinned 的 XDP/TC 程序仍会附着在网卡上运行
// 必须显式卸载，让服务器变成一块没有任何网络拦截能力的"废铁"
func (se *ScorchedEarth) detachKernelHooks() {
	// 1. 清除 bpffs 中的 Pinned Map/Prog（如果存在）
	bpfPaths := []string{
		"/sys/fs/bpf/mirage",
		"/sys/fs/bpf/mirage-gateway",
	}
	for _, path := range bpfPaths {
		if err := os.RemoveAll(path); err != nil {
			log.Printf("[ScorchedEarth] ⚠️ 清除 bpffs %s 失败: %v", path, err)
		} else {
			log.Printf("[ScorchedEarth] ✅ 已清除 bpffs: %s", path)
		}
	}

	// 2. 通过 Loader.Close() 卸载所有 XDP/TC Hook 和 Link
	// Loader.Close() 会：
	//   - netlink.FilterDel 卸载所有 TC filter
	//   - link.Close() 卸载 XDP、cgroup sockops、sk_msg
	//   - collection.Close() 关闭所有 Map FD
	if se.loader != nil {
		if err := se.loader.Close(); err != nil {
			log.Printf("[ScorchedEarth] ⚠️ Loader 卸载失败: %v", err)
		} else {
			log.Println("[ScorchedEarth] ✅ Phase 0: 所有 XDP/TC Hook 已卸载，网卡已裸奔")
		}
	}
}

// wipeEBPFMaps 清空所有敏感 eBPF Map
func (se *ScorchedEarth) wipeEBPFMaps() {
	if se.emergencyMgr != nil {
		if err := se.emergencyMgr.TriggerWipe(); err != nil {
			log.Printf("[ScorchedEarth] ⚠️ eBPF Map 清空失败: %v", err)
		} else {
			log.Println("[ScorchedEarth] ✅ Phase 1: eBPF Map 已清空")
		}
	}
}

// wipeMemorySecrets 3 次覆写内存中的密钥（原地覆写原始 slice）
func (se *ScorchedEarth) wipeMemorySecrets() {
	se.mu.Lock()
	keys := make([][]byte, len(se.sessionKeys))
	copy(keys, se.sessionKeys)
	se.mu.Unlock()

	for i, key := range keys {
		if key == nil || len(key) == 0 {
			continue
		}
		// Pass 1: crypto/rand 随机数据
		rand.Read(key)
		// Pass 2: 0xFF 全填充
		for j := range key {
			key[j] = 0xFF
		}
		// Pass 3: 0x00 清零
		for j := range key {
			key[j] = 0x00
		}
		log.Printf("[ScorchedEarth] ✅ 会话密钥 #%d 已覆写 (3-pass, %d bytes)", i, len(key))
	}
	log.Println("[ScorchedEarth] ✅ Phase 2: 内存密钥已擦除")
}

// wipeDiskCerts 物理覆写磁盘证书（Seek(0) + Write + Sync × 3）
func (se *ScorchedEarth) wipeDiskCerts() {
	se.mu.Lock()
	paths := make([]string, len(se.certPaths))
	copy(paths, se.certPaths)
	se.mu.Unlock()

	for _, path := range paths {
		if err := se.secureDeleteFile(path); err != nil {
			log.Printf("[ScorchedEarth] ⚠️ 证书擦除失败 %s: %v", path, err)
		} else {
			log.Printf("[ScorchedEarth] ✅ 已物理擦除: %s", path)
		}
	}
	log.Println("[ScorchedEarth] ✅ Phase 3: 磁盘证书已物理擦除")
}

// wipeConfigFiles 删除临时配置文件
func (se *ScorchedEarth) wipeConfigFiles() {
	se.mu.Lock()
	paths := make([]string, len(se.configPaths))
	copy(paths, se.configPaths)
	se.mu.Unlock()

	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("[ScorchedEarth] ⚠️ 配置删除失败 %s: %v", path, err)
		}
	}

	// 清理临时目录
	tmpPatterns := []string{"/tmp/mirage-*", "/var/lib/mirage/tmp"}
	for _, pattern := range tmpPatterns {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			os.RemoveAll(m)
		}
	}
	log.Println("[ScorchedEarth] ✅ Phase 4: 临时配置已清除")
}

// secureDeleteFile 安全删除文件：Seek(0) + Write(random/0xFF/0x00) + Sync × 3 → Remove
// 注意：在 SSD 上 3-pass 覆写因磨损均衡机制无法保证物理擦除
// 生产环境必须将敏感文件存储在 tmpfs 中（断电即消失）
func (se *ScorchedEarth) secureDeleteFile(path string) error {
	if !isTmpfsPath(path) {
		log.Printf("⚠️ [ScorchedEarth] 文件 %s 不在 tmpfs 中，3-pass 覆写在 SSD 上可能无效（磨损均衡）", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在，视为成功
		}
		return err
	}

	size := info.Size()
	if size == 0 {
		return os.Remove(path)
	}

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}

	buf := make([]byte, size)

	// Pass 1: /dev/urandom 随机数据
	rand.Read(buf)
	f.Seek(0, 0)
	f.Write(buf)
	f.Sync() // 强制刷入物理磁盘

	// Pass 2: 0xFF 全填充
	for i := range buf {
		buf[i] = 0xFF
	}
	f.Seek(0, 0)
	f.Write(buf)
	f.Sync()

	// Pass 3: 0x00 清零
	for i := range buf {
		buf[i] = 0x00
	}
	f.Seek(0, 0)
	f.Write(buf)
	f.Sync()

	f.Close()

	// 最后解除 inode 链接
	return os.Remove(path)
}

// isTmpfsPath 检查路径是否位于 tmpfs 挂载点
// tmpfs 路径特征：/var/mirage（docker-compose.tmpfs.yml 挂载）、/tmp、/dev/shm
// 或环境变量 MIRAGE_TMPFS_MODE=1 时所有路径视为 tmpfs
func isTmpfsPath(path string) bool {
	// 环境变量强制模式
	if os.Getenv("MIRAGE_TMPFS_MODE") == "1" {
		return true
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// 已知 tmpfs 挂载点前缀
	tmpfsPrefixes := []string{
		"/var/mirage",
		"/tmp",
		"/dev/shm",
		"/run",
	}

	for _, prefix := range tmpfsPrefixes {
		if len(absPath) >= len(prefix) && absPath[:len(prefix)] == prefix {
			return true
		}
	}

	// Linux: 读取 /proc/mounts 检查实际文件系统类型
	if mountData, err := os.ReadFile("/proc/mounts"); err == nil {
		// 简单匹配：查找包含路径前缀且类型为 tmpfs 的行
		lines := splitLines(mountData)
		for _, line := range lines {
			fields := splitFields(line)
			if len(fields) >= 3 && fields[2] == "tmpfs" {
				mountPoint := fields[1]
				if len(absPath) >= len(mountPoint) && absPath[:len(mountPoint)] == mountPoint {
					return true
				}
			}
		}
	}

	return false
}

// splitLines 按换行分割（避免引入 strings 包）
func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

// splitFields 按空格分割（避免引入 strings 包）
func splitFields(s string) []string {
	var fields []string
	start := -1
	for i, c := range s {
		if c == ' ' || c == '\t' {
			if start >= 0 {
				fields = append(fields, s[start:i])
				start = -1
			}
		} else {
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		fields = append(fields, s[start:])
	}
	return fields
}

// TriggerWipe 实现 security.EmergencyWiper 接口
// 当 Watchdog 超时调用此方法时，执行完整焦土协议（不含 os.Exit，由 Watchdog 控制退出）
func (se *ScorchedEarth) TriggerWipe() error {
	log.Println("[ScorchedEarth] 🔥 TriggerWipe 被调用（Watchdog 超时）")

	// Phase 0: 卸载内核 Hook（防止幽灵程序残留）
	se.detachKernelHooks()

	// Phase 1: 清空 eBPF Map
	se.wipeEBPFMaps()

	// Phase 2: 内存密钥覆写
	se.wipeMemorySecrets()

	// Phase 3: 磁盘证书物理覆写
	se.wipeDiskCerts()

	// Phase 4: 临时配置清除
	se.wipeConfigFiles()

	return nil
}
