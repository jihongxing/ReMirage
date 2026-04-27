#!/usr/bin/env bash
# Runtime eBPF attach/detach smoke for Linux.
#
# This script creates a temporary veth pair, runs the real Go Loader against one
# side, verifies control-plane map visibility, then closes the loader and removes
# the veth pair. It is intentionally separate from smoke-ebpf-load.sh:
# - smoke-ebpf-load.sh: compile + verifier load
# - smoke-ebpf-runtime-attach.sh: loader wiring + TC/XDP/cgroup attach/detach

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATEWAY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
RUN_MAKE_BPF="${RUN_MAKE_BPF:-1}"
REQUIRE_BDNA_MAPS="${REQUIRE_BDNA_MAPS:-0}"

pid="${BASHPID:-$$}"
SMOKE_IFACE="${SMOKE_IFACE:-rmg${pid}a}"
SMOKE_PEER="${SMOKE_PEER:-rmg${pid}b}"
TMP_DIR="$GATEWAY_DIR/.tmp-ebpf-runtime-smoke"
HELPER="$TMP_DIR/main.go"
created_link=0

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cleanup() {
  if [[ "$created_link" == "1" ]]; then
    ip link del "$SMOKE_IFACE" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

[[ "$(uname -s)" == "Linux" ]] || fail "Linux is required"
[[ "${EUID:-$(id -u)}" -eq 0 ]] || fail "root privileges are required"

command -v ip >/dev/null 2>&1 || fail "ip command not found"
command -v tc >/dev/null 2>&1 || fail "tc command not found"
command -v go >/dev/null 2>&1 || fail "go command not found"

if ip link show "$SMOKE_IFACE" >/dev/null 2>&1 || ip link show "$SMOKE_PEER" >/dev/null 2>&1; then
  fail "smoke interfaces already exist: $SMOKE_IFACE / $SMOKE_PEER"
fi

cd "$GATEWAY_DIR"

if [[ "$RUN_MAKE_BPF" == "1" ]]; then
  echo "== eBPF compile smoke =="
  make bpf
  echo
fi

echo "== create temporary veth pair =="
ip link add "$SMOKE_IFACE" type veth peer name "$SMOKE_PEER"
created_link=1
ip link set dev "$SMOKE_IFACE" up
ip link set dev "$SMOKE_PEER" up
ip -brief link show "$SMOKE_IFACE"

mkdir -p "$TMP_DIR"
cat >"$HELPER" <<'GO'
package main

import (
	"fmt"
	"os"
	"sort"

	"mirage-gateway/pkg/ebpf"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: attach-smoke <iface>")
		return 2
	}

	iface := os.Args[1]
	loader := ebpf.NewLoader(iface)
	if err := loader.LoadAndAttach(); err != nil {
		_ = loader.Close()
		fmt.Fprintf(os.Stderr, "loader attach failed: %v\n", err)
		return 1
	}
	defer loader.Close()

	coreMaps := []string{
		"ctrl_map",
		"npm_config_map",
		"jitter_config_map",
		"quota_map",
		"npm_target_distribution_map",
	}
	bdnaMaps := []string{
		"conn_profile_map",
		"profile_select_map",
		"profile_count_map",
	}

	var missingCore []string
	for _, name := range coreMaps {
		if loader.GetMap(name) == nil {
			missingCore = append(missingCore, name)
		}
	}

	var missingBDNA []string
	for _, name := range bdnaMaps {
		if loader.GetMap(name) == nil {
			missingBDNA = append(missingBDNA, name)
		}
	}

	sort.Strings(missingCore)
	sort.Strings(missingBDNA)

	if len(missingCore) > 0 {
		fmt.Fprintf(os.Stderr, "missing required runtime maps: %v\n", missingCore)
		return 1
	}

	if len(missingBDNA) > 0 {
		if os.Getenv("REQUIRE_BDNA_MAPS") == "1" {
			fmt.Fprintf(os.Stderr, "missing B-DNA runtime maps: %v\n", missingBDNA)
			return 1
		}
		fmt.Printf("WARN: B-DNA maps missing, likely because bdna.o degraded: %v\n", missingBDNA)
	} else {
		fmt.Println("PASS: B-DNA runtime maps are visible")
	}

	fmt.Println("PASS: core runtime maps are visible")
	fmt.Println("PASS: loader attach completed; closing loader for detach cleanup")
	return 0
}
GO

echo
echo "== runtime attach smoke =="
REQUIRE_BDNA_MAPS="$REQUIRE_BDNA_MAPS" go run "$HELPER" "$SMOKE_IFACE"

echo
echo "== post-close link state =="
tc filter show dev "$SMOKE_IFACE" ingress || true
tc filter show dev "$SMOKE_IFACE" egress || true
ip -details link show "$SMOKE_IFACE" | sed -n '1,8p'

echo
echo "PASS: runtime attach smoke completed on $SMOKE_IFACE"
