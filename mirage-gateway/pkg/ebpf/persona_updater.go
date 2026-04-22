// Package ebpf - PersonaMapUpdater 将 Persona 参数批量写入 eBPF Map 双 Slot
package ebpf

import "fmt"

// PersonaParams 收敛后的全部 eBPF Map 参数
type PersonaParams struct {
	DNA    DNATemplateEntry // → dna_template_map
	Jitter JitterConfig     // → jitter_config_map
	VPC    VPCConfig        // → vpc_config_map
	NPM    NPMConfig        // → npm_config_map
}

// PersonaMapUpdater 负责将 Persona 参数批量写入 eBPF Map 双 Slot
type PersonaMapUpdater interface {
	WriteShadow(params *PersonaParams) (slotID uint32, err error)
	VerifyShadow(slotID uint32, expected *PersonaParams) error
	Flip(newActiveSlot uint32) error
	GetActiveSlot() (uint32, error)
}

// EBPFPersonaMapUpdater 基于真实 eBPF Loader 的实现
type EBPFPersonaMapUpdater struct {
	loader *Loader
}

// NewEBPFPersonaMapUpdater 创建基于 Loader 的 PersonaMapUpdater
func NewEBPFPersonaMapUpdater(loader *Loader) *EBPFPersonaMapUpdater {
	return &EBPFPersonaMapUpdater{loader: loader}
}

// GetActiveSlot 从 active_slot_map 读取当前活跃 Slot 编号
func (u *EBPFPersonaMapUpdater) GetActiveSlot() (uint32, error) {
	m := u.loader.GetMap("active_slot_map")
	if m == nil {
		return 0, fmt.Errorf("active_slot_map not found")
	}
	key := uint32(0)
	var value uint32
	if err := m.Lookup(&key, &value); err != nil {
		return 0, fmt.Errorf("read active_slot_map: %w", err)
	}
	return value, nil
}

// WriteShadow 将参数写入当前非活跃 Slot
func (u *EBPFPersonaMapUpdater) WriteShadow(params *PersonaParams) (uint32, error) {
	active, err := u.GetActiveSlot()
	if err != nil {
		return 0, err
	}
	shadow := uint32(1) - active

	// 写入 dna_template_map
	if m := u.loader.GetMap("dna_template_map"); m != nil {
		if err := m.Put(&shadow, &params.DNA); err != nil {
			return 0, &mapWriteError{MapName: "dna_template_map", Err: err}
		}
	} else {
		return 0, &mapWriteError{MapName: "dna_template_map"}
	}

	// 写入 jitter_config_map
	if m := u.loader.GetMap("jitter_config_map"); m != nil {
		if err := m.Put(&shadow, &params.Jitter); err != nil {
			return 0, &mapWriteError{MapName: "jitter_config_map", Err: err}
		}
	} else {
		return 0, &mapWriteError{MapName: "jitter_config_map"}
	}

	// 写入 vpc_config_map
	if m := u.loader.GetMap("vpc_config_map"); m != nil {
		if err := m.Put(&shadow, &params.VPC); err != nil {
			return 0, &mapWriteError{MapName: "vpc_config_map", Err: err}
		}
	} else {
		return 0, &mapWriteError{MapName: "vpc_config_map"}
	}

	// 写入 npm_config_map
	if m := u.loader.GetMap("npm_config_map"); m != nil {
		key := uint32(0)
		if err := m.Put(&key, &params.NPM); err != nil {
			return 0, &mapWriteError{MapName: "npm_config_map", Err: err}
		}
	} else {
		return 0, &mapWriteError{MapName: "npm_config_map"}
	}

	return shadow, nil
}

// VerifyShadow 从 Shadow Slot 回读全部参数并逐字段比对
func (u *EBPFPersonaMapUpdater) VerifyShadow(slotID uint32, expected *PersonaParams) error {
	// 回读 dna_template_map
	if m := u.loader.GetMap("dna_template_map"); m != nil {
		var actual DNATemplateEntry
		if err := m.Lookup(&slotID, &actual); err != nil {
			return &shadowVerifyError{MapName: "dna_template_map", Field: "lookup", Err: err}
		}
		if actual != expected.DNA {
			return &shadowVerifyError{MapName: "dna_template_map", Field: "mismatch"}
		}
	}

	// 回读 jitter_config_map
	if m := u.loader.GetMap("jitter_config_map"); m != nil {
		var actual JitterConfig
		if err := m.Lookup(&slotID, &actual); err != nil {
			return &shadowVerifyError{MapName: "jitter_config_map", Field: "lookup", Err: err}
		}
		if actual != expected.Jitter {
			return &shadowVerifyError{MapName: "jitter_config_map", Field: "mismatch"}
		}
	}

	// 回读 vpc_config_map
	if m := u.loader.GetMap("vpc_config_map"); m != nil {
		var actual VPCConfig
		if err := m.Lookup(&slotID, &actual); err != nil {
			return &shadowVerifyError{MapName: "vpc_config_map", Field: "lookup", Err: err}
		}
		if actual != expected.VPC {
			return &shadowVerifyError{MapName: "vpc_config_map", Field: "mismatch"}
		}
	}

	// 回读 npm_config_map
	if m := u.loader.GetMap("npm_config_map"); m != nil {
		var actual NPMConfig
		key := uint32(0)
		if err := m.Lookup(&key, &actual); err != nil {
			return &shadowVerifyError{MapName: "npm_config_map", Field: "lookup", Err: err}
		}
		if actual != expected.NPM {
			return &shadowVerifyError{MapName: "npm_config_map", Field: "mismatch"}
		}
	}

	return nil
}

// Flip 单次 active_slot_map.Put 完成原子切换
func (u *EBPFPersonaMapUpdater) Flip(newActiveSlot uint32) error {
	m := u.loader.GetMap("active_slot_map")
	if m == nil {
		return fmt.Errorf("active_slot_map not found")
	}
	key := uint32(0)
	if err := m.Put(&key, &newActiveSlot); err != nil {
		return fmt.Errorf("atomic flip failed: %w", err)
	}
	return nil
}

// 内部错误类型
type mapWriteError struct {
	MapName string
	Err     error
}

func (e *mapWriteError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("map write failed: %s: %v", e.MapName, e.Err)
	}
	return fmt.Sprintf("map write failed: %s: map not found", e.MapName)
}

type shadowVerifyError struct {
	MapName string
	Field   string
	Err     error
}

func (e *shadowVerifyError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("shadow verify failed: map=%s, field=%s: %v", e.MapName, e.Field, e.Err)
	}
	return fmt.Sprintf("shadow verify failed: map=%s, field=%s", e.MapName, e.Field)
}
