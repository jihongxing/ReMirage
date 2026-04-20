#!/usr/bin/env bash
# mirage-proto 代码生成脚本
# 在 Linux 服务器上执行
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# 1. 安装 protoc（如果不存在）
if ! command -v protoc &>/dev/null; then
    echo "📦 安装 protoc..."
    PROTOC_VERSION="28.0"
    curl -sLO "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-x86_64.zip"
    unzip -qo "protoc-${PROTOC_VERSION}-linux-x86_64.zip" -d /usr/local
    rm -f "protoc-${PROTOC_VERSION}-linux-x86_64.zip"
    echo "✅ protoc $(protoc --version)"
fi

# 2. 安装 Go 插件
echo "📦 安装 protoc-gen-go / protoc-gen-go-grpc..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

export PATH="$(go env GOPATH)/bin:$PATH"

# 3. 生成代码
echo "🔨 生成 proto 代码..."
mkdir -p gen
protoc --go_out=gen --go_opt=paths=source_relative \
       --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
       mirage.proto

echo "✅ 生成完成: gen/mirage.pb.go, gen/mirage_grpc.pb.go"

# 4. 验证
echo "🔍 验证编译..."
go build ./gen/...
echo "✅ 编译通过"
