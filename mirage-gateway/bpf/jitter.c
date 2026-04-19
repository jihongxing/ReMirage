/* Jitter-Lite 时域扰动协议
 * 核心功能：控制 skb->tstamp 实现绝对精准 IAT
 * 挂载点：TC (Traffic Control) egress
 * 
 * 这是让流量在时域上彻底消失的关键
 */

#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <linux/if_ether.h>
#include <linux/in.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#include "common.h"

/* ============================================
 * 核心逻辑：时域扰动
 * ============================================ */

SEC("tc")
int jitter_lite_egress(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    // 0. 配额熔断检查（最高优先级 - 生死裁决硬对齐）
    __u32 quota_key = 0;
    __u64 *remaining = bpf_map_lookup_elem(&quota_map, &quota_key);
    if (remaining) {
        if (*remaining == 0) {
            // 配额耗尽，立即熔断，一滴流量都不放
            return TC_ACT_STOLEN;
        }
        // 原子扣减（按包大小扣减字节数）
        __u64 pkt_len = (__u64)skb->len;
        if (*remaining < pkt_len) {
            // 剩余不足以覆盖本包，熔断
            __sync_fetch_and_and(remaining, 0);
            return TC_ACT_STOLEN;
        }
        __sync_fetch_and_sub(remaining, pkt_len);
    }
    
    // 1. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    // 2. 只处理 IPv4
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    // 3. 记录业务流量（原始包大小）
    __u32 traffic_key_base = 0;
    __u64 *base_bytes = bpf_map_lookup_elem(&traffic_stats, &traffic_key_base);
    if (base_bytes) {
        __sync_fetch_and_add(base_bytes, skb->len);
    }
    
    // 4. 查询 B-DNA 拟态模板（V2 生产版）
    __u32 template_key = 0;  // 默认使用模板 0
    struct dna_template *tpl = bpf_map_lookup_elem(&dna_template_map, &template_key);
    
    if (!tpl) {
        // 降级到 V1 Jitter-Lite 配置
        struct jitter_config *cfg = bpf_map_lookup_elem(&jitter_config_map, &template_key);
        if (!cfg || !cfg->enabled)
            return TC_ACT_OK;
        
        // V1 简单随机延迟
        __u64 delay_ns = gaussian_sample(cfg->mean_iat_us, cfg->stddev_iat_us) * 1000;
        __u64 now = bpf_ktime_get_ns();
        skb->tstamp = now + delay_ns;
        
        return TC_ACT_OK;
    }
    
    // 5. B-DNA 拟态延迟采样（非线性扰动）
    __u64 delay_us = get_mimic_delay(tpl);
    __u64 delay_ns = delay_us * 1000;
    
    // 6. 关键动作：修改 skb->tstamp（内核时间戳）
    __u64 now = bpf_ktime_get_ns();
    skb->tstamp = now + delay_ns;
    
    // 7. NPM Padding 填充（根据策略）
    // TODO: 根据 tpl->padding_strategy 应用不同的填充策略
    // 0: 固定填充
    // 1: 正态分布填充
    // 2: 跟随载荷填充
    
    // 8. 记录防御流量（如果应用了 Padding）
    // __u32 traffic_key_defense = 1;
    // __u64 *defense_bytes = bpf_map_lookup_elem(&traffic_stats, &traffic_key_defense);
    // if (defense_bytes) {
    //     __sync_fetch_and_add(defense_bytes, padding_size);
    // }
    
    return TC_ACT_OK;
}

/* ============================================
 * 高级功能：流量分类与自适应
 * ============================================ */

// 识别流量类型
static __always_inline __u32 classify_traffic(struct iphdr *ip, void *data_end) {
    __u32 template_id = 0;  // 默认：Conference-Pro
    
    // TCP 流量
    if (ip->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = (void *)ip + sizeof(*ip);
        if ((void *)(tcp + 1) > data_end)
            return template_id;
        
        __u16 dport = bpf_ntohs(tcp->dest);
        
        // 识别常见端口
        if (dport == 443 || dport == 80) {
            template_id = 1;  // Cinema-Ultra (视频流)
        } else if (dport == 22) {
            template_id = 2;  // SSH-Like
        }
    }
    // UDP 流量
    else if (ip->protocol == IPPROTO_UDP) {
        struct udphdr *udp = (void *)ip + sizeof(*ip);
        if ((void *)(udp + 1) > data_end)
            return template_id;
        
        __u16 dport = bpf_ntohs(udp->dest);
        
        // QUIC/H3
        if (dport == 443) {
            template_id = 1;  // Cinema-Ultra
        }
        // 游戏流量（高频小包）
        else if (dport >= 27000 && dport <= 28000) {
            template_id = 3;  // Gamer-Zero
        }
    }
    
    return template_id;
}

// 自适应 Jitter（根据流量类型）
SEC("tc")
int jitter_lite_adaptive(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    // 1. 识别流量类型
    __u32 template_id = classify_traffic(ip, data_end);
    
    // 2. 查询对应模板配置
    struct jitter_config *cfg = bpf_map_lookup_elem(&jitter_config_map, &template_id);
    
    if (!cfg || !cfg->enabled) {
        // Fallback 到默认模板
        template_id = 0;
        cfg = bpf_map_lookup_elem(&jitter_config_map, &template_id);
        if (!cfg || !cfg->enabled)
            return TC_ACT_OK;
    }
    
    // 3. 应用时域扰动
    __u64 delay_ns = gaussian_sample(cfg->mean_iat_us, cfg->stddev_iat_us) * 1000;
    __u64 now = bpf_ktime_get_ns();
    skb->tstamp = now + delay_ns;
    
    return TC_ACT_OK;
}

/* ============================================
 * 物理噪音注入（增强版 V2）
 * ============================================ */

// VPC 噪音配置（增强版）
struct vpc_noise_profile {
    __u32 fiber_base_us;        // 光缆基础延迟（微秒）
    __u32 fiber_variance_us;    // 光缆抖动方差
    __u32 router_hops;          // 路由器跳数
    __u32 router_queue_us;      // 每跳队列延迟
    __u32 congestion_factor;    // 拥塞因子 (0-100)
    __u32 packet_loss_rate;     // 丢包率 (0-1000, 千分比)
    __u32 reorder_rate;         // 乱序率 (0-100)
    __u32 duplicate_rate;       // 重复率 (0-100)
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 8);     // 支持 8 种地理区域配置
    __type(key, __u32);
    __type(value, struct vpc_noise_profile);
} vpc_noise_profiles SEC(".maps");

// 当前激活的噪音配置
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);
} active_noise_profile SEC(".maps");

// VPC 噪音统计
struct vpc_noise_stats {
    __u64 total_packets;
    __u64 delayed_packets;
    __u64 total_delay_us;
    __u64 dropped_packets;
    __u64 reordered_packets;
    __u64 duplicated_packets;
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct vpc_noise_stats);
} vpc_noise_stats SEC(".maps");

// 模拟光缆抖动（泊松分布近似）
static __always_inline __u64 simulate_fiber_jitter_v2(struct vpc_noise_profile *profile) {
    if (!profile)
        return 0;
    
    __u32 random = bpf_get_prandom_u32();
    
    // 泊松分布近似：使用指数分布
    // λ = 1 / mean, P(X > x) = e^(-λx)
    // 简化：base + random % variance
    __u64 jitter = profile->fiber_base_us;
    jitter += (random % (profile->fiber_variance_us * 2));
    
    return jitter;
}

// 模拟路由器队列延迟（多跳累积）
static __always_inline __u64 simulate_router_queue_v2(struct vpc_noise_profile *profile) {
    if (!profile || profile->router_hops == 0)
        return 0;
    
    __u64 total_delay = 0;
    
    // 每跳独立计算延迟
    #pragma unroll
    for (__u32 i = 0; i < 8 && i < profile->router_hops; i++) {
        __u32 rand = bpf_get_prandom_u32();
        __u64 hop_delay = profile->router_queue_us;
        
        // 拥塞因子影响
        hop_delay += (hop_delay * profile->congestion_factor * (rand % 100)) / 10000;
        
        total_delay += hop_delay;
    }
    
    return total_delay;
}

// 模拟跨洋光缆特征（周期性抖动）
static __always_inline __u64 simulate_submarine_cable(__u64 timestamp) {
    // 海底光缆有周期性的信号放大器，产生规律性抖动
    // 周期约 50-80km，传播延迟约 5μs/km
    
    __u64 cycle = timestamp / 1000000;  // 毫秒级周期
    __u32 phase = cycle % 100;
    
    // 正弦波近似
    __u64 jitter = 0;
    if (phase < 25)
        jitter = phase * 4;
    else if (phase < 50)
        jitter = (50 - phase) * 4;
    else if (phase < 75)
        jitter = (phase - 50) * 4;
    else
        jitter = (100 - phase) * 4;
    
    return jitter;  // 0-100 微秒
}

// 模拟数据中心内部网络抖动
static __always_inline __u64 simulate_datacenter_jitter() {
    __u32 rand = bpf_get_prandom_u32();
    
    // 数据中心内部延迟通常很低（10-500μs）
    // 但偶尔会有微突发（micro-burst）
    
    if ((rand & 0xFF) < 5) {
        // 0.5% 概率出现微突发
        return 500 + (rand % 2000);
    }
    
    return 10 + (rand % 100);
}

// 综合物理噪音（增强版）
SEC("tc")
int jitter_lite_physical(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    // 1. 获取噪音配置
    __u32 key = 0;
    __u32 *profile_id = bpf_map_lookup_elem(&active_noise_profile, &key);
    __u32 pid = profile_id ? *profile_id : 0;
    
    struct vpc_noise_profile *profile = bpf_map_lookup_elem(&vpc_noise_profiles, &pid);
    struct jitter_config *jitter_cfg = bpf_map_lookup_elem(&jitter_config_map, &key);
    
    if (!jitter_cfg || !jitter_cfg->enabled)
        return TC_ACT_OK;
    
    // 2. 更新统计
    struct vpc_noise_stats *stats = bpf_map_lookup_elem(&vpc_noise_stats, &key);
    if (stats)
        __sync_fetch_and_add(&stats->total_packets, 1);
    
    // 3. 丢包模拟
    if (profile && profile->packet_loss_rate > 0) {
        __u32 rand = bpf_get_prandom_u32() % 1000;
        if (rand < profile->packet_loss_rate) {
            if (stats)
                __sync_fetch_and_add(&stats->dropped_packets, 1);
            return TC_ACT_SHOT;  // 丢弃
        }
    }
    
    // 4. 计算综合延迟
    __u64 total_delay_us = 0;
    __u64 now = bpf_ktime_get_ns();
    
    // 4.1 基础 Jitter-Lite 延迟
    total_delay_us += gaussian_sample(jitter_cfg->mean_iat_us, jitter_cfg->stddev_iat_us);
    
    // 4.2 光缆抖动
    if (profile) {
        total_delay_us += simulate_fiber_jitter_v2(profile);
    }
    
    // 4.3 路由器队列延迟
    if (profile) {
        total_delay_us += simulate_router_queue_v2(profile);
    }
    
    // 4.4 跨洋光缆特征
    total_delay_us += simulate_submarine_cable(now);
    
    // 4.5 数据中心抖动
    total_delay_us += simulate_datacenter_jitter();
    
    // 5. 应用延迟到 skb->tstamp
    __u64 delay_ns = total_delay_us * 1000;
    skb->tstamp = now + delay_ns;
    
    // 6. 更新统计
    if (stats) {
        __sync_fetch_and_add(&stats->delayed_packets, 1);
        __sync_fetch_and_add(&stats->total_delay_us, total_delay_us);
    }
    
    // 7. 乱序模拟（通过额外延迟实现）
    if (profile && profile->reorder_rate > 0) {
        __u32 rand = bpf_get_prandom_u32() % 100;
        if (rand < profile->reorder_rate) {
            // 添加额外随机延迟造成乱序
            __u64 extra_delay = (bpf_get_prandom_u32() % 5000) * 1000;  // 0-5ms
            skb->tstamp += extra_delay;
            if (stats)
                __sync_fetch_and_add(&stats->reordered_packets, 1);
        }
    }
    
    return TC_ACT_OK;
}

/* ============================================
 * 地理区域噪音配置预设
 * ============================================ */

// 区域 ID
#define REGION_LOCAL        0   // 本地/同城
#define REGION_DOMESTIC     1   // 国内跨省
#define REGION_ASIA_PACIFIC 2   // 亚太区域
#define REGION_TRANS_PACIFIC 3  // 跨太平洋
#define REGION_TRANS_ATLANTIC 4 // 跨大西洋
#define REGION_GLOBAL       5   // 全球

// 社交时钟感知（根据时间调整噪音）
struct social_clock_config {
    __u32 enabled;
    __u32 peak_hour_start;      // 高峰开始（小时）
    __u32 peak_hour_end;        // 高峰结束
    __u32 peak_multiplier;      // 高峰期噪音倍数 (100 = 1x)
    __u32 night_multiplier;     // 夜间噪音倍数
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct social_clock_config);
} social_clock_map SEC(".maps");

// 获取社交时钟调整因子
static __always_inline __u32 get_social_clock_factor() {
    __u32 key = 0;
    struct social_clock_config *cfg = bpf_map_lookup_elem(&social_clock_map, &key);
    if (!cfg || !cfg->enabled)
        return 100;  // 默认 1x
    
    // 获取当前小时（简化：使用纳秒时间戳）
    __u64 now = bpf_ktime_get_ns();
    __u32 hour = (now / 3600000000000ULL) % 24;
    
    // 判断时段
    if (hour >= cfg->peak_hour_start && hour < cfg->peak_hour_end) {
        return cfg->peak_multiplier;
    }
    
    // 夜间（22:00 - 06:00）
    if (hour >= 22 || hour < 6) {
        return cfg->night_multiplier;
    }
    
    return 100;
}

// 带社交时钟的物理噪音
SEC("tc")
int jitter_lite_social(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    // 1. 获取配置
    __u32 key = 0;
    struct jitter_config *jitter_cfg = bpf_map_lookup_elem(&jitter_config_map, &key);
    if (!jitter_cfg || !jitter_cfg->enabled)
        return TC_ACT_OK;
    
    // 2. 获取社交时钟因子
    __u32 social_factor = get_social_clock_factor();
    
    // 3. 计算基础延迟
    __u64 base_delay = gaussian_sample(jitter_cfg->mean_iat_us, jitter_cfg->stddev_iat_us);
    
    // 4. 应用社交时钟调整
    __u64 adjusted_delay = (base_delay * social_factor) / 100;
    
    // 5. 应用到 skb
    __u64 now = bpf_ktime_get_ns();
    skb->tstamp = now + (adjusted_delay * 1000);
    
    return TC_ACT_OK;
}

/* ============================================
 * VPC 威胁感知（Ingress）
 * ============================================ */

// VPC Ingress 威胁检测
SEC("tc")
int vpc_ingress_detect(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    // 1. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    // 2. 只处理 IPv4
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    // 3. 只处理 TCP
    if (ip->protocol != IPPROTO_TCP)
        return TC_ACT_OK;
    
    struct tcphdr *tcp = (void *)ip + sizeof(*ip);
    if ((void *)(tcp + 1) > data_end)
        return TC_ACT_OK;
    
    // 4. SYN 扫描检测
    if (tcp->syn && !tcp->ack) {
        // 使用 common.h 中的辅助函数上报威胁
        report_threat(
            THREAT_ACTIVE_PROBING,
            ip->saddr,
            bpf_ntohs(tcp->source),
            5  // 中等威胁
        );
        
        // 检测 nmap 特征窗口大小
        __u16 window = bpf_ntohs(tcp->window);
        if (window == 1024 || window == 2048 || window == 4096) {
            // 典型扫描器特征，提升威胁等级
            report_threat(
                THREAT_DPI_INSPECTION,
                ip->saddr,
                bpf_ntohs(tcp->source),
                8  // 高威胁
            );
        }
    }
    
    // 5. 异常 TCP 标志位检测（XMAS 扫描）
    if (tcp->fin && tcp->urg && tcp->psh) {
        report_threat(
            THREAT_ACTIVE_PROBING,
            ip->saddr,
            bpf_ntohs(tcp->source),
            7  // 高威胁
        );
    }
    
    // 6. NULL 扫描检测
    if (!tcp->syn && !tcp->ack && !tcp->fin && !tcp->rst && !tcp->psh && !tcp->urg) {
        report_threat(
            THREAT_ACTIVE_PROBING,
            ip->saddr,
            bpf_ntohs(tcp->source),
            6  // 中高威胁
        );
    }
    
    return TC_ACT_OK;
}

/* ============================================
 * 紧急自毁逻辑（Dead Man's Switch）
 * ============================================ */

// 紧急指令码
#define EMERGENCY_WIPE_CODE 0xDEADBEEF

// emergency_ctrl_map 已在 common.h 中定义

// 清空单个 Map 的所有条目
static __always_inline int wipe_map_entries(void *map, __u32 max_entries) {
    // 注意：eBPF 不支持动态迭代删除
    // 这里采用覆盖策略：将所有 key 对应的 value 置零
    for (__u32 i = 0; i < max_entries && i < 16; i++) {
        __u32 key = i;
        __u64 zero = 0;
        bpf_map_update_elem(map, &key, &zero, BPF_ANY);
    }
    return 0;
}

// 紧急自毁入口（由 Go 控制面触发）
SEC("tc")
int emergency_wipe(struct __sk_buff *skb) {
    // 1. 检查紧急指令码
    __u32 key = 0;
    __u32 *cmd = bpf_map_lookup_elem(&emergency_ctrl_map, &key);
    
    if (!cmd || *cmd != EMERGENCY_WIPE_CODE) {
        // 未收到紧急指令，正常放行
        return TC_ACT_OK;
    }
    
    // 2. 清空所有敏感 Map
    // 注意：由于 eBPF 限制，这里只能清空部分条目
    // 完整清空需要 Go 控制面配合
    
    // 清空 B-DNA 模板
    wipe_map_entries(&dna_template_map, 16);
    
    // 清空 Jitter 配置
    wipe_map_entries(&jitter_config_map, 16);
    
    // 清空 NPM 配置
    wipe_map_entries(&npm_config_map, 1);
    
    // 清空 VPC 配置
    wipe_map_entries(&vpc_config_map, 1);
    
    // 清空配额状态
    wipe_map_entries(&quota_map, 1);
    
    // 清空蜂窝阶段
    wipe_map_entries(&cell_phase_map, 1);
    
    // 3. 重置紧急指令码
    __u32 zero = 0;
    bpf_map_update_elem(&emergency_ctrl_map, &key, &zero, BPF_ANY);
    
    // 4. 丢弃所有后续流量（进入静默模式）
    return TC_ACT_STOLEN;
}

// 心跳检测入口（检测控制面是否存活）
SEC("tc")
int heartbeat_check(struct __sk_buff *skb) {
    // 检查是否收到紧急自毁指令
    __u32 key = 0;
    __u32 *cmd = bpf_map_lookup_elem(&emergency_ctrl_map, &key);
    
    if (cmd && *cmd == EMERGENCY_WIPE_CODE) {
        // 触发紧急自毁
        return emergency_wipe(skb);
    }
    
    return TC_ACT_OK;
}
