/* Mirage Gateway - 公共协议头文件
 * 定义 C 数据面与 Go 控制面的通信协议
 * 这是系统的"宪法"
 */

#ifndef __MIRAGE_COMMON_H__
#define __MIRAGE_COMMON_H__

#include <linux/bpf.h>
#include <linux/types.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

// 常量定义
#ifndef ETH_P_IP
#define ETH_P_IP 0x0800
#endif

#ifndef ETH_HLEN
#define ETH_HLEN 14
#endif

// TC action codes
#ifndef TC_ACT_OK
#define TC_ACT_OK 0
#endif

#ifndef TC_ACT_SHOT
#define TC_ACT_SHOT 2
#endif

#ifndef TC_ACT_REDIRECT
#define TC_ACT_REDIRECT 7
#endif

// XDP action codes
#ifndef XDP_PASS
#define XDP_PASS 2
#endif

#ifndef XDP_DROP
#define XDP_DROP 1
#endif

#ifndef XDP_TX
#define XDP_TX 3
#endif

// BPF flags
#ifndef BPF_F_PSEUDO_HDR
#define BPF_F_PSEUDO_HDR (1ULL << 4)
#endif

#ifndef BPF_F_RECOMPUTE_CSUM
#define BPF_F_RECOMPUTE_CSUM (1ULL << 0)
#endif

/* ============================================
 * 配置结构体（Go → C）
 * ============================================ */

// 防御强度配置
struct defense_config {
    __u32 padding_ratio;        // NPM 填充比例（0-30，表示 0%-30%）
    __u32 jitter_interval_us;   // Jitter-Lite 扰动区间（微秒）
    __u32 path_count;           // G-Tunnel 路径数量（3-7）
    __u32 threat_level;         // 威胁等级（0-4）
    __u64 timestamp;            // 配置时间戳
};

// NPM 填充配置
struct npm_config {
    __u32 enabled;              // 是否启用（0/1）
    __u32 probability;          // 填充概率（0-100）
    __u32 min_padding;          // 最小填充（字节）
    __u32 max_padding;          // 最大填充（字节）
    __u32 distribution;         // 分布类型（0=uniform, 1=gaussian）
};

// Jitter-Lite 配置
struct jitter_config {
    __u32 enabled;              // 是否启用
    __u32 mean_iat_us;          // 平均包间隔（微秒）
    __u32 stddev_iat_us;        // 标准差（微秒）
    __u32 template_id;          // 拟态模板 ID
};

// B-DNA 拟态模板（V2 生产版）
struct dna_template {
    __u32 target_iat_mu;        // 目标间隔均值（微秒）
    __u32 target_iat_sigma;     // 目标间隔标准差（微秒）
    __u32 padding_strategy;     // 0:固定, 1:正态分布填充, 2:跟随载荷
    __u16 target_mtu;           // 模拟特定 MTU（如 Zoom 的 1432）
    __u16 reserved;             // 对齐填充
    __u32 burst_size;           // 突发包数量
    __u32 burst_interval;       // 突发间隔（微秒）
};

// VPC 噪声配置
struct vpc_config {
    __u32 enabled;              // 是否启用
    __u32 fiber_jitter_us;      // 光缆抖动（微秒）
    __u32 router_delay_us;      // 路由器延迟（微秒）
    __u32 noise_intensity;      // 噪声强度（0-100）
};

/* ============================================
 * 事件结构体（C → Go）
 * ============================================ */

// 威胁事件类型
enum threat_type {
    THREAT_NONE = 0,
    THREAT_ACTIVE_PROBING = 1,      // 主动探测
    THREAT_JA4_SCAN = 2,            // JA4 指纹扫描
    THREAT_SNI_PROBE = 3,           // SNI 探测
    THREAT_DPI_INSPECTION = 4,      // DPI 深度检测
    THREAT_TIMING_ATTACK = 5,       // 时序攻击
};

// 威胁事件（通过 Ring Buffer 上报）
struct threat_event {
    __u64 timestamp;            // 时间戳（纳秒）
    __u32 threat_type;          // 威胁类型
    __u32 source_ip;            // 源 IP
    __u16 source_port;          // 源端口
    __u16 dest_port;            // 目标端口
    __u32 packet_count;         // 数据包计数
    __u32 severity;             // 严重程度（0-10）
};

// 性能统计事件
struct perf_stats {
    __u64 timestamp;            // 时间戳
    __u64 total_packets;        // 总包数
    __u64 padded_packets;       // 填充包数
    __u64 dropped_packets;      // 丢弃包数
    __u32 avg_latency_ns;       // 平均延迟（纳秒）
    __u32 cpu_usage;            // CPU 占用（百分比）
};

/* ============================================
 * eBPF Map 定义
 * ============================================ */

// 控制 Map（Go → C）
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16);
    __type(key, __u32);         // 配置类型 ID
    __type(value, struct defense_config);
} ctrl_map SEC(".maps");

// NPM 配置 Map
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct npm_config);
} npm_config_map SEC(".maps");

// Jitter-Lite 配置 Map
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16);
    __type(key, __u32);         // 模板 ID
    __type(value, struct jitter_config);
} jitter_config_map SEC(".maps");

// B-DNA 拟态模板 Map（V2 生产版）
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16);
    __type(key, __u32);         // 模板 ID（0-15）
    __type(value, struct dna_template);
} dna_template_map SEC(".maps");

// VPC 配置 Map
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct vpc_config);
} vpc_config_map SEC(".maps");

// 流量统计 Map（计费探针）
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 2);
    __type(key, __u32);         // 0: 业务流量, 1: 防御流量
    __type(value, __u64);       // 字节数
} traffic_stats SEC(".maps");

// 配额状态 Map（欠费熔断）
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);       // 剩余流量 (字节)
} quota_map SEC(".maps");

// 蜂窝生命周期 Map（影子蜂窝）
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);       // 0:潜伏, 1:校准, 2:服役
} cell_phase_map SEC(".maps");

// 紧急自毁控制 Map
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);       // 0xDEADBEEF = 触发自毁
} emergency_ctrl_map SEC(".maps");

// 全局战术策略 Map（Go → C）
struct global_policy {
    __u32 tactical_mode;        // 0=Normal, 1=Sleep, 2=Aggressive, 3=Stealth
    __u32 social_jitter;        // 0-100
    __u32 cid_rotation_rate;    // 次/分钟
    __u32 fec_redundancy;       // 百分比
    __u32 stealth_filter;       // 隐匿模式最低威胁等级
    __u64 timestamp;            // 更新时间戳
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct global_policy);
} global_policy_map SEC(".maps");

// Ghost Mode 控制 Map
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);       // 0=正常, 1=Ghost Mode
} ghost_mode_map SEC(".maps");

// 威胁事件 Ring Buffer（C → Go）
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1024 * 1024);  // 1MB 共享缓冲区（防止高威胁场景溢出）
} threat_events SEC(".maps");

// 性能统计 Ring Buffer
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);   // 256KB
} perf_stats_events SEC(".maps");

/* ============================================
 * 辅助函数
 * ============================================ */

// 高斯分布采样（Box-Muller 变换简化版）
static __always_inline __u64 gaussian_sample(__u32 mean, __u32 stddev) {
    __u32 u1 = bpf_get_prandom_u32();
    
    // 简化版本：使用线性近似
    // 真实实现需要更复杂的数学运算
    __u64 z = mean + (u1 % (stddev * 2)) - stddev;
    
    return z;
}

// B-DNA 拟态延迟采样（V2 生产版）
// 基于中央极限定理的正态分布近似
static __always_inline __u64 get_mimic_delay(struct dna_template *tpl) {
    if (!tpl)
        return 0;
    
    // 1. 获取随机数（0-UINT32_MAX）
    __u32 rand = bpf_get_prandom_u32();
    
    // 2. 映射到正态分布
    // 使用 Box-Muller 变换的简化版本
    // Z = μ + σ × (rand / UINT32_MAX - 0.5) × 6
    // 这样约 99.7% 的值落在 [μ-3σ, μ+3σ] 区间
    __s64 offset = ((__s64)rand * 6 * tpl->target_iat_sigma) / 0xFFFFFFFF - (3 * tpl->target_iat_sigma);
    __s64 delay = (__s64)tpl->target_iat_mu + offset;
    
    // 3. 确保延迟为正数
    if (delay < 0)
        delay = tpl->target_iat_mu / 2;
    
    // 4. 限制最大延迟（防止异常值）
    __u64 max_delay = tpl->target_iat_mu + 5 * tpl->target_iat_sigma;
    if (delay > max_delay)
        delay = max_delay;
    
    return (__u64)delay;
}

// 提取源 IP
static __always_inline __u32 get_source_ip(void *data, void *data_end) {
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return 0;
    
    if (eth->h_proto != bpf_htons(0x0800)) // IPv4
        return 0;
    
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return 0;
    
    return ip->saddr;
}

// 上报威胁事件
static __always_inline int report_threat(
    __u32 threat_type,
    __u32 source_ip,
    __u16 source_port,
    __u32 severity
) {
    struct threat_event *event;
    
    event = bpf_ringbuf_reserve(&threat_events, sizeof(*event), 0);
    if (!event)
        return -1;
    
    event->timestamp = bpf_ktime_get_ns();
    event->threat_type = threat_type;
    event->source_ip = source_ip;
    event->source_port = source_port;
    event->severity = severity;
    event->packet_count = 1;
    
    bpf_ringbuf_submit(event, 0);
    return 0;
}

/* ============================================
 * 许可证与版权
 * ============================================ */

// 注意：license 定义移至各 .c 文件中，避免重复定义

#endif /* __MIRAGE_COMMON_H__ */
