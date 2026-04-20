package api

import (
	"context"
	"log"
	"time"

	pb "mirage-proto/gen"
	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/gswitch"
	"mirage-gateway/pkg/threat"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CommandHandler 下行指令处理器
type CommandHandler struct {
	loader        *ebpf.Loader
	blacklist     *threat.BlacklistManager
	gswitch       *gswitch.GSwitchManager
	motorDownlink MotorDownlinkApplier
	pb.UnimplementedGatewayDownlinkServer
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

// PushStrategy 处理策略下发 → 通过 MotorDownlink 幂等写入 eBPF Map（< 100ms）
func (h *CommandHandler) PushStrategy(ctx context.Context, req *pb.StrategyPush) (*pb.PushResponse, error) {
	if req.DefenseLevel < 0 || req.DefenseLevel > 5 {
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

// PushQuota 处理配额下发 → 写入 eBPF quota_map
func (h *CommandHandler) PushQuota(ctx context.Context, req *pb.QuotaPush) (*pb.PushResponse, error) {
	quotaMap := h.loader.GetMap("quota_map")
	if quotaMap == nil {
		return &pb.PushResponse{Success: false, Message: "quota_map 不存在"}, nil
	}

	key := uint32(0)
	value := req.RemainingBytes
	if err := quotaMap.Put(&key, &value); err != nil {
		return &pb.PushResponse{Success: false, Message: err.Error()}, nil
	}

	if value == 0 {
		log.Println("[Handler] ⚠️ 配额为 0，内核态流量阻断已触发")
	}

	log.Printf("[Handler] 配额已更新: %d bytes", value)
	return &pb.PushResponse{Success: true, Message: "ok"}, nil
}

// PushReincarnation 处理转生指令 → 调用 GSwitch.TriggerEscape
func (h *CommandHandler) PushReincarnation(ctx context.Context, req *pb.ReincarnationPush) (*pb.PushResponse, error) {
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
