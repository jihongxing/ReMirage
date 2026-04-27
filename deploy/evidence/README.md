# Phase 1 受控演练证据目录

## 证据强度等级

**代码级模拟（mock transport）**：所有演练基于 mock transport，不依赖真实网络连接。若需升级为真实网络演练证据，应单独立后续任务。

## 目录结构

| 文件 | 用途 | 生成方式 |
|------|------|----------|
| `m2-degradation-drill.log` | M2 降级/回升演练运行日志 | `bash deploy/scripts/drill-m2-degradation.sh` |
| `m3-node-death-drill.log` | M3 节点阵亡恢复演练运行日志 | `bash deploy/scripts/drill-m3-node-death.sh` |
| `m4-continuity-drill.log` | M4 业务连续性演练运行日志 | `bash deploy/scripts/drill-m4-continuity.sh` |
| `m4-continuity-report.md` | M4 业务连续性分层结论报告 | 手动维护 + drill 脚本辅助 |

## 复验命令

```bash
# M2 降级/回升
bash deploy/scripts/drill-m2-degradation.sh

# M3 节点阵亡恢复
bash deploy/scripts/drill-m3-node-death.sh

# M4 业务连续性
bash deploy/scripts/drill-m4-continuity.sh
```

## 与 capability-truth-source 的关系

本目录产出的证据文件是 `docs/governance/capability-truth-source.md` 状态回写的直接依据。回写时必须如实标注证据强度等级（代码级模拟）。

---

# Phase 2 隐匿与数据面证据目录

## 证据强度等级

- **受控环境基线（Linux）**：Linux + eBPF 真实加载环境下产出的实验数据
- **模拟环境参考（非 Linux）**：非 Linux 环境下的 Mock PBT 和文档检查

## 目录结构

| 文件 | 用途 | 生成方式 |
|------|------|----------|
| `m5-experiment-plan-drill.log` | M5 实验方案冻结验证日志 | `bash deploy/scripts/drill-m5-experiment-plan.sh` |
| `m6-experiment-drill.log` | M6 四个检测面实验执行日志 | `bash deploy/scripts/drill-m6-experiment.sh` |
| `m7-ebpf-coverage-drill.log` | M7 覆盖图与性能验证日志 | `bash deploy/scripts/drill-m7-ebpf-coverage.sh` |

## 报告文件

| 文件 | 用途 |
|------|------|
| `docs/reports/stealth-experiment-plan.md` | 隐匿实验方案（M5 冻结） |
| `docs/reports/stealth-claims-boundary.md` | 对外表述边界清单 |
| `docs/reports/stealth-experiment-results.md` | 隐匿实验结果报告（待采集） |
| `docs/reports/ebpf-coverage-map.md` | eBPF 覆盖图 |

## 证据产物目录

| 目录 | 用途 |
|------|------|
| `artifacts/dpi-audit/handshake/` | 握手指纹实验数据 |
| `artifacts/dpi-audit/packet-length/` | 包长分布实验数据 |
| `artifacts/dpi-audit/timing/` | 时序分布实验数据 |
| `artifacts/dpi-audit/classifier/` | 分类器实验数据 |
| `artifacts/ebpf-perf/` | eBPF 性能采集数据 |

## 复验命令

```bash
bash deploy/scripts/drill-m5-experiment-plan.sh
bash deploy/scripts/drill-m6-experiment.sh
bash deploy/scripts/drill-m7-ebpf-coverage.sh
```
