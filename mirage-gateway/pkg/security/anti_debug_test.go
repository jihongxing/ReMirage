package security

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Property 6: TracerPid 解析正确性
func TestProperty_ParseTracerPid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pid := rapid.IntRange(0, 65535).Draw(t, "pid")
		content := fmt.Sprintf(
			"Name:\ttest\nUmask:\t0022\nState:\tS (sleeping)\nTgid:\t1234\nNgid:\t0\nPid:\t1234\nPPid:\t1\nTracerPid:\t%d\nUid:\t0\t0\t0\t0\n",
			pid,
		)

		parsed, err := ParseTracerPid(content)
		if err != nil {
			t.Fatalf("ParseTracerPid 失败: %v", err)
		}

		if parsed != pid {
			t.Fatalf("解析值不匹配: 期望 %d, 实际 %d", pid, parsed)
		}
	})
}

// 单元测试: 静默模式进入退出
func TestAntiDebug_SilentMode(t *testing.T) {
	ad := NewAntiDebug(30 * time.Second)

	var silentCalled atomic.Int32
	var recoverCalled atomic.Int32

	ad.SetCallbacks(
		func() { silentCalled.Add(1) },
		func() { recoverCalled.Add(1) },
	)

	if ad.IsSilent() {
		t.Fatal("初始不应处于静默模式")
	}

	ad.EnterSilentMode()
	if !ad.IsSilent() {
		t.Fatal("应处于静默模式")
	}
	if silentCalled.Load() != 1 {
		t.Fatal("onSilent 回调未调用")
	}

	ad.ExitSilentMode()
	if ad.IsSilent() {
		t.Fatal("不应处于静默模式")
	}
	if recoverCalled.Load() != 1 {
		t.Fatal("onRecover 回调未调用")
	}
}

// 单元测试: 调试器进程名匹配
func TestParseTracerPid_NoField(t *testing.T) {
	content := "Name:\ttest\nPid:\t1234\n"
	_, err := ParseTracerPid(content)
	if err == nil {
		t.Fatal("应返回错误")
	}
}

// 单元测试: TracerPid 非零检测
func TestParseTracerPid_NonZero(t *testing.T) {
	content := "Name:\ttest\nTracerPid:\t12345\n"
	pid, err := ParseTracerPid(content)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if pid != 12345 {
		t.Fatalf("期望 12345, 实际 %d", pid)
	}
}

// 单元测试: Stop 不 panic
func TestAntiDebug_Stop(t *testing.T) {
	ad := NewAntiDebug(1 * time.Second)
	ad.Stop()
}
