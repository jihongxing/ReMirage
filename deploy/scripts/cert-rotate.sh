#!/bin/bash
# cert-rotate.sh - mTLS 证书轮换脚本
# 用途：检查 Gateway/OS 证书有效期，到期前自动轮换
# 用法：sudo bash cert-rotate.sh [--check-only] [--days-before 30] [--cert-dir /var/mirage/certs]
# 建议 cron: 0 3 * * * /opt/mirage/scripts/cert-rotate.sh >> /var/log/mirage-cert-rotate.log 2>&1

set -e

CHECK_ONLY=0
DAYS_BEFORE=30
CERT_DIR="/var/mirage/certs"
CA_DIR="/etc/mirage/ca"
OPENSSL_CNF="/etc/mirage/openssl.cnf"
GATEWAY_ID=""
RESTART_GATEWAY=1

# 解析参数
while [ $# -gt 0 ]; do
    case "$1" in
        --check-only) CHECK_ONLY=1 ;;
        --days-before) DAYS_BEFORE="$2"; shift ;;
        --cert-dir) CERT_DIR="$2"; shift ;;
        --ca-dir) CA_DIR="$2"; shift ;;
        --gateway-id) GATEWAY_ID="$2"; shift ;;
        --no-restart) RESTART_GATEWAY=0 ;;
    esac
    shift
done

TIMESTAMP=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
echo "[$TIMESTAMP] 证书轮换检查"
echo "  证书目录: $CERT_DIR"
echo "  预警天数: $DAYS_BEFORE 天"
echo ""

# 检查证书有效期
check_cert_expiry() {
    local cert_file=$1
    local name=$2

    if [ ! -f "$cert_file" ]; then
        echo "  ❌ $name: 文件不存在 ($cert_file)"
        return 2
    fi

    local expiry_date
    expiry_date=$(openssl x509 -enddate -noout -in "$cert_file" 2>/dev/null | cut -d= -f2)
    if [ -z "$expiry_date" ]; then
        echo "  ❌ $name: 无法解析证书"
        return 2
    fi

    local expiry_epoch
    expiry_epoch=$(date -d "$expiry_date" +%s 2>/dev/null || date -j -f "%b %d %T %Y %Z" "$expiry_date" +%s 2>/dev/null)
    local now_epoch
    now_epoch=$(date +%s)
    local days_left=$(( (expiry_epoch - now_epoch) / 86400 ))

    local subject
    subject=$(openssl x509 -subject -noout -in "$cert_file" 2>/dev/null | sed 's/subject=//')

    if [ "$days_left" -le 0 ]; then
        echo "  🔴 $name: 已过期 ($expiry_date)"
        echo "      Subject: $subject"
        return 2
    elif [ "$days_left" -le "$DAYS_BEFORE" ]; then
        echo "  🟡 $name: ${days_left} 天后过期 ($expiry_date)"
        echo "      Subject: $subject"
        return 1
    else
        echo "  🟢 $name: ${days_left} 天后过期 ($expiry_date)"
        return 0
    fi
}

NEED_ROTATE=0

# 检查各证书
echo "--- 证书有效期检查 ---"
check_cert_expiry "$CERT_DIR/gateway.crt" "Gateway 证书" || NEED_ROTATE=1
check_cert_expiry "$CERT_DIR/ca.crt" "CA 证书" || true  # CA 过期需要手动处理
check_cert_expiry "$CA_DIR/root-ca.crt" "Root CA" || true

echo ""

if [ "$CHECK_ONLY" = "1" ]; then
    if [ "$NEED_ROTATE" = "1" ]; then
        echo "⚠️  需要轮换证书"
        exit 1
    else
        echo "✅ 所有证书有效期充足"
        exit 0
    fi
fi

if [ "$NEED_ROTATE" = "0" ]; then
    echo "✅ 无需轮换"
    exit 0
fi

# ─── 执行证书轮换 ───
echo "--- 开始证书轮换 ---"

# 确认 CA 密钥可用
if [ ! -f "$CA_DIR/root-ca.key" ]; then
    echo "❌ CA 私钥不存在 ($CA_DIR/root-ca.key)，无法自动轮换"
    echo "   请手动执行证书签发"
    exit 2
fi

# 确定 Gateway ID
if [ -z "$GATEWAY_ID" ]; then
    if [ -f /var/lib/mirage/gateway_id ]; then
        GATEWAY_ID=$(cat /var/lib/mirage/gateway_id)
    else
        GATEWAY_ID=$(hostname)
    fi
fi
echo "  Gateway ID: $GATEWAY_ID"

# 备份旧证书
BACKUP_DIR="$CERT_DIR/backup-$(date +%Y%m%d%H%M%S)"
mkdir -p "$BACKUP_DIR"
cp -f "$CERT_DIR/gateway.crt" "$BACKUP_DIR/" 2>/dev/null || true
cp -f "$CERT_DIR/gateway.key" "$BACKUP_DIR/" 2>/dev/null || true
echo "  旧证书已备份: $BACKUP_DIR"

# 生成新私钥
openssl ecparam -genkey -name prime256v1 -out "$CERT_DIR/gateway.key.new" 2>/dev/null
chmod 600 "$CERT_DIR/gateway.key.new"

# 生成 CSR
openssl req -new \
    -key "$CERT_DIR/gateway.key.new" \
    -out "$CERT_DIR/gateway.csr" \
    -subj "/CN=mirage-gateway-${GATEWAY_ID}/O=Mirage Project" \
    2>/dev/null

# 签发证书（365 天）
SERIAL=$(date +%s%N | sha256sum | head -c 16)
openssl x509 -req \
    -in "$CERT_DIR/gateway.csr" \
    -CA "$CA_DIR/root-ca.crt" \
    -CAkey "$CA_DIR/root-ca.key" \
    -set_serial "0x${SERIAL}" \
    -days 365 \
    -sha256 \
    -extfile <(printf "subjectAltName=DNS:mirage-gateway,DNS:localhost,IP:127.0.0.1\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=clientAuth,serverAuth") \
    -out "$CERT_DIR/gateway.crt.new" \
    2>/dev/null

# 验证新证书
if openssl verify -CAfile "$CA_DIR/root-ca.crt" "$CERT_DIR/gateway.crt.new" >/dev/null 2>&1; then
    echo "  ✅ 新证书验证通过"
else
    echo "  ❌ 新证书验证失败，回滚"
    rm -f "$CERT_DIR/gateway.key.new" "$CERT_DIR/gateway.crt.new" "$CERT_DIR/gateway.csr"
    exit 2
fi

# 原子替换
mv -f "$CERT_DIR/gateway.key.new" "$CERT_DIR/gateway.key"
mv -f "$CERT_DIR/gateway.crt.new" "$CERT_DIR/gateway.crt"
rm -f "$CERT_DIR/gateway.csr"

echo "  ✅ 证书已替换"

# 安全擦除备份中的旧私钥（3-pass）
if command -v shred &>/dev/null; then
    shred -n 3 -z -u "$BACKUP_DIR/gateway.key" 2>/dev/null || true
    echo "  🔒 旧私钥已安全擦除"
fi

# 重启 Gateway（触发证书热加载）
if [ "$RESTART_GATEWAY" = "1" ]; then
    if systemctl is-active mirage-gateway >/dev/null 2>&1; then
        # 发送 SIGHUP 触发证书热加载（如果支持）
        GW_PID=$(pgrep -f "mirage-gateway" | head -1)
        if [ -n "$GW_PID" ]; then
            kill -HUP "$GW_PID" 2>/dev/null || true
            echo "  🔄 已发送 SIGHUP 触发证书热加载 (PID=$GW_PID)"
        fi
    fi
fi

echo ""
echo "✅ 证书轮换完成"

# 验证新证书有效期
echo ""
echo "--- 轮换后验证 ---"
check_cert_expiry "$CERT_DIR/gateway.crt" "新 Gateway 证书"
