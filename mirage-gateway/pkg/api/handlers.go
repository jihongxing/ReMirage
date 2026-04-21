package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/gswitch"
	"mirage-gateway/pkg/threat"
	pb "mirage-proto/gen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// CommandHandler 下行指令处理器
type CommandHandler struct {
	loader        *ebpf.Loader
	blacklist     *threat.BlacklistManager
	gswitch       *gswitch.GSwitchManager
	motorDownlink MotorDownlinkApplier
	auth          *CommandAuthenticator
	audit         *CommandAuditor
	rateLimiter   *CommandRateLimiter
	securityFSM   SecurityFSMForcer
	quotaBuckets  *QuotaBucketManager
	pb.UnimplementedGatewayDownlinkServer
}

// SecurityFSMForcer 安全状态机强制切换接口
type SecurityFSMForcer interface {
	ForceState(state threat.SecurityState)
}

// MotorDownlinkApplier 下行状态映射接口
type MotorDownlinkApplier interface {
	ApplyDesiredState(cfg *DesiredStatePayload) (bool, error)
}

// DesiredStatePayload 期望状态载荷（避免循环依赖）
type DesiredStatePayload struct {
	JitterMeanUs   uint32
	JitterStddevUs uint32
	NoiseIntensity uint32
	PaddingRate    uint32
	TemplateID     uint32
	FiberJitterUs  uint32
	RouterDelayUs  uint32
}

// NewCommandHandler 创建处理器
func NewCommandHandler(
	loader *ebpf.Loader,
	blacklist *threat.BlacklistManager,
	gswitchMgr *gswitch.GSwitchManager,
) *CommandHandler {
	return &CommandHandler{
		loader:    loader,
		blacklist: blacklist,
		gswitch:   gswitchMgr,
	}
}

// SetMotorDownlink 设置下行状态映射器
func (h *CommandHandler) SetMotorDownlink(md MotorDownlinkApplier) {
	h.motorDownlink = md
}

// SetAuth 设置签名校验器
func (h *CommandHandler) SetAuth(auth *CommandAuthenticator) {
	h.auth = auth
}

// SetAudit 设置审计日志记录器
func (h *CommandHandler) SetAudit(audit *CommandAuditor) {
	h.audit = audit
}

// SetRateLimiter 设置速率限制器
func (h *CommandHandler) SetRateLimiter(rl *CommandRateLimiter) {
	h.rateLimiter = rl
}

// SetSecurityFSM 设置安全状态机（用于 OS 强制切换状态）
func (h *CommandHandler) SetSecurityFSM(fsm SecurityFSMForcer) {
	h.securityFSM = fsm
}

// SetQuotaBuckets 设置配额桶管理器
func (h *CommandHandler) SetQuotaBuckets(qb *QuotaBucketManager) {
	h.quotaBuckets = qb
}

// PushStrategy 处理策略下发 → 通过 MotorDownlink 幂等写入 eBPF Map（< 100ms）
func (h *CommandHandler) PushStrategy(ctx context.Context, req *pb.StrategyPush) (*pb.PushResponse, error) {
	src := peerAddr(ctx)
	params := fmt.Sprintf("level=%d", req.DefenseLevel)

	// 高危命令校验：defense_level >= 4
	if req.DefenseLevel >= 4 {
		if h.rateLimiter != nil {
			if err := h.rateLimiter.Check(src); err != nil {
				if h.audit != nil {
					h.audit.Log("PushStrategy", src, params, false, err.Error())
				}
				return nil, status.Errorf(codes.ResourceExhausted, "rate limited")
			}
		}
		if h.auth != nil {
			if err := h.auth.Verify(ctx, "PushStrategy"); err != nil {
				if h.audit != nil {
					h.audit.Log("PushStrategy", src, params, false, err.Error())
				}
				return nil, status.Errorf(codes.PermissionDenied, "auth failed")
			}
		}
	}

	// 审计日志
	if h.audit != nil {
		defer h.audit.Log("PushStrategy", src, params, true, "ok")
	}

	if req.DefenseLevel < 0 || req.DefenseLevel > 5 {
		// TODO: Proto 需要增加 security_state 字段到 StrategyPush
		// 当前约定：defense_level 100-104 映射到安全状态强制切换
		// 100=Normal, 101=Alert, 102=HighPressure, 103=Isolated, 104=Silent
		if req.DefenseLevel >= 100 && req.DefenseLevel <= 104 && h.securityFSM != nil {
			state := threat.SecurityState(req.DefenseLevel - 100)
			h.securityFSM.ForceState(state)
			log.Printf("[Handler] OS 强制切换安全状态: %d", state)
			return &pb.PushResponse{Success: true, Message: "security_state forced"}, nil
		}
		return nil, status.Errorf(codes.InvalidArgument, "defense_level 越界: %d", req.DefenseLevel)
	}

	// 优先使用 MotorDownlink（幂等 Hash 校验）
	if h.motorDownlink != nil {
		applied, err := h.motorDownlink.ApplyDesiredState(&DesiredStatePayload{
			JitterMeanUs:   req.JitterMeanUs,
			JitterStddevUs: req.JitterStddevUs,
			NoiseIntensity: req.NoiseIntensity,
			PaddingRate:    req.PaddingRate,
			TemplateID:     req.TemplateId,
		})
		if err != nil {
			log.Printf("[Handler] PushStrategy MotorDownlink 失败: %v", err)
			return &pb.PushResponse{Success: false, Message: err.Error()}, nil
		}
		if !applied {
			log.Printf("[Handler] 策略未变化（幂等跳过）: level=%d", req.DefenseLevel)
		} else {
			log.Printf("[Handler] 策略已更新（MotorDownlink）: level=%d", req.DefenseLevel)
		}
		return &pb.PushResponse{Success: true, Message: "ok"}, nil
	}

	// Fallback: 直接写入 eBPF Map
	strat := &ebpf.DefenseStrategy{
		JitterMeanUs:   req.JitterMeanUs,
		JitterStddevUs: req.JitterStddevUs,
		NoiseIntensity: req.NoiseIntensity,
		TemplateID:     req.TemplateId,
	}

	if err := h.loader.UpdateStrategy(strat); err != nil {
		log.Printf("[Handler] PushStrategy 写入 eBPF 失败: %v", err)
		return &pb.PushResponse{Success: false, Message: err.Error()}, nil
	}

	log.Printf("[Handler] 策略已更新（直写）: level=%d", req.DefenseLevel)
	return &pb.PushResponse{Success: true, Message: "ok"}, nil
}

// PushBlacklist 处理黑名单下发 → 合并到 BlacklistManager
func (h *CommandHandler) PushBlacklist(ctx context.Context, req *pb.BlacklistPush) (*pb.PushResponse, error) {
	src := peerAddr(ctx)
	params := fmt.Sprintf("entries=%d", len(req.Entries))

	// 审计日志（黑名单下发不做签名校验，已通过 mTLS 认证）
	if h.audit != nil {
		defer h.audit.Log("PushBlacklist", src, params, true, "ok")
	}

	if len(req.Entries) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "entries 不能为空")
	}

	entries := make([]threat.BlacklistEntry, 0, len(req.Entries))
	for _, e := range req.Entries {
		if e.Cidr == "" {
			return nil, status.Errorf(codes.InvalidArgument, "CIDR 不能为空")
		}
		entries = append(entries, threat.BlacklistEntry{
			CIDR:     e.Cidr,
			ExpireAt: time.Unix(e.ExpireAt, 0),
			Source:   threat.SourceGlobal,
		})
	}

	if err := h.blacklist.MergeGlobal(entries); err != nil {
		return &pb.PushResponse{Success: false, Message: err.Error()}, nil
	}

	log.Printf("[Handler] 黑名单已合并: %d 条", len(entries))
	return &pb.PushResponse{Success: true, Message: "ok"}, nil
}

// PushQuota 处理配额下发 → 按 user_id 更新隔离配额桶 + 写入 eBPF quota_map
func (h *CommandHandler) PushQuota(ctx context.Context, req *pb.QuotaPush) (*pb.PushResponse, error) {
	src := peerAddr(ctx)
	params := fmt.Sprintf("remaining=%d,user_id=%s", req.RemainingBytes, req.UserId)

	// 高危命令校验：remaining_bytes == 0（配额清零 = 流量阻断）
	if req.RemainingBytes == 0 {
		if h.rateLimiter != nil {
			if err := h.rateLimiter.Check(src); err != nil {
				if h.audit != nil {
					h.audit.Log("PushQuota", src, params, false, err.Error())
				}
				return nil, status.Errorf(codes.ResourceExhausted, "rate limited")
			}
		}
		if h.auth != nil {
			if err := h.auth.Verify(ctx, "PushQuota"); err != nil {
				if h.audit != nil {
					h.audit.Log("PushQuota", src, params, false, err.Error())
				}
				return nil, status.Errorf(codes.PermissionDenied, "auth failed")
			}
		}
	}

	// 审计日志
	if h.audit != nil {
		defer h.audit.Log("PushQuota", src, params, true, "ok")
	}

	// 按 user_id 更新隔离配额桶
	if h.quotaBuckets != nil {
		userID := req.UserId
		if userID == "" {
			userID = GlobalBucketKey // 兼容旧模式
		}
		h.quotaBuckets.UpdateQuota(userID, req.RemainingBytes)
		log.Printf("[Handler] 配额桶已更新: user=%s, remaining=%d bytes", userID, req.RemainingBytes)
	}

	// 同时写入 eBPF quota_map（保持内核态配额检查兼容）
	if h.loader != nil {
		quotaMap := h.loader.GetMap("quota_map")
		if quotaMap != nil {
			key := uint32(0)
			value := req.RemainingBytes
			if err := quotaMap.Put(&key, &value); err != nil {
				log.Printf("[Handler] eBPF quota_map 写入失败: %v", err)
			}
		}
	}

	if req.RemainingBytes == 0 {
		log.Printf("[Handler] ⚠️ 配额为 0 (user=%s)，流量阻断已触发", req.UserId)
	}

	return &pb.PushResponse{Success: true, Message: "ok"}, nil
}

// PushReincarnation 处理转生指令 → 调用 GSwitch.TriggerEscape
func (h *CommandHandler) PushReincarnation(ctx context.Context, req *pb.ReincarnationPush) (*pb.PushResponse, error) {
	src := peerAddr(ctx)
	params := fmt.Sprintf("domain=%s,ip=%s", req.NewDomain, req.NewIp)

	// 1. 速率限制
	if h.rateLimiter != nil {
		if err := h.rateLimiter.Check(src); err != nil {
			if h.audit != nil {
				h.audit.Log("PushReincarnation", src, params, false, err.Error())
			}
			return nil, status.Errorf(codes.ResourceExhausted, "rate limited")
		}
	}

	// 2. 签名校验
	if h.auth != nil {
		if err := h.auth.Verify(ctx, "PushReincarnation"); err != nil {
			if h.audit != nil {
				h.audit.Log("PushReincarnation", src, params, false, err.Error())
			}
			return nil, status.Errorf(codes.PermissionDenied, "auth failed")
		}
	}

	// 3. 审计日志
	if h.audit != nil {
		defer h.audit.Log("PushReincarnation", src, params, true, "ok")
	}

	if req.NewDomain == "" {
		return nil, status.Errorf(codes.InvalidArgument, "new_domain 不能为空")
	}
	if req.DeadlineSeconds <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "deadline_seconds 必须 > 0")
	}

	reason := req.Reason
	if reason == "" {
		reason = "os_push_reincarnation"
	}

	if err := h.gswitch.TriggerEscape(reason); err != nil {
		return &pb.PushResponse{Success: false, Message: err.Error()}, nil
	}

	log.Printf("[Handler] 转生指令已执行: %s → %s", req.NewDomain, req.NewIp)
	return &pb.PushResponse{Success: true, Message: "ok"}, nil
}

// peerAddr 从 context 中提取对端地址
func peerAddr(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return "unknown"
	}
	return p.Addr.String()
}
