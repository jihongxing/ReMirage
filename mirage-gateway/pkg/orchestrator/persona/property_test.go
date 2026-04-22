package persona

import (
	"encoding/json"
	"fmt"
	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/orchestrator"
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genNonEmptyString 生成非空字符串
func genNonEmptyString() *rapid.Generator[string] {
	return rapid.StringMatching("[a-z0-9]{4,16}")
}

// genMaybeEmptyString 生成可能为空的字符串
func genMaybeEmptyString() *rapid.Generator[string] {
	return rapid.OneOf(rapid.Just(""), genNonEmptyString())
}

// genLifecycle 随机生成 PersonaLifecycle
func genLifecycle() *rapid.Generator[PersonaLifecycle] {
	return rapid.SampledFrom(AllLifecycles)
}

// ============================================
// Property 1: Manifest 完整性校验
// ============================================

func TestProperty1_ManifestIntegrityValidation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		handshake := genMaybeEmptyString().Draw(t, "handshake")
		packetShape := genMaybeEmptyString().Draw(t, "packet_shape")
		timing := genMaybeEmptyString().Draw(t, "timing")
		background := genMaybeEmptyString().Draw(t, "background")

		m := &PersonaManifest{
			HandshakeProfileID:   handshake,
			PacketShapeProfileID: packetShape,
			TimingProfileID:      timing,
			BackgroundProfileID:  background,
		}

		err := ValidateManifest(m)
		allNonEmpty := handshake != "" && packetShape != "" && timing != "" && background != ""

		if allNonEmpty && err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 1: all fields non-empty but got error: %v", err)
		}
		if !allNonEmpty && err == nil {
			t.Fatalf("Feature: v2-persona-engine, Property 1: some fields empty but no error")
		}
	})
}

// ============================================
// Property 2: Checksum 确定性与唯一性
// ============================================

func TestProperty2_ChecksumDeterminismAndUniqueness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		m1 := &PersonaManifest{
			HandshakeProfileID:   genNonEmptyString().Draw(t, "h1"),
			PacketShapeProfileID: genNonEmptyString().Draw(t, "p1"),
			TimingProfileID:      genNonEmptyString().Draw(t, "t1"),
			BackgroundProfileID:  genNonEmptyString().Draw(t, "b1"),
			MTUProfileID:         genMaybeEmptyString().Draw(t, "m1"),
			FECProfileID:         genMaybeEmptyString().Draw(t, "f1"),
		}

		// 确定性：相同输入产生相同 checksum
		c1a := ComputeChecksum(m1)
		c1b := ComputeChecksum(m1)
		if c1a != c1b {
			t.Fatalf("Feature: v2-persona-engine, Property 2: same input produced different checksums: %s vs %s", c1a, c1b)
		}

		// 唯一性：不同输入产生不同 checksum
		m2 := &PersonaManifest{
			HandshakeProfileID:   genNonEmptyString().Draw(t, "h2"),
			PacketShapeProfileID: genNonEmptyString().Draw(t, "p2"),
			TimingProfileID:      genNonEmptyString().Draw(t, "t2"),
			BackgroundProfileID:  genNonEmptyString().Draw(t, "b2"),
			MTUProfileID:         genMaybeEmptyString().Draw(t, "m2"),
			FECProfileID:         genMaybeEmptyString().Draw(t, "f2"),
		}

		sameInput := m1.HandshakeProfileID == m2.HandshakeProfileID &&
			m1.PacketShapeProfileID == m2.PacketShapeProfileID &&
			m1.TimingProfileID == m2.TimingProfileID &&
			m1.BackgroundProfileID == m2.BackgroundProfileID &&
			m1.MTUProfileID == m2.MTUProfileID &&
			m1.FECProfileID == m2.FECProfileID

		c2 := ComputeChecksum(m2)
		if sameInput && c1a != c2 {
			t.Fatalf("Feature: v2-persona-engine, Property 2: same profiles but different checksums")
		}
		if !sameInput && c1a == c2 {
			t.Fatalf("Feature: v2-persona-engine, Property 2: different profiles but same checksum")
		}
	})
}

// ============================================
// Property 5: 生命周期转换合法性
// ============================================

func TestProperty5_LifecycleTransitionValidity(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		from := genLifecycle().Draw(t, "from")
		to := genLifecycle().Draw(t, "to")

		err := TransitionLifecycle(from, to)
		expected := validTransitions[[2]PersonaLifecycle{from, to}]

		if expected && err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 5: transition %s->%s should be valid but got error: %v", from, to, err)
		}
		if !expected && err == nil {
			t.Fatalf("Feature: v2-persona-engine, Property 5: transition %s->%s should be invalid but no error", from, to)
		}
	})
}

// ============================================
// 内存版本存储（测试用）
// ============================================

type memVersionStore struct {
	manifests map[string][]*PersonaManifest // persona_id -> versions
}

func newMemVersionStore() *memVersionStore {
	return &memVersionStore{manifests: make(map[string][]*PersonaManifest)}
}

func (s *memVersionStore) GetMaxVersion(personaID string) (uint64, error) {
	versions := s.manifests[personaID]
	var max uint64
	for _, m := range versions {
		if m.Version > max {
			max = m.Version
		}
	}
	return max, nil
}

func (s *memVersionStore) Save(m *PersonaManifest) error {
	cp := *m
	s.manifests[m.PersonaID] = append(s.manifests[m.PersonaID], &cp)
	return nil
}

type fixedEpochProvider struct {
	epoch uint64
}

func (p *fixedEpochProvider) CurrentEpoch() uint64 { return p.epoch }

// ============================================
// Property 3: 版本严格递增与 Epoch 对齐
// ============================================

func TestProperty3_VersionStrictlyIncreasingAndEpochAligned(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(2, 20).Draw(t, "n")
		store := newMemVersionStore()
		personaID := genNonEmptyString().Draw(t, "persona_id")

		var created []*PersonaManifest
		for i := 0; i < n; i++ {
			epoch := uint64(i + 1)
			ep := &fixedEpochProvider{epoch: epoch}
			m := &PersonaManifest{
				PersonaID:            personaID,
				Version:              uint64(i + 1),
				HandshakeProfileID:   genNonEmptyString().Draw(t, "h"),
				PacketShapeProfileID: genNonEmptyString().Draw(t, "p"),
				TimingProfileID:      genNonEmptyString().Draw(t, "t"),
				BackgroundProfileID:  genNonEmptyString().Draw(t, "b"),
			}
			err := CreateManifestWithVersion(m, store, ep)
			if err != nil {
				t.Fatalf("Feature: v2-persona-engine, Property 3: create %d failed: %v", i, err)
			}
			created = append(created, m)
		}

		// 验证版本严格递增
		for i := 1; i < len(created); i++ {
			if created[i].Version <= created[i-1].Version {
				t.Fatalf("Feature: v2-persona-engine, Property 3: version[%d]=%d not > version[%d]=%d",
					i, created[i].Version, i-1, created[i-1].Version)
			}
		}

		// 验证 epoch 对齐
		for i, m := range created {
			expectedEpoch := uint64(i + 1)
			if m.Epoch != expectedEpoch {
				t.Fatalf("Feature: v2-persona-engine, Property 3: epoch[%d]=%d, want %d", i, m.Epoch, expectedEpoch)
			}
		}
	})
}

// ============================================
// Property 4: 创建后不可变字段
// ============================================

func TestProperty4_ImmutableFieldsAfterCreation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := newMemVersionStore()
		ep := &fixedEpochProvider{epoch: rapid.Uint64Range(1, 1000).Draw(t, "epoch")}

		m := &PersonaManifest{
			PersonaID:            genNonEmptyString().Draw(t, "persona_id"),
			Version:              1,
			HandshakeProfileID:   genNonEmptyString().Draw(t, "h"),
			PacketShapeProfileID: genNonEmptyString().Draw(t, "p"),
			TimingProfileID:      genNonEmptyString().Draw(t, "t"),
			BackgroundProfileID:  genNonEmptyString().Draw(t, "b"),
		}
		if err := CreateManifestWithVersion(m, store, ep); err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 4: create failed: %v", err)
		}

		origVersion := m.Version
		origEpoch := m.Epoch
		origChecksum := m.Checksum

		// 尝试修改 version
		newVersion := origVersion + 1
		err := UpdateManifest(m, &newVersion, nil, nil)
		if err == nil {
			t.Fatalf("Feature: v2-persona-engine, Property 4: modifying version should be rejected")
		}

		// 尝试修改 epoch
		newEpoch := origEpoch + 1
		err = UpdateManifest(m, nil, &newEpoch, nil)
		if err == nil {
			t.Fatalf("Feature: v2-persona-engine, Property 4: modifying epoch should be rejected")
		}

		// 尝试修改 checksum
		newChecksum := "tampered"
		err = UpdateManifest(m, nil, nil, &newChecksum)
		if err == nil {
			t.Fatalf("Feature: v2-persona-engine, Property 4: modifying checksum should be rejected")
		}

		// 验证原始值未变
		if m.Version != origVersion || m.Epoch != origEpoch || m.Checksum != origChecksum {
			t.Fatalf("Feature: v2-persona-engine, Property 4: immutable fields were modified")
		}
	})
}

// ============================================
// Property 8: 原子切换后状态一致性
// ============================================

func TestProperty8_AtomicSwitchStateConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mock := ebpf.NewMockPersonaMapUpdater()
		manifestStore := newMemManifestStore()
		sessionStore := newMemSessionStore()
		controlStore := newMemControlStore(1)

		engine := NewEngine(mock, manifestStore, sessionStore, controlStore)
		sessionID := "session-1"

		newManifest := &PersonaManifest{
			PersonaID:            genNonEmptyString().Draw(t, "persona_id"),
			Version:              1,
			HandshakeProfileID:   genNonEmptyString().Draw(t, "h"),
			PacketShapeProfileID: genNonEmptyString().Draw(t, "p"),
			TimingProfileID:      genNonEmptyString().Draw(t, "t"),
			BackgroundProfileID:  genNonEmptyString().Draw(t, "b"),
			Lifecycle:            LifecyclePrepared,
		}
		newManifest.Checksum = ComputeChecksum(newManifest)
		_ = manifestStore.Save(newManifest)

		params := &ebpf.PersonaParams{
			DNA:    ebpf.DNATemplateEntry{TargetIATMu: rapid.Uint32().Draw(t, "iat")},
			Jitter: ebpf.JitterConfig{Enabled: 1},
			VPC:    ebpf.VPCConfig{Enabled: 1},
			NPM:    ebpf.NewDefaultNPMConfig(50),
		}

		err := engine.SwitchPersona(nil, sessionID, newManifest, params)
		if err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 8: SwitchPersona failed: %v", err)
		}

		// 验证 active_slot_map 指向新 Slot
		activeSlot, _ := mock.GetActiveSlot()
		if activeSlot != 1 { // 初始 active=0，shadow=1
			t.Fatalf("Feature: v2-persona-engine, Property 8: active slot=%d, want 1", activeSlot)
		}

		// 验证 Session.current_persona_id
		currentID, _ := sessionStore.GetCurrentPersonaID(sessionID)
		if currentID != newManifest.PersonaID {
			t.Fatalf("Feature: v2-persona-engine, Property 8: session persona=%s, want %s", currentID, newManifest.PersonaID)
		}

		// 验证 ControlState.persona_version
		if controlStore.GetPersonaVersion() != newManifest.Version {
			t.Fatalf("Feature: v2-persona-engine, Property 8: control version=%d, want %d", controlStore.GetPersonaVersion(), newManifest.Version)
		}
	})
}

// ============================================
// Property 9: 切换失败保持不变
// ============================================

func TestProperty9_SwitchFailurePreservesState(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mock := ebpf.NewMockPersonaMapUpdater()
		manifestStore := newMemManifestStore()
		sessionStore := newMemSessionStore()
		controlStore := newMemControlStore(1)

		engine := NewEngine(mock, manifestStore, sessionStore, controlStore)
		sessionID := "session-1"
		origPersonaID := "orig-persona"
		_ = sessionStore.SetCurrentPersonaID(sessionID, origPersonaID)
		_ = controlStore.SetPersonaVersion(10)

		// 记录切换前状态
		prevSlot, _ := mock.GetActiveSlot()
		prevPersonaID, _ := sessionStore.GetCurrentPersonaID(sessionID)
		prevVersion := controlStore.GetPersonaVersion()

		// 注入错误：随机选择一种失败模式
		failMode := rapid.IntRange(0, 2).Draw(t, "fail_mode")
		switch failMode {
		case 0:
			mock.WriteErr = fmt.Errorf("injected write error")
		case 1:
			mock.VerifyErr = fmt.Errorf("injected verify error")
		case 2:
			mock.FlipErr = fmt.Errorf("injected flip error")
		}

		newManifest := &PersonaManifest{
			PersonaID:            genNonEmptyString().Draw(t, "persona_id"),
			Version:              11,
			HandshakeProfileID:   genNonEmptyString().Draw(t, "h"),
			PacketShapeProfileID: genNonEmptyString().Draw(t, "p"),
			TimingProfileID:      genNonEmptyString().Draw(t, "t"),
			BackgroundProfileID:  genNonEmptyString().Draw(t, "b"),
		}

		params := &ebpf.PersonaParams{}
		err := engine.SwitchPersona(nil, sessionID, newManifest, params)
		if err == nil {
			t.Fatalf("Feature: v2-persona-engine, Property 9: expected error but got nil")
		}

		// 验证状态未变
		currentSlot, _ := mock.GetActiveSlot()
		if currentSlot != prevSlot {
			t.Fatalf("Feature: v2-persona-engine, Property 9: active slot changed from %d to %d", prevSlot, currentSlot)
		}

		currentPersonaID, _ := sessionStore.GetCurrentPersonaID(sessionID)
		if currentPersonaID != prevPersonaID {
			t.Fatalf("Feature: v2-persona-engine, Property 9: persona changed from %s to %s", prevPersonaID, currentPersonaID)
		}

		if controlStore.GetPersonaVersion() != prevVersion {
			t.Fatalf("Feature: v2-persona-engine, Property 9: version changed from %d to %d", prevVersion, controlStore.GetPersonaVersion())
		}
	})
}

// ============================================
// Property 10: 回滚恢复到 Cooling 版本
// ============================================

func TestProperty10_RollbackRestoresCoolingVersion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mock := ebpf.NewMockPersonaMapUpdater()
		manifestStore := newMemManifestStore()
		sessionStore := newMemSessionStore()
		controlStore := newMemControlStore(1)

		engine := NewEngine(mock, manifestStore, sessionStore, controlStore)
		sessionID := "session-1"

		// 创建第一个 Persona 并切换
		persona1 := &PersonaManifest{
			PersonaID:            "persona-1",
			Version:              1,
			HandshakeProfileID:   genNonEmptyString().Draw(t, "h1"),
			PacketShapeProfileID: genNonEmptyString().Draw(t, "p1"),
			TimingProfileID:      genNonEmptyString().Draw(t, "t1"),
			BackgroundProfileID:  genNonEmptyString().Draw(t, "b1"),
			Lifecycle:            LifecyclePrepared,
		}
		persona1.Checksum = ComputeChecksum(persona1)
		_ = manifestStore.Save(persona1)

		params1 := &ebpf.PersonaParams{DNA: ebpf.DNATemplateEntry{TargetIATMu: 100}}
		_ = engine.SwitchPersona(nil, sessionID, persona1, params1)
		manifestStore.SetSessionPersona(sessionID, persona1.PersonaID)

		// 创建第二个 Persona 并切换（persona1 → Cooling）
		persona2 := &PersonaManifest{
			PersonaID:            "persona-2",
			Version:              1,
			HandshakeProfileID:   genNonEmptyString().Draw(t, "h2"),
			PacketShapeProfileID: genNonEmptyString().Draw(t, "p2"),
			TimingProfileID:      genNonEmptyString().Draw(t, "t2"),
			BackgroundProfileID:  genNonEmptyString().Draw(t, "b2"),
			Lifecycle:            LifecyclePrepared,
		}
		persona2.Checksum = ComputeChecksum(persona2)
		_ = manifestStore.Save(persona2)

		params2 := &ebpf.PersonaParams{DNA: ebpf.DNATemplateEntry{TargetIATMu: 200}}
		_ = engine.SwitchPersona(nil, sessionID, persona2, params2)
		manifestStore.SetSessionPersona(sessionID, persona2.PersonaID)

		// 执行回滚
		err := engine.Rollback(nil, sessionID)
		if err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 10: Rollback failed: %v", err)
		}

		// 验证 Cooling → Active
		p1, _ := manifestStore.GetByPersonaIDAndVersion("persona-1", 1)
		if p1.Lifecycle != LifecycleActive {
			t.Fatalf("Feature: v2-persona-engine, Property 10: cooling persona lifecycle=%s, want Active", p1.Lifecycle)
		}

		// 验证原 Active → Retired
		p2, _ := manifestStore.GetByPersonaIDAndVersion("persona-2", 1)
		if p2.Lifecycle != LifecycleRetired {
			t.Fatalf("Feature: v2-persona-engine, Property 10: active persona lifecycle=%s, want Retired", p2.Lifecycle)
		}

		// 验证 Session 指向回滚目标
		currentID, _ := sessionStore.GetCurrentPersonaID(sessionID)
		if currentID != "persona-1" {
			t.Fatalf("Feature: v2-persona-engine, Property 10: session persona=%s, want persona-1", currentID)
		}
	})
}

// ============================================
// Property 6: Session 维度 Active/Cooling 唯一性
// ============================================

func TestProperty6_SessionActiveCoolingUniqueness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mock := ebpf.NewMockPersonaMapUpdater()
		manifestStore := newMemManifestStore()
		sessionStore := newMemSessionStore()
		controlStore := newMemControlStore(1)

		engine := NewEngine(mock, manifestStore, sessionStore, controlStore)
		sessionID := "session-1"

		n := rapid.IntRange(2, 5).Draw(t, "n")
		for i := 0; i < n; i++ {
			m := &PersonaManifest{
				PersonaID:            fmt.Sprintf("persona-%d", i),
				Version:              1,
				HandshakeProfileID:   genNonEmptyString().Draw(t, "h"),
				PacketShapeProfileID: genNonEmptyString().Draw(t, "p"),
				TimingProfileID:      genNonEmptyString().Draw(t, "t"),
				BackgroundProfileID:  genNonEmptyString().Draw(t, "b"),
				Lifecycle:            LifecyclePrepared,
			}
			m.Checksum = ComputeChecksum(m)
			_ = manifestStore.Save(m)

			params := &ebpf.PersonaParams{DNA: ebpf.DNATemplateEntry{TargetIATMu: uint32(i * 100)}}
			_ = engine.SwitchPersona(nil, sessionID, m, params)
			manifestStore.SetSessionPersona(sessionID, m.PersonaID)
		}

		// 统计 Active 和 Cooling 数量
		activeCount := 0
		coolingCount := 0
		for _, versions := range manifestStore.manifests {
			for _, m := range versions {
				if m.Lifecycle == LifecycleActive {
					activeCount++
				}
				if m.Lifecycle == LifecycleCooling {
					coolingCount++
				}
			}
		}

		if activeCount > 1 {
			t.Fatalf("Feature: v2-persona-engine, Property 6: %d Active personas, want <= 1", activeCount)
		}
		if coolingCount > 1 {
			t.Fatalf("Feature: v2-persona-engine, Property 6: %d Cooling personas, want <= 1", coolingCount)
		}
	})
}

// ============================================
// Property 11: 切换互斥
// ============================================

func TestProperty11_SwitchMutualExclusion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mock := ebpf.NewMockPersonaMapUpdater()
		manifestStore := newMemManifestStore()
		sessionStore := newMemSessionStore()
		controlStore := newMemControlStore(1)

		engine := NewEngine(mock, manifestStore, sessionStore, controlStore)
		sessionID := "session-1"

		n := rapid.IntRange(2, 10).Draw(t, "n")
		results := make(chan error, n)
		var wg sync.WaitGroup
		wg.Add(n)
		start := make(chan struct{})

		for i := 0; i < n; i++ {
			go func(idx int) {
				defer wg.Done()
				m := &PersonaManifest{
					PersonaID:            fmt.Sprintf("persona-%d", idx),
					Version:              1,
					HandshakeProfileID:   "h",
					PacketShapeProfileID: "p",
					TimingProfileID:      "t",
					BackgroundProfileID:  "b",
					Lifecycle:            LifecyclePrepared,
				}
				m.Checksum = ComputeChecksum(m)
				_ = manifestStore.Save(m)
				params := &ebpf.PersonaParams{}
				<-start // 等待所有 goroutine 就绪
				results <- engine.SwitchPersona(nil, sessionID, m, params)
			}(i)
		}

		close(start) // 同时释放所有 goroutine
		wg.Wait()
		close(results)

		successCount := 0
		inProgressCount := 0
		otherErrors := 0
		for err := range results {
			if err == nil {
				successCount++
			} else if err == ErrSwitchInProgress {
				inProgressCount++
			} else {
				otherErrors++
			}
		}

		// 至少一个成功，其余要么 ErrSwitchInProgress 要么其他错误
		if successCount < 1 {
			t.Fatalf("Feature: v2-persona-engine, Property 11: %d successes, want >= 1", successCount)
		}
		if successCount+inProgressCount+otherErrors != n {
			t.Fatalf("Feature: v2-persona-engine, Property 11: results don't add up: %d+%d+%d != %d", successCount, inProgressCount, otherErrors, n)
		}
	})
}

// ============================================
// Property 12: 三重约束选择一致性
// ============================================

func TestProperty12_TripleConstraintSelectionConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成候选 Persona
		n := rapid.IntRange(1, 5).Draw(t, "n_candidates")
		var candidates []*PersonaCandidate
		for i := 0; i < n; i++ {
			classes := []orchestrator.ServiceClass{orchestrator.ServiceClassStandard}
			if rapid.Bool().Draw(t, "platinum") {
				classes = append(classes, orchestrator.ServiceClassPlatinum)
			}
			if rapid.Bool().Draw(t, "diamond") {
				classes = append(classes, orchestrator.ServiceClassDiamond)
			}
			candidates = append(candidates, &PersonaCandidate{
				Manifest: &PersonaManifest{
					PersonaID:            fmt.Sprintf("p-%d", i),
					Version:              1,
					HandshakeProfileID:   "h",
					PacketShapeProfileID: "p",
					TimingProfileID:      "t",
					BackgroundProfileID:  "b",
					Lifecycle:            LifecycleActive,
				},
				ServiceClasses:  classes,
				DefenseStrength: rapid.IntRange(0, 100).Draw(t, "defense"),
				ResourceCost:    rapid.IntRange(0, 100).Draw(t, "cost"),
			})
		}

		selector := NewPersonaSelector(candidates)

		constraints := &SelectionConstraints{
			ServiceClass: orchestrator.ServiceClassStandard,
			LinkHealth:   rapid.Float64Range(0, 100).Draw(t, "health"),
			SurvivalMode: rapid.SampledFrom([]orchestrator.SurvivalMode{
				orchestrator.SurvivalModeNormal,
				orchestrator.SurvivalModeHardened,
				orchestrator.SurvivalModeEscape,
				orchestrator.SurvivalModeLastResort,
			}).Draw(t, "mode"),
		}

		result, err := selector.Select(nil, constraints)
		if err != nil {
			// 如果没有匹配，验证确实没有兼容的候选
			for _, c := range candidates {
				if matchServiceClass(c.ServiceClasses, constraints.ServiceClass) {
					t.Fatalf("Feature: v2-persona-engine, Property 12: no match but found compatible candidate %s", c.Manifest.PersonaID)
				}
			}
			return
		}

		// 验证返回的 Manifest 存在于候选中
		found := false
		for _, c := range candidates {
			if c.Manifest.PersonaID == result.PersonaID {
				found = true
				// 验证 ServiceClass 兼容
				if !matchServiceClass(c.ServiceClasses, constraints.ServiceClass) {
					t.Fatalf("Feature: v2-persona-engine, Property 12: selected persona %s not compatible with %s", result.PersonaID, constraints.ServiceClass)
				}
			}
		}
		if !found {
			t.Fatalf("Feature: v2-persona-engine, Property 12: selected persona %s not in candidates", result.PersonaID)
		}
	})
}

// ============================================
// Property 13: Manifest JSON round-trip
// ============================================

func TestProperty13_ManifestJSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		m := &PersonaManifest{
			PersonaID:            genNonEmptyString().Draw(t, "persona_id"),
			Version:              rapid.Uint64Range(1, 10000).Draw(t, "version"),
			Epoch:                rapid.Uint64Range(1, 10000).Draw(t, "epoch"),
			HandshakeProfileID:   genNonEmptyString().Draw(t, "h"),
			PacketShapeProfileID: genNonEmptyString().Draw(t, "p"),
			TimingProfileID:      genNonEmptyString().Draw(t, "t"),
			BackgroundProfileID:  genNonEmptyString().Draw(t, "b"),
			MTUProfileID:         genMaybeEmptyString().Draw(t, "mtu"),
			FECProfileID:         genMaybeEmptyString().Draw(t, "fec"),
			LifecyclePolicyID:    genMaybeEmptyString().Draw(t, "policy"),
			Lifecycle:            genLifecycle().Draw(t, "lifecycle"),
			CreatedAt:            time.Now().UTC().Truncate(time.Second),
		}
		m.Checksum = ComputeChecksum(m)

		// 序列化
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 13: marshal failed: %v", err)
		}

		// 反序列化
		var decoded PersonaManifest
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 13: unmarshal failed: %v", err)
		}

		// 验证等价
		if decoded.PersonaID != m.PersonaID || decoded.Version != m.Version ||
			decoded.Epoch != m.Epoch || decoded.Checksum != m.Checksum ||
			decoded.HandshakeProfileID != m.HandshakeProfileID ||
			decoded.PacketShapeProfileID != m.PacketShapeProfileID ||
			decoded.TimingProfileID != m.TimingProfileID ||
			decoded.BackgroundProfileID != m.BackgroundProfileID ||
			decoded.Lifecycle != m.Lifecycle {
			t.Fatalf("Feature: v2-persona-engine, Property 13: round-trip mismatch")
		}

		// 验证 created_at 符合 RFC 3339
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(data, &raw)
		var createdAtStr string
		_ = json.Unmarshal(raw["created_at"], &createdAtStr)
		_, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			t.Fatalf("Feature: v2-persona-engine, Property 13: created_at not RFC 3339: %s", createdAtStr)
		}
	})
}
