#!/bin/bash
# 生成由 Root CA 签发的 OS 节点证书（RSA 2048，1 年）
set -euo pipefail

CA_DIR="${1:-/etc/mirage/certs}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ ! -f "$CA_DIR/ca.key" ] || [ ! -f "$CA_DIR/ca.crt" ]; then
    echo "[ERROR] Root CA 不存在，请先运行 gen_root_ca.sh"
    exit 1
fi

echo "[INFO] 生成 OS 节点证书..."

openssl genrsa -out "$CA_DIR/os.key" 2048

openssl req -new \
    -key "$CA_DIR/os.key" \
    -out "$CA_DIR/os.csr" \
    -subj "/CN=mirage-os/O=Mirage" \
    -config "$SCRIPT_DIR/openssl.cnf"

openssl x509 -req \
    -in "$CA_DIR/os.csr" \
    -CA "$CA_DIR/ca.crt" \
    -CAkey "$CA_DIR/ca.key" \
    -CAcreateserial \
    -out "$CA_DIR/os.crt" \
    -days 365 \
    -extensions v3_req \
    -extfile "$SCRIPT_DIR/openssl.cnf"

chmod 600 "$CA_DIR/os.key"
chmod 644 "$CA_DIR/os.crt"
rm -f "$CA_DIR/os.csr"

echo "[INFO] OS 证书已生成: $CA_DIR/os.crt"
