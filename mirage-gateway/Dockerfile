FROM ubuntu:22.04

# 安装编译依赖
RUN apt update && apt install -y \
    clang \
    llvm \
    libbpf-dev \
    linux-headers-generic \
    golang-1.21 \
    make \
    git \
    && rm -rf /var/lib/apt/lists/*

# 设置 Go 环境
ENV PATH="/usr/lib/go-1.21/bin:${PATH}"
ENV CGO_ENABLED=1

# 工作目录
WORKDIR /build

# 复制源码
COPY . .

# 下载 Go 依赖
RUN go mod download

# 编译 eBPF 程序（全部数据面）
RUN for f in bpf/*.c; do \
      clang -O2 -target bpf -g -c "$f" -o "${f%.c}.o" \
        -I/usr/include \
        -I/usr/include/x86_64-linux-gnu; \
    done

# 编译 Go 程序
RUN go build -o bin/mirage-gateway cmd/gateway/main.go

# 运行时镜像
FROM ubuntu:22.04

RUN apt update && apt install -y \
    libbpf0 \
    iproute2 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 复制编译产物
COPY --from=0 /build/bin/mirage-gateway /app/
COPY --from=0 /build/bpf/*.o /app/bpf/
COPY --from=0 /build/configs/gateway.yaml /app/configs/

# 暴露端口（如需要）
# EXPOSE 8080

# 启动命令
ENTRYPOINT ["/app/mirage-gateway"]
CMD ["-iface", "eth0", "-defense", "20"]
