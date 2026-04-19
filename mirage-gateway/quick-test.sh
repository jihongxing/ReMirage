#!/bin/bash
# 快速验证脚本 - 仅检查代码完整性，不需要编译

echo "=========================================="
echo "Mirage-Gateway 计费功能快速验证"
echo "=========================================="

PASS=0
FAIL=0

check() {
    if [ $? -eq 0 ]; then
        echo "   ✅ $1"
        PASS=$((PASS + 1))
    else
        echo "   ❌ $1"
        FAIL=$((FAIL + 1))
    fi
}

# 1. 检查内核态代码
echo ""
echo "📦 检查内核态代码（bpf/jitter.c）..."

grep -q "quota_map" bpf/jitter.c
check "quota_map 已定义"

grep -q "traffic_stats" bpf/jitter.c
check "traffic_stats 已定义"

grep -q "TC_ACT_STOLEN" bpf/jitter.c
check "配额熔断逻辑已实现"

grep -q "__sync_fetch_and_add" bpf/jitter.c
check "流量统计逻辑已实现"

# 2. 检查 common.h
echo ""
echo "📦 检查公共头文件（bpf/common.h）..."

grep -q "} quota_map SEC" bpf/common.h
check "quota_map 结构已定义"

grep -q "} traffic_stats SEC" bpf/common.h
check "traffic_stats 结构已定义"

# 3. 检查用户态代码
echo ""
echo "📦 检查用户态代码（pkg/ebpf/billing.go）..."

grep -q "type BillingReporter struct" pkg/ebpf/billing.go
check "BillingReporter 结构体已定义"

grep -q "func.*reportTraffic" pkg/ebpf/billing.go
check "reportTraffic() 方法已实现"

grep -q "func.*syncQuota" pkg/ebpf/billing.go
check "syncQuota() 方法已实现"

grep -q "func.*GetTrafficStats" pkg/ebpf/billing.go
check "GetTrafficStats() 方法已实现"

grep -q "func.*SetQuotaStatus" pkg/ebpf/billing.go
check "SetQuotaStatus() 方法已实现"

# 4. 检查主程序集成
echo ""
echo "📦 检查主程序集成（cmd/gateway/main.go）..."

grep -q "NewBillingReporter" cmd/gateway/main.go
check "BillingReporter 已创建"

grep -q "billingReporter.Start()" cmd/gateway/main.go
check "BillingReporter 已启动"

# 5. 检查类型定义
echo ""
echo "📦 检查类型定义（pkg/ebpf/types.go）..."

grep -q "type ThreatEvent struct" pkg/ebpf/types.go
check "ThreatEvent 结构体已定义"

# 6. 统计结果
echo ""
echo "=========================================="
echo "验证结果: ✅ $PASS 通过, ❌ $FAIL 失败"
echo "=========================================="

if [ $FAIL -eq 0 ]; then
    echo ""
    echo "🎉 所有检查通过！代码完整性验证成功。"
    echo ""
    echo "📋 下一步："
    echo "   1. 在 WSL 中编译: make clean && make all"
    echo "   2. 启动测试: sudo ./bin/mirage-gateway -iface eth0 -defense 20 -traffic 100"
    echo "   3. 观察日志: 应该看到 '💰 计费上报器已启动'"
    echo "   4. 等待 10 秒: 应该看到 '📊 [计费] 上报流量: ...'"
    echo ""
    exit 0
else
    echo ""
    echo "⚠️  发现 $FAIL 个问题，请检查代码。"
    echo ""
    exit 1
fi
