package survival

import orchestrator "mirage-gateway/pkg/orchestrator"

// SessionAdmissionControllerIface 会话准入控制器接口
type SessionAdmissionControllerIface interface {
	Check(serviceClass orchestrator.ServiceClass) error
	UpdatePolicy(policy SessionAdmissionPolicy)
	GetCurrentPolicy() SessionAdmissionPolicy
}
