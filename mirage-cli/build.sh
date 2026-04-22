#!/bin/bash
# Mirage CLI 编译脚本

VERSION="2.0.0"
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS="-s -w \
  -X 'mirage-cli/cmd.Version=${VERSION}' \
  -X 'mirage-cli/cmd.BuildTime=${BUILD_TIME}' \
  -X 'mirage-cli/cmd.GitCommit=${GIT_COMMIT}'"

echo "Building Mirage CLI v${VERSION}..."
echo "  Build Time: ${BUILD_TIME}"
echo "  Git Commit: ${GIT_COMMIT}"

# Linux AMD64
GOOS=linux GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-linux-amd64 .

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-linux-arm64 .

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-windows-amd64.exe .

# macOS AMD64
GOOS=darwin GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-darwin-amd64 .

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-darwin-arm64 .

echo "Build complete."
ls -la bin/
