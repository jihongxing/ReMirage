// Package main - WebSocket Gateway 启动入口
package main

import (
	"log"
	"net/http"
	"os"

	wsgateway "mirage-os/services/ws-gateway"

	"github.com/go-redis/redis/v8"
	"github.com/rs/cors"
)

func main() {
	log.Println("🚀 Mirage-OS WebSocket Gateway 启动中...")

	// 1. 连接 Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	})

	// 2. 创建 Hub
	hub := wsgateway.NewHub(rdb)
	go hub.Run()

	// 3. 设置路由
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.HandleWS)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 4. CORS 配置
	handler := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	}).Handler(mux)

	// 5. JWT 校验中间件（JWT_SECRET 非空时启用）
	var finalHandler http.Handler = handler
	if jwtSecret := getEnv("JWT_SECRET", ""); jwtSecret != "" {
		finalHandler = wsgateway.JWTAuthMiddleware(jwtSecret)(handler)
		log.Println("🔒 JWT 校验中间件已启用")
	} else {
		log.Println("⚠️  JWT_SECRET 未设置，WebSocket 连接无鉴权保护")
	}

	// 6. 启动服务器
	port := getEnv("WS_PORT", "8080")
	log.Printf("✅ WebSocket 服务器已启动，监听端口: %s", port)
	log.Fatal(http.ListenAndServe(":"+port, finalHandler))
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
