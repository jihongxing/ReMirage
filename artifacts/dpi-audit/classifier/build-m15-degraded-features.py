#!/usr/bin/env python3
"""Build M15-degraded classifier features from real M13 baseline evidence.

This script intentionally produces degraded evidence:
  - control rows are bootstrapped from completed real M13 baseline families
  - ReMirage rows come from the current simulation/reference metadata
  - missing native families are recorded and never fabricated

The output is compatible with train-classifier.py.
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


def discover_family(root: Path, family: str, min_connections: int) -> tuple[dict[str, Any] | None, dict[str, Any]]:
    family_dir = root / family
    status: dict[str, Any] = {
        "family": family,
        "status": "missing",
        "errors": [],
    }
    stats_path = family_dir / "baseline-stats.csv"
    dist_path = family_dir / "baseline-distribution.json"
    meta_path = family_dir / "capture-metadata.json"
    pcaps = sorted(family_dir.glob("*.pcapng")) + sorted(family_dir.glob("*.pcap"))
    nonempty_pcaps = [path for path in pcaps if path.stat().st_size > 0]

    if not family_dir.exists():
        status["errors"].append("family directory missing")
        return None, status
    if not stats_path.exists():
        status["errors"].append("baseline-stats.csv missing")
    if not dist_path.exists():
        status["errors"].append("baseline-distribution.json missing")
    if not meta_path.exists():
        status["errors"].append("capture-metadata.json missing")
    if not nonempty_pcaps:
        status["errors"].append("non-empty pcapng/pcap missing")
    if status["errors"]:
        return None, status

    stats = read_stats(stats_path)
    distribution = read_json(dist_path)
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
        status["packet_count"] = integer(stats.get("packet_count"))
        return None, status

    evidence = {
        "family": family,
        "dir": str(family_dir),
        "stats": stats,
        "distribution": distribution,
        "metadata": metadata,
        "pcaps": [p.name for p in nonempty_pcaps],
    }
    status.update({
        "status": "complete",
        "connection_count": connection_count,
        "packet_count": integer(stats.get("packet_count")),
        "pcap_count": len(nonempty_pcaps),
        "captured_at": metadata.get("captured_at", ""),
    })
    return evidence, status


def weighted_length_sample(distribution: dict[str, Any], rng: random.Random, fallback_mean: float) -> int:
    bins = distribution.get("bins", [])
    positive_bins = [item for item in bins if numeric(item.get("probability")) > 0]
    if not positive_bins:
        return int(clamp(rng.gauss(fallback_mean or 512.0, 96.0), 40.0, 1500.0))

    roll = rng.random()
    selected = positive_bins[-1]
    cumulative = 0.0
    for item in positive_bins:
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
    spread = max(1.0, abs(base) * pct)
    return max(lower, int(round(rng.gauss(base, spread))))


def jitter_float(base: float, rng: random.Random, pct: float, lower: float = 0.0) -> float:
    if base <= 0:
        return lower
    spread = max(1.0, abs(base) * pct)
    return max(lower, rng.gauss(base, spread))


def build_control_row(evidence: dict[str, Any], rng: random.Random) -> dict[str, Any]:
    family = evidence["family"]
    stats = evidence["stats"]
    distribution = evidence["distribution"]
    mean_len = numeric(stats.get("packet_len_mean"), 512.0)
    packet_lengths = [
        weighted_length_sample(distribution, rng, mean_len)
        for _ in range(10)
    ]
    packet_dirs = [1 if idx % 2 == 0 else -1 for idx in range(10)]
    if rng.random() < 0.2:
        packet_dirs = [-value for value in packet_dirs]

    up_bytes = sum(length for length, direction in zip(packet_lengths, packet_dirs) if direction > 0)
    down_bytes = sum(length for length, direction in zip(packet_lengths, packet_dirs) if direction < 0)
    row: dict[str, Any] = {
        "label": 0,
        "tcp_window": jitter_int(integer(stats.get("tcp_window")), rng, 0.03),
        "tcp_mss": jitter_int(integer(stats.get("tcp_mss")), rng, 0.01),
        "tcp_wscale": integer(stats.get("tcp_wscale")),
        "tcp_sack": integer(stats.get("tcp_sack_ok")),
        "tcp_timestamps": integer(stats.get("tcp_timestamps")),
        "tls_ext_count": TLS_EXT_COUNT_HEURISTIC.get(family, 0),
        "up_down_ratio": round(up_bytes / max(1, down_bytes), 6),
        "pkt_len_entropy": round(entropy(packet_lengths), 6),
        "pkt_len_mean": round(statistics.fmean(packet_lengths), 6),
        "pkt_len_std": round(statistics.pstdev(packet_lengths), 6) if len(packet_lengths) > 1 else 0.0,
        "iat_mean": round(jitter_float(numeric(stats.get("iat_mean_us")), rng, 0.20), 6),
        "iat_std": round(jitter_float(numeric(stats.get("iat_std_us")), rng, 0.25), 6),
        "iat_p50": round(jitter_float(numeric(stats.get("iat_p50_us")), rng, 0.20), 6),
        "iat_p95": round(jitter_float(numeric(stats.get("iat_p95_us")), rng, 0.20), 6),
        "iat_p99": round(jitter_float(numeric(stats.get("iat_p99_us")), rng, 0.20), 6),
        "burst_count": 0,
        "burst_mean_size": 0.0,
        "burst_mean_interval": 0.0,
    }
    for idx, length in enumerate(packet_lengths, start=1):
        row[f"pkt_len_{idx}"] = length
    for idx, direction in enumerate(packet_dirs, start=1):
        row[f"pkt_dir_{idx}"] = direction
    return {column: row.get(column, 0) for column in FEATURE_COLUMNS}


def build_control_rows(
    evidences: list[dict[str, Any]],
    total_rows: int,
    rng: random.Random,
) -> tuple[list[dict[str, Any]], dict[str, int]]:
    if not evidences:
        raise ValueError("no complete control families available")
    per_family = total_rows // len(evidences)
    remainder = total_rows % len(evidences)
    rows: list[dict[str, Any]] = []
    counts: dict[str, int] = {}
    for idx, evidence in enumerate(evidences):
        count = per_family + (1 if idx < remainder else 0)
        counts[evidence["family"]] = count
        rows.extend(build_control_row(evidence, rng) for _ in range(count))
    rng.shuffle(rows)
    return rows, counts


def load_remirage_rows(path: Path) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    metadata = read_json(path)
    rows = []
    for row in metadata.get("classifier_samples", []):
        if integer(row.get("label"), -1) == 1:
            rows.append({column: row.get(column, 0) for column in FEATURE_COLUMNS})
    if not rows:
        raise ValueError(f"no label=1 ReMirage samples found in {path}")
    return rows, {
        "path": str(path),
        "mode": metadata.get("mode", "unknown"),
        "evidence_strength": metadata.get("evidence_strength", "unknown"),
        "source_rows": len(rows),
    }


def cycle_rows(rows: list[dict[str, Any]], target_count: int, rng: random.Random) -> list[dict[str, Any]]:
    if target_count <= len(rows):
        selected = list(rows)
        rng.shuffle(selected)
        return selected[:target_count]
    selected = []
    while len(selected) < target_count:
        batch = list(rows)
        rng.shuffle(batch)
        selected.extend(batch)
    return selected[:target_count]


def write_features(path: Path, rows: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=FEATURE_COLUMNS)
        writer.writeheader()
        for row in rows:
            writer.writerow({column: row.get(column, 0) for column in FEATURE_COLUMNS})


def write_metadata(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Build M15-degraded features from real M13 baseline and current ReMirage samples",
    )
    parser.add_argument(
        "--baseline-root",
        default=str(Path(__file__).resolve().parent.parent / "baseline"),
        help="Root directory containing baseline/<family>/ evidence",
    )
    parser.add_argument(
        "--simulation-metadata",
        default=str(Path(__file__).resolve().parent.parent / "simulation-metadata.json"),
        help="Current simulation/reference metadata containing label=1 ReMirage samples",
    )
    parser.add_argument(
        "--output",
        default=str(Path(__file__).resolve().parent / "features-m15-degraded.csv"),
        help="Output feature CSV path",
    )
    parser.add_argument(
        "--metadata-output",
        default=str(Path(__file__).resolve().parent / "m15-degraded-metadata.json"),
        help="Output metadata JSON path",
    )
    parser.add_argument("--min-connections", type=int, default=100)
    parser.add_argument("--min-control-families", type=int, default=2)
    parser.add_argument("--control-count", type=int, default=0, help="Default: match ReMirage row count")
    parser.add_argument("--remirage-count", type=int, default=0, help="Default: use all current ReMirage rows")
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    rng = random.Random(args.seed)
    baseline_root = Path(args.baseline_root)
    simulation_metadata = Path(args.simulation_metadata)
    output_path = Path(args.output)
    metadata_path = Path(args.metadata_output)

    statuses = []
    complete_evidences = []
    for family in FAMILIES:
        evidence, status = discover_family(baseline_root, family, args.min_connections)
        statuses.append(status)
        if evidence is not None:
            complete_evidences.append(evidence)

    control_families = [item["family"] for item in complete_evidences]
    if len(complete_evidences) < args.min_control_families:
        raise SystemExit(
            "M15-degraded requires at least "
            f"{args.min_control_families} complete real control families; got {control_families}"
        )

    remirage_source_rows, remirage_source = load_remirage_rows(simulation_metadata)
    remirage_count = args.remirage_count or len(remirage_source_rows)
    remirage_rows = cycle_rows(remirage_source_rows, remirage_count, rng)
    control_count = args.control_count or len(remirage_rows)
    control_rows, generated_control_counts = build_control_rows(complete_evidences, control_count, rng)

    rows = control_rows + remirage_rows
    rng.shuffle(rows)
    write_features(output_path, rows)

    missing_families = [
        item["family"]
        for item in statuses
        if item.get("status") != "complete"
    ]
    metadata = {
        "mode": "M15-degraded",
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "random_seed": args.seed,
        "upgrade_eligible": False,
        "feature_file": str(output_path),
        "control_evidence": "real_m13_degraded_bootstrap",
        "control_families": control_families,
        "required_full_families": list(FAMILIES),
        "missing_or_incomplete_families": missing_families,
        "family_status": statuses,
        "generated_control_rows": generated_control_counts,
        "remirage_evidence": "current_simulation_metadata_label_1",
        "remirage_source": remirage_source,
        "label_distribution": {
            "control_0": len(control_rows),
            "remirage_1": len(remirage_rows),
        },
        "limitations": [
            "Control rows are bootstrapped from family-level M13 stats and packet-length CDFs, not per-connection raw feature rows.",
            "TLS extension count is a family heuristic because M13 extraction does not yet emit per-flow TLS extension counts.",
            "ReMirage rows use the current simulation/reference metadata until real ReMirage pcap-derived rows exist.",
            "chrome-macos is not fabricated; missing native macOS evidence keeps this result degraded.",
            "This output can show risk trend evidence only and cannot satisfy the Capability-Upgrade Gate.",
        ],
    }
    write_metadata(metadata_path, metadata)

    print(f"wrote {len(rows)} rows to {output_path}")
    print(f"wrote metadata to {metadata_path}")
    print(f"control families: {', '.join(control_families)}")
    if missing_families:
        print(f"missing/incomplete families: {', '.join(missing_families)}")
    print("status: M15-degraded (upgrade_eligible=false)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
