/* Sockmap - 零拷贝套接字重定向
 * 核心功能：内核态 Socket 间数据直传，绕过用户态
 * 挂载点：cgroup/sk_msg + sockops
 * 
 * 这是让流量在内核态"横跳"的关键
 */

#include <linux/bpf.h>
#include <linux/types.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

/* ============================================
 * Sockmap 核心 Map 定义
 * ============================================ */

// Sockmap：存储活跃 Socket FD
struct {
    __uint(type, BPF_MAP_TYPE_SOCKMAP);
    __uint(max_entries, 65535);
    __type(key, __u32);
    __type(value, __u64);
} sock_map SEC(".maps");

// Sockhash：基于四元组的快速查找
struct sock_key {
    __u32 sip;      // 源 IP
    __u32 dip;      // 目的 IP
    __u32 sport;    // 源端口
    __u32 dport;    // 目的端口
};

struct {
    __uint(type, BPF_MAP_TYPE_SOCKHASH);
    __uint(max_entries, 65535);
    __type(key, struct sock_key);
    __type(value, __u64);
} sock_hash SEC(".maps");

// 代理对映射：源 Socket → 目的 Socket 索引
struct proxy_pair {
    __u32 peer_idx;     // 对端 Socket 在 sock_map 中的索引
    __u32 flags;        // 标志位
    __u64 bytes_tx;     // 已传输字节数
    __u64 bytes_rx;     // 已接收字节数
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65535);
    __type(key, __u32);             // 本端 Socket 索引
    __type(value, struct proxy_pair);
} proxy_map SEC(".maps");

// 连接状态 Map
struct conn_state {
    __u32 state;        // 0=初始化, 1=已建立, 2=关闭中
    __u64 established;  // 建立时间戳
    __u64 last_active;  // 最后活跃时间
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65535);
    __type(key, __u32);
    __type(value, struct conn_state);
} conn_state_map SEC(".maps");

// 流量统计（零拷贝路径）
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 4);
    __type(key, __u32);
    __type(value, __u64);
} sockmap_stats SEC(".maps");

#define STAT_REDIRECT_OK    0
#define STAT_REDIRECT_FAIL  1
#define STAT_BYTES_TX       2
#define STAT_BYTES_RX       3

/* ============================================
 * sk_msg 程序：数据重定向核心
 * ============================================ */

SEC("sk_msg")
int sockmap_redirect(struct sk_msg_md *msg)
{
    // 1. 获取本端 Socket 索引（由 Go 层在注入时设置）
    __u32 idx = msg->local_port;  // 使用本地端口作为索引
    
    // 2. 查找代理对
    struct proxy_pair *pair = bpf_map_lookup_elem(&proxy_map, &idx);
    if (!pair) {
        // 未找到代理对，放行到用户态处理
        return SK_PASS;
    }
    
    // 3. 更新统计
    __u32 stat_key = STAT_BYTES_TX;
    __u64 *bytes = bpf_map_lookup_elem(&sockmap_stats, &stat_key);
    if (bytes) {
        __sync_fetch_and_add(bytes, msg->size);
    }
    
    // 4. 更新代理对统计
    __sync_fetch_and_add(&pair->bytes_tx, msg->size);
    
    // 5. 执行重定向：将数据直接推送到对端 Socket
    long ret = bpf_msg_redirect_map(msg, &sock_map, pair->peer_idx, BPF_F_INGRESS);
    
    if (ret == SK_PASS) {
        // 重定向成功
        stat_key = STAT_REDIRECT_OK;
        __u64 *ok_count = bpf_map_lookup_elem(&sockmap_stats, &stat_key);
        if (ok_count) {
            __sync_fetch_and_add(ok_count, 1);
        }
        return SK_PASS;
    }
    
    // 重定向失败，记录统计
    stat_key = STAT_REDIRECT_FAIL;
    __u64 *fail_count = bpf_map_lookup_elem(&sockmap_stats, &stat_key);
    if (fail_count) {
        __sync_fetch_and_add(fail_count, 1);
    }
    
    // 降级：放行到用户态
    return SK_PASS;
}

/* ============================================
 * sockops 程序：连接生命周期管理
 * ============================================ */

SEC("sockops")
int sockmap_sockops(struct bpf_sock_ops *skops)
{
    __u32 op = skops->op;
    
    switch (op) {
    case BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB:
    case BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB:
        // 连接建立：可选择性加入 Sockmap
        // 实际注入由 Go 层通过 bpf_map_update_elem 完成
        {
            // 记录连接状态
            __u32 key = skops->local_port;
            struct conn_state state = {
                .state = 1,
                .established = bpf_ktime_get_ns(),
                .last_active = bpf_ktime_get_ns(),
            };
            bpf_map_update_elem(&conn_state_map, &key, &state, BPF_ANY);
        }
        break;
        
    case BPF_SOCK_OPS_STATE_CB:
        // 连接状态变化
        if (skops->args[1] == BPF_TCP_CLOSE) {
            // 连接关闭：清理 Map
            __u32 key = skops->local_port;
            bpf_map_delete_elem(&conn_state_map, &key);
            bpf_map_delete_elem(&proxy_map, &key);
            // sock_map 由 Go 层清理
        }
        break;
    }
    
    return 0;
}

/* ============================================
 * Sockhash 版本：基于四元组重定向
 * ============================================ */

// 构建四元组 Key
static __always_inline void build_sock_key(struct sk_msg_md *msg, struct sock_key *key)
{
    key->sip = msg->remote_ip4;
    key->dip = msg->local_ip4;
    key->sport = bpf_ntohl(msg->remote_port) >> 16;
    key->dport = msg->local_port;
}

// 构建对端四元组 Key（交换源/目的）
static __always_inline void build_peer_key(struct sock_key *src, struct sock_key *dst)
{
    dst->sip = src->dip;
    dst->dip = src->sip;
    dst->sport = src->dport;
    dst->dport = src->sport;
}

SEC("sk_msg")
int sockmap_redirect_hash(struct sk_msg_md *msg)
{
    // 1. 构建本端四元组
    struct sock_key key = {};
    build_sock_key(msg, &key);
    
    // 2. 构建对端四元组
    struct sock_key peer_key = {};
    build_peer_key(&key, &peer_key);
    
    // 3. 更新统计
    __u32 stat_key = STAT_BYTES_TX;
    __u64 *bytes = bpf_map_lookup_elem(&sockmap_stats, &stat_key);
    if (bytes) {
        __sync_fetch_and_add(bytes, msg->size);
    }
    
    // 4. 执行重定向
    long ret = bpf_msg_redirect_hash(msg, &sock_hash, &peer_key, BPF_F_INGRESS);
    
    if (ret == SK_PASS) {
        stat_key = STAT_REDIRECT_OK;
        __u64 *ok = bpf_map_lookup_elem(&sockmap_stats, &stat_key);
        if (ok) __sync_fetch_and_add(ok, 1);
    } else {
        stat_key = STAT_REDIRECT_FAIL;
        __u64 *fail = bpf_map_lookup_elem(&sockmap_stats, &stat_key);
        if (fail) __sync_fetch_and_add(fail, 1);
    }
    
    return SK_PASS;
}

/* ============================================
 * Sockhash sockops：自动注入
 * ============================================ */

SEC("sockops")
int sockmap_sockops_hash(struct bpf_sock_ops *skops)
{
    __u32 op = skops->op;
    
    // 只处理 TCP 连接建立
    if (op != BPF_SOCK_OPS_PASSIVE_ESTABLISHED_CB &&
        op != BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB) {
        return 0;
    }
    
    // 构建四元组 Key
    struct sock_key key = {
        .sip = skops->local_ip4,
        .dip = skops->remote_ip4,
        .sport = skops->local_port,
        .dport = bpf_ntohl(skops->remote_port) >> 16,
    };
    
    // 将 Socket 加入 Sockhash
    bpf_sock_hash_update(skops, &sock_hash, &key, BPF_NOEXIST);
    
    return 0;
}

char _license[] SEC("license") = "GPL";
