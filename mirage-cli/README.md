# Mirage CLI v2 使用说明

Mirage Gateway 管理工具 — 融合网关状态查询、隧道控制、认证签名、证书管理、策略调整、运维诊断。

## 安装

```bash
cd mirage-cli
go build -o mirage-cli .

# 或使用构建脚本（交叉编译全平台）
bash build.sh
```

## 全局参数

```
-g, --gateway string   Gateway 健康检查地址 (默认 "127.0.0.1:9090")
-o, --os string        Mirage-OS API 地址 (例: https://mirage-os:3000)
-c, --config string    配置文件路径 (默认 /etc/mirage/gateway.yaml)
    --json             JSON 格式输出
```

---

## 命令一览

### 基础命令（v1 延续）

| 命令 | 说明 |
|------|------|
| `version` | 显示版本信息 |
| `status` | 查询 Gateway 运行状态 |
| `tunnel status` | 查看 G-Tunnel 隧道状态 |
| `tunnel paths` | 列出所有传输路径 |
| `threat summary` | 威胁摘要 |
| `threat list` | 列出最近威胁事件 |
| `threat blacklist` | 查看当前黑名单 |
| `quota` | 配额与流量查询 |
| `config show` | 显示当前配置 |
| `config protocols` | 显示协议启用状态 |
| `config defense` | 显示防御策略参数 |
| `diag all` | 完整诊断报告 |
| `diag ebpf` | eBPF 程序诊断 |
| `diag conn` | 连接诊断 |
| `keygen` | 生成 Ed25519 密钥对 |
| `sign <challenge>` | 对挑战码进行硬件签名 |

### v2 新增命令

| 命令 | 说明 |
|------|------|
| `cert check` | 检查证书有效期 |
| `cert rotate` | 轮换 Gateway mTLS 证书 |
| `cert inspect [file]` | 查看证书详情（Subject/Issuer/SAN/有效期） |
| `backup create` | 创建 Gateway 状态备份（配置/证书指纹/eBPF/网络） |
| `backup list` | 列出已有备份 |
| `wipe` | 焦土协议 — 安全擦除所有 Mirage 痕迹 |
| `preflight` | eBPF 环境前置检查（内核/BPF/BTF/XDP/TC/memlock） |
| `tune check` | 检查系统参数（sysctl）是否已优化 |
| `tune apply` | 应用推荐系统参数 |
| `topology list` | 列出所有网关节点 |
| `topology ping` | 测试到各网关节点的延迟 |
| `strategy show` | 显示当前防御策略 |
| `strategy set --level N` | 调整防御等级 (0-30) |
| `strategy cost` | 查看防御成本分析 |
| `phantom status` | Phantom 欺骗引擎状态 |
| `phantom traps` | 列出蜜罐详情 |
| `phantom persona` | 查看当前伪装身份 |
| `log show` | 查看 Gateway 内存日志 |
| `log audit` | 查看命令审计日志 |
| `log stats` | 日志统计 |
| `health` | 深度健康巡检（全模块探测） |

---

## 典型使用场景

### 场景 1：首次部署

```bash
# 1. 环境检查
mirage-cli preflight

# 2. 系统调优
sudo mirage-cli tune apply

# 3. 生成密钥对
mirage-cli keygen

# 4. 验证 Gateway 连接
mirage-cli status -g <gateway_ip>:9090

# 5. 确认协议状态
mirage-cli config protocols
```

### 场景 2：日常监控

```bash
mirage-cli health                    # 全模块巡检
mirage-cli status                    # 快速状态
mirage-cli tunnel status             # 隧道连通性
mirage-cli quota                     # 配额剩余
mirage-cli strategy show             # 防御策略
```

### 场景 3：威胁排查

```bash
mirage-cli threat summary            # 威胁态势
mirage-cli threat list -n 50         # 最近事件
mirage-cli threat blacklist          # 黑名单
mirage-cli phantom status            # 欺骗引擎
mirage-cli log show --level error    # 错误日志
mirage-cli log audit                 # 审计日志
```

### 场景 4：证书运维

```bash
mirage-cli cert check                # 检查有效期
mirage-cli cert inspect              # 查看详情
sudo mirage-cli cert rotate          # 轮换证书
```

### 场景 5：故障诊断

```bash
mirage-cli diag all                  # 完整诊断
mirage-cli diag ebpf                 # eBPF 状态
mirage-cli topology list             # 网关拓扑
mirage-cli topology ping             # 节点延迟
```

### 场景 6：紧急响应

```bash
mirage-cli strategy set --level 30   # 极限防御
mirage-cli backup create             # 备份状态
sudo mirage-cli wipe                 # 焦土协议（不可逆）
```

---

## 退出码

| 退出码 | 含义 |
|--------|------|
| 0 | 成功 |
| 1 | 命令执行失败 |
