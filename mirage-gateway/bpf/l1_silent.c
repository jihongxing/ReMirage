/* L1 静默响应 - TC egress 层
 * 功能：拦截出站 ICMP Unreachable 和 TCP RST，实现 Default-Drop 静默
 * 挂载点：TC egress（通过 netlink 挂载到目标网卡）
 *
 * 原理：攻击者扫描端口时，操作系统默认会回复 ICMP Unreachable 或 TCP RST，
 * 这些响应会暴露系统存在。本程序在出站方向拦截这些响应，让扫描器陷入超时黑洞。
 */

#include "common.h"

/* 内联 ICMP 头定义，避免 <linux/icmp.h> 拉入 <sys/socket.h> 导致 BPF 编译失败 */
struct icmphdr {
    __u8  type;
    __u8  code;
    __sum16 checksum;
    union {
        struct { __be16 id; __be16 sequence; } echo;
        __be32 gateway;
    } un;
};

SEC("tc")
int l1_silent_egress(struct __sk_buff *skb)
{
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    /* 1. 解析以太网头 */
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;

    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;

    /* 2. 解析 IP 头 */
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;

    /* 3. 读取静默响应配置 */
    __u32 cfg_key = 0;
    struct silent_config *cfg = bpf_map_lookup_elem(&silent_config_map, &cfg_key);
    if (!cfg || !cfg->enabled)
        return TC_ACT_OK;

    /* 4. 检查 ICMP Destination Unreachable (Type=3) */
    if (ip->protocol == IPPROTO_ICMP) {
        struct icmphdr *icmp = (void *)ip + (ip->ihl * 4);
        if ((void *)(icmp + 1) > data_end)
            return TC_ACT_OK;

        if (icmp->type == 3 && cfg->drop_icmp_unreachable) {
            /* 递增 silent_drops 统计 */
            __u32 stats_key = 0;
            struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
            if (stats)
                __sync_fetch_and_add(&stats->silent_drops, 1);

            return TC_ACT_SHOT;
        }
    }

    /* 5. 检查 TCP RST */
    if (ip->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = (void *)ip + (ip->ihl * 4);
        if ((void *)(tcp + 1) > data_end)
            return TC_ACT_OK;

        if (tcp->rst && cfg->drop_tcp_rst) {
            /* 递增 silent_drops 统计 */
            __u32 stats_key = 0;
            struct l1_stats *stats = bpf_map_lookup_elem(&l1_stats_map, &stats_key);
            if (stats)
                __sync_fetch_and_add(&stats->silent_drops, 1);

            return TC_ACT_SHOT;
        }
    }

    /* 6. 其余流量放行 */
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
