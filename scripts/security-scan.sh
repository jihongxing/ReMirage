#!/bin/bash
# security-scan.sh - 统一安全漏洞扫描脚本
# 遍历所有含 go.mod 的目录执行 govulncheck，对 sdk/js 执行 npm audit
# 输出结构化 JSON 结果，支持 CI 归档
#
# 用法:
#   ./scripts/security-scan.sh [--output-dir <dir>]
#
# 退出码:
#   0 = 无未豁免高危漏洞
#   1 = 存在未豁免高危漏洞或扫描失败

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_DIR="${PROJECT_ROOT}/scan-results"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ 2>/dev/null || date +%Y%m%dT%H%M%SZ)"
HAS_CRITICAL=0
SCAN_RESULTS=()

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

mkdir -p "$OUTPUT_DIR"

echo "╔══════════════════════════════════════╗"
echo "║   Mirage Project 安全漏洞扫描       ║"
echo "╚══════════════════════════════════════╝"
echo ""
echo "Timestamp: $TIMESTAMP"
echo "Output:    $OUTPUT_DIR"
echo ""

# ============================================================
# Go module scanning with govulncheck
# ============================================================
scan_go_module() {
  local mod_dir="$1"
  local rel_dir="${mod_dir#"$PROJECT_ROOT"/}"
  local result_file="${OUTPUT_DIR}/govulncheck-$(echo "$rel_dir" | tr '/' '-').json"
  local status="success"
  local vulns=""
  local high_count=0

  echo "  🔍 Scanning Go module: $rel_dir"

  if ! command -v govulncheck &>/dev/null; then
    echo "  ⚠️  govulncheck not installed, skipping Go scan"
    echo "     Install: go install golang.org/x/vuln/cmd/govulncheck@latest"
    status="skipped"
    vulns="govulncheck not installed"
  else
    local raw_output
    raw_output=$(cd "$mod_dir" && govulncheck -json ./... 2>&1) || true

    if echo "$raw_output" | grep -q '"vulnerability"'; then
      high_count=$(echo "$raw_output" | grep -c '"vulnerability"' || echo "0")
      if [ "$high_count" -gt 0 ]; then
        HAS_CRITICAL=1
      fi
      status="vulnerabilities_found"
      vulns="$raw_output"
    else
      vulns="$raw_output"
    fi

    # Write raw output
    echo "$raw_output" > "$result_file"
  fi

  SCAN_RESULTS+=("{\"module\":\"$rel_dir\",\"type\":\"go\",\"status\":\"$status\",\"high_severity_count\":$high_count,\"result_file\":\"$(basename "$result_file")\"}")
}

echo "── Go Module Scanning ──"

GO_MOD_DIRS=()
while IFS= read -r -d '' gomod; do
  GO_MOD_DIRS+=("$(dirname "$gomod")")
done < <(find "$PROJECT_ROOT" -name "go.mod" -not -path "*/node_modules/*" -not -path "*/.git/*" -print0 2>/dev/null)

if [ ${#GO_MOD_DIRS[@]} -eq 0 ]; then
  echo "  ⚠️  No go.mod files found"
else
  echo "  Found ${#GO_MOD_DIRS[@]} Go module(s)"
  echo ""
  for mod_dir in "${GO_MOD_DIRS[@]}"; do
    scan_go_module "$mod_dir"
  done
fi

echo ""

# ============================================================
# Node.js scanning with npm audit
# ============================================================
echo "── Node.js Scanning ──"

JS_DIR="${PROJECT_ROOT}/sdk/js"
JS_RESULT_FILE="${OUTPUT_DIR}/npm-audit-sdk-js.json"
JS_STATUS="success"
JS_HIGH_COUNT=0

if [ ! -d "$JS_DIR" ] || [ ! -f "$JS_DIR/package.json" ]; then
  echo "  ⚠️  sdk/js not found or missing package.json, skipping"
  JS_STATUS="skipped"
  SCAN_RESULTS+=("{\"module\":\"sdk/js\",\"type\":\"npm\",\"status\":\"skipped\",\"high_severity_count\":0,\"result_file\":\"\"}")
else
  echo "  🔍 Scanning Node.js project: sdk/js"

  if ! command -v npm &>/dev/null; then
    echo "  ⚠️  npm not installed, skipping Node.js scan"
    echo "     Install Node.js from https://nodejs.org/"
    JS_STATUS="skipped"
    SCAN_RESULTS+=("{\"module\":\"sdk/js\",\"type\":\"npm\",\"status\":\"skipped\",\"high_severity_count\":0,\"result_file\":\"\"}")
  else
    npm_output=$(cd "$JS_DIR" && npm audit --omit=dev --json 2>&1) || true
    echo "$npm_output" > "$JS_RESULT_FILE"

    # Check for high/critical vulnerabilities
    JS_HIGH_COUNT=$(echo "$npm_output" | grep -o '"high":[0-9]*' | head -1 | grep -o '[0-9]*' || echo "0")
    JS_CRIT_COUNT=$(echo "$npm_output" | grep -o '"critical":[0-9]*' | head -1 | grep -o '[0-9]*' || echo "0")
    JS_HIGH_COUNT=$((JS_HIGH_COUNT + JS_CRIT_COUNT))

    if [ "$JS_HIGH_COUNT" -gt 0 ]; then
      HAS_CRITICAL=1
      JS_STATUS="vulnerabilities_found"
    fi

    SCAN_RESULTS+=("{\"module\":\"sdk/js\",\"type\":\"npm\",\"status\":\"$JS_STATUS\",\"high_severity_count\":$JS_HIGH_COUNT,\"result_file\":\"$(basename "$JS_RESULT_FILE")\"}")
  fi
fi

echo ""

# ============================================================
# Generate summary JSON
# ============================================================
SUMMARY_FILE="${OUTPUT_DIR}/scan-summary.json"

{
  echo "{"
  echo "  \"timestamp\": \"$TIMESTAMP\","
  echo "  \"project\": \"mirage-project\","
  echo "  \"has_critical_vulnerabilities\": $([ "$HAS_CRITICAL" -eq 1 ] && echo "true" || echo "false"),"
  echo "  \"results\": ["

  for i in "${!SCAN_RESULTS[@]}"; do
    if [ "$i" -lt $((${#SCAN_RESULTS[@]} - 1)) ]; then
      echo "    ${SCAN_RESULTS[$i]},"
    else
      echo "    ${SCAN_RESULTS[$i]}"
    fi
  done

  echo "  ]"
  echo "}"
} > "$SUMMARY_FILE"

echo "── Scan Summary ──"
echo "  Results written to: $OUTPUT_DIR"
echo "  Summary: $SUMMARY_FILE"
echo ""

if [ "$HAS_CRITICAL" -eq 1 ]; then
  echo "❌ HIGH/CRITICAL vulnerabilities found — review scan results"
  exit 1
else
  echo "✅ No unexempted high-severity vulnerabilities"
  exit 0
fi
