# M13 Real Baseline Degraded Report

> Status: M13-degraded
> Evidence Strength: 两族真实原生 OS 采集，三族基线不完整
> Date: 2026-04-28
> Source: `artifacts/dpi-audit/baseline/`

## Summary

M13 真实对照基线采集已完成 `firefox-linux` 与 `chrome-win` 两个画像族。由于当前没有 macOS 原生采集节点，`chrome-macos` 缺失，因此本轮不能判定为 `M13-full`。

本轮结果可以继续用于 M15 的降级复验和风险下降评估，但不能用于 Capability-Upgrade Gate。能力域状态必须保持“部分实现”，直到 `chrome-macos` 原生采集补齐且 M15 AUC 门禁达标。

## Family Status

| Profile Family | Native OS | Status | Connection Count | Packet Count | Evidence |
|----------------|-----------|--------|------------------|--------------|----------|
| `firefox-linux` | Linux / OpenCloudOS | DONE | 9505 | 406637 | pcapng + metadata + stats + distribution |
| `chrome-win` | Windows 11 | DONE | 102 | 2011 | pcapng + metadata + stats + distribution |
| `chrome-macos` | macOS | MISSING | 0 | 0 | no macOS capture node |

## Captured Metrics

### firefox-linux

| Metric | Value |
|--------|-------|
| `connection_count` | 9505 |
| `packet_count` | 406637 |
| `tcp_window` | 57474 |
| `tcp_mss` | 1444 |
| `tcp_wscale` | 8 |
| `tcp_sack_ok` | 1 |
| `tcp_timestamps` | 1 |
| `packet_len_mean` | 759.9096491465361 |
| `packet_len_std` | 696.7411869051433 |
| `iat_mean_us` | 466140.5792247903 |
| `iat_std_us` | 7040856.509500313 |

### chrome-win

| Metric | Value |
|--------|-------|
| `connection_count` | 102 |
| `packet_count` | 2011 |
| `tcp_window` | 63817 |
| `tcp_mss` | 1445 |
| `tcp_wscale` | 8 |
| `tcp_sack_ok` | 1 |
| `tcp_timestamps` | 0 |
| `packet_len_mean` | 576.553953257086 |
| `packet_len_std` | 704.0563126239512 |
| `iat_mean_us` | 3168449.3084363504 |
| `iat_std_us` | 36796078.06656563 |

## Gate Result

`verify-m13-full.py` result:

```text
M13-degraded
- chrome-macos: no pcapng/pcap files
- chrome-macos: missing capture-metadata.json
- chrome-macos: connection_count 0 < 100
```

The `chrome-win` failure was cleared after supplementing the Windows capture from 95 to 102 connections.

## Interpretation

Allowed conclusions:

- The M13 capture and extraction pipeline works on Linux Firefox and Windows Chrome native environments.
- The project now has real baseline inputs for two out of three required browser families.
- M15 may continue as a degraded rerun using available real families, with any result labeled as partial evidence.

Not allowed:

- Do not claim M13-full.
- Do not use this evidence to upgrade stealth capability status.
- Do not claim Chrome macOS behavior is covered or approximated by Windows/Linux data.

## Next Step

Proceed with M15 as `M15-degraded`:

1. Use the available `firefox-linux` and `chrome-win` baselines as real control evidence.
2. Generate or collect comparable ReMirage-side samples under the current TCP/WSS, no-UDP deployment.
3. Rerun classifier experiments and record AUC/F1/Accuracy as risk trend evidence only.

