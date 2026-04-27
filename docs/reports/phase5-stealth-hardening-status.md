# Phase 5 Stealth Hardening Implementation Status

Updated: 2026-04-28

## Completed And Verified

- M13 runner scaffolding exists:
  - `artifacts/dpi-audit/baseline/capture-baseline.sh`
  - `artifacts/dpi-audit/baseline/capture-baseline-macos.sh`
  - `artifacts/dpi-audit/baseline/capture-baseline.ps1`
- M13 extraction and gate tooling exists:
  - `artifacts/dpi-audit/baseline/extract-baseline-stats.py`
  - `artifacts/dpi-audit/baseline/verify-m13-full.py`
  - Python syntax check passed.
  - PowerShell runner syntax check passed.
  - Bash runner syntax check passed.
- M14 Go control-plane support exists and is verified by `go test ./...` in `mirage-gateway`:
  - `ConnKey` includes `l4_proto`.
  - B-DNA profile selector entries support sparse profile IDs.
  - `OverrideConnectionProfile(connKey ConnKey, profileID uint32)` accepts full `ConnKey`.
  - NPM merged baseline distribution loader exists.
  - Jitter merged baseline IAT loader exists.
  - Property tests cover TCP/UDP key isolation and NPM MIMIC invariants.
- M14 eBPF source compile smoke:
  - Full `make bpf` completed under WSL clang for all `mirage-gateway/bpf/*.c` programs.
  - `scripts/smoke-ebpf-load.sh` completed the compile smoke path.
  - Load smoke skipped locally because `bpftool` is not installed.

## Completed But Not Verified In Target Environment

- M14 eBPF data-plane changes are implemented in source:
  - `bdna.c` adds `conn_profile_map`, `profile_select_map`, `profile_count_map`.
  - `bdna.c` routes TCP/TLS/QUIC through `select_profile_for_conn`.
  - `bdna.c` prevents invalid profile IDs from being written to `conn_profile_map`.
  - `npm.c` adds `NPM_MODE_MIMIC` and `npm_target_distribution_map`.
- These eBPF changes were not loaded or attached in a Linux kernel environment during this pass.
- eBPF verifier load is not verified locally because `bpftool` is not installed.
- Real capture runners are implemented but have not been executed on native Windows/macOS/Linux nodes.

## Not Completed

- M13-full real baseline is not available yet:
  - No `chrome-win`, `chrome-macos`, or `firefox-linux` pcapng captures are present.
  - No per-family `capture-metadata.json`, `baseline-stats.csv`, or `baseline-distribution.json` evidence is present.
  - `verify-m13-full.py` currently reports `M13-degraded`.
- M15 classifier rerun with real baseline has not been completed.
- M15 TLS/QUIC/WebSocket fingerprint audit has not been completed.
- AUC/F1/Accuracy targets have not been verified.
- Capability status must remain "部分实现" until M13-full plus M15 AUC gates pass.
- Linux eBPF attach/runtime behavior has not been verified.
- Per-family NPM CDF and per-family Jitter IAT calibration remain out of scope for this spec and are not implemented.

## Verification Commands Run

```powershell
go test ./pkg/ebpf
go test ./...
python -m py_compile artifacts\dpi-audit\baseline\extract-baseline-stats.py artifacts\dpi-audit\baseline\verify-m13-full.py
python artifacts\dpi-audit\baseline\verify-m13-full.py
$errors=$null; [System.Management.Automation.PSParser]::Tokenize((Get-Content artifacts\dpi-audit\baseline\capture-baseline.ps1 -Raw), [ref]$errors)
bash -n artifacts/dpi-audit/baseline/capture-baseline.sh; bash -n artifacts/dpi-audit/baseline/capture-baseline-macos.sh
bash -lc "cd /mnt/d/codeSpace/ReMirage/mirage-gateway && make bpf"
bash -lc "cd /mnt/d/codeSpace/ReMirage/mirage-gateway && bash -n scripts/smoke-ebpf-load.sh"
bash -lc "cd /mnt/d/codeSpace/ReMirage/mirage-gateway && rm -f bpf/*.o && ./scripts/smoke-ebpf-load.sh"
```

`go test ./...` passed in `mirage-gateway`. `verify-m13-full.py` correctly failed closed as `M13-degraded` because real capture evidence is not present. `smoke-ebpf-load.sh` compiled all BPF objects and skipped load with `bpftool not found` in the local WSL environment.
