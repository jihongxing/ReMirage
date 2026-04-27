# eBPF Compile/Load Smoke

This smoke check verifies the Linux eBPF objects at two levels:

- compile smoke: runs `make bpf` for every `mirage-gateway/bpf/*.c` program.
- load smoke: when running as root with `bpftool`, loads every generated object
  into the kernel verifier with `bpftool prog loadall`, then removes the pinned
  smoke objects.

Run from a Linux host or WSL distro with clang/llvm installed:

```bash
cd mirage-gateway
./scripts/smoke-ebpf-load.sh
```

In non-root environments the compile step still runs, and the load step exits
successfully with a clear `SKIP load smoke` message. To require a real load
check in CI or on a Linux lab node:

```bash
cd mirage-gateway
sudo REQUIRE_EBPF_LOAD=1 ./scripts/smoke-ebpf-load.sh
```

Expected result on a fully provisioned Linux node:

```text
PASS: compiled and verifier-loaded N eBPF object(s)
```

If the load step fails, treat it as a verifier/runtime compatibility issue even
when `make bpf` succeeds.
