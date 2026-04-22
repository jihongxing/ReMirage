// SPDX-License-Identifier: GPL-2.0
// Mirage-Phantom: 影子欺骗数据面
// 将威胁流量无感重定向至蜜罐

#include "common.h"
#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

// 名单条目结构
struct phantom_entry {
    __u64 first_seen;
    __u64 last_seen;
    __u32 hit_count;
    __u8  risk_level;  // 0-4
    __u8  pad[3];
    __u32 ttl_seconds;
};

// 钓鱼名单：存储需要重定向的威胁 IP
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u32);   // src_ip
    __type(value, struct phantom_entry);
} phishing_list_map SEC(".maps");

// 蜜罐配置：分层目标重定向地址（按 risk_level 索引）
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 8);
    __type(key, __u32);
    __type(value, __u32); // honeypot_ip (network order)
} honeypot_config SEC(".maps");

// 重定向统计
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 4);
    __type(key, __u32);
    __type(value, __u64);
} phantom_stats SEC(".maps");

#define STAT_REDIRECTED   0
#define STAT_PASSED       1
#define STAT_TRAPPED      2
#define STAT_ERRORS       3

// 欺骗事件上报
struct phantom_event {
    __u64 timestamp;
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port;
    __u16 dst_port;
    __u32 honeypot_ip;
    __u8  event_type;  // 0=redirect, 1=trap_hit
    __u8  pad[3];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} phantom_events SEC(".maps");

// 更新 IP 校验和
static __always_inline void update_ip_csum(struct iphdr *ip, __u32 old_ip, __u32 new_ip) {
    __u32 csum = ~bpf_ntohs(ip->check);
    csum = csum - (old_ip >> 16) - (old_ip & 0xFFFF);
    csum = csum + (new_ip >> 16) + (new_ip & 0xFFFF);
    csum = (csum & 0xFFFF) + (csum >> 16);
    csum = (csum & 0xFFFF) + (csum >> 16);
    ip->check = bpf_htons(~csum);
}

SEC("tc")
int phantom_redirect(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    // 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    // 解析 IP 头
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    // 只处理 TCP
    if (ip->protocol != IPPROTO_TCP)
        return TC_ACT_OK;
    
    __u32 src_ip = ip->saddr;
    
    // 检查是否在钓鱼名单中
    struct phantom_entry *entry = bpf_map_lookup_elem(&phishing_list_map, &src_ip);
    if (!entry) {
        // 不在名单中，正常放行
        __u32 pass_key = STAT_PASSED;
        __u64 *passed = bpf_map_lookup_elem(&phantom_stats, &pass_key);
        if (passed) __sync_fetch_and_add(passed, 1);
        return TC_ACT_OK;
    }
    
    // 更新命中信息
    entry->last_seen = bpf_ktime_get_ns();
    __sync_fetch_and_add(&entry->hit_count, 1);
    
    // 获取蜜罐 IP（按 risk_level 索引）
    __u32 level_key = (__u32)entry->risk_level;
    __u32 *honeypot_ip = bpf_map_lookup_elem(&honeypot_config, &level_key);
    if (!honeypot_ip || *honeypot_ip == 0) {
        // 回退到 level=0 默认蜜罐
        __u32 default_key = 0;
        honeypot_ip = bpf_map_lookup_elem(&honeypot_config, &default_key);
        if (!honeypot_ip || *honeypot_ip == 0)
            return TC_ACT_OK;
    }
    
    // 保存原始目的 IP
    __u32 old_dst = ip->daddr;
    __u32 new_dst = *honeypot_ip;
    
    // 修改目的 IP 为蜜罐地址
    ip->daddr = new_dst;
    
    // 更新校验和
    update_ip_csum(ip, old_dst, new_dst);
    
    // 上报重定向事件
    struct phantom_event *event = bpf_ringbuf_reserve(&phantom_events, sizeof(*event), 0);
    if (event) {
        event->timestamp = bpf_ktime_get_ns();
        event->src_ip = src_ip;
        event->dst_ip = old_dst;
        event->honeypot_ip = new_dst;
        event->event_type = 0; // redirect
        
        // 解析端口（位掩码黄金法则）
        __u32 ip_hlen = ip->ihl * 4;
        if (ip_hlen < 20 || ip_hlen > 60)
            goto submit_event;
        ip_hlen &= 0x3C;
        
        struct tcphdr *tcp = (void *)((__u8 *)ip) + ip_hlen;
        if ((void *)(tcp + 1) <= data_end) {
            event->src_port = bpf_ntohs(tcp->source);
            event->dst_port = bpf_ntohs(tcp->dest);
        }
        
submit_event:
        bpf_ringbuf_submit(event, 0);
    }
    
    // 更新统计
    __u32 redir_key = STAT_REDIRECTED;
    __u64 *redirected = bpf_map_lookup_elem(&phantom_stats, &redir_key);
    if (redirected) __sync_fetch_and_add(redirected, 1);
    
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
