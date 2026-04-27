#!/usr/bin/env python3
"""
简单分类器可分性实验（检测面 4）

训练 RandomForest + XGBoost 分类器，量化 ReMirage 流量与对照组流量的可区分性。
用于回答核心问题：一个简单分类器能否区分 ReMirage 流量与对照组流量？

输入: features.csv（检测面 1-3 的特征数据）
输出: results.json（AUC、F1、准确率、混淆矩阵）

Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.5, 5.6
"""

import argparse
import json
import os
import sys
import warnings
from datetime import datetime, timezone

import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestClassifier
from sklearn.metrics import (
    accuracy_score,
    confusion_matrix,
    f1_score,
    roc_auc_score,
)
from sklearn.model_selection import StratifiedKFold, train_test_split

# XGBoost 降级策略
XGBOOST_AVAILABLE = True
try:
    from xgboost import XGBClassifier
except ImportError:
    XGBOOST_AVAILABLE = False

warnings.filterwarnings("ignore", category=UserWarning)

RANDOM_STATE = 42

# 特征集定义（来自实验方案 §6.3）
HANDSHAKE_FEATURES = [
    "tcp_window", "tcp_mss", "tcp_wscale",
    "tcp_sack", "tcp_timestamps", "tls_ext_count",
]

PACKET_LENGTH_FEATURES = [
    "pkt_len_1", "pkt_len_2", "pkt_len_3", "pkt_len_4", "pkt_len_5",
    "pkt_len_6", "pkt_len_7", "pkt_len_8", "pkt_len_9", "pkt_len_10",
    "pkt_dir_1", "pkt_dir_2", "pkt_dir_3", "pkt_dir_4", "pkt_dir_5",
    "pkt_dir_6", "pkt_dir_7", "pkt_dir_8", "pkt_dir_9", "pkt_dir_10",
    "up_down_ratio", "pkt_len_entropy", "pkt_len_mean", "pkt_len_std",
]

TIMING_FEATURES = [
    "iat_mean", "iat_std", "iat_p50", "iat_p95", "iat_p99",
    "burst_count", "burst_mean_size", "burst_mean_interval",
]

EXPERIMENTS = [
    {"id": "C1", "name": "仅握手特征", "feature_set": "handshake", "features": HANDSHAKE_FEATURES},
    {"id": "C2", "name": "仅包长特征", "feature_set": "packet_length", "features": PACKET_LENGTH_FEATURES},
    {"id": "C3", "name": "仅时序特征", "feature_set": "timing", "features": TIMING_FEATURES},
    {"id": "C4", "name": "握手+包长+时序联合", "feature_set": "combined",
     "features": HANDSHAKE_FEATURES + PACKET_LENGTH_FEATURES + TIMING_FEATURES},
]

AUC_HIGH_RISK = 0.9
AUC_MEDIUM_RISK = 0.7

LIMITATIONS = [
    "分类器复杂度有限（RandomForest / XGBoost），不代表高级 DPI/ML 系统的检测能力",
    "受控环境样本不包含真实网络噪声、中间设备干扰和多跳路由影响",
    "样本规模受限于本地采集能力，不代表大规模流量场景",
    "本实验结论不可外推为'可抵抗生产级 DPI/ML 系统'",
    "基于受控本地环境实验，不代表真实对抗网络环境下的效果",
]


def load_features(csv_path):
    if not os.path.exists(csv_path):
        print(f"错误: 特征文件不存在: {csv_path}", file=sys.stderr)
        sys.exit(1)
    df = pd.read_csv(csv_path)
    if "label" not in df.columns:
        print("错误: features.csv 缺少 'label' 列", file=sys.stderr)
        sys.exit(1)
    return df


def get_available_features(df, requested):
    return [f for f in requested if f in df.columns]


def risk_label(auc, experiment_id):
    if experiment_id == "C4":
        return "综合高可区分性风险" if auc > AUC_HIGH_RISK else ""
    if auc > AUC_HIGH_RISK:
        return "高可区分性风险"
    if auc >= AUC_MEDIUM_RISK:
        return "中等可区分性"
    return "低可区分性"


def build_classifiers():
    classifiers = [
        ("RandomForest", RandomForestClassifier(n_estimators=100, random_state=RANDOM_STATE)),
    ]
    if XGBOOST_AVAILABLE:
        classifiers.append(
            ("XGBoost", XGBClassifier(
                n_estimators=100, random_state=RANDOM_STATE,
                use_label_encoder=False, eval_metric="logloss")),
        )
    return classifiers


def evaluate_classifier(clf, X_train, X_test, y_train, y_test, X_full, y_full):
    clf.fit(X_train, y_train)
    y_pred = clf.predict(X_test)
    y_proba = clf.predict_proba(X_test)[:, 1]
    auc = float(roc_auc_score(y_test, y_proba))
    f1 = float(f1_score(y_test, y_pred))
    accuracy = float(accuracy_score(y_test, y_pred))
    cm = confusion_matrix(y_test, y_pred).tolist()

    skf = StratifiedKFold(n_splits=5, shuffle=True, random_state=RANDOM_STATE)
    cv_aucs = []
    for train_idx, val_idx in skf.split(X_full, y_full):
        clf_cv = clf.__class__(**clf.get_params())
        clf_cv.fit(X_full.iloc[train_idx], y_full.iloc[train_idx])
        val_proba = clf_cv.predict_proba(X_full.iloc[val_idx])[:, 1]
        cv_aucs.append(float(roc_auc_score(y_full.iloc[val_idx], val_proba)))

    return {
        "auc": round(auc, 4), "f1": round(f1, 4), "accuracy": round(accuracy, 4),
        "confusion_matrix": cm,
        "cv_auc_mean": round(float(np.mean(cv_aucs)), 4),
        "cv_auc_std": round(float(np.std(cv_aucs)), 4),
    }


def run_experiment(df, experiment, classifiers):
    exp_id = experiment["id"]
    available = get_available_features(df, experiment["features"])
    if not available:
        return {"id": exp_id, "name": experiment["name"], "feature_set": experiment["feature_set"],
                "status": "跳过 — 无可用特征列", "classifiers": []}

    X = df[available].copy().fillna(0)
    y = df["label"].copy()
    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.3, stratify=y, random_state=RANDOM_STATE)

    result = {"id": exp_id, "name": experiment["name"], "feature_set": experiment["feature_set"],
              "features_used": available, "feature_count": len(available),
              "sample_count": len(df), "train_count": len(X_train), "test_count": len(X_test),
              "classifiers": []}

    for clf_name, clf in classifiers:
        metrics = evaluate_classifier(clf, X_train, X_test, y_train, y_test, X, y)
        metrics["name"] = clf_name
        label = risk_label(metrics["auc"], exp_id)
        if label:
            metrics["risk_label"] = label
        result["classifiers"].append(metrics)
    return result


def main():
    parser = argparse.ArgumentParser(description="简单分类器可分性实验（检测面 4）")
    parser.add_argument("--input", "-i",
                        default=os.path.join(os.path.dirname(__file__), "features.csv"),
                        help="特征数据 CSV 路径（默认: features.csv）")
    parser.add_argument("--output", "-o",
                        default=os.path.join(os.path.dirname(__file__), "results.json"),
                        help="结果输出 JSON 路径（默认: results.json）")
    args = parser.parse_args()

    print(f"加载特征数据: {args.input}")
    df = load_features(args.input)
    print(f"  样本数: {len(df)}, 标签分布: {dict(df['label'].value_counts())}")

    classifiers = build_classifiers()
    clf_names = [name for name, _ in classifiers]
    print(f"分类器: {clf_names}")
    if not XGBOOST_AVAILABLE:
        print("  ⚠ XGBoost 不可用，仅使用 RandomForest")

    experiments_results = []
    for exp in EXPERIMENTS:
        print(f"\n运行实验 {exp['id']}: {exp['name']}")
        result = run_experiment(df, exp, classifiers)
        experiments_results.append(result)
        for clf_result in result.get("classifiers", []):
            risk = clf_result.get("risk_label", "")
            risk_str = f" ⚠ {risk}" if risk else ""
            print(f"  {clf_result['name']}: AUC={clf_result['auc']}, F1={clf_result['f1']}, Acc={clf_result['accuracy']}{risk_str}")

    output = {
        "experiments": experiments_results,
        "metadata": {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "random_state": RANDOM_STATE, "split_ratio": "70/30 stratified",
            "cross_validation": "5-fold", "classifiers": clf_names,
            "xgboost_available": XGBOOST_AVAILABLE,
            "input_file": os.path.basename(args.input), "sample_count": len(df),
            "label_distribution": {"control_0": int((df["label"] == 0).sum()),
                                   "remirage_1": int((df["label"] == 1).sum())},
        },
        "limitations": LIMITATIONS,
    }
    if not XGBOOST_AVAILABLE:
        output["metadata"]["degradation_notice"] = "XGBoost 不可用，仅 RandomForest 结果"

    os.makedirs(os.path.dirname(args.output) or ".", exist_ok=True)
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(output, f, ensure_ascii=False, indent=2)
    print(f"\n结果已写入: {args.output}")


if __name__ == "__main__":
    main()
