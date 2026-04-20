package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	pb "mirage-os/gateway-bridge/proto"
)

// newTestRedis 创建 miniredis 实例和对应的 go-redis 客户端
func newTestRedis(t *testing.T) (*miniredis.Miniredis, *goredis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return mr, rdb
}

// newTestDownlinkService 创建测试用 DownlinkService
func newTestDownlinkService(t *testing.T) (*miniredis.Miniredis, *DownlinkService) {
	t.Helper()
	mr, rdb := newTestRedis(t)
	connMgr := NewGatewayConnectionManager(rdb)
	ds := NewDownlinkService(connMgr, rdb)
	return mr, ds
}

// ==================== UpdateDesiredState 测试 ====================

func TestUpdateDesiredState_WritesToRedis(t *testing.T) {
	mr, ds := newTestDownlinkService(t)
	ctx := context.Background()

	state := &DesiredState{
		DefenseLevel:   3,
		JitterMeanUs:   100,
		NoiseIntensity: 80,
		RemainingBytes: 1024000,
	}

	err := ds.UpdateDesiredState(ctx, "gw-001", state)
	if err != nil {
		t.Fatalf("UpdateDesiredState 失败: %v", err)
	}

	// 验证 Redis 中写入了 desired_state
	stateJSON, err := mr.Get("gateway:gw-001:desired_state")
	if err != nil {
		t.Fatalf("Redis 中未找到 desired_state: %v", err)
	}
	var stored DesiredState
	if err := json.Unmarshal([]byte(stateJSON), &stored); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if stored.DefenseLevel != 3 {
		t.Errorf("DefenseLevel = %d, want 3", stored.DefenseLevel)
	}
	if stored.NoiseIntensity != 80 {
		t.Errorf("NoiseIntensity = %d, want 80", stored.NoiseIntensity)
	}

	// 验证 Redis 中写入了 state_hash
	hash, err := mr.Get("gateway:gw-001:state_hash")
	if err != nil {
		t.Fatalf("Redis 中未找到 state_hash: %v", err)
	}
	if hash == "" {
		t.Fatal("state_hash 为空")
	}
}

func TestUpdateDesiredState_HashConsistency(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	state := &DesiredState{
		DefenseLevel:   5,
		JitterMeanUs:   200,
		NoiseIntensity: 60,
	}

	// 相同状态写入两次，hash 应一致（UpdatedAt 会变，但我们验证 ComputeStateHash 的确定性）
	hash1 := ComputeStateHash(state)
	hash2 := ComputeStateHash(state)
	if hash1 != hash2 {
		t.Errorf("相同状态的 hash 不一致: %s vs %s", hash1, hash2)
	}

	// 写入后读取验证
	if err := ds.UpdateDesiredState(ctx, "gw-hash", state); err != nil {
		t.Fatalf("UpdateDesiredState 失败: %v", err)
	}
	_, storedHash, err := ds.GetDesiredState(ctx, "gw-hash")
	if err != nil {
		t.Fatalf("GetDesiredState 失败: %v", err)
	}
	if storedHash == "" {
		t.Fatal("存储的 hash 为空")
	}
}

// ==================== ReconcileState 测试 ====================

func TestReconcileState_HashMatch_ReturnsNil(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	state := &DesiredState{DefenseLevel: 3, NoiseIntensity: 80}
	if err := ds.UpdateDesiredState(ctx, "gw-rec", state); err != nil {
		t.Fatalf("UpdateDesiredState 失败: %v", err)
	}

	// 获取当前 hash
	_, currentHash, _ := ds.GetDesiredState(ctx, "gw-rec")

	// 用相同 hash 调用 ReconcileState，应返回 nil
	result, needSync, err := ds.ReconcileState(ctx, "gw-rec", currentHash)
	if err != nil {
		t.Fatalf("ReconcileState 失败: %v", err)
	}
	if needSync {
		t.Error("hash 一致时不应需要同步")
	}
	if result != nil {
		t.Error("hash 一致时应返回 nil state")
	}
}

func TestReconcileState_HashMismatch_ReturnsState(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	state := &DesiredState{DefenseLevel: 5, NoiseIntensity: 90}
	if err := ds.UpdateDesiredState(ctx, "gw-rec2", state); err != nil {
		t.Fatalf("UpdateDesiredState 失败: %v", err)
	}

	// 用错误的 hash 调用
	result, needSync, err := ds.ReconcileState(ctx, "gw-rec2", "wrong-hash")
	if err != nil {
		t.Fatalf("ReconcileState 失败: %v", err)
	}
	if !needSync {
		t.Error("hash 不一致时应需要同步")
	}
	if result == nil {
		t.Fatal("hash 不一致时应返回 state")
	}
	if result.DefenseLevel != 5 {
		t.Errorf("DefenseLevel = %d, want 5", result.DefenseLevel)
	}
}

func TestReconcileState_NoDesiredState_ReturnsNil(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	// 未设置任何 desired state
	result, needSync, err := ds.ReconcileState(ctx, "gw-nonexist", "any-hash")
	if err != nil {
		t.Fatalf("ReconcileState 失败: %v", err)
	}
	if needSync {
		t.Error("无期望状态时不应需要同步")
	}
	if result != nil {
		t.Error("无期望状态时应返回 nil")
	}
}

// ==================== PushBlacklist 测试 ====================

func TestPushBlacklist_UpdatesDesiredState(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	entries := []*pb.BlacklistEntryProto{
		{Cidr: "10.0.0.0/8", ExpireAt: 1700000000, Source: pb.BlacklistSourceType_BL_GLOBAL},
		{Cidr: "192.168.1.0/24", ExpireAt: 1700001000, Source: pb.BlacklistSourceType_BL_LOCAL},
	}

	err := ds.PushBlacklist(ctx, "gw-bl", entries)
	if err != nil {
		t.Fatalf("PushBlacklist 失败: %v", err)
	}

	state, _, err := ds.GetDesiredState(ctx, "gw-bl")
	if err != nil {
		t.Fatalf("GetDesiredState 失败: %v", err)
	}
	if len(state.Blacklist) != 2 {
		t.Fatalf("Blacklist 长度 = %d, want 2", len(state.Blacklist))
	}
	if state.Blacklist[0].CIDR != "10.0.0.0/8" {
		t.Errorf("Blacklist[0].CIDR = %s, want 10.0.0.0/8", state.Blacklist[0].CIDR)
	}
	if state.Blacklist[1].Source != int32(pb.BlacklistSourceType_BL_LOCAL) {
		t.Errorf("Blacklist[1].Source = %d, want BL_LOCAL", state.Blacklist[1].Source)
	}
}

func TestPushBlacklist_OverwritesPrevious(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	// 第一次推送
	entries1 := []*pb.BlacklistEntryProto{
		{Cidr: "10.0.0.0/8", ExpireAt: 1700000000},
	}
	if err := ds.PushBlacklist(ctx, "gw-bl2", entries1); err != nil {
		t.Fatalf("第一次 PushBlacklist 失败: %v", err)
	}

	// 第二次推送（覆盖）
	entries2 := []*pb.BlacklistEntryProto{
		{Cidr: "172.16.0.0/12", ExpireAt: 1700002000},
		{Cidr: "192.168.0.0/16", ExpireAt: 1700003000},
	}
	if err := ds.PushBlacklist(ctx, "gw-bl2", entries2); err != nil {
		t.Fatalf("第二次 PushBlacklist 失败: %v", err)
	}

	state, _, _ := ds.GetDesiredState(ctx, "gw-bl2")
	if len(state.Blacklist) != 2 {
		t.Fatalf("覆盖后 Blacklist 长度 = %d, want 2", len(state.Blacklist))
	}
	if state.Blacklist[0].CIDR != "172.16.0.0/12" {
		t.Errorf("覆盖后 Blacklist[0].CIDR = %s, want 172.16.0.0/12", state.Blacklist[0].CIDR)
	}
}

// ==================== PushStrategy 测试 ====================

func TestPushStrategy_UpdatesDesiredState(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	strategy := &pb.StrategyPush{
		DefenseLevel:   7,
		JitterMeanUs:   500,
		JitterStddevUs: 100,
		NoiseIntensity: 90,
		PaddingRate:    20,
		TemplateId:     42,
	}

	err := ds.PushStrategy(ctx, "gw-strat", strategy)
	if err != nil {
		t.Fatalf("PushStrategy 失败: %v", err)
	}

	state, _, err := ds.GetDesiredState(ctx, "gw-strat")
	if err != nil {
		t.Fatalf("GetDesiredState 失败: %v", err)
	}
	if state.DefenseLevel != 7 {
		t.Errorf("DefenseLevel = %d, want 7", state.DefenseLevel)
	}
	if state.JitterMeanUs != 500 {
		t.Errorf("JitterMeanUs = %d, want 500", state.JitterMeanUs)
	}
	if state.JitterStddevUs != 100 {
		t.Errorf("JitterStddevUs = %d, want 100", state.JitterStddevUs)
	}
	if state.NoiseIntensity != 90 {
		t.Errorf("NoiseIntensity = %d, want 90", state.NoiseIntensity)
	}
	if state.PaddingRate != 20 {
		t.Errorf("PaddingRate = %d, want 20", state.PaddingRate)
	}
	if state.TemplateID != 42 {
		t.Errorf("TemplateID = %d, want 42", state.TemplateID)
	}
}

// ==================== PushQuota 测试 ====================

func TestPushQuota_UpdatesDesiredState(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	err := ds.PushQuota(ctx, "gw-quota", 1099511627776) // 1TB
	if err != nil {
		t.Fatalf("PushQuota 失败: %v", err)
	}

	state, _, err := ds.GetDesiredState(ctx, "gw-quota")
	if err != nil {
		t.Fatalf("GetDesiredState 失败: %v", err)
	}
	if state.RemainingBytes != 1099511627776 {
		t.Errorf("RemainingBytes = %d, want 1099511627776", state.RemainingBytes)
	}
}

func TestPushQuota_PreservesOtherFields(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	// 先设置策略
	strategy := &pb.StrategyPush{DefenseLevel: 5, NoiseIntensity: 80}
	if err := ds.PushStrategy(ctx, "gw-qp", strategy); err != nil {
		t.Fatalf("PushStrategy 失败: %v", err)
	}

	// 再推送配额
	if err := ds.PushQuota(ctx, "gw-qp", 500000); err != nil {
		t.Fatalf("PushQuota 失败: %v", err)
	}

	state, _, _ := ds.GetDesiredState(ctx, "gw-qp")
	if state.DefenseLevel != 5 {
		t.Errorf("PushQuota 后 DefenseLevel = %d, want 5（应保留）", state.DefenseLevel)
	}
	if state.NoiseIntensity != 80 {
		t.Errorf("PushQuota 后 NoiseIntensity = %d, want 80（应保留）", state.NoiseIntensity)
	}
	if state.RemainingBytes != 500000 {
		t.Errorf("RemainingBytes = %d, want 500000", state.RemainingBytes)
	}
}

// ==================== PushReincarnation 测试 ====================

func TestPushReincarnation_AddsToEventQueue(t *testing.T) {
	mr, ds := newTestDownlinkService(t)
	ctx := context.Background()

	push := &pb.ReincarnationPush{
		NewDomain:       "new.example.com",
		NewIp:           "1.2.3.4",
		Reason:          "threat_detected",
		DeadlineSeconds: 300,
	}

	err := ds.PushReincarnation(ctx, "gw-reincarn", push)
	if err != nil {
		t.Fatalf("PushReincarnation 失败: %v", err)
	}

	// 验证事件入队
	eventsKey := "mirage:downlink:events:gw-reincarn"
	items, err := mr.List(eventsKey)
	if err != nil {
		t.Fatalf("读取事件队列失败: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("事件队列长度 = %d, want 1", len(items))
	}

	// 验证事件内容
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(items[0]), &event); err != nil {
		t.Fatalf("反序列化事件失败: %v", err)
	}
	if event["type"] != "reincarnation" {
		t.Errorf("event type = %v, want reincarnation", event["type"])
	}
	data, ok := event["data"].(map[string]interface{})
	if !ok {
		t.Fatal("event data 不是 map")
	}
	if data["new_domain"] != "new.example.com" {
		t.Errorf("new_domain = %v, want new.example.com", data["new_domain"])
	}
	if data["new_ip"] != "1.2.3.4" {
		t.Errorf("new_ip = %v, want 1.2.3.4", data["new_ip"])
	}
}

func TestPushReincarnation_Dedup(t *testing.T) {
	mr, ds := newTestDownlinkService(t)
	ctx := context.Background()

	push := &pb.ReincarnationPush{
		NewDomain:       "dup.example.com",
		NewIp:           "5.6.7.8",
		Reason:          "threat",
		DeadlineSeconds: 600,
	}

	// 推送两次相同 domain
	if err := ds.PushReincarnation(ctx, "gw-dedup", push); err != nil {
		t.Fatalf("第一次 PushReincarnation 失败: %v", err)
	}
	if err := ds.PushReincarnation(ctx, "gw-dedup", push); err != nil {
		t.Fatalf("第二次 PushReincarnation 失败: %v", err)
	}

	eventsKey := "mirage:downlink:events:gw-dedup"
	items, err := mr.List(eventsKey)
	if err != nil {
		t.Fatalf("读取事件队列失败: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("去重后事件队列长度 = %d, want 1", len(items))
	}
}

func TestPushReincarnation_DifferentDomains_BothQueued(t *testing.T) {
	mr, ds := newTestDownlinkService(t)
	ctx := context.Background()

	push1 := &pb.ReincarnationPush{
		NewDomain: "domain1.example.com",
		NewIp:     "1.1.1.1",
		Reason:    "reason1",
	}
	push2 := &pb.ReincarnationPush{
		NewDomain: "domain2.example.com",
		NewIp:     "2.2.2.2",
		Reason:    "reason2",
	}

	if err := ds.PushReincarnation(ctx, "gw-multi", push1); err != nil {
		t.Fatalf("PushReincarnation 1 失败: %v", err)
	}
	if err := ds.PushReincarnation(ctx, "gw-multi", push2); err != nil {
		t.Fatalf("PushReincarnation 2 失败: %v", err)
	}

	eventsKey := "mirage:downlink:events:gw-multi"
	items, err := mr.List(eventsKey)
	if err != nil {
		t.Fatalf("读取事件队列失败: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("不同 domain 事件队列长度 = %d, want 2", len(items))
	}
}

// ==================== GatewayConnectionManager 测试 ====================

func TestGatewayConnectionManager_GetConn_UnknownGateway(t *testing.T) {
	_, rdb := newTestRedis(t)
	connMgr := NewGatewayConnectionManager(rdb)
	ctx := context.Background()

	_, err := connMgr.GetConn(ctx, "gw-unknown")
	if err == nil {
		t.Fatal("未注册地址的 Gateway 应返回错误")
	}
}

func TestGatewayConnectionManager_CloseConn_NoOp(t *testing.T) {
	_, rdb := newTestRedis(t)
	connMgr := NewGatewayConnectionManager(rdb)

	// 关闭不存在的连接不应 panic
	connMgr.CloseConn("gw-nonexist")
}

func TestGatewayConnectionManager_CloseAll(t *testing.T) {
	_, rdb := newTestRedis(t)
	connMgr := NewGatewayConnectionManager(rdb)

	// CloseAll 空连接池不应 panic
	connMgr.CloseAll()
}

// ==================== ComputeStateHash 测试 ====================

func TestComputeStateHash_Deterministic(t *testing.T) {
	state := &DesiredState{
		DefenseLevel:   3,
		JitterMeanUs:   100,
		NoiseIntensity: 80,
		RemainingBytes: 1024,
		UpdatedAt:      1700000000,
	}

	h1 := ComputeStateHash(state)
	h2 := ComputeStateHash(state)
	if h1 != h2 {
		t.Errorf("相同输入 hash 不一致: %s vs %s", h1, h2)
	}
	if len(h1) != 32 { // 16 bytes hex = 32 chars
		t.Errorf("hash 长度 = %d, want 32", len(h1))
	}
}

func TestComputeStateHash_DifferentStates(t *testing.T) {
	s1 := &DesiredState{DefenseLevel: 1}
	s2 := &DesiredState{DefenseLevel: 2}

	h1 := ComputeStateHash(s1)
	h2 := ComputeStateHash(s2)
	if h1 == h2 {
		t.Error("不同状态的 hash 不应相同")
	}
}

// ==================== 端到端流程测试 ====================

func TestEndToEnd_StrategyThenReconcile(t *testing.T) {
	_, ds := newTestDownlinkService(t)
	ctx := context.Background()

	// 1. 推送策略
	strategy := &pb.StrategyPush{
		DefenseLevel:   8,
		NoiseIntensity: 95,
		TemplateId:     10,
	}
	if err := ds.PushStrategy(ctx, "gw-e2e", strategy); err != nil {
		t.Fatalf("PushStrategy 失败: %v", err)
	}

	// 2. 获取当前 hash
	_, currentHash, _ := ds.GetDesiredState(ctx, "gw-e2e")

	// 3. 用正确 hash 对齐 → 无需同步
	_, needSync, _ := ds.ReconcileState(ctx, "gw-e2e", currentHash)
	if needSync {
		t.Error("hash 一致时不应需要同步")
	}

	// 4. 推送配额（状态变更）
	if err := ds.PushQuota(ctx, "gw-e2e", 999999); err != nil {
		t.Fatalf("PushQuota 失败: %v", err)
	}

	// 5. 用旧 hash 对齐 → 需要同步
	result, needSync, _ := ds.ReconcileState(ctx, "gw-e2e", currentHash)
	if !needSync {
		t.Error("状态变更后应需要同步")
	}
	if result == nil {
		t.Fatal("应返回最新状态")
	}
	if result.DefenseLevel != 8 {
		t.Errorf("同步后 DefenseLevel = %d, want 8", result.DefenseLevel)
	}
	if result.RemainingBytes != 999999 {
		t.Errorf("同步后 RemainingBytes = %d, want 999999", result.RemainingBytes)
	}
}

func TestEndToEnd_BlacklistThenReincarnation(t *testing.T) {
	mr, ds := newTestDownlinkService(t)
	ctx := context.Background()

	// 1. 推送黑名单
	entries := []*pb.BlacklistEntryProto{
		{Cidr: "10.0.0.0/8", ExpireAt: 1700000000},
	}
	if err := ds.PushBlacklist(ctx, "gw-e2e2", entries); err != nil {
		t.Fatalf("PushBlacklist 失败: %v", err)
	}

	// 2. 推送转生指令
	push := &pb.ReincarnationPush{
		NewDomain: "reborn.example.com",
		NewIp:     "9.9.9.9",
		Reason:    "scheduled",
	}
	if err := ds.PushReincarnation(ctx, "gw-e2e2", push); err != nil {
		t.Fatalf("PushReincarnation 失败: %v", err)
	}

	// 3. 验证黑名单在 desired state 中
	state, _, _ := ds.GetDesiredState(ctx, "gw-e2e2")
	if len(state.Blacklist) != 1 {
		t.Errorf("Blacklist 长度 = %d, want 1", len(state.Blacklist))
	}

	// 4. 验证转生指令在事件队列中
	eventsKey := fmt.Sprintf("mirage:downlink:events:%s", "gw-e2e2")
	items, _ := mr.List(eventsKey)
	if len(items) != 1 {
		t.Errorf("事件队列长度 = %d, want 1", len(items))
	}
}
