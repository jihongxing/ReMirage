# Mirage-OS Makefile
# 控制中心编译配置

.PHONY: all proto clean test docker test-security

# 变量定义
PROTO_DIR := api/proto
PROTO_OUT := api/proto
GO_OUT := .

# 默认目标
all: proto build

# 编译 Protobuf
proto:
	@echo "编译 Protobuf 文件..."
	@mkdir -p $(PROTO_OUT)/gateway
	@mkdir -p $(PROTO_OUT)/billing
	@mkdir -p $(PROTO_OUT)/cell
	protoc --go_out=$(GO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(GO_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/gateway.proto
	protoc --go_out=$(GO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(GO_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/billing.proto
	protoc --go_out=$(GO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(GO_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/cell.proto
	@echo "✅ Protobuf 编译完成"

# 编译 Go 服务
build:
	@echo "编译 Mirage-OS 服务..."
	@mkdir -p bin
	go build -o bin/api-gateway ./services/api-gateway
	go build -o bin/billing-service ./services/billing
	go build -o bin/cell-manager ./services/cell
	@echo "✅ 编译完成"

# 清理
clean:
	@echo "清理编译产物..."
	rm -rf bin/
	rm -f $(PROTO_OUT)/**/*.pb.go
	@echo "✅ 清理完成"

# 测试
test:
	@echo "运行测试..."
	go test -v ./...
	@echo "✅ 测试完成"

# Docker 构建
docker:
	@echo "构建 Docker 镜像..."
	docker build -t mirage-os:latest .
	@echo "✅ Docker 镜像构建完成"

# 安装依赖
deps:
	@echo "安装依赖..."
	go mod download
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "✅ 依赖安装完成"

# 初始化数据库
db-init:
	@echo "初始化数据库..."
	psql -U postgres -f deployments/db/schema.sql
	@echo "✅ 数据库初始化完成"

# 运行服务
run-api:
	@echo "启动 API Gateway..."
	./bin/api-gateway

run-billing:
	@echo "启动 Billing Service..."
	./bin/billing-service

run-cell:
	@echo "启动 Cell Manager..."
	./bin/cell-manager

# 安全回归测试
test-security:
	@echo "运行安全回归测试..."
	@echo "=== OS 侧安全回归测试 ==="
	cd api-server && npx jest --testPathPattern=security-regression --no-cache
	@echo "=== Gateway 侧安全回归测试 ==="
	cd ../mirage-gateway && go test ./pkg/threat/ -run SecurityRegression -v
	cd ../mirage-gateway && go test ./pkg/api/ -run SecurityRegression -v
	cd ../mirage-gateway && go test ./pkg/cortex/ -run SecurityRegression -v
	@echo "✅ 安全回归测试完成"

# 开发模式（热重载）
dev:
	@echo "启动开发模式..."
	air -c .air.toml
