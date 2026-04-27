#!/usr/bin/env python3
from __future__ import annotations

import csv
import json
import sys
from pathlib import Path

FAMILIES = ("chrome-win", "chrome-macos", "firefox-linux")


def check_family(root: Path, family: str) -> list[str]:
    errors: list[str] = []
    family_dir = root / family
    metadata = family_dir / "capture-metadata.json"
    stats = family_dir / "baseline-stats.csv"
    dist = family_dir / "baseline-distribution.json"
    pcaps = list(family_dir.glob("*.pcapng")) + list(family_dir.glob("*.pcap"))

    if not pcaps:
        errors.append(f"{family}: no pcapng/pcap files")
    if not metadata.exists():
        errors.append(f"{family}: missing capture-metadata.json")
    else:
        data = json.loads(metadata.read_text(encoding="utf-8"))
        if data.get("profile_family") != family:
            errors.append(f"{family}: metadata profile_family mismatch")
        if data.get("native_os") is not True:
            errors.append(f"{family}: metadata native_os is not true")
    if not stats.exists():
        errors.append(f"{family}: missing baseline-stats.csv")
    else:
        with stats.open(newline="", encoding="utf-8") as f:
            rows = list(csv.DictReader(f))
        if not rows:
            errors.append(f"{family}: empty baseline-stats.csv")
        else:
            row = rows[0]
            if row.get("profile_family") != family:
                errors.append(f"{family}: stats profile_family mismatch")
            try:
                count = int(float(row.get("connection_count", "0")))
            except ValueError:
                count = 0
            if count < 100:
                errors.append(f"{family}: connection_count {count} < 100")
    if not dist.exists():
        errors.append(f"{family}: missing baseline-distribution.json")

    return errors


def main() -> int:
    root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("artifacts/dpi-audit/baseline")
    errors = []
    for family in FAMILIES:
        errors.extend(check_family(root, family))

    if errors:
        print("M13-degraded")
        for error in errors:
            print(f"- {error}")
        return 1

    print("M13-full")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
