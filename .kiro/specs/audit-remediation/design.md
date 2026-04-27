# 审计整改 Bugfix 设计

## Overview

Mirage Project 全项目上线审计（`docs/audit-report.md` v3）发现 3 项 P0 阻断项和 7 项 P1 必须修复项，当前发布状态为 `release_blocked`。本设计文档将 10 个审计发现形式化为 Bug Condition，定义修复策略、根因假设和验证方案，确保修复精准且不引入回归。

修复按审计报告建议的四批次推进：
1. 供应链与配置基线（F-P0-01, F-P0-02, F-P0-03）
2. 部署与密钥注入口径（F-P1-04, F-P1-05）
3. 代码与测试回归（F-P1-03, F-P1-06, F-P1-08）
4. 架构与日志治理（F-P1-01, F-P1-02）

## Glossary

- **Bug_Condition (C)**: 触发审计阻断的条件集合——涵盖供应链缺失、配置危险默认值、未鉴权服务、口径矛盾、竞态缺陷、编译回归缺失等
- **Property (P)**: 修复后的期望行为——扫描可执行、配置安全、服务鉴权、文档一致、测试稳定、编译可验证
- **Preservation**: 现有构建流水线、开发环境、Orchestrator 编排、P0 runtime 测试、chaos 测试、发布签名、证书轮转、wintun.dll 嵌入机制等必须保持不变
- **QuotaBucketManager**: `mirage-gateway/pkg/api/quota_bucket.go` 中按 user_id 隔离的配额桶管理器，使用 CAS 原子操作消费配额
- **Orchestrator**: `mirage-gateway/pkg/gtunnel/orchestrator.go` 中的多路径自适应调度器，替代 TransportManager 的唯一编排主链
- **TransportManager**: `mirage-gateway/pkg/gtunnel/transport.go` 中已标记 deprecated 的传输管理器，待收敛移除

## Bug Details

### Bug Condition

审计阻断由 10 个独立缺陷条件的并集触发。任一条件为真即构成发布阻断。

**Formal Specification:**
```
FUNCTION isBugCondition(finding)
  INPUT: finding of type AuditFinding
  OUTPUT: boolean

  // P0 阻断条件
  C1 := finding.id == "F-P0-01"
        AND NOT exists(securityScanScript)
        AND NOT exists(govulncheckResults)
        AND NOT exists(npmAuditResults)

  C2 := finding.id == "F-P0-02"
        AND fileContains("mirage-os/configs/config.yaml", "password: postgres")
        AND fileContains("mirage-os/configs/config.yaml", "change-this-in-production")
        AND NOT fileMarkedAsDevOnly("mirage-os/configs/config.yaml")

  C3 := finding.id == "F-P0-03"
        AND existsUntrackedBinaries(workDir, [".exe", ".dll"])
        AND NOT documented(wintunDLL.source, wintunDLL.version, wintunDLL.sha256)

  // P1 阻断条件
  C4 := finding.id == "F-P1-01"
        AND NOT exists(unifiedRedactPackage)
        AND logsMayContainSensitiveFields(["IP", "user_id", "token", "Authorization", "secret", "password"])

  C5 := finding.id == "F-P1-02"
        AND hasDirectCallsTo("TransportManager") IN productionCodePath
        AND NOT allCallsRoutedThrough("Orchestrator")

  C6 := finding.id == "F-P1-03"
        AND compileTest("compile_test.go").onlyChecks("L1Stats struct alignment")
        AND NOT compilesAnyBPFCFile()

  C7 := finding.id == "F-P1-04"
        AND redisConfig("deploy/docker-compose.os.yml").hasNoAuth()
        AND consumers(["gateway-bridge", "api-server"]).connectWithoutPassword()

  C8 := finding.id == "F-P1-05"
        AND compose("deploy/docker-compose.os.yml").usesEnvVarInjection(["POSTGRES_PASSWORD", "JWT_SECRET", "QUERY_HMAC_SECRET"])
        AND runbook("secret-injection.md").requires("K8s Secret / Vault Volume Mount")
        AND compose.method != runbook.method

  C9 := finding.id == "F-P1-06"
        AND testFlaky("TestQuotaBucket_IsolationTwoUsers", count=10)
        AND quotaBucket.Consume().CASLoop.missesExhaustedCallback(
              WHEN remaining == bytes  // 恰好耗尽场景
              AND CAS(remaining, 0) succeeds
              BUT Exhausted flag not set to 1
            )

  C10 := finding.id == "F-P1-08"
         AND exec("go test ./...", cwd="benchmarks/").exitCode != 0
         AND errorContains("go: updates to go.mod needed")

  RETURN C1 OR C2 OR C3 OR C4 OR C5 OR C6 OR C7 OR C8 OR C9 OR C10
END FUNCTION
```

### Examples

- **F-P0-01**: 在任意 Go 模块目录执行 `govulncheck ./...`，无脚本、无 CI 记录、无结果归档——阻断
- **F-P0-02**: `mirage-os/configs/config.yaml` 第 13 行 `password: postgres`，第 107 行 `jwt_secret: ${JWT_SECRET:change-this-in-production}`，文件无开发环境标注——阻断
- **F-P0-03**: `phantom-client/cmd/phantom/wintun.dll` 被 `go:embed` 嵌入交付物，但无来源版本/SHA256 记录——阻断
- **F-P1-04**: `deploy/docker-compose.os.yml` Redis 服务 `command: redis-server --appendonly yes` 无 `requirepass`，`gateway-bridge` 和 `api-server` 连接 `redis://redis:6379` 无密码——阻断
- **F-P1-05**: compose 用 `${MIRAGE_DB_PASSWORD}` 环境变量注入，runbook 要求"密钥不通过普通环境变量长期保存"——口径矛盾
- **F-P1-06**: `quota_bucket.go` Consume 方法 CAS 循环中，当 `remaining >= bytes` 且 CAS 成功将余额减至 0 时，未检查新余额是否为 0 来触发 `onExhausted` 回调——间歇性失败
- **F-P1-08**: `benchmarks/go.mod` 依赖 `mirage-gateway v0.0.0` 通过 replace 指令，但 `go.sum` 与实际依赖不同步——直接失败

## Expected Behavior

### Preservation Requirements

**Unchanged Behaviors:**
- Gateway/OS/Client/CLI 所有组件 `go build` + `go vet` 继续 EXIT_CODE=0
- `deploy/docker-compose.dev.yml` 开发环境正常启动不受影响
- Orchestrator 编排主链（路径调度、BBR v3、多路径传输）功能不变
- `tests/p0_runtime/` 全部测试继续通过
- `deploy/chaos/chaos_test.sh` 混沌测试继续正常运行
- `deploy/release/manifest.go` + `verify.go` Ed25519 签名验证流程不变
- `cert-rotate.sh` 和 `emergency-wipe.sh` 安全操作流程不变
- `phantom-client` 的 `go:embed` wintun.dll 嵌入机制不变（仅整理来源，不改嵌入路径）
- 非零正数配额用户的正常 API 调用和配额计算不变
- 生产环境 mTLS 强制模式继续拒绝非加密连接

**Scope:**
所有不涉及上述 10 个审计发现的功能路径应完全不受影响。包括：
- 现有 eBPF 数据面运行时行为（NPM/B-DNA/VPC/Jitter/G-Tunnel）
- G-Switch 域名转生协议
- SDK 各语言客户端
- Proto 契约定义
- 现有 Ansible 部署流程

## Hypothesized Root Cause

### 第一批：供应链与配置基线

1. **F-P0-01 依赖漏洞扫描缺失**: 项目从未建立统一的安全扫描流程，各组件独立开发但无人负责跨组件扫描自动化
2. **F-P0-02 配置危险默认值**: `mirage-os/configs/config.yaml` 最初作为开发配置创建，随项目演进未拆分开发/生产配置，`password: postgres` 和 `jwt_secret` 默认值从未清理
3. **F-P0-03 未追踪构建产物**: 开发过程中本地构建产物未及时清理，`.gitignore` 虽排除了 `*.exe`/`*.dll` 但工作目录残留；`wintun.dll` 作为 Windows 驱动依赖被手动复制到多个位置，未建立来源追溯机制

### 第二批：部署与密钥注入口径

4. **F-P1-04 Redis 未鉴权**: `docker-compose.os.yml` 最初为内网部署设计，假设网络隔离足够安全，未配置 Redis 密码；随着部署场景扩展，安全假设不再成立
5. **F-P1-05 密钥注入口径矛盾**: runbook 按 K8s/Vault 生产标准编写，但实际部署仍依赖 compose 环境变量注入，两条路径并行演进未统一

### 第三批：代码与测试回归

6. **F-P1-03 eBPF 编译回归不足**: `compile_test.go` 仅验证 `L1Stats` 结构体 9 个字段与 C 侧对齐（赋值 + 读取），未调用 `clang -target bpf` 编译任何 `.c` 文件。`mirage-gateway/bpf/` 下有 12 个 `.c` 文件但编译验证完全依赖手动或 Makefile
7. **F-P1-06 配额测试竞态**: `quota_bucket.go` 的 `Consume` 方法 CAS 循环存在逻辑缺陷——当 `remaining >= bytes` 且 CAS 成功将余额减至恰好 0 时，方法返回 `true` 但未检查新余额是否为 0，导致 `Exhausted` 标记未被设置，后续 `onExhausted` 回调不触发。具体代码路径：
   ```
   remaining := atomic.LoadUint64(&bucket.RemainingBytes)  // e.g. 100
   if remaining < bytes { ... }                             // 100 >= 100, 不进入
   if CAS(&bucket.RemainingBytes, 100, 0) {                // 成功，余额变 0
       return true                                          // 直接返回，未设置 Exhausted=1
   }
   ```
   下一次 Consume 调用时 `remaining < bytes` 才会触发回调，但如果测试在此之前检查 `IsExhausted`，则看到 `false`
8. **F-P1-08 benchmarks 不可运行**: `benchmarks/go.mod` 使用 `replace mirage-gateway => ../mirage-gateway` 指向本地模块，但 `go.sum` 未同步更新，`go test` 要求先执行 `go mod tidy`

### 第四批：架构与日志治理

9. **F-P1-01 日志脱敏缺失**: 项目使用标准 `log.Printf` 直接输出，未建立统一的日志中间件层。`SecureString` 类型（`ram_shield.go`）仅用于内存保护场景，未推广到日志输出
10. **F-P1-02 编排主链未收敛**: `TransportManager` 已在 `transport.go` 和 `chameleon_client.go` 中标记 `Deprecated`，但 `ConnectWithFallback`、`probeAndPromote`、`promoteToQUIC`、`recvLoop` 等方法仍存在且可被调用，source-of-truth-map 标记为"待收敛 S-01"

## Correctness Properties

Property 1: Bug Condition - 供应链安全扫描可执行

_For any_ 项目中的 Go 模块或 Node 项目，修复后 SHALL 存在统一扫描脚本，能对所有 Go 模块执行 `govulncheck ./...`、对 `sdk/js` 执行 `npm audit`，且扫描结果可归档追溯。

**Validates: Requirements 2.1**

Property 2: Bug Condition - 配置文件无危险默认值

_For any_ 配置文件路径 `mirage-os/configs/config.yaml`（或其替代），修复后 SHALL 不包含 `password: postgres` 或 `change-this-in-production` 等硬编码危险默认值，且文件明确标注为开发环境专用。

**Validates: Requirements 2.2**

Property 3: Bug Condition - 构建产物可追溯

_For any_ 工作目录中的 `.exe`/`.dll` 构建产物，修复后 SHALL 被清理或有明确来源记录；`wintun.dll` SHALL 有官方来源、版本号、下载地址和 SHA256 记录，且仅保留单一 authoritative 副本。

**Validates: Requirements 2.3**

Property 4: Bug Condition - Redis 服务鉴权

_For any_ 生产部署的 Redis 服务实例，修复后 SHALL 配置 `requirepass` 或 ACL 鉴权，消费方连接串包含密码，未授权连接被拒绝。

**Validates: Requirements 2.7**

Property 5: Bug Condition - 密钥注入口径一致

_For any_ 密钥注入相关的文档（runbook、compose、代码），修复后 SHALL 三者口径一致，无互相冲突的说明。

**Validates: Requirements 2.8**

Property 6: Bug Condition - eBPF 编译回归验证

_For any_ `mirage-gateway/bpf/` 下的关键 `.c` 文件，修复后 SHALL 存在自动化编译测试，至少验证 `clang -target bpf` 可将其编译为目标产物。

**Validates: Requirements 2.6**

Property 7: Bug Condition - 配额耗尽回调稳定触发

_For any_ 用户配额从正数消费至恰好 0 的场景，修复后的 `QuotaBucketManager.Consume` SHALL 正确设置 `Exhausted=1` 并触发 `onExhausted` 回调，`go test -count=10 ./pkg/api` 稳定通过。

**Validates: Requirements 2.9**

Property 8: Bug Condition - benchmarks 可直接运行

_For any_ 干净环境下在 `benchmarks/` 目录执行 `go test ./...`，修复后 SHALL 直接运行成功，不再提示 `go mod tidy`。

**Validates: Requirements 2.10**

Property 9: Bug Condition - 日志脱敏

_For any_ Gateway/OS/Client 的生产日志输出路径，修复后 SHALL 通过统一 `redact` 包对敏感字段执行脱敏，日志中不出现明文 IP/user_id/token/secret/password。

**Validates: Requirements 2.4**

Property 10: Bug Condition - 编排主链收敛

_For any_ Gateway 运行时的传输编排调用路径，修复后 SHALL 以 Orchestrator 为唯一主链，TransportManager 仅保留兼容层并标记 deprecated。

**Validates: Requirements 2.5**

Property 11: Preservation - 构建与测试回归

_For any_ 现有的构建、测试和部署流程（包括 `go build`、`go vet`、P0 runtime 测试、chaos 测试、发布签名、证书轮转、wintun.dll 嵌入），修复后 SHALL 产生与修复前完全相同的结果。

**Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9, 3.10**

Property 12: Document Closure - 审计文档闭环

_For any_ 已修复并通过复验的 P0/P1 Finding，修复后 SHALL 在 `docs/audit-report.md` 中将对应 Finding 状态更新为 `verified`，在 `docs/audit-remediation-checklist.md` 的执行记录模板中回写负责人、修复 Commit、复验命令、复验结果和审计状态。当所有 P0/P1 Finding 均为 `verified` 时，审计总结论 SHALL 从 `release_blocked` 更新为 `release_ready`。

**Validates: Requirements 4.1, 4.2, 4.3, 4.4**

## Fix Implementation

### Changes Required

假设根因分析正确，按四批次实施：

### 第一批：供应链与配置基线

**F-P0-01 — 新增统一安全扫描脚本**

**File**: `scripts/security-scan.sh`（新建）+ `scripts/security-scan.ps1`（新建）

**Specific Changes**:
1. 创建 `scripts/security-scan.sh`（Bash）和 `scripts/security-scan.ps1`（PowerShell），遍历所有含 `go.mod` 的目录执行 `govulncheck ./...`
2. 对 `sdk/js` 执行 `npm audit --omit=dev`
3. 输出结构化扫描结果（JSON），支持 CI 归档
4. 非零退出码表示存在未豁免高危漏洞
5. 两个脚本功能等价，确保 Windows（PowerShell）和 Linux/macOS（Bash）均可执行

---

**F-P0-02 — 配置文件安全化**

**File**: `mirage-os/configs/config.yaml`

**Specific Changes**:
1. 在文件头部添加显式标注：`# ⚠️ 仅限开发环境 — 生产环境禁止引用此文件`
2. 将 `password: postgres` 改为 `password: ${DB_PASSWORD:}` 并添加注释说明生产必须设置
3. 将 `jwt_secret: ${JWT_SECRET:change-this-in-production}` 改为 `jwt_secret: ${JWT_SECRET:}`，移除危险默认值
4. 考虑将文件改名为 `config.dev.yaml` 以进一步降低误用风险

---

**F-P0-03 — 构建产物清理与 DLL 追溯**

**Files**: 工作目录 `.exe`/`.dll` 文件、`phantom-client/WINTUN_SOURCE.md`（新建）

**Specific Changes**:
1. 清理工作目录中所有 `.exe`/`.dll` 构建产物（`git clean -fdX` 或手动删除）
2. 确认 `wintun.dll` 的官方来源（wintun.net）、版本号、下载地址、SHA256
3. 在 `phantom-client/` 下创建 `WINTUN_SOURCE.md` 记录追溯信息
4. 确立 `phantom-client/cmd/phantom/wintun.dll` 为唯一 authoritative 副本（`go:embed` 源）
5. 删除多余副本：`phantom-client/wintun.dll`（根目录）和 `phantom-client/embed/wintun.dll`
6. 确认 `Dockerfile.chaos` 中 `RUN touch cmd/phantom/wintun.dll` 占位逻辑不受影响（Linux 构建时创建空文件满足 `go:embed`，不依赖真实 DLL）
7. 确认无其他代码引用 `phantom-client/embed/wintun.dll` 或 `phantom-client/wintun.dll` 路径
8. 在 CI 中增加 `git status --porcelain` 检查防止产物误提交

### 第二批：部署与密钥注入口径

**F-P1-04 — Redis 鉴权**

**File**: `deploy/docker-compose.os.yml`

**Specific Changes**:
1. Redis 服务 command 改为 `redis-server --appendonly yes --requirepass ${MIRAGE_REDIS_PASSWORD}`
2. `gateway-bridge` 环境变量 `REDIS_URL` 改为 `redis://:${MIRAGE_REDIS_PASSWORD}@redis:6379`
3. `api-server` 环境变量 `REDIS_URL` 同上
4. Redis healthcheck 改为 `redis-cli -a ${MIRAGE_REDIS_PASSWORD} ping`
5. 同步更新 runbook 中 Redis 相关说明

---

**F-P1-05 — 密钥注入口径统一**

**Files**: `deploy/runbooks/secret-injection.md`、`deploy/docker-compose.os.yml`

**Specific Changes**:
1. 在 `secret-injection.md` 中新增"过渡方案"章节，明确 compose 环境变量注入为当前发布的过渡方案
2. 标注适用边界：仅限受控网络内的 compose 部署，生产 K8s 部署必须走 Secret/Vault
3. 在 compose 文件头部添加注释说明密钥注入方式与 runbook 的关系
4. 确保 runbook、compose、代码三者对当前发布方式的描述一致

### 第三批：代码与测试回归

**F-P1-03 — eBPF 编译回归测试**

**File**: `mirage-gateway/pkg/ebpf/compile_test.go`（扩展）或 `mirage-gateway/scripts/test-ebpf-compile.sh`（新建）

**Specific Changes**:
1. 新增测试函数或脚本，对 `bpf/` 下关键 `.c` 文件执行 `clang -O2 -target bpf -c`
2. 至少覆盖：`npm.c`、`bdna.c`、`jitter.c`、`vpc.c`、`l1_defense.c`、`l1_silent.c`
3. 验证编译产物（`.o` 文件）生成成功
4. 若本机无 clang/BPF 工具链，测试应 skip 并输出明确提示
5. 将该测试纳入 CI（可通过容器化编译环境）

---

**F-P1-06 — 配额耗尽竞态修复**

**File**: `mirage-gateway/pkg/api/quota_bucket.go`

**Function**: `Consume`

**Specific Changes**:
1. 在 CAS 成功后检查新余额是否为 0：
   ```
   newRemaining := remaining - bytes
   if CAS(&bucket.RemainingBytes, remaining, newRemaining) {
       if newRemaining == 0 {
           if CAS(&bucket.Exhausted, 0, 1) {
               go cb(userID)
           }
       }
       return true
   }
   ```
2. 这样当配额恰好消费至 0 时，`Exhausted` 标记立即设置，回调立即触发
3. 将 `TestQuotaBucket_IsolationTwoUsers` 改为确定性用例，避免依赖时序
4. 新增专门的"恰好耗尽"测试用例
5. CI 中以 `-count=10` 复验稳定性

---

**F-P1-08 — benchmarks 模块修复**

**File**: `benchmarks/go.mod`、`benchmarks/go.sum`

**Specific Changes**:
1. 在 `benchmarks/` 目录执行 `go mod tidy` 同步模块元数据
2. 确认 `go.sum` 更新后提交
3. 验证 `go test ./...` 在干净环境可直接运行
4. 在 CI 中增加 `benchmarks/` 的最小编译验证

### 第四批：架构与日志治理

**F-P1-01 — 日志脱敏中间件**

**Files**: 新建 `pkg/redact/redact.go`（共享包）、Gateway/OS/Client 关键日志路径

**Specific Changes**:
1. 创建统一 `redact` 包，提供 `RedactIP`、`RedactToken`、`RedactSecret` 等函数
2. 定义敏感字段白名单规则：IP → `x.x.x.***`、token → `***`、password → `[REDACTED]`
3. 优先改造高频日志路径：Gateway 审计日志、OS 鉴权日志、Client Provisioning 日志
4. 为每个组件新增至少 1 个脱敏单测

---

**F-P1-02 — 编排主链收敛**

**Files**: `mirage-gateway/pkg/gtunnel/transport.go`、`mirage-gateway/pkg/gtunnel/chameleon_client.go`

**Specific Changes**:
1. 盘点 `TransportManager` 所有直接调用点（当前已标记 deprecated）
2. 确认 `ConnectWithFallback`、`probeAndPromote`、`promoteToQUIC`、`recvLoop` 等方法无生产代码直接调用
3. 将 TransportManager 方法体改为委托到 Orchestrator 的兼容适配层
4. 更新 `source-of-truth-map.md` 将 S-01 标记为已收敛
5. 补充迁移回归测试，确保关键路径走 Orchestrator

### 第五步：审计文档闭环

**文档状态回写**

**Files**: `docs/audit-report.md`、`docs/audit-remediation-checklist.md`

**Specific Changes**:
1. 每个 P0/P1 Finding 修复并通过复验后，将 `docs/audit-report.md` 第八章 Findings 汇总表中对应行的状态从 `open` 更新为 `verified`
2. 在 `docs/audit-remediation-checklist.md` 第七章执行记录模板中，为每个任务回写：负责人（角色）、修复 Commit、复验命令、复验结果、审计状态
3. 当所有 P0/P1 Finding 均为 `verified` 后，将 `docs/audit-report.md` 第一章审计总结论从 `release_blocked` 更新为 `release_ready`
4. 同步更新审计报告第九章"文档更新要求"中列出的关联文档状态

## Testing Strategy

### Validation Approach

测试策略分两阶段：先在未修复代码上验证缺陷可复现（探索性测试），再在修复后验证缺陷消除且现有行为保持不变。

由于 10 个审计发现涵盖脚本/配置/文档/代码多种类型，测试策略按类型分层：
- **脚本/配置类**（F-P0-01, F-P0-02, F-P0-03, F-P1-04, F-P1-05, F-P1-08）：以执行验证和内容检查为主
- **代码逻辑类**（F-P1-06）：以单元测试和属性测试为主
- **编译验证类**（F-P1-03）：以编译脚本执行为主
- **架构/中间件类**（F-P1-01, F-P1-02）：以集成测试和单元测试为主

### Exploratory Bug Condition Checking

**Goal**: 在修复前验证每个缺陷条件可复现，确认根因分析正确。

**Test Plan**: 对每个 Finding 执行最小复现命令，记录失败输出。所有探索/复验命令同时提供 Bash 和 PowerShell 版本，确保在 Windows 和 Linux 环境均可执行。

**Test Cases**:
1. **F-P0-01 扫描缺失**: 搜索仓库中 `govulncheck` 或 `npm audit` 的脚本/CI 配置——预期无结果
   - Bash: `find scripts/ -name "*security*" 2>/dev/null`
   - PowerShell: `Get-ChildItem -Path scripts -Filter "*security*" -ErrorAction SilentlyContinue`
2. **F-P0-02 危险默认值**: 检查配置文件中的危险默认值——预期匹配
   - Bash: `grep -n "password: postgres\|change-this-in-production" mirage-os/configs/config.yaml`
   - PowerShell: `Select-String -Path mirage-os\configs\config.yaml -Pattern 'password: postgres|change-this-in-production'`
3. **F-P0-03 未追踪产物**: 检查工作目录中的二进制文件——预期存在多个文件；wintun.dll 无 SHA256 记录
   - Bash: `find . -name "*.exe" -o -name "*.dll" | grep -v node_modules`
   - PowerShell: `Get-ChildItem -Recurse -Include *.exe,*.dll | Where-Object { $_.FullName -notmatch 'node_modules' }`
4. **F-P1-04 Redis 无密码**: 检查 Redis 配置——预期无匹配
   - Bash: `grep "requirepass" deploy/docker-compose.os.yml`
   - PowerShell: `Select-String -Path deploy\docker-compose.os.yml -Pattern 'requirepass'`
5. **F-P1-05 口径矛盾**: 对比 compose 环境变量注入方式与 runbook 要求——预期矛盾
6. **F-P1-06 配额竞态（双层验证）**:
   - **Layer 1 确定性单元测试**: 编写 `TestConsume_ExactExhaustion` 测试，构造 `initialQuota == consumeAmount` 场景，验证 Consume 后 `IsExhausted` 返回 true——预期在未修复代码上 FAIL（确定性复现根因）
   - **Layer 2 压力复验**: `cd mirage-gateway && go test -count=10 ./pkg/api -run TestQuotaBucket_IsolationTwoUsers`——预期在未修复代码上间歇性 FAIL（验证 flaky 性质）
7. **F-P1-08 benchmarks 失败**: `cd benchmarks && go test ./...`——预期提示 `go mod tidy`
8. **F-P1-03 eBPF 编译**: 检查 `compile_test.go` 内容——预期仅有结构体对齐测试

**Expected Counterexamples**:
- F-P1-06 Layer 1：`TestConsume_ExactExhaustion` 中 `Consume(100, 100)` 返回 true 但 `IsExhausted` 返回 false（确定性失败，直接证明根因）
- F-P1-06 Layer 2：`TestQuotaBucket_IsolationTwoUsers` 在 10 次执行中至少 1 次 `onExhausted` 回调未触发（间歇性失败，证明 flaky 性质）
- F-P1-08：`go test` 直接报错 `go: updates to go.mod needed`
- 其他 Finding：通过内容检查/命令执行确认缺陷存在

### Fix Checking

**Goal**: 验证修复后所有 Bug Condition 不再成立。

**Pseudocode:**
```
FOR ALL finding WHERE isBugCondition(finding) DO
  result := applyFix(finding)
  ASSERT expectedBehavior(result)
END FOR
```

**具体验证**:
- F-P0-01: `scripts/security-scan.sh`（Bash）和 `scripts/security-scan.ps1`（PowerShell）均可执行，输出扫描结果
- F-P0-02: `Select-String` / `grep` 不再匹配危险默认值
- F-P0-03: `Get-ChildItem` / `find` 不再发现未追踪产物，`wintun.dll` 有 SHA256 记录，多余副本已删除
- F-P1-04: Redis 连接无密码被拒绝
- F-P1-05: runbook/compose/代码三者一致
- F-P1-06 Layer 1: `TestConsume_ExactExhaustion` 确定性通过（余额归零时 Exhausted=1）
- F-P1-06 Layer 2: `go test -count=10 ./pkg/api` 稳定通过（不再 flaky）
- F-P1-08: `cd benchmarks && go test ./...` 直接成功
- F-P1-03: eBPF 编译测试通过
- F-P1-01: 脱敏单测通过
- F-P1-02: 主链调用走 Orchestrator

### Preservation Checking

**Goal**: 验证修复不影响现有功能。

**Pseudocode:**
```
FOR ALL input WHERE NOT isBugCondition(input) DO
  ASSERT originalBehavior(input) == fixedBehavior(input)
END FOR
```

**Testing Approach**: 属性测试推荐用于 F-P1-06 的保全验证，因为：
- 可自动生成大量随机配额消费场景
- 能捕获手动测试遗漏的边界条件
- 对非零正数配额的正常消费路径提供强保证

**Test Plan**: 先在未修复代码上观察正常行为基线，再编写属性测试确保修复后行为一致。

**Test Cases**:
1. **构建保全**: `go build ./cmd/gateway/`、`go build ./...`（OS）、`go build ./cmd/phantom/`、`go build ./...`（CLI）全部 EXIT_CODE=0
2. **Vet 保全**: `go vet ./...` 对所有组件 EXIT_CODE=0
3. **P0 Runtime 保全**: `tests/p0_runtime/` 全部测试通过
4. **开发环境保全**: `deploy/docker-compose.dev.yml` 服务正常启动
5. **配额正常消费保全**: 非零正数配额用户的 Consume 调用返回 true，余额正确递减
6. **wintun.dll 嵌入保全**: `phantom-client` 构建成功，嵌入机制不变

### Unit Tests

- F-P1-06: 配额恰好耗尽场景的确定性测试（`remaining == bytes` 时 Exhausted 必须为 1）
- F-P1-06: 配额充足场景的正常消费测试（行为不变）
- F-P1-06: 未知用户拒绝测试（行为不变）
- F-P1-01: 各脱敏函数的输入输出测试（IP/token/password）
- F-P1-03: eBPF `.c` 文件编译成功/失败的测试

### Property-Based Tests

- F-P1-06: 生成随机 (initialQuota, consumeAmount, numConcurrentUsers) 组合，验证：当总消费 >= 配额时 Exhausted 必须为 1 且回调触发恰好一次
- F-P1-06: 生成随机非耗尽场景，验证修复前后 Consume 返回值一致（保全属性）
- F-P1-01: 生成随机包含敏感字段的日志字符串，验证 redact 后不包含原始敏感值

### Integration Tests

- 全组件构建验证：所有 `go build` + `go vet` 通过
- Redis 鉴权端到端：compose up 后验证无密码连接被拒绝、有密码连接成功
- 安全扫描端到端：`scripts/security-scan.sh` 在 CI 环境完整执行
- benchmarks 端到端：`cd benchmarks && go test -bench=. -benchtime=1x` 至少执行一轮
