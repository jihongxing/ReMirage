package survival

import (
	"errors"
	"testing"

	orchestrator "mirage-gateway/pkg/orchestrator"

	"pgregory.net/rapid"
)

var allServiceClasses = []orchestrator.ServiceClass{
	orchestrator.ServiceClassStandard,
	orchestrator.ServiceClassPlatinum,
	orchestrator.ServiceClassDiamond,
}

// admissionMatrix 准入矩阵
var admissionMatrix = map[SessionAdmissionPolicy]map[orchestrator.ServiceClass]bool{
	AdmissionOpen:             {orchestrator.ServiceClassStandard: true, orchestrator.ServiceClassPlatinum: true, orchestrator.ServiceClassDiamond: true},
	AdmissionRestrictNew:      {orchestrator.ServiceClassStandard: false, orchestrator.ServiceClassPlatinum: true, orchestrator.ServiceClassDiamond: true},
	AdmissionHighPriorityOnly: {orchestrator.ServiceClassStandard: false, orchestrator.ServiceClassPlatinum: false, orchestrator.ServiceClassDiamond: true},
	AdmissionClosed:           {orchestrator.ServiceClassStandard: false, orchestrator.ServiceClassPlatinum: false, orchestrator.ServiceClassDiamond: false},
}

// Property 11: Session 准入矩阵正确性
func TestProperty11_SessionAdmissionMatrixCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		policy := AllSessionAdmissionPolicies[rapid.IntRange(0, len(AllSessionAdmissionPolicies)-1).Draw(t, "policy")]
		sc := allServiceClasses[rapid.IntRange(0, len(allServiceClasses)-1).Draw(t, "service_class")]

		controller := NewSessionAdmissionController(policy)
		err := controller.Check(sc)

		expectedAllowed := admissionMatrix[policy][sc]

		if expectedAllowed {
			if err != nil {
				t.Fatalf("policy=%s sc=%s: expected allowed, got error: %v", policy, sc, err)
			}
		} else {
			if err == nil {
				t.Fatalf("policy=%s sc=%s: expected denied, got nil", policy, sc)
			}
			var denied *ErrAdmissionDenied
			if !errors.As(err, &denied) {
				t.Fatalf("expected ErrAdmissionDenied, got %T", err)
			}
			if denied.Policy != policy {
				t.Fatalf("expected policy %s, got %s", policy, denied.Policy)
			}
			if denied.ServiceClass != sc {
				t.Fatalf("expected service_class %s, got %s", sc, denied.ServiceClass)
			}
		}
	})
}
