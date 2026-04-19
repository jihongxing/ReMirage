#!/bin/bash
# Mirage Gateway 负载测试脚本
# 调用 Go benchmark 并汇总输出资源占用报告
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPORT_FILE="${1:-/tmp/mirage-load-test-report.txt}"

echo "========================================" | tee "$REPORT_FILE"
echo "  Mirage Gateway 负载测试报告" | tee -a "$REPORT_FILE"
echo "  时间: $(date -Iseconds)" | tee -a "$REPORT_FILE"
echo "========================================" | tee -a "$REPORT_FILE"

echo "" | tee -a "$REPORT_FILE"
echo "--- FEC 编码延迟 ---" | tee -a "$REPORT_FILE"
cd "$SCRIPT_DIR"
go test -bench=BenchmarkFECEncode -benchtime=100x -count=1 -run=^$ ./... 2>&1 | tee -a "$REPORT_FILE"

echo "" | tee -a "$REPORT_FILE"
echo "--- FEC 解码延迟 ---" | tee -a "$REPORT_FILE"
go test -bench=BenchmarkFECDecode -benchtime=100x -count=1 -run=^$ ./... 2>&1 | tee -a "$REPORT_FILE"

echo "" | tee -a "$REPORT_FILE"
echo "--- G-Switch 转生延迟 ---" | tee -a "$REPORT_FILE"
go test -bench=BenchmarkGSwitchReincarnation -benchtime=10x -count=1 -run=^$ ./... 2>&1 | tee -a "$REPORT_FILE"

echo "" | tee -a "$REPORT_FILE"
echo "--- 资源占用 (10/50/100 并发) ---" | tee -a "$REPORT_FILE"
go test -bench=BenchmarkResourceUsage -benchtime=1x -count=1 -run=^$ ./... 2>&1 | tee -a "$REPORT_FILE"

echo "" | tee -a "$REPORT_FILE"
echo "========================================" | tee -a "$REPORT_FILE"
echo "  报告已保存: $REPORT_FILE" | tee -a "$REPORT_FILE"
echo "========================================" | tee -a "$REPORT_FILE"
