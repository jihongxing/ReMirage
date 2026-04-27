#!/usr/bin/env python3
"""从 pcapng 文件分析 IAT（包间到达时间）分布，输出 iat-stats.csv。

统计维度：
  - IAT 均值、标准差
  - IAT P50 / P95 / P99
  - Burst 结构：burst 数量、平均 burst 大小、平均 burst 间隔
  - 乱序率、丢包率（基于 IP ID 的受控环境粗估）

对照条件：Jitter ± VPC 的 2×2 矩阵
  (a) config-none          — 基线（无扰动）
  (b) config-jitter-only   — 仅 Jitter-Lite
  (c) config-vpc-only      — 仅 VPC 噪声
  (d) config-jitter-vpc    — Jitter + VPC 联合

依赖: scapy, numpy (pip install scapy numpy)
需求: 4.1, 4.2, 4.3, 4.6
"""

import argparse
import csv
import logging
import sys
from pathlib import Path

logging.basicConfig(
    level=logging.INFO,
    format="[%(asctime)s] %(levelname)s: %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%SZ",
)
log = logging.getLogger(__name__)

try:
    from scapy.all import rdpcap, IP
except ImportError:
    log.error("scapy 未安装，请执行: pip install scapy")
    sys.exit(1)

try:
    import numpy as np
except ImportError:
    log.error("numpy 未安装，请执行: pip install numpy")
    sys.exit(1)


# ---------------------------------------------------------------------------
# 常量
# ---------------------------------------------------------------------------

BURST_THRESHOLD_US = 100.0  # burst 判定阈值（微秒）：连续 IAT < 此值视为同一 burst


# ---------------------------------------------------------------------------
# 时间戳与 IAT 提取
# ---------------------------------------------------------------------------

def extract_packet_records(filepath):
    """从 pcapng 文件提取所有 IP 包的时间戳和 IP ID 列表。"""
    log.info("读取: %s", filepath)
    try:
        packets = rdpcap(str(filepath))
    except Exception as exc:
        log.error("读取 pcapng 失败: %s — %s", filepath, exc)
        return [], []

    timestamps = []
    ip_ids = []
    for pkt in packets:
        if pkt.haslayer(IP):
            timestamps.append(float(pkt.time))
            ip_ids.append(int(pkt[IP].id))

    log.info("  提取 %d 个 IP 包记录", len(timestamps))
    return timestamps, ip_ids


def compute_iat_us(timestamps):
    """从有序时间戳列表计算 IAT（微秒）。

    Returns:
        numpy array of IAT values in microseconds, or empty array.
    """
    if len(timestamps) < 2:
        return np.array([])

    ts = np.array(timestamps, dtype=float)
    iat_s = np.diff(ts)
    iat_us = iat_s * 1e6  # 秒 → 微秒
    return iat_us


# ---------------------------------------------------------------------------
# Burst 结构检测
# ---------------------------------------------------------------------------

def detect_bursts(iat_us, threshold_us=BURST_THRESHOLD_US):
    """检测 burst 结构：连续 IAT < threshold 的包组成一个 burst。

    Returns:
        dict with keys: burst_count, burst_mean_size, burst_mean_interval_us
    """
    result = {
        "burst_count": 0,
        "burst_mean_size": 0.0,
        "burst_mean_interval_us": 0.0,
    }

    if len(iat_us) == 0:
        return result

    # 标记每个 IAT 是否属于 burst 内部（< threshold）
    in_burst = iat_us < threshold_us

    bursts = []       # 每个 burst 的包数量（IAT 数 + 1）
    intervals = []    # burst 之间的间隔（微秒）

    current_burst_size = 0
    last_burst_end_idx = None

    for i, is_burst in enumerate(in_burst):
        if is_burst:
            if current_burst_size == 0:
                # burst 开始：记录与上一个 burst 的间隔
                if last_burst_end_idx is not None:
                    gap_iats = iat_us[last_burst_end_idx + 1 : i]
                    if len(gap_iats) > 0:
                        intervals.append(float(np.sum(gap_iats)) + float(iat_us[i]))
                    else:
                        intervals.append(float(iat_us[i]))
            current_burst_size += 1
        else:
            if current_burst_size > 0:
                # burst 结束：burst 包含 current_burst_size 个 IAT → current_burst_size + 1 个包
                bursts.append(current_burst_size + 1)
                last_burst_end_idx = i - 1
                current_burst_size = 0

    # 处理末尾 burst
    if current_burst_size > 0:
        bursts.append(current_burst_size + 1)

    result["burst_count"] = len(bursts)
    if bursts:
        result["burst_mean_size"] = round(float(np.mean(bursts)), 2)
    if intervals:
        result["burst_mean_interval_us"] = round(float(np.mean(intervals)), 2)

    return result


def compute_reorder_and_loss(ip_ids):
    """根据 IP ID 粗略估算乱序率和丢包率。

    仅用于受控/模拟环境参考，不外推为真实网络指标。
    """
    if len(ip_ids) < 2:
        return 0.0, 0.0

    reorder_events = 0
    missing = 0
    prev = ip_ids[0]
    for current in ip_ids[1:]:
        if current < prev:
            reorder_events += 1
        elif current > prev + 1:
            missing += current - prev - 1
        prev = current

    reorder_rate = round(reorder_events / max(1, len(ip_ids) - 1), 4)
    loss_rate = round(missing / max(1, len(ip_ids) + missing), 4)
    return reorder_rate, loss_rate


# ---------------------------------------------------------------------------
# 单文件分析
# ---------------------------------------------------------------------------

def analyze_config(filepath, config_label, burst_threshold_us=BURST_THRESHOLD_US):
    """分析单个 pcapng 文件的 IAT 分布，返回统计结果 dict 或 None。"""
    timestamps, ip_ids = extract_packet_records(filepath)
    if len(timestamps) < 2:
        log.warning("包数不足（需 ≥2）: %s (%d 包)", filepath, len(timestamps))
        return None

    iat_us = compute_iat_us(timestamps)
    if len(iat_us) == 0:
        log.warning("无有效 IAT 数据: %s", filepath)
        return None

    burst_info = detect_bursts(iat_us, threshold_us=burst_threshold_us)
    reorder_rate, loss_rate = compute_reorder_and_loss(ip_ids)

    result = {
        "config": config_label,
        "iat_mean_us": round(float(np.mean(iat_us)), 2),
        "iat_std_us": round(float(np.std(iat_us)), 2),
        "iat_p50_us": round(float(np.percentile(iat_us, 50)), 2),
        "iat_p95_us": round(float(np.percentile(iat_us, 95)), 2),
        "iat_p99_us": round(float(np.percentile(iat_us, 99)), 2),
        "burst_count": burst_info["burst_count"],
        "burst_mean_size": burst_info["burst_mean_size"],
        "burst_mean_interval_us": burst_info["burst_mean_interval_us"],
        "reorder_rate": reorder_rate,
        "loss_rate": loss_rate,
    }

    log.info(
        "  %s: mean=%.2f μs, std=%.2f μs, P50=%.2f, P95=%.2f, P99=%.2f, bursts=%d",
        config_label,
        result["iat_mean_us"],
        result["iat_std_us"],
        result["iat_p50_us"],
        result["iat_p95_us"],
        result["iat_p99_us"],
        result["burst_count"],
    )

    return result


# ---------------------------------------------------------------------------
# CSV 输出
# ---------------------------------------------------------------------------

CSV_COLUMNS = [
    "config",
    "iat_mean_us",
    "iat_std_us",
    "iat_p50_us",
    "iat_p95_us",
    "iat_p99_us",
    "burst_count",
    "burst_mean_size",
    "burst_mean_interval_us",
    "reorder_rate",
    "loss_rate",
]


def write_csv(rows, output_path):
    """将统计结果写入 iat-stats.csv。"""
    with open(output_path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=CSV_COLUMNS)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)
    log.info("已写入: %s (%d 行)", output_path, len(rows))


# ---------------------------------------------------------------------------
# 默认文件映射
# ---------------------------------------------------------------------------

DEFAULT_CONFIGS = {
    "none": "config-none.pcapng",
    "jitter-only": "config-jitter-only.pcapng",
    "vpc-only": "config-vpc-only.pcapng",
    "jitter-vpc": "config-jitter-vpc.pcapng",
}


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="从 pcapng 文件分析 IAT 分布，输出 iat-stats.csv",
    )
    parser.add_argument(
        "pcaps",
        nargs="*",
        help="pcapng 文件路径（格式: label:path 或 path）。"
        "未指定时使用同目录下的默认文件。",
    )
    parser.add_argument(
        "-o", "--output",
        default=None,
        help="输出 CSV 路径（默认: 同目录下 iat-stats.csv）",
    )
    parser.add_argument(
        "--burst-threshold",
        type=float,
        default=BURST_THRESHOLD_US,
        help="burst 判定阈值（微秒，默认: %.1f）" % BURST_THRESHOLD_US,
    )
    args = parser.parse_args()

    burst_threshold = args.burst_threshold

    script_dir = Path(__file__).resolve().parent

    # 构建 (label, path) 列表
    sources = []
    if args.pcaps:
        for entry in args.pcaps:
            if ":" in entry and not entry.startswith("/") and not (len(entry) > 1 and entry[1] == ":"):
                label, path = entry.split(":", 1)
            else:
                label = Path(entry).stem.replace("config-", "")
                path = entry
            sources.append((label, Path(path)))
    else:
        for label, filename in DEFAULT_CONFIGS.items():
            sources.append((label, script_dir / filename))

    output_path = Path(args.output) if args.output else script_dir / "iat-stats.csv"

    # 分析所有文件
    rows = []
    for label, filepath in sources:
        if not filepath.exists():
            log.warning("文件不存在，跳过: %s", filepath)
            continue

        result = analyze_config(filepath, label, burst_threshold_us=burst_threshold)
        if result is not None:
            rows.append(result)

    if not rows:
        log.warning("无有效数据，生成空 CSV（仅表头）")

    write_csv(rows, output_path)
    log.info("分析完成")


if __name__ == "__main__":
    main()
