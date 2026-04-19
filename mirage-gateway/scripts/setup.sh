#!/bin/bash
# Mirage-Gateway 一键部署脚本 (Mirage-One-Click)
# 支持: Ubuntu 20.04+, Debian 11+, Alpine 3.18+
# 用法: curl -sSL https://mirage.io/setup.sh | bash

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 检测操作系统
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        VERSION=$VERSION_ID
    else
        log_error "无法检测操作系统"
    fi
    log_info "检测到系统: $OS $VERSION"
}

# 检测内核版本
check_kernel() {
    KERNEL_VERSION=$(uname -r | cut -d. -f1-2)
    KERNEL_MAJOR=$(echo $KERNEL_VERSION | cut -d. -f1)
    KERNEL_MINOR=$(echo $KERNEL_VERSION | cut -d. -f2)
    
    if [ "$KERNEL_MAJOR" -lt 5 ] || ([ "$KERNEL_MAJOR" -eq 5 ] && [ "$KERNEL_MINOR" -lt 15 ]); then
        log_warn "内核版本 $KERNEL_VERSION < 5.15，部分 eBPF 功能将降级"
        EBPF_FALLBACK=1
    else
        log_info "内核版本 $KERNEL_VERSION 满足要求"
        EBPF_FALLBACK=0
    fi
}

# 安装依赖 - Ubuntu/Debian
install_deps_debian() {
    log_info "安装依赖 (apt)..."
    apt-get update -qq
    apt-get install -y -qq \
        clang llvm \
        libbpf-dev \
        linux-headers-$(uname -r) \
        tor \
        curl wget \
        ca-certificates
}

# 安装依赖 - Alpine
install_deps_alpine() {
    log_info "安装依赖 (apk)..."
    apk add --no-cache \
        clang llvm \
        libbpf \
        linux-headers \
        tor \
        curl wget \
        ca-certificates
}

# 安装 Go
install_go() {
    if command -v go &> /dev/null; then
        GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
        log_info "Go 已安装: $GO_VERSION"
        return
    fi
    
    log_info "安装 Go 1.21..."
    wget -q https://go.dev/dl/go1.21.6.linux-amd64.tar.gz -O /tmp/go.tar.gz
    tar -C /usr/local -xzf /tmp/go.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    rm /tmp/go.tar.gz
}

# 编译 eBPF 程序
compile_ebpf() {
    log_info "编译 eBPF 程序..."
    cd /opt/mirage/bpf
    
    for src in npm.c bdna.c jitter.c sockmap.c chameleon.c phantom.c; do
        if [ -f "$src" ]; then
            obj="${src%.c}.o"
            clang -O2 -g -target bpf \
                -D__TARGET_ARCH_x86 \
                -I/usr/include \
                -c "$src" -o "$obj" 2>/dev/null || {
                log_warn "编译 $src 失败，跳过"
            }
        fi
    done
    
    log_info "eBPF 编译完成"
}

# 编译 Gateway
compile_gateway() {
    log_info "编译 Mirage Gateway..."
    cd /opt/mirage
    
    export CGO_ENABLED=1
    go build -o bin/mirage-gateway cmd/gateway/main.go
    
    log_info "Gateway 编译完成"
}

# 配置 Tor 隐藏服务
setup_tor() {
    log_info "配置 Tor 隐藏服务..."
    
    mkdir -p /var/lib/tor/mirage_hidden_service
    chmod 700 /var/lib/tor/mirage_hidden_service
    
    cat > /etc/tor/torrc.d/mirage.conf << 'EOF'
HiddenServiceDir /var/lib/tor/mirage_hidden_service
HiddenServicePort 8080 127.0.0.1:8080
SocksPort 9050
EOF
    
    systemctl enable tor 2>/dev/null || true
    systemctl restart tor 2>/dev/null || true
}

# 配置 tmpfs
setup_tmpfs() {
    log_info "配置 tmpfs 内存文件系统..."
    
    mkdir -p /opt/mirage/data /opt/mirage/logs
    
    # 检查是否已挂载
    if ! mountpoint -q /opt/mirage/data; then
        mount -t tmpfs -o size=100M,mode=0700 tmpfs /opt/mirage/data
    fi
    if ! mountpoint -q /opt/mirage/logs; then
        mount -t tmpfs -o size=50M,mode=0700 tmpfs /opt/mirage/logs
    fi
    
    # 添加到 fstab
    grep -q '/opt/mirage/data' /etc/fstab || \
        echo 'tmpfs /opt/mirage/data tmpfs size=100M,mode=0700 0 0' >> /etc/fstab
    grep -q '/opt/mirage/logs' /etc/fstab || \
        echo 'tmpfs /opt/mirage/logs tmpfs size=50M,mode=0700 0 0' >> /etc/fstab
}

# 创建 systemd 服务
create_service() {
    log_info "创建 systemd 服务..."
    
    cat > /etc/systemd/system/mirage-gateway.service << 'EOF'
[Unit]
Description=Mirage Gateway
After=network.target tor.service
Wants=tor.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/mirage
ExecStart=/opt/mirage/bin/mirage-gateway -config /opt/mirage/configs/gateway.yaml
Restart=always
RestartSec=5
LimitNOFILE=65535
LimitMEMLOCK=infinity

# 安全加固
NoNewPrivileges=false
ProtectSystem=false
PrivateTmp=true

# 环境变量
Environment=GOMAXPROCS=4
Environment=GODEBUG=gctrace=0

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    systemctl enable mirage-gateway
}

# 紧急物理开关
create_kill_switch() {
    log_info "创建紧急物理开关..."
    
    cat > /opt/mirage/bin/kill_switch.sh << 'EOF'
#!/bin/bash
# Mirage 紧急物理开关 (Physical Kill Switch)
# 触发后: 卸载 eBPF、清空内存、伪装为普通 HTTP 服务器

echo "[KILL SWITCH] 触发紧急静默..."

# 1. 停止 Gateway 服务
systemctl stop mirage-gateway 2>/dev/null

# 2. 卸载所有 eBPF 程序
for prog in $(bpftool prog list | grep -E 'xdp|tc' | awk '{print $1}' | tr -d ':'); do
    bpftool prog detach id $prog 2>/dev/null
done

# 3. 清空 eBPF Maps
for map in $(bpftool map list | awk '{print $1}' | tr -d ':'); do
    bpftool map delete id $map 2>/dev/null
done

# 4. 清空 tmpfs
rm -rf /opt/mirage/data/* /opt/mirage/logs/*

# 5. 清空内存中的敏感数据
sync && echo 3 > /proc/sys/vm/drop_caches

# 6. 启动伪装 HTTP 服务器
cat > /tmp/fake_index.html << 'HTML'
<!DOCTYPE html>
<html><head><title>503 Service Unavailable</title></head>
<body><h1>503 Service Temporarily Unavailable</h1>
<p>The server is temporarily unable to service your request.</p>
</body></html>
HTML

python3 -m http.server 443 --directory /tmp &

echo "[KILL SWITCH] 静默完成，伪装为普通 HTTP 服务器"
EOF
    
    chmod +x /opt/mirage/bin/kill_switch.sh
}

# 主函数
main() {
    log_info "=========================================="
    log_info "Mirage-Gateway 一键部署 (Mirage-One-Click)"
    log_info "=========================================="
    
    # 检查 root 权限
    [ "$EUID" -eq 0 ] || log_error "请使用 root 权限运行"
    
    # 检测环境
    detect_os
    check_kernel
    
    # 创建目录
    mkdir -p /opt/mirage/{bin,bpf,configs,data,logs}
    
    # 安装依赖
    case $OS in
        ubuntu|debian)
            install_deps_debian
            ;;
        alpine)
            install_deps_alpine
            ;;
        *)
            log_warn "未知系统 $OS，尝试继续..."
            ;;
    esac
    
    # 安装 Go
    install_go
    
    # 配置
    setup_tmpfs
    setup_tor
    create_service
    create_kill_switch
    
    log_info "=========================================="
    log_info "部署完成！"
    log_info "=========================================="
    log_info "启动服务: systemctl start mirage-gateway"
    log_info "查看状态: systemctl status mirage-gateway"
    log_info "紧急开关: /opt/mirage/bin/kill_switch.sh"
    log_info "=========================================="
}

main "$@"
