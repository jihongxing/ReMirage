#!/bin/bash
# ebpf-preflight.sh - eBPF 环境预检脚本
# 用途：部署 Mirage-Gateway 前验证系统是否满足 eBPF 运行条件
# 用法：sudo bash ebpf-preflight.sh

set -e

PASS=0; FAIL=0; WARN=0

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; NC='\033[0m'
pass() { echo -e "${GREEN}[PASS]${NC} $1"; ((++PASS)); }
fail() { echo -e "${RED}[FAIL]${NC} $1"; ((++FAIL)); }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; ((++WARN)); }

echo "============================================"
echo " Mirage eBPF 环境预检"
echo "============================================"
echo ""

# ─── 1. 内核版本 ───
echo "--- 内核版本 ---"
KVER=$(uname -r)
KMAJOR=$(echo "$KVER" | cut -d. -f1)
KMINOR=$(echo "$KVER" | cut -d. -f2)

if [ "$KMAJOR" -gt 5 ] || ([ "$KMAJOR" -eq 5 ] && [ "$KMINOR" -ge 15 ]); then
    pass "内核版本: $KVER (>= 5.15, 完整支持)"
elif [ "$KMAJOR" -eq 5 ] && [ "$KMINOR" -ge 4 ]; then
    warn "内核版本: $KVER (5.4-5.14, 部分功能受限)"
elif [ "$KMAJOR" -eq 4 ] && [ "$KMINOR" -ge 19 ]; then
    warn "内核版本: $KVER (4.19, 仅基础 XDP/TC，无 Ring Buffer)"
else
    fail "内核版本: $KVER (< 4.19, 不支持 eBPF)"
fi

# ─── 2. BPF 文件系统 ───
echo ""
echo "--- BPF 文件系统 ---"
if mount | grep -q "bpf on /sys/fs/bpf"; then
    pass "bpffs 已挂载: /sys/fs/bpf"
else
    if [ -d /sys/fs/bpf ]; then
        warn "bpffs 未挂载（需要: mount -t bpf bpf /sys/fs/bpf）"
    else
        fail "bpffs 不存在"
    fi
fi

# ─── 3. 内核配置 ───
echo ""
echo "--- 内核配置 ---"

check_kconfig() {
    local key=$1
    local desc=$2
    local critical=$3

    # 尝试多种方式读取内核配置
    local val=""
    if [ -f /proc/config.gz ]; then
        val=$(zcat /proc/config.gz 2>/dev/null | grep "^${key}=" | cut -d= -f2)
    elif [ -f "/boot/config-$(uname -r)" ]; then
        val=$(grep "^${key}=" "/boot/config-$(uname -r)" 2>/dev/null | cut -d= -f2)
    elif [ -f /lib/modules/$(uname -r)/config ]; then
        val=$(grep "^${key}=" "/lib/modules/$(uname -r)/config" 2>/dev/null | cut -d= -f2)
    fi

    if [ "$val" = "y" ] || [ "$val" = "m" ]; then
        pass "$desc ($key=$val)"
    elif [ -z "$val" ]; then
        if [ "$critical" = "1" ]; then
            warn "$desc ($key 未找到，无法确认)"
        fi
    else
        if [ "$critical" = "1" ]; then
            fail "$desc ($key=$val)"
        else
            warn "$desc ($key=$val)"
        fi
    fi
}

check_kconfig "CONFIG_BPF" "BPF 支持" 1
check_kconfig "CONFIG_BPF_SYSCALL" "BPF 系统调用" 1
check_kconfig "CONFIG_BPF_JIT" "BPF JIT 编译" 1
check_kconfig "CONFIG_XDP_SOCKETS" "XDP Socket" 1
check_kconfig "CONFIG_NET_CLS_BPF" "TC BPF 分类器" 1
check_kconfig "CONFIG_NET_ACT_BPF" "TC BPF 动作" 1
check_kconfig "CONFIG_BPF_STREAM_PARSER" "BPF Stream Parser (Sockmap)" 0
check_kconfig "CONFIG_CGROUP_BPF" "Cgroup BPF" 0
check_kconfig "CONFIG_BPF_LSM" "BPF LSM" 0

# ─── 4. BPF JIT 状态 ───
echo ""
echo "--- BPF JIT ---"
JIT_ENABLE=$(cat /proc/sys/net/core/bpf_jit_enable 2>/dev/null || echo "?")
if [ "$JIT_ENABLE" = "1" ] || [ "$JIT_ENABLE" = "2" ]; then
    pass "BPF JIT 已启用 (bpf_jit_enable=$JIT_ENABLE)"
elif [ "$JIT_ENABLE" = "0" ]; then
    warn "BPF JIT 未启用（性能下降，建议: sysctl net.core.bpf_jit_enable=1）"
else
    warn "无法读取 BPF JIT 状态"
fi

# unprivileged BPF
UNPRIV=$(cat /proc/sys/kernel/unprivileged_bpf_disabled 2>/dev/null || echo "?")
if [ "$UNPRIV" = "1" ] || [ "$UNPRIV" = "2" ]; then
    pass "非特权 BPF 已禁用 (安全)"
else
    warn "非特权 BPF 未禁用 (unprivileged_bpf_disabled=$UNPRIV)"
fi

# ─── 5. 工具链 ───
echo ""
echo "--- 编译工具链 ---"

# clang
if command -v clang &>/dev/null; then
    CLANG_VER=$(clang --version | head -1 | grep -oP '\d+\.\d+\.\d+' | head -1)
    CLANG_MAJOR=$(echo "$CLANG_VER" | cut -d. -f1)
    if [ "$CLANG_MAJOR" -ge 14 ]; then
        pass "clang: $CLANG_VER (>= 14)"
    elif [ "$CLANG_MAJOR" -ge 11 ]; then
        warn "clang: $CLANG_VER (建议 >= 14)"
    else
        fail "clang: $CLANG_VER (< 11, 不支持 BPF CO-RE)"
    fi
else
    fail "clang 未安装"
fi

# llc
if command -v llc &>/dev/null; then
    LLC_VER=$(llc --version 2>&1 | grep -oP '\d+\.\d+\.\d+' | head -1)
    pass "llc: $LLC_VER"
else
    warn "llc 未安装（非必须，clang 可直接编译 BPF）"
fi

# bpftool
if command -v bpftool &>/dev/null; then
    BPFTOOL_VER=$(bpftool version 2>/dev/null | head -1)
    pass "bpftool: $BPFTOOL_VER"
else
    warn "bpftool 未安装（调试用，非必须）"
fi

# Go
if command -v go &>/dev/null; then
    GO_VER=$(go version | grep -oP 'go\d+\.\d+(\.\d+)?')
    GO_MINOR=$(echo "$GO_VER" | grep -oP '\d+\.\d+' | cut -d. -f2)
    if [ "$GO_MINOR" -ge 21 ]; then
        pass "Go: $GO_VER (>= 1.21)"
    else
        warn "Go: $GO_VER (建议 >= 1.21)"
    fi
else
    fail "Go 未安装"
fi

# ─── 6. 内核头文件 ───
echo ""
echo "--- 内核头文件 ---"
HEADER_PATH="/lib/modules/$(uname -r)/build"
if [ -d "$HEADER_PATH" ]; then
    pass "内核头文件: $HEADER_PATH"
elif [ -d "/usr/src/linux-headers-$(uname -r)" ]; then
    pass "内核头文件: /usr/src/linux-headers-$(uname -r)"
else
    warn "内核头文件未找到（CO-RE 模式可能不需要）"
fi

# BTF 支持
if [ -f "/sys/kernel/btf/vmlinux" ]; then
    pass "BTF vmlinux 可用 (CO-RE 支持)"
else
    warn "BTF vmlinux 不可用（需要非 CO-RE 编译或安装 BTF）"
fi

# ─── 7. 网络接口 XDP 支持 ───
echo ""
echo "--- 网络接口 XDP 支持 ---"

# 检查主要接口
for iface in $(ip -o link show up | awk -F': ' '{print $2}' | grep -v "^lo$" | head -5); do
    DRIVER=$(ethtool -i "$iface" 2>/dev/null | grep "^driver:" | awk '{print $2}')
    if [ -z "$DRIVER" ]; then
        DRIVER="unknown"
    fi

    # 已知支持 XDP native 的驱动
    XDP_NATIVE_DRIVERS="i40e|ixgbe|mlx5_core|mlx4_en|bnxt_en|nfp|virtio_net|veth|ena|ice|igc"
    if echo "$DRIVER" | grep -qP "$XDP_NATIVE_DRIVERS"; then
        pass "$iface ($DRIVER) — 支持 XDP native"
    else
        warn "$iface ($DRIVER) — 可能仅支持 XDP generic（性能较低）"
    fi
done

# ─── 8. 资源限制 ───
echo ""
echo "--- 资源限制 ---"

# RLIMIT_MEMLOCK
MEMLOCK=$(ulimit -l 2>/dev/null)
if [ "$MEMLOCK" = "unlimited" ]; then
    pass "RLIMIT_MEMLOCK: unlimited"
elif [ "$MEMLOCK" -ge 65536 ]; then
    pass "RLIMIT_MEMLOCK: ${MEMLOCK} KB"
else
    warn "RLIMIT_MEMLOCK: ${MEMLOCK} KB (建议 unlimited: ulimit -l unlimited)"
fi

# 可用内存
MEM_AVAIL=$(awk '/MemAvailable/ {print int($2/1024)}' /proc/meminfo 2>/dev/null || echo "0")
if [ "$MEM_AVAIL" -ge 512 ]; then
    pass "可用内存: ${MEM_AVAIL} MB"
elif [ "$MEM_AVAIL" -ge 256 ]; then
    warn "可用内存: ${MEM_AVAIL} MB (建议 >= 512 MB)"
else
    fail "可用内存: ${MEM_AVAIL} MB (不足)"
fi

# ─── 9. 编译验证 ───
echo ""
echo "--- eBPF 编译验证 ---"

TMPDIR=$(mktemp -d)
cat > "$TMPDIR/test.c" << 'EOF'
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

SEC("xdp")
int test_xdp(struct xdp_md *ctx) {
    return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
EOF

if clang -O2 -target bpf -c "$TMPDIR/test.c" -o "$TMPDIR/test.o" 2>/dev/null; then
    pass "eBPF 编译测试通过"
    # 尝试加载
    if command -v bpftool &>/dev/null; then
        if bpftool prog load "$TMPDIR/test.o" /sys/fs/bpf/mirage_test 2>/dev/null; then
            pass "eBPF 加载测试通过"
            rm -f /sys/fs/bpf/mirage_test
        else
            warn "eBPF 加载测试失败（可能需要 root 或 CAP_BPF）"
        fi
    fi
else
    fail "eBPF 编译测试失败（检查 clang 和头文件）"
fi
rm -rf "$TMPDIR"

# ─── 汇总 ───
echo ""
echo "============================================"
echo " 预检结果汇总"
echo "============================================"
echo -e " ${GREEN}通过: $PASS${NC} | ${YELLOW}警告: $WARN${NC} | ${RED}失败: $FAIL${NC}"
echo ""

if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}✅ 环境满足 Mirage-Gateway eBPF 运行要求${NC}"
    exit 0
elif [ "$FAIL" -le 2 ]; then
    echo -e "${YELLOW}⚠️  存在问题，部分功能可能受限${NC}"
    exit 1
else
    echo -e "${RED}❌ 环境不满足要求，无法部署${NC}"
    exit 2
fi
