// Package api - 会话管理器：session_id → user_id/client_id 映射
package api

import (
	"sync"
	"time"
)

// SessionInfo 会话信息
type SessionInfo struct {
	SessionID   string
	UserID      string
	ClientID    string
	ConnectedAt time.Time
}

// SessionManager 会话管理器
type SessionManager struct {
	sessions map[string]*SessionInfo // session_id → info
	byUser   map[string][]string     // user_id → []session_id
	mu       sync.RWMutex
}

// NewSessionManager 创建会话管理器
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*SessionInfo),
		byUser:   make(map[string][]string),
	}
}

// Register 注册新会话
func (sm *SessionManager) Register(sessionID, userID, clientID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[sessionID] = &SessionInfo{
		SessionID:   sessionID,
		UserID:      userID,
		ClientID:    clientID,
		ConnectedAt: time.Now(),
	}
	sm.byUser[userID] = append(sm.byUser[userID], sessionID)
}

// Unregister 注销会话，返回被注销的会话信息
func (sm *SessionManager) Unregister(sessionID string) *SessionInfo {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	info, ok := sm.sessions[sessionID]
	if !ok {
		return nil
	}
	delete(sm.sessions, sessionID)

	// 从 byUser 中移除
	sids := sm.byUser[info.UserID]
	for i, s := range sids {
		if s == sessionID {
			sm.byUser[info.UserID] = append(sids[:i], sids[i+1:]...)
			break
		}
	}
	if len(sm.byUser[info.UserID]) == 0 {
		delete(sm.byUser, info.UserID)
	}
	return info
}

// GetUserID 根据 session_id 查找 user_id
func (sm *SessionManager) GetUserID(sessionID string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if info, ok := sm.sessions[sessionID]; ok {
		return info.UserID
	}
	return ""
}

// ActiveSessionCount 当前活跃会话数
func (sm *SessionManager) ActiveSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// GetActiveSessionsByUser 获取指定用户的所有活跃 session_id
func (sm *SessionManager) GetActiveSessionsByUser(userID string) []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sids := sm.byUser[userID]
	if len(sids) == 0 {
		return nil
	}
	result := make([]string, len(sids))
	copy(result, sids)
	return result
}

// GetSession 获取指定会话信息
func (sm *SessionManager) GetSession(sessionID string) *SessionInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[sessionID]
}

// DisconnectUser 断开指定用户的所有会话，返回断开的会话数
// 用于配额熔断时仅断开该用户连接，不影响其他用户
func (sm *SessionManager) DisconnectUser(userID string) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sids := sm.byUser[userID]
	count := len(sids)
	for _, sid := range sids {
		delete(sm.sessions, sid)
	}
	delete(sm.byUser, userID)
	return count
}
