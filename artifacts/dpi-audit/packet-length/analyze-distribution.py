#!/usr/bin/env python3
"""从 pcapng 文件分析包长分布，输出 distributions.csv + 直方图。

统计维度：
  - 前 10 包 IP 长度
  - 前 10 包方向、上下行比例
  - 整体包长直方图（bin=10 bytes）
  - Shannon 熵、均值、标准差
  - KL 散度、JS 散度（各模式 vs 无填充基线）

依赖: scapy, numpy (pip install scapy numpy)
需求: 3.2, 3.5, 3.6
"""

import argparse
import csv
import logging
import os
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

HIST_BIN_SIZE = 10  # 直方图 bin 宽度（bytes）
MAX_PKT_LEN = 1600  # 直方图上界
EPSILON = 1e-12  # 避免 log(0)


# ---------------------------------------------------------------------------
# 包长提取
# ---------------------------------------------------------------------------

def extract_packet_records(filepath):
    """从 pcapng 文件提取所有 IP 包的总长度和方向列表。"""
    log.info("读取: %s", filepath)
    try:
        packets = rdpcap(str(filepath))
    except Exception as exc:
        log.error("读取 pcapng 失败: %s — %s", filepath, exc)
        return [], []

    lengths = []
    directions = []
    initiator = None
    for pkt in packets:
        if pkt.haslayer(IP):
            ip = pkt[IP]
            lengths.append(ip.len)
            if initiator is None:
                initiator = ip.src
            directions.append(1 if ip.src == initiator else -1)

    log.info("  提取 %d 个 IP 包记录", len(lengths))
    return lengths, directions


# ---------------------------------------------------------------------------
# 直方图构建
# ---------------------------------------------------------------------------

def build_histogram(lengths):
    """构建包长直方图（bin=HIST_BIN_SIZE bytes），返回归一化概率分布。

    Returns:
        (bin_edges, counts, probs) — bin 边界、原始计数、归一化概率
    """
    if not lengths:
        return np.array([]), np.array([]), np.array([])

    bins = np.arange(0, MAX_PKT_LEN + HIST_BIN_SIZE, HIST_BIN_SIZE)
    counts, bin_edges = np.histogram(lengths, bins=bins)
    total = counts.sum()
    probs = counts / total if total > 0 else counts.astype(float)
    return bin_edges, counts, probs


# ---------------------------------------------------------------------------
# 统计指标
# ---------------------------------------------------------------------------

def shannon_entropy(probs):
    """计算 Shannon 熵: H = -sum(p_i * log2(p_i))。"""
    p = probs[probs > 0]
    if len(p) == 0:
        return 0.0
    return -np.sum(p * np.log2(p))


def kl_divergence(p, q):
    """计算 KL 散度: D_KL(P || Q)。

    P、Q 必须长度相同且已归一化。对零概率 bin 加 epsilon 平滑。
    """
    p = np.asarray(p, dtype=float) + EPSILON
    q = np.asarray(q, dtype=float) + EPSILON
    p = p / p.sum()
    q = q / q.sum()
    return np.sum(p * np.log(p / q))


def js_divergence(p, q):
    """计算 JS 散度: D_JS = 0.5 * D_KL(P||M) + 0.5 * D_KL(Q||M), M=(P+Q)/2。"""
    p = np.asarray(p, dtype=float) + EPSILON
    q = np.asarray(q, dtype=float) + EPSILON
    p = p / p.sum()
    q = q / q.sum()
    m = 0.5 * (p + q)
    return 0.5 * np.sum(p * np.log(p / m)) + 0.5 * np.sum(q * np.log(q / m))


# ---------------------------------------------------------------------------
# 单文件分析
# ---------------------------------------------------------------------------

def analyze_source(filepath, source_label, baseline_probs=None):
    """分析单个 pcapng 文件，返回 (统计结果 dict, 概率分布, bin_edges, counts) 或 None。"""
    lengths, directions = extract_packet_records(filepath)
    if not lengths:
        log.warning("无 IP 包数据: %s", filepath)
        return None

    arr = np.array(lengths, dtype=float)
    first_10 = lengths[:10]
    first_10_dirs = directions[:10]
    bin_edges, counts, probs = build_histogram(lengths)

    result = {
        "source": source_label,
        "up_down_ratio": round(float(sum(1 for value in directions if value > 0)) / max(1, sum(1 for value in directions if value < 0)), 4),
        "pkt_len_mean": round(float(np.mean(arr)), 2),
        "pkt_len_std": round(float(np.std(arr)), 2),
        "pkt_len_entropy": round(float(shannon_entropy(probs)), 4),
        "kl_divergence": "",
        "js_divergence": "",
        "first_10_lengths": ";".join(str(v) for v in first_10),
    }

    for idx in range(10):
        result[f"pkt_len_{idx + 1}"] = first_10[idx] if idx < len(first_10) else ""
        result[f"pkt_dir_{idx + 1}"] = first_10_dirs[idx] if idx < len(first_10_dirs) else ""

    if baseline_probs is not None and len(baseline_probs) == len(probs):
        result["kl_divergence"] = round(float(kl_divergence(probs, baseline_probs)), 6)
        result["js_divergence"] = round(float(js_divergence(probs, baseline_probs)), 6)

    return result, probs, bin_edges, counts


# ---------------------------------------------------------------------------
# CSV 输出
# ---------------------------------------------------------------------------

CSV_COLUMNS = [
    "source",
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
    "pkt_len_mean",
    "pkt_len_std",
    "pkt_len_entropy",
    "kl_divergence",
    "js_divergence",
    "first_10_lengths",
]


def write_csv(rows, output_path):
    """将统计结果写入 distributions.csv。"""
    with open(output_path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=CSV_COLUMNS)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)
    log.info("已写入: %s (%d 行)", output_path, len(rows))


# ---------------------------------------------------------------------------
# 直方图输出（文本格式，无 matplotlib 依赖）
# ---------------------------------------------------------------------------

def write_histogram_txt(source_label, bin_edges, counts, output_dir):
    """将直方图数据写入文本文件。"""
    hist_path = output_dir / f"histogram-{source_label}.txt"
    with open(hist_path, "w", encoding="utf-8") as f:
        f.write(f"# 包长直方图: {source_label}\n")
        f.write(f"# bin_start\tbin_end\tcount\n")
        for i in range(len(counts)):
            f.write(f"{int(bin_edges[i])}\t{int(bin_edges[i+1])}\t{int(counts[i])}\n")
    log.info("直方图已写入: %s", hist_path)


# ---------------------------------------------------------------------------
# 默认文件映射
# ---------------------------------------------------------------------------

BASELINE_KEY = "baseline-no-padding"

DEFAULT_SOURCES = {
    BASELINE_KEY: "baseline-no-padding.pcapng",
    "mode-fixed-mtu": "mode-fixed-mtu.pcapng",
    "mode-random-range": "mode-random-range.pcapng",
    "mode-gaussian": "mode-gaussian.pcapng",
}


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="从 pcapng 文件分析包长分布，输出 distributions.csv",
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
        help="输出 CSV 路径（默认: 同目录下 distributions.csv）",
    )
    parser.add_argument(
        "--baseline",
        default=None,
        help="基线文件 label（默认: baseline-no-padding）",
    )
    args = parser.parse_args()

    script_dir = Path(__file__).resolve().parent
    baseline_label = args.baseline or BASELINE_KEY

    # 构建 (label, path) 列表
    sources = []
    if args.pcaps:
        for entry in args.pcaps:
            if ":" in entry and not entry.startswith("/") and not (len(entry) > 1 and entry[1] == ":"):
                label, path = entry.split(":", 1)
            else:
                label = Path(entry).stem
                path = entry
            sources.append((label, Path(path)))
    else:
        for label, filename in DEFAULT_SOURCES.items():
            sources.append((label, script_dir / filename))

    output_path = Path(args.output) if args.output else script_dir / "distributions.csv"

    # 第一遍：处理基线文件，获取基线概率分布
    baseline_probs = None
    baseline_path = None
    for label, filepath in sources:
        if label == baseline_label:
            baseline_path = filepath
            break

    if baseline_path and baseline_path.exists():
        baseline_lengths, _ = extract_packet_records(baseline_path)
        if baseline_lengths:
            _, _, baseline_probs = build_histogram(baseline_lengths)
            log.info("基线分布已加载: %s (%d 包)", baseline_label, len(baseline_lengths))
    else:
        log.warning("基线文件不存在或未指定，KL/JS 散度将为空")

    # 第二遍：分析所有文件
    rows = []
    for label, filepath in sources:
        if not filepath.exists():
            log.warning("文件不存在，跳过: %s", filepath)
            continue

        # 基线自身不计算散度
        bp = None if label == baseline_label else baseline_probs
        result = analyze_source(filepath, label, baseline_probs=bp)
        if result is None:
            continue

        row, probs, bin_edges, counts = result
        rows.append(row)

        # 输出直方图文本文件
        write_histogram_txt(label, bin_edges, counts, script_dir)

    if not rows:
        log.warning("无有效数据，生成空 CSV（仅表头）")

    write_csv(rows, output_path)
    log.info("分析完成")


if __name__ == "__main__":
    main()
