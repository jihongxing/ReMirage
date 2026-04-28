#!/usr/bin/env python3
"""Rank classifier feature gaps between control and ReMirage rows."""

from __future__ import annotations

import argparse
import csv
import json
import math
from pathlib import Path
from typing import Any

import pandas as pd


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Analyze M15 classifier feature gaps")
    parser.add_argument(
        "--input",
        "-i",
        default=str(Path(__file__).resolve().parent / "features-m15-degraded.csv"),
        help="Input feature CSV",
    )
    parser.add_argument(
        "--output",
        "-o",
        default=str(Path(__file__).resolve().parent / "feature-gap-m15-degraded.csv"),
        help="Output ranked CSV",
    )
    parser.add_argument(
        "--json-output",
        default=str(Path(__file__).resolve().parent / "feature-gap-m15-degraded.json"),
        help="Output JSON summary",
    )
    parser.add_argument("--top", type=int, default=25)
    return parser.parse_args()


def effect_size(control: pd.Series, remirage: pd.Series) -> float:
    pooled = math.sqrt((float(control.var()) + float(remirage.var())) / 2.0)
    if pooled == 0:
        return 0.0
    return abs(float(control.mean()) - float(remirage.mean())) / pooled


def main() -> int:
    args = parse_args()
    input_path = Path(args.input)
    output_path = Path(args.output)
    json_path = Path(args.json_output)
    if not input_path.exists():
        raise FileNotFoundError(f"feature CSV not found: {input_path}")

    df = pd.read_csv(input_path)
    if "label" not in df.columns:
        raise ValueError("feature CSV must contain label column")

    numeric = df.drop(columns=["label"]).apply(pd.to_numeric, errors="coerce").fillna(0)
    labels = pd.to_numeric(df["label"], errors="coerce").fillna(-1).astype(int)
    rows: list[dict[str, Any]] = []
    for column in numeric.columns:
        control = numeric[labels == 0][column]
        remirage = numeric[labels == 1][column]
        rows.append({
            "feature": column,
            "effect_size": round(effect_size(control, remirage), 6),
            "control_mean": round(float(control.mean()), 6),
            "remirage_mean": round(float(remirage.mean()), 6),
            "control_std": round(float(control.std()), 6),
            "remirage_std": round(float(remirage.std()), 6),
        })

    rows.sort(key=lambda item: item["effect_size"], reverse=True)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=list(rows[0].keys()) if rows else [])
        writer.writeheader()
        writer.writerows(rows)

    summary = {
        "input": str(input_path),
        "sample_count": int(len(df)),
        "label_distribution": {
            "control_0": int((labels == 0).sum()),
            "remirage_1": int((labels == 1).sum()),
        },
        "top_features": rows[:args.top],
        "interpretation": [
            "Large effect_size values identify features the classifier can separate easily.",
            "This diagnostic does not prove causality, but it is the first remediation priority list.",
        ],
    }
    json_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    for row in rows[:args.top]:
        print(
            f"{row['effect_size']:9.3f}  {row['feature']:24s} "
            f"control={row['control_mean']:.3f} remirage={row['remirage_mean']:.3f}"
        )
    print(f"wrote {output_path}")
    print(f"wrote {json_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
