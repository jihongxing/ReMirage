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

## Feature Gap Diagnostic

The formal diagnostic script is:

```bash
python3 artifacts/dpi-audit/classifier/analyze-feature-gap.py \
  --input artifacts/dpi-audit/classifier/features-m15-degraded.csv \
  --output artifacts/dpi-audit/classifier/feature-gap-m15-degraded.csv \
  --json-output artifacts/dpi-audit/classifier/feature-gap-m15-degraded.json
```

The first OpenCloudOS diagnostic run showed the top gaps:

| Rank | Feature | Effect Size | Control Mean | ReMirage Mean |
|------|---------|-------------|--------------|---------------|
| 1 | `tcp_mss` | 7.402 | 1446.108 | 1380.000 |
| 2 | `pkt_len_std` | 3.780 | 611.833 | 214.825 |
| 3 | `pkt_len_mean` | 2.911 | 645.918 | 1173.558 |
| 4 | `iat_p95` | 2.332 | 2507526.727 | 1507.429 |
| 5 | `iat_p99` | 2.123 | 34003914.749 | 1645.850 |

This indicates the classifier primarily separates stable MSS, packet-size distribution, and timing-scale differences.

## Calibrated Reference Candidate

The remediation experiment should use a separate calibrated metadata file, leaving the original simulation metadata intact:

```bash
python3 artifacts/dpi-audit/classifier/calibrate-remirage-reference.py \
  --baseline-root artifacts/dpi-audit/baseline \
  --input-metadata artifacts/dpi-audit/simulation-metadata.json \
  --output-metadata artifacts/dpi-audit/simulation-metadata-calibrated.json
```

Then rerun the degraded feature builder against the calibrated reference:

```bash
python3 artifacts/dpi-audit/classifier/build-m15-degraded-features.py \
  --baseline-root artifacts/dpi-audit/baseline \
  --simulation-metadata artifacts/dpi-audit/simulation-metadata-calibrated.json \
  --output artifacts/dpi-audit/classifier/features-m15-calibrated.csv \
  --metadata-output artifacts/dpi-audit/classifier/m15-calibrated-metadata.json

python3 artifacts/dpi-audit/classifier/train-classifier.py \
  -i artifacts/dpi-audit/classifier/features-m15-calibrated.csv \
  -o artifacts/dpi-audit/classifier/results-m15-calibrated.json
```

This calibrated run is still not upgrade-eligible. It is a remediation experiment to verify whether baseline-driven feature calibration reduces classifier shortcuts before collecting real ReMirage pcap-derived samples.

## Calibrated Reference Result

The OpenCloudOS remediation experiment using `simulation-metadata-calibrated.json` completed successfully.

Input:

- Control: same `chrome-win` + `firefox-linux` real M13 baseline bootstrap, 120 rows.
- ReMirage: calibrated simulation/reference rows generated from completed baseline families, 120 rows.
- Missing: `chrome-macos`.
- Classifier: RandomForest only; XGBoost unavailable on target.

| Experiment | Feature Set | AUC | F1 | Accuracy | CV AUC Mean | Interpretation |
|------------|-------------|-----|----|----------|-------------|----------------|
| C1 | Handshake | 0.5293 | 0.5479 | 0.5417 | 0.4983 | low separability in calibrated reference |
| C2 | Packet Length | 0.4294 | 0.4571 | 0.4722 | 0.4497 | low separability in calibrated reference |
| C3 | Timing | 0.3920 | 0.4474 | 0.4167 | 0.5149 | low separability in calibrated reference |
| C4 | Combined | 0.3931 | 0.4658 | 0.4583 | 0.4656 | low separability in calibrated reference |

Compared with the degraded reference run, the classifier moved from `AUC=1.0` on every surface to near-random separability on the calibrated candidate. This confirms the remediation hypothesis: stable MSS, packet-size distribution, and timing-scale mismatches were the dominant classifier shortcuts.

Boundary:

- This is still calibrated simulation/reference evidence, not real ReMirage traffic evidence.
- It can guide implementation of real data-plane/profile calibration.
- It cannot upgrade the stealth capability state.
- The next evidence target is real ReMirage-side pcap-derived feature extraction under the no-UDP TCP/WSS deployment.
