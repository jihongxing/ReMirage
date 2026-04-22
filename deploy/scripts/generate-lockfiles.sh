#!/bin/bash
# generate-lockfiles.sh - 生成发布 manifest
# 记录源码提交 hash、lockfile hash、产物 hash
set -euo pipefail

OUTPUT="${1:-release-manifest.json}"

GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_SHORT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

# Lockfile hashes
LOCKFILE_HASH="none"
if [ -f "mirage-os/api-server/package-lock.json" ]; then
    LOCKFILE_HASH=$(sha256sum mirage-os/api-server/package-lock.json | cut -d' ' -f1)
fi

GO_SUM_HASH="none"
if [ -f "mirage-gateway/go.sum" ]; then
    GO_SUM_HASH=$(sha256sum mirage-gateway/go.sum | cut -d' ' -f1)
fi

cat > "$OUTPUT" <<EOF
{
  "version": "v2.0.0-${GIT_SHORT}",
  "build_time": "${BUILD_TIME}",
  "git_commit": "${GIT_COMMIT}",
  "lockfile_hashes": {
    "api_server_package_lock": "${LOCKFILE_HASH}",
    "gateway_go_sum": "${GO_SUM_HASH}"
  },
  "binary_sha256": "",
  "signature": ""
}
EOF

echo "[INFO] Release manifest generated: $OUTPUT"
echo "  Git commit: $GIT_COMMIT"
echo "  Build time: $BUILD_TIME"
