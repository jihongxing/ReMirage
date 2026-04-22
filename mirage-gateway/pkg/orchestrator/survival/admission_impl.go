package survival

import (
	"sync"

	orchestrator "mirage-gateway/pkg/orchestrator"
)

type sessionAdmissionController struct {
	mu     sync.RWMutex
	policy SessionAdmissionPolicy
}

// NewSessionAdmissionController 创建 SessionAdmissionController
func NewSessionAdmissionController(policy SessionAdmissionPolicy) SessionAdmissionControllerIface {
	return &sessionAdmissionController{policy: policy}
}

func (c *sessionAdmissionController) Check(serviceClass orchestrator.ServiceClass) error {
	c.mu.RLock()
	policy := c.policy
	c.mu.RUnlock()

	allowed := false
	switch policy {
	case AdmissionOpen:
		allowed = true
	case AdmissionRestrictNew:
		allowed = serviceClass == orchestrator.ServiceClassPlatinum || serviceClass == orchestrator.ServiceClassDiamond
	case AdmissionHighPriorityOnly:
		allowed = serviceClass == orchestrator.ServiceClassDiamond
	case AdmissionClosed:
		allowed = false
	}

	if !allowed {
		return &ErrAdmissionDenied{Policy: policy, ServiceClass: serviceClass}
	}
	return nil
}

func (c *sessionAdmissionController) UpdatePolicy(policy SessionAdmissionPolicy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.policy = policy
}

func (c *sessionAdmissionController) GetCurrentPolicy() SessionAdmissionPolicy {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.policy
}
