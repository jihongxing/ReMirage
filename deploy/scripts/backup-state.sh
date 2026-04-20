#!/bin/bash
# backup-state.sh - Gateway 状态备份脚本
# 用途：备份 Gateway 运行状态快照（配置、证书指纹、eBPF Map 快照）
# 用法：sudo bash backup-state.sh [--output-dir /path] [--encrypt]
# 注意：不备份私钥本身，仅备份证书指纹用于验证

set -e

OUTPUT_DIR="/var/backups/mirage"
ENCRYPT=0
GPG_RECIPIENT=""
CERT_DIR="/var/mirage/certs"
CONFIG_DIR="/etc/mirage"

while [ $# -gt 0 ]; do
    case "$1" in
        --output-dir) OUTPUT_DIR="$2"; shift ;;
        --encrypt) ENCRYPT=1 ;;
        --gpg-recipient) GPG_RECIPIENT="$2"; shift ;;
        --cert-dir) CERT_DIR="$2"; shift ;;
        --config-dir) CONFIG_DIR="$2"; shift ;;
    esac
    shift
done

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_NAME="mirage-state-${TIMESTAMP}"
BACKUP_DIR="${OUTPUT_DIR}/${BACKUP_NAME}"

mkdir -p "$BACKUP_DIR"

echo "============================================"
echo " Mirage Gateway 状态备份"
echo " 输出: $BACKUP_DIR"
echo "============================================"
echo ""

# ─── 1. 系统信息 ───
echo "[1/6] 系统信息..."
cat > "$BACKUP_DIR/system-info.txt" << EOF
Hostname: $(hostname)
Kernel: $(uname -r)
OS: $(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d= -f2 | tr -d '"')
Arch: $(uname -m)
Date: $(date -u '+%Y-%m-%dT%H:%M:%SZ')
Uptime: $(uptime -p 2>/dev/null || uptime)
Gateway PID: $(pgrep -f "mirage-gateway" | head -1 || echo "N/A")
EOF
echo "  ✅ system-info.txt"

# ─── 2. 配置文件 ───
echo "[2/6] 配置文件..."
if [ -d "$CONFIG_DIR" ]; then
    cp -r "$CONFIG_DIR" "$BACKUP_DIR/config" 2>/dev/null || true
    # 移除敏感信息
    find "$BACKUP_DIR/config" -name "*.key" -delete 2>/dev/null || true
    echo "  ✅ config/ (私钥已排除)"
else
    echo "  ⚠️  配置目录不存在: $CONFIG_DIR"
fi

# ─── 3. 证书指纹（不备份私钥）───
echo "[3/6] 证书指纹..."
CERT_INFO="$BACKUP_DIR/cert-fingerprints.txt"
echo "# 证书指纹快照 - $(date -u)" > "$CERT_INFO"
echo "" >> "$CERT_INFO"

for cert_file in "$CERT_DIR"/*.crt "$CONFIG_DIR"/*.crt; do
    if [ -f "$cert_file" ]; then
        echo "--- $cert_file ---" >> "$CERT_INFO"
        openssl x509 -in "$cert_file" -noout \
            -subject -issuer -dates -fingerprint -serial \
            2>/dev/null >> "$CERT_INFO" || true
        echo "" >> "$CERT_INFO"
    fi
done
echo "  ✅ cert-fingerprints.txt"

# ─── 4. eBPF 状态快照 ───
echo "[4/6] eBPF 状态..."
EBPF_DIR="$BACKUP_DIR/ebpf"
mkdir -p "$EBPF_DIR"

if command -v bpftool &>/dev/null; then
    bpftool prog list --json 2>/dev/null > "$EBPF_DIR/programs.json" || true
    bpftool map list --json 2>/dev/null > "$EBPF_DIR/maps.json" || true

    # 导出关键 Map 内容
    for map_name in threat_level_map defense_config quota_map; do
        MAP_ID=$(bpftool map list 2>/dev/null | grep "$map_name" | awk '{print $1}' | tr -d ':')
        if [ -n "$MAP_ID" ]; then
            bpftool map dump id "$MAP_ID" --json 2>/dev/null > "$EBPF_DIR/map-${map_name}.json" || true
        fi
    done
    echo "  ✅ ebpf/ (programs + maps)"
else
    echo "  ⚠️  bpftool 未安装，跳过 eBPF 快照"
fi

# ─── 5. 网络状态 ───
echo "[5/6] 网络状态..."
NET_DIR="$BACKUP_DIR/network"
mkdir -p "$NET_DIR"

ip addr show > "$NET_DIR/ip-addr.txt" 2>/dev/null || true
ip route show > "$NET_DIR/ip-route.txt" 2>/dev/null || true
ss -tunlp > "$NET_DIR/listening-ports.txt" 2>/dev/null || true
iptables -L -n > "$NET_DIR/iptables.txt" 2>/dev/null || true

# TC 规则
for iface in $(ip -o link show up | awk -F': ' '{print $2}' | grep -v "^lo$"); do
    tc filter show dev "$iface" ingress 2>/dev/null > "$NET_DIR/tc-${iface}-ingress.txt" || true
    tc filter show dev "$iface" egress 2>/dev/null > "$NET_DIR/tc-${iface}-egress.txt" || true
done

# XDP 状态
ip link show | grep -A1 "xdp" > "$NET_DIR/xdp-status.txt" 2>/dev/null || true

echo "  ✅ network/"

# ─── 6. Gateway 运行时状态 ───
echo "[6/6] 运行时状态..."
RUNTIME_DIR="$BACKUP_DIR/runtime"
mkdir -p "$RUNTIME_DIR"

# 从 health API 获取
GATEWAY_ADDR="127.0.0.1:9090"
curl -s "http://${GATEWAY_ADDR}/health" > "$RUNTIME_DIR/health.json" 2>/dev/null || true
curl -s "http://${GATEWAY_ADDR}/api/tunnel/status" > "$RUNTIME_DIR/tunnel.json" 2>/dev/null || true
curl -s "http://${GATEWAY_ADDR}/api/threat/summary" > "$RUNTIME_DIR/threat.json" 2>/dev/null || true
curl -s "http://${GATEWAY_ADDR}/api/quota" > "$RUNTIME_DIR/quota.json" 2>/dev/null || true

# 进程信息
GW_PID=$(pgrep -f "mirage-gateway" | head -1)
if [ -n "$GW_PID" ]; then
    ps -p "$GW_PID" -o pid,ppid,%cpu,%mem,rss,vsz,etime,args > "$RUNTIME_DIR/process.txt" 2>/dev/null || true
    cat /proc/$GW_PID/status > "$RUNTIME_DIR/proc-status.txt" 2>/dev/null || true
fi

echo "  ✅ runtime/"

# ─── 打包 ───
echo ""
echo "--- 打包 ---"

ARCHIVE="${OUTPUT_DIR}/${BACKUP_NAME}.tar.gz"
tar -czf "$ARCHIVE" -C "$OUTPUT_DIR" "$BACKUP_NAME" 2>/dev/null
rm -rf "$BACKUP_DIR"

if [ "$ENCRYPT" = "1" ]; then
    if [ -n "$GPG_RECIPIENT" ]; then
        gpg --encrypt --recipient "$GPG_RECIPIENT" "$ARCHIVE" 2>/dev/null
        rm -f "$ARCHIVE"
        ARCHIVE="${ARCHIVE}.gpg"
        echo "  🔒 已加密: $ARCHIVE"
    else
        echo "  ⚠️  未指定 GPG 接收者，跳过加密"
    fi
fi

# 清理旧备份（保留最近 7 个）
ls -t "${OUTPUT_DIR}"/mirage-state-*.tar.gz* 2>/dev/null | tail -n +8 | xargs rm -f 2>/dev/null || true

SIZE=$(du -h "$ARCHIVE" | awk '{print $1}')
echo ""
echo "✅ 备份完成: $ARCHIVE ($SIZE)"
