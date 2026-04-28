#!/usr/bin/env python3
"""Generate calibrated ReMirage reference metadata from M13 baseline.

This does not produce real ReMirage traffic evidence. It creates a candidate
metadata file that uses completed M13 family stats/CDFs to calibrate the
label=1 rows for the next classifier iteration.
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import random
import statistics
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


FAMILIES = ("chrome-win", "chrome-macos", "firefox-linux")
DEFAULT_SEED = 20260428
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
TLS_EXT_COUNT_HEURISTIC = {
    "chrome-win": 12,
    "chrome-macos": 12,
    "firefox-linux": 10,
}


def read_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8-sig"))


def read_stats(path: Path) -> dict[str, Any]:
    with path.open("r", newline="", encoding="utf-8-sig") as handle:
        rows = list(csv.DictReader(handle))
    if not rows:
        raise ValueError(f"stats file has no rows: {path}")
    return rows[0]


def numeric(value: Any, default: float = 0.0) -> float:
    if value in (None, ""):
        return default
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def integer(value: Any, default: int = 0) -> int:
    return int(round(numeric(value, float(default))))


def clamp(value: float, lower: float, upper: float) -> float:
    return max(lower, min(upper, value))


def discover_families(root: Path, min_connections: int) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    complete: list[dict[str, Any]] = []
    statuses: list[dict[str, Any]] = []
    for family in FAMILIES:
        family_dir = root / family
        status: dict[str, Any] = {"family": family, "status": "missing", "errors": []}
        stats_path = family_dir / "baseline-stats.csv"
        dist_path = family_dir / "baseline-distribution.json"
        meta_path = family_dir / "capture-metadata.json"
        pcaps = sorted(family_dir.glob("*.pcapng")) + sorted(family_dir.glob("*.pcap"))
        nonempty_pcaps = [path for path in pcaps if path.stat().st_size > 0]

        if not stats_path.exists():
            status["errors"].append("baseline-stats.csv missing")
        if not dist_path.exists():
            status["errors"].append("baseline-distribution.json missing")
        if not meta_path.exists():
            status["errors"].append("capture-metadata.json missing")
        if not nonempty_pcaps:
            status["errors"].append("non-empty pcapng/pcap missing")
        if status["errors"]:
            statuses.append(status)
            continue

        stats = read_stats(stats_path)
        metadata = read_json(meta_path)
        connection_count = integer(stats.get("connection_count"))
        if stats.get("profile_family") != family:
            status["errors"].append(f"profile_family mismatch: {stats.get('profile_family')!r}")
        if connection_count < min_connections:
            status["errors"].append(f"connection_count {connection_count} < {min_connections}")
        if metadata.get("native_os") is not True:
            status["errors"].append("metadata native_os is not true")
        if status["errors"]:
            status["status"] = "degraded"
            status["connection_count"] = connection_count
            statuses.append(status)
            continue

        evidence = {
            "family": family,
            "stats": stats,
            "distribution": read_json(dist_path),
            "metadata": metadata,
        }
        complete.append(evidence)
        status.update({
            "status": "complete",
            "connection_count": connection_count,
            "packet_count": integer(stats.get("packet_count")),
            "pcap_count": len(nonempty_pcaps),
        })
        statuses.append(status)
    return complete, statuses


def sample_family(evidences: list[dict[str, Any]], rng: random.Random) -> dict[str, Any]:
    return rng.choice(evidences)


def sample_length(distribution: dict[str, Any], rng: random.Random, fallback_mean: float) -> int:
    bins = [item for item in distribution.get("bins", []) if numeric(item.get("probability")) > 0]
    if not bins:
        return int(clamp(rng.gauss(fallback_mean or 512.0, 96.0), 40.0, 1500.0))
    roll = rng.random()
    selected = bins[-1]
    cumulative = 0.0
    for item in bins:
        cumulative += numeric(item.get("probability"))
        if roll <= cumulative:
            selected = item
            break
    low = integer(selected.get("low"), 40)
    high = integer(selected.get("high"), low)
    return int(clamp(rng.randint(min(low, high), max(low, high)), 40.0, 1500.0))


def entropy(values: list[int]) -> float:
    if not values:
        return 0.0
    counts: dict[int, int] = {}
    for value in values:
        counts[value] = counts.get(value, 0) + 1
    total = len(values)
    return -sum((count / total) * math.log2(count / total) for count in counts.values())


def jitter_int(base: int, rng: random.Random, pct: float, lower: int = 0) -> int:
    if base <= 0:
        return lower
    return max(lower, int(round(rng.gauss(base, max(1.0, abs(base) * pct)))))


def jitter_float(base: float, rng: random.Random, pct: float, lower: float = 0.0) -> float:
    if base <= 0:
        return lower
    return max(lower, rng.gauss(base, max(1.0, abs(base) * pct)))


def calibrated_row(evidences: list[dict[str, Any]], rng: random.Random) -> dict[str, Any]:
    evidence = sample_family(evidences, rng)
    family = evidence["family"]
    stats = evidence["stats"]
    lengths = [
        sample_length(evidence["distribution"], rng, numeric(stats.get("packet_len_mean"), 512.0))
        for _ in range(10)
    ]
    dirs = [1 if idx % 2 == 0 else -1 for idx in range(10)]
    if rng.random() < 0.2:
        dirs = [-value for value in dirs]
    up_bytes = sum(length for length, direction in zip(lengths, dirs) if direction > 0)
    down_bytes = sum(length for length, direction in zip(lengths, dirs) if direction < 0)

    row: dict[str, Any] = {
        "label": 1,
        "tcp_window": jitter_int(integer(stats.get("tcp_window")), rng, 0.03),
        "tcp_mss": jitter_int(integer(stats.get("tcp_mss")), rng, 0.01),
        "tcp_wscale": integer(stats.get("tcp_wscale")),
        "tcp_sack": integer(stats.get("tcp_sack_ok")),
        "tcp_timestamps": integer(stats.get("tcp_timestamps")),
        "tls_ext_count": TLS_EXT_COUNT_HEURISTIC.get(family, 0),
        "up_down_ratio": round(up_bytes / max(1, down_bytes), 6),
        "pkt_len_entropy": round(entropy(lengths), 6),
        "pkt_len_mean": round(statistics.fmean(lengths), 6),
        "pkt_len_std": round(statistics.pstdev(lengths), 6) if len(lengths) > 1 else 0.0,
        "iat_mean": round(jitter_float(numeric(stats.get("iat_mean_us")), rng, 0.20), 6),
        "iat_std": round(jitter_float(numeric(stats.get("iat_std_us")), rng, 0.25), 6),
        "iat_p50": round(jitter_float(numeric(stats.get("iat_p50_us")), rng, 0.20), 6),
        "iat_p95": round(jitter_float(numeric(stats.get("iat_p95_us")), rng, 0.20), 6),
        "iat_p99": round(jitter_float(numeric(stats.get("iat_p99_us")), rng, 0.20), 6),
        "burst_count": 0,
        "burst_mean_size": 0.0,
        "burst_mean_interval": 0.0,
    }
    for idx, length in enumerate(lengths, start=1):
        row[f"pkt_len_{idx}"] = length
    for idx, direction in enumerate(dirs, start=1):
        row[f"pkt_dir_{idx}"] = direction
    return {column: row.get(column, 0) for column in FEATURE_COLUMNS}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Calibrate ReMirage reference samples from M13 baseline")
    parser.add_argument(
        "--baseline-root",
        default=str(Path(__file__).resolve().parent.parent / "baseline"),
        help="Root directory containing baseline/<family>/ evidence",
    )
    parser.add_argument(
        "--input-metadata",
        default=str(Path(__file__).resolve().parent.parent / "simulation-metadata.json"),
        help="Input simulation metadata",
    )
    parser.add_argument(
        "--output-metadata",
        default=str(Path(__file__).resolve().parent.parent / "simulation-metadata-calibrated.json"),
        help="Output calibrated metadata",
    )
    parser.add_argument("--min-connections", type=int, default=100)
    parser.add_argument("--min-control-families", type=int, default=2)
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    rng = random.Random(args.seed)
    baseline_root = Path(args.baseline_root)
    input_path = Path(args.input_metadata)
    output_path = Path(args.output_metadata)

    evidences, statuses = discover_families(baseline_root, args.min_connections)
    if len(evidences) < args.min_control_families:
        raise SystemExit(
            "calibration requires at least "
            f"{args.min_control_families} complete baseline families; got {[e['family'] for e in evidences]}"
        )

    metadata = read_json(input_path)
    source_rows = metadata.get("classifier_samples", [])
    remirage_count = sum(1 for row in source_rows if integer(row.get("label"), -1) == 1)
    if remirage_count == 0:
        raise SystemExit(f"no label=1 rows found in {input_path}")

    calibrated_rows = [calibrated_row(evidences, rng) for _ in range(remirage_count)]
    calibrated_iter = iter(calibrated_rows)
    output_rows = []
    for row in source_rows:
        if integer(row.get("label"), -1) == 1:
            output_rows.append(next(calibrated_iter))
        else:
            output_rows.append(row)

    metadata["mode"] = "calibrated_simulation"
    metadata["evidence_strength"] = "校准后模拟参考"
    metadata["calibration"] = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "baseline_root": str(baseline_root),
        "source_metadata": str(input_path),
        "control_families": [item["family"] for item in evidences],
        "family_status": statuses,
        "remirage_rows_calibrated": remirage_count,
        "upgrade_eligible": False,
        "limitations": [
            "This is a calibrated simulation/reference dataset, not real ReMirage traffic evidence.",
            "chrome-macos is not fabricated when missing.",
            "The output is suitable for remediation experiments only and cannot satisfy the Capability-Upgrade Gate.",
        ],
    }
    metadata["classifier_samples"] = output_rows

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote calibrated metadata to {output_path}")
    print(f"calibrated remirage rows: {remirage_count}")
    print(f"control families: {', '.join(item['family'] for item in evidences)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
