/* B-DNA - 行为识别协议 (Behavioral DNA)
 * 核心功能：JA4 指纹拟态 + TCP/QUIC 栈重写
 * 挂载点：TC egress
 * 
 * 让流量的"基因"与目标浏览器完全一致
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

/* ============================================
 * B-DNA 指纹模板定义
 * ============================================ */

// 协议栈指纹模板
struct stack_fingerprint {
    // TCP 参数
    __u16 tcp_window;           // 初始窗口大小
    __u8  tcp_wscale;           // 窗口缩放因子
    __u16 tcp_mss;              // 最大段大小
    __u8  tcp_sack_ok;          // SACK 允许
    __u8  tcp_timestamps;       // 时间戳选项
    
    // QUIC 参数
    __u32 quic_max_idle;        // max_idle_timeout
    __u32 quic_max_data;        // initial_max_data
    __u32 quic_max_streams_bi;  // initial_max_streams_bidi
    __u32 quic_max_streams_uni; // initial_max_streams_uni
    __u16 quic_ack_delay_exp;   // ack_delay_exponent
    
    // TLS 参数
    __u16 tls_version;          // TLS 版本
    __u8  tls_ext_order[32];    // Extension 顺序
    __u8  tls_ext_count;        // Extension 数量
    
    // 元数据
    __u32 profile_id;           // 配置文件 ID
    char  profile_name[32];     // 配置文件名称
};

// 预定义指纹模板 ID
#define PROFILE_CHROME_WIN11    0
#define PROFILE_CHROME_MACOS    1
#define PROFILE_FIREFOX_WIN11   2
#define PROFILE_FIREFOX_LINUX   3
#define PROFILE_SAFARI_MACOS    4
#define PROFILE_EDGE_WIN11      5

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 16);
    __type(key, __u32);
    __type(value, struct stack_fingerprint);
} fingerprint_map SEC(".maps");

// 当前激活的指纹 ID
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);
} active_profile_map SEC(".maps");

// B-DNA 统计
struct bdna_stats {
    __u64 tcp_rewritten;
    __u64 quic_rewritten;
    __u64 tls_rewritten;
    __u64 skipped;
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct bdna_stats);
} bdna_stats_map SEC(".maps");

/* ============================================
 * TCP 选项常量
 * ============================================ */

#define TCPOPT_EOL      0
#define TCPOPT_NOP      1
#define TCPOPT_MSS      2
#define TCPOPT_WSCALE   3
#define TCPOPT_SACK_OK  4
#define TCPOPT_SACK     5
#define TCPOPT_TIMESTAMP 8

#define TCPOLEN_MSS     4
#define TCPOLEN_WSCALE  3
#define TCPOLEN_SACK_OK 2
#define TCPOLEN_TIMESTAMP 10

/* ============================================
 * TCP 指纹重写
 * ============================================ */

SEC("tc")
int bdna_tcp_rewrite(struct __sk_buff *skb)
{
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    // 1. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    // 2. 解析 IP 头（固定偏移，SYN 包无 IP Options）
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    if (ip->protocol != IPPROTO_TCP)
        return TC_ACT_OK;
    
    if (ip->ihl != 5)
        return TC_ACT_OK;
    
    // 3. 解析 TCP 头（固定 IP 偏移）
    struct tcphdr *tcp = (void *)(ip + 1);
    if ((void *)(tcp + 1) > data_end)
        return TC_ACT_OK;
    
    // 4. 只处理 SYN 包（握手阶段）
    if (!tcp->syn)
        return TC_ACT_OK;
    
    // 5. 获取当前激活的指纹
    __u32 key = 0;
    __u32 *profile_id = bpf_map_lookup_elem(&active_profile_map, &key);
    if (!profile_id)
        return TC_ACT_OK;
    
    struct stack_fingerprint *fp = bpf_map_lookup_elem(&fingerprint_map, profile_id);
    if (!fp) {
        struct bdna_stats *stats = bpf_map_lookup_elem(&bdna_stats_map, &key);
        if (stats)
            __sync_fetch_and_add(&stats->skipped, 1);
        return TC_ACT_OK;
    }
    
    // 6. 重写 TCP Window Size
    __u16 old_window = tcp->window;
    tcp->window = bpf_htons(fp->tcp_window);
    
    // 7. 重写 TCP 选项（switch 分支编译期常量方案）
    //    每个 bpf_skb_load_bytes 的 size 参数必须是编译期常量
    __u32 doff = tcp->doff;
    __u8 opts[40] = {};
    __u32 opt_offset = ETH_HLEN + sizeof(struct iphdr) + sizeof(struct tcphdr);
    int loaded = -1;
    __u32 opt_len = 0;
    
    switch (doff) {
    case 6:  loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 4);  opt_len = 4;  break;
    case 7:  loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 8);  opt_len = 8;  break;
    case 8:  loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 12); opt_len = 12; break;
    case 9:  loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 16); opt_len = 16; break;
    case 10: loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 20); opt_len = 20; break;
    case 11: loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 24); opt_len = 24; break;
    case 12: loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 28); opt_len = 28; break;
    case 13: loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 32); opt_len = 32; break;
    case 14: loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 36); opt_len = 36; break;
    case 15: loaded = bpf_skb_load_bytes(skb, opt_offset, opts, 40); opt_len = 40; break;
    default: goto skip_options;
    }
    
    if (loaded < 0)
        goto skip_options;
    
    // 在栈上遍历并修改 TCP Options
    {
        __u32 pos = 0;
        #pragma unroll
        for (int i = 0; i < 10; i++) {
            if (pos >= opt_len)
                break;
            
            __u8 kind = opts[pos];
            
            if (kind == TCPOPT_EOL)
                break;
            
            if (kind == TCPOPT_NOP) {
                pos++;
                continue;
            }
            
            if (pos + 1 >= opt_len)
                break;
            
            __u8 len = opts[pos + 1];
            if (len < 2 || pos + len > opt_len)
                break;
            
            switch (kind) {
            case TCPOPT_MSS:
                if (len == TCPOLEN_MSS && pos + 3 < opt_len) {
                    __u16 mss = bpf_htons(fp->tcp_mss);
                    opts[pos + 2] = (__u8)(mss >> 8);
                    opts[pos + 3] = (__u8)(mss & 0xFF);
                }
                break;
                
            case TCPOPT_WSCALE:
                if (len == TCPOLEN_WSCALE && pos + 2 < opt_len) {
                    opts[pos + 2] = fp->tcp_wscale;
                }
                break;
            }
            
            pos += len;
        }
    }
    
    // 写回修改后的 Options（同样使用 switch 分支常量 size）
    switch (doff) {
    case 6:  bpf_skb_store_bytes(skb, opt_offset, opts, 4, 0);  break;
    case 7:  bpf_skb_store_bytes(skb, opt_offset, opts, 8, 0);  break;
    case 8:  bpf_skb_store_bytes(skb, opt_offset, opts, 12, 0); break;
    case 9:  bpf_skb_store_bytes(skb, opt_offset, opts, 16, 0); break;
    case 10: bpf_skb_store_bytes(skb, opt_offset, opts, 20, 0); break;
    case 11: bpf_skb_store_bytes(skb, opt_offset, opts, 24, 0); break;
    case 12: bpf_skb_store_bytes(skb, opt_offset, opts, 28, 0); break;
    case 13: bpf_skb_store_bytes(skb, opt_offset, opts, 32, 0); break;
    case 14: bpf_skb_store_bytes(skb, opt_offset, opts, 36, 0); break;
    case 15: bpf_skb_store_bytes(skb, opt_offset, opts, 40, 0); break;
    }
    
skip_options:
    // 8. 重新计算 TCP 校验和（增量更新）
    ;
    __u32 csum = (~tcp->check) & 0xFFFF;
    csum += (~old_window) & 0xFFFF;
    csum += bpf_htons(fp->tcp_window);
    while (csum >> 16)
        csum = (csum & 0xFFFF) + (csum >> 16);
    tcp->check = ~csum;
    
    // 9. 更新统计
    struct bdna_stats *stats = bpf_map_lookup_elem(&bdna_stats_map, &key);
    if (stats)
        __sync_fetch_and_add(&stats->tcp_rewritten, 1);
    
    return TC_ACT_OK;
}

/* ============================================
 * QUIC 指纹重写
 * ============================================ */

// QUIC 包类型
#define QUIC_LONG_HEADER    0x80
#define QUIC_INITIAL        0x00
#define QUIC_0RTT           0x01
#define QUIC_HANDSHAKE      0x02
#define QUIC_RETRY          0x03

// QUIC Transport Parameter IDs
#define TP_MAX_IDLE_TIMEOUT         0x01
#define TP_MAX_UDP_PAYLOAD_SIZE     0x03
#define TP_INITIAL_MAX_DATA         0x04
#define TP_INITIAL_MAX_STREAM_DATA_BIDI_LOCAL   0x05
#define TP_INITIAL_MAX_STREAM_DATA_BIDI_REMOTE  0x06
#define TP_INITIAL_MAX_STREAM_DATA_UNI          0x07
#define TP_INITIAL_MAX_STREAMS_BIDI 0x08
#define TP_INITIAL_MAX_STREAMS_UNI  0x09
#define TP_ACK_DELAY_EXPONENT       0x0a

// 检测 QUIC Initial 包
static __always_inline int is_quic_initial(void *data, void *data_end)
{
    __u8 *ptr = data;
    if ((void *)(ptr + 5) > data_end)
        return 0;
    
    // 检查长包头标志
    if (!(ptr[0] & QUIC_LONG_HEADER))
        return 0;
    
    // 检查包类型（Initial = 0x00）
    __u8 type = (ptr[0] & 0x30) >> 4;
    if (type != QUIC_INITIAL)
        return 0;
    
    // 检查版本（QUIC v1 = 0x00000001）
    __u32 version = *(__u32 *)(ptr + 1);
    if (version != bpf_htonl(0x00000001) && version != bpf_htonl(0xff000000))
        return 0;
    
    return 1;
}

SEC("tc")
int bdna_quic_rewrite(struct __sk_buff *skb)
{
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    // 1. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    // 2. 解析 IP 头
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    if (ip->protocol != IPPROTO_UDP)
        return TC_ACT_OK;
    
    // 3. 解析 UDP 头
    if (ip->ihl != 5)
        return TC_ACT_OK;
    
    struct udphdr *udp = (void *)(ip + 1);
    if ((void *)(udp + 1) > data_end)
        return TC_ACT_OK;
    
    // 4. 检查端口（QUIC 通常使用 443）
    if (bpf_ntohs(udp->dest) != 443)
        return TC_ACT_OK;
    
    // 5. 获取 QUIC 载荷
    void *quic_data = (void *)udp + sizeof(struct udphdr);
    if (!is_quic_initial(quic_data, data_end))
        return TC_ACT_OK;
    
    // 6. 获取指纹配置
    __u32 key = 0;
    __u32 *profile_id = bpf_map_lookup_elem(&active_profile_map, &key);
    if (!profile_id)
        return TC_ACT_OK;
    
    struct stack_fingerprint *fp = bpf_map_lookup_elem(&fingerprint_map, profile_id);
    if (!fp)
        return TC_ACT_OK;
    
    // 7. QUIC Transport Parameters 重写
    // 注意：QUIC Initial 包是加密的，需要在加密前修改
    // 这里标记包，由用户态完成实际重写
    skb->mark |= 0x51554943;  // "QUIC" magic
    
    // 8. 更新统计
    struct bdna_stats *stats = bpf_map_lookup_elem(&bdna_stats_map, &key);
    if (stats)
        __sync_fetch_and_add(&stats->quic_rewritten, 1);
    
    return TC_ACT_OK;
}

/* ============================================
 * TLS ClientHello 指纹重写
 * ============================================ */

// TLS 记录类型
#define TLS_HANDSHAKE       0x16
#define TLS_CLIENT_HELLO    0x01

// TLS Extension IDs
#define TLS_EXT_SNI                 0x0000
#define TLS_EXT_SUPPORTED_GROUPS    0x000a
#define TLS_EXT_EC_POINT_FORMATS    0x000b
#define TLS_EXT_SIGNATURE_ALGOS     0x000d
#define TLS_EXT_ALPN                0x0010
#define TLS_EXT_SUPPORTED_VERSIONS  0x002b
#define TLS_EXT_PSK_KEY_EXCHANGE    0x002d
#define TLS_EXT_KEY_SHARE           0x0033

// 检测 TLS ClientHello
static __always_inline int is_tls_client_hello(void *data, void *data_end)
{
    __u8 *ptr = data;
    if ((void *)(ptr + 6) > data_end)
        return 0;
    
    // 检查记录类型
    if (ptr[0] != TLS_HANDSHAKE)
        return 0;
    
    // 检查 TLS 版本
    if (ptr[1] != 0x03)
        return 0;
    
    // 检查 Handshake 类型
    if (ptr[5] != TLS_CLIENT_HELLO)
        return 0;
    
    return 1;
}

SEC("tc")
int bdna_tls_rewrite(struct __sk_buff *skb)
{
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    // 1. 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;
    
    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;
    
    // 2. 解析 IP 头（固定偏移）
    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;
    
    if (ip->protocol != IPPROTO_TCP)
        return TC_ACT_OK;
    
    if (ip->ihl != 5)
        return TC_ACT_OK;
    
    // 3. 解析 TCP 头（固定 IP 偏移）
    struct tcphdr *tcp = (void *)(ip + 1);
    if ((void *)(tcp + 1) > data_end)
        return TC_ACT_OK;
    
    // 4. 位掩码黄金法则：跳过 TCP Options 到达 payload
    __u32 doff_len = tcp->doff * 4;
    if (doff_len < sizeof(struct tcphdr) || doff_len > 60)
        return TC_ACT_OK;
    doff_len &= 0x3C;  // 强制定界：最大 60，且 4 字节对齐
    
    void *payload = (void *)((__u8 *)tcp) + doff_len;
    if ((void *)(payload + 6) > data_end)
        return TC_ACT_OK;
    
    if (!is_tls_client_hello(payload, data_end))
        return TC_ACT_OK;
    
    // 5. 获取指纹配置
    __u32 key = 0;
    __u32 *profile_id = bpf_map_lookup_elem(&active_profile_map, &key);
    if (!profile_id)
        return TC_ACT_OK;
    
    struct stack_fingerprint *fp = bpf_map_lookup_elem(&fingerprint_map, profile_id);
    if (!fp)
        return TC_ACT_OK;
    
    // 6. TLS Extension 顺序重排
    // 由于 eBPF 限制，复杂的 TLS 重写需要用户态配合
    // 这里标记包，由用户态完成实际重写
    skb->mark |= 0x544C5348;  // "TLSH" magic
    
    // 7. 更新统计
    struct bdna_stats *stats = bpf_map_lookup_elem(&bdna_stats_map, &key);
    if (stats)
        __sync_fetch_and_add(&stats->tls_rewritten, 1);
    
    return TC_ACT_OK;
}

/* ============================================
 * JA4 指纹计算与验证
 * ============================================ */

// JA4 指纹结构
struct ja4_fingerprint {
    char protocol;          // 't' for TCP, 'q' for QUIC
    char version[2];        // TLS version
    char sni;               // 'd' for domain, 'i' for IP
    char cipher_count[2];   // Number of ciphers
    char ext_count[2];      // Number of extensions
    char alpn[2];           // First ALPN
    char hash[12];          // Truncated SHA256
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, __u32);     // 源 IP
    __type(value, struct ja4_fingerprint);
} ja4_cache SEC(".maps");

// JA4 指纹事件
struct ja4_event {
    __u64 timestamp;
    __u32 source_ip;
    struct ja4_fingerprint fingerprint;
    __u32 matched_profile;
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 64 * 1024);
} ja4_events SEC(".maps");

SEC("tc")
int bdna_ja4_capture(struct __sk_buff *skb)
{
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
    
    if (ip->protocol != IPPROTO_TCP)
        return TC_ACT_OK;
    
    if (ip->ihl != 5)
        return TC_ACT_OK;
    
    struct tcphdr *tcp = (void *)(ip + 1);
    if ((void *)(tcp + 1) > data_end)
        return TC_ACT_OK;
    
    // 位掩码黄金法则
    __u32 doff_len = tcp->doff * 4;
    if (doff_len < sizeof(struct tcphdr) || doff_len > 60)
        return TC_ACT_OK;
    doff_len &= 0x3C;
    
    void *payload = (void *)((__u8 *)tcp) + doff_len;
    if ((void *)(payload + 6) > data_end)
        return TC_ACT_OK;
    
    if (!is_tls_client_hello(payload, data_end))
        return TC_ACT_OK;
    
    // 捕获 JA4 指纹（简化版）
    struct ja4_fingerprint fp = {
        .protocol = 't',
        .version = {'1', '3'},
        .sni = 'd',
    };
    
    // 缓存指纹
    __u32 src_ip = ip->saddr;
    bpf_map_update_elem(&ja4_cache, &src_ip, &fp, BPF_ANY);
    
    // 上报事件
    struct ja4_event *event = bpf_ringbuf_reserve(&ja4_events, sizeof(*event), 0);
    if (event) {
        event->timestamp = bpf_ktime_get_ns();
        event->source_ip = src_ip;
        event->fingerprint = fp;
        event->matched_profile = 0;
        bpf_ringbuf_submit(event, 0);
    }
    
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
