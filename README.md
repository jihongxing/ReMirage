# Mirage Project

下一代网络隐身基础设施。基于 eBPF 数据面 + Go 控制面的二元架构，实现六维协议协同防御。

## 架构

```
┌─────────────────────────────────────────────────┐
│                  Mirage-OS                       │
│         (控制中心 · Raft 集群 · Web Console)      │
└────────────────────┬────────────────────────────┘
                     │ gRPC / mTLS
┌────────────────────▼────────────────────────────┐
│               Mirage-Gateway                     │
│  ┌───────────────────────────────────────────┐  │
│  │  Go 控制面                                 │  │
│  │  策略引擎 · G-Switch · G-Tunnel · Cortex   │  │
│  └───────────────────┬───────────────────────┘  │
│                      │ eBPF Map / Ring Buffer    │
│  ┌───────────────────▼───────────────────────┐  │
│  │  C 数据面 (eBPF XDP/TC)                    │  │
│  │  NPM · B-DNA · Jitter-Lite · VPC          │  │
│  └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────┐
│            Phantom Client                        │
│         (终端用户 · TUN 隧道 · G-Tunnel)         │
└─────────────────────────────────────────────────┘
```

## 六大协议

| 协议 | 维度 | 实现层 |
|------|------|--------|
| NPM | 空间伪装 | C (eBPF XDP) |
| B-DNA | 指纹拟态 | C (eBPF TC) |
| Jitter-Lite | 时域扰动 | C (eBPF TC) |
| VPC | 背景噪声 | C (eBPF TC) |
| G-Tunnel | 多路径传输 | Go + C |
| G-Switch | 域名转生 | Go (M.C.C.) |

## 系统要求

- Linux Kernel ≥ 5.15（生产）/ ≥ 4.19（降级模式）
- clang ≥ 14（eBPF 编译）
- Go ≥ 1.24
- Docker + Docker Compose（Mirage-OS 部署）

## 构建

```bash
# Gateway
cd mirage-gateway
make build

# Mirage-OS
cd mirage-os
docker-compose up -d

# 客户端
cd phantom-client
make build
```

## 项目结构

```
mirage-gateway/     # 融合网关（Go 控制面 + C 数据面）
mirage-os/          # 控制中心（Raft + API + Web）
phantom-client/     # 终端客户端（Windows/Linux）
mirage-cli/         # 运维 CLI 工具
sdk/                # 多语言 SDK
deploy/             # 部署脚本（Ansible/Certs/Chaos）
benchmarks/         # 性能基准测试
docs/               # 项目文档
```

## 许可证

**严格专有软件** — 禁止任何形式的使用、复制、借鉴或分发。详见 [LICENSE](LICENSE)。
