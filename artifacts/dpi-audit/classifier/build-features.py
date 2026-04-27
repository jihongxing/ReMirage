#!/usr/bin/env python3
"""Build classifier features.csv from Phase 2 sample metadata."""

from __future__ import annotations

import argparse
import csv
import json
from pathlib import Path


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


def main() -> None:
    parser = argparse.ArgumentParser(description="Build features.csv from simulation metadata")
    parser.add_argument(
        "--metadata",
        default=str(Path(__file__).resolve().parent.parent / "simulation-metadata.json"),
        help="Input metadata JSON path",
    )
    parser.add_argument(
        "--output",
        default=str(Path(__file__).resolve().parent / "features.csv"),
        help="Output CSV path",
    )
    args = parser.parse_args()

    metadata_path = Path(args.metadata)
    output_path = Path(args.output)
    if not metadata_path.exists():
        raise FileNotFoundError(f"metadata not found: {metadata_path}")

    metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
    rows = metadata.get("classifier_samples", [])

    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=FEATURE_COLUMNS)
        writer.writeheader()
        for row in rows:
            writer.writerow({column: row.get(column, "") for column in FEATURE_COLUMNS})

    print(f"wrote {len(rows)} rows to {output_path}")


if __name__ == "__main__":
    main()
