#!/usr/bin/env bash
set -euo pipefail

FAMILY="${PROFILE_FAMILY:-chrome-macos}"
OUT_DIR="${OUT_DIR:-artifacts/dpi-audit/baseline/${FAMILY}}"
SITES="${SITES:-google.com youtube.com cloudflare.com github.com wikipedia.org}"
COUNT="${COUNT:-20}"
IFACE="${IFACE:-en0}"
CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"

mkdir -p "${OUT_DIR}"

if [[ "${FAMILY}" != "chrome-macos" ]]; then
  echo "This macOS runner only captures chrome-macos; got ${FAMILY}" >&2
  exit 2
fi

if ! command -v tcpdump >/dev/null 2>&1; then
  echo "tcpdump is required" >&2
  exit 2
fi

if [[ ! -x "${CHROME}" ]]; then
  echo "Chrome is required at ${CHROME}" >&2
  exit 2
fi

PCAP="${OUT_DIR}/${FAMILY}-$(date -u +%Y%m%dT%H%M%SZ).pcapng"
META="${OUT_DIR}/capture-metadata.json"

tcpdump -i "${IFACE}" -w "${PCAP}" '(tcp port 443 or udp port 443)' &
TCPDUMP_PID=$!
trap 'kill ${TCPDUMP_PID} 2>/dev/null || true' EXIT
sleep 2

for i in $(seq 1 "${COUNT}"); do
  for site in ${SITES}; do
    "${CHROME}" --headless=new --disable-gpu "https://${site}/?remirage_capture=${i}" >/dev/null 2>&1 || true
  done
done

sleep 2
kill "${TCPDUMP_PID}" 2>/dev/null || true
wait "${TCPDUMP_PID}" 2>/dev/null || true
trap - EXIT

cat > "${META}" <<JSON
{
  "profile_family": "${FAMILY}",
  "native_os": true,
  "os": "macos",
  "os_version": "$(sw_vers -productVersion | sed 's/"/\\"/g')",
  "browser": "Google Chrome",
  "browser_version": "$("${CHROME}" --version 2>/dev/null | head -n 1 | sed 's/"/\\"/g')",
  "capture_tool": "tcpdump",
  "interface": "${IFACE}",
  "network_conditions": "uncontrolled",
  "captured_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "pcapng": "$(basename "${PCAP}")"
}
JSON

echo "Captured ${PCAP}"
