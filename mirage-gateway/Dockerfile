FROM golang:1.25 AS gobuilder

# 安装 eBPF 编译依赖
RUN apt update && apt install -y \
    clang \
    llvm \
    libbpf-dev \
    linux-headers-generic \
    make \
    && rm -rf /var/lib/apt/lists/*

ENV CGO_ENABLED=1

WORKDIR /build

# 先复制 mirage-proto（共享依赖）
COPY mirage-proto/ /mirage-proto/

# 复制 mirage-gateway 源码
COPY mirage-gateway/ .

# 修正 Docker 构建中的 replace 路径
RUN sed -i 's|=> ../mirage-proto|=> /mirage-proto|' go.mod

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

COPY --from=gobuilder /build/bin/mirage-gateway /app/
COPY --from=gobuilder /build/bpf/*.o /app/bpf/
COPY --from=gobuilder /build/configs/gateway.yaml /app/configs/

ENTRYPOINT ["/app/mirage-gateway"]
CMD ["-config", "configs/gateway.yaml"]
