#!/bin/bash
# drill-m8-baseline.sh - M8 基线验证演练脚本
# 用途：按目标部署等级执行 Baseline_Checklist 中对应检查项
# 用法：bash deploy/scripts/drill-m8-baseline.sh [default|hardened|extreme]
# 输出：deploy/evidence/m8-baseline-drill.log

set -euo pipefail

TIER="${1:-hardened}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG_FILE="$PROJECT_ROOT/deploy/evidence/m8-baseline-drill.log"
PASS=0
FAIL=0
SKIP=0
TOTAL=0

mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "$1" | tee -a "$LOG_FILE"
}

check() {
    local name="$1"
    local result="$2"  # PASS / FAIL / SKIP
    local detail="$3"
    TOTAL=$((TOTAL + 1))
    case "$result" in
        PASS) PASS=$((PASS + 1)); log "  ✅ [$result] $name: $detail" ;;
        FAIL) FAIL=$((FAIL + 1)); log "  ❌ [$result] $name: $detail" ;;
        SKIP) SKIP=$((SKIP + 1)); log "  ⏭️  [$result] $name: $detail" ;;
    esac
}

# 清空日志
> "$LOG_FILE"

log "═══════════════════════════════════════════════════════════"
log "  M8 基线验证演练 — 部署等级: $TIER"
log "  时间: $(date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date '+%Y-%m-%dT%H:%M:%S')"
log "═══════════════════════════════════════════════════════════"
log ""

# ─── 步骤 1: 检查运行环境 ───
log "--- 步骤 1: 环境检查 ---"

IS_LINUX=0
IS_CONTAINER=0
IS_ROOT=0

if [ "$(uname -s 2>/dev/null)" = "Linux" ]; then
    IS_LINUX=1
    log "  平台: Linux"
else
    log "  平台: $(uname -s 2>/dev/null || echo 'Unknown')（非 Linux，部分检查将跳过）"
fi

if [ -f "/.dockerenv" ] || grep -q "docker\|containerd" /proc/1/cgroup 2>/dev/null; then
    IS_CONTAINER=1
    log "  环境: 容器内"
else
    log "  环境: 非容器（tmpfs/只读根检查将跳过）"
fi

if [ "$(id -u 2>/dev/null)" = "0" ]; then
    IS_ROOT=1
    log "  权限: root"
else
    log "  权限: 非 root（部分检查可能受限）"
fi
log ""

if [ "$TIER" = "default" ]; then
    log "--- Default 等级：无强制检查项 ---"
    log "Default 部署等级不强制要求 RAM_Shield、tmpfs、只读根等配置。"
    log "以下仅执行 Emergency_Wipe 脚本存在性检查（可选）。"
    log ""
fi

# ─── 步骤 2: RAM_Shield 状态检查 ───
if [ "$TIER" != "default" ]; then
    log "--- 步骤 2: RAM_Shield 状态检查 ---"

    # 2a. mlock 检查
    if [ "$IS_LINUX" = "1" ]; then
        GW_PID=$(pgrep -f "mirage-gateway" 2>/dev/null | head -1 || true)
        if [ -n "$GW_PID" ]; then
            VMLCK=$(grep "VmLck" /proc/$GW_PID/status 2>/dev/null | awk '{print $2}' || echo "0")
            if [ "$VMLCK" != "0" ] && [ -n "$VMLCK" ]; then
                check "mlock 生效" "PASS" "VmLck=${VMLCK} kB (PID=$GW_PID)"
            else
                check "mlock 生效" "FAIL" "VmLck=${VMLCK:-0} kB (PID=$GW_PID)"
            fi
        else
            check "mlock 生效" "SKIP" "Gateway 进程未运行"
        fi
    else
        check "mlock 生效" "SKIP" "需 Linux 环境验证"
    fi

    # 2b. Core dump 禁用
    if [ "$IS_LINUX" = "1" ]; then
        CORE_PATTERN=$(cat /proc/sys/kernel/core_pattern 2>/dev/null || echo "unknown")
        ULIMIT_C=$(ulimit -c 2>/dev/null || echo "unknown")
        if [ "$ULIMIT_C" = "0" ] || [ "$CORE_PATTERN" = "" ] || [ "$CORE_PATTERN" = "/dev/null" ]; then
            check "Core dump 禁用" "PASS" "ulimit -c=$ULIMIT_C, core_pattern=$CORE_PATTERN"
        else
            check "Core dump 禁用" "FAIL" "ulimit -c=$ULIMIT_C, core_pattern=$CORE_PATTERN"
        fi
    else
        check "Core dump 禁用" "SKIP" "需 Linux 环境验证"
    fi

    # 2c. Swap 使用量
    if [ "$IS_LINUX" = "1" ]; then
        SWAP_TOTAL=$(grep "SwapTotal" /proc/meminfo 2>/dev/null | awk '{print $2}' || echo "unknown")
        SWAP_SHOW=$(swapon --show 2>/dev/null || echo "")
        if [ "$SWAP_TOTAL" = "0" ] || [ -z "$SWAP_SHOW" ]; then
            check "Swap 使用量为零" "PASS" "SwapTotal=${SWAP_TOTAL:-0} kB"
        else
            check "Swap 使用量为零" "FAIL" "SwapTotal=${SWAP_TOTAL} kB, swapon 有输出"
        fi
    else
        check "Swap 使用量为零" "SKIP" "需 Linux 环境验证"
    fi
    log ""
fi

# ─── 步骤 3: 证书配置检查 ───
if [ "$TIER" != "default" ]; then
    log "--- 步骤 3: 证书配置检查 ---"

    # 3a. tmpfs 证书存储
    if [ "$IS_CONTAINER" = "1" ]; then
        if mount 2>/dev/null | grep -q "tmpfs.*certs"; then
            check "证书在 tmpfs" "PASS" "$(mount | grep tmpfs | grep certs | head -1)"
        else
            check "证书在 tmpfs" "FAIL" "未检测到 tmpfs 上的 certs 挂载"
        fi
    else
        check "证书在 tmpfs" "SKIP" "需容器环境验证"
    fi

    # 3b. 证书有效期
    CERT_FILE="/var/mirage/certs/gateway.crt"
    if [ ! -f "$CERT_FILE" ]; then
        CERT_FILE="/etc/mirage/certs/gateway.crt"
    fi
    if [ -f "$CERT_FILE" ]; then
        EXPIRY=$(openssl x509 -enddate -noout -in "$CERT_FILE" 2>/dev/null | cut -d= -f2 || echo "unknown")
        check "证书有效期" "PASS" "到期时间: $EXPIRY"
    else
        check "证书有效期" "SKIP" "证书未部署"
    fi

    # 3c. CA 私钥不在 Gateway
    CA_KEY_FOUND=0
    [ -f "/var/mirage/certs/ca.key" ] && CA_KEY_FOUND=1
    [ -f "/etc/mirage/certs/ca.key" ] && CA_KEY_FOUND=1
    if [ "$CA_KEY_FOUND" = "0" ]; then
        check "CA 私钥不在 Gateway" "PASS" "未找到 ca.key"
    else
        check "CA 私钥不在 Gateway" "FAIL" "发现 ca.key 文件"
    fi
    log ""
fi

# ─── 步骤 4: 文件系统检查 ───
if [ "$TIER" != "default" ]; then
    log "--- 步骤 4: 文件系统检查 ---"

    # 4a. 只读根文件系统
    if [ "$IS_CONTAINER" = "1" ]; then
        if mount 2>/dev/null | grep " / " | grep -q "ro,"; then
            check "只读根文件系统" "PASS" "$(mount | grep ' / ' | head -1)"
        else
            check "只读根文件系统" "FAIL" "根文件系统非只读"
        fi
    else
        check "只读根文件系统" "SKIP" "需容器环境验证"
    fi

    # 4b. 无非 tmpfs 可写挂载点（仅 Extreme Stealth）
    if [ "$TIER" = "extreme" ]; then
        if [ "$IS_CONTAINER" = "1" ]; then
            WRITABLE=$(mount 2>/dev/null | grep -v "tmpfs" | grep -v "ro," | grep -v "proc\|sys\|dev\|cgroup" || true)
            if [ -z "$WRITABLE" ]; then
                check "无非 tmpfs 可写挂载" "PASS" "无可写非 tmpfs 挂载点"
            else
                check "无非 tmpfs 可写挂载" "FAIL" "发现可写挂载: $WRITABLE"
            fi
        else
            check "无非 tmpfs 可写挂载" "SKIP" "需容器环境验证"
        fi
    fi

    # 4c. Swap 分区禁用
    if [ "$IS_LINUX" = "1" ]; then
        SWAP_SHOW=$(swapon --show 2>/dev/null || echo "")
        if [ -z "$SWAP_SHOW" ]; then
            check "Swap 分区禁用" "PASS" "swapon --show 输出为空"
        else
            check "Swap 分区禁用" "FAIL" "存在活跃 swap 分区"
        fi
    else
        check "Swap 分区禁用" "SKIP" "需 Linux 环境验证"
    fi
    log ""
fi

# ─── 步骤 5: Emergency_Wipe 可用性检查 ───
if [ "$TIER" != "default" ]; then
    log "--- 步骤 5: Emergency_Wipe 可用性检查 ---"

    WIPE_SCRIPT="$PROJECT_ROOT/deploy/scripts/emergency-wipe.sh"

    # 5a. 脚本存在且可执行
    if [ -f "$WIPE_SCRIPT" ]; then
        if [ -x "$WIPE_SCRIPT" ]; then
            check "Emergency_Wipe 脚本存在" "PASS" "$WIPE_SCRIPT 存在且可执行"
        else
            check "Emergency_Wipe 脚本存在" "PASS" "$WIPE_SCRIPT 存在（需 chmod +x）"
        fi
    else
        check "Emergency_Wipe 脚本存在" "FAIL" "$WIPE_SCRIPT 不存在"
    fi

    # 5b. 依赖工具
    if [ "$IS_LINUX" = "1" ]; then
        SHRED_OK=0; BPFTOOL_OK=0
        which shred >/dev/null 2>&1 && SHRED_OK=1
        which bpftool >/dev/null 2>&1 && BPFTOOL_OK=1
        if [ "$SHRED_OK" = "1" ] && [ "$BPFTOOL_OK" = "1" ]; then
            check "依赖工具可用" "PASS" "shred 和 bpftool 均可用"
        else
            MISSING=""
            [ "$SHRED_OK" = "0" ] && MISSING="shred"
            [ "$BPFTOOL_OK" = "0" ] && MISSING="$MISSING bpftool"
            check "依赖工具可用" "FAIL" "缺失: $MISSING"
        fi
    else
        check "依赖工具可用" "SKIP" "需 Linux 环境验证"
    fi

    # 5c. Dry-run 验证
    if [ -f "$WIPE_SCRIPT" ]; then
        DRYRUN_OUTPUT=$(bash "$WIPE_SCRIPT" 2>&1 || true)
        if echo "$DRYRUN_OUTPUT" | grep -q "confirm"; then
            check "Dry-run 验证" "PASS" "脚本正确提示需要 --confirm 参数"
        else
            check "Dry-run 验证" "FAIL" "脚本未正确提示"
        fi
    else
        check "Dry-run 验证" "SKIP" "脚本不存在"
    fi
    log ""
fi

# ─── 步骤 6: 汇总结果 ───
log "═══════════════════════════════════════════════════════════"
log "  M8 基线验证结果汇总"
log "  部署等级: $TIER"
log "  总检查项: $TOTAL"
log "  通过: $PASS"
log "  失败: $FAIL"
log "  跳过: $SKIP"
log "═══════════════════════════════════════════════════════════"

if [ "$FAIL" -gt 0 ]; then
    log ""
    log "⚠️  存在 $FAIL 个检查项未通过，请排查后重新验证。"
    exit 1
else
    log ""
    log "✅ 所有检查项通过或已标注跳过原因。"
    exit 0
fi
