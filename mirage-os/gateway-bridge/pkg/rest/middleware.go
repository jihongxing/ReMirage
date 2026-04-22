package rest

import (
	"log"
	"net/http"
	"time"
)

// InternalAuthMiddleware 校验 X-Internal-Secret Header，不匹配时返回 401
// secret 为空时拒绝所有请求（Fail-Closed）
func InternalAuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				http.Error(w, "internal secret not configured", http.StatusForbidden)
				return
			}
			if r.Header.Get("X-Internal-Secret") != secret {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// responseRecorder 包装 ResponseWriter 以捕获状态码
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

// AccessLogMiddleware 记录每个请求的来源 IP、路径、鉴权结果和时间戳。
func AccessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rec, r)

		authResult := "ok"
		if rec.statusCode == http.StatusUnauthorized {
			authResult = "denied"
		} else if rec.statusCode == http.StatusForbidden {
			authResult = "forbidden"
		}

		log.Printf("[ACCESS] ip=%s path=%s method=%s status=%d auth=%s duration=%s ts=%s",
			r.RemoteAddr, r.URL.Path, r.Method, rec.statusCode, authResult,
			time.Since(start).Round(time.Microsecond), start.Format(time.RFC3339))
	})
}
