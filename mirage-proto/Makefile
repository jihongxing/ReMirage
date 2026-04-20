# mirage-proto — 协议中枢编译
# 依赖: protoc, protoc-gen-go, protoc-gen-go-grpc
#
# 安装:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

.PHONY: gen clean

gen:
	@mkdir -p gen
	protoc --go_out=gen --go_opt=paths=source_relative \
	       --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
	       mirage.proto
	@echo "✅ proto 生成完成: gen/"

clean:
	rm -rf gen/
