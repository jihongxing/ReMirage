// Package ebpf - PersonaMapUpdater Mock 实现（测试用）
package ebpf

import "fmt"

// MockPersonaMapUpdater 内存模拟双 Slot 存储
type MockPersonaMapUpdater struct {
	slots      [2]*PersonaParams
	activeSlot uint32

	// 错误注入
	WriteErr  error // 注入写入错误
	VerifyErr error // 注入校验错误
	FlipErr   error // 注入 Flip 错误
}

// NewMockPersonaMapUpdater 创建 Mock
func NewMockPersonaMapUpdater() *MockPersonaMapUpdater {
	return &MockPersonaMapUpdater{
		slots:      [2]*PersonaParams{{}, {}},
		activeSlot: 0,
	}
}

// GetActiveSlot 读取当前活跃 Slot
func (m *MockPersonaMapUpdater) GetActiveSlot() (uint32, error) {
	return m.activeSlot, nil
}

// WriteShadow 写入 Shadow Slot
func (m *MockPersonaMapUpdater) WriteShadow(params *PersonaParams) (uint32, error) {
	if m.WriteErr != nil {
		return 0, m.WriteErr
	}
	shadow := uint32(1) - m.activeSlot
	cp := *params
	m.slots[shadow] = &cp
	return shadow, nil
}

// VerifyShadow 回读校验
func (m *MockPersonaMapUpdater) VerifyShadow(slotID uint32, expected *PersonaParams) error {
	if m.VerifyErr != nil {
		return m.VerifyErr
	}
	if slotID > 1 {
		return fmt.Errorf("invalid slot: %d", slotID)
	}
	actual := m.slots[slotID]
	if actual == nil {
		return fmt.Errorf("slot %d is empty", slotID)
	}
	if actual.DNA != expected.DNA {
		return &shadowVerifyError{MapName: "dna_template_map", Field: "mismatch"}
	}
	if actual.Jitter != expected.Jitter {
		return &shadowVerifyError{MapName: "jitter_config_map", Field: "mismatch"}
	}
	if actual.VPC != expected.VPC {
		return &shadowVerifyError{MapName: "vpc_config_map", Field: "mismatch"}
	}
	if actual.NPM != expected.NPM {
		return &shadowVerifyError{MapName: "npm_config_map", Field: "mismatch"}
	}
	return nil
}

// Flip 原子切换
func (m *MockPersonaMapUpdater) Flip(newActiveSlot uint32) error {
	if m.FlipErr != nil {
		return m.FlipErr
	}
	m.activeSlot = newActiveSlot
	return nil
}

// GetSlotParams 获取指定 Slot 的参数（测试辅助）
func (m *MockPersonaMapUpdater) GetSlotParams(slot uint32) *PersonaParams {
	if slot > 1 {
		return nil
	}
	return m.slots[slot]
}
