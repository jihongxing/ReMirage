#!/usr/bin/env python3
"""Generate simulation-mode Phase 2 sample artifacts.

This script is used when real Linux loopback capture is unavailable or unsafe.
It produces:
  - synthetic pcapng samples for handshake / packet length / timing dimensions
  - simulation metadata used to build classifier features

Evidence strength must be labeled as "模拟环境参考".
"""

from __future__ import annotations

import json
import math
import random
import statistics
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

from scapy.layers.inet import IP, TCP, Ether
from scapy.packet import Raw
from scapy.utils import PcapNgWriter


ROOT = Path(__file__).resolve().parent
HANDSHAKE_DIR = ROOT / "handshake"
PACKET_DIR = ROOT / "packet-length"
TIMING_DIR = ROOT / "timing"
CLASSIFIER_DIR = ROOT / "classifier"
METADATA_PATH = ROOT / "simulation-metadata.json"

RNG = random.Random(42)


@dataclass(frozen=True)
class HandshakeProfile:
    tcp_window: int
    tcp_mss: int
    tcp_wscale: int
    tcp_sack: bool
    tcp_timestamps: bool
    tls_extensions: list[int]


HANDSHAKE_PROFILES = {
    "remirage": HandshakeProfile(64240, 1380, 8, True, True, [0, 10, 11, 13, 16, 18, 23, 43, 45, 51]),
    "chrome": HandshakeProfile(65535, 1460, 8, True, True, [0, 5, 10, 11, 13, 16, 18, 21, 23, 43, 45, 51]),
    "utls": HandshakeProfile(64240, 1460, 7, True, True, [0, 10, 11, 13, 16, 18, 23, 35, 43, 45, 51]),
}


PACKET_MODES = ("baseline-no-padding", "mode-fixed-mtu", "mode-random-range", "mode-gaussian")
TIMING_CONFIGS = ("none", "jitter-only", "vpc-only", "jitter-vpc")
MAC_A = "02:00:00:00:00:01"
MAC_B = "02:00:00:00:00:02"


def ensure_dirs() -> None:
    for path in (HANDSHAKE_DIR, PACKET_DIR, TIMING_DIR, CLASSIFIER_DIR):
        path.mkdir(parents=True, exist_ok=True)


def make_tls_client_hello(extension_ids: list[int]) -> bytes:
    client_random = bytes((i % 256 for i in range(32)))
    session_id = b""
    cipher_suites = b"\x13\x01"
    compression_methods = b"\x00"

    ext_parts = []
    for ext_id in extension_ids:
        ext_parts.append(ext_id.to_bytes(2, "big") + (0).to_bytes(2, "big"))
    extensions = b"".join(ext_parts)

    body = (
        b"\x03\x03"
        + client_random
        + len(session_id).to_bytes(1, "big")
        + session_id
        + len(cipher_suites).to_bytes(2, "big")
        + cipher_suites
        + len(compression_methods).to_bytes(1, "big")
        + compression_methods
        + len(extensions).to_bytes(2, "big")
        + extensions
    )
    handshake = b"\x01" + len(body).to_bytes(3, "big") + body
    record = b"\x16\x03\x01" + len(handshake).to_bytes(2, "big") + handshake
    return record


def write_pcapng(path: Path, packets: Iterable) -> None:
    writer = PcapNgWriter(str(path))
    try:
        for pkt in packets:
            writer.write(pkt)
    finally:
        writer.close()


def build_ether(src_mac: str, dst_mac: str):
    """Use explicit MAC addresses to avoid Scapy route resolution warnings."""
    return Ether(src=src_mac, dst=dst_mac)


def build_syn_packet(profile: HandshakeProfile, src: str, dst: str, sport: int, dport: int, ts: float):
    options = [("MSS", profile.tcp_mss), ("WScale", profile.tcp_wscale)]
    if profile.tcp_sack:
        options.append(("SAckOK", b""))
    if profile.tcp_timestamps:
        options.append(("Timestamp", (123456, 0)))

    pkt = build_ether(MAC_A, MAC_B) / IP(src=src, dst=dst, id=1) / TCP(
        sport=sport,
        dport=dport,
        flags="S",
        seq=1000,
        window=profile.tcp_window,
        options=options,
    )
    pkt.time = ts
    return pkt


def build_tls_packet(profile: HandshakeProfile, src: str, dst: str, sport: int, dport: int, ts: float):
    pkt = build_ether(MAC_A, MAC_B) / IP(src=src, dst=dst, id=2) / TCP(
        sport=sport,
        dport=dport,
        flags="PA",
        seq=1001,
        ack=1,
    ) / Raw(load=make_tls_client_hello(profile.tls_extensions))
    pkt.time = ts
    return pkt


def generate_handshake_pcaps() -> None:
    base_ts = 1_700_000_000.0
    for name, profile in HANDSHAKE_PROFILES.items():
        src, dst = "10.0.0.2", "10.0.0.1"
        sport = 40000 + len(name)
        dport = 443
        packets = [
            build_syn_packet(profile, src, dst, sport, dport, base_ts),
            build_tls_packet(profile, src, dst, sport, dport, base_ts + 0.010),
        ]
        write_pcapng(HANDSHAKE_DIR / f"{name}-syn.pcapng", packets)


def payload_for_ip_length(target_ip_len: int) -> bytes:
    payload_len = max(0, target_ip_len - 40)
    return b"x" * payload_len


def packet_lengths_for_mode(mode: str, count: int) -> list[int]:
    lengths = []
    for _ in range(count):
        if mode == "baseline-no-padding":
            lengths.append(RNG.choice([90, 128, 192, 320, 512, 768, 960]))
        elif mode == "mode-fixed-mtu":
            lengths.append(1400 if RNG.random() > 0.15 else RNG.choice([300, 512]))
        elif mode == "mode-random-range":
            lengths.append(RNG.randint(780, 1450))
        else:
            val = int(RNG.gauss(1180, 140))
            lengths.append(max(420, min(1460, val)))
    return lengths


def generate_packet_pcaps() -> None:
    base_ts = 1_700_000_100.0
    for mode in PACKET_MODES:
        packets = []
        src_a, src_b = "10.0.1.2", "10.0.1.1"
        for idx, ip_len in enumerate(packet_lengths_for_mode(mode, 220)):
            src = src_a if idx % 2 == 0 else src_b
            dst = src_b if idx % 2 == 0 else src_a
            src_mac = MAC_A if idx % 2 == 0 else MAC_B
            dst_mac = MAC_B if idx % 2 == 0 else MAC_A
            pkt = build_ether(src_mac, dst_mac) / IP(src=src, dst=dst, id=idx + 1) / TCP(
                sport=44300 if idx % 2 == 0 else 8443,
                dport=8443 if idx % 2 == 0 else 44300,
                flags="A",
                seq=idx + 1000,
                ack=1,
            ) / Raw(load=payload_for_ip_length(ip_len))
            pkt.time = base_ts + idx * 0.001
            packets.append(pkt)
        write_pcapng(PACKET_DIR / f"{mode}.pcapng", packets)


def timing_iats(config: str, count: int) -> list[float]:
    values: list[float] = []
    for _ in range(count):
        if config == "none":
            values.append(1000.0)
        elif config == "jitter-only":
            values.append(max(150.0, RNG.gauss(1050.0, 180.0)))
        elif config == "vpc-only":
            spike = 450.0 if RNG.random() < 0.1 else 0.0
            values.append(max(200.0, 980.0 + spike + RNG.uniform(-80.0, 80.0)))
        else:
            spike = 600.0 if RNG.random() < 0.14 else 0.0
            values.append(max(180.0, RNG.gauss(1100.0, 240.0) + spike))
    return values


def generate_timing_pcaps() -> None:
    base_ts = 1_700_000_200.0
    for config in TIMING_CONFIGS:
        packets = []
        current_ts = base_ts
        next_ip_id = 1
        for idx, iat_us in enumerate(timing_iats(config, 240)):
            # Simulate sparse reorder/loss for VPC variants via IP ID irregularities.
            if config in {"vpc-only", "jitter-vpc"} and idx % 37 == 0:
                ip_id = next_ip_id + 2
            elif config == "jitter-vpc" and idx % 53 == 0:
                ip_id = max(1, next_ip_id - 1)
            else:
                ip_id = next_ip_id
            next_ip_id += 1

            current_ts += iat_us / 1_000_000.0
            pkt = build_ether(MAC_A, MAC_B) / IP(src="10.0.2.2", dst="10.0.2.1", id=ip_id) / TCP(
                sport=44300,
                dport=8443,
                flags="A",
                seq=idx + 2000,
                ack=1,
            ) / Raw(load=b"timing")
            pkt.time = current_ts
            packets.append(pkt)
        write_pcapng(TIMING_DIR / f"config-{config}.pcapng", packets)


def shannon_entropy(values: list[int]) -> float:
    if not values:
        return 0.0
    counts: dict[int, int] = {}
    for value in values:
        counts[value] = counts.get(value, 0) + 1
    total = len(values)
    entropy = 0.0
    for count in counts.values():
        p = count / total
        entropy -= p * math.log2(p)
    return entropy


def sample_packet_feature_row(mode: str) -> dict[str, float | int]:
    lengths = packet_lengths_for_mode(mode, 14)
    dirs = [1 if idx % 2 == 0 else -1 for idx in range(10)]
    uplink = sum(1 for value in dirs if value > 0)
    downlink = sum(1 for value in dirs if value < 0) or 1
    row: dict[str, float | int] = {}
    for idx in range(10):
        row[f"pkt_len_{idx + 1}"] = lengths[idx]
        row[f"pkt_dir_{idx + 1}"] = dirs[idx]
    row["up_down_ratio"] = round(uplink / downlink, 4)
    row["pkt_len_entropy"] = round(shannon_entropy(lengths), 4)
    row["pkt_len_mean"] = round(statistics.mean(lengths), 2)
    row["pkt_len_std"] = round(statistics.pstdev(lengths), 2)
    return row


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

    count = len(burst_sizes)
    mean_size = round(statistics.mean(burst_sizes), 2) if burst_sizes else 0.0
    mean_gap = round(statistics.mean(burst_gaps), 2) if burst_gaps else 0.0
    return count, mean_size, mean_gap


def sample_timing_feature_row(config: str) -> dict[str, float | int]:
    iats = timing_iats(config, 48)
    p50 = statistics.quantiles(iats, n=100)[49]
    p95 = statistics.quantiles(iats, n=100)[94]
    p99 = statistics.quantiles(iats, n=100)[98]
    burst_count, burst_mean_size, burst_mean_interval = burst_stats(iats)
    return {
        "iat_mean": round(statistics.mean(iats), 2),
        "iat_std": round(statistics.pstdev(iats), 2),
        "iat_p50": round(p50, 2),
        "iat_p95": round(p95, 2),
        "iat_p99": round(p99, 2),
        "burst_count": burst_count,
        "burst_mean_size": burst_mean_size,
        "burst_mean_interval": burst_mean_interval,
    }


def build_classifier_samples(sample_count_per_class: int = 120) -> list[dict[str, float | int]]:
    rows: list[dict[str, float | int]] = []

    control_profiles = ("chrome", "utls")
    remirage_packet_modes = ("mode-fixed-mtu", "mode-random-range", "mode-gaussian")
    remirage_timing = ("jitter-only", "vpc-only", "jitter-vpc")

    for _ in range(sample_count_per_class):
        profile = HANDSHAKE_PROFILES[RNG.choice(control_profiles)]
        row = {
            "label": 0,
            "tcp_window": profile.tcp_window,
            "tcp_mss": profile.tcp_mss,
            "tcp_wscale": profile.tcp_wscale,
            "tcp_sack": int(profile.tcp_sack),
            "tcp_timestamps": int(profile.tcp_timestamps),
            "tls_ext_count": len(profile.tls_extensions),
        }
        row.update(sample_packet_feature_row("baseline-no-padding"))
        row.update(sample_timing_feature_row("none"))
        rows.append(row)

    for _ in range(sample_count_per_class):
        profile = HANDSHAKE_PROFILES["remirage"]
        row = {
            "label": 1,
            "tcp_window": profile.tcp_window,
            "tcp_mss": profile.tcp_mss,
            "tcp_wscale": profile.tcp_wscale,
            "tcp_sack": int(profile.tcp_sack),
            "tcp_timestamps": int(profile.tcp_timestamps),
            "tls_ext_count": len(profile.tls_extensions),
        }
        row.update(sample_packet_feature_row(RNG.choice(remirage_packet_modes)))
        row.update(sample_timing_feature_row(RNG.choice(remirage_timing)))
        rows.append(row)

    RNG.shuffle(rows)
    return rows


def write_notice() -> None:
    notice = (
        "# Phase 2 模拟样本说明\n\n"
        "本目录由 `generate-simulated-samples.py` 生成，证据强度为“模拟环境参考”。\n"
        "这些样本用于在缺少 root/CAP_NET_RAW、Linux Go toolchain 或抓包依赖时，"
        "验证 M6 数据链路、分析脚本与分类器训练是否能端到端运行。\n"
    )
    (ROOT / "SIMULATION_NOTICE.md").write_text(notice, encoding="utf-8")


def main() -> None:
    ensure_dirs()
    generate_handshake_pcaps()
    generate_packet_pcaps()
    generate_timing_pcaps()
    write_notice()

    metadata = {
        "mode": "simulation",
        "evidence_strength": "模拟环境参考",
        "handshake_profiles": {
            name: {
                "tcp_window": profile.tcp_window,
                "tcp_mss": profile.tcp_mss,
                "tcp_wscale": profile.tcp_wscale,
                "tcp_sack": profile.tcp_sack,
                "tcp_timestamps": profile.tcp_timestamps,
                "tls_extensions": profile.tls_extensions,
            }
            for name, profile in HANDSHAKE_PROFILES.items()
        },
        "classifier_samples": build_classifier_samples(),
    }
    METADATA_PATH.write_text(json.dumps(metadata, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"simulation metadata written to {METADATA_PATH}")


if __name__ == "__main__":
    main()
