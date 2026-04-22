// Package persona - 版本管理逻辑
package persona

// VersionStore 版本存储接口（用于版本管理逻辑的抽象）
type VersionStore interface {
	// GetMaxVersion 获取指定 persona_id 下的最大版本号，不存在返回 0
	GetMaxVersion(personaID string) (uint64, error)
	// Save 持久化 Manifest
	Save(m *PersonaManifest) error
}

// EpochProvider 提供当前 Epoch 值
type EpochProvider interface {
	CurrentEpoch() uint64
}

// CreateManifestWithVersion 创建 Manifest 并执行版本管理逻辑
// - version 严格大于同一 persona_id 下已存在的最大 version
// - epoch 设置为当前 ControlState 的 Epoch 值
// - 创建后 version、epoch、checksum 不可变
func CreateManifestWithVersion(m *PersonaManifest, store VersionStore, ep EpochProvider) error {
	if err := ValidateManifest(m); err != nil {
		return err
	}

	maxVer, err := store.GetMaxVersion(m.PersonaID)
	if err != nil {
		return err
	}

	if m.Version <= maxVer {
		return &ErrVersionConflict{
			PersonaID:   m.PersonaID,
			ExistingMax: maxVer,
			Attempted:   m.Version,
		}
	}

	m.Epoch = ep.CurrentEpoch()
	m.Checksum = ComputeChecksum(m)
	m.Lifecycle = LifecyclePrepared

	return store.Save(m)
}

// UpdateManifest 更新 Manifest，拒绝修改不可变字段
func UpdateManifest(existing *PersonaManifest, version *uint64, epoch *uint64, checksum *string) error {
	if version != nil && *version != existing.Version {
		return &ErrImmutableField{FieldName: "version"}
	}
	if epoch != nil && *epoch != existing.Epoch {
		return &ErrImmutableField{FieldName: "epoch"}
	}
	if checksum != nil && *checksum != existing.Checksum {
		return &ErrImmutableField{FieldName: "checksum"}
	}
	return nil
}
