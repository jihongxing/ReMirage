# Stealth Experiment Results

> Status: simulated-reference + M13-degraded + M15-degraded completed
> Evidence Strength: 模拟环境参考；两族真实原生 OS 基线；M15 degraded 复验未达标
> Source: `artifacts/dpi-audit/`
> Generated From: `deploy/evidence/m6-experiment-drill.log`

## Summary

当前 M6 隐匿实验链路已能端到端运行：样本生成、握手特征提取、包长分布分析、时序分析、分类器特征构建和 RandomForest 训练均已完成。

但本轮数据来自模拟样本，不能作为 Linux 受控环境基线，也不能作为生产 DPI/ML 对抗效果证明。当前结论只能用于验证实验管线和暴露风险方向。

2026-04-28 更新：M13 真实基线已完成 `firefox-linux` 与 `chrome-win` 两族采集，但缺少 `chrome-macos` 原生采集节点，因此结果为 `M13-degraded`。后续 M15 可以继续作为降级复验推进，但不得作为能力状态升级依据。

2026-04-28 更新：新增 `build-m15-degraded-features.py`，用于把两族真实 M13 baseline 与当前 ReMirage 参考样本合成为 `features-m15-degraded.csv`。该输出明确标注 `upgrade_eligible=false`，只能作为 M15 风险趋势复验输入。

2026-04-28 更新：M15 degraded 分类器复验已完成。RandomForest 在 C1/C2/C3/C4 四个检测面均为 `AUC=1.0`、`F1=1.0`、`Accuracy=1.0`，风险未下降，能力状态不得升级。

## Evidence Inputs

| Evidence | Path | Status |
|----------|------|--------|
| 模拟样本说明 | `artifacts/dpi-audit/SIMULATION_NOTICE.md` | present |
| 样本元数据 | `artifacts/dpi-audit/simulation-metadata.json` | present |
| 握手特征 | `artifacts/dpi-audit/handshake/comparison.csv` | present |
| 包长分布 | `artifacts/dpi-audit/packet-length/distributions.csv` | present |
| 时序统计 | `artifacts/dpi-audit/timing/iat-stats.csv` | present |
| 分类器特征 | `artifacts/dpi-audit/classifier/features.csv` | present |
| 分类器结果 | `artifacts/dpi-audit/classifier/results.json` | present |
| M13 降级真实基线报告 | `docs/reports/m13-real-baseline-degraded.md` | present |
| M15 降级特征构建脚本 | `artifacts/dpi-audit/classifier/build-m15-degraded-features.py` | present |
| M15 降级特征 | `artifacts/dpi-audit/classifier/features-m15-degraded.csv` | generated on target, not committed |
| M15 降级分类器结果 | `artifacts/dpi-audit/classifier/results-m15-degraded.json` | generated on target, not committed |
| M15 降级结果报告 | `docs/reports/m15-degraded-classifier-results.md` | present |

## Classifier Results — M6 Simulated Reference

| Experiment | Feature Set | Classifier | AUC | F1 | Accuracy | Risk |
|------------|-------------|------------|-----|----|----------|------|
| C1 | 握手特征 | RandomForest | 1.0 | 1.0 | 1.0 | 高可区分性风险 |
| C2 | 包长特征 | RandomForest | 1.0 | 1.0 | 1.0 | 高可区分性风险 |
| C3 | 时序特征 | RandomForest | 1.0 | 1.0 | 1.0 | 高可区分性风险 |
| C4 | 握手+包长+时序 | RandomForest | 1.0 | 1.0 | 1.0 | 综合高可区分性风险 |

## Classifier Results — M15 Degraded

Input:

- Control: `chrome-win` + `firefox-linux` real M13 baseline bootstrap, 120 rows.
- ReMirage: current `simulation-metadata.json` label=1 reference samples, 120 rows.
- Missing: `chrome-macos`.
- Classifier: RandomForest only; XGBoost unavailable on target.

| Experiment | Feature Set | Classifier | AUC | F1 | Accuracy | Risk |
|------------|-------------|------------|-----|----|----------|------|
| C1 | 握手特征 | RandomForest | 1.0 | 1.0 | 1.0 | 高可区分性风险 |
| C2 | 包长特征 | RandomForest | 1.0 | 1.0 | 1.0 | 高可区分性风险 |
| C3 | 时序特征 | RandomForest | 1.0 | 1.0 | 1.0 | 高可区分性风险 |
| C4 | 握手+包长+时序 | RandomForest | 1.0 | 1.0 | 1.0 | 综合高可区分性风险 |

## Observations

握手侧：模拟数据中 `remirage` 与 `chrome` 的 TCP MSS、TLS extension count 和 extension sequence 仍可区分。

包长侧：三种 padding 模式都显著改变分布，但相对 baseline 的 JS divergence 仍较高，不能证明“拟态真实浏览器流量”。

时序侧：Jitter/VPC 会扩大 IAT 方差，但模拟分类器仍能区分样本类别。

综合分类：RandomForest 在 240 个模拟样本上达到 AUC/F1/Accuracy 全 1.0，说明当前模拟特征集存在强可分性。

M15 degraded 复验：将两族真实 baseline 引入 control 后，当前 ReMirage 参考样本仍被 RandomForest 完全区分。该结果说明当前 M14/M15 链条还没有产生可量化的分类风险下降，需要继续修正 ReMirage 样本侧的握手、包长和时序特征。

## Claim Boundary

允许表述：M6 实验管线已跑通，当前模拟样本暴露出高可区分性风险。

不允许表述：不得宣称已抵抗 DPI/ML，不得宣称与 Chrome/真实浏览器流量不可区分，不得把当前结果写成 Linux 受控基线或生产环境观测。

M13-degraded 限制：当前只覆盖 `firefox-linux` 与 `chrome-win`，不覆盖 `chrome-macos`。任何后续 M15 改善只能表述为“风险下降的部分证据”，不能触发 Capability-Upgrade Gate。

M15-degraded 限制：`features-m15-degraded.csv` 的 control 侧来自真实 M13 两族 baseline 的统计/CDF bootstrap；ReMirage 侧来自当前 `simulation-metadata.json` 中 label=1 的参考样本。该结果可用于比较风险趋势，但不能替代真实 ReMirage pcap 派生样本，也不能用于能力状态升级。本轮结果为四项 AUC 全 1.0，因此不支持任何风险下降或能力升级表述。

## Upgrade Conditions

要把本报告从 `simulated-reference` 升级为 `controlled-linux-baseline`，必须在 Linux 环境重新执行：

```bash
bash deploy/scripts/drill-m6-experiment.sh
```

并满足以下条件：

- 不生成 `artifacts/dpi-audit/simulation-metadata.json`
- `deploy/evidence/m6-experiment-drill.log` 记录为完整受控环境基线，而不是降级模式
- 抓包文件来自真实网络路径，而不是 `generate-simulated-samples.py`
- 分类器结果和限制说明回写本文
