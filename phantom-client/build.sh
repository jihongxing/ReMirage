#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-1.0.0}"
BUILD_TIME=$(date -u +%Y%m%d%H%M%S)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}"
DISGUISE_NAME="enterprise-sync"
OUT_DIR="bin"

mkdir -p "${OUT_DIR}"

echo "=== Building Phantom Client (disguised as ${DISGUISE_NAME}) ==="

# --- Windows amd64 ---
echo "[1/3] Windows amd64..."
# go-winres: inject Version Info + icon into .syso
if command -v go-winres &>/dev/null; then
    echo "  Generating Windows resources..."
    go-winres make --in assets/winres.json --out cmd/phantom/rsrc_windows_amd64.syso \
        --arch amd64 2>/dev/null || echo "  WARN: go-winres failed, skipping resource embedding"
fi

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
    -ldflags="${LDFLAGS} -X main.DisguiseName=${DISGUISE_NAME}" \
    -o "${OUT_DIR}/${DISGUISE_NAME}.exe" \
    cmd/phantom/main.go

# Windows code signing (optional)
if [ -n "${SIGN_CERT:-}" ]; then
    echo "  Signing Windows binary..."
    signtool sign /f "${SIGN_CERT}" /p "${SIGN_PASS:-}" \
        /tr http://timestamp.digicert.com /td sha256 \
        "${OUT_DIR}/${DISGUISE_NAME}.exe" 2>/dev/null \
        || echo "  WARN: Windows signing failed, continuing without signature"
fi

# --- macOS arm64 ---
echo "[2/3] macOS arm64..."
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
    -ldflags="${LDFLAGS} -X main.DisguiseName=${DISGUISE_NAME}" \
    -o "${OUT_DIR}/${DISGUISE_NAME}-darwin" \
    cmd/phantom/main.go

# macOS code signing (optional)
if [ -n "${APPLE_IDENTITY:-}" ]; then
    echo "  Signing macOS binary..."
    codesign --sign "${APPLE_IDENTITY}" --timestamp \
        "${OUT_DIR}/${DISGUISE_NAME}-darwin" 2>/dev/null \
        || echo "  WARN: macOS signing failed, continuing without signature"
fi

# --- Linux amd64 ---
echo "[3/3] Linux amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="${LDFLAGS} -X main.DisguiseName=${DISGUISE_NAME}" \
    -o "${OUT_DIR}/${DISGUISE_NAME}" \
    cmd/phantom/main.go

echo "=== Build complete ==="
ls -lh "${OUT_DIR}/"
