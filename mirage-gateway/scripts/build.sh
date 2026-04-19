#!/bin/bash
# Mirage-Gateway 编译脚本

set -e

echo "🚀 开始编译 Mirage-Gateway..."

# 检查环境
echo "📋 检查编译环境..."

if ! command -v clang &> /dev/null; then
    echo "❌ clang 未安装"
    echo "   Ubuntu/Debian: sudo apt install clang llvm"
    echo "   CentOS/RHEL: sudo yum install clang llvm"
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "❌ Go 未安装"
    echo "   Ubuntu/Debian: sudo apt install golang-1.21"
    exit 1
fi

echo "✅ clang: $(clang --version | head -n1)"
echo "✅ Go: $(go version)"

# 创建输出目录
mkdir -p bin bpf

# 编译 eBPF 程序
echo ""
echo "🔨 编译 C 数据面（eBPF）..."
clang -O2 -target bpf -c bpf/jitter.c -o bpf/jitter.o \
    -I/usr/include \
    -I/usr/include/x86_64-linux-gnu

if [ $? -eq 0 ]; then
    echo "✅ eBPF 编译成功: bpf/jitter.o"
else
    echo "❌ eBPF 编译失败"
    exit 1
fi

# 下载 Go 依赖
echo ""
echo "📦 下载 Go 依赖..."
go mod download

# 编译 Go 程序
echo ""
echo "🔨 编译 Go 控制面..."
CGO_ENABLED=1 go build -o bin/mirage-gateway cmd/gateway/main.go

if [ $? -eq 0 ]; then
    echo "✅ Go 编译成功: bin/mirage-gateway"
else
    echo "❌ Go 编译失败"
    exit 1
fi

# 验证
echo ""
echo "🔍 验证编译产物..."
ls -lh bin/mirage-gateway
ls -lh bpf/jitter.o

echo ""
echo "✅ 编译完成！"
echo ""
echo "运行命令："
echo "  sudo ./bin/mirage-gateway -iface eth0 -defense 20"
