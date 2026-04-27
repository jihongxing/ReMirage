package ebpf

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBPFCompile_KeyCFiles eBPF 编译回归测试：验证 clang -target bpf 可编译关键 .c 文件
// Bug Condition C6: compile_test.go 仅检查 L1Stats 结构体对齐，未编译任何 BPF C 文件
// 修复后：自动化编译测试覆盖关键 .c 文件
//
// 前置条件：
//   - Linux 环境（eBPF 程序依赖 linux/bpf.h 等内核头文件）
//   - clang 已安装
//
// 非 Linux 或无 clang 时自动 SKIP。
func TestBPFCompile_KeyCFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("skipping eBPF compile test on %s (requires Linux kernel headers)", runtime.GOOS)
	}

	clangPath, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("clang not found, skipping eBPF compile test")
	}
	t.Logf("using clang: %s", clangPath)

	bpfDir, err := filepath.Abs(filepath.Join("..", "..", "bpf"))
	if err != nil {
		t.Fatalf("failed to resolve bpf/ directory: %v", err)
	}
	if _, err := os.Stat(bpfDir); os.IsNotExist(err) {
		t.Fatalf("bpf/ directory not found at %s", bpfDir)
	}

	tmpDir := t.TempDir()

	// 关键 .c 文件（必须与 bpf/ 目录实际文件一致）
	keyCFiles := []string{
		"npm.c",
		"bdna.c",
		"jitter.c",
		"l1_defense.c",
		"l1_silent.c",
	}

	for _, cFile := range keyCFiles {
		t.Run(cFile, func(t *testing.T) {
			srcPath := filepath.Join(bpfDir, cFile)
			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				t.Fatalf("source file not found: %s", srcPath)
			}

			oFile := cFile[:len(cFile)-2] + ".o"
			outPath := filepath.Join(tmpDir, oFile)

			cmd := exec.Command(clangPath,
				"-O2",
				"-target", "bpf",
				"-I", bpfDir,
				"-c", srcPath,
				"-o", outPath,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("clang compile failed for %s: %v\noutput:\n%s", cFile, err, string(output))
			}

			info, err := os.Stat(outPath)
			if err != nil {
				t.Fatalf("compiled artifact not found: %s", outPath)
			}
			if info.Size() == 0 {
				t.Fatalf("compiled artifact is empty: %s", outPath)
			}
			t.Logf("%s -> %s (%d bytes)", cFile, oFile, info.Size())
		})
	}
}
