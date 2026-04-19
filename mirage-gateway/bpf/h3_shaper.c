// SPDX-License-Identifier: GPL-2.0
// H3 流量整形器 (eBPF TC)
// 功能：CID 替换、MTU 对齐、噪声注入

#include "common.h"

// ============== 数据结构 ==============

// CID 轮换配置（支持双 CID 静默期）
struct cid_config {
    __u8 active_cid[8];     // 当前活跃 CID
    __u8 graceful_cid[8];   // 静默期旧 CID
    __u64 graceful_expire;  // 静默期过期时间 (ns)
    __u8 noise_rate;        // 0-100
    __u8 graceful_enabled;  // 是否启用静默期
    __u8 padding[6];
};

// H3 配置
struct h3_config {
    __u8 frame_type;
    __u8 mimicry_type;  // 0=YouTube, 1=Netflix, 2=Zoom, 3=Spotify
    __u16 padding_min;
    __u16 padding_max;
    __u16 mtu_target;
    __u8 deep_mimicry;  // 前 N 包深度拟态
    __u8 reserved;
};

// 噪声配置
struct noise_config {
    __u8 enabled;
    __u8 rate;          // 0-100
    __u16 min_size;
    __u16 max_size;
    __u8 reserved[2];
};

// 统计
struct h3_stats {
    __u64 packets_processed;
    __u64 cid_replaced;
    __u64 padding_added;
    __u64 noise_injected;
};

// ============== eBPF Maps ==============

// CID 轮换映射 (Go 下发)
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, __u32);
    __type(value, struct cid_config);
} cid_rotation_map SEC(".maps");

// H3 配置映射 (Go 下发)
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct h3_config);
} h3_config_map SEC(".maps");

// 噪声配置映射 (Go 下发)
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct noise_config);
} noise_config_map SEC(".maps");

// 统计映射
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct h3_stats);
} h3_stats_map SEC(".maps");

// 事件上报 Ring Buffer
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} h3_events SEC(".maps");

// 事件结构
struct h3_event {
    __u64 timestamp;
    __u8 event_type;    // 0=CID替换, 1=Padding, 2=Noise
    __u8 old_cid[8];
    __u8 new_cid[8];
    __u16 packet_len;
    __u8 reserved[5];
};

// ============== 辅助函数 ==============

// 静默期时长 (300ms = 300,000,000 ns)
#define GRACEFUL_PERIOD_NS 300000000ULL

// UDP 头部偏移
#define ETH_HLEN 14
#define IP_HLEN 20
#define UDP_HLEN 8
#define UDP_PAYLOAD_OFFSET (ETH_HLEN + IP_HLEN + UDP_HLEN)

// 获取 UDP 负载偏移
static __always_inline int get_udp_payload_offset(struct __sk_buff *skb) {
    return UDP_PAYLOAD_OFFSET;
}

// 检查是否为 QUIC 包
static __always_inline int is_quic_packet(struct __sk_buff *skb, int offset) {
    __u8 first_byte;
    if (bpf_skb_load_bytes(skb, offset, &first_byte, 1) < 0)
        return 0;
    
    // Long Header: 第一位为 1
    // Short Header: 第一位为 0，第二位为 1
    return (first_byte & 0x80) || (first_byte & 0x40);
}

// 获取 CID 偏移（Short Header）
static __always_inline int get_cid_offset_short(int udp_offset) {
    // Short Header: Form(1) + DCID
    return udp_offset + 1;
}

// 获取 CID 偏移（Long Header）
static __always_inline int get_cid_offset_long(struct __sk_buff *skb, int udp_offset) {
    // Long Header: Form(1) + Version(4) + DCID_Len(1) + DCID
    __u8 dcid_len;
    if (bpf_skb_load_bytes(skb, udp_offset + 5, &dcid_len, 1) < 0)
        return -1;
    return udp_offset + 6;
}

// 替换 CID（带增量 Checksum 修正）
static __always_inline int replace_cid_with_csum(struct __sk_buff *skb, int offset, 
                                                   __u8 *old_cid, __u8 *new_cid, int cid_len) {
    // 1. 替换 CID 字节
    int ret = bpf_skb_store_bytes(skb, offset, new_cid, cid_len, 0);
    if (ret < 0)
        return ret;

    // 2. 增量 Checksum 修正 (RFC 1624)
    // UDP Checksum 偏移: ETH(14) + IP(20) + 6 = 40
    int csum_offset = ETH_HLEN + IP_HLEN + 6;

    // 手动展开：对每 2 字节进行增量更新 (避免 break 导致无法展开)
    __u16 old_val, new_val;

    old_val = ((__u16)old_cid[0] << 8) | old_cid[1];
    new_val = ((__u16)new_cid[0] << 8) | new_cid[1];
    if (old_val != new_val)
        bpf_l4_csum_replace(skb, csum_offset, old_val, new_val, BPF_F_PSEUDO_HDR | sizeof(__u16));

    old_val = ((__u16)old_cid[2] << 8) | old_cid[3];
    new_val = ((__u16)new_cid[2] << 8) | new_cid[3];
    if (old_val != new_val)
        bpf_l4_csum_replace(skb, csum_offset, old_val, new_val, BPF_F_PSEUDO_HDR | sizeof(__u16));

    old_val = ((__u16)old_cid[4] << 8) | old_cid[5];
    new_val = ((__u16)new_cid[4] << 8) | new_cid[5];
    if (old_val != new_val)
        bpf_l4_csum_replace(skb, csum_offset, old_val, new_val, BPF_F_PSEUDO_HDR | sizeof(__u16));

    old_val = ((__u16)old_cid[6] << 8) | old_cid[7];
    new_val = ((__u16)new_cid[6] << 8) | new_cid[7];
    if (old_val != new_val)
        bpf_l4_csum_replace(skb, csum_offset, old_val, new_val, BPF_F_PSEUDO_HDR | sizeof(__u16));

    return 0;
}

// 检查 CID 是否匹配（支持双 CID）
static __always_inline int check_cid_match(struct __sk_buff *skb, int cid_offset,
                                            struct cid_config *cfg, __u8 *matched_cid) {
    __u8 current_cid[8];
    if (bpf_skb_load_bytes(skb, cid_offset, current_cid, 8) < 0)
        return 0;

    // 检查活跃 CID (手动展开，避免 break)
    int match_active = (current_cid[0] == cfg->active_cid[0]) &&
                       (current_cid[1] == cfg->active_cid[1]) &&
                       (current_cid[2] == cfg->active_cid[2]) &&
                       (current_cid[3] == cfg->active_cid[3]) &&
                       (current_cid[4] == cfg->active_cid[4]) &&
                       (current_cid[5] == cfg->active_cid[5]) &&
                       (current_cid[6] == cfg->active_cid[6]) &&
                       (current_cid[7] == cfg->active_cid[7]);

    if (match_active) {
        __builtin_memcpy(matched_cid, cfg->active_cid, 8);
        return 1; // 匹配活跃 CID
    }

    // 检查静默期 CID
    if (cfg->graceful_enabled) {
        __u64 now = bpf_ktime_get_ns();
        if (now < cfg->graceful_expire) {
            // 手动展开，避免 break
            int match_graceful = (current_cid[0] == cfg->graceful_cid[0]) &&
                                 (current_cid[1] == cfg->graceful_cid[1]) &&
                                 (current_cid[2] == cfg->graceful_cid[2]) &&
                                 (current_cid[3] == cfg->graceful_cid[3]) &&
                                 (current_cid[4] == cfg->graceful_cid[4]) &&
                                 (current_cid[5] == cfg->graceful_cid[5]) &&
                                 (current_cid[6] == cfg->graceful_cid[6]) &&
                                 (current_cid[7] == cfg->graceful_cid[7]);
            if (match_graceful) {
                __builtin_memcpy(matched_cid, cfg->graceful_cid, 8);
                return 2; // 匹配静默期 CID
            }
        }
    }

    return 0; // 不匹配
}

// 计算需要的 Padding
static __always_inline __u16 calculate_padding(struct h3_config *cfg, __u16 current_len) {
    if (!cfg || cfg->mtu_target == 0)
        return 0;
    
    // 对齐到 MTU 目标
    if (current_len >= cfg->mtu_target)
        return 0;
    
    __u16 diff = cfg->mtu_target - current_len;
    
    // 限制在 min-max 范围内
    if (diff < cfg->padding_min)
        return cfg->padding_min;
    if (diff > cfg->padding_max)
        return cfg->padding_max;
    
    return diff;
}

// 更新统计
static __always_inline void update_stats(int field) {
    __u32 key = 0;
    struct h3_stats *stats = bpf_map_lookup_elem(&h3_stats_map, &key);
    if (!stats)
        return;
    
    switch (field) {
        case 0: stats->packets_processed++; break;
        case 1: stats->cid_replaced++; break;
        case 2: stats->padding_added++; break;
        case 3: stats->noise_injected++; break;
    }
}

// 上报事件
static __always_inline void report_event(__u8 type, __u8 *old_cid, __u8 *new_cid, __u16 len) {
    struct h3_event *event = bpf_ringbuf_reserve(&h3_events, sizeof(*event), 0);
    if (!event)
        return;
    
    event->timestamp = bpf_ktime_get_ns();
    event->event_type = type;
    event->packet_len = len;
    
    if (old_cid)
        __builtin_memcpy(event->old_cid, old_cid, 8);
    if (new_cid)
        __builtin_memcpy(event->new_cid, new_cid, 8);
    
    bpf_ringbuf_submit(event, 0);
}

// ============== TC 程序 ==============

SEC("tc")
int h3_shaper_egress(struct __sk_buff *skb) {
    // 获取 UDP 负载偏移
    int udp_offset = get_udp_payload_offset(skb);
    
    // 检查是否为 QUIC 包
    if (!is_quic_packet(skb, udp_offset))
        return TC_ACT_OK;
    
    update_stats(0); // packets_processed
    
    // 获取配置
    __u32 key = 0;
    struct h3_config *h3_cfg = bpf_map_lookup_elem(&h3_config_map, &key);
    struct noise_config *noise_cfg = bpf_map_lookup_elem(&noise_config_map, &key);
    struct cid_config *cid_cfg = bpf_map_lookup_elem(&cid_rotation_map, &key);
    
    // 1. CID 替换（支持双 CID 静默期）
    if (cid_cfg) {
        __u8 first_byte;
        if (bpf_skb_load_bytes(skb, udp_offset, &first_byte, 1) == 0) {
            int cid_offset;
            
            if (first_byte & 0x80) {
                // Long Header
                cid_offset = get_cid_offset_long(skb, udp_offset);
            } else {
                // Short Header
                cid_offset = get_cid_offset_short(udp_offset);
            }
            
            if (cid_offset > 0) {
                __u8 matched_cid[8];
                int match_type = check_cid_match(skb, cid_offset, cid_cfg, matched_cid);
                
                if (match_type > 0) {
                    // 使用增量 Checksum 修正替换 CID
                    if (replace_cid_with_csum(skb, cid_offset, matched_cid, 
                                               cid_cfg->active_cid, 8) == 0) {
                        update_stats(1); // cid_replaced
                        report_event(0, matched_cid, cid_cfg->active_cid, skb->len);
                    }
                }
            }
        }
    }
    
    // 2. MTU 对齐 Padding
    if (h3_cfg && h3_cfg->mtu_target > 0) {
        __u16 padding = calculate_padding(h3_cfg, skb->len);
        if (padding > 0) {
            // 使用 bpf_skb_change_tail 添加 padding
            int new_len = skb->len + padding;
            if (bpf_skb_change_tail(skb, new_len, 0) == 0) {
                update_stats(2); // padding_added
            }
        }
    }
    
    // 3. 噪声注入
    if (noise_cfg && noise_cfg->enabled) {
        __u32 rand = bpf_get_prandom_u32();
        if ((rand % 100) < noise_cfg->rate) {
            // 在包尾添加随机噪声
            __u16 noise_size = noise_cfg->min_size + 
                (rand % (noise_cfg->max_size - noise_cfg->min_size + 1));
            
            int new_len = skb->len + noise_size;
            if (bpf_skb_change_tail(skb, new_len, 0) == 0) {
                update_stats(3); // noise_injected
                report_event(2, NULL, NULL, noise_size);
            }
        }
    }
    
    return TC_ACT_OK;
}

// Ingress 处理（可选）
SEC("tc")
int h3_shaper_ingress(struct __sk_buff *skb) {
    // 入站流量暂不处理
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
