# 部署等级定义文档 (Deployment Tier Document)

> 本文档定义 Mirage Gateway 的三个部署等级：默认部署（Default）、加固部署（Hardened）、极限隐匿部署（Extreme Stealth）。
> 每个等级包含适用场景、安全边界、配置要求，以及当前代码支持状态。
> 本文档为 Phase 3 M8 里程碑产出，与 Baseline_Checklist、Runbook 保持对齐。

## 一、部署等级总览

| 等级 | 适用场景 | 安全边界 |
|------|----------|----------|
| Default（默认部署） | 受控网络内的开发/测试环境 | 基础安全，依赖宿主机隔离 |
| Hardened（加固部署） | 生产环境、预发布环境 | 内存隔离 + 短生命周期证书 + 只读文件系统 |
| Extreme Stealth（极限隐匿部署） | 高风险场景、对抗取证场景 | 无持久化 + 极短证书 + 自动擦除（部分当前不支持） |

## 二、配置项详细定义

### 2.1 RAM_Shield（mlock + core dump 禁用）

| 等级 | 要求 | 当前支持状态 | 证据锚点 |
|------|------|-------------|----------|
| Default | 可选 | 已支持 | `mirage-gateway/pkg/security/ram_shield.go` |
| Hardened | 强制启用 | 已支持 | `mirage-gateway/pkg/security/ram_shield.go` → `LockMemory()`, `DisableCoreDump()` |
| Extreme Stealth | 强制启用 | 已支持 | 同上 |

### 2.2 证书存储（磁盘 vs tmpfs）

| 等级 | 要求 | 当前支持状态 | 证据锚点 |
|------|------|-------------|----------|
| Default | 磁盘存储 | 已支持 | 标准文件系统路径 `/etc/mirage/certs/` |
| Hardened | tmpfs 存储 | 已支持 | `mirage-gateway/docker-compose.tmpfs.yml` → `tmpfs: /var/mirage/certs:size=16M,mode=0700` |
| Extreme Stealth | tmpfs 存储 | 已支持 | 同上 |

### 2.3 Swap 状态

| 等级 | 要求 | 当前支持状态 | 证据锚点 |
|------|------|-------------|----------|
| Default | 允许 | — | 无特殊配置 |
| Hardened | 禁用（`mem_swappiness: 0`） | 已支持 | `mirage-gateway/docker-compose.tmpfs.yml` → `mem_swappiness: 0` |
| Extreme Stealth | 禁用 | 已支持 | 同上 |

Swap 检测代码：`mirage-gateway/pkg/security/ram_shield.go` → `CheckSwapUsage()`

### 2.4 Cert_Rotate（证书轮换）

| 等级 | 要求 | 当前支持状态 | 证据锚点 |
|------|------|-------------|----------|
| Default | 可选 | 已支持 | `deploy/scripts/cert-rotate.sh` |
| Hardened | 启用，证书有效期 ≤72h | 已支持 | `deploy/certs/gen_gateway_cert.sh` → `-days 3`（72h）；`cert-rotate.sh` → `--days-before` 预警阈值 |
| Extreme Stealth | 启用，证书有效期 ≤72h（当前签发有效期 3 天） | 已支持 | 同上 |

**候选强化项**：证书 ≤24h 有效期。当前 `cert-rotate.sh` 的 `--days-before` 是轮转预警阈值，不是签发有效期；`gen_gateway_cert.sh` 签发有效期为 3 天（72h）。若需 ≤24h 签发策略，需新增签发参数。此项列为候选强化项，不作为当前 tier 的既有配置要求。

### 2.5 只读根文件系统

| 等级 | 要求 | 当前支持状态 | 证据锚点 |
|------|------|-------------|----------|
| Default | 否 | — | — |
| Hardened | 是 | 已支持 | `mirage-gateway/docker-compose.tmpfs.yml` → `read_only: true` |
| Extreme Stealth | 是 | 已支持 | 同上 |

### 2.6 日志持久化

| 等级 | 要求 | 当前支持状态 | 证据锚点 |
|------|------|-------------|----------|
| Default | 允许 | — | 标准日志配置 |
| Hardened | 允许 | — | — |
| Extreme Stealth | 不持久化（`max-file: 1` + 内存日志） | 已支持 | `mirage-gateway/docker-compose.tmpfs.yml` → `logging: max-file: "1"` |

### 2.7 Emergency_Wipe（紧急擦除）

| 等级 | 要求 | 当前支持状态 | 证据锚点 |
|------|------|-------------|----------|
| Default | 可选 | 已支持 | `deploy/scripts/emergency-wipe.sh` |
| Hardened | 预装（手动触发） | 已支持 | 同上（`--confirm` + "WIPE" 输入触发） |
| Extreme Stealth | 自动触发条件预配置 | **当前不支持** | 当前仅支持手动触发（`--confirm` + "WIPE" 输入）。自动触发条件（如检测到入侵自动执行）需新增代码 |

### 2.8 持久化卷

| 等级 | 要求 | 当前支持状态 | 证据锚点 |
|------|------|-------------|----------|
| Default | 允许 | — | 标准卷挂载 |
| Hardened | 最小化 | 需配置 | 需手动限制卷挂载范围 |
| Extreme Stealth | 无持久化卷 | 需配置 | `docker-compose.tmpfs.yml` 仅挂载 `configs/gateway.yaml:ro`，其余为 tmpfs |

### 2.9 密钥注入方式

| 等级 | 推荐注入路径 | 当前支持状态 | 证据锚点 |
|------|-------------|-------------|----------|
| Default | Compose 环境变量 | 已支持 | `deploy/runbooks/secret-injection.md` → "过渡方案" |
| Hardened | Docker Secrets 或 Vault | 需配置 | `deploy/runbooks/secret-injection.md` → "迁移路径" 第 2/3 步 |
| Extreme Stealth | Vault + tmpfs Volume Mount | 需配置 | `deploy/runbooks/secret-injection.md` → "流程（生产 K8s 部署）" |

## 三、当前代码支持状态汇总

| 配置项 | 支持状态 | 说明 |
|--------|----------|------|
| RAM_Shield (mlock + core dump) | 已支持 | `ram_shield.go` 提供完整 API |
| Swap 检测 | 已支持 | `ram_shield.go` → `CheckSwapUsage()` |
| tmpfs 证书存储 | 已支持 | `docker-compose.tmpfs.yml` 已配置 |
| Cert_Rotate (local + API) | 已支持 | `cert-rotate.sh` 支持本地 CA 和 OS API 两条路径 |
| 只读根文件系统 | 已支持 | `docker-compose.tmpfs.yml` → `read_only: true` |
| `mem_swappiness: 0` | 已支持 | `docker-compose.tmpfs.yml` 已配置 |
| Emergency_Wipe（手动） | 已支持 | `emergency-wipe.sh` 7 步焦土协议 |
| 日志 `max-file: 1` | 已支持 | `docker-compose.tmpfs.yml` → logging 配置 |
| 证书 ≤24h 有效期 | **候选强化项** | `--days-before` 是轮转预警阈值，非签发有效期；`gen_gateway_cert.sh` 签发 3 天；需新增签发策略 |
| Emergency_Wipe 自动触发 | **不支持** | 当前仅手动触发，需新增入侵检测联动代码 |
| Vault + tmpfs Volume Mount | 需配置 | `secret-injection.md` 描述了迁移路径，当前 Compose 用环境变量 |

**重要说明**：当前 `mirage-gateway/docker-compose.tmpfs.yml` 配置对应**加固部署（Hardened）**等级，不自动等于极限隐匿部署（Extreme Stealth）。极限隐匿部署在加固部署基础上还需要：Emergency_Wipe 自动触发、无持久化卷、日志不持久化等额外配置，其中部分当前不支持。

## 四、Runbook 交叉引用

### 4.1 Secret_Injection_Runbook（`deploy/runbooks/secret-injection.md`）

| 部署等级 | 推荐注入路径 | Runbook 对应章节 |
|----------|-------------|-----------------|
| Default | Compose 环境变量 | "过渡方案（Compose 部署）" |
| Hardened | Docker Secrets / Vault | "迁移路径" 第 2/3 步 |
| Extreme Stealth | Vault + tmpfs Volume Mount | "流程（生产 K8s 部署）" |

### 4.2 Least_Privilege_Runbook（`deploy/runbooks/least-privilege-model.md`）

所有部署等级均遵循权限矩阵定义：
- Gateway 节点：无对象存储/数据库权限，仅拉取镜像、读取自身密钥、写日志
- 各等级权限配置不超出矩阵定义

### 4.3 Node_Replacement_Runbook（`deploy/runbooks/compromised-node-replacement.md`）

| 部署等级 | 替换注意事项 |
|----------|-------------|
| Default | 标准流程（隔离 → 部署替换 → 验证） |
| Hardened | 证书 72h 自然过期，与 `compromised-node-replacement.md` 一致（"失陷节点证书最长 72h 后自然过期，无需手动吊销"） |
| Extreme Stealth | 证书 72h 过期 + Emergency_Wipe 优先执行。≤24h 证书有效期为候选强化项，当前签发脚本不支持 |

### 4.4 不一致处理规则

当本文档与 Runbook 存在不一致时：
- **密钥注入方式**：以 `deploy/runbooks/secret-injection.md` 为准
- **权限矩阵**：以 `deploy/runbooks/least-privilege-model.md` 为准
- **节点替换流程**：以 `deploy/runbooks/compromised-node-replacement.md` 为准
- **部署等级定义与配置要求**：以本文档为准

## 五、配置示例

### 5.1 加固部署（Hardened）配置示例

基于 `mirage-gateway/docker-compose.tmpfs.yml`：

```yaml
services:
  mirage-gateway:
    # 只读根文件系统
    read_only: true
    # 禁用 swap
    mem_swappiness: 0
    # tmpfs 挂载（证书仅存内存）
    tmpfs:
      - /var/mirage:size=256M,mode=0700
      - /var/mirage/certs:size=16M,mode=0700,uid=0,gid=0
      - /tmp:size=64M,mode=1777
    # 日志配置（允许持久化）
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
    environment:
      - MIRAGE_TMPFS_MODE=1
      - MIRAGE_NO_PERSIST=1
```

证书轮换（`deploy/scripts/cert-rotate.sh`）：
```bash
# 每日检查，30 天预警（证书有效期 72h，实际每 ~42h 触发轮换）
sudo bash cert-rotate.sh --days-before 1 --cert-dir /var/mirage/certs
```

### 5.2 极限隐匿部署（Extreme Stealth）配置示例

在加固部署基础上增加：

```yaml
services:
  mirage-gateway:
    read_only: true
    mem_swappiness: 0
    tmpfs:
      - /var/mirage:size=256M,mode=0700
      - /var/mirage/certs:size=16M,mode=0700,uid=0,gid=0
      - /tmp:size=64M,mode=1777
    # 日志不持久化
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "1"    # 仅保留 1 个日志文件
    environment:
      - MIRAGE_TMPFS_MODE=1
      - MIRAGE_NO_PERSIST=1
    # 无额外持久化卷（仅只读配置）
    volumes:
      - ./configs/gateway.yaml:/etc/mirage/gateway.yaml:ro
```

Emergency_Wipe 预装：
```bash
# 确保脚本可用
chmod +x deploy/scripts/emergency-wipe.sh
# 验证依赖工具
which shred bpftool
```

**当前不支持项**：
- Emergency_Wipe 自动触发条件预配置（需新增入侵检测联动代码）
- 证书 ≤24h 有效期签发策略（需新增 `gen_gateway_cert.sh` 签发参数）
