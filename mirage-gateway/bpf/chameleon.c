/* Chameleon - 协议变色龙 eBPF 数据面
 * 功能：TLS/QUIC 指纹重写
 * 挂载点：TC egress
 */

#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#include "common.h"

/* TLS 记录类型 */
#define TLS_HANDSHAKE 0x16
#define TLS_CHANGE_CIPHER_SPEC 0x14
#define TLS_ALERT 0x15
#define TLS_APPLICATION_DATA 0x17

/* Handshake 类型 */
#define TLS_CLIENT_HELLO 0x01
#define TLS_SERVER_HELLO 0x02

/* Chameleon 配置 Map */
struct chameleon_config {
    __u32 enabled;           // 是否启用
    __u32 profile_id;        // 配置文件 ID
    __u16 tls_version;       // TLS 版本
    __u16 cipher_count;      // 密码套件数量
    __u16 ciphers[16];       // 密码套件列表
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct chameleon_config);
} chameleon_config_map SEC(".maps");

/* TLS ClientHello 检测 */
static __always_inline int is_tls_client_hello(void *data, void *data_end) {
    // 检查是否有足够的数据
    if (data + 6 > data_end)
        return 0;
    
    __u8 *ptr = data;
    
    // 检查记录类型
    if (ptr[0] != TLS_HANDSHAKE)
        return 0;
    
    // 检查 TLS 版本 (0x0301 = TLS 1.0, 0x0303 = TLS 1.2)
    if (ptr[1] != 0x03)
        return 0;
    
    // 检查 Handshake 类型
    if (ptr[5] != TLS_CLIENT_HELLO)
        return 0;
    
    return 1;
}

/* QUIC 包检测 */
static __always_inline int is_quic_packet(void *data, void *data_end) {
    if (data + 1 > data_end)
        return 0;
    
    __u8 *ptr = data;
    
    // QUIC 长包头：第一个字节的最高位为 1
    if ((*ptr & 0x80) == 0x80)
        return 1;
    
    return 0;
}

/* TLS 指纹重写 */
SEC("tc")
int chameleon_tls_rewrite(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    // 1. 检查配置
    __u32 key = 0;
    struct chameleon_config *cfg = bpf_map_lookup_elem(&chameleon_config_map, &key);
    if (!cfg || !cfg->enabled)
        return TC_ACT_OK;
    
    // 2. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    // 3. 解析 IP 头
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    // 4. 只处理 TCP
    if (ip->protocol != IPPROTO_TCP)
        return TC_ACT_OK;
    
    struct tcphdr *tcp = (void *)ip + sizeof(*ip);
    if ((void *)(tcp + 1) > data_end)
        return TC_ACT_OK;
    
    // 5. 检查是否为 TLS ClientHello
    void *payload = (void *)tcp + (tcp->doff * 4);
    if (!is_tls_client_hello(payload, data_end))
        return TC_ACT_OK;
    
    // 6. TLS 指纹重写
    // 注意：eBPF 无法直接修改包内容（需要 bpf_skb_store_bytes）
    // 这里只做标记，实际重写在用户态完成
    
    // 记录事件
    report_threat(
        THREAT_NONE,
        ip->saddr,
        bpf_ntohs(tcp->source),
        0
    );
    
    return TC_ACT_OK;
}

/* QUIC 指纹重写 */
SEC("tc")
int chameleon_quic_rewrite(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    // 1. 检查配置
    __u32 key = 0;
    struct chameleon_config *cfg = bpf_map_lookup_elem(&chameleon_config_map, &key);
    if (!cfg || !cfg->enabled)
        return TC_ACT_OK;
    
    // 2. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    // 3. 解析 IP 头
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    // 4. 只处理 UDP
    if (ip->protocol != IPPROTO_UDP)
        return TC_ACT_OK;
    
    struct udphdr *udp = (void *)ip + sizeof(*ip);
    if ((void *)(udp + 1) > data_end)
        return TC_ACT_OK;
    
    // 5. 检查是否为 QUIC
    void *payload = (void *)udp + sizeof(*udp);
    if (!is_quic_packet(payload, data_end))
        return TC_ACT_OK;
    
    // 6. QUIC 指纹重写
    // 实际重写在用户态完成
    
    return TC_ACT_OK;
}

/* VPC 内容注入 */
SEC("tc")
int vpc_content_injection(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    // 1. 检查 VPC 配置
    __u32 key = 0;
    struct vpc_config *cfg = bpf_map_lookup_elem(&vpc_config_map, &key);
    if (!cfg || !cfg->enabled)
        return TC_ACT_OK;
    
    // 2. 检查蜂窝阶段
    __u32 *phase = bpf_map_lookup_elem(&cell_phase_map, &key);
    if (!phase || *phase != 0) // 只在潜伏期注入
        return TC_ACT_OK;
    
    // 3. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    // 4. 解析 IP 头
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    // 5. 只处理 UDP（VPC 噪声）
    if (ip->protocol != IPPROTO_UDP)
        return TC_ACT_OK;
    
    // 6. 标记为 VPC 噪声包
    // 用户态会注入合法内容片段
    
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
