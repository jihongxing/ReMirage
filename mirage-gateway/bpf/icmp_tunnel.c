/* ICMP Tunnel eBPF 数据面
 * 伪装形态：网络连通性诊断（Ping）
 * 
 * TC egress: 从发送队列读取数据，构造 ICMP Echo Request
 * TC ingress: 截获匹配的 ICMP Echo Reply，提取 Payload 上报
 * 
 * Go → C: icmp_config_map (配置), icmp_tx_map (发送队列)
 * C → Go: icmp_data_events (Ring Buffer)
 */

#include "common.h"

#define ICMP_ECHO_REQUEST 8
#define ICMP_ECHO_REPLY   0
#define ICMP_MAX_PAYLOAD  1024

struct mirage_icmphdr {
    __u8 type;
    __u8 code;
    __sum16 checksum;
    union {
        struct {
            __be16 id;
            __be16 sequence;
        } echo;
        __u32 gateway;
    } un;
};

/* ICMP Tunnel 配置（Go → C） */
struct icmp_config {
    __u32 enabled;          /* 是否启用 */
    __u32 target_ip;        /* 目标 IP（网络字节序） */
    __u32 gateway_ip;       /* 网关 IP（网络字节序） */
    __u16 identifier;       /* ICMP Identifier（会话标识） */
    __u16 reserved;
};

/* ICMP 发送队列条目（Go → C） */
struct icmp_tx_entry {
    __u32 seq;              /* 序列号 */
    __u16 data_len;         /* 数据长度 */
    __u16 reserved;
    __u8  data[ICMP_MAX_PAYLOAD]; /* 加密后的 Payload */
};

/* ICMP 接收事件（C → Go） */
struct icmp_rx_event {
    __u64 timestamp;
    __u32 src_ip;
    __u16 identifier;
    __u16 seq;
    __u16 data_len;
    __u16 reserved;
    __u8  data[ICMP_MAX_PAYLOAD]; /* 提取的 Payload */
};

/* eBPF Maps */

/* 配置 Map（Go → C，HASH 类型） */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct icmp_config);
} icmp_config_map SEC(".maps");

/* 发送队列 Map（Go → C，QUEUE 类型） */
struct {
    __uint(type, BPF_MAP_TYPE_QUEUE);
    __uint(max_entries, 64);
    __type(value, struct icmp_tx_entry);
} icmp_tx_map SEC(".maps");

/* Per-CPU scratch buffer for queue pop results.
 * icmp_tx_entry is larger than the BPF stack limit, so egress must not place it
 * on the stack even when the current implementation only consumes the queue.
 */
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct icmp_tx_entry);
} icmp_tx_scratch_map SEC(".maps");

/* 接收事件 Ring Buffer（C → Go） */
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024); /* 256KB */
} icmp_data_events SEC(".maps");

/* 读取配置 */
static __always_inline struct icmp_config *get_icmp_config(void) {
    __u32 key = 0;
    return bpf_map_lookup_elem(&icmp_config_map, &key);
}

/* TC egress: 截获出站 ICMP Echo Request 并注入 Payload
 * 实际生产中，Go 控制面构造原始 ICMP 包通过 raw socket 发送，
 * 此 TC hook 负责在内核态对匹配的 ICMP 包进行 Payload 替换/增强
 */
SEC("tc")
int icmp_tunnel_egress(struct __sk_buff *skb) {
    struct icmp_config *cfg = get_icmp_config();
    if (!cfg || !cfg->enabled)
        return TC_ACT_OK;

    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    /* 解析以太网头 */
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;

    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;

    /* 解析 IP 头 */
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;

    /* 仅处理 ICMP 协议 */
    if (ip->protocol != IPPROTO_ICMP)
        return TC_ACT_OK;

    /* 检查目标 IP */
    if (ip->daddr != cfg->target_ip)
        return TC_ACT_OK;

    /* 解析 ICMP 头 */
    struct mirage_icmphdr *icmp = (void *)(ip + 1);
    if ((void *)(icmp + 1) > data_end)
        return TC_ACT_OK;

    /* 仅处理 Echo Request */
    if (icmp->type != ICMP_ECHO_REQUEST)
        return TC_ACT_OK;

    /* 检查 Identifier */
    if (bpf_ntohs(icmp->un.echo.id) != cfg->identifier)
        return TC_ACT_OK;

    /* 从发送队列读取数据 */
    __u32 scratch_key = 0;
    struct icmp_tx_entry *tx = bpf_map_lookup_elem(&icmp_tx_scratch_map, &scratch_key);
    if (!tx)
        return TC_ACT_OK;

    if (bpf_map_pop_elem(&icmp_tx_map, tx) != 0)
        return TC_ACT_OK; /* 队列为空，放行原始包 */

    /* 注入 Payload 数据到 ICMP 包
     * 注意：实际实现需要 bpf_skb_change_tail 调整包大小
     * 这里提供框架逻辑 */
    
    return TC_ACT_OK;
}

/* TC ingress: 截获入站 ICMP Echo Reply，提取 Payload 上报 */
SEC("tc")
int icmp_tunnel_ingress(struct __sk_buff *skb) {
    struct icmp_config *cfg = get_icmp_config();
    if (!cfg || !cfg->enabled)
        return TC_ACT_OK;

    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    /* 解析以太网头 */
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;

    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;

    /* 解析 IP 头 */
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;

    /* 仅处理 ICMP 协议 */
    if (ip->protocol != IPPROTO_ICMP)
        return TC_ACT_OK;

    /* 检查源 IP（应来自目标网关） */
    if (ip->saddr != cfg->target_ip)
        return TC_ACT_OK;

    /* 解析 ICMP 头 */
    struct mirage_icmphdr *icmp = (void *)(ip + 1);
    if ((void *)(icmp + 1) > data_end)
        return TC_ACT_OK;

    /* 仅处理 Echo Reply */
    if (icmp->type != ICMP_ECHO_REPLY)
        return TC_ACT_OK;

    /* 检查 Identifier */
    if (bpf_ntohs(icmp->un.echo.id) != cfg->identifier)
        return TC_ACT_OK;

    /* 计算 Payload 偏移和长度 */
    void *payload = (void *)(icmp + 1);
    if (payload >= data_end)
        return TC_ACT_OK;

    __u16 payload_len = data_end - payload;
    if (payload_len > ICMP_MAX_PAYLOAD)
        payload_len = ICMP_MAX_PAYLOAD;
    if (payload_len == 0)
        return TC_ACT_OK;

    /* 通过 Ring Buffer 上报 */
    struct icmp_rx_event *event;
    event = bpf_ringbuf_reserve(&icmp_data_events, sizeof(*event), 0);
    if (!event)
        return TC_ACT_OK; /* Ring Buffer 满，放行 */

    event->timestamp = bpf_ktime_get_ns();
    event->src_ip = ip->saddr;
    event->identifier = bpf_ntohs(icmp->un.echo.id);
    event->seq = bpf_ntohs(icmp->un.echo.sequence);
    event->data_len = payload_len;
    event->reserved = 0;

    /* 复制 Payload 数据 */
    if (payload_len > 0 && payload + payload_len <= data_end) {
        bpf_probe_read_kernel(event->data, payload_len & 0x3FF, payload);
    }

    bpf_ringbuf_submit(event, 0);

    return TC_ACT_OK; /* 放行原始包 */
}

char _license[] SEC("license") = "GPL";
