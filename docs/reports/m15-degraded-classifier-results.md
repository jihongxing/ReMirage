# M15 Degraded Classifier Results

> Status: M15-degraded completed
> Evidence Strength: 两族真实 M13 baseline bootstrap + 当前 ReMirage 模拟参考样本
> Date: 2026-04-28
> Source: `artifacts/dpi-audit/classifier/results-m15-degraded.json`

## Summary

M15 degraded 分类器复验已在 OpenCloudOS 目标节点完成。输入为：

- Control: `chrome-win` 与 `firefox-linux` 两族真实 M13 baseline 的统计/CDF bootstrap。
- ReMirage: 当前 `artifacts/dpi-audit/simulation-metadata.json` 中 `label=1` 的参考样本。
- Missing: `chrome-macos` 原生采集缺失。

本轮结果不能用于 Capability-Upgrade Gate。更重要的是，RandomForest 在四个检测面均得到 `AUC=1.0`、`F1=1.0`、`Accuracy=1.0`，说明当前 ReMirage 参考样本相对两族真实 baseline 仍高度可分，未观察到风险下降。

## Evidence Metadata

| Item | Value |
|------|-------|
| Mode | `M15-degraded` |
| Upgrade Eligible | `false` |
| Control Families | `chrome-win`, `firefox-linux` |
| Missing Families | `chrome-macos` |
| Control Rows | 120 |
| ReMirage Rows | 120 |
| Classifier | RandomForest |
| XGBoost | unavailable on target |

Family status:

| Family | Status | Connection Count | Packet Count | Pcap Count |
|--------|--------|------------------|--------------|------------|
| `chrome-win` | complete | 102 | 2011 | 2 |
| `firefox-linux` | complete | 9505 | 406637 | 1 |
| `chrome-macos` | missing | 0 | 0 | 0 |

## Classifier Results

| Experiment | Feature Set | AUC | F1 | Accuracy | Risk |
|------------|-------------|-----|----|----------|------|
| C1 | Handshake | 1.0 | 1.0 | 1.0 | 高可区分性风险 |
| C2 | Packet Length | 1.0 | 1.0 | 1.0 | 高可区分性风险 |
| C3 | Timing | 1.0 | 1.0 | 1.0 | 高可区分性风险 |
| C4 | Combined | 1.0 | 1.0 | 1.0 | 综合高可区分性风险 |

## Interpretation

Allowed conclusions:

- M15 degraded feature generation and classifier rerun are operational.
- The current ReMirage reference sample remains trivially separable from the two completed real baseline families.
- No capability status upgrade is allowed.
- No “risk下降” claim is supported by this run.

Not allowed:

- Do not claim M13-full or M15-full.
- Do not claim DPI/ML resistance.
- Do not claim browser-like indistinguishability.
- Do not treat bootstrap control rows or simulated ReMirage rows as production traffic evidence.

## Next Engineering Target

The result points to three likely gaps:

1. Handshake: ReMirage reference rows still expose stable TCP/TLS constants relative to browser baselines.
2. Packet length: generated ReMirage packet lengths remain distributionally distinct from M13 baseline CDFs.
3. Timing: generated ReMirage IAT values remain far from observed browser/network timing.

The next pass should prioritize real ReMirage-side sample extraction and feature calibration before attempting another classifier gate.
