#!/usr/bin/env bash
# Capture real ReMirage-side traffic on a Linux Gateway node.
#
# This script only captures. It does not generate client traffic.
# Run the Phantom/ReMirage client from another machine while this script is active.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

PROFILE_FAMILY="${PROFILE_FAMILY:-remirage-real}"
OUT_DIR="${OUT_DIR:-$PROJECT_ROOT/artifacts/dpi-audit/remirage/$PROFILE_FAMILY}"
IFACE="${IFACE:-any}"
DURATION_SECONDS="${DURATION_SECONDS:-180}"
PORTS="${PORTS:-8443 50847}"
SERVER_IP="${SERVER_IP:-}"
TRAFFIC_NOTE="${TRAFFIC_NOTE:-manual client traffic required}"

if ! command -v tcpdump >/dev/null 2>&1; then
  echo "FAIL: tcpdump is required" >&2
  exit 2
fi

if [[ ${EUID:-1} -ne 0 ]]; then
  echo "FAIL: run as root or with CAP_NET_RAW/CAP_NET_ADMIN" >&2
  exit 2
fi

mkdir -p "$OUT_DIR"
ts="$(date -u '+%Y%m%dT%H%M%SZ')"
pcap="$OUT_DIR/${PROFILE_FAMILY}-${ts}.pcapng"
metadata="$OUT_DIR/capture-metadata.json"

filter=""
for port in $PORTS; do
  if [[ -z "$filter" ]]; then
    filter="tcp port $port"
  else
    filter="$filter or tcp port $port"
  fi
done
filter="($filter)"

echo "== ReMirage real-side capture =="
echo "interface: $IFACE"
echo "duration: ${DURATION_SECONDS}s"
echo "ports: $PORTS"
echo "output: $pcap"
echo
echo "Now generate real client traffic from another machine against this Gateway."
echo "For this deployment, target TCP/WSS surfaces are expected on 119.28.50.29:8443 and/or :50847."
echo

set +e
timeout "$DURATION_SECONDS" tcpdump -i "$IFACE" -w "$pcap" "$filter"
status=$?
set -e
if [[ "$status" -ne 0 && "$status" -ne 124 ]]; then
  echo "FAIL: tcpdump exited with status $status" >&2
  exit "$status"
fi

if [[ ! -s "$pcap" ]]; then
  echo "FAIL: pcap was not created or is empty: $pcap" >&2
  exit 1
fi

python3 - "$metadata" "$PROFILE_FAMILY" "$IFACE" "$PORTS" "$SERVER_IP" "$TRAFFIC_NOTE" "$(basename "$pcap")" <<'PY'
import json
import platform
import sys
from datetime import datetime, timezone

path, family, iface, ports, server_ip, note, pcap = sys.argv[1:]
payload = {
    "profile_family": family,
    "native_os": True,
    "os": "linux",
    "os_version": platform.platform(),
    "capture_tool": "tcpdump",
    "interface": iface,
    "ports": ports.split(),
    "server_ip": server_ip,
    "network_conditions": "uncontrolled",
    "traffic_note": note,
    "captured_at": datetime.now(timezone.utc).isoformat(),
    "pcapng": pcap,
    "evidence_strength": "real ReMirage-side pcap on Gateway node",
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
PY

echo "Captured $pcap"
echo "Metadata $metadata"
