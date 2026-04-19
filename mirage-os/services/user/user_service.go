// 用户匿名服务 - UID 派生与会话管理
package user

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type UserProfile struct {
	UID           string    `json:"uid"`
	PublicKeyHash string    `json:"public_key_hash"`
	CreditLevel   int       `json:"credit_level"`
	CreatedAt     time.Time `json:"created_at"`
	LastSeen      time.Time `json:"last_seen"`
	AssignedNode  string    `json:"assigned_node"`
	GeoRegion     string    `json:"geo_region"`
}

type SecurityContext struct {
	UID         string
	Token       string
	ExpiresAt   time.Time
	NodeID      string
	Permissions []string
}

type UserService struct {
	mu       sync.RWMutex
	profiles map[string]*UserProfile
	sessions map[string]*SecurityContext
	jwtKey   []byte
	geoSvc   GeoService
}

type GeoService interface {
	GetRegion(ip string) string
	GetNearestNode(region string) string
}

func NewUserService(jwtKey []byte, geoSvc GeoService) *UserService {
	return &UserService{
		profiles: make(map[string]*UserProfile),
		sessions: make(map[string]*SecurityContext),
		jwtKey:   jwtKey,
		geoSvc:   geoSvc,
	}
}

// GenerateUIDFromPublicKey 从公钥派生全局唯一 UID（不可逆推）
func GenerateUIDFromPublicKey(pubKey []byte) string {
	// 双重哈希确保不可逆
	first := sha256.Sum256(pubKey)
	second := sha256.Sum256(first[:])
	// 取前 12 字节作为 UID
	return "u-" + hex.EncodeToString(second[:6])
}

// CreateOrGetProfile 创建或获取用户 Profile
func (s *UserService) CreateOrGetProfile(pubKey []byte, clientIP string) (*UserProfile, error) {
	uid := GenerateUIDFromPublicKey(pubKey)
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if profile, exists := s.profiles[uid]; exists {
		profile.LastSeen = time.Now()
		return profile, nil
	}
	
	// 新用户
	region := ""
	assignedNode := ""
	if s.geoSvc != nil {
		region = s.geoSvc.GetRegion(clientIP)
		assignedNode = s.geoSvc.GetNearestNode(region)
	}
	
	profile := &UserProfile{
		UID:           uid,
		PublicKeyHash: hex.EncodeToString(sha256.New().Sum(pubKey)),
		CreditLevel:   1,
		CreatedAt:     time.Now(),
		LastSeen:      time.Now(),
		AssignedNode:  assignedNode,
		GeoRegion:     region,
	}
	
	s.profiles[uid] = profile
	return profile, nil
}

// CreateSession 创建 JWT 会话
func (s *UserService) CreateSession(uid string) (*SecurityContext, error) {
	s.mu.RLock()
	profile, exists := s.profiles[uid]
	s.mu.RUnlock()
	
	if !exists {
		return nil, errors.New("user not found")
	}
	
	expiresAt := time.Now().Add(24 * time.Hour)
	
	claims := jwt.MapClaims{
		"uid":    uid,
		"node":   profile.AssignedNode,
		"region": profile.GeoRegion,
		"exp":    expiresAt.Unix(),
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(s.jwtKey)
	if err != nil {
		return nil, err
	}
	
	ctx := &SecurityContext{
		UID:         uid,
		Token:       tokenStr,
		ExpiresAt:   expiresAt,
		NodeID:      profile.AssignedNode,
		Permissions: []string{"traffic", "config"},
	}
	
	s.mu.Lock()
	s.sessions[uid] = ctx
	s.mu.Unlock()
	
	return ctx, nil
}

// ValidateToken 验证 JWT Token
func (s *UserService) ValidateToken(tokenStr string) (*SecurityContext, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return s.jwtKey, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}
	
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	
	uid, _ := claims["uid"].(string)
	
	s.mu.RLock()
	ctx, exists := s.sessions[uid]
	s.mu.RUnlock()
	
	if !exists || time.Now().After(ctx.ExpiresAt) {
		return nil, errors.New("session expired")
	}
	
	return ctx, nil
}

// AssignNode 动态分配节点
func (s *UserService) AssignNode(uid string, clientIP string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	profile, exists := s.profiles[uid]
	if !exists {
		return "", errors.New("user not found")
	}
	
	if s.geoSvc != nil {
		region := s.geoSvc.GetRegion(clientIP)
		profile.AssignedNode = s.geoSvc.GetNearestNode(region)
		profile.GeoRegion = region
	}
	
	return profile.AssignedNode, nil
}
