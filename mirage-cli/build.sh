#!/bin/bash
# Mirage CLI 编译脚本 - 注入版本元数据

VERSION="1.0.0"
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S UTC')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS="-s -w \
  -X 'main.Version=${VERSION}' \
  -X 'main.BuildTime=${BUILD_TIME}' \
  -X 'main.GitCommit=${GIT_COMMIT}'"

echo "Building Mirage CLI v${VERSION}..."
echo "  Build Time: ${BUILD_TIME}"
echo "  Git Commit: ${GIT_COMMIT}"

# Linux AMD64
GOOS=linux GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-linux-amd64 main.go

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-linux-arm64 main.go

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-windows-amd64.exe main.go

# macOS AMD64
GOOS=darwin GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-darwin-amd64 main.go

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o bin/mirage-cli-darwin-arm64 main.go

echo "Build complete. Binaries in bin/"
ls -la bin/
