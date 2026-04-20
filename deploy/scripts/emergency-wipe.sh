#!/bin/bash
# emergency-wipe.sh - 紧急擦除脚本
# 用途：手动触发焦土协议，安全擦除所有 Mirage 痕迹
# ⚠️  此操作不可逆！执行后 Gateway 将完全停止工作
# 用法：sudo bash emergency-wipe.sh [--confirm] [--include-logs]

set -e

CONFIRM=0
INCLUDE_LOGS=0

while [ $# -gt 0 ]; do
    case "$1" in
        --confirm) CONFIRM=1 ;;
        --include-logs) INCLUDE_LOGS=1 ;;
    esac
    shift
done

RED='\033[0;31m'; YELLOW='\033[1;33m'; NC='\033[0m'

echo -e "${RED}╔══════════════════════════════════════════════════╗${NC}"
echo -e "${RED}║         ⚠️  EMERGENCY WIPE / 焦土协议           ║${NC}"
echo -e "${RED}║                                                  ║${NC}"
echo -e "${RED}║  此操作将永久销毁所有 Mirage 相关数据：          ║${NC}"
echo -e "${RED}║  • mTLS 证书和私钥                               ║${NC}"
echo -e "${RED}║  • Gateway 配置文件                               ║${NC}"
echo -e "${RED}║  • eBPF 程序和 Map                                ║${NC}"
echo -e "${RED}║  • 认证密钥                                       ║${NC}"
echo -e "${RED}║  • 运行时状态                                     ║${NC}"
echo -e "${RED}║                                                  ║${NC}"
echo -e "${RED}║  执行后无法恢复！                                 ║${NC}"
echo -e "${RED}╚══════════════════════════════════════════════════╝${NC}"
echo ""

if [ "$CONFIRM" != "1" ]; then
    echo -e "${YELLOW}如确认执行，请添加 --confirm 参数${NC}"
    echo "  sudo bash emergency-wipe.sh --confirm"
    exit 1
fi

# 二次确认
echo -n "输入 'WIPE' 确认执行焦土协议: "
read -r INPUT
if [ "$INPUT" != "WIPE" ]; then
    echo "已取消"
    exit 1
fi

echo ""
echo "[$(date -u '+%H:%M:%S')] 开始执行焦土协议..."

# ─── 1. 停止 Gateway 进程 ───
echo "[1/7] 停止 Gateway 进程..."
systemctl stop mirage-gateway 2>/dev/null || true
pkill -9 -f "mirage-gateway" 2>/dev/null || true
sleep 1

# ─── 2. 卸载 eBPF 程序 ───
echo "[2/7] 卸载 eBPF 程序..."
# 清除 XDP
for iface in $(ip -o link show | awk -F': ' '{print $2}' | grep -v "^lo$"); do
    ip link set dev "$iface" xdp off 2>/dev/null || true
    ip link set dev "$iface" xdpgeneric off 2>/dev/null || true
done

# 清除 TC
for iface in $(ip -o link show | awk -F': ' '{print $2}' | grep -v "^lo$"); do
    tc filter del dev "$iface" ingress 2>/dev/null || true
    tc filter del dev "$iface" egress 2>/dev/null || true
    tc qdisc del dev "$iface" clsact 2>/dev/null || true
done

# 清除 bpffs
rm -rf /sys/fs/bpf/mirage* 2>/dev/null || true

echo "  ✅ eBPF 已卸载"

# ─── 3. 安全擦除证书和密钥 ───
echo "[3/7] 安全擦除证书和密钥..."

secure_delete() {
    local file=$1
    if [ -f "$file" ]; then
        # 3-pass 随机覆写 + 零化
        if command -v shred &>/dev/null; then
            shred -n 3 -z -u "$file" 2>/dev/null
        else
            # fallback: dd 覆写
            local size
            size=$(stat -c %s "$file" 2>/dev/null || echo "4096")
            dd if=/dev/urandom of="$file" bs=1 count="$size" conv=notrunc 2>/dev/null || true
            dd if=/dev/urandom of="$file" bs=1 count="$size" conv=notrunc 2>/dev/null || true
            dd if=/dev/urandom of="$file" bs=1 count="$size" conv=notrunc 2>/dev/null || true
            dd if=/dev/zero of="$file" bs=1 count="$size" conv=notrunc 2>/dev/null || true
            rm -f "$file"
        fi
        echo "    🔒 $file"
    fi
}

# 证书目录（tmpfs 上的）
secure_delete "/var/mirage/certs/gateway.key"
secure_delete "/var/mirage/certs/gateway.crt"
secure_delete "/var/mirage/certs/ca.crt"

# 备用位置
secure_delete "/etc/mirage/certs/gateway.key"
secure_delete "/etc/mirage/certs/gateway.crt"
secure_delete "/etc/mirage/certs/ca.crt"

# 用户密钥
secure_delete "$HOME/.mirage/private.key"

echo "  ✅ 密钥已擦除"

# ─── 4. 擦除配置文件 ───
echo "[4/7] 擦除配置文件..."

secure_delete "/etc/mirage/gateway.yaml"
secure_delete "/var/lib/mirage/gateway_id"
rm -rf /etc/mirage 2>/dev/null || true
rm -rf /var/lib/mirage 2>/dev/null || true

echo "  ✅ 配置已擦除"

# ─── 5. 清除运行时数据 ───
echo "[5/7] 清除运行时数据..."

rm -rf /var/mirage 2>/dev/null || true
rm -rf /tmp/mirage-* 2>/dev/null || true
rm -rf /run/mirage-* 2>/dev/null || true
rm -f /var/run/mirage-gateway.sock 2>/dev/null || true

echo "  ✅ 运行时数据已清除"

# ─── 6. 清除日志 ───
if [ "$INCLUDE_LOGS" = "1" ]; then
    echo "[6/7] 清除日志..."
    rm -rf /var/log/mirage-* 2>/dev/null || true
    journalctl --vacuum-time=1s --unit=mirage-gateway 2>/dev/null || true
    echo "  ✅ 日志已清除"
else
    echo "[6/7] 跳过日志清除（添加 --include-logs 清除）"
fi

# ─── 7. 清除 systemd 服务 ───
echo "[7/7] 清除服务配置..."
systemctl disable mirage-gateway 2>/dev/null || true
rm -f /etc/systemd/system/mirage-gateway.service 2>/dev/null || true
systemctl daemon-reload 2>/dev/null || true

echo "  ✅ 服务已移除"

# ─── 验证 ───
echo ""
echo "--- 擦除验证 ---"
REMNANTS=0

check_remnant() {
    if [ -e "$1" ]; then
        echo "  ⚠️  残留: $1"
        ((REMNANTS++))
    fi
}

check_remnant "/var/mirage"
check_remnant "/etc/mirage"
check_remnant "/var/lib/mirage"
check_remnant "/var/run/mirage-gateway.sock"
check_remnant "$HOME/.mirage/private.key"

# 检查 eBPF 残留
BPF_REMNANTS=$(bpftool prog list 2>/dev/null | grep -c "mirage" || echo "0")
if [ "$BPF_REMNANTS" -gt 0 ]; then
    echo "  ⚠️  eBPF 程序残留: $BPF_REMNANTS 个"
    ((REMNANTS++))
fi

if [ "$REMNANTS" -eq 0 ]; then
    echo "  ✅ 无残留"
fi

echo ""
echo -e "${RED}═══════════════════════════════════════════════════${NC}"
echo -e "${RED}  焦土协议执行完毕。所有 Mirage 痕迹已销毁。${NC}"
echo -e "${RED}═══════════════════════════════════════════════════${NC}"
