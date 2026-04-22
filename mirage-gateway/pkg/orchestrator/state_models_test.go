package orchestrator

import "testing"

func TestLinkPhaseStringValues(t *testing.T) {
	tests := []struct {
		phase LinkPhase
		want  string
	}{
		{LinkPhaseProbing, "Probing"},
		{LinkPhaseActive, "Active"},
		{LinkPhaseDegrading, "Degrading"},
		{LinkPhaseStandby, "Standby"},
		{LinkPhaseUnavailable, "Unavailable"},
	}
	for _, tt := range tests {
		if string(tt.phase) != tt.want {
			t.Errorf("LinkPhase = %q, want %q", tt.phase, tt.want)
		}
	}
}

func TestSessionPhaseStringValues(t *testing.T) {
	tests := []struct {
		phase SessionPhase
		want  string
	}{
		{SessionPhaseBootstrapping, "Bootstrapping"},
		{SessionPhaseActive, "Active"},
		{SessionPhaseProtected, "Protected"},
		{SessionPhaseMigrating, "Migrating"},
		{SessionPhaseDegraded, "Degraded"},
		{SessionPhaseSuspended, "Suspended"},
		{SessionPhaseClosed, "Closed"},
	}
	for _, tt := range tests {
		if string(tt.phase) != tt.want {
			t.Errorf("SessionPhase = %q, want %q", tt.phase, tt.want)
		}
	}
}

func TestControlHealthStringValues(t *testing.T) {
	tests := []struct {
		h    ControlHealth
		want string
	}{
		{ControlHealthHealthy, "Healthy"},
		{ControlHealthRecovering, "Recovering"},
		{ControlHealthFaulted, "Faulted"},
	}
	for _, tt := range tests {
		if string(tt.h) != tt.want {
			t.Errorf("ControlHealth = %q, want %q", tt.h, tt.want)
		}
	}
}

func TestLinkTransitionsCompleteness(t *testing.T) {
	// 合法转换
	legal := [][2]LinkPhase{
		{LinkPhaseProbing, LinkPhaseActive},
		{LinkPhaseProbing, LinkPhaseUnavailable},
		{LinkPhaseActive, LinkPhaseDegrading},
		{LinkPhaseActive, LinkPhaseStandby},
		{LinkPhaseDegrading, LinkPhaseActive},
		{LinkPhaseDegrading, LinkPhaseStandby},
		{LinkPhaseDegrading, LinkPhaseUnavailable},
		{LinkPhaseStandby, LinkPhaseProbing},
		{LinkPhaseStandby, LinkPhaseUnavailable},
		{LinkPhaseUnavailable, LinkPhaseProbing},
	}
	for _, pair := range legal {
		if !IsValidLinkTransition(pair[0], pair[1]) {
			t.Errorf("expected valid transition %s -> %s", pair[0], pair[1])
		}
	}

	// 非法转换样本
	illegal := [][2]LinkPhase{
		{LinkPhaseProbing, LinkPhaseStandby},
		{LinkPhaseActive, LinkPhaseProbing},
		{LinkPhaseUnavailable, LinkPhaseActive},
		{LinkPhaseStandby, LinkPhaseActive},
		{LinkPhaseStandby, LinkPhaseDegrading},
	}
	for _, pair := range illegal {
		if IsValidLinkTransition(pair[0], pair[1]) {
			t.Errorf("expected invalid transition %s -> %s", pair[0], pair[1])
		}
	}
}

func TestSessionTransitionsCompleteness(t *testing.T) {
	legal := [][2]SessionPhase{
		{SessionPhaseBootstrapping, SessionPhaseActive},
		{SessionPhaseBootstrapping, SessionPhaseClosed},
		{SessionPhaseActive, SessionPhaseProtected},
		{SessionPhaseActive, SessionPhaseMigrating},
		{SessionPhaseActive, SessionPhaseDegraded},
		{SessionPhaseActive, SessionPhaseSuspended},
		{SessionPhaseActive, SessionPhaseClosed},
		{SessionPhaseProtected, SessionPhaseActive},
		{SessionPhaseProtected, SessionPhaseMigrating},
		{SessionPhaseProtected, SessionPhaseDegraded},
		{SessionPhaseMigrating, SessionPhaseActive},
		{SessionPhaseMigrating, SessionPhaseDegraded},
		{SessionPhaseMigrating, SessionPhaseClosed},
		{SessionPhaseDegraded, SessionPhaseActive},
		{SessionPhaseDegraded, SessionPhaseSuspended},
		{SessionPhaseDegraded, SessionPhaseClosed},
		{SessionPhaseSuspended, SessionPhaseActive},
		{SessionPhaseSuspended, SessionPhaseClosed},
	}
	for _, pair := range legal {
		if !IsValidSessionTransition(pair[0], pair[1]) {
			t.Errorf("expected valid transition %s -> %s", pair[0], pair[1])
		}
	}

	illegal := [][2]SessionPhase{
		{SessionPhaseBootstrapping, SessionPhaseMigrating},
		{SessionPhaseClosed, SessionPhaseActive},
		{SessionPhaseSuspended, SessionPhaseDegraded},
		{SessionPhaseProtected, SessionPhaseSuspended},
	}
	for _, pair := range illegal {
		if IsValidSessionTransition(pair[0], pair[1]) {
			t.Errorf("expected invalid transition %s -> %s", pair[0], pair[1])
		}
	}
}

func TestLinkStateDefaults(t *testing.T) {
	ls := LinkState{}
	if ls.HealthScore != 0 {
		t.Errorf("default HealthScore = %f, want 0", ls.HealthScore)
	}
	if ls.Available {
		t.Error("default Available should be false")
	}
	if ls.Degraded {
		t.Error("default Degraded should be false")
	}
}

func TestSessionStateDefaults(t *testing.T) {
	ss := SessionState{}
	if ss.Priority != 0 {
		t.Errorf("default Priority = %d, want 0 (DB default is 50)", ss.Priority)
	}
	if ss.MigrationPending {
		t.Error("default MigrationPending should be false")
	}
}

func TestControlStateDefaults(t *testing.T) {
	cs := ControlState{}
	if cs.Epoch != 0 {
		t.Errorf("default Epoch = %d, want 0", cs.Epoch)
	}
	if cs.ActiveTxID != "" {
		t.Errorf("default ActiveTxID = %q, want empty", cs.ActiveTxID)
	}
}
