# 审计整改实施任务清单

- [x] 1. 编写 Bug Condition 探索性测试（修复前执行）
  - **Property 1: Bug Condition** - 审计阻断条件复现
  - **重要**: 此测试必须在实施任何修复之前编写和运行
  - **目标**: 通过自动化验证确认所有 10 个审计缺陷条件可复现
  - **跨平台要求**: 所有探索/复验命令必须同时提供 Bash 和 PowerShell 版本（当前环境为 Windows PowerShell）
  - **Scoped PBT 方法**: 针对每个 Finding 的具体缺陷条件编写确定性验证
  - 验证项（对应设计文档 isBugCondition 伪代码）：
    - C1: 搜索仓库中安全扫描脚本——预期无结果
      - PowerShell: `Get-ChildItem -Path scripts -Filter "*security*" -ErrorAction SilentlyContinue`
      - Bash: `find scripts/ -name "*security*" 2>/dev/null`
    - C2: 检查配置文件危险默认值——预期匹配
      - PowerShell: `Select-String -Path mirage-os\configs\config.yaml -Pattern 'password: postgres|change-this-in-production'`
      - Bash: `grep -n "password: postgres\|change-this-in-production" mirage-os/configs/config.yaml`
    - C3: 检查工作目录二进制文件——预期存在多个文件；wintun.dll 无 SHA256 记录
      - PowerShell: `Get-ChildItem -Recurse -Include *.exe,*.dll | Where-Object { $_.FullName -notmatch 'node_modules' }`
      - Bash: `find . -name "*.exe" -o -name "*.dll" | grep -v node_modules`
    - C7: 检查 Redis 鉴权配置——预期无匹配
      - PowerShell: `Select-String -Path deploy\docker-compose.os.yml -Pattern 'requirepass'`
      - Bash: `grep "requirepass" deploy/docker-compose.os.yml`
    - C8: 对比 compose 环境变量注入方式与 runbook 要求——预期矛盾
    - C6: 检查 `compile_test.go` 内容——预期仅有 L1Stats 结构体对齐测试，无 `clang -target bpf` 编译
    - C9: 对 `quota_bucket.go` Consume 方法分析——当 `remaining == bytes` 且 CAS 成功时未设置 `Exhausted=1`
    - C10: `cd benchmarks && go test ./...`——预期提示 `go: updates to go.mod needed`
  - 对 F-P1-06 编写双层验证测试：
    - **Layer 1 确定性单元测试**: 编写 `TestConsume_ExactExhaustion`，构造 `initialQuota == consumeAmount` 场景，验证 Consume 后 `IsExhausted` 返回 true——预期在未修复代码上确定性 FAIL（直接证明根因）
    - **Layer 2 压力复验**: `go test -count=10 ./pkg/api -run TestQuotaBucket_IsolationTwoUsers`——预期在未修复代码上间歇性 FAIL（验证 flaky 性质）
  - 在未修复代码上运行测试
  - **预期结果**: Layer 1 确定性 FAIL + Layer 2 间歇性 FAIL（确认缺陷存在）
  - 记录反例：F-P1-06 Layer 1 `Consume(100, 100)` 返回 true 但 `IsExhausted` 返回 false（确定性）；Layer 2 间歇性回调未触发；F-P1-08 `go test` 报错 `go: updates to go.mod needed`
  - 任务完成标准：测试已编写、已运行、失败已记录
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 1.10_

- [x] 2. 编写保全属性测试（修复前执行）
  - **Property 2: Preservation** - 现有行为基线保全
  - **重要**: 遵循观察优先方法论——先在未修复代码上观察行为，再编写测试
  - 观察并记录未修复代码上的正常行为基线：
    - 观察: `go build ./cmd/gateway/` EXIT_CODE=0
    - 观察: `go vet ./...`（所有组件）EXIT_CODE=0
    - 观察: 非零正数配额用户 `Consume("user", 200)` 在 quota=1000 时返回 true，余额正确递减至 800
    - 观察: 未知用户 `Consume("unknown", 1)` 返回 false
    - 观察: `phantom-client` 构建成功，`go:embed` wintun.dll 嵌入机制正常
    - 观察: Orchestrator 编排主链功能正常
  - 编写属性测试（重点 F-P1-06 保全）：
    - 对所有 `initialQuota > consumeAmount > 0` 的场景，`Consume` 返回 true 且余额 == initialQuota - consumeAmount
    - 对所有未知用户，`Consume` 返回 false
    - 对所有 `consumeAmount > initialQuota` 的场景，`Consume` 返回 false 且触发耗尽
  - 在未修复代码上运行测试
  - **预期结果**: 测试 PASS（确认基线行为）
  - 任务完成标准：测试已编写、已运行、在未修复代码上通过
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9, 3.10_

- [x] 3. 第一批修复：供应链与配置基线（F-P0-01, F-P0-02, F-P0-03）

  - [x] 3.1 F-P0-01: 新增统一安全扫描脚本
    - 创建 `scripts/security-scan.sh`（Bash）和 `scripts/security-scan.ps1`（PowerShell），功能等价
    - 遍历所有含 `go.mod` 的目录执行 `govulncheck ./...`
    - 对 `sdk/js` 执行 `npm audit --omit=dev`
    - 输出结构化扫描结果（JSON），支持 CI 归档
    - 非零退出码表示存在未豁免高危漏洞
    - _Bug_Condition: C1 — NOT exists(securityScanScript) AND NOT exists(govulncheckResults)_
    - _Expected_Behavior: 跨平台统一扫描脚本可执行，输出扫描结果，无未豁免高危漏洞_
    - _Preservation: 现有构建流水线不受影响_
    - _Requirements: 2.1_

  - [x] 3.2 F-P0-02: 配置文件安全化
    - 在 `mirage-os/configs/config.yaml` 头部添加 `# ⚠️ 仅限开发环境 — 生产环境禁止引用此文件`
    - 将 `password: postgres` 改为 `password: ${DB_PASSWORD:}`
    - 将 `jwt_secret: ${JWT_SECRET:change-this-in-production}` 改为 `jwt_secret: ${JWT_SECRET:}`
    - 考虑改名为 `config.dev.yaml`
    - _Bug_Condition: C2 — fileContains("password: postgres") AND fileContains("change-this-in-production")_
    - _Expected_Behavior: 配置文件不含危险默认值，明确标注为开发环境专用_
    - _Preservation: deploy/docker-compose.dev.yml 开发环境正常启动不受影响_
    - _Requirements: 2.2_

  - [x] 3.3 F-P0-03: 构建产物清理与 DLL 追溯
    - 清理工作目录中所有 `.exe` 构建产物（`gateway.exe`、`api-gateway.exe`、`phantom.exe` 等）
    - 确认 `wintun.dll` 官方来源（wintun.net）、版本号、下载地址、SHA256
    - 在 `phantom-client/` 下创建 `WINTUN_SOURCE.md` 记录追溯信息
    - 确立 `phantom-client/cmd/phantom/wintun.dll` 为唯一 authoritative 副本（`go:embed` 源）
    - 删除多余副本：`phantom-client/wintun.dll`（根目录副本）和 `phantom-client/embed/wintun.dll`（历史副本）
    - 确认无其他 Go 代码引用 `phantom-client/embed/wintun.dll` 或 `phantom-client/wintun.dll` 路径（当前仅 `cmd/phantom/main.go` 的 `//go:embed wintun.dll` 引用 `cmd/phantom/wintun.dll`）
    - 确认 `Dockerfile.chaos` 中 `RUN touch cmd/phantom/wintun.dll` 占位逻辑不受影响（Linux 构建时创建空文件满足 `go:embed`，不依赖真实 DLL 内容）
    - _Bug_Condition: C3 — existsUntrackedBinaries AND NOT documented(wintunDLL.source, version, sha256)_
    - _Expected_Behavior: 无未追踪产物，wintun.dll 有完整来源记录，仅保留单一 authoritative 副本_
    - _Preservation: phantom-client 的 go:embed wintun.dll 嵌入机制不变（嵌入路径 cmd/phantom/wintun.dll 未改动）_
    - _Requirements: 2.3_

- [x] 4. 第二批修复：部署与密钥注入口径（F-P1-04, F-P1-05）

  - [x] 4.1 F-P1-04: Redis 鉴权
    - Redis 服务 command 改为 `redis-server --appendonly yes --requirepass ${MIRAGE_REDIS_PASSWORD}`
    - `gateway-bridge` 环境变量 `REDIS_URL` 改为 `redis://:${MIRAGE_REDIS_PASSWORD}@redis:6379`
    - `api-server` 环境变量 `REDIS_URL` 同上
    - Redis healthcheck 改为 `redis-cli -a ${MIRAGE_REDIS_PASSWORD} ping`
    - 同步更新 runbook 中 Redis 相关说明
    - _Bug_Condition: C7 — redisConfig.hasNoAuth() AND consumers.connectWithoutPassword()_
    - _Expected_Behavior: Redis 配置 requirepass，消费方连接串包含密码，未授权连接被拒绝_
    - _Preservation: deploy/docker-compose.dev.yml 开发环境不受影响_
    - _Requirements: 2.7_

  - [x] 4.2 F-P1-05: 密钥注入口径统一
    - 在 `secret-injection.md` 中新增"过渡方案"章节
    - 标注适用边界：仅限受控网络内的 compose 部署
    - 在 compose 文件头部添加注释说明密钥注入方式与 runbook 的关系
    - 确保 runbook、compose、代码三者对当前发布方式的描述一致
    - _Bug_Condition: C8 — compose.usesEnvVarInjection AND runbook.requires("K8s Secret / Vault") AND compose.method != runbook.method_
    - _Expected_Behavior: runbook、compose、代码三者口径一致_
    - _Preservation: 现有密钥注入流程不中断_
    - _Requirements: 2.8_

- [x] 5. 第三批修复：代码与测试回归（F-P1-03, F-P1-06, F-P1-08）

  - [x] 5.1 F-P1-03: eBPF 编译回归测试
    - 新增测试函数或脚本，对 `bpf/` 下关键 `.c` 文件执行 `clang -O2 -target bpf -c`
    - 至少覆盖：`npm.c`、`bdna.c`、`jitter.c`、`vpc.c`、`l1_defense.c`、`l1_silent.c`
    - 验证编译产物（`.o` 文件）生成成功
    - 若本机无 clang/BPF 工具链，测试应 skip 并输出明确提示
    - _Bug_Condition: C6 — compileTest.onlyChecks("L1Stats struct alignment") AND NOT compilesAnyBPFCFile()_
    - _Expected_Behavior: 存在自动化编译测试，至少验证 clang -target bpf 可编译关键 .c 文件_
    - _Preservation: 现有 compile_test.go 中 L1Stats 对齐测试保留不变_
    - _Requirements: 2.6_

  - [x] 5.2 F-P1-06: 配额耗尽竞态修复
    - 修改 `mirage-gateway/pkg/api/quota_bucket.go` 的 `Consume` 方法
    - 在 CAS 成功后检查新余额是否为 0：`newRemaining := remaining - bytes; if newRemaining == 0 { CAS(&bucket.Exhausted, 0, 1); go cb(userID) }`
    - 新增确定性单元测试 `TestConsume_ExactExhaustion`：验证 `initialQuota == consumeAmount` 时 Exhausted=1 且回调触发
    - 将 `TestQuotaBucket_IsolationTwoUsers` 改为确定性用例，避免依赖时序
    - CI 中以 `-count=10` 压力复验稳定性
    - _Bug_Condition: C9 — CAS(remaining, 0) succeeds BUT Exhausted flag not set to 1 when remaining == bytes_
    - _Expected_Behavior: 配额从正数消费至恰好 0 时，Exhausted=1 且 onExhausted 回调触发_
    - _验证双层: Layer 1 确定性单元测试证明根因修复；Layer 2 压力测试 -count=10 证明不再 flaky_
    - _Preservation: 非零正数配额用户的正常消费行为不变_
    - _Requirements: 2.9_

  - [x] 5.3 F-P1-08: benchmarks 模块修复
    - 在 `benchmarks/` 目录执行 `go mod tidy` 同步模块元数据
    - 确认 `go.sum` 更新后提交
    - 验证 `go test ./...` 在干净环境可直接运行
    - _Bug_Condition: C10 — exec("go test ./...", cwd="benchmarks/").exitCode != 0 AND errorContains("go: updates to go.mod needed")_
    - _Expected_Behavior: benchmarks/ 在干净环境下 go test ./... 直接运行成功_
    - _Preservation: 现有 benchmark 测试逻辑不变_
    - _Requirements: 2.10_

  - [x] 5.4 验证 Bug Condition 探索性测试通过
    - **Property 1: Expected Behavior** - 审计阻断条件已消除
    - **重要**: 重新运行任务 1 中的同一测试——不要编写新测试
    - 任务 1 的测试编码了期望行为，通过即确认缺陷已修复
    - 重点验证 F-P1-06 Layer 1: `TestConsume_ExactExhaustion` 确定性通过（余额归零时 Exhausted=1）
    - 重点验证 F-P1-06 Layer 2: `go test -count=10 ./pkg/api` 稳定通过（不再 flaky）
    - 重点验证 F-P1-08: `cd benchmarks && go test ./...` 成功
    - **预期结果**: 测试 PASS（确认缺陷已修复）
    - _Requirements: 2.1, 2.2, 2.3, 2.6, 2.7, 2.8, 2.9, 2.10_

  - [x] 5.5 验证保全属性测试仍然通过
    - **Property 2: Preservation** - 现有行为未被破坏
    - **重要**: 重新运行任务 2 中的同一测试——不要编写新测试
    - 验证所有保全属性测试在修复后仍然通过
    - 确认非零正数配额消费行为不变
    - 确认未知用户拒绝行为不变
    - **预期结果**: 测试 PASS（确认无回归）
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9, 3.10_

- [x] 6. 第四批修复：架构与日志治理（F-P1-01, F-P1-02）

  - [x] 6.1 F-P1-01: 日志脱敏中间件
    - 创建统一 `pkg/redact/redact.go` 共享包
    - 提供 `RedactIP`、`RedactToken`、`RedactSecret` 等函数
    - 定义敏感字段规则：IP → `x.x.x.***`、token → `***`、password → `[REDACTED]`
    - 优先改造高频日志路径：Gateway 审计日志、OS 鉴权日志、Client Provisioning 日志
    - 为每个组件新增至少 1 个脱敏单测
    - _Bug_Condition: C4 — NOT exists(unifiedRedactPackage) AND logsMayContainSensitiveFields_
    - _Expected_Behavior: 通过统一 redact 包对敏感字段执行脱敏，日志中不出现明文敏感信息_
    - _Preservation: 现有日志输出路径的非敏感内容不变_
    - _Requirements: 2.4_

  - [x] 6.2 F-P1-02: 编排主链收敛
    - 盘点 `TransportManager` 所有直接调用点
    - 确认 `ConnectWithFallback`、`probeAndPromote`、`promoteToQUIC`、`recvLoop` 等方法无生产代码直接调用
    - 将 TransportManager 方法体改为委托到 Orchestrator 的兼容适配层
    - 更新 `source-of-truth-map.md` 将 S-01 标记为已收敛
    - 补充迁移回归测试
    - _Bug_Condition: C5 — hasDirectCallsTo("TransportManager") IN productionCodePath AND NOT allCallsRoutedThrough("Orchestrator")_
    - _Expected_Behavior: Orchestrator 为唯一主链，TransportManager 仅保留兼容层_
    - _Preservation: Orchestrator 编排主链功能不变（路径调度、BBR v3、多路径传输）_
    - _Requirements: 2.5_

- [x] 7. 审计文档闭环 — 回写审计报告与整改清单
  - **Property 12: Document Closure** - 审计文档状态回写
  - 对每个已修复并通过复验的 P0/P1 Finding：
    - 将 `docs/audit-report.md` 第八章 Findings 汇总表中对应行的状态从 `open` 更新为 `verified`
    - 在 `docs/audit-remediation-checklist.md` 第七章执行记录模板中回写：负责人（角色）、修复 Commit、复验命令、复验结果、审计状态（`verified`）
  - 当所有 P0/P1 Finding 均为 `verified` 后：
    - 将 `docs/audit-report.md` 第一章审计总结论从 `release_blocked` 更新为 `release_ready`
    - 更新第十章发布判定中被 BLOCKED 的条件为通过
  - 同步更新审计报告第九章"文档更新要求"中列出的关联文档状态
  - _Requirements: 4.1, 4.2, 4.3, 4.4_

- [x] 8. 最终检查点 - 确保所有测试通过且文档闭环完成
  - 运行全组件构建验证：`go build` + `go vet` 对 Gateway/OS/Client/CLI
  - 运行 `go test -count=10 ./pkg/api` 确认配额测试稳定通过（Layer 1 确定性 + Layer 2 压力）
  - 运行 `cd benchmarks && go test ./...` 确认 benchmarks 可直接运行
  - 运行保全属性测试确认无回归
  - 确认所有 10 个审计 Finding 的复验命令通过
  - 确认 `docs/audit-report.md` 所有 P0/P1 状态为 `verified`，总结论为 `release_ready`
  - 确认 `docs/audit-remediation-checklist.md` 执行记录模板已完整回写
  - 如有问题，询问用户确认
