#!/usr/bin/env python3
"""Extract per-family baseline statistics from captured pcapng files.

The extractor intentionally produces two levels of evidence:
  - baseline/<family>/baseline-stats.csv and baseline-distribution.json
  - baseline/baseline-stats-merged.csv and baseline-distribution-merged.json

The merged files are used by this phase's global NPM/Jitter calibration.
"""

from __future__ import annotations

import csv
import glob
import json
import math
import statistics
import sys
from collections import defaultdict
from pathlib import Path

try:
    from scapy.all import IP, TCP, UDP, PcapReader
except Exception as exc:  # pragma: no cover - environment guard
    print(f"scapy is required to parse pcapng files: {exc}", file=sys.stderr)
    sys.exit(2)


FAMILIES = ("chrome-win", "chrome-macos", "firefox-linux")
BIN_COUNT = 256
MAX_LEN = 1500


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0.0
    data = sorted(values)
    pos = (len(data) - 1) * pct
    lo = math.floor(pos)
    hi = math.ceil(pos)
    if lo == hi:
        return data[int(pos)]
    return data[lo] * (hi - pos) + data[hi] * (pos - lo)


def tcp_options(pkt: TCP) -> dict[str, int]:
    out = {"mss": 0, "wscale": 0, "sack": 0, "timestamps": 0}
    for kind, value in pkt.options:
        if kind == "MSS":
            out["mss"] = int(value)
        elif kind == "WScale":
            out["wscale"] = int(value)
        elif kind == "SAckOK":
            out["sack"] = 1
        elif kind == "Timestamp":
            out["timestamps"] = 1
    return out


def flow_key(ip, l4, proto: str) -> tuple:
    return (ip.src, ip.dst, int(l4.sport), int(l4.dport), proto)


def analyze_family(family_dir: Path, family: str) -> dict:
    pcaps = sorted(glob.glob(str(family_dir / "*.pcapng"))) + sorted(glob.glob(str(family_dir / "*.pcap")))
    flows: dict[tuple, dict] = {}
    lengths: list[int] = []
    iats: list[float] = []
    tcp_windows: list[int] = []
    tcp_mss: list[int] = []
    tcp_wscale: list[int] = []
    tcp_sack: list[int] = []
    tcp_timestamps: list[int] = []

    for pcap in pcaps:
        try:
            reader = PcapReader(pcap)
        except Exception as exc:
            print(f"warning: cannot read {pcap}: {exc}", file=sys.stderr)
            continue
        with reader:
            for pkt in reader:
                if IP not in pkt:
                    continue
                ip = pkt[IP]
                ts = float(pkt.time)
                l4 = None
                proto = ""
                if TCP in pkt:
                    l4 = pkt[TCP]
                    proto = "tcp"
                    if l4.flags & 0x02:  # SYN
                        opts = tcp_options(l4)
                        tcp_windows.append(int(l4.window))
                        tcp_mss.append(opts["mss"])
                        tcp_wscale.append(opts["wscale"])
                        tcp_sack.append(opts["sack"])
                        tcp_timestamps.append(opts["timestamps"])
                elif UDP in pkt:
                    l4 = pkt[UDP]
                    proto = "udp"
                if l4 is None:
                    continue

                key = flow_key(ip, l4, proto)
                state = flows.setdefault(key, {"last_ts": None, "count": 0})
                if state["last_ts"] is not None:
                    iats.append(max(0.0, (ts - state["last_ts"]) * 1_000_000.0))
                state["last_ts"] = ts
                state["count"] += 1
                lengths.append(int(len(pkt)))

    connection_count = len(flows)
    mean_len = statistics.fmean(lengths) if lengths else 0.0
    std_len = statistics.pstdev(lengths) if len(lengths) > 1 else 0.0
    mean_iat = statistics.fmean(iats) if iats else 0.0
    std_iat = statistics.pstdev(iats) if len(iats) > 1 else 0.0

    return {
        "profile_family": family,
        "connection_count": connection_count,
        "packet_count": len(lengths),
        "tcp_window": round(statistics.fmean(tcp_windows)) if tcp_windows else 0,
        "tcp_mss": round(statistics.fmean([v for v in tcp_mss if v])) if any(tcp_mss) else 0,
        "tcp_wscale": round(statistics.fmean(tcp_wscale)) if tcp_wscale else 0,
        "tcp_sack_ok": round(statistics.fmean(tcp_sack)) if tcp_sack else 0,
        "tcp_timestamps": round(statistics.fmean(tcp_timestamps)) if tcp_timestamps else 0,
        "packet_len_mean": mean_len,
        "packet_len_std": std_len,
        "iat_mean_us": mean_iat,
        "iat_std_us": std_iat,
        "iat_p50_us": percentile(iats, 0.50),
        "iat_p95_us": percentile(iats, 0.95),
        "iat_p99_us": percentile(iats, 0.99),
        "lengths": lengths,
    }


def distribution(lengths: list[int]) -> dict:
    width = math.ceil((MAX_LEN + 1) / BIN_COUNT)
    counts = [0 for _ in range(BIN_COUNT)]
    for value in lengths:
        idx = min(BIN_COUNT - 1, max(0, int(value) // width))
        counts[idx] += 1

    total = sum(counts)
    cumulative = 0
    bins = []
    for idx, count in enumerate(counts):
        low = idx * width
        high = min(MAX_LEN, ((idx + 1) * width) - 1)
        cumulative += count
        prob = (count / total) if total else 0.0
        cdf = (cumulative / total) if total else 0.0
        bins.append({
            "low": low,
            "high": high,
            "count": count,
            "probability": prob,
            "cumulative_prob": cdf,
        })
    return {"bin_count": BIN_COUNT, "max_len": MAX_LEN, "bins": bins}


def write_stats(path: Path, row: dict) -> None:
    fields = [
        "profile_family",
        "connection_count",
        "packet_count",
        "tcp_window",
        "tcp_mss",
        "tcp_wscale",
        "tcp_sack_ok",
        "tcp_timestamps",
        "packet_len_mean",
        "packet_len_std",
        "iat_mean_us",
        "iat_std_us",
        "iat_p50_us",
        "iat_p95_us",
        "iat_p99_us",
    ]
    with path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fields)
        writer.writeheader()
        writer.writerow({field: row.get(field, "") for field in fields})


def main() -> int:
    root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("artifacts/dpi-audit/baseline")
    root.mkdir(parents=True, exist_ok=True)

    merged_lengths: list[int] = []
    merged_rows = []
    weighted_iat_mean = 0.0
    weighted_iat_std = 0.0
    total_packets = 0

    for family in FAMILIES:
        family_dir = root / family
        family_dir.mkdir(parents=True, exist_ok=True)
        result = analyze_family(family_dir, family)
        lengths = result.pop("lengths")
        merged_lengths.extend(lengths)
        merged_rows.append(result)

        weight = max(1, int(result["packet_count"]))
        total_packets += weight
        weighted_iat_mean += float(result["iat_mean_us"]) * weight
        weighted_iat_std += float(result["iat_std_us"]) * weight

        write_stats(family_dir / "baseline-stats.csv", result)
        (family_dir / "baseline-distribution.json").write_text(
            json.dumps(distribution(lengths), indent=2),
            encoding="utf-8",
        )

    merged = {
        "profile_family": "merged",
        "connection_count": sum(int(row["connection_count"]) for row in merged_rows),
        "packet_count": sum(int(row["packet_count"]) for row in merged_rows),
        "tcp_window": 0,
        "tcp_mss": 0,
        "tcp_wscale": 0,
        "tcp_sack_ok": 0,
        "tcp_timestamps": 0,
        "packet_len_mean": statistics.fmean(merged_lengths) if merged_lengths else 0.0,
        "packet_len_std": statistics.pstdev(merged_lengths) if len(merged_lengths) > 1 else 0.0,
        "iat_mean_us": weighted_iat_mean / total_packets if total_packets else 0.0,
        "iat_std_us": weighted_iat_std / total_packets if total_packets else 0.0,
        "iat_p50_us": 0.0,
        "iat_p95_us": 0.0,
        "iat_p99_us": 0.0,
    }
    write_stats(root / "baseline-stats-merged.csv", merged)
    (root / "baseline-distribution-merged.json").write_text(
        json.dumps(distribution(merged_lengths), indent=2),
        encoding="utf-8",
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
