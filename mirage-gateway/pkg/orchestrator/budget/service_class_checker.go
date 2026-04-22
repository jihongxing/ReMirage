package budget

import (
	"context"

	"mirage-gateway/pkg/orchestrator"
	"mirage-gateway/pkg/orchestrator/commit"
)

// SessionGetter 获取 Session 信息的接口
type SessionGetter interface {
	Get(ctx context.Context, sessionID string) (*orchestrator.SessionState, error)
}

// ServiceClassCheckerImpl 实现 commit.ServiceClassChecker 接口
type ServiceClassCheckerImpl struct {
	slaPolicy     ExternalSLAPolicy
	sessionGetter SessionGetter
}

// 编译期接口检查
var _ commit.ServiceClassChecker = (*ServiceClassCheckerImpl)(nil)

// NewServiceClassCheckerImpl 创建 ServiceClassCheckerImpl 实例
func NewServiceClassCheckerImpl(slaPolicy ExternalSLAPolicy, sessionGetter SessionGetter) *ServiceClassCheckerImpl {
	return &ServiceClassCheckerImpl{
		slaPolicy:     slaPolicy,
		sessionGetter: sessionGetter,
	}
}

// Check 执行服务等级校验
func (sc *ServiceClassCheckerImpl) Check(ctx context.Context, tx *commit.CommitTransaction) error {
	// 非 SurvivalModeSwitch 事务始终通过
	if tx.TxType != commit.TxTypeSurvivalModeSwitch {
		return nil
	}

	// 获取 session 信息
	session, err := sc.sessionGetter.Get(ctx, tx.TargetSessionID)
	if err != nil {
		return err
	}

	// 获取 SLA 策略
	policy := sc.slaPolicy.GetPolicy(session.ServiceClass)

	// 校验目标 survival mode 权限
	var allowed bool
	switch tx.TargetSurvivalMode {
	case string(orchestrator.SurvivalModeHardened):
		allowed = policy.HardenedAllowed
	case string(orchestrator.SurvivalModeEscape):
		allowed = policy.EscapeAllowed
	case string(orchestrator.SurvivalModeLastResort):
		allowed = policy.LastResortAllowed
	default:
		// Normal, LowNoise, Degraded 始终允许
		allowed = true
	}

	if !allowed {
		return &ErrServiceClassDenied{
			ServiceClass: string(session.ServiceClass),
			DeniedMode:   tx.TargetSurvivalMode,
		}
	}

	return nil
}
