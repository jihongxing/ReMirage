# 部署基线检查清单 (Baseline Checklist)

> 本文档为每个部署等级列出可执行的检查项。
> 每项包含：检查项名称、检查命令或验证方法、预期结果、对应部署等级、自动化程度。
> 配合 `deploy/scripts/drill-m8-baseline.sh` 使用。

## 一、RAM_Shield 检查项

### 1.1 mlock 生效检查

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `grep VmLck /proc/<pid>/status` |
| 预期结果 | `VmLck` 字段非零（表示内存已锁定） |
| 自动化程度 | 可脚本化（需知道 Gateway 进程 PID） |
| 环境依赖 | Linux（`/proc` 文件系统） |

### 1.2 Core dump 禁用检查

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `ulimit -c`（进程内）或 `cat /proc/sys/kernel/core_pattern` |
| 预期结果 | `ulimit -c` 为 `0`，或 `core_pattern` 为空/指向 `/dev/null` |
| 自动化程度 | 可脚本化 |
| 环境依赖 | Linux |

### 1.3 Swap 使用量检查

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `grep SwapTotal /proc/meminfo` 和 `swapon --show` |
| 预期结果 | `SwapTotal` 为 0 或 `swapon --show` 输出为空 |
| 自动化程度 | 可脚本化 |
| 环境依赖 | Linux |

## 二、证书配置检查项

### 2.1 证书存储路径在 tmpfs

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `mount \| grep tmpfs \| grep certs` |
| 预期结果 | 证书目录（`/var/mirage/certs`）挂载在 tmpfs 上 |
| 自动化程度 | 可脚本化 |
| 环境依赖 | 容器环境（需 tmpfs 挂载） |

### 2.2 证书有效期符合等级要求

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened（≤72h）, Extreme Stealth（≤72h，候选强化 ≤24h） |
| 检查命令 | `openssl x509 -enddate -noout -in /var/mirage/certs/gateway.crt` |
| 预期结果 | 证书剩余有效期不超过等级要求 |
| 自动化程度 | 可脚本化 |
| 环境依赖 | 需已部署证书 |

### 2.3 CA 私钥不在 Gateway 节点

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `test ! -f /var/mirage/certs/ca.key && test ! -f /etc/mirage/certs/ca.key` |
| 预期结果 | Gateway 节点上不存在 `ca.key` 文件 |
| 自动化程度 | 可脚本化 |
| 环境依赖 | 无 |

## 三、文件系统检查项

### 3.1 根文件系统只读

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `mount \| grep " / " \| grep "ro,"` |
| 预期结果 | 根文件系统挂载选项包含 `ro` |
| 自动化程度 | 可脚本化 |
| 环境依赖 | 容器环境（`read_only: true`） |

### 3.2 无非 tmpfs 可写挂载点

| 属性 | 值 |
|------|-----|
| 对应等级 | Extreme Stealth |
| 检查命令 | `mount \| grep -v "tmpfs" \| grep -v "ro," \| grep -v "proc\|sys\|dev"` |
| 预期结果 | 输出为空（除 tmpfs 外无可写挂载点） |
| 自动化程度 | 可脚本化 |
| 环境依赖 | 容器环境 |

### 3.3 Swap 分区禁用

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `swapon --show` |
| 预期结果 | 输出为空（无活跃 swap 分区） |
| 自动化程度 | 可脚本化 |
| 环境依赖 | Linux |

## 四、Emergency_Wipe 检查项

### 4.1 脚本存在且可执行

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `test -x deploy/scripts/emergency-wipe.sh` |
| 预期结果 | 脚本存在且有执行权限 |
| 自动化程度 | 可脚本化 |
| 环境依赖 | 无 |

### 4.2 依赖工具可用

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `which shred && which bpftool` |
| 预期结果 | `shred` 和 `bpftool` 均可找到 |
| 自动化程度 | 可脚本化 |
| 环境依赖 | Linux（需安装 coreutils 和 bpftool） |

### 4.3 Dry-run 等效验证

| 属性 | 值 |
|------|-----|
| 对应等级 | Hardened, Extreme Stealth |
| 检查命令 | `bash deploy/scripts/emergency-wipe.sh`（不带 `--confirm`，验证提示信息） |
| 预期结果 | 脚本输出安全提示并退出（退出码 1），不执行实际擦除 |
| 自动化程度 | 可脚本化 |
| 环境依赖 | 无 |

## 五、检查项汇总矩阵

| # | 检查项 | Default | Hardened | Extreme Stealth | 自动化程度 |
|---|--------|---------|----------|-----------------|-----------|
| 1.1 | mlock 生效 | — | ✓ | ✓ | 可脚本化 |
| 1.2 | Core dump 禁用 | — | ✓ | ✓ | 可脚本化 |
| 1.3 | Swap 使用量为零 | — | ✓ | ✓ | 可脚本化 |
| 2.1 | 证书在 tmpfs | — | ✓ | ✓ | 可脚本化 |
| 2.2 | 证书有效期 | — | ✓ (≤72h) | ✓ (≤72h) | 可脚本化 |
| 2.3 | CA 私钥不在 Gateway | — | ✓ | ✓ | 可脚本化 |
| 3.1 | 只读根文件系统 | — | ✓ | ✓ | 可脚本化 |
| 3.2 | 无非 tmpfs 可写挂载 | — | — | ✓ | 可脚本化 |
| 3.3 | Swap 分区禁用 | — | ✓ | ✓ | 可脚本化 |
| 4.1 | Emergency_Wipe 脚本存在 | — | ✓ | ✓ | 可脚本化 |
| 4.2 | 依赖工具可用 | — | ✓ | ✓ | 可脚本化 |
| 4.3 | Dry-run 验证 | — | ✓ | ✓ | 可脚本化 |
