#!/usr/bin/env bash
# Minimal eBPF compile/load smoke for Linux.
#
# The script always runs `make bpf`. It then attempts a verifier load with
# bpftool only when root privileges and bpftool are available. Set
# REQUIRE_EBPF_LOAD=1 to make skipped load checks fail the script.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATEWAY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PIN_ROOT="${PIN_ROOT:-/sys/fs/bpf/remirage-smoke}"
REQUIRE_EBPF_LOAD="${REQUIRE_EBPF_LOAD:-0}"

skip_load() {
  local reason="$1"
  echo "SKIP load smoke: $reason"
  if [[ "$REQUIRE_EBPF_LOAD" == "1" ]]; then
    exit 1
  fi
  exit 0
}

echo "== eBPF compile smoke =="
cd "$GATEWAY_DIR"
make bpf

echo
echo "== eBPF load smoke =="

command -v bpftool >/dev/null 2>&1 || skip_load "bpftool not found"
[[ "${EUID:-$(id -u)}" -eq 0 ]] || skip_load "root privileges required"

if ! mountpoint -q /sys/fs/bpf; then
  mount -t bpf bpf /sys/fs/bpf 2>/dev/null || skip_load "/sys/fs/bpf is not mounted and could not be mounted"
fi

case "$PIN_ROOT" in
  ""|"/"|"/sys"|"/sys/"|"/sys/fs"|"/sys/fs/"|"/sys/fs/bpf"|"/sys/fs/bpf/")
    echo "FAIL: unsafe PIN_ROOT: $PIN_ROOT"
    exit 1
    ;;
esac

rm -rf "$PIN_ROOT"
mkdir -p "$PIN_ROOT"
cleanup() {
  rm -rf "$PIN_ROOT"
}
trap cleanup EXIT

shopt -s nullglob
objects=(bpf/*.o)
if [[ "${#objects[@]}" -eq 0 ]]; then
  echo "FAIL: no BPF objects found after make bpf"
  exit 1
fi

for obj in "${objects[@]}"; do
  name="$(basename "$obj" .o)"
  obj_pin_dir="$PIN_ROOT/$name"
  mkdir -p "$obj_pin_dir"
  echo "loadall $obj"
  bpftool prog loadall "$obj" "$obj_pin_dir"
done

echo "PASS: compiled and verifier-loaded ${#objects[@]} eBPF object(s)"
