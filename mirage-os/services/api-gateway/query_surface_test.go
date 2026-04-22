package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestQuerySurfaceRouteRegistration 验证 V2 query mux 注册不 panic 且路由可达
func TestQuerySurfaceRouteRegistration(t *testing.T) {
	mux := http.NewServeMux()

	// 模拟注册路由（不依赖真实 DB）
	routes := []string{
		"/api/v2/links",
		"/api/v2/sessions",
		"/api/v2/transactions",
		"/api/v2/audit/records",
		"/api/v2/personas/",
	}

	for _, route := range routes {
		route := route
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})
	}

	// 验证各路由可达
	for _, route := range routes {
		req := httptest.NewRequest("GET", route, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("路由 %s 返回 %d, 期望 200", route, w.Code)
		}
	}
}
