# Phase 5 Stealth Hardening Implementation Status

Updated: 2026-04-28

## Completed And Verified

- M13 degraded real baseline capture:
  - `firefox-linux` native Linux capture completed: `connection_count=9505`, `packet_count=406637`.
  - `chrome-win` native Windows capture completed: `connection_count=102`, `packet_count=2011`.
  - `chrome-macos` is missing because no macOS capture node is currently available.
  - Result remains `M13-degraded`; see `docs/reports/m13-real-baseline-degraded.md`.
- Linux eBPF runtime evidence completed on OpenCloudOS:
  - `REQUIRE_EBPF_LOAD=1 ./scripts/smoke-ebpf-load.sh` verifier-loaded 11 eBPF objects.
  - `./scripts/smoke-ebpf-runtime-attach.sh` passed on a temporary veth interface.
  - `REQUIRE_BDNA_MAPS=1 ./scripts/smoke-ebpf-runtime-attach.sh` passed, confirming B-DNA runtime maps are visible.
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
- M15 degraded feature builder exists:
  - `artifacts/dpi-audit/classifier/build-m15-degraded-features.py`
  - Builds `features-m15-degraded.csv` from completed M13 real baseline families plus current ReMirage reference samples.
  - Writes `m15-degraded-metadata.json` with `upgrade_eligible=false`, missing family status, source evidence, and limitations.
  - Python syntax check passed.
- M15 degraded classifier rerun completed on OpenCloudOS:
  - Input rows: control=120, remirage=120.
  - Control families: `chrome-win`, `firefox-linux`.
  - Missing family: `chrome-macos`.
  - RandomForest C1/C2/C3/C4 all returned `AUC=1.0`, `F1=1.0`, `Accuracy=1.0`.
  - Result: high distinguishability risk remains; no capability upgrade is allowed.
- M15 remediation tooling exists:
  - `artifacts/dpi-audit/classifier/analyze-feature-gap.py` ranks classifier feature gaps by effect size.
  - `artifacts/dpi-audit/classifier/calibrate-remirage-reference.py` creates `simulation-metadata-calibrated.json` for remediation experiments without overwriting the original simulation metadata.
  - These tools are not upgrade evidence; they are for feature calibration and next-iteration diagnosis.
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

## Completed But Requires Continued Evidence

- M14 eBPF data-plane changes are implemented and now load/attach on the OpenCloudOS target:
  - `bdna.c` adds `conn_profile_map`, `profile_select_map`, `profile_count_map`.
  - `bdna.c` routes TCP/TLS/QUIC through `select_profile_for_conn`.
  - `bdna.c` prevents invalid profile IDs from being written to `conn_profile_map`.
  - `npm.c` adds `NPM_MODE_MIMIC` and `npm_target_distribution_map`.
- Runtime map wiring is verified for:
  - `conn_profile_map`
  - `profile_select_map`
  - `profile_count_map`
  - `npm_target_distribution_map`
- Remaining evidence needed: real ReMirage-side pcap-derived sample extraction and another classifier iteration after feature calibration.

## Not Completed

- M13-full real baseline is not available yet:
  - `firefox-linux` and `chrome-win` are complete.
  - `chrome-macos` is missing.
  - `verify-m13-full.py` reports `M13-degraded`.
- M15-full classifier rerun has not been completed because M13-full is unavailable.
- M15 TLS/QUIC/WebSocket fingerprint audit has not been completed.
- AUC/F1/Accuracy targets are not met in degraded rerun; all four RandomForest AUC values are 1.0.
- Capability status must remain "部分实现" until M13-full plus M15 AUC gates pass.
- Per-family NPM CDF and per-family Jitter IAT calibration remain out of scope for this spec and are not implemented.

## Verification Commands Run

```powershell
go test ./pkg/ebpf
go test ./...
python -m py_compile artifacts\dpi-audit\baseline\extract-baseline-stats.py artifacts\dpi-audit\baseline\verify-m13-full.py
python -m py_compile artifacts\dpi-audit\classifier\build-m15-degraded-features.py
python artifacts\dpi-audit\classifier\build-m15-degraded-features.py --baseline-root artifacts\dpi-audit\baseline --simulation-metadata artifacts\dpi-audit\simulation-metadata.json --output artifacts\dpi-audit\classifier\features-m15-degraded.csv --metadata-output artifacts\dpi-audit\classifier\m15-degraded-metadata.json
python artifacts\dpi-audit\classifier\train-classifier.py -i artifacts\dpi-audit\classifier\features-m15-degraded.csv -o artifacts\dpi-audit\classifier\results-m15-degraded.json
python artifacts\dpi-audit\classifier\analyze-feature-gap.py --input artifacts\dpi-audit\classifier\features-m15-degraded.csv --output artifacts\dpi-audit\classifier\feature-gap-m15-degraded.csv --json-output artifacts\dpi-audit\classifier\feature-gap-m15-degraded.json
python artifacts\dpi-audit\baseline\verify-m13-full.py
$errors=$null; [System.Management.Automation.PSParser]::Tokenize((Get-Content artifacts\dpi-audit\baseline\capture-baseline.ps1 -Raw), [ref]$errors)
bash -n artifacts/dpi-audit/baseline/capture-baseline.sh; bash -n artifacts/dpi-audit/baseline/capture-baseline-macos.sh
bash -lc "cd /mnt/d/codeSpace/ReMirage/mirage-gateway && make bpf"
bash -lc "cd /mnt/d/codeSpace/ReMirage/mirage-gateway && bash -n scripts/smoke-ebpf-load.sh"
bash -lc "cd /mnt/d/codeSpace/ReMirage/mirage-gateway && rm -f bpf/*.o && ./scripts/smoke-ebpf-load.sh"
```

`go test ./...` passed in `mirage-gateway`. Target OpenCloudOS eBPF verifier load and runtime attach smoke passed after reducing B-DNA verifier complexity. `verify-m13-full.py` correctly fails closed as `M13-degraded` because `chrome-macos` is not present. M15 degraded classifier rerun completed, but all four RandomForest AUC values remain 1.0, so the stealth capability remains `部分实现`.
