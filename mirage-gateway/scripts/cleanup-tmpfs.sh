#!/bin/bash
# Mirage Gateway - tmpfs 清理脚本

set -e

echo "=========================================="
echo "🧹 Mirage Gateway - tmpfs 清理"
echo "=========================================="

TMPFS_MOUNT="/var/mirage"

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then 
    echo "❌ 需要 root 权限运行"
    exit 1
fi

# 1. 停止 Gateway 进程
echo ""
echo "1️⃣ 停止 Gateway 进程..."
pkill -f "mirage-gateway" || echo "   ℹ️ 未找到运行中的 Gateway"

# 2. 清空 tmpfs 内容
echo ""
echo "2️⃣ 清空 tmpfs 内容..."
if [ -d "$TMPFS_MOUNT" ]; then
    rm -rf "$TMPFS_MOUNT"/*
    echo "   ✅ 已清空 $TMPFS_MOUNT"
else
    echo "   ℹ️ 目录不存在: $TMPFS_MOUNT"
fi

# 3. 卸载 tmpfs
echo ""
echo "3️⃣ 卸载 tmpfs..."
if mount | grep -q "$TMPFS_MOUNT"; then
    umount "$TMPFS_MOUNT"
    echo "   ✅ 已卸载 $TMPFS_MOUNT"
else
    echo "   ℹ️ tmpfs 未挂载"
fi

# 4. 删除挂载点
echo ""
echo "4️⃣ 删除挂载点..."
if [ -d "$TMPFS_MOUNT" ]; then
    rmdir "$TMPFS_MOUNT"
    echo "   ✅ 已删除 $TMPFS_MOUNT"
fi

# 5. 重新启用 swap（可选）
echo ""
echo "5️⃣ 重新启用 swap..."
read -p "   是否重新启用 swap？(y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    swapon -a
    echo "   ✅ 已启用 swap"
else
    echo "   ⏭️ 跳过"
fi

echo ""
echo "✅ 清理完成！"
echo "=========================================="
