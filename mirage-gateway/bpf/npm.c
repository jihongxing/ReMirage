/* NPM - 流量伪装协议 (Network Packet Morphing)
 * 核心功能：XDP 层概率填充 + 包长度统一
 * 挂载点：XDP (eXpress Data Path)
 * 
 * 对抗"包长序列（SPL）"分析的关键武器
 */

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#include "common.h"

/* ============================================
 * NPM 专用 Map 定义
 * ============================================ */

// NPM 全局配置
struct npm_global_config {
    __u32 enabled;              // 是否启用
    __u32 filling_rate;         // 填充概率 (0-100)
    __u32 global_mtu;           // 全局 MTU (默认 1460)
    __u32 min_packet_size;      // 最小包大小阈值
    __u32 padding_mode;         // 0=固定MTU, 1=随机范围, 2=正态分布
    __u32 decoy_rate;           // 诱饵包注入率 (0-100)
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct npm_global_config);
} npm_global_map SEC(".maps");

// NPM 统计
struct npm_stats {
    __u64 total_packets;        // 总包数
    __u64 padded_packets;       // 填充包数
    __u64 padding_bytes;        // 填充字节数
    __u64 decoy_packets;        // 诱饵包数
    __u64 skipped_packets;      // 跳过包数
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct npm_stats);
} npm_stats_map SEC(".maps");

// 填充模式常量
#define NPM_MODE_FIXED_MTU      0
#define NPM_MODE_RANDOM_RANGE   1
#define NPM_MODE_GAUSSIAN       2

// 默认 MTU
#define DEFAULT_GLOBAL_MTU      1460
#define MIN_PADDING_SIZE        64
#define MAX_PADDING_SIZE        1400

/* ============================================
 * 辅助函数
 * ============================================ */

// 计算填充大小（对齐到 MTU）
static __always_inline __u32 calculate_padding(
    __u32 current_size,
    __u32 target_mtu,
    __u32 mode
) {
    if (current_size >= target_mtu)
        return 0;
    
    __u32 padding = 0;
    
    switch (mode) {
    case NPM_MODE_FIXED_MTU:
        // 固定对齐到 MTU
        padding = target_mtu - current_size;
        break;
        
    case NPM_MODE_RANDOM_RANGE:
        // 随机范围填充
        {
            __u32 max_pad = target_mtu - current_size;
            __u32 rand = bpf_get_prandom_u32();
            padding = (rand % max_pad) + 1;
        }
        break;
        
    case NPM_MODE_GAUSSIAN:
        // 正态分布填充（简化版）
        {
            __u32 target = (target_mtu + current_size) / 2;
            __u32 rand = bpf_get_prandom_u32();
            __u32 variance = (target_mtu - current_size) / 4;
            __s32 offset = (rand % (variance * 2)) - variance;
            padding = target - current_size + offset;
            if (padding > target_mtu - current_size)
                padding = target_mtu - current_size;
        }
        break;
    }
    
    // 确保填充大小合理
    if (padding < MIN_PADDING_SIZE && padding > 0)
        padding = MIN_PADDING_SIZE;
    if (padding > MAX_PADDING_SIZE)
        padding = MAX_PADDING_SIZE;
    
    return padding;
}

// 填充随机数据（熵注入）
static __always_inline void fill_random_padding(
    void *data,
    void *data_end,
    __u32 offset,
    __u32 size
) {
    // eBPF 限制：无法直接写入大量数据
    // 使用 bpf_xdp_adjust_tail 扩展后，新区域自动为零
    // 这里可以选择性注入随机字节
    
    __u8 *ptr = data + offset;
    if ((void *)(ptr + 4) > data_end)
        return;
    
    // 注入随机魔数（用于调试和验证）
    __u32 magic = bpf_get_prandom_u32();
    *(__u32 *)ptr = magic;
}

/* ============================================
 * XDP 核心程序：出口填充
 * ============================================ */

SEC("xdp")
int npm_padding_egress(struct xdp_md *ctx)
{
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    
    // 1. 获取配置
    __u32 key = 0;
    struct npm_global_config *cfg = bpf_map_lookup_elem(&npm_global_map, &key);
    if (!cfg || !cfg->enabled)
        return XDP_PASS;
    
    // 2. 更新统计
    struct npm_stats *stats = bpf_map_lookup_elem(&npm_stats_map, &key);
    if (stats)
        __sync_fetch_and_add(&stats->total_packets, 1);
    
    // 3. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;
    
    // 4. 只处理 IPv4
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return XDP_PASS;
    
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return XDP_PASS;
    
    // 5. 计算当前包大小
    __u32 current_size = data_end - data;
    
    // 6. 小包跳过（避免填充控制包）
    if (current_size < cfg->min_packet_size) {
        if (stats)
            __sync_fetch_and_add(&stats->skipped_packets, 1);
        return XDP_PASS;
    }
    
    // 7. 概率决策：是否填充
    __u32 rand = bpf_get_prandom_u32();
    __u32 threshold = (cfg->filling_rate * 0xFFFFFFFF) / 100;
    
    if (rand > threshold) {
        // 不填充
        return XDP_PASS;
    }
    
    // 8. 计算填充大小
    __u32 target_mtu = cfg->global_mtu ? cfg->global_mtu : DEFAULT_GLOBAL_MTU;
    __u32 padding = calculate_padding(current_size, target_mtu, cfg->padding_mode);
    
    if (padding == 0)
        return XDP_PASS;
    
    // 9. 执行尾部扩展
    int ret = bpf_xdp_adjust_tail(ctx, padding);
    if (ret < 0) {
        // 扩展失败（可能超过 MTU）
        return XDP_PASS;
    }
    
    // 10. 重新获取数据指针（adjust 后指针可能变化）
    data = (void *)(long)ctx->data;
    data_end = (void *)(long)ctx->data_end;
    
    // 11. 更新 IP 头长度
    ip = data + sizeof(struct ethhdr);
    if ((void *)(ip + 1) > data_end)
        return XDP_PASS;
    
    __u16 old_len = ip->tot_len;
    __u16 new_len = bpf_htons(bpf_ntohs(old_len) + padding);
    ip->tot_len = new_len;
    
    // 12. 重新计算 IP 校验和（增量更新）
    __u32 csum = (~ip->check) & 0xFFFF;
    csum += (~old_len) & 0xFFFF;
    csum += new_len;
    while (csum >> 16)
        csum = (csum & 0xFFFF) + (csum >> 16);
    ip->check = ~csum;
    
    // 13. 填充随机数据
    fill_random_padding(data, data_end, current_size, padding);
    
    // 14. 更新统计
    if (stats) {
        __sync_fetch_and_add(&stats->padded_packets, 1);
        __sync_fetch_and_add(&stats->padding_bytes, padding);
    }
    
    return XDP_PASS;
}

/* ============================================
 * XDP 入口程序：剥离填充
 * ============================================ */

SEC("xdp")
int npm_strip_ingress(struct xdp_md *ctx)
{
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    
    // 1. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return XDP_PASS;
    
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return XDP_PASS;
    
    // 2. 计算实际 IP 长度 vs 帧长度
    __u32 frame_len = data_end - data;
    __u32 ip_len = bpf_ntohs(ip->tot_len) + sizeof(struct ethhdr);
    
    // 3. 如果帧长度大于 IP 长度，说明有填充
    if (frame_len > ip_len) {
        __u32 padding = frame_len - ip_len;
        
        // 剥离填充
        int ret = bpf_xdp_adjust_tail(ctx, -(__s32)padding);
        if (ret < 0)
            return XDP_PASS;
    }
    
    return XDP_PASS;
}

/* ============================================
 * 诱饵包注入（TC 层）
 * ============================================ */

// 诱饵包类型
#define DECOY_TYPE_EMPTY    0   // 空载包
#define DECOY_TYPE_REPLAY   1   // 重复包
#define DECOY_TYPE_REORDER  2   // 乱序包

struct decoy_config {
    __u32 type;
    __u32 interval_ms;
    __u32 burst_count;
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct decoy_config);
} decoy_config_map SEC(".maps");

// 诱饵包计数器
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} decoy_counter SEC(".maps");

/* ============================================
 * TC 程序：诱饵包标记
 * ============================================ */

SEC("tc")
int npm_decoy_marker(struct __sk_buff *skb)
{
    // 1. 获取配置
    __u32 key = 0;
    struct npm_global_config *cfg = bpf_map_lookup_elem(&npm_global_map, &key);
    if (!cfg || !cfg->enabled || cfg->decoy_rate == 0)
        return TC_ACT_OK;
    
    // 2. 概率决策
    __u32 rand = bpf_get_prandom_u32();
    __u32 threshold = (cfg->decoy_rate * 0xFFFFFFFF) / 100;
    
    if (rand > threshold)
        return TC_ACT_OK;
    
    // 3. 标记为诱饵包（通过 skb->mark）
    skb->mark |= 0x4D495241;  // "MIRA" magic
    
    // 4. 更新统计
    struct npm_stats *stats = bpf_map_lookup_elem(&npm_stats_map, &key);
    if (stats)
        __sync_fetch_and_add(&stats->decoy_packets, 1);
    
    return TC_ACT_OK;
}

/* ============================================
 * MTU 动态探测（辅助功能）
 * ============================================ */

struct mtu_probe_state {
    __u32 current_mtu;
    __u32 min_mtu;
    __u32 max_mtu;
    __u32 probe_count;
    __u64 last_probe_time;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, __u32);         // 目标 IP
    __type(value, struct mtu_probe_state);
} mtu_probe_map SEC(".maps");

// MTU 探测结果上报
struct mtu_event {
    __u64 timestamp;
    __u32 target_ip;
    __u32 discovered_mtu;
    __u32 probe_count;
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 64 * 1024);
} mtu_events SEC(".maps");

SEC("xdp")
int npm_mtu_probe(struct xdp_md *ctx)
{
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return XDP_PASS;
    
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return XDP_PASS;
    
    // 检测 ICMP "Fragmentation Needed" 消息
    if (ip->protocol != 1)  // ICMP
        return XDP_PASS;
    
    // 解析 ICMP
    __u8 *icmp = (void *)ip + (ip->ihl * 4);
    if ((void *)(icmp + 8) > data_end)
        return XDP_PASS;
    
    __u8 type = icmp[0];
    __u8 code = icmp[1];
    
    // Type 3, Code 4 = Fragmentation Needed
    if (type == 3 && code == 4) {
        // 提取 Next-Hop MTU
        __u16 next_mtu = *(__u16 *)(icmp + 6);
        next_mtu = bpf_ntohs(next_mtu);
        
        // 更新 MTU 探测状态
        __u32 src_ip = ip->saddr;
        struct mtu_probe_state *state = bpf_map_lookup_elem(&mtu_probe_map, &src_ip);
        
        if (state) {
            state->current_mtu = next_mtu;
            state->probe_count++;
            state->last_probe_time = bpf_ktime_get_ns();
        } else {
            struct mtu_probe_state new_state = {
                .current_mtu = next_mtu,
                .min_mtu = next_mtu,
                .max_mtu = 1500,
                .probe_count = 1,
                .last_probe_time = bpf_ktime_get_ns(),
            };
            bpf_map_update_elem(&mtu_probe_map, &src_ip, &new_state, BPF_ANY);
        }
        
        // 上报事件
        struct mtu_event *event = bpf_ringbuf_reserve(&mtu_events, sizeof(*event), 0);
        if (event) {
            event->timestamp = bpf_ktime_get_ns();
            event->target_ip = src_ip;
            event->discovered_mtu = next_mtu;
            event->probe_count = state ? state->probe_count : 1;
            bpf_ringbuf_submit(event, 0);
        }
    }
    
    return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
