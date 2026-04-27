# Phase 4 七域证据盘点报告

> 盘点日期：2026-04-25
> 盘点依据：`docs/governance/capability-truth-source.md` 第五节能力真相矩阵
> 证据来源：Phase 1（M1-M4）、Phase 2（M5-M7）、Phase 3（M8-M9）产出

---

## 一、多承载编排与降级

| 字段 | 内容 |
|------|------|
| 当前 Status_Level | `已实现（限定表述）` |
| Phase 1-3 Evidence_Anchor | `docs/governance/carrier-matrix.md`（承载矩阵已冻结）、`deploy/evidence/m2-degradation-drill.log`（降级/回升演练日志）、`phantom-client/pkg/gtclient/client_orchestrator_test.go`（PBT Property 2/3）、`mirage-gateway/pkg/gtunnel/orchestrator_test.go`（PBT Property 4/5） |

**验收标准逐项达成情况：**

1. Gateway 与 Client 均有统一主链 — **达成**（Gateway Orchestrator 为唯一主链，Client ClientOrchestrator 已接线）
2. 至少完成主通道失败后的自动降级验证 — **达成**（QUIC→WSS 降级已通过代码级模拟验证，PBT Property 2/3 各 100 次迭代）
3. 回升路径有可复验证据 — **达成**（PBT Property 3 验证回升路径）
4. 未经验证的承载不得按稳定能力宣称 — **达成**（承载矩阵已冻结，WebRTC/ICMP/DNS 标注为已接线待闭环）

**升级判定：维持 `已实现（限定表述）`**

差距说明：当前证据为代码级模拟（mock transport），尚无真实网络演练证据。QUIC/WSS 为正式承诺，WebRTC/ICMP/DNS 仍为已接线待闭环。不满足升级为"已实现"的条件。

---

## 二、节点恢复与共振发现

| 字段 | 内容 |
|------|------|
| 当前 Status_Level | `已实现（限定表述）` |
| Phase 1-3 Evidence_Anchor | `deploy/evidence/m3-node-death-drill.log`（节点阵亡恢复演练日志）、`phantom-client/pkg/gtclient/node_death_drill_test.go`（集成测试）、`phantom-client/pkg/gtclient/recovery_fsm_test.go`（PBT Property 1）、`phantom-client/pkg/resonance/resolver_test.go`（PBT Property 6/7） |

**验收标准逐项达成情况：**

1. DoH/Gist/Mastodon 至少一条恢复链路有自动化验证 — **达成**（Resolver First-Win 竞速已通过 PBT Property 6/7 验证）
2. 真实演练能证明节点阵亡后可恢复 — **部分达成**（代码级模拟验证通过，尚无真实节点阵亡演练）
3. 恢复失败会进入明确定义的降级路径 — **达成**（恢复失败返回明确错误）

**升级判定：维持 `已实现（限定表述）`**

差距说明：当前证据为代码级模拟，尚无真实节点阵亡演练证据。不满足升级为"已实现"的条件。

---

## 三、会话连续性与链路漂移

| 字段 | 内容 |
|------|------|
| 当前 Status_Level | `已实现（限定表述）` |
| Phase 1-3 Evidence_Anchor | `deploy/evidence/m4-continuity-report.md`（业务连续性样板报告）、`phantom-client/pkg/gtclient/business_continuity_test.go`（PBT Property 8/9）、`deploy/scripts/drill-m4-continuity.sh`（演练脚本） |

**验收标准逐项达成情况：**

1. 至少对一个受支持路径对完成长连接业务演练 — **达成**（switchWithTransaction 事务式切换已通过验证）
2. 证明链路切换不会造成目标业务中断 — **部分达成**（mock 环境丢包为 0，不能直接推导为业务层无感）
3. 若只能保证传输层切换，不得对外写成"所有 TCP 业务无感" — **达成**（已有分层结论）

**升级判定：维持 `已实现（限定表述）`**

差距说明：mock 环境丢包为 0，不能直接推导为业务层无感。真实网络环境下的业务层影响需单独演练量化。

---

## 四、流量整形与特征隐匿

| 字段 | 内容 |
|------|------|
| 当前 Status_Level | `部分实现` |
| Phase 1-3 Evidence_Anchor | `docs/reports/stealth-experiment-plan.md`（实验方案已冻结）、`docs/reports/stealth-experiment-results.md`（实验结论待采集）、`docs/reports/stealth-claims-boundary.md`（表述边界）、`artifacts/dpi-audit/`（DPI 实验数据目录）、`deploy/scripts/drill-m6-experiment.sh`（实验编排脚本） |

**验收标准逐项达成情况：**

1. 配置主源、加载路径、编译路径、运行时挂载全部存在 — **达成**
2. 关键 `.c` 文件可真实编译 — **达成**（eBPF 编译回归已通过）
3. 若要宣称 DPI/ML 对抗效果，必须有独立实验或基准证据 — **未达成**（受控环境基线待 Linux 环境实际采集）

**升级判定：维持 `部分实现`**

差距说明：实验框架已建立（方案冻结、脚本就绪、PBT 通过），但缺少实际抓包数据和分类器实验结果。受控环境基线待 Linux 环境实际采集。证据不足以升级。

---

## 五、eBPF 深度参与的数据面与防护

| 字段 | 内容 |
|------|------|
| 当前 Status_Level | `已实现（限定表述）` |
| Phase 1-3 Evidence_Anchor | `docs/reports/ebpf-coverage-map.md`（覆盖图已产出）、`artifacts/ebpf-perf/`（性能数据占位文件）、`mirage-gateway/pkg/ebpf/bpf_compile_test.go`（编译回归）、`deploy/scripts/drill-m7-ebpf-coverage.sh`（覆盖验证脚本） |

**验收标准逐项达成情况：**

1. 关键 BPF 程序可编译、可挂载、可接线 — **达成**（编译回归已通过）
2. 关键 Map/Ring Buffer/Threat 回调闭环存在 — **达成**
3. 若要宣称"全流量全链路零拷贝"，必须有更高等级证据 — **达成**（覆盖图已明确区分运行态挂载/源码未挂载/纯用户态路径，结论为"深度参与关键路径，非全链路零拷贝"）

**升级判定：维持 `已实现（限定表述）`**

差距说明：覆盖图证据已闭环，但性能证据待 Linux 环境实际采集。不满足升级为"已实现"的条件。

---

## 六、反取证与最小运行痕迹

| 字段 | 内容 |
|------|------|
| 当前 Status_Level | `已实现（限定表述）` |
| Phase 1-3 Evidence_Anchor | `docs/reports/deployment-tiers.md`（部署等级说明）、`docs/reports/deployment-baseline-checklist.md`（基线检查清单）、`deploy/scripts/drill-m8-baseline.sh`（基线验证脚本）、`deploy/scripts/emergency-wipe.sh`、`deploy/scripts/cert-rotate.sh` |

**验收标准逐项达成情况：**

1. RAM Shield、自毁/紧急擦除、密钥轮转/擦除有脚本和配置证据 — **达成**
2. 若要宣称"无盘化运行"，必须有对应部署基线而不是个别模式 — **达成**（三种部署等级已定义：默认/加固/极限隐匿）

**升级判定：维持 `已实现（限定表述）`**

差距说明：加固部署已有完整配置锚点，但极限隐匿部署部分配置项当前不支持（Emergency_Wipe 自动触发需新增代码、证书 ≤24h 有效期为候选强化项）。

---

## 七、准入控制与防滥用

| 字段 | 内容 |
|------|------|
| 当前 Status_Level | `已实现` |
| Phase 1-3 Evidence_Anchor | `docs/reports/access-control-joint-drill.md`（联合演练记录）、`deploy/evidence/m9-joint-drill.log`（演练日志）、`mirage-gateway/pkg/api/security_regression_test.go`（安全回归）、`mirage-gateway/pkg/api/quota_bucket_test.go`（配额隔离 PBT）、`mirage-os/services/ws-gateway/auth_test.go`（JWT 鉴权） |

**验收标准逐项达成情况：**

1. 关键鉴权链路有回归测试 — **达成**（HMAC/JWT/Redis 鉴权回归全部通过）
2. Redis 鉴权、JWT/HMAC/mTLS、配额熔断、日志脱敏有发布级验证 — **达成**（M9 联合演练全部通过，5 个 PBT 各 100 次迭代，3 个 Critical Tests 通过）
3. 对外宣称必须与当前运行脚本一致 — **达成**

**升级判定：确认维持 `已实现`**

无差距。联合演练未发现跨组件协同缺陷。

---

## 八、盘点总结

| 能力域 | 盘点前状态 | 盘点后状态 | 是否升级 |
|--------|-----------|-----------|---------|
| 多承载编排与降级 | `已实现（限定表述）` | `已实现（限定表述）` | 否 |
| 节点恢复与共振发现 | `已实现（限定表述）` | `已实现（限定表述）` | 否 |
| 会话连续性与链路漂移 | `已实现（限定表述）` | `已实现（限定表述）` | 否 |
| 流量整形与特征隐匿 | `部分实现` | `部分实现` | 否 |
| eBPF 深度参与 | `已实现（限定表述）` | `已实现（限定表述）` | 否 |
| 反取证与最小运行痕迹 | `已实现（限定表述）` | `已实现（限定表述）` | 否 |
| 准入控制与防滥用 | `已实现` | `已实现` | 否（确认维持） |

七域均维持当前状态，无升级。主要原因：代码级模拟证据充分，但真实网络/环境演练证据尚未采集（需 Linux 环境）。
