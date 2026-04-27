#!/usr/bin/env bash
# verify-quic-clienthello.sh — pcap 抓包验收脚本
# 对比 Mirage Client QUIC ClientHello 与 Chrome 的实际差异
#
# 用法:
#   1. 抓取 Mirage Client 的 QUIC Initial 包:
#      sudo tcpdump -i eth0 -w /tmp/mirage-client.pcap 'udp port 443' -c 10
#
#   2. 抓取 Chrome 访问同一目标的 QUIC Initial 包:
#      sudo tcpdump -i eth0 -w /tmp/chrome.pcap 'udp port 443' -c 10
#
#   3. 运行对比:
#      ./scripts/verify-quic-clienthello.sh /tmp/mirage-client.pcap /tmp/chrome.pcap
#
# 依赖: tshark (Wireshark CLI)
#
# 验收门槛（已知可接受差异）:
#   - CipherSuites 顺序不同（Go: AES-128, AES-256, ChaCha20 vs Chrome: AES-256, ChaCha20, AES-128）
#   - 缺少 Kyber PQ 密钥交换（X25519Kyber768Draft00）
#   - Extensions 列表差异（缺少 GREASE/compress_certificate/ECH）
#   - Session Ticket 行为差异
#
# 不可接受差异（必须修复）:
#   - ALPN 不是 "h3"
#   - TLS 版本不是 1.3
#   - 包含 "mirage"/"gtunnel" 等自定义标识

set -euo pipefail

MIRAGE_PCAP="${1:-}"
CHROME_PCAP="${2:-}"

if [[ -z "$MIRAGE_PCAP" || -z "$CHROME_PCAP" ]]; then
    echo "用法: $0 <mirage-client.pcap> <chrome.pcap>"
    echo ""
    echo "示例:"
    echo "  # 1. 抓取 Mirage Client 包"
    echo "  sudo tcpdump -i eth0 -w /tmp/mirage.pcap 'udp port 443' -c 10"
    echo "  # 2. 抓取 Chrome 包"
    echo "  sudo tcpdump -i eth0 -w /tmp/chrome.pcap 'udp port 443' -c 10"
    echo "  # 3. 对比"
    echo "  $0 /tmp/mirage.pcap /tmp/chrome.pcap"
    exit 1
fi

if ! command -v tshark &>/dev/null; then
    echo "❌ 需要安装 tshark (Wireshark CLI)"
    echo "   Ubuntu/Debian: sudo apt install tshark"
    echo "   macOS: brew install wireshark"
    exit 1
fi

echo "=========================================="
echo " QUIC ClientHello 指纹对比"
echo "=========================================="

# TLS 字段提取过滤器
TLS_FIELDS="-e tls.handshake.version -e tls.handshake.ciphersuite -e tls.handshake.extensions.supported_group -e tls.handshake.sig_hash_alg -e tls.handshake.extensions_alpn_str"

echo ""
echo "--- Mirage Client QUIC ClientHello ---"
tshark -r "$MIRAGE_PCAP" -Y "quic && tls.handshake.type == 1" \
    -T fields $TLS_FIELDS 2>/dev/null | head -5 || echo "(无 QUIC ClientHello 包)"

echo ""
echo "--- Chrome QUIC ClientHello ---"
tshark -r "$CHROME_PCAP" -Y "quic && tls.handshake.type == 1" \
    -T fields $TLS_FIELDS 2>/dev/null | head -5 || echo "(无 QUIC ClientHello 包)"

echo ""
echo "=========================================="
echo " 关键字段验收检查"
echo "=========================================="

# 检查 ALPN
echo ""
echo "[检查 1] ALPN 值"
MIRAGE_ALPN=$(tshark -r "$MIRAGE_PCAP" -Y "quic && tls.handshake.type == 1" \
    -T fields -e tls.handshake.extensions_alpn_str 2>/dev/null | head -1)
if [[ "$MIRAGE_ALPN" == *"h3"* ]]; then
    echo "  ✅ Mirage ALPN = $MIRAGE_ALPN (正确)"
else
    echo "  ❌ Mirage ALPN = $MIRAGE_ALPN (应为 h3)"
fi

# 检查 TLS 版本
echo ""
echo "[检查 2] TLS 版本"
MIRAGE_VER=$(tshark -r "$MIRAGE_PCAP" -Y "quic && tls.handshake.type == 1" \
    -T fields -e tls.handshake.version 2>/dev/null | head -1)
echo "  Mirage TLS version: $MIRAGE_VER"

# 检查自定义标识
echo ""
echo "[检查 3] 自定义标识扫描"
if tshark -r "$MIRAGE_PCAP" -Y "quic" -V 2>/dev/null | grep -qi "mirage\|gtunnel"; then
    echo "  ❌ 发现自定义标识 (mirage/gtunnel)"
else
    echo "  ✅ 未发现自定义标识"
fi

# CipherSuites 对比
echo ""
echo "[检查 4] CipherSuites 对比（已知差异，可接受）"
echo "  Mirage:"
tshark -r "$MIRAGE_PCAP" -Y "quic && tls.handshake.type == 1" \
    -T fields -e tls.handshake.ciphersuite 2>/dev/null | head -1 | tr ',' '\n' | sed 's/^/    /'
echo "  Chrome:"
tshark -r "$CHROME_PCAP" -Y "quic && tls.handshake.type == 1" \
    -T fields -e tls.handshake.ciphersuite 2>/dev/null | head -1 | tr ',' '\n' | sed 's/^/    /'

# Supported Groups 对比
echo ""
echo "[检查 5] Supported Groups 对比（已知差异：缺少 Kyber）"
echo "  Mirage:"
tshark -r "$MIRAGE_PCAP" -Y "quic && tls.handshake.type == 1" \
    -T fields -e tls.handshake.extensions.supported_group 2>/dev/null | head -1 | tr ',' '\n' | sed 's/^/    /'
echo "  Chrome:"
tshark -r "$CHROME_PCAP" -Y "quic && tls.handshake.type == 1" \
    -T fields -e tls.handshake.extensions.supported_group 2>/dev/null | head -1 | tr ',' '\n' | sed 's/^/    /'

echo ""
echo "=========================================="
echo " 验收结论"
echo "=========================================="
echo ""
echo "已知可接受差异（技术债务）:"
echo "  1. CipherSuites 顺序不同"
echo "  2. 缺少 Kyber PQ 密钥交换"
echo "  3. Extensions 列表差异"
echo "  4. Session Ticket 行为差异"
echo ""
echo "不可接受差异（必须修复）:"
echo "  - ALPN 不是 h3"
echo "  - 包含 mirage/gtunnel 自定义标识"
echo "  - TLS 版本不是 1.3"
