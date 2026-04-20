// sock_hijack.c - DNS-less 透明劫持（cgroup sock_addr）
// 挂载点：BPF_CGROUP_INET4_CONNECT (TCP) + BPF_CGROUP_UDP4_SENDMSG (UDP)
//
// 原理：在应用程序调用 connect()/sendmsg() 的瞬间，将 Socket 级别的
// 目标 IP 从假 IP (198.18.0.1) 替换为真实 Gateway IP。
// 之后的路由查找、Conntrack、Checksum 全部由内核自动完成。
//
// 优势：
//   - 零 Checksum 重算（内核在 sock_addr 之后才封包）
//   - 天然双向互通（Conntrack 基于修改后的真实 IP 建立）
//   - 零 DNS 痕迹（宿主机无任何 DNS 请求记录）
//   - 零性能损耗（仅在 connect/sendmsg 时触发一次，非每包）

#include <linux/bpf.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

// 假 IP：198.18.0.1 (0xC6120001)，客户端配置的代理地址
#define FAKE_IP_BE bpf_htonl(0xC6120001)

// gw_ip_map: Go 控制面写入真实 Gateway IP
// key=0 → real_ip (网络字节序)
// key=1 → real_port (网络字节序, 16-bit 存在 32-bit 中)
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 2);
    __type(key, __u32);
    __type(value, __u32);
} gw_ip_map SEC(".maps");

// hijack_enabled_map: 开关（key=0, value=1 启用, 0 禁用）
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u32);
} hijack_enabled_map SEC(".maps");

// 检查是否启用
static __always_inline int is_enabled(void)
{
    __u32 key = 0;
    __u32 *val = bpf_map_lookup_elem(&hijack_enabled_map, &key);
    if (!val)
        return 0;
    return *val == 1;
}

// 获取真实 Gateway IP（网络字节序）
static __always_inline __u32 get_real_ip(void)
{
    __u32 key = 0;
    __u32 *val = bpf_map_lookup_elem(&gw_ip_map, &key);
    if (!val)
        return 0;
    return *val;
}

// 获取真实 Gateway 端口（网络字节序，存储在 32-bit 中）
static __always_inline __u32 get_real_port(void)
{
    __u32 key = 1;
    __u32 *val = bpf_map_lookup_elem(&gw_ip_map, &key);
    if (!val)
        return 0;
    return *val;
}

// TCP connect() 劫持
SEC("cgroup/connect4")
int hijack_tcp_connect(struct bpf_sock_addr *ctx)
{
    // 1. 检查开关
    if (!is_enabled())
        return 1; // 放行，不修改

    // 2. 检查目标 IP 是否为假 IP
    if (ctx->user_ip4 != FAKE_IP_BE)
        return 1; // 非目标流量，放行

    // 3. 读取真实 Gateway IP
    __u32 real_ip = get_real_ip();
    if (real_ip == 0)
        return 1; // 未配置真实 IP，放行

    // 4. 偷天换日：修改 Socket 目标地址
    ctx->user_ip4 = real_ip;

    // 5. 如果端口也需要替换
    __u32 real_port = get_real_port();
    if (real_port != 0) {
        ctx->user_port = real_port;
    }

    return 1; // 允许连接
}

// UDP sendmsg() 劫持（QUIC 走 UDP）
SEC("cgroup/sendmsg4")
int hijack_udp_sendmsg(struct bpf_sock_addr *ctx)
{
    if (!is_enabled())
        return 1;

    if (ctx->user_ip4 != FAKE_IP_BE)
        return 1;

    __u32 real_ip = get_real_ip();
    if (real_ip == 0)
        return 1;

    ctx->user_ip4 = real_ip;

    __u32 real_port = get_real_port();
    if (real_port != 0) {
        ctx->user_port = real_port;
    }

    return 1;
}

// UDP recvmsg() 地址回写（让应用层看到假 IP 而非真实 IP）
SEC("cgroup/recvmsg4")
int hijack_udp_recvmsg(struct bpf_sock_addr *ctx)
{
    if (!is_enabled())
        return 1;

    // 如果源 IP 是真实 Gateway IP，回写为假 IP
    // 这样应用层的 recvfrom() 看到的源地址仍然是 198.18.0.1
    __u32 real_ip = get_real_ip();
    if (real_ip == 0)
        return 1;

    if (ctx->user_ip4 == real_ip) {
        ctx->user_ip4 = FAKE_IP_BE;
    }

    return 1;
}

char _license[] SEC("license") = "GPL";
