---
Status: active
Source of Truth: docs/governance/capability-truth-source.md
Role: 对外表述边界清单 — 基于当前代码能力和验收标准，划定允许/不允许表述
Version: M7 收口版
Last Updated: M7
Next Update: Phase 2 出关判定后最终确认
---

# 对外表述边界清单（Stealth Claims Boundary）

## 一、文档目的

本文档基于 `capability-truth-source.md` 的三层模型和验收标准，结合当前代码能力的实际证据，划定 ReMirage 在隐匿能力和 eBPF 数据面两个能力域的对外允许表述和不允许表述。

核心原则：

- 只有通过验收标准且有证据支撑的能力，才允许对外表述
- 没有独立实验结果的能力，不得按满额效果对外宣称
- 本文档在 M6（隐匿实验结果）和 M7（eBPF 覆盖图与性能证据）完成后，根据实验结果更新

## 二、当前证据基线

截至 M5 冻结时，两个能力域的证据状态如下：

### 流量整形与特征隐匿

| 证据项 | 状态 | 证据来源 |
|--------|------|----------|
| 协议主源已建立（NPM / B-DNA / Jitter-Lite / VPC） | ✅ 已确认 | `docs/protocols/source-of-truth.md`、各协议主文档 |
| 运行时锚点已建立（loader.go 挂载路径） | ✅ 已确认 | `mirage-gateway/pkg/ebpf/loader.go` |
| 关键 .c 文件可真实编译 | ✅ 已确认 | `mirage-gateway/pkg/ebpf/bpf_compile_test.go`、`mirage-gateway/scripts/test-ebpf-compile.sh` |
| eBPF 编译回归通过 | ✅ 已确认 | 编译回归测试 |
| JA3/JA4/DPI 对抗实验结果 | ❌ 不存在 | 无 |
| ML 分类器可分性实验结果 | ❌ 不存在 | 无 |
| 包长分布改变效果实验结果 | ❌ 不存在 | 无 |
| 时序扰动效果实验结果 | ❌ 不存在 | 无 |

当前状态：**部分实现**（`capability-truth-source.md` 已确认）

### eBPF 深度参与的数据面与防护

| 证据项 | 状态 | 证据来源 |
|--------|------|----------|
| 关键 BPF 程序可编译、可挂载 | ✅ 已确认 | 编译回归测试、loader.go |
| Map / Ring Buffer / Threat 回调闭环存在 | ✅ 已确认 | `mirage-gateway/pkg/ebpf/*` |
| B-DNA / Jitter / L1 防护已接线 | ✅ 已确认 | loader.go 挂载记录 |
| eBPF 覆盖图（参与 vs 用户态路径对照） | ✅ 已产出 | `docs/reports/ebpf-coverage-map.md` |
| 性能观测数据（延迟/CPU/内存） | ⏳ 待采集（需 Linux 环境） | `artifacts/ebpf-perf/`（占位文件已创建） |

当前状态：**已实现（限定表述）**（`capability-truth-source.md` 已确认）

## 三、允许表述清单

以下表述有当前证据支撑，允许在对外材料中使用：

### 流量整形与特征隐匿

| 编号 | 允许表述 | 证据依据 |
|------|----------|----------|
| A-1 | 协议主源和运行时锚点已建立 | `source-of-truth.md` + `loader.go` |
| A-2 | eBPF 编译回归通过 | `bpf_compile_test.go` |
| A-3 | 关键 .c 文件（npm.c / bdna.c / jitter.c / chameleon.c）可真实编译 | `test-ebpf-compile.sh` |
| A-4 | NPM 支持三种包长填充模式（固定 MTU / 随机区间 / 高斯分布），在 XDP 层实现 | `npm.c` 源码 + 编译回归 |
| A-5 | B-DNA 已接管 TCP SYN 重写入口，支持 Window Size / MSS / WScale 等字段重写 | `bdna.c` 源码 + 编译回归 |
| A-6 | B-DNA 具备 JA4 捕获和 Ring Buffer 上报能力 | `bdna.c` 源码 + 编译回归 |
| A-7 | Jitter-Lite 通过 `skb->tstamp` 控制发送时机，支持 IAT 扰动 | `jitter.c` 源码 + 编译回归 |
| A-8 | VPC 可叠加物理噪声模型（光缆抖动、路由器延迟等） | `jitter.c` 源码 + 编译回归 |
| A-9 | B-DNA 对 TLS ClientHello / QUIC Initial 的处理方式为"内核标记 + 用户态协同"，不是内核完整重写 | `bdna.c` 源码 + `capability-truth-source.md` |

### eBPF 数据面

| 编号 | 允许表述 | 证据依据 |
|------|----------|----------|
| A-10 | eBPF 加载、Threat 事件、B-DNA / Jitter / L1 防护、编译回归均已具备证据 | `loader.go` + 编译回归 + `audit-report.md` |
| A-11 | 项目保留大量用户态处理链路（G-Tunnel 分片重组、FEC、QUIC 握手、TLS 完整重写、G-Switch 等） | `capability-truth-source.md` 已确认 |
| A-12 | eBPF 深度参与关键路径（XDP 包长控制、TC 指纹重写、TC 时域扰动、sockmap 加速） | `docs/reports/ebpf-coverage-map.md` 覆盖图路径对照表 + `artifacts/ebpf-perf/` 性能证据 |

## 四、不允许表述清单

以下表述缺少证据支撑，在对应实验结果产出前不得在对外材料中使用：

### 流量整形与特征隐匿

| 编号 | 不允许表述 | 缺失证据 | 解除条件 |
|------|------------|----------|----------|
| D-1 | DPI/ML 对抗效果达到 N 分 | 无独立对抗实验结果 | M6 分类器实验产出 AUC/F1 数据 |
| D-2 | 隐匿效果已显著领先同类产品 | 无对比实验结果 | 需独立对比实验 |
| D-3 | 可抵抗生产级 DPI/ML 系统 | 无真实对抗网络实验 | 需生产环境观测数据 |
| D-4 | 流量特征与真实 Chrome 浏览器无法区分 | 无握手指纹对比实验 | M6 检测面 1 实验结论 |
| D-5 | 包长分布已完全拟态目标流量 | 无包长分布实验 | M6 检测面 2 实验结论 |
| D-6 | 时序特征已无法被 ML 分类器识别 | 无时序分类实验 | M6 检测面 3 实验结论 |
| D-7 | 10 分隐匿效果 | 无任何实验支撑满额评分 | M6 全部检测面实验结论 |
| D-8 | 完整浏览器栈克隆 / 完整 TLS/HTTP2/HTTP3 行为克隆 | TLS/QUIC 仍依赖用户态协同，非内核完整重写 | 需完整协同链路验证 |

### eBPF 数据面

| 编号 | 不允许表述 | 缺失证据 | 解除条件 |
|------|------------|----------|----------|
| D-9 | 所有流量全链路零拷贝 | 覆盖图已产出（`docs/reports/ebpf-coverage-map.md`），用户态路径占比显著（G-Tunnel/FEC/QUIC/TLS/G-Switch 均为纯用户态） | 覆盖图证明用户态路径占比可忽略（当前不满足） |
| D-10 | eBPF 全流量全链路处理 | 覆盖图已确认大量用户态路径存在（`docs/reports/ebpf-coverage-map.md` §5, §6） | 覆盖图证明用户态路径占比可忽略（当前不满足） |
| D-11 | eBPF 数据面延迟 < 1ms（作为已验证事实） | 无性能观测数据 | M7 性能采集结论 |
| D-12 | eBPF 数据面 CPU < 5%（作为已验证事实） | 无性能观测数据 | M7 性能采集结论 |
| D-13 | 所有流量全链路零拷贝 | 覆盖图（`docs/reports/ebpf-coverage-map.md` §7）证明用户态路径占比显著（G-Tunnel 分片重组、FEC、QUIC 握手、TLS 完整重写、G-Switch 等均为纯用户态） | 覆盖图证明用户态路径占比可忽略（当前不满足） |

## 五、表述升级路径

### M6 实验完成后可评估升级的表述

| 当前不允许 | 升级条件 | 可能的升级表述 |
|------------|----------|----------------|
| D-1 | 分类器 AUC/F1 数据产出 | "受控环境下简单分类器 AUC 为 X"（限定表述） |
| D-4 | 握手指纹对比数据产出 | "TCP SYN 层面指纹与目标画像匹配度为 X%"（限定表述） |
| D-5 | 包长分布 KL/JS 散度数据产出 | "N 种填充模式的 JS 散度分别为 X/Y/Z"（限定表述） |
| D-6 | 时序分类实验数据产出 | "启用扰动后时序分类 AUC 从 X 降至 Y"（限定表述） |

### M7 实验完成后可评估升级的表述

| 当前不允许 | 升级条件 | 升级结果 |
|------------|----------|----------------|
| D-9 / D-10 | 覆盖图路径对照表产出 | ✅ 已升级为 A-12："eBPF 深度参与关键路径（XDP/TC/sockmap），但非全链路零拷贝"（限定表述） |
| D-11 | 延迟采集数据产出 | ⏳ 待 Linux 环境实际采集；覆盖图已标注采集方法和对照基准 |
| D-12 | CPU 采集数据产出 | ⏳ 待 Linux 环境实际采集；覆盖图已标注采集方法和对照基准 |

## 六、M6 实验结论回写（待更新）

> **当前状态：M6 实验已设计，受控环境基线待采集。**
>
> 以下表述升级/降级判定将在实验结果产出后更新。

### 待回写项

| 检测面 | 当前不允许表述 | 实验完成后判定 |
|--------|---------------|---------------|
| 握手指纹 | D-4: 流量特征与真实 Chrome 浏览器无法区分 | 待 M6 检测面 1 实验结论 |
| 包长分布 | D-5: 包长分布已完全拟态目标流量 | 待 M6 检测面 2 实验结论 |
| 时序分布 | D-6: 时序特征已无法被 ML 分类器识别 | 待 M6 检测面 3 实验结论 |
| 综合分类 | D-1: DPI/ML 对抗效果达到 N 分 | 待 M6 检测面 4 实验结论 |
| 综合评分 | D-7: 10 分隐匿效果 | 待 M6 全部检测面实验结论 |

### 回写规则

- 若实验证明某维度有效（AUC < 0.7）→ 对应限定表述可升级为允许
- 若实验证明某维度部分有效（AUC ∈ [0.7, 0.9]）→ 满额表述维持不允许，可增加限定表述
- 若实验证明某维度无效（AUC > 0.9）→ 满额表述维持不允许，标注"高可区分性风险"
- 所有升级后的表述必须附带证据强度标注（受控环境基线）

### 实验结果锚点

- 实验结果报告：`docs/reports/stealth-experiment-results.md`
- 实验执行编排：`deploy/scripts/drill-m6-experiment.sh`
- 抓包证据目录：`artifacts/dpi-audit/`

## 七、治理规则

1. 本文档的允许/不允许判定以 `capability-truth-source.md` 的验收标准为最终依据
2. 任何对外材料（销售矩阵、路演稿、官网话术）中涉及隐匿能力或 eBPF 数据面的表述，必须在本文档的允许清单范围内
3. 不允许清单中的表述，在对应实验结果产出并回写本文档之前，不得以任何形式对外使用
4. 实验结果产出后，表述升级必须附带证据强度标注（受控环境基线 / 生产环境观测）
5. 即使实验结果支撑某项表述升级，升级后的表述仍必须包含限定条件，不得使用绝对化词汇

## 八、引用文档

- `docs/governance/capability-truth-source.md` — 能力真相源（本文档的判定依据）
- `docs/governance/dpi-risk-audit-checklist.md` — DPI 风险审计清单
- `docs/protocols/source-of-truth.md` — 协议主权归属
- `docs/reports/stealth-experiment-plan.md` — 隐匿实验方案（M5 产出）
- `docs/reports/stealth-experiment-results.md` — 实验结果报告（M6 产出，待创建）
- `docs/reports/ebpf-coverage-map.md` — eBPF 覆盖图（M7 产出）
