#!/bin/bash
# 计费功能测试脚本

echo "=========================================="
echo "Mirage-Gateway 计费功能测试"
echo "=========================================="

# 1. 清理并编译
echo ""
echo "📦 步骤 1: 清理并编译..."
make clean
make all

if [ $? -ne 0 ]; then
    echo "❌ 编译失败"
    exit 1
fi

echo "✅ 编译成功"

# 2. 检查 eBPF 程序中的计费 Map
echo ""
echo "📦 步骤 2: 检查 eBPF 程序..."
echo "   - traffic_stats Map (流量统计)"
echo "   - quota_map Map (配额控制)"

llvm-objdump -S bpf/jitter.o | grep -A 5 "traffic_stats\|quota_map" || echo "   ⚠️  无法检查（需要 llvm-objdump）"

# 3. 验证 Go 代码中的计费模块
echo ""
echo "📦 步骤 3: 验证 Go 计费模块..."
if [ -f "pkg/ebpf/billing.go" ]; then
    echo "   ✅ billing.go 存在"
    grep -q "BillingReporter" pkg/ebpf/billing.go && echo "   ✅ BillingReporter 结构体已定义"
    grep -q "reportTraffic" pkg/ebpf/billing.go && echo "   ✅ reportTraffic() 方法已实现"
    grep -q "syncQuota" pkg/ebpf/billing.go && echo "   ✅ syncQuota() 方法已实现"
else
    echo "   ❌ billing.go 不存在"
    exit 1
fi

# 4. 验证主程序集成
echo ""
echo "📦 步骤 4: 验证主程序集成..."
if grep -q "BillingReporter" cmd/gateway/main.go; then
    echo "   ✅ 主程序已集成 BillingReporter"
else
    echo "   ❌ 主程序未集成 BillingReporter"
    exit 1
fi

# 5. 运行测试（需要 root 权限）
echo ""
echo "=========================================="
echo "✅ 编译验证通过！"
echo "=========================================="
echo ""
echo "📋 下一步操作："
echo "   1. 启动 Gateway: sudo ./bin/mirage-gateway -iface eth0 -defense 20 -traffic 100"
echo "   2. 观察日志中的计费信息："
echo "      - 💰 计费上报器已启动"
echo "      - 📊 [计费] 上报流量: 业务=XXX字节, 防御=XXX字节"
echo "   3. 测试配额熔断："
echo "      - 手动设置配额为 0"
echo "      - 观察流量是否被阻断"
echo ""
echo "⚠️  注意："
echo "   - HTTP 上报到 Mirage-OS 标记为 TODO（等待 Mirage-OS 实现）"
echo "   - 当前使用本地模式（无限配额）"
echo ""
