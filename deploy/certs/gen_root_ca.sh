#!/bin/bash
# 生成自签名 Root CA（RSA 4096，10 年有效期）
# 幂等：已存在则跳过
set -euo pipefail

CA_DIR="${1:-/etc/mirage/certs}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

mkdir -p "$CA_DIR"

if [ -f "$CA_DIR/ca.key" ] && [ -f "$CA_DIR/ca.crt" ]; then
    echo "[INFO] Root CA 已存在，跳过生成: $CA_DIR/ca.key"
    exit 0
fi

echo "[INFO] 生成 Root CA..."

openssl genrsa -out "$CA_DIR/ca.key" 4096

openssl req -new -x509 \
    -key "$CA_DIR/ca.key" \
    -out "$CA_DIR/ca.crt" \
    -days 3650 \
    -subj "/CN=Mirage Root CA/O=Mirage" \
    -extensions v3_ca \
    -config "$SCRIPT_DIR/openssl.cnf"

chmod 600 "$CA_DIR/ca.key"
chmod 644 "$CA_DIR/ca.crt"

echo "[INFO] Root CA 已生成: $CA_DIR/ca.crt"
