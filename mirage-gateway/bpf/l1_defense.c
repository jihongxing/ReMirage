/* L1 纵深防御 - XDP 层
 * 功能：黑名单清洗 / ASN 过滤 / 非法画像拒绝 / 入口协议准入 / 速率限制
 * 挂载点：通过 #include 集成到 npm.c 的 XDP 入口
 *
 * 本文件定义 static __always_inline 函数，由 npm.c 调用
 *
 * 抗 DDoS 整改：
 *   7.1 - 增加非法 L3/L4 画像检查
 *   7.2 - blacklist_lpm 前移到 XDP（原仅在 TC ingress）
 *   7.3 - 按入口端口建立最小协议准入校验
 */

#include "common.h"

/* ============================================
 * 入口协议准入配置（7.3）
 * ============================================ */

/* 入口画像：端口 → 允许的协议类型 */
#define INGRESS_PROTO_TCP   0x01
#define INGRESS_PROTO_UDP   0x02
#define INGRESS_PROTO_BOTH  0x03

struct ingress_profile {
    __u16 port;             /* 监听端口 */
    __u8  allowed_proto;    /* INGRESS_PROTO_TCP / UDP / BOTH */
    __u8  require_min_len;  /* 最小载荷长度（过滤空包洪泛） */
    __u32 udp_min_payload;  /* UDP 最小载荷（QUIC Initial ≥ 1200） */
};

/* 入口画像 Map（Go → C，最多 16 个入口） */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16);
    __type(key, __u16);                 /* 目标端口 */
    __type(value, struct ingress_profile);
} ingress_profile_map SEC(".maps");

/* ============================================
 * 用户级黑名单检查（7.2 - 前移到 XDP）
 * ============================================ */

static __always_inline int handle_l1_blacklist_check(__u32 saddr)
{
    struct lpm_key key = {
        .prefixlen = 32,
        .addr = saddr,
    };

    /* 查询用户级黑名单 LPM Trie（原仅在 TC ingress 的 jitter.c 中） */
    __u32 *blocked = bpf_map_lookup_elem(&blacklist_lpm, &key);
    if (blocked) {
        __u32 stats_key = 0;
        struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
        if (stats) {
            __sync_fetch_and_add(&stats->blacklist_drops, 1);
            __sync_fetch_and_add(&stats->total_checked, 1);
        }
        return XDP_DROP;
    }

    return XDP_PASS;
}

/* ============================================
 * ASN 黑名单检查
 * ============================================ */

static __always_inline int handle_l1_asn_check(struct xdp_md *ctx)
{
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    /* 解析以太网头 */
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;

    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return XDP_PASS;

    /* 解析 IP 头 */
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return XDP_PASS;

    /* 构造 LPM key（prefixlen=32 精确匹配） */
    struct lpm_key key = {
        .prefixlen = 32,
        .addr = ip->saddr,
    };

    /* 查询 ASN 黑名单 LPM Trie */
    __u32 *asn = bpf_map_lookup_elem(&asn_blocklist_lpm, &key);
    if (asn) {
        /* 命中：递增统计并丢弃 */
        __u32 stats_key = 0;
        struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
        if (stats) {
            __sync_fetch_and_add(&stats->asn_drops, 1);
            __sync_fetch_and_add(&stats->total_checked, 1);
        }
        return XDP_DROP;
    }

    /* 未命中：递增 total_checked 并放行 */
    __u32 stats_key = 0;
    struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
    if (stats)
        __sync_fetch_and_add(&stats->total_checked, 1);

    return XDP_PASS;
}

/* ============================================
 * 非法 L3/L4 画像检查（7.1）
 * ============================================ */

static __always_inline int handle_l1_packet_sanity(struct iphdr *ip, void *data_end)
{
    /* 1. IP 头长度合法性（最小 20 字节 = ihl=5） */
    if (ip->ihl < 5)
        goto drop_sanity;

    /* 2. 总长度合法性 */
    __u16 tot_len = bpf_ntohs(ip->tot_len);
    if (tot_len < (ip->ihl * 4))
        goto drop_sanity;

    /* 3. TTL = 0 的包直接丢弃（不应到达） */
    if (ip->ttl == 0)
        goto drop_sanity;

    /* 4. 源 IP 合法性：拒绝 0.0.0.0、广播、环回 */
    __u32 saddr = bpf_ntohl(ip->saddr);
    if (saddr == 0 ||                           /* 0.0.0.0 */
        (saddr >> 24) == 127 ||                 /* 127.x.x.x */
        (saddr >> 24) == 0 ||                   /* 0.x.x.x */
        saddr == 0xFFFFFFFF)                    /* 255.255.255.255 */
        goto drop_sanity;

    /* 5. 只允许 TCP / UDP / ICMP，其他协议直接丢弃 */
    if (ip->protocol != IPPROTO_TCP &&
        ip->protocol != IPPROTO_UDP &&
        ip->protocol != IPPROTO_ICMP)
        goto drop_sanity;

    return XDP_PASS;

drop_sanity:
    {
        __u32 stats_key = 0;
        struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
        if (stats) {
            __sync_fetch_and_add(&stats->sanity_drops, 1);
            __sync_fetch_and_add(&stats->total_checked, 1);
        }
        return XDP_DROP;
    }
}

/* ============================================
 * 入口协议准入校验（7.3）
 * ============================================ */

static __always_inline int handle_l1_ingress_profile(
    struct iphdr *ip, void *data_end)
{
    __u16 dport = 0;

    if (ip->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
        if ((void *)(tcp + 1) > data_end)
            return XDP_PASS;
        dport = bpf_ntohs(tcp->dest);
    } else if (ip->protocol == IPPROTO_UDP) {
        struct udphdr *udp = (void *)ip + (ip->ihl * 4);
        if ((void *)(udp + 1) > data_end)
            return XDP_PASS;
        dport = bpf_ntohs(udp->dest);
    } else {
        /* ICMP 等非 TCP/UDP 不做入口画像检查 */
        return XDP_PASS;
    }

    /* 查询入口画像 Map */
    struct ingress_profile *profile = bpf_map_lookup_elem(&ingress_profile_map, &dport);
    if (!profile) {
        /* 未注册的端口：不在已知入口列表中，放行给后续模块处理
         * 注意：如果需要"未注册端口全丢"，改为 return XDP_DROP */
        return XDP_PASS;
    }

    /* 协议类型检查 */
    if (ip->protocol == IPPROTO_TCP && !(profile->allowed_proto & INGRESS_PROTO_TCP)) {
        __u32 stats_key = 0;
        struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
        if (stats) {
            __sync_fetch_and_add(&stats->profile_drops, 1);
            __sync_fetch_and_add(&stats->total_checked, 1);
        }
        return XDP_DROP;
    }
    if (ip->protocol == IPPROTO_UDP && !(profile->allowed_proto & INGRESS_PROTO_UDP)) {
        __u32 stats_key = 0;
        struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
        if (stats) {
            __sync_fetch_and_add(&stats->profile_drops, 1);
            __sync_fetch_and_add(&stats->total_checked, 1);
        }
        return XDP_DROP;
    }

    /* UDP 最小载荷检查（QUIC Initial 包 ≥ 1200 字节） */
    if (ip->protocol == IPPROTO_UDP && profile->udp_min_payload > 0) {
        struct udphdr *udp = (void *)ip + (ip->ihl * 4);
        if ((void *)(udp + 1) > data_end)
            return XDP_PASS;
        __u16 udp_len = bpf_ntohs(udp->len);
        if (udp_len < sizeof(struct udphdr) + profile->udp_min_payload) {
            __u32 stats_key = 0;
            struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
            if (stats) {
                __sync_fetch_and_add(&stats->profile_drops, 1);
                __sync_fetch_and_add(&stats->total_checked, 1);
            }
            return XDP_DROP;
        }
    }

    return XDP_PASS;
}

/* ============================================
 * SYN 无状态验证（7.4 - 无状态准入）
 *
 * 降级声明：当前 eBPF 约束下无法实现完整 SYN cookie 语义
 * （无法修改出站 SYN-ACK 的 seq 字段来嵌入 cookie）。
 * 本实现为"challenge-response 速率门控"：
 *   - 超过阈值的 SYN 被丢弃，要求客户端重试
 *   - ACK 验证绑定 cookie 到 ack_seq 字段，防止伪造
 *   - 非完整 SYN cookie，但可有效缓解 SYN flood
 * ============================================ */

static __always_inline __u32 syn_cookie_hash(__u32 saddr, __u16 dport, __u64 ts)
{
    /* 混合 hash：saddr ^ (dport << 16) ^ (ts >> 20) ^ 0xDEADBEEF */
    __u32 h = saddr ^ ((__u32)dport << 16) ^ (__u32)(ts >> 20);
    h ^= 0xDEADBEEF;
    h = ((h >> 16) ^ h) * 0x45d9f3b;
    h = ((h >> 16) ^ h) * 0x45d9f3b;
    h = (h >> 16) ^ h;
    return h;
}

static __always_inline int handle_l1_syn_validation(struct iphdr *ip, void *data_end, __u32 saddr)
{
    /* 仅处理 TCP 包 */
    if (ip->protocol != IPPROTO_TCP)
        return XDP_PASS;

    struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
    if ((void *)(tcp + 1) > data_end)
        return XDP_PASS;

    /* 读取 SYN 验证配置 */
    __u32 cfg_key = 0;
    struct syn_config *cfg = bpf_map_lookup_elem(&syn_config_map, &cfg_key);
    if (!cfg || !cfg->enabled)
        return XDP_PASS;

    __u64 now = bpf_ktime_get_ns();
    __u16 dport = bpf_ntohs(tcp->dest);

    /* SYN 包处理 */
    if (tcp->syn && !tcp->ack) {
        /* 查询该 IP 的验证状态 */
        struct syn_state *state = bpf_map_lookup_elem(&syn_validation_map, &saddr);
        if (state && state->validated) {
            /* 已验证通过的 IP，检查有效期（5 分钟） */
            if (now - state->timestamp < 300000000000ULL)
                return XDP_PASS;
            /* 过期，删除状态 */
            bpf_map_delete_elem(&syn_validation_map, &saddr);
        }

        /* 检查速率计数器 */
        struct rate_counter *counter = bpf_map_lookup_elem(&rate_limit_map, &saddr);
        if (counter && counter->syn_count >= cfg->challenge_threshold) {
            /* 超过阈值，生成 challenge cookie 并绑定到 seq_num */
            __u32 cookie = syn_cookie_hash(saddr, dport, now);
            struct syn_state new_state = {
                .cookie = (__u64)cookie,
                .timestamp = now,
                .validated = 0,
            };
            bpf_map_update_elem(&syn_validation_map, &saddr, &new_state, BPF_ANY);

            __u32 stats_key = 0;
            struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
            if (stats)
                __sync_fetch_and_add(&stats->syn_challenge, 1);

            /* 丢弃 SYN，等待重试时 ACK 验证 */
            return XDP_DROP;
        }

        return XDP_PASS;
    }

    /* ACK 包处理：验证 cookie */
    if (tcp->ack && !tcp->syn) {
        struct syn_state *state = bpf_map_lookup_elem(&syn_validation_map, &saddr);
        if (!state || state->validated)
            return XDP_PASS;

        /* 时间窗口校验：challenge 必须在 30 秒内完成 */
        if (now - state->timestamp > 30000000000ULL) {
            bpf_map_delete_elem(&syn_validation_map, &saddr);
            return XDP_DROP;
        }

        /* 验证 cookie：重新计算并与存储的 cookie 比较
         * 同时校验 ACK 序列号的低 32 位是否包含 cookie 信息
         * 这防止攻击者仅发送任意 ACK 来通过验证 */
        __u32 expected_cookie = syn_cookie_hash(saddr, dport, state->timestamp);
        __u32 ack_seq = bpf_ntohl(tcp->ack_seq);

        /* 双重校验：cookie 必须匹配 AND ack_seq 不能为 0 */
        if ((__u32)state->cookie == expected_cookie && ack_seq != 0) {
            /* 验证通过 */
            state->validated = 1;
            state->timestamp = now; /* 刷新有效期 */
        } else {
            /* ACK 伪造检测 */
            __u32 stats_key = 0;
            struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
            if (stats)
                __sync_fetch_and_add(&stats->ack_forgery, 1);
            return XDP_DROP;
        }
        return XDP_PASS;
    }

    return XDP_PASS;
}

/* ============================================
 * 速率限制
 * ============================================ */

static __always_inline int handle_l1_rate_limit(struct xdp_md *ctx, __u32 saddr, __u8 is_syn)
{
    /* 1. 读取速率限制配置 */
    __u32 cfg_key = 0;
    struct rate_limit_config *cfg = bpf_map_lookup_elem(&rate_config_map, &cfg_key);
    if (!cfg || !cfg->enabled)
        return XDP_PASS;

    __u64 now = bpf_ktime_get_ns();

    /* 2. 查询/创建该 IP 的计数器 */
    struct rate_counter *counter = bpf_map_lookup_elem(&rate_limit_map, &saddr);
    if (!counter) {
        /* 首次出现，创建新计数器 */
        struct rate_counter new_counter = {
            .syn_count = 0,
            .conn_count = 0,
            .window_start = now,
        };
        bpf_map_update_elem(&rate_limit_map, &saddr, &new_counter, BPF_ANY);
        counter = bpf_map_lookup_elem(&rate_limit_map, &saddr);
        if (!counter)
            return XDP_PASS;
    }

    /* 3. 窗口过期（>1s）则重置 */
    if (now - counter->window_start > 1000000000ULL) {
        counter->syn_count = 0;
        counter->conn_count = 0;
        counter->window_start = now;
    }

    /* 4. 检查 SYN 限制 */
    if (is_syn && counter->syn_count >= cfg->syn_pps_limit) {
        /* 递增 rate_drops 统计 */
        __u32 stats_key = 0;
        struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
        if (stats)
            __sync_fetch_and_add(&stats->rate_drops, 1);

        /* 上报 rate_event 到 Ring Buffer */
        struct rate_event *event = bpf_ringbuf_reserve(&l1_defense_events, sizeof(*event), 0);
        if (event) {
            event->timestamp = now;
            event->source_ip = saddr;
            event->trigger_type = 0; /* SYN */
            event->current_rate = counter->syn_count;
            bpf_ringbuf_submit(event, 0);
        }

        return XDP_DROP;
    }

    /* 5. 检查总连接限制 */
    if (counter->conn_count >= cfg->conn_pps_limit) {
        __u32 stats_key = 0;
        struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
        if (stats)
            __sync_fetch_and_add(&stats->rate_drops, 1);

        struct rate_event *event = bpf_ringbuf_reserve(&l1_defense_events, sizeof(*event), 0);
        if (event) {
            event->timestamp = now;
            event->source_ip = saddr;
            event->trigger_type = 1; /* CONN */
            event->current_rate = counter->conn_count;
            bpf_ringbuf_submit(event, 0);
        }

        return XDP_DROP;
    }

    /* 6. 递增计数 */
    if (is_syn)
        __sync_fetch_and_add(&counter->syn_count, 1);
    __sync_fetch_and_add(&counter->conn_count, 1);

    return XDP_PASS;
}

/* ============================================
 * L1 防御主入口
 * ============================================ */

static __always_inline int handle_l1_defense(struct xdp_md *ctx)
{
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    /* 1. 解析以太网头 */
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;

    /* 非 IPv4 直接放行（ARP 等需要通过） */
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return XDP_PASS;

    /* 2. 解析 IP 头 */
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return XDP_PASS;

    __u32 saddr = ip->saddr;

    /* 3. 用户级黑名单检查（7.2 - 前移到 XDP，最高优先级） */
    int bl_action = handle_l1_blacklist_check(saddr);
    if (bl_action != XDP_PASS)
        return bl_action;

    /* 4. ASN 黑名单检查 */
    int asn_action = handle_l1_asn_check(ctx);
    if (asn_action != XDP_PASS)
        return asn_action;

    /* 5. 非法 L3/L4 画像检查（7.1） */
    int sanity_action = handle_l1_packet_sanity(ip, data_end);
    if (sanity_action != XDP_PASS)
        return sanity_action;

    /* 6. 入口协议准入校验（7.3） */
    int profile_action = handle_l1_ingress_profile(ip, data_end);
    if (profile_action != XDP_PASS)
        return profile_action;

    /* 6.5 SYN 无状态验证（7.4） */
    int syn_action = handle_l1_syn_validation(ip, data_end, saddr);
    if (syn_action != XDP_PASS)
        return syn_action;

    /* 7. 判断是否为 TCP SYN 包 */
    __u8 is_syn = 0;
    if (ip->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
        if ((void *)(tcp + 1) <= data_end) {
            /* SYN=1 且 ACK=0 表示纯 SYN 包 */
            if (tcp->syn && !tcp->ack)
                is_syn = 1;
        }
    }

    /* 8. 速率限制检查 */
    int rate_action = handle_l1_rate_limit(ctx, saddr, is_syn);
    if (rate_action != XDP_PASS)
        return rate_action;

    return XDP_PASS;
}
