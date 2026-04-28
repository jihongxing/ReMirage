#!/usr/bin/env python3
"""Extract classifier-compatible label=1 rows from real ReMirage pcaps."""

from __future__ import annotations

import argparse
import csv
import glob
import json
import math
import statistics
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

try:
    from scapy.all import IP, TCP, UDP, PcapReader
except Exception as exc:  # pragma: no cover - environment guard
    print(f"scapy is required to parse pcapng files: {exc}", file=sys.stderr)
    sys.exit(2)


FEATURE_COLUMNS = [
    "label",
    "tcp_window",
    "tcp_mss",
    "tcp_wscale",
    "tcp_sack",
    "tcp_timestamps",
    "tls_ext_count",
    "pkt_len_1",
    "pkt_len_2",
    "pkt_len_3",
    "pkt_len_4",
    "pkt_len_5",
    "pkt_len_6",
    "pkt_len_7",
    "pkt_len_8",
    "pkt_len_9",
    "pkt_len_10",
    "pkt_dir_1",
    "pkt_dir_2",
    "pkt_dir_3",
    "pkt_dir_4",
    "pkt_dir_5",
    "pkt_dir_6",
    "pkt_dir_7",
    "pkt_dir_8",
    "pkt_dir_9",
    "pkt_dir_10",
    "up_down_ratio",
    "pkt_len_entropy",
    "pkt_len_mean",
    "pkt_len_std",
    "iat_mean",
    "iat_std",
    "iat_p50",
    "iat_p95",
    "iat_p99",
    "burst_count",
    "burst_mean_size",
    "burst_mean_interval",
]


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


def entropy(values: list[int]) -> float:
    if not values:
        return 0.0
    counts: dict[int, int] = {}
    for value in values:
        counts[value] = counts.get(value, 0) + 1
    total = len(values)
    return -sum((count / total) * math.log2(count / total) for count in counts.values())


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


def canonical_flow(ip: Any, l4: Any, proto: str) -> tuple:
    a = (str(ip.src), int(l4.sport))
    b = (str(ip.dst), int(l4.dport))
    return (proto, a, b) if a <= b else (proto, b, a)


def direction(ip: Any, l4: Any, gateway_ip: str, ports: set[int]) -> int:
    if gateway_ip:
        if str(ip.dst) == gateway_ip:
            return 1
        if str(ip.src) == gateway_ip:
            return -1
    if int(l4.dport) in ports:
        return 1
    if int(l4.sport) in ports:
        return -1
    return 1


def parse_ports(value: str) -> set[int]:
    return {int(item.strip()) for item in value.replace(",", " ").split() if item.strip()}


def discover_pcaps(input_path: Path) -> list[Path]:
    if input_path.is_file():
        return [input_path]
    pcaps = sorted(Path(p) for p in glob.glob(str(input_path / "*.pcapng")))
    pcaps += sorted(Path(p) for p in glob.glob(str(input_path / "*.pcap")))
    return pcaps


def parse_pcaps(pcaps: list[Path], gateway_ip: str, ports: set[int]) -> dict[tuple, dict[str, Any]]:
    flows: dict[tuple, dict[str, Any]] = {}
    for pcap in pcaps:
        try:
            reader = PcapReader(str(pcap))
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
                elif UDP in pkt:
                    l4 = pkt[UDP]
                    proto = "udp"
                if l4 is None:
                    continue
                if ports and int(l4.sport) not in ports and int(l4.dport) not in ports:
                    continue

                key = canonical_flow(ip, l4, proto)
                state = flows.setdefault(key, {
                    "packets": [],
                    "iats": [],
                    "last_ts": None,
                    "syn": None,
                    "proto": proto,
                })
                if state["last_ts"] is not None:
                    state["iats"].append(max(0.0, (ts - float(state["last_ts"])) * 1_000_000.0))
                state["last_ts"] = ts
                pkt_dir = direction(ip, l4, gateway_ip, ports)
                state["packets"].append({
                    "ts": ts,
                    "len": int(len(pkt)),
                    "dir": pkt_dir,
                })

                if proto == "tcp" and state["syn"] is None and (int(l4.flags) & 0x02):
                    opts = tcp_options(l4)
                    state["syn"] = {
                        "tcp_window": int(l4.window),
                        "tcp_mss": opts["mss"],
                        "tcp_wscale": opts["wscale"],
                        "tcp_sack": opts["sack"],
                        "tcp_timestamps": opts["timestamps"],
                    }
    return flows


def burst_stats(iats: list[float], threshold_us: float = 100.0) -> tuple[int, float, float]:
    burst_sizes: list[int] = []
    burst_gaps: list[float] = []
    current_size = 0
    gap_sum = 0.0
    for value in iats:
        if value < threshold_us:
            current_size += 1
            if gap_sum > 0:
                burst_gaps.append(gap_sum)
                gap_sum = 0.0
        else:
            gap_sum += value
            if current_size > 0:
                burst_sizes.append(current_size + 1)
                current_size = 0
    if current_size > 0:
        burst_sizes.append(current_size + 1)
    return (
        len(burst_sizes),
        round(statistics.fmean(burst_sizes), 6) if burst_sizes else 0.0,
        round(statistics.fmean(burst_gaps), 6) if burst_gaps else 0.0,
    )


def flow_to_row(flow: dict[str, Any]) -> dict[str, Any] | None:
    packets = sorted(flow["packets"], key=lambda item: item["ts"])
    if len(packets) < 2:
        return None
    first = packets[:10]
    lengths = [int(item["len"]) for item in first]
    dirs = [int(item["dir"]) for item in first]
    while len(lengths) < 10:
        lengths.append(0)
        dirs.append(0)

    up_bytes = sum(int(item["len"]) for item in packets if int(item["dir"]) > 0)
    down_bytes = sum(int(item["len"]) for item in packets if int(item["dir"]) < 0)
    all_lengths = [int(item["len"]) for item in packets]
    iats = [float(value) for value in flow["iats"]]
    syn = flow.get("syn") or {}
    burst_count, burst_mean_size, burst_mean_interval = burst_stats(iats)

    row: dict[str, Any] = {
        "label": 1,
        "tcp_window": syn.get("tcp_window", 0),
        "tcp_mss": syn.get("tcp_mss", 0),
        "tcp_wscale": syn.get("tcp_wscale", 0),
        "tcp_sack": syn.get("tcp_sack", 0),
        "tcp_timestamps": syn.get("tcp_timestamps", 0),
        "tls_ext_count": 0,
        "up_down_ratio": round(up_bytes / max(1, down_bytes), 6),
        "pkt_len_entropy": round(entropy(all_lengths), 6),
        "pkt_len_mean": round(statistics.fmean(all_lengths), 6),
        "pkt_len_std": round(statistics.pstdev(all_lengths), 6) if len(all_lengths) > 1 else 0.0,
        "iat_mean": round(statistics.fmean(iats), 6) if iats else 0.0,
        "iat_std": round(statistics.pstdev(iats), 6) if len(iats) > 1 else 0.0,
        "iat_p50": round(percentile(iats, 0.50), 6),
        "iat_p95": round(percentile(iats, 0.95), 6),
        "iat_p99": round(percentile(iats, 0.99), 6),
        "burst_count": burst_count,
        "burst_mean_size": burst_mean_size,
        "burst_mean_interval": burst_mean_interval,
    }
    for idx, value in enumerate(lengths, start=1):
        row[f"pkt_len_{idx}"] = value
    for idx, value in enumerate(dirs, start=1):
        row[f"pkt_dir_{idx}"] = value
    return {column: row.get(column, 0) for column in FEATURE_COLUMNS}


def write_csv(path: Path, rows: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=FEATURE_COLUMNS)
        writer.writeheader()
        writer.writerows(rows)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Extract real ReMirage pcap-derived classifier metadata")
    parser.add_argument("--input", required=True, help="Input pcap file or directory")
    parser.add_argument(
        "--output-metadata",
        default="artifacts/dpi-audit/remirage/remirage-real-metadata.json",
        help="Output metadata JSON with classifier_samples label=1",
    )
    parser.add_argument(
        "--output-csv",
        default="artifacts/dpi-audit/remirage/remirage-real-features.csv",
        help="Output label=1 feature CSV for inspection",
    )
    parser.add_argument("--gateway-ip", default="", help="Gateway public/private IP used for direction inference")
    parser.add_argument("--ports", default="8443 50847", help="Capture ports to include")
    parser.add_argument("--min-flows", type=int, default=20)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    input_path = Path(args.input)
    ports = parse_ports(args.ports)
    pcaps = discover_pcaps(input_path)
    if not pcaps:
        raise SystemExit(f"no pcapng/pcap files found in {input_path}")

    flows = parse_pcaps(pcaps, args.gateway_ip, ports)
    rows = []
    skipped = 0
    for flow in flows.values():
        row = flow_to_row(flow)
        if row is None:
            skipped += 1
            continue
        rows.append(row)

    if len(rows) < args.min_flows:
        print(
            f"warning: extracted only {len(rows)} flows < min_flows {args.min_flows}; "
            "classifier result will be weak",
            file=sys.stderr,
        )
    if not rows:
        raise SystemExit("no classifier rows extracted")

    output_metadata = Path(args.output_metadata)
    output_csv = Path(args.output_csv)
    write_csv(output_csv, rows)
    metadata = {
        "mode": "real_remirage_pcap",
        "evidence_strength": "real ReMirage-side pcap-derived",
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "input": str(input_path),
        "pcaps": [str(path) for path in pcaps],
        "gateway_ip": args.gateway_ip,
        "ports": sorted(ports),
        "flow_count": len(flows),
        "classifier_sample_count": len(rows),
        "skipped_short_flows": skipped,
        "upgrade_eligible": False,
        "limitations": [
            "M13 remains degraded until chrome-macos native baseline is present.",
            "This is real ReMirage-side pcap-derived evidence, but it must still be reviewed for sample size and traffic generation quality.",
        ],
        "classifier_samples": rows,
    }
    output_metadata.parent.mkdir(parents=True, exist_ok=True)
    output_metadata.write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"extracted {len(rows)} classifier rows from {len(pcaps)} pcap file(s)")
    print(f"wrote {output_csv}")
    print(f"wrote {output_metadata}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
