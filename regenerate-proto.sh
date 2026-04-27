#!/bin/bash
# Proto 重新生成脚本

set -e

echo "=== 重新生成 Proto 文件 ==="

# 1. 生成 mirage-proto
echo "1. 生成 mirage-proto..."
cd mirage-proto
make gen
cd ..

# 2. 生成 mirage-os proto
echo "2. 生成 mirage-os proto..."
cd mirage-os
make proto
cd ..

echo "✅ 所有 Proto 文件已重新生成"
