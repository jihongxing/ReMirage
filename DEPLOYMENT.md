# ReMirage 项目部署指南

## 🎯 项目概述

ReMirage 是一个高级流量隐匿与反审计系统，采用三层架构：
- **Mirage-Gateway**: eBPF内核级流量处理
- **Mirage-OS**: 分布式控制中心
- **Web Frontend**: React可视化界面

## 📊 当前完成度：**90%**

### ✅ 已完成组件
- ✅ **eBPF程序**: 7个核心协议完全实现并编译成功
- ✅ **Gateway主程序**: 523行完整实现，包含所有核心功能
- ✅ **后端服务**: API Gateway、WebSocket Gateway、Billing等
- ✅ **前端界面**: React + TypeScript，支持实时WebSocket通信
- ✅ **部署配置**: 6套完整的Docker Compose配置

## 🚀 快速开始

### 0. OpenCloudOS 无 UDP 单机部署

如果目标服务器没有可用 UDP，请不要使用 QUIC/H3 作为公开数据面。已验证的 OpenCloudOS 单机部署流程见：

- [OpenCloudOS 无 UDP 单机部署实录](docs/05-实施指南/OpenCloudOS-无UDP单机部署实录.md)

该流程会以 TCP/WSS 降级模式启动 Gateway，保持 `data_plane.enable_quic: false`，并使用 systemd 托管 `mirage-gateway-bridge` 与 `mirage-gateway`。

### 1. 开发环境部署

```bash
# 启动基础服务（PostgreSQL + Redis）
cd mirage-os
docker-compose -f docker-compose.dev.yml up -d

# 启动前端开发服务器
cd web
npm install
npm run dev
# 访问: http://localhost:3000
```

### 2. 完整系统部署

```bash
# 设置环境变量
export POSTGRES_PASSWORD=your_secure_password
export JWT_SECRET=your_jwt_secret

# 启动完整服务栈
cd mirage-os
docker-compose up -d

# 服务访问地址：
# - Web界面: http://localhost:8080
# - API服务: http://localhost:3000
# - Gateway Bridge: localhost:50051
```

## 🏗️ 部署架构

### 核心服务栈

| 服务 | 端口 | 功能 | 状态 |
|------|------|------|------|
| **Web Frontend** | 8080 | React用户界面 | ✅ 可用 |
| **API Server** | 3000 | REST API服务 | ✅ 可用 |
| **Gateway Bridge** | 50051 | gRPC通信桥接 | ✅ 可用 |
| **WebSocket Gateway** | 8080/ws | 实时通信 | ✅ 可用 |
| **PostgreSQL** | 5432 | 主数据库 | ✅ 可用 |
| **Redis** | 6379 | 缓存和消息队列 | ✅ 可用 |

### 高级部署选项

#### 1. 高安全Gateway部署
```bash
# 使用tmpfs内存文件系统，无磁盘痕迹
cd mirage-gateway
docker-compose -f docker-compose.tmpfs.yml up -d
```

特性：
- ✅ 只读根文件系统
- ✅ tmpfs内存挂载
- ✅ 特权模式支持eBPF
- ✅ 网络模式：host

#### 2. 分布式Raft集群
```bash
# 启动3节点Raft集群（冰岛、瑞士、新加坡）
cd mirage-os
docker-compose -f docker-compose.raft.yml up -d
```

节点配置：
- **冰岛节点**: 172.20.0.11:7001
- **瑞士节点**: 172.20.0.12:7002
- **新加坡节点**: 172.20.0.13:7003

#### 3. Monero支付服务
```bash
# 启动门罗币支付栈
cd mirage-os
export MONERO_RPC_PASSWORD=your_monero_password
docker-compose -f docker-compose.monero.yml up -d
```

服务：
- **Monerod**: 全节点同步（剪裁模式）
- **Wallet RPC**: 对账服务

## 🔧 配置说明

### Gateway配置 (configs/gateway.yaml)

```yaml
# 核心配置项
defense:
  level: 20                   # 防御强度 (10=经济, 20=平衡, 30=极限)
  auto_adjust: false          # 自动调节

# 协议配置
npm:
  enabled: true              # NPM流量伪装
  padding_rate: 20           # 填充率 20%

jitter:
  enabled: true              # Jitter-Lite时域扰动
  interval: 50ms             # 扰动区间

bdna:
  enabled: true              # B-DNA指纹识别
  ja4_database: "/etc/mirage/ja4.db"

# 安全配置
security:
  self_destruct: true        # 自毁机制
  ram_shield:
    enabled: true            # RAM保护
    disable_core_dump: true  # 禁用core dump
```

### 环境变量

```bash
# 数据库配置
POSTGRES_PASSWORD=your_secure_password
MIRAGE_DB_USER=mirage
MIRAGE_DB_NAME=mirage_os

# JWT配置
JWT_SECRET=your_jwt_secret_change_in_production

# TLS证书路径
MIRAGE_CERT_DIR=./certs

# Monero配置
MONERO_RPC_USER=mirage
MONERO_RPC_PASSWORD=your_monero_password
```

## 🛡️ 安全特性

### 已实现的安全机制

1. **内核级保护**
   - ✅ eBPF程序加载和验证
   - ✅ 透明代理（TPROXY）
   - ✅ 内存保护（RAM Shield）

2. **加密通信**
   - ✅ mTLS双向认证
   - ✅ 证书钉扎（Certificate Pinning）
   - ✅ gRPC安全通信

3. **反调试机制**
   - ✅ 反调试检测
   - ✅ 进程监控
   - ✅ 自毁机制

4. **数据保护**
   - ✅ 内存零化
   - ✅ 禁用core dump
   - ✅ tmpfs内存文件系统

## 📋 部署检查清单

### 部署前准备
- [ ] 确认Linux内核版本 >= 5.15（推荐）或 >= 4.19（最低）
- [ ] 安装Docker和Docker Compose
- [ ] 准备TLS证书文件
- [ ] 设置环境变量
- [ ] 配置防火墙规则

### 部署验证
- [ ] eBPF程序编译成功
- [ ] 所有服务健康检查通过
- [ ] 前端界面可访问
- [ ] WebSocket连接正常
- [ ] API接口响应正常
- [ ] 数据库连接正常

### 安全验证
- [ ] mTLS握手成功
- [ ] 证书验证通过
- [ ] 反调试机制激活
- [ ] 内存保护启用
- [ ] 日志仅内存存储

## 🔍 故障排查

### UDP 网络连通性（关键前置检查）

G-Tunnel 使用 QUIC 协议（基于 UDP）传输加密流量。**如果客户端所在网络封锁了 UDP 出站，整个隧道将无法建立。**

若部署环境确认没有 UDP，请直接使用 TCP/WSS 降级部署路径，并保持 `data_plane.enable_quic: false`。参考：[OpenCloudOS 无 UDP 单机部署实录](docs/05-实施指南/OpenCloudOS-无UDP单机部署实录.md)。

#### 症状

- Phantom Client 反复打印 `timeout: no recent network activity`
- 服务器侧 `tcpdump -i eth0 udp port <端口> -n` 完全无输出
- 但同一客户端 `ping` 服务器正常（ICMP 不受影响）

#### 根因

部分网络环境会完全封锁 UDP 出站流量：
- 国内部分 ISP（尤其企业宽带、校园网）
- 公司内网防火墙
- 某些家庭路由器的 UDP 限速/拦截策略
- VPN/代理软件残留的 WFP（Windows Filtering Platform）驱动规则

#### 诊断步骤

```bash
# 1. 服务器侧开启抓包
sudo tcpdump -i eth0 udp port 443 -n

# 2. 客户端发送测试 UDP 包（PowerShell）
$udp = New-Object System.Net.Sockets.UdpClient
$udp.Send([byte[]](1,2,3,4,5), 5, "<服务器IP>", 443)
$udp.Close()

# 3. 如果 tcpdump 无输出 → UDP 被封锁
# 4. 换高端口（51234）重试，确认是端口级还是协议级封锁
```

#### 服务器选择要求

| 检查项 | 要求 | 验证方式 |
|--------|------|---------|
| UDP 入站 | 安全组/防火墙放行目标端口 | `ss -ulnp \| grep <端口>` |
| UDP 出站 | 安全组允许 ALL 出站 | 安全组规则确认 |
| 无 UDP QoS | ISP 不限速/不拦截 UDP | tcpdump 抓包验证 |
| QUIC 友好 | 不做 UDP 深度包检测 | QUIC 握手成功 |

#### 客户端网络要求

| 检查项 | 风险 | 解决方案 |
|--------|------|---------|
| ISP 封锁 UDP | 所有 UDP 端口不可达 | 换 4G/5G 热点、或使用 TCP fallback |
| VPN 软件残留 | WFP 驱动拦截 UDP | 重启电脑清除；卸载残留驱动 |
| 企业防火墙 | 仅放行 TCP 80/443 | 需要 QUIC-over-TCP fallback 方案 |
| Windows 防火墙 | 拦截入站 UDP 回复 | 关闭或添加放行规则 |

#### 部署前必做的 UDP 连通性验证

```bash
# 在目标服务器上启动 UDP 回显测试
sudo ./bin/loopback-test 0.0.0.0:443

# 从客户端网络环境发送测试包
# 如果 loopback-test 打印 "🔗 客户端" → UDP 通路正常
# 如果无任何输出 → 该网络环境不支持 UDP，需要换网络或启用 TCP fallback
```

#### 多网卡环境的源 IP 绑定

当客户端存在虚拟网卡（如 Wintun/TAP）时，Windows 可能将 UDP 包的源 IP 绑定到虚拟网卡 IP，导致包在 NAT 或防火墙处被丢弃（Martian Packet）。

解决方案：QUIC 拨号时显式绑定物理网卡 IP：

```go
// 探测物理出站 IP
probeConn, _ := net.Dial("udp4", "8.8.8.8:53")
physicalIP := probeConn.LocalAddr().(*net.UDPAddr).IP
probeConn.Close()

// 绑定物理网卡
localAddr := &net.UDPAddr{IP: physicalIP, Port: 0}
udpConn, _ := net.ListenUDP("udp4", localAddr)

// 使用绑定的 socket 拨号
conn, _ := quic.Dial(ctx, udpConn, remoteAddr, tlsConf, quicConf)
```

### 常见问题

1. **eBPF加载失败**
   ```bash
   # 检查内核版本
   uname -r

   # 检查eBPF支持
   ls /sys/fs/bpf/

   # 查看内核日志
   dmesg | grep bpf
   ```

2. **权限不足**
   ```bash
   # 确保容器有必要权限
   docker run --privileged --cap-add=NET_ADMIN --cap-add=SYS_ADMIN
   ```

3. **网络连接问题**
   ```bash
   # 检查端口占用
   netstat -tlnp | grep :8080

   # 检查防火墙
   iptables -L
   ```

### 日志查看

```bash
# Gateway日志
docker logs mirage-gateway-tmpfs

# API服务日志
docker logs mirage-os_api-server_1

# 数据库日志
docker logs mirage-postgres
```

## 📈 性能优化

### 推荐配置

1. **系统优化**
   ```bash
   # 增加文件描述符限制
   echo "* soft nofile 65536" >> /etc/security/limits.conf
   echo "* hard nofile 65536" >> /etc/security/limits.conf

   # 优化网络参数
   echo "net.core.rmem_max = 16777216" >> /etc/sysctl.conf
   echo "net.core.wmem_max = 16777216" >> /etc/sysctl.conf
   ```

2. **Docker优化**
   ```yaml
   # 资源限制
   deploy:
     resources:
       limits:
         cpus: '2'
         memory: 512M
       reservations:
         cpus: '1'
         memory: 256M
   ```

## 🚀 生产部署建议

### 1. 基础部署（单机）
适合：测试环境、小规模部署
```bash
docker-compose up -d
```

### 2. 高安全部署（单机）
适合：高安全要求场景
```bash
docker-compose -f docker-compose.tmpfs.yml up -d
```

### 3. 分布式部署（多机）
适合：生产环境、高可用要求
```bash
# 在3个不同地理位置的服务器上部署
docker-compose -f docker-compose.raft.yml up -d
```

### 4. 完整商业部署
适合：商业化运营
```bash
# 启动所有服务栈
docker-compose up -d
docker-compose -f docker-compose.monero.yml up -d
docker-compose -f docker-compose.raft.yml up -d
```

## 📞 技术支持

如遇到部署问题，请检查：
1. 系统要求是否满足
2. 环境变量是否正确设置
3. 网络端口是否可用
4. 证书文件是否有效
5. 日志中的错误信息

---

**注意**: 本系统涉及高级网络技术，请确保在合规的环境中使用，并遵守当地法律法规。
