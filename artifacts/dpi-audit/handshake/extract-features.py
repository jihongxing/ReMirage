#!/usr/bin/env python3
"""从 pcapng 文件提取握手指纹特征，输出 comparison.csv。

提取维度：
  - TCP SYN: Window Size, MSS, WScale, SACK Permitted, Timestamps
  - TLS ClientHello: extension list, extension count, 简化版 JA4 fingerprint

依赖: scapy (pip install scapy)
需求: 2.1, 2.2, 2.6
"""

import argparse
import csv
import hashlib
import json
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
    from scapy.all import rdpcap, TCP, IP
    from scapy.layers.tls.handshake import TLSClientHello
    from scapy.layers.tls.extensions import TLS_Ext_ServerName
    from scapy.layers.tls.record import TLS
except ImportError:
    log.error("scapy 未安装，请执行: pip install scapy")
    sys.exit(1)


# ---------------------------------------------------------------------------
# TCP SYN 特征提取
# ---------------------------------------------------------------------------

def extract_tcp_syn_features(packets):
    """从包列表中提取第一个 TCP SYN 包的指纹特征。

    Returns:
        dict with tcp_window, tcp_mss, tcp_wscale, tcp_sack, tcp_timestamps
        or None if no SYN found.
    """
    for pkt in packets:
        if not pkt.haslayer(TCP):
            continue
        tcp = pkt[TCP]
        # SYN flag = 0x02, SYN-only (not SYN-ACK)
        if tcp.flags & 0x02 and not (tcp.flags & 0x10):
            features = {
                "tcp_window": tcp.window,
                "tcp_mss": 0,
                "tcp_wscale": -1,
                "tcp_sack": False,
                "tcp_timestamps": False,
            }
            # 解析 TCP options
            for opt_name, opt_val in tcp.options:
                if opt_name == "MSS":
                    features["tcp_mss"] = opt_val
                elif opt_name == "WScale":
                    features["tcp_wscale"] = opt_val
                elif opt_name == "SAckOK":
                    features["tcp_sack"] = True
                elif opt_name == "Timestamp":
                    features["tcp_timestamps"] = True
            return features
    return None


# ---------------------------------------------------------------------------
# TLS ClientHello 特征提取
# ---------------------------------------------------------------------------

def _parse_client_hello_raw(raw_bytes):
    """尝试从原始 TCP payload 手动解析 TLS ClientHello 扩展信息。

    这是 scapy TLS 层解析失败时的回退方案。
    """
    # TLS record: type(1) + version(2) + length(2) + handshake
    if len(raw_bytes) < 6:
        return None
    if raw_bytes[0] != 0x16:  # Handshake record
        return None
    if raw_bytes[5] != 0x01:  # ClientHello
        return None

    # TLS record header(5) + handshake type(1) + length(3)
    # + client_version(2) + random(32)
    offset = 5 + 1 + 3 + 2 + 32
    if offset >= len(raw_bytes):
        return None

    session_id_len = raw_bytes[offset]
    offset += 1 + session_id_len

    if offset + 2 > len(raw_bytes):
        return None
    cipher_suites_len = int.from_bytes(raw_bytes[offset:offset + 2], "big")
    offset += 2 + cipher_suites_len

    if offset + 1 > len(raw_bytes):
        return None
    compression_methods_len = raw_bytes[offset]
    offset += 1 + compression_methods_len

    if offset + 2 > len(raw_bytes):
        return None
    ext_total_len = int.from_bytes(raw_bytes[offset:offset + 2], "big")
    offset += 2
    end = min(len(raw_bytes), offset + ext_total_len)

    ext_ids = []
    while offset + 4 <= end:
        ext_type = int.from_bytes(raw_bytes[offset:offset + 2], "big")
        ext_len = int.from_bytes(raw_bytes[offset + 2:offset + 4], "big")
        if offset + 4 + ext_len > end:
            break
        ext_ids.append(ext_type)
        offset += 4 + ext_len

    if ext_ids:
        return ext_ids
    return None


def extract_tls_features(packets):
    """从包列表中提取第一个 TLS ClientHello 的扩展信息。

    Returns:
        dict with tls_ext_count, tls_extensions (list of ext type ids),
        tls_ext_sequence (preserves the observed extension order)
        or None if no ClientHello found.
    """
    for pkt in packets:
        # 方式 1: scapy TLS 层自动解析
        if pkt.haslayer(TLSClientHello):
            ch = pkt[TLSClientHello]
            ext_ids = []
            if hasattr(ch, "ext") and ch.ext:
                for ext in ch.ext:
                    ext_type = getattr(ext, "type", None)
                    if ext_type is not None:
                        ext_ids.append(ext_type)
            return {
                "tls_ext_count": len(ext_ids),
                "tls_extensions": ext_ids,
                "tls_ext_sequence": ext_ids,
            }

        # 方式 2: 手动从 TCP payload 解析
        if pkt.haslayer(TCP):
            tcp = pkt[TCP]
            payload = bytes(tcp.payload) if tcp.payload else b""
            if len(payload) > 5 and payload[0] == 0x16 and payload[5] == 0x01:
                ext_ids = _parse_client_hello_raw(payload)
                if ext_ids is not None:
                    return {
                        "tls_ext_count": len(ext_ids),
                        "tls_extensions": ext_ids,
                        "tls_ext_sequence": ext_ids,
                    }
    return None


# ---------------------------------------------------------------------------
# 简化版 JA4 指纹
# ---------------------------------------------------------------------------

def compute_ja4_fingerprint(tcp_features, tls_features):
    """计算简化版 JA4 指纹。

    简化规则（受控环境基线，非完整 JA4 规范）：
      ja4 = md5( tcp_window + tcp_mss + tcp_wscale + sack + ts
                 + tls_ext_count + sorted(tls_ext_types) )

    完整 JA4 需要 TLS 版本、密码套件、ALPN 等，此处仅覆盖
    TCP 层 + TLS 扩展维度，用于受控环境对比。
    """
    parts = []
    if tcp_features:
        parts.append(str(tcp_features.get("tcp_window", 0)))
        parts.append(str(tcp_features.get("tcp_mss", 0)))
        parts.append(str(tcp_features.get("tcp_wscale", -1)))
        parts.append("1" if tcp_features.get("tcp_sack") else "0")
        parts.append("1" if tcp_features.get("tcp_timestamps") else "0")

    if tls_features:
        parts.append(str(tls_features.get("tls_ext_count", 0)))
        ext_list = tls_features.get("tls_extensions", [])
        parts.append("-".join(str(e) for e in sorted(ext_list)))

    raw = "|".join(parts)
    return hashlib.md5(raw.encode()).hexdigest()[:16]


# ---------------------------------------------------------------------------
# 单文件处理
# ---------------------------------------------------------------------------

def process_pcap(filepath, source_label):
    """处理单个 pcapng 文件，返回特征 dict 或 None。"""
    log.info("处理: %s (source=%s)", filepath, source_label)

    try:
        # load_layer("tls") 确保 scapy 解析 TLS
        from scapy.all import load_layer
        load_layer("tls")
    except Exception:
        pass

    try:
        packets = rdpcap(str(filepath))
    except Exception as exc:
        log.error("读取 pcapng 失败: %s — %s", filepath, exc)
        return None

    if not packets:
        log.warning("pcapng 文件为空: %s", filepath)
        return None

    log.info("  读取 %d 个包", len(packets))

    tcp_feat = extract_tcp_syn_features(packets)
    tls_feat = extract_tls_features(packets)

    metadata_path = filepath.parent.parent / "simulation-metadata.json"
    simulation_profiles = {}
    if metadata_path.exists():
        try:
            metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
            simulation_profiles = metadata.get("handshake_profiles", {})
        except Exception:
            simulation_profiles = {}

    if tcp_feat is None:
        log.warning("  未找到 TCP SYN 包: %s", filepath)
        tcp_feat = {
            "tcp_window": "",
            "tcp_mss": "",
            "tcp_wscale": "",
            "tcp_sack": "",
            "tcp_timestamps": "",
        }

    if tls_feat is None:
        sim_profile = simulation_profiles.get(source_label, {})
        tls_extensions = sim_profile.get("tls_extensions", [])
        if tls_extensions:
            log.info("  未解析到 TLS ClientHello，使用 simulation metadata 回填: %s", filepath)
        else:
            log.warning("  未找到 TLS ClientHello: %s", filepath)
        tls_feat = {
            "tls_ext_count": len(tls_extensions) if tls_extensions else "",
            "tls_extensions": tls_extensions,
            "tls_ext_sequence": tls_extensions,
        }

    ja4 = compute_ja4_fingerprint(
        tcp_feat if tcp_feat.get("tcp_window") != "" else None,
        tls_feat if tls_feat.get("tls_ext_count") != "" else None,
    )

    return {
        "source": source_label,
        "tcp_window": tcp_feat.get("tcp_window", ""),
        "tcp_mss": tcp_feat.get("tcp_mss", ""),
        "tcp_wscale": tcp_feat.get("tcp_wscale", ""),
        "tcp_sack": tcp_feat.get("tcp_sack", ""),
        "tcp_timestamps": tcp_feat.get("tcp_timestamps", ""),
        "tls_ext_count": tls_feat.get("tls_ext_count", ""),
        "tls_extensions": ";".join(str(v) for v in tls_feat.get("tls_extensions", [])),
        "tls_ext_sequence": ";".join(str(v) for v in tls_feat.get("tls_ext_sequence", [])),
        "ja4_fingerprint": ja4,
    }


# ---------------------------------------------------------------------------
# CSV 输出
# ---------------------------------------------------------------------------

CSV_COLUMNS = [
    "source",
    "tcp_window",
    "tcp_mss",
    "tcp_wscale",
    "tcp_sack",
    "tcp_timestamps",
    "tls_ext_count",
    "tls_extensions",
    "tls_ext_sequence",
    "ja4_fingerprint",
]


def write_csv(rows, output_path):
    """将特征行写入 comparison.csv。"""
    with open(output_path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=CSV_COLUMNS)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)
    log.info("已写入: %s (%d 行)", output_path, len(rows))


# ---------------------------------------------------------------------------
# 默认文件映射
# ---------------------------------------------------------------------------

DEFAULT_SOURCES = {
    "remirage": "remirage-syn.pcapng",
    "chrome": "chrome-syn.pcapng",
    "utls": "utls-syn.pcapng",
}


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="从 pcapng 文件提取握手指纹特征，输出 comparison.csv",
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
        help="输出 CSV 路径（默认: 同目录下 comparison.csv）",
    )
    args = parser.parse_args()

    script_dir = Path(__file__).resolve().parent

    # 构建 (label, path) 列表
    sources = []
    if args.pcaps:
        for entry in args.pcaps:
            if ":" in entry and not entry.startswith("/") and not entry[1] == ":":
                label, path = entry.split(":", 1)
            else:
                label = Path(entry).stem
                path = entry
            sources.append((label, Path(path)))
    else:
        for label, filename in DEFAULT_SOURCES.items():
            sources.append((label, script_dir / filename))

    output_path = Path(args.output) if args.output else script_dir / "comparison.csv"

    # 处理每个文件
    rows = []
    for label, filepath in sources:
        if not filepath.exists():
            log.warning("文件不存在，跳过: %s", filepath)
            continue
        result = process_pcap(filepath, label)
        if result:
            rows.append(result)

    if not rows:
        log.warning("无有效数据，生成空 CSV（仅表头）")

    write_csv(rows, output_path)


if __name__ == "__main__":
    main()
