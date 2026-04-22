#!/bin/bash
# cert-rotate.sh - mTLS 证书轮换脚本
# 用途：检查 Gateway/OS 证书有效期，到期前自动轮换
# 用法：sudo bash cert-rotate.sh [--check-only] [--days-before 30] [--cert-dir /var/mirage/certs]
# 建议 cron: 0 3 * * * /opt/mirage/scripts/cert-rotate.sh >> /var/log/mirage-cert-rotate.log 2>&1
#
# 签发路径说明：
#   生产环境：设置 OS_CERT_API 指向 OS 证书签发端点（POST /internal/cert/sign）
#   开发环境：设置 CERT_MODE=local 使用本地 CA key 签发
#   默认行为：CERT_MODE=api，必须配置 OS_CERT_API
#
# 证书目录约定：
#   /var/mirage/certs/ca.crt      - CA 证书
#   /var/mirage/certs/ca.key      - CA 私钥（仅开发环境本地签发时需要）
#   /var/mirage/certs/gateway.crt - Gateway 叶子证书
#   /var/mirage/certs/gateway.key - Gateway 私钥
#   /var/mirage/certs/os.crt      - OS 节点证书
#   /var/mirage/certs/os.key      - OS 节点私钥

set -e

CHECK_ONLY=0
DAYS_BEFORE=30
CERT_DIR="/var/mirage/certs"
CA_DIR="/var/mirage/certs"
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

# 签发路径选择（唯一路径，不允许口径并存）
# 生产环境：必须使用 OS 证书签发 API（POST /internal/cert/sign）
# 开发环境：可使用本地 CA key（需显式设置 CERT_MODE=local）
OS_CERT_API="${OS_CERT_API:-}"
CERT_MODE="${CERT_MODE:-api}"
HMAC_SECRET="${HMAC_SECRET:-}"

if [ "$CERT_MODE" = "local" ]; then
    if [ ! -f "$CA_DIR/ca.key" ]; then
        echo "❌ CERT_MODE=local 但 CA 私钥不存在: $CA_DIR/ca.key"
        exit 2
    fi
    echo "  ⚠️  使用本地 CA 签发（仅限开发环境）"
elif [ -n "$OS_CERT_API" ]; then
    echo "  使用 OS 证书签发 API: $OS_CERT_API"
else
    echo "❌ 未配置签发路径"
    echo "   生产环境: 设置 OS_CERT_API=https://mirage-os:3000/internal/cert/sign"
    echo "   开发环境: 设置 CERT_MODE=local"
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

if [ "$CERT_MODE" = "local" ]; then
    # ─── 本地 CA 签发（仅开发环境） ───
    SERIAL=$(date +%s%N | sha256sum | head -c 16)
    openssl x509 -req \
        -in "$CERT_DIR/gateway.csr" \
        -CA "$CA_DIR/ca.crt" \
        -CAkey "$CA_DIR/ca.key" \
        -set_serial "0x${SERIAL}" \
        -days 3 \
        -sha256 \
        -extfile <(printf "subjectAltName=DNS:mirage-gateway,DNS:localhost,IP:127.0.0.1\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=clientAuth,serverAuth") \
        -out "$CERT_DIR/gateway.crt.new" \
        2>/dev/null
else
    # ─── OS API 签发（生产路径） ───
    CSR_PEM=$(cat "$CERT_DIR/gateway.csr")
    PAYLOAD=$(printf '{"csr":"%s","gatewayId":"%s"}' "$(echo "$CSR_PEM" | sed ':a;N;$!ba;s/\n/\\n/g')" "$GATEWAY_ID")

    HMAC_HEADERS=""
    if [ -n "$HMAC_SECRET" ]; then
        TIMESTAMP=$(date +%s)
        HMAC_SIG=$(printf '%s' "${TIMESTAMP}:${PAYLOAD}" | openssl dgst -sha256 -hmac "$HMAC_SECRET" -binary | xxd -p -c 256)
        HMAC_HEADERS="-H \"X-HMAC-Timestamp: ${TIMESTAMP}\" -H \"X-HMAC-Signature: ${HMAC_SIG}\""
    fi

    HTTP_RESPONSE=$(eval curl -s -w "\n%{http_code}" \
        --cacert "$CERT_DIR/ca.crt" \
        --cert "$CERT_DIR/gateway.crt" \
        --key "$CERT_DIR/gateway.key" \
        -X POST "$OS_CERT_API" \
        -H "Content-Type: application/json" \
        $HMAC_HEADERS \
        -d "'$PAYLOAD'" \
        2>/dev/null)

    HTTP_CODE=$(echo "$HTTP_RESPONSE" | tail -1)
    HTTP_BODY=$(echo "$HTTP_RESPONSE" | sed '$d')

    if [ "$HTTP_CODE" != "201" ] && [ "$HTTP_CODE" != "200" ]; then
        echo "  ❌ OS API 签发失败 (HTTP $HTTP_CODE): $HTTP_BODY"
        rm -f "$CERT_DIR/gateway.key.new" "$CERT_DIR/gateway.csr"
        exit 2
    fi

    # 提取证书 PEM
    echo "$HTTP_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['certificate'])" \
        > "$CERT_DIR/gateway.crt.new" 2>/dev/null \
        || echo "$HTTP_BODY" | jq -r '.certificate' > "$CERT_DIR/gateway.crt.new" 2>/dev/null

    if [ ! -s "$CERT_DIR/gateway.crt.new" ]; then
        echo "  ❌ 无法从 API 响应中提取证书"
        rm -f "$CERT_DIR/gateway.key.new" "$CERT_DIR/gateway.csr"
        exit 2
    fi
fi

# 验证新证书
CA_VERIFY_FILE="$CA_DIR/ca.crt"
if [ ! -f "$CA_VERIFY_FILE" ] && [ -f "$CERT_DIR/ca.crt" ]; then
    CA_VERIFY_FILE="$CERT_DIR/ca.crt"
fi
if openssl verify -CAfile "$CA_VERIFY_FILE" "$CERT_DIR/gateway.crt.new" >/dev/null 2>&1; then
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
