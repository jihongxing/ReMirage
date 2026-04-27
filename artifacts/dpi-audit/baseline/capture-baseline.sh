#!/usr/bin/env bash
set -euo pipefail

FAMILY="${PROFILE_FAMILY:-firefox-linux}"
OUT_DIR="${OUT_DIR:-artifacts/dpi-audit/baseline/${FAMILY}}"
SITES="${SITES:-google.com youtube.com cloudflare.com github.com wikipedia.org}"
COUNT="${COUNT:-20}"
IFACE="${IFACE:-any}"
BROWSER="${BROWSER:-firefox}"

mkdir -p "${OUT_DIR}"

if [[ "${FAMILY}" != "firefox-linux" ]]; then
  echo "This Linux runner only captures firefox-linux; got ${FAMILY}" >&2
  exit 2
fi

if ! command -v tcpdump >/dev/null 2>&1; then
  echo "tcpdump is required" >&2
  exit 2
fi

if ! command -v "${BROWSER}" >/dev/null 2>&1; then
  echo "${BROWSER} is required" >&2
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
    "${BROWSER}" --headless "https://${site}/?remirage_capture=${i}" >/dev/null 2>&1 || true
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
  "os": "linux",
  "os_version": "$(uname -srmo | sed 's/"/\\"/g')",
  "browser": "${BROWSER}",
  "browser_version": "$(${BROWSER} --version 2>/dev/null | head -n 1 | sed 's/"/\\"/g')",
  "capture_tool": "tcpdump",
  "interface": "${IFACE}",
  "network_conditions": "uncontrolled",
  "captured_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "pcapng": "$(basename "${PCAP}")"
}
JSON

echo "Captured ${PCAP}"
