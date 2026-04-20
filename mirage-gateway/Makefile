# Mirage Gateway Makefile
# C 做内核数据面，Go 做控制面

# 编译器配置
CLANG := clang
GO := go
LLVM_STRIP := llvm-strip

# 编译选项
CLANG_FLAGS := -O2 -target bpf -g -D__TARGET_ARCH_x86 -Wall
INCLUDES := -I/usr/include -I/usr/include/x86_64-linux-gnu -I./bpf

# 目标文件
# TC 程序各自独立编译（通过 Map Pinning 共享 common.h 中的 Map）
# Sockmap 独立编译（不共享 common.h Map）
# NPM 独立编译（XDP 程序）
BPF_SOURCES := $(wildcard bpf/*.c)
BPF_OBJECTS := $(patsubst bpf/%.c,bpf/%.o,$(BPF_SOURCES))

# Go 配置
GO_BUILD_FLAGS := -ldflags="-s -w"
BINARY := bin/mirage-gateway

# Garble 混淆编译（O9 反逆向工程）
# 抹除所有 Type/Function Name/文件路径信息
# 安装: go install mvdan.cc/garble@latest
GARBLE := garble
GARBLE_FLAGS := -literals -tiny -seed=random

.PHONY: all clean bpf go go-obfuscated install test

all: bpf go

# 编译 eBPF 程序（C 数据面）
bpf: $(BPF_OBJECTS)

bpf/%.o: bpf/%.c bpf/common.h
	@echo "编译 eBPF 程序: $<"
	$(CLANG) $(CLANG_FLAGS) $(INCLUDES) -c $< -o $@

# 生成 Go 绑定
generate:
	@echo "生成 Go eBPF 绑定..."
	$(GO) generate ./pkg/ebpf/...

# 编译 Go 控制面
go: bpf
	@echo "编译 Go 控制面..."
	mkdir -p bin
	CGO_ENABLED=1 $(GO) build $(GO_BUILD_FLAGS) -o $(BINARY) ./cmd/gateway

# 编译 Go 控制面（混淆版 - 生产部署用）
# 使用 garble 抹除所有符号信息，防止逆向工程
# 编译产物中不包含任何函数名、类型名、文件路径
go-obfuscated: bpf
	@echo "编译 Go 控制面（garble 混淆）..."
	mkdir -p bin
	CGO_ENABLED=1 $(GARBLE) $(GARBLE_FLAGS) build $(GO_BUILD_FLAGS) -o $(BINARY) ./cmd/gateway
	@echo "✅ 混淆编译完成，验证符号信息已抹除:"
	@strings $(BINARY) | grep -c "mirage-gateway" || echo "  ✅ 无 mirage-gateway 字符串残留"
	@strings $(BINARY) | grep -c "github.com" || echo "  ✅ 无 github.com 路径残留"

# 安装依赖
install:
	@echo "安装依赖..."
	$(GO) mod download
	$(GO) install github.com/cilium/ebpf/cmd/bpf2go@latest

# 测试
test:
	@echo "运行测试..."
	$(GO) test -v ./...

# 清理
clean:
	@echo "清理编译产物..."
	rm -f bpf/*.o
	rm -rf bin/
	rm -f pkg/ebpf/*_bpfel.go pkg/ebpf/*_bpfeb.go

# 部署（需要 root 权限）
deploy: all
	@echo "部署 Mirage Gateway..."
	sudo $(BINARY) --config configs/gateway.yaml

# 检查内核版本
check-kernel:
	@echo "检查内核版本..."
	@uname -r
	@echo "需要内核版本 >= 5.15（推荐）或 >= 4.19（Fallback）"

# 验证 eBPF 程序
verify: bpf
	@echo "验证 eBPF 程序..."
	@for obj in $(BPF_OBJECTS); do \
		echo "验证 $$obj"; \
		llvm-objdump -S $$obj | head -20; \
	done

.DEFAULT_GOAL := all
