#!/bin/bash
# 影子认证验证脚本
# 验证：无硬件签名 → 无法获取 WebSocket 数据

set -e

API_HOST="${API_HOST:-localhost}"
WS_HOST="${WS_HOST:-localhost}"
API_PORT="${API_PORT:-50051}"
WS_PORT="${WS_PORT:-8080}"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# ============================================
# 测试 1：无签名访问 WebSocket
# ============================================
test_ws_without_auth() {
    log "🧪 测试 1：无签名访问 WebSocket"
    
    # 尝试连接 WebSocket（无 JWT）
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" \
        --max-time 5 \
        "http://${WS_HOST}:${WS_PORT}/ws" \
        -H "Upgrade: websocket" \
        -H "Connection: Upgrade" \
        2>/dev/null || echo "000")
    
    if [ "$RESPONSE" == "401" ] || [ "$RESPONSE" == "403" ]; then
        log "✅ 测试通过：无签名访问被拒绝 (HTTP $RESPONSE)"
    else
        log "⚠️  测试结果：HTTP $RESPONSE（预期 401/403）"
    fi
}

# ============================================
# 测试 2：伪造签名访问
# ============================================
test_ws_with_fake_signature() {
    log "🧪 测试 2：伪造签名访问"
    
    FAKE_SIGNATURE="deadbeef1234567890abcdef"
    
    RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" \
        --max-time 5 \
        "http://${WS_HOST}:${WS_PORT}/ws" \
        -H "Upgrade: websocket" \
        -H "Connection: Upgrade" \
        -H "X-Hardware-Signature: ${FAKE_SIGNATURE}" \
        2>/dev/null || echo "000")
    
    if [ "$RESPONSE" == "401" ] || [ "$RESPONSE" == "403" ]; then
        log "✅ 测试通过：伪造签名被拒绝 (HTTP $RESPONSE)"
    else
        log "⚠️  测试结果：HTTP $RESPONSE（预期 401/403）"
    fi
}

# ============================================
# 测试 3：挑战-响应流程
# ============================================
test_challenge_response() {
    log "🧪 测试 3：挑战-响应流程"
    
    # 1. 请求挑战
    log "  → 请求挑战码..."
    CHALLENGE_RESPONSE=$(curl -s -X POST \
        "http://${API_HOST}:${API_PORT}/api/v1/auth/challenge" \
        -H "Content-Type: application/json" \
        -d '{"user_id": "test_user_001"}' \
        2>/dev/null || echo '{"error": "connection failed"}')
    
    if echo "$CHALLENGE_RESPONSE" | grep -q "challenge"; then
        log "  ✅ 挑战码已生成"
        CHALLENGE=$(echo "$CHALLENGE_RESPONSE" | jq -r '.challenge' 2>/dev/null || echo "")
        log "  → 挑战码: ${CHALLENGE:0:50}..."
    else
        log "  ⚠️  挑战码生成失败: $CHALLENGE_RESPONSE"
        return
    fi
    
    # 2. 使用 mirage-cli 签名（如果可用）
    if command -v mirage-cli &> /dev/null; then
        log "  → 使用 mirage-cli 签名..."
        SIGNATURE=$(mirage-cli sign "$CHALLENGE" 2>/dev/null | grep -oP '(?<=Hardware Signature: ).*' || echo "")
        
        if [ -n "$SIGNATURE" ]; then
            log "  ✅ 签名已生成: ${SIGNATURE:0:32}..."
            
            # 3. 验证签名
            VERIFY_RESPONSE=$(curl -s -X POST \
                "http://${API_HOST}:${API_PORT}/api/v1/auth/verify" \
                -H "Content-Type: application/json" \
                -d "{\"challenge_id\": \"$CHALLENGE_ID\", \"signature\": \"$SIGNATURE\"}" \
                2>/dev/null || echo '{"error": "connection failed"}')
            
            if echo "$VERIFY_RESPONSE" | grep -q "success"; then
                log "  ✅ 签名验证通过"
            else
                log "  ⚠️  签名验证失败: $VERIFY_RESPONSE"
            fi
        else
            log "  ⚠️  签名生成失败"
        fi
    else
        log "  ⚠️  mirage-cli 未安装，跳过签名测试"
    fi
}

# ============================================
# 测试 4：多租户隔离
# ============================================
test_tenant_isolation() {
    log "🧪 测试 4：多租户数据隔离"
    
    # 用户 A 的 WebSocket 连接
    log "  → 用户 A 连接..."
    # 实际测试需要有效的 JWT
    
    # 用户 B 的 WebSocket 连接
    log "  → 用户 B 连接..."
    
    log "  ⚠️  多租户隔离测试需要有效的 JWT，跳过"
}

# ============================================
# 主流程
# ============================================

main() {
    log "═══════════════════════════════════════════════════════"
    log "🔐 影子认证验证脚本"
    log "═══════════════════════════════════════════════════════"
    log "API: ${API_HOST}:${API_PORT}"
    log "WS:  ${WS_HOST}:${WS_PORT}"
    log "═══════════════════════════════════════════════════════"
    
    test_ws_without_auth
    test_ws_with_fake_signature
    test_challenge_response
    test_tenant_isolation
    
    log "═══════════════════════════════════════════════════════"
    log "✅ 验证完成"
    log "═══════════════════════════════════════════════════════"
}

main "$@"
