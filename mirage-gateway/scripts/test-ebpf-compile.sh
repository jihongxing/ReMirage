#!/bin/bash
# test-ebpf-compile.sh — eBPF 编译回归验证（Linux 环境）
# 用法: ./scripts/test-ebpf-compile.sh
# 退出码: 0=全部通过, 1=编译失败
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BPF_DIR="$(cd "$SCRIPT_DIR/../bpf" && pwd)"
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

if ! command -v clang &>/dev/null; then
  echo "❌ clang not found. Install: sudo apt-get install -y clang llvm"
  exit 1
fi

KEY_FILES=(npm.c bdna.c jitter.c l1_defense.c l1_silent.c)
PASS=0
FAIL=0

echo "═══ eBPF Compile Regression Test ═══"
echo "clang: $(clang --version | head -1)"
echo "bpf/: $BPF_DIR"
echo ""

for f in "${KEY_FILES[@]}"; do
  src="$BPF_DIR/$f"
  out="$TMP_DIR/${f%.c}.o"
  if [ ! -f "$src" ]; then
    echo "❌ MISSING: $f"
    FAIL=$((FAIL+1))
    continue
  fi
  if clang -O2 -target bpf -I "$BPF_DIR" -c "$src" -o "$out" 2>&1; then
    size=$(stat -c%s "$out" 2>/dev/null || stat -f%z "$out" 2>/dev/null)
    echo "✅ $f → ${f%.c}.o ($size bytes)"
    PASS=$((PASS+1))
  else
    echo "❌ FAIL: $f"
    FAIL=$((FAIL+1))
  fi
done

echo ""
echo "Result: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
