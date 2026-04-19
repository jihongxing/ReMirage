#!/bin/bash
# 生成由 Root CA 签发的 Gateway 节点证书（RSA 2048，1 年）
set -euo pipefail

NODE_ID="${1:?用法: $0 <NODE_ID> [CA_DIR]}"
CA_DIR="${2:-/etc/mirage/certs}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ ! -f "$CA_DIR/ca.key" ] || [ ! -f "$CA_DIR/ca.crt" ]; then
    echo "[ERROR] Root CA 不存在，请先运行 gen_root_ca.sh"
    exit 1
fi

echo "[INFO] 生成 Gateway 证书: NODE_ID=$NODE_ID"

openssl genrsa -out "$CA_DIR/gateway.key" 2048

openssl req -new \
    -key "$CA_DIR/gateway.key" \
    -out "$CA_DIR/gateway.csr" \
    -subj "/CN=mirage-gateway-${NODE_ID}/O=Mirage" \
    -config "$SCRIPT_DIR/openssl.cnf"

openssl x509 -req \
    -in "$CA_DIR/gateway.csr" \
    -CA "$CA_DIR/ca.crt" \
    -CAkey "$CA_DIR/ca.key" \
    -CAcreateserial \
    -out "$CA_DIR/gateway.crt" \
    -days 365 \
    -extensions v3_req \
    -extfile "$SCRIPT_DIR/openssl.cnf"

chmod 600 "$CA_DIR/gateway.key"
chmod 644 "$CA_DIR/gateway.crt"
rm -f "$CA_DIR/gateway.csr"

echo "[INFO] Gateway 证书已生成: $CA_DIR/gateway.crt"
