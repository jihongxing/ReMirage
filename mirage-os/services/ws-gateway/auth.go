package wsgateway

import (
	"log"
	"mirage-os/pkg/redact"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// JWTAuthMiddleware 校验 WebSocket 连接的 JWT token。
// token 来源优先级：query param "token" > Authorization: Bearer <token>。
// 校验失败返回 401。
func JWTAuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 跳过健康检查
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			token := r.URL.Query().Get("token")
			if token == "" {
				auth := r.Header.Get("Authorization")
				token = strings.TrimPrefix(auth, "Bearer ")
				if token == auth {
					// 没有 Bearer 前缀，视为无 token
					token = ""
				}
			}

			if token == "" {
				log.Printf("[WARN] ws-gateway JWT: missing token from %s", redact.IP(r.RemoteAddr))
				http.Error(w, "unauthorized: missing token", http.StatusUnauthorized)
				return
			}

			_, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})
			if err != nil {
				log.Printf("[WARN] ws-gateway JWT: invalid token from %s: %v", redact.IP(r.RemoteAddr), err)
				http.Error(w, "unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
