#!/bin/bash
# Mirage Gateway - tmpfs 无盘部署脚本
# 确保网关在内存中运行，物理查封时无痕迹

set -e

echo "=========================================="
echo "🔒 Mirage Gateway - tmpfs 无盘部署"
echo "=========================================="

# 配置
TMPFS_SIZE="512M"
TMPFS_MOUNT="/var/mirage"
GATEWAY_BIN="./bin/mirage-gateway"
CONFIG_FILE="./configs/gateway.yaml"

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then 
    echo "❌ 需要 root 权限运行"
    echo "   请使用: sudo $0"
    exit 1
fi

# 1. 创建 tmpfs 挂载点
echo ""
echo "1️⃣ 创建 tmpfs 挂载点..."
if [ ! -d "$TMPFS_MOUNT" ]; then
    mkdir -p "$TMPFS_MOUNT"
    echo "   ✅ 创建目录: $TMPFS_MOUNT"
else
    echo "   ⚠️ 目录已存在: $TMPFS_MOUNT"
fi

# 2. 挂载 tmpfs
echo ""
echo "2️⃣ 挂载 tmpfs (大小: $TMPFS_SIZE)..."
if mount | grep -q "$TMPFS_MOUNT"; then
    echo "   ⚠️ tmpfs 已挂载，先卸载..."
    umount "$TMPFS_MOUNT"
fi

mount -t tmpfs -o size=$TMPFS_SIZE,mode=0700 tmpfs "$TMPFS_MOUNT"
echo "   ✅ tmpfs 已挂载到 $TMPFS_MOUNT"

# 3. 复制二进制文件到 tmpfs
echo ""
echo "3️⃣ 复制 Gateway 到 tmpfs..."
if [ ! -f "$GATEWAY_BIN" ]; then
    echo "❌ Gateway 二进制文件不存在: $GATEWAY_BIN"
    echo "   请先编译: make build"
    exit 1
fi

cp "$GATEWAY_BIN" "$TMPFS_MOUNT/mirage-gateway"
chmod 700 "$TMPFS_MOUNT/mirage-gateway"
echo "   ✅ 已复制: $TMPFS_MOUNT/mirage-gateway"

# 4. 复制配置文件到 tmpfs
echo ""
echo "4️⃣ 复制配置文件到 tmpfs..."
if [ -f "$CONFIG_FILE" ]; then
    cp "$CONFIG_FILE" "$TMPFS_MOUNT/gateway.yaml"
    chmod 600 "$TMPFS_MOUNT/gateway.yaml"
    echo "   ✅ 已复制: $TMPFS_MOUNT/gateway.yaml"
else
    echo "   ⚠️ 配置文件不存在，跳过"
fi

# 5. 复制 eBPF 对象文件到 tmpfs
echo ""
echo "5️⃣ 复制 eBPF 程序到 tmpfs..."
if [ -d "./bpf" ]; then
    mkdir -p "$TMPFS_MOUNT/bpf"
    cp ./bpf/*.o "$TMPFS_MOUNT/bpf/" 2>/dev/null || true
    echo "   ✅ 已复制 eBPF 程序"
else
    echo "   ⚠️ eBPF 目录不存在，跳过"
fi

# 6. 禁用 swap（防止内存泄露到磁盘）
echo ""
echo "6️⃣ 禁用 swap..."
if swapon --show | grep -q "/"; then
    swapoff -a
    echo "   ✅ 已禁用所有 swap"
else
    echo "   ℹ️ 系统未启用 swap"
fi

# 7. 设置内存锁定限制
echo ""
echo "7️⃣ 设置内存锁定限制..."
ulimit -l unlimited
echo "   ✅ 已设置 ulimit -l unlimited"

# 8. 禁用 core dump
echo ""
echo "8️⃣ 禁用 core dump..."
ulimit -c 0
echo "   ✅ 已禁用 core dump"

# 9. 清理原始文件（可选）
echo ""
echo "9️⃣ 清理原始文件..."
read -p "   是否删除原始二进制文件？(y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -f "$GATEWAY_BIN"
    echo "   ✅ 已删除原始文件"
else
    echo "   ⏭️ 跳过删除"
fi

# 10. 启动 Gateway
echo ""
echo "🚀 启动 Gateway..."
cd "$TMPFS_MOUNT"

# 设置环境变量
export MIRAGE_TMPFS_MODE=1
export MIRAGE_NO_PERSIST=1

# 后台启动
nohup ./mirage-gateway > /dev/null 2>&1 &
GATEWAY_PID=$!

echo "   ✅ Gateway 已启动 (PID: $GATEWAY_PID)"
echo "   📍 工作目录: $TMPFS_MOUNT"

# 11. 验证部署
echo ""
echo "✅ 部署完成！"
echo ""
echo "=========================================="
echo "验证信息"
echo "=========================================="
echo "tmpfs 挂载点: $TMPFS_MOUNT"
echo "tmpfs 大小:   $TMPFS_SIZE"
echo "Gateway PID:  $GATEWAY_PID"
echo "Swap 状态:    $(swapon --show | wc -l) 个 swap 设备"
echo ""
echo "=========================================="
echo "安全提示"
echo "=========================================="
echo "✅ 所有数据存储在内存中"
echo "✅ 重启后自动清除"
echo "✅ 无磁盘持久化"
echo "⚠️ 断电/重启会丢失所有数据"
echo ""
echo "=========================================="
echo "清理命令"
echo "=========================================="
echo "停止 Gateway:  kill $GATEWAY_PID"
echo "卸载 tmpfs:    umount $TMPFS_MOUNT"
echo "完整清理:      ./scripts/cleanup-tmpfs.sh"
echo "=========================================="
