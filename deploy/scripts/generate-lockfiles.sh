#!/bin/bash
# 生成 package-lock.json（部署前必须执行）
# 确保 Ansible 部署链路中 npm ci 可用

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "[INFO] Generating package-lock.json for api-server..."
cd "$PROJECT_ROOT/mirage-os/api-server"
npm install --package-lock-only
echo "[OK] api-server/package-lock.json generated"

echo "[INFO] Generating package-lock.json for web..."
cd "$PROJECT_ROOT/mirage-os/web"
npm install --package-lock-only
echo "[OK] web/package-lock.json generated"

echo "[DONE] All lockfiles generated. Commit them to the repo."
