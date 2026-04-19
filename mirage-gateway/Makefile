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

.PHONY: all clean bpf go install test

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
