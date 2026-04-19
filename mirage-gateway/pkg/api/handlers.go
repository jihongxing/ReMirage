package api

import (
	"context"
	"log"
	"time"

	"mirage-gateway/pkg/api/proto"
	"mirage-gateway/pkg/ebpf"
	"mirage-gateway/pkg/gswitch"
	"mirage-gateway/pkg/threat"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CommandHandler 下行指令处理器
type CommandHandler struct {
	loader    *ebpf.Loader
	blacklist *threat.BlacklistManager
	gswitch   *gswitch.GSwitchManager
	proto.UnimplementedGatewayDownlinkServer
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

// PushStrategy 处理策略下发 → 写入 eBPF Map（< 100ms）
func (h *CommandHandler) PushStrategy(ctx context.Context, req *proto.StrategyPush) (*proto.PushResponse, error) {
	if req.DefenseLevel < 0 || req.DefenseLevel > 5 {
		return nil, status.Errorf(codes.InvalidArgument, "defense_level 越界: %d", req.DefenseLevel)
	}

	strat := &ebpf.DefenseStrategy{
		JitterMeanUs:   req.JitterMeanUs,
		JitterStddevUs: req.JitterStddevUs,
		NoiseIntensity: req.NoiseIntensity,
		TemplateID:     req.TemplateId,
	}

	if err := h.loader.UpdateStrategy(strat); err != nil {
		log.Printf("[Handler] PushStrategy 写入 eBPF 失败: %v", err)
		return &proto.PushResponse{Success: false, Message: err.Error()}, nil
	}

	log.Printf("[Handler] 策略已更新: level=%d", req.DefenseLevel)
	return &proto.PushResponse{Success: true, Message: "ok"}, nil
}

// PushBlacklist 处理黑名单下发 → 合并到 BlacklistManager
func (h *CommandHandler) PushBlacklist(ctx context.Context, req *proto.BlacklistPush) (*proto.PushResponse, error) {
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
		return &proto.PushResponse{Success: false, Message: err.Error()}, nil
	}

	log.Printf("[Handler] 黑名单已合并: %d 条", len(entries))
	return &proto.PushResponse{Success: true, Message: "ok"}, nil
}

// PushQuota 处理配额下发 → 写入 eBPF quota_map
func (h *CommandHandler) PushQuota(ctx context.Context, req *proto.QuotaPush) (*proto.PushResponse, error) {
	quotaMap := h.loader.GetMap("quota_map")
	if quotaMap == nil {
		return &proto.PushResponse{Success: false, Message: "quota_map 不存在"}, nil
	}

	key := uint32(0)
	value := req.RemainingBytes
	if err := quotaMap.Put(&key, &value); err != nil {
		return &proto.PushResponse{Success: false, Message: err.Error()}, nil
	}

	if value == 0 {
		log.Println("[Handler] ⚠️ 配额为 0，内核态流量阻断已触发")
	}

	log.Printf("[Handler] 配额已更新: %d bytes", value)
	return &proto.PushResponse{Success: true, Message: "ok"}, nil
}

// PushReincarnation 处理转生指令 → 调用 GSwitch.TriggerEscape
func (h *CommandHandler) PushReincarnation(ctx context.Context, req *proto.ReincarnationPush) (*proto.PushResponse, error) {
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
		return &proto.PushResponse{Success: false, Message: err.Error()}, nil
	}

	log.Printf("[Handler] 转生指令已执行: %s → %s", req.NewDomain, req.NewIp)
	return &proto.PushResponse{Success: true, Message: "ok"}, nil
}
