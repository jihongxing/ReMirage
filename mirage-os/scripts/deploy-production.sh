#!/bin/bash
# Mirage-OS V2 生产环境部署脚本
# 三节点 Raft 集群：冰岛(Leader)、瑞士、新加坡

set -e

# ============================================
# 配置
# ============================================
MIRAGE_VERSION="v2.0.0"
GEOIP_URL="https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=YOUR_LICENSE_KEY&suffix=tar.gz"

# 节点配置
declare -A NODES=(
    ["iceland"]="10.0.1.1"
    ["switzerland"]="10.0.2.1"
    ["singapore"]="10.0.3.1"
)

LEADER_NODE="iceland"

# ============================================
# 函数定义
# ============================================

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# 安全加固
harden_kernel() {
    log "🔒 应用内核安全加固..."
    
    cat >> /etc/sysctl.d/99-mirage-hardening.conf << 'EOF'
# Mirage 安全加固参数

# 禁止 eBPF 逃逸
kernel.unprivileged_bpf_disabled = 1
kernel.bpf_stats_enabled = 0

# 网络安全
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0
net.ipv4.conf.all.accept_source_route = 0
net.ipv4.conf.default.accept_source_route = 0

# 内存保护
kernel.randomize_va_space = 2
kernel.kptr_restrict = 2
kernel.dmesg_restrict = 1

# 文件系统保护
fs.protected_hardlinks = 1
fs.protected_symlinks = 1
fs.suid_dumpable = 0
EOF

    sysctl -p /etc/sysctl.d/99-mirage-hardening.conf
    log "✅ 内核加固完成"
}

# 配置防火墙
setup_firewall() {
    log "🔥 配置防火墙规则..."
    
    # 清空现有规则
    iptables -F
    iptables -X
    
    # 默认策略：拒绝所有入站
    iptables -P INPUT DROP
    iptables -P FORWARD DROP
    iptables -P OUTPUT ACCEPT
    
    # 允许本地回环
    iptables -A INPUT -i lo -j ACCEPT
    
    # 允许已建立的连接
    iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
    
    # 允许 SSH（仅限管理网段）
    iptables -A INPUT -p tcp --dport 22 -s 10.0.0.0/8 -j ACCEPT
    
    # 允许 gRPC（50051）
    iptables -A INPUT -p tcp --dport 50051 -j ACCEPT
    
    # 允许 WebSocket（8080）
    iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
    
    # 允许 Raft 集群通信（7000-7002）
    for node_ip in "${NODES[@]}"; do
        iptables -A INPUT -p tcp --dport 7000:7002 -s "$node_ip" -j ACCEPT
    done
    
    # 允许 Redis 集群通信（6379）
    for node_ip in "${NODES[@]}"; do
        iptables -A INPUT -p tcp --dport 6379 -s "$node_ip" -j ACCEPT
    done
    
    # 允许 PostgreSQL 集群通信（5432）
    for node_ip in "${NODES[@]}"; do
        iptables -A INPUT -p tcp --dport 5432 -s "$node_ip" -j ACCEPT
    done
    
    # 保存规则
    iptables-save > /etc/iptables/rules.v4
    
    log "✅ 防火墙配置完成"
}

# 下载 GeoIP 数据库
download_geoip() {
    log "🌍 下载 GeoIP 数据库..."
    
    GEOIP_DIR="/var/lib/mirage/geoip"
    mkdir -p "$GEOIP_DIR"
    
    if [ -f "$GEOIP_DIR/GeoLite2-City.mmdb" ]; then
        log "GeoIP 数据库已存在，跳过下载"
        return
    fi
    
    cd /tmp
    curl -sL "$GEOIP_URL" -o geoip.tar.gz
    tar -xzf geoip.tar.gz
    mv GeoLite2-City_*/GeoLite2-City.mmdb "$GEOIP_DIR/"
    rm -rf GeoLite2-City_* geoip.tar.gz
    
    log "✅ GeoIP 数据库已下载到 $GEOIP_DIR"
}

# 部署 Mirage-OS
deploy_mirage_os() {
    log "🚀 部署 Mirage-OS..."
    
    # 创建数据目录（tmpfs）
    mkdir -p /mnt/mirage-tmpfs
    mount -t tmpfs -o size=512M,mode=0700 tmpfs /mnt/mirage-tmpfs
    
    # 拉取镜像
    docker pull mirage/mirage-os:${MIRAGE_VERSION}
    
    # 获取当前节点角色
    HOSTNAME=$(hostname)
    NODE_IP="${NODES[$HOSTNAME]}"
    
    if [ "$HOSTNAME" == "$LEADER_NODE" ]; then
        RAFT_BOOTSTRAP="true"
    else
        RAFT_BOOTSTRAP="false"
    fi
    
    # 启动容器
    docker run -d \
        --name mirage-os \
        --restart unless-stopped \
        --network host \
        --privileged \
        -v /mnt/mirage-tmpfs:/data \
        -v /var/lib/mirage/geoip:/geoip:ro \
        -e NODE_ID="$HOSTNAME" \
        -e NODE_IP="$NODE_IP" \
        -e RAFT_BOOTSTRAP="$RAFT_BOOTSTRAP" \
        -e RAFT_PEERS="${NODES[iceland]},${NODES[switzerland]},${NODES[singapore]}" \
        -e GEOIP_DB_PATH="/geoip/GeoLite2-City.mmdb" \
        -e DB_HOST="localhost" \
        -e REDIS_ADDR="localhost:6379" \
        mirage/mirage-os:${MIRAGE_VERSION}
    
    log "✅ Mirage-OS 已启动"
}

# 部署 Mirage-Gateway
deploy_mirage_gateway() {
    log "🚀 部署 Mirage-Gateway..."
    
    docker pull mirage/mirage-gateway:${MIRAGE_VERSION}
    
    docker run -d \
        --name mirage-gateway \
        --restart unless-stopped \
        --network host \
        --privileged \
        --cap-add=SYS_ADMIN \
        --cap-add=NET_ADMIN \
        -v /sys/fs/bpf:/sys/fs/bpf \
        -v /mnt/mirage-tmpfs:/data \
        -e MIRAGE_OS_ADDR="${NODES[$LEADER_NODE]}:50051" \
        mirage/mirage-gateway:${MIRAGE_VERSION}
    
    log "✅ Mirage-Gateway 已启动"
}

# 健康检查
health_check() {
    log "🏥 执行健康检查..."
    
    # 检查 Mirage-OS
    if docker ps | grep -q mirage-os; then
        log "✅ Mirage-OS 运行中"
    else
        log "❌ Mirage-OS 未运行"
        exit 1
    fi
    
    # 检查 gRPC 端口
    if nc -z localhost 50051; then
        log "✅ gRPC 端口 50051 可达"
    else
        log "❌ gRPC 端口 50051 不可达"
        exit 1
    fi
    
    # 检查 WebSocket 端口
    if nc -z localhost 8080; then
        log "✅ WebSocket 端口 8080 可达"
    else
        log "❌ WebSocket 端口 8080 不可达"
        exit 1
    fi
    
    log "✅ 健康检查通过"
}

# ============================================
# 主流程
# ============================================

main() {
    log "═══════════════════════════════════════════════════════"
    log "🚀 Mirage-OS V2 生产环境部署"
    log "═══════════════════════════════════════════════════════"
    
    # 1. 内核加固
    harden_kernel
    
    # 2. 防火墙配置
    setup_firewall
    
    # 3. 下载 GeoIP
    download_geoip
    
    # 4. 部署 Mirage-OS
    deploy_mirage_os
    
    # 5. 部署 Mirage-Gateway
    deploy_mirage_gateway
    
    # 6. 健康检查
    sleep 10
    health_check
    
    log "═══════════════════════════════════════════════════════"
    log "✅ 部署完成"
    log "═══════════════════════════════════════════════════════"
}

# 执行
main "$@"
