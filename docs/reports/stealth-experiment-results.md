# Stealth Experiment Results

> Status: simulated-reference
> Evidence Strength: 模拟环境参考
> Source: `artifacts/dpi-audit/`
> Generated From: `deploy/evidence/m6-experiment-drill.log`

## Summary

当前 M6 隐匿实验链路已能端到端运行：样本生成、握手特征提取、包长分布分析、时序分析、分类器特征构建和 RandomForest 训练均已完成。

但本轮数据来自模拟样本，不能作为 Linux 受控环境基线，也不能作为生产 DPI/ML 对抗效果证明。当前结论只能用于验证实验管线和暴露风险方向。

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

## Classifier Results

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

## Claim Boundary

允许表述：M6 实验管线已跑通，当前模拟样本暴露出高可区分性风险。

不允许表述：不得宣称已抵抗 DPI/ML，不得宣称与 Chrome/真实浏览器流量不可区分，不得把当前结果写成 Linux 受控基线或生产环境观测。

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
