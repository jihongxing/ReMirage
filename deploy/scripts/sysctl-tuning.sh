#!/bin/bash
# sysctl-tuning.sh - 系统内核参数调优脚本
# 用途：为 Mirage-Gateway 优化网络栈和 eBPF 相关内核参数
# 用法：sudo bash sysctl-tuning.sh [--apply] [--revert]
# 不带 --apply 时仅显示建议，不修改系统

set -e

APPLY=0
REVERT=0
BACKUP_FILE="/etc/sysctl.d/99-mirage-backup.conf"
CONF_FILE="/etc/sysctl.d/90-mirage-gateway.conf"

while [ $# -gt 0 ]; do
    case "$1" in
        --apply) APPLY=1 ;;
        --revert) REVERT=1 ;;
    esac
    shift
done

if [ "$REVERT" = "1" ]; then
    if [ -f "$BACKUP_FILE" ]; then
        echo "恢复原始参数..."
        sysctl -p "$BACKUP_FILE" >/dev/null 2>&1
        rm -f "$CONF_FILE"
        echo "✅ 已恢复"
    else
        echo "❌ 备份文件不存在，无法恢复"
    fi
    exit 0
fi

echo "============================================"
echo " Mirage-Gateway 内核参数调优"
echo "============================================"
echo ""

# 定义调优参数
declare -A PARAMS
declare -A DESCRIPTIONS

# ─── 网络缓冲区 ───
PARAMS["net.core.rmem_max"]="67108864"
DESCRIPTIONS["net.core.rmem_max"]="UDP 接收缓冲区上限 (64MB)"

PARAMS["net.core.wmem_max"]="67108864"
DESCRIPTIONS["net.core.wmem_max"]="UDP 发送缓冲区上限 (64MB)"

PARAMS["net.core.rmem_default"]="1048576"
DESCRIPTIONS["net.core.rmem_default"]="默认接收缓冲区 (1MB)"

PARAMS["net.core.wmem_default"]="1048576"
DESCRIPTIONS["net.core.wmem_default"]="默认发送缓冲区 (1MB)"

PARAMS["net.core.netdev_max_backlog"]="65536"
DESCRIPTIONS["net.core.netdev_max_backlog"]="网卡接收队列长度"

PARAMS["net.core.somaxconn"]="65535"
DESCRIPTIONS["net.core.somaxconn"]="Socket 监听队列上限"

# ─── UDP 优化 ───
PARAMS["net.ipv4.udp_mem"]="4096 87380 67108864"
DESCRIPTIONS["net.ipv4.udp_mem"]="UDP 内存限制 (min/pressure/max)"

PARAMS["net.ipv4.udp_rmem_min"]="8192"
DESCRIPTIONS["net.ipv4.udp_rmem_min"]="UDP 最小接收缓冲区"

PARAMS["net.ipv4.udp_wmem_min"]="8192"
DESCRIPTIONS["net.ipv4.udp_wmem_min"]="UDP 最小发送缓冲区"

# ─── conntrack ───
PARAMS["net.netfilter.nf_conntrack_udp_timeout"]="60"
DESCRIPTIONS["net.netfilter.nf_conntrack_udp_timeout"]="UDP conntrack 超时 (60s)"

PARAMS["net.netfilter.nf_conntrack_udp_timeout_stream"]="180"
DESCRIPTIONS["net.netfilter.nf_conntrack_udp_timeout_stream"]="UDP stream conntrack 超时"

PARAMS["net.netfilter.nf_conntrack_max"]="1048576"
DESCRIPTIONS["net.netfilter.nf_conntrack_max"]="conntrack 表最大条目"

# ─── TCP 优化（WSS 传输用）───
PARAMS["net.ipv4.tcp_fastopen"]="3"
DESCRIPTIONS["net.ipv4.tcp_fastopen"]="TCP Fast Open (客户端+服务端)"

PARAMS["net.ipv4.tcp_congestion_control"]="bbr"
DESCRIPTIONS["net.ipv4.tcp_congestion_control"]="拥塞控制算法 (BBR)"

PARAMS["net.core.default_qdisc"]="fq"
DESCRIPTIONS["net.core.default_qdisc"]="默认队列调度 (Fair Queue, BBR 需要)"

PARAMS["net.ipv4.tcp_mtu_probing"]="1"
DESCRIPTIONS["net.ipv4.tcp_mtu_probing"]="TCP MTU 探测"

# ─── BPF ───
PARAMS["net.core.bpf_jit_enable"]="1"
DESCRIPTIONS["net.core.bpf_jit_enable"]="BPF JIT 编译"

PARAMS["kernel.unprivileged_bpf_disabled"]="1"
DESCRIPTIONS["kernel.unprivileged_bpf_disabled"]="禁止非特权 BPF (安全)"

# ─── 安全加固 ───
PARAMS["kernel.core_pattern"]="/dev/null"
DESCRIPTIONS["kernel.core_pattern"]="禁用 core dump (防内存泄露)"

PARAMS["kernel.dmesg_restrict"]="1"
DESCRIPTIONS["kernel.dmesg_restrict"]="限制 dmesg 访问"

PARAMS["kernel.kptr_restrict"]="2"
DESCRIPTIONS["kernel.kptr_restrict"]="隐藏内核指针"

PARAMS["net.ipv4.conf.all.rp_filter"]="1"
DESCRIPTIONS["net.ipv4.conf.all.rp_filter"]="反向路径过滤 (防 IP 欺骗)"

# ─── 显示当前值与建议值 ───
echo "参数                                          当前值              建议值              说明"
echo "──────────────────────────────────────────────────────────────────────────────────────────────────────"

CHANGES=0
BACKUP_CONTENT=""
CONF_CONTENT="# Mirage-Gateway 内核参数调优\n# 生成时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ')\n# 恢复: sudo bash sysctl-tuning.sh --revert\n\n"

for key in "${!PARAMS[@]}"; do
    target="${PARAMS[$key]}"
    desc="${DESCRIPTIONS[$key]}"
    current=$(sysctl -n "$key" 2>/dev/null | tr '\t' ' ' || echo "N/A")

    if [ "$current" = "N/A" ]; then
        printf "%-46s %-20s %-20s %s\n" "$key" "N/A" "$target" "$desc"
        continue
    fi

    if [ "$current" = "$target" ]; then
        printf "%-46s %-20s %-20s %s\n" "$key" "✅ $current" "$target" "$desc"
    else
        printf "%-46s %-20s %-20s %s\n" "$key" "⚠️  $current" "$target" "$desc"
        BACKUP_CONTENT="${BACKUP_CONTENT}${key} = ${current}\n"
        CONF_CONTENT="${CONF_CONTENT}${key} = ${target}\n"
        ((CHANGES++))
    fi
done

echo ""
echo "需要修改: $CHANGES 项"

if [ "$CHANGES" -eq 0 ]; then
    echo "✅ 所有参数已是最优值"
    exit 0
fi

if [ "$APPLY" = "0" ]; then
    echo ""
    echo "提示: 添加 --apply 参数执行修改"
    echo "      添加 --revert 参数恢复原始值"
    exit 0
fi

# ─── 应用修改 ───
echo ""
echo "--- 应用修改 ---"

# 备份当前值
echo -e "$BACKUP_CONTENT" > "$BACKUP_FILE"
echo "  📋 原始值已备份: $BACKUP_FILE"

# 写入配置
echo -e "$CONF_CONTENT" > "$CONF_FILE"
echo "  📝 配置已写入: $CONF_FILE"

# 加载 BBR 模块（如果需要）
if [ "${PARAMS["net.ipv4.tcp_congestion_control"]}" = "bbr" ]; then
    modprobe tcp_bbr 2>/dev/null || true
fi

# 应用
sysctl -p "$CONF_FILE" >/dev/null 2>&1
echo "  ✅ 参数已生效"

# 验证
echo ""
echo "--- 验证 ---"
VERIFY_FAIL=0
for key in "${!PARAMS[@]}"; do
    target="${PARAMS[$key]}"
    actual=$(sysctl -n "$key" 2>/dev/null | tr '\t' ' ' || echo "N/A")
    if [ "$actual" != "$target" ] && [ "$actual" != "N/A" ]; then
        echo "  ⚠️  $key = $actual (期望: $target)"
        ((VERIFY_FAIL++))
    fi
done

if [ "$VERIFY_FAIL" -eq 0 ]; then
    echo "  ✅ 全部验证通过"
else
    echo "  ⚠️  $VERIFY_FAIL 项未生效（可能需要重启或内核不支持）"
fi
